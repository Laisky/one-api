# Change Manual: Time-of-Day Pricing Windows

- Status: Proposed
- Date: 2026-06-29
- Area: pricing / billing / channel config / model display / frontend
- Related: [`20260627_upstream-provider-expansion.md`](20260627_upstream-provider-expansion.md), tiered pricing (`ModelRatioTier`)

---

## 1. Background

### 1.1 Motivation

Several upstream LLM providers have begun charging **different prices depending on the
wall-clock time of the request**, independent of request size:

| Provider | Models | Window (wall-clock) | Timezone | Effect |
| --- | --- | --- | --- | --- |
| **DeepSeek** (canonical) | `deepseek-chat` (V3), `deepseek-reasoner` (R1) | `00:30–08:30` (≡ `16:30–00:30` UTC, crosses UTC midnight) | Asia/Shanghai (UTC+8) | Off-peak discount: **V3 −50%**, **R1 −75%** applied uniformly to input, cache-hit input, and output |
| Alibaba Qwen / DashScope | "valley token" tiers | `22:00–08:00` (crosses local midnight) | Asia/Shanghai | Off-peak credit multiplier (~60% cheaper) |
| Zhipu GLM | peak-hour multiplier | `14:00–18:00` | Asia/Shanghai | Peak **surcharge** (multiplier > 1) |

The pattern generalizes to two shapes that must both be expressible:

1. **Off-peak discount** — a cheaper window inside an otherwise-standard day (DeepSeek).
2. **Peak surcharge** — a more-expensive window (Zhipu); the "default" is the cheap baseline.

DeepSeek's discount is applied at **request-start time** and the off-peak window
**crosses midnight**; promotional pricing also tends to be bounded by a **date range**.
These three properties (timezone, midnight-crossing, optional date bounds) drive the data model below.

> Note: only the DeepSeek V3/R1 off-peak discount is treated as a verified, concrete
> anchor (used for the worked example and test fixtures). Other providers are cited to
> establish that the feature must support both discount and surcharge semantics; their
> exact schedules are not hard-coded into any adaptor by this proposal.

### 1.2 Current pricing architecture (recap)

Pricing today resolves through **three layers** and one **tier** sub-layer:

- `adaptor.ModelConfig` ([`relay/adaptor/interface.go:21`](../../relay/adaptor/interface.go#L21)) is the
  canonical in-memory pricing record: scalar ratios (`Ratio`, `CompletionRatio`,
  `CachedInputRatio`, `CacheWrite5mRatio`, `CacheWrite1hRatio`), a `Tiers []ModelRatioTier`
  ladder, and nested modality blocks (`Video`, `Audio`, `Image`, `Embedding`, `PerCall`).
- `model.ModelConfigLocal` ([`model/channel.go:116`](../../model/channel.go#L116)) is the
  channel-scoped override, persisted as JSON in the `Channel.ModelConfigs *string gorm:"type:text"`
  column ([`model/channel.go:48`](../../model/channel.go#L48)) and mirrors the pricing subset of `ModelConfig`.
- `relay/pricing` resolves a model to an effective `adaptor.ModelConfig` with **channel →
  adaptor-default → global** precedence: `ResolveModelConfig`, `ResolveModelConfigRatioOnly`
  ([`relay/pricing/resolver.go:22`](../../relay/pricing/resolver.go#L22),
  [`:240`](../../relay/pricing/resolver.go#L240)), plus modality-specific `ResolveAudioPricing`,
  `ResolveImagePricing`, and `GetVideoPricingWithThreeLayers`, `GetModelRatioWithThreeLayers`,
  `GetCompletionRatioWithThreeLayers` ([`relay/pricing/global.go`](../../relay/pricing/global.go)).
- **Tiers** are applied on top of the resolved config by `ResolveEffectivePricingFromConfig`
  ([`relay/pricing/global.go:504`](../../relay/pricing/global.go#L504)): it walks the sorted ladder and,
  per field, **`0` means "inherit from base"** (negative cached ratio means "free").
- Billing is computed by `quota.Compute` ([`relay/quota/quota.go:43`](../../relay/quota/quota.go#L43)),
  which calls `ResolveModelConfigRatioOnly` then `ResolveEffectivePricingFromConfig`.
- The canonical **request-start timestamp** already exists: `meta.StartTime time.Time`, set once
  per request to `time.Now()` ([`relay/meta/relay_meta.go:41`](../../relay/meta/relay_meta.go#L41),
  [`:117`](../../relay/meta/relay_meta.go#L117)) and threaded into every billing path.

### 1.3 The gap

There is no layer that varies any of the above by time of day. Off-peak/peak pricing can
only be approximated by manually editing channel pricing on a cron, which is error-prone and
cannot express "this rate only between 00:30 and 08:30 Asia/Shanghai".

### 1.4 Design goals

1. Add a pricing layer **above Tiers**: a request first selects an active **time window**;
   that window supplies a complete, self-consistent pricing overlay (ratios, tiers, video,
   image, audio, embedding, per-call). Tiers then resolve **within** the windowed prices.
2. Keep the existing `ModelConfig`/`ModelConfigLocal` fields **unchanged** as the **default**
   (used whenever no window is active). Add **one** new field carrying the time-segmented detail.
3. **Incremental override**: a window overlay is *sparse* — only fields it defines override the
   default; everything else inherits. (Same `0 == inherit` convention as Tiers.)
4. Windows are expressed in an explicit **timezone** and support **midnight-crossing** ranges,
   optional **day-of-week** and **date-range** bounds.
5. The active window is chosen by the **request-start time** (`meta.StartTime`) so a streaming
   request that crosses a boundary mid-stream is billed at **one** consistent rate.
6. **Zero behavior change and near-zero cost** when no window is configured (the overwhelming
   majority of models): the only added hot-path work is `len(TimeWindows) == 0` → return base.
7. Backward-compatible storage: old rows (no field) deserialize as "no windows".

### 1.5 Non-goals

- No new DB column or schema migration (reuse the existing JSON `ModelConfigs` text column).
- No rich window-builder UI in this iteration — windows are authored in the existing
  `model_configs` JSON textarea; the read-only price display is extended to *show* them.
- No per-request "spot price" beyond the configured windows; no automatic upstream price scraping.
- Realtime/websocket sessions and provider-tool surcharges keep their current
  single-evaluation-at-`StartTime` semantics (documented, not changed).
- Channel-level `PerCall` overrides/windows: `ModelConfigLocal` has no `PerCall` field today, so a
  channel JSON cannot override per-call pricing (only adaptor-shipped windows can). Adding
  `PerCall *PerCallPricingLocal` to `ModelConfigLocal` is a separate, pre-existing gap.

---

## 2. Design Overview

### 2.1 Where the new layer slots in

```
three-layer resolve (channel → adaptor default → global)
        │   → base adaptor.ModelConfig  (+ its TimeWindows list)
        ▼
[NEW] select active window by meta.StartTime, deep-merge its sparse overlay  ← time layer
        │   → effective adaptor.ModelConfig (TimeWindows cleared)
        ▼
ResolveEffectivePricingFromConfig(inputTokens, effective)                    ← tier layer
        │   → EffectivePricing
        ▼
quota.Compute / modality billing
```

The time layer is a **post-processing step on the resolved base config**, applied before tiers.
Because the winning layer's config carries its own `TimeWindows`, both adaptor-shipped schedules
(e.g. a DeepSeek adaptor default) and per-channel admin overrides work through the same path —
whichever layer wins supplies the windows, exactly as it already supplies `Tiers`.

### 2.2 Data model

Add `TimeWindows` to `adaptor.ModelConfig` and mirror it in `model.ModelConfigLocal`,
following the existing pattern of every other pricing field.

```go
// relay/adaptor/interface.go — added to ModelConfig
// TimeWindows holds time-of-day pricing overlays applied ABOVE Tiers. When a request's
// start time matches a window, that window's sparse Overlay is deep-merged onto this
// config before tier resolution. Empty/absent => time-invariant pricing (today's behavior).
TimeWindows []TimeWindow `json:"time_windows,omitempty"`

type TimeWindow struct {
    // Name is a human-readable label for display and billing audit logs (e.g. "deepseek-offpeak").
    Name string `json:"name,omitempty"`
    // TimeZone is an IANA location name (e.g. "Asia/Shanghai"). Empty => "UTC".
    TimeZone string `json:"timezone,omitempty"`
    // Ranges are wall-clock spans in TimeZone. End <= Start means the span crosses midnight.
    // At least one range is required.
    Ranges []ClockRange `json:"ranges"`
    // DaysOfWeek restricts the window to specific days (0=Sunday … 6=Saturday). Empty => every day.
    DaysOfWeek []int `json:"days_of_week,omitempty"`
    // DateFrom/DateTo bound the window to a calendar range (layout "2006-01-02", in TimeZone).
    // DateFrom is inclusive, DateTo is exclusive. Empty => unbounded on that side.
    DateFrom string `json:"date_from,omitempty"`
    DateTo   string `json:"date_to,omitempty"`
    // Overlay is a sparse ModelConfig: only non-zero/non-nil pricing fields override the base.
    // Overlay.TimeWindows MUST be empty (no recursion) — enforced by validation.
    Overlay ModelConfig `json:"overlay"`
}

type ClockRange struct {
    Start string `json:"start"` // "15:04" 24h, in the window's TimeZone
    End   string `json:"end"`   // "15:04"; End <= Start => the range wraps past midnight
}
```

`model.ModelConfigLocal` gets the structural twin (`TimeWindowLocal` / `ClockRangeLocal`) whose
`Overlay` is a `ModelConfigLocal`, reusing the existing `convertLocal*` machinery. The overlay
reuses the full config type for DRY-ness; only its **pricing** fields are honored and its own
`TimeWindows` is rejected during validation.

### 2.3 Merge semantics (incremental override)

`mergePricing(base, overlay)` — applied field-by-field; the resulting config has `TimeWindows`
cleared so downstream code cannot re-apply it:

| Field group | Rule |
| --- | --- |
| Scalar ratios (`Ratio`, `CompletionRatio`, `CachedInputRatio`, `CacheWrite5mRatio`, `CacheWrite1hRatio`) | overlay value **non-zero** overrides; `0` inherits base. Negative is preserved verbatim (means "free", per existing convention). |
| `Tiers []ModelRatioTier` | if `len(overlay.Tiers) > 0` → **replace** base tiers wholesale (a tier ladder is atomic; partial merge is ambiguous). Empty → inherit base tiers. The merged tiers are still subject to the normal `0 == inherit` rule **relative to the merged scalar base**. |
| Nested blocks (`Video`, `Audio`, `Image`, `Embedding`, `PerCall`) | overlay block `nil` → inherit base. Overlay block present → **deep field-merge** onto base block: **every** numeric sub-field (e.g. `PerSecondUsd`, `PromptRatio`, `UsdPerThousandCalls`, `TextTokenRatio`/`ImageTokenRatio`/…, `PricePerImageUsd`) follows the non-zero-override rule (`0` inherits, non-zero overrides, negative preserved); **string** sub-fields (`BaseResolution`, `DefaultSize`, `DefaultQuality`) inherit when empty, override when non-empty; **map** sub-fields (`ResolutionMultipliers`, `SizeMultipliers`, `QualityMultipliers`, `QualitySizeMultipliers`) merge **key-by-key** (overlay key wins, base keys preserved). If base block is `nil`, the overlay block is cloned in. |
| Capability metadata (`MaxTokens`, `ContextLength`, modalities, reasoning, etc.) | **not** time-varying; ignored in the overlay. |

The override rule is **uniform and recursive**: the same `0 == inherit` convention used for
top-level scalars applies inside every nested block. One consequence is that an overlay
**cannot** force a numeric field to `0` (zero is indistinguishable from "unset"); to express
"free", use a negative value where the field supports it (cached/cache-write ratios), otherwise
inherit by omission. `mergePricing`/`ApplyTimeWindow` return a config with `TimeWindows`
explicitly set to `nil` so a merged config can never re-apply a window (unit-tested invariant).

Tier inheritance example: base `Ratio=0.001, Tiers=[{Ratio:0, InputTokenThreshold:1_000_000}]`;
overlay `Ratio=0.002` (no `tiers`). Result: `Ratio=0.002`, base tiers inherited; the tier's
`Ratio:0` inherits the **merged** base `0.002` (not the original `0.001`). If the overlay had
provided `tiers`, they would replace the base ladder wholesale.

### 2.4 Active-window selection

`matchWindow(w, at)` returns true iff **all** hold:

1. **Date bounds.** Convert `at` to its wall-clock **calendar date** in `w.TimeZone`. `DateFrom`
   (parsed as a date in `w.TimeZone`) is **inclusive** (`localDate >= DateFrom`); `DateTo` is
   **exclusive** (`localDate < DateTo`). Either may be empty (open on that side). Example: `at =
   2026-06-29T23:59:59Z`, `w.TimeZone = Asia/Shanghai` → local date `2026-06-30`; with
   `DateTo = 2026-07-01` it is in-range, with `DateTo = 2026-06-30` it is not.
2. **Day-of-week.** If `DaysOfWeek` is set, the weekday of the **instant** `at` in `w.TimeZone`
   must be listed. The check is on the instant only, not the span of the range — so a
   midnight-crossing range combined with a day restriction is **asymmetric**: for range
   `22:00–06:00` with `DaysOfWeek=[Mon]`, a request Mon 23:30 matches, but Tue 05:30 does **not**
   (its weekday is Tue even though the clock time is inside the range). Documented; covered by a test.
3. **Time-of-day.** `at`'s wall-clock time-of-day `tod` in `w.TimeZone` falls inside **any** `Range`.
   Start is inclusive, End is exclusive in all cases:
   - normal (`Start < End`): `Start <= tod < End`. (`22:00–06:00` → `21:59` no, `22:00` yes.)
   - midnight-crossing (`End < Start`): `tod >= Start || tod < End`. (`22:00–06:00` → `22:00` yes,
     `05:59` yes, `06:00` no.)
   - all-day (`Start == End`, e.g. `00:00`–`00:00`): always matches (subject to day/date bounds).
     This is how a 24-hour promo is expressed — note `"24:00"` is **not** a valid `"15:04"` value.

**Precedence: first match wins.** `TimeWindows` is an ordered list; admins place the
highest-priority window first (e.g. a bounded promo before a recurring off-peak). This is
deterministic and documented. If no window matches, the base config is used unchanged.

Timezones are resolved via `loadLocationCached(tz string) (*time.Location, error)`, a
`sync.Map`-backed cache of `*time.Location` (no timezone utility exists in the codebase today
and `LoadLocation` does first-load I/O). Only **successful** loads are cached; invalid names are
not cached (they are rejected at save time, see §3.4, so a runtime miss is not expected). If a
load nonetheless fails inside `ApplyTimeWindow`, the window is **skipped** and resolution falls
back to the base config (billing never fails on a bad zone); the event is logged with the window
name. DST is handled for free: we convert the **instant** `at` into wall-clock in the IANA zone,
so a "00:30–08:30 local" window tracks DST transitions (Asia/Shanghai has none; America/* zones
shift, which is the intended "local wall-clock" behavior — covered by a test).

### 2.5 Timestamp source & threading

The single source of truth is `meta.StartTime`. It is:

- stable across channel-retry within a request (set once, cached),
- already present at every billing site,
- threaded into resolution via a new **explicit `at time.Time` parameter** on the resolver
  entry points (the compiler then forces every call site to supply it — no silent `time.Now()`
  that could mis-bill a boundary-crossing stream and is untestable).

`quota.ComputeInput` gains `RequestTime time.Time`; `quota.Compute` passes it through.
Display/admin-preview callers pass `time.Now()` (or a previewed instant) — so the
`active_time_window` shown in the UI reflects *display* time, which may legitimately differ from
the window a past request was billed under (billing always keys on that request's `StartTime`).
This distinction is documented in the display section.

### 2.6 Worked example (DeepSeek off-peak)

A channel `model_configs` entry for `deepseek-reasoner` (R1, −75% off-peak):

```json
{
  "deepseek-reasoner": {
    "ratio": 0.00000055,
    "completion_ratio": 4.0,
    "cached_input_ratio": 0.00000014,
    "time_windows": [
      {
        "name": "deepseek-offpeak",
        "timezone": "Asia/Shanghai",
        "ranges": [{ "start": "00:30", "end": "08:30" }],
        "overlay": {
          "ratio": 0.0000001375,
          "cached_input_ratio": 0.000000035
        }
      }
    ]
  }
}
```

- `completion_ratio` is omitted from the overlay → inherited (4.0), so output is automatically
  discounted because output price = `ratio * completion_ratio`.
- A request at 03:00 Asia/Shanghai bills at the overlay ratios; at 12:00 it bills at the base.
- A request starting 08:29 and streaming until 08:40 bills **entirely** at the off-peak rate
  (single evaluation at `StartTime`).

---

## 3. Change List

> Everything below is **proposed/new**: the `at time.Time` parameters, `ComputeInput.RequestTime`,
> the `TimeWindow*` types, and the display/i18n keys do not exist in the tree yet. Signature
> changes are breaking at the call-site level (compiler-enforced), which is intentional — it
> guarantees every caller is updated and no path silently bills at the wrong rate.

### 3.1 Core pricing types

| File | Change |
| --- | --- |
| [`relay/adaptor/interface.go`](../../relay/adaptor/interface.go) | Add `TimeWindow`, `ClockRange` structs; add `TimeWindows []TimeWindow` to `ModelConfig`. Add `Clone()` for `TimeWindow` (deep-clones `Overlay` + slices). Extend `ModelConfig` doc to note the time layer sits above Tiers. |
| [`model/channel.go`](../../model/channel.go#L116) | Add `TimeWindowLocal`, `ClockRangeLocal` (Overlay = `ModelConfigLocal`); add `TimeWindows []TimeWindowLocal` to `ModelConfigLocal` with `json:"time_windows,omitempty"`. **Append the field at the END of the struct** so a window-less config serializes byte-identically to today (no spurious diffs, clean rollback — see §7). |

### 3.2 Resolution & merge (`relay/pricing`)

| File | Change |
| --- | --- |
| `relay/pricing/timewindow.go` (new) | `ApplyTimeWindow(cfg adaptor.ModelConfig, at time.Time) adaptor.ModelConfig` (fast-path `len==0` returns input unchanged); `matchWindow(w, at)`; `mergePricing(base, overlay)` and per-block merge helpers; `loadLocationCached(tz string) (*time.Location, error)`. A `ratio-only` merge variant that touches only scalars + tiers for the hot billing path. |
| [`relay/pricing/resolver.go`](../../relay/pricing/resolver.go) | Thread `at time.Time` into `ResolveModelConfig` (:22), `ResolveModelConfigRatioOnly` (:240), `ResolveAudioPricing` (:48), `ResolveImagePricing` (:81); apply `ApplyTimeWindow` after the three-layer resolve, before returning. Carry `TimeWindows` through `convertLocalModelConfig` (:111) and `convertLocalModelConfigRatioOnly` (:273). |
| [`relay/pricing/global.go`](../../relay/pricing/global.go) | (a) **`cloneModelConfig` (:431) `PerCall` fix (prerequisite):** it does `clone := src` (shallow) and then `Clone()`s Video/Audio/Image/Embedding but **not** `PerCall`, leaving the pointer aliased to the cached source. Add `if src.PerCall != nil { clone.PerCall = src.PerCall.Clone() }`. This matters because `mergePricing` may mutate the `PerCall` block in place; without the clone, the mutation leaks into the shared cache and corrupts later requests (a real data race, caught by `-race`). (b) `GetGlobalModelConfigRatioOnly` (:254) must copy `TimeWindows` so the global layer carries them. (c) New `ResolveModelRatioAt` / `ResolveCompletionRatioAt` windowed scalar helpers — see *Scalar-shortcut paths* below. |

**`mergePricing` invariant.** `mergePricing(base, overlay)` and `ApplyTimeWindow` return a config
with `TimeWindows` set to `nil`, so a merged config can never re-trigger windowing. Unit-tested.

**Ratio-only path carries windows in all three layers.** `ResolveModelConfigRatioOnly` must
surface `TimeWindows` from whichever layer wins: `convertLocalModelConfigRatioOnly` (channel,
:273) copies the slice; the adaptor-default branch (:248) already shallow-copies it via
`clone := cfg`; `GetGlobalModelConfigRatioOnly` (global) is updated to copy it. `ApplyTimeWindow`
then runs on the result. The `else if pricingAdaptor != nil` branch of `quota.Compute` (:104) is
reached **only** when no config resolved in any layer — by construction there are no windows
there, so nothing is bypassed (an adaptor-shipped default *with* windows makes
`hasResolvedModelCfg == true` and flows through the windowed branch).

**Scalar-shortcut paths.** `GetModelRatioWithThreeLayers` (:348),
`GetCompletionRatioWithThreeLayers` (:381) and `GetVideoPricingWithThreeLayers` (:409) take only
`channelOverrides map[string]float64` (the legacy flat `model_ratio` map) and/or a pre-extracted
block — they **cannot see channel `TimeWindows`** (which live in the `ModelConfigLocal` map). The
design:

- The **main token path does not depend on them** for windowing. `quota.Compute` resolves the
  full windowed config; `usedModelRatio`/`usedCompletionRatio` come from `eff.*` (the windowed
  tier result) and **override** the non-windowed `input.ModelRatio` the controller computed via
  `GetModelRatioWithThreeLayers`.
- Paths that bill **directly off a scalar** — rerank per-call ([`rerank.go:59`](../../relay/controller/rerank.go#L59))
  and the **token-based audio fallback** ([`output_audio_billing.go:125`](../../relay/controller/output_audio_billing.go#L125)) —
  must switch to the new `ResolveModelRatioAt(modelName, channelConfigs, channelOverrides, provider, at)`
  (and completion twin), which resolves the full windowed `ModelConfig` and extracts the scalar.
- **Video** billing must resolve the full windowed config via `ResolveModelConfig(..., at)` and
  read `.Video`, because `GetVideoPricingWithThreeLayers` lacks the `channelConfigs` needed to see
  windows. `ResolveImagePricing`/`ResolveAudioPricing` already take `channelConfigs`, so they only
  need the `at` parameter to apply windows.
- The legacy `GetModelRatioWithThreeLayers` signature is retained for non-billing/display callers.

**Legacy flat-override precedence (decision).** Where a channel sets the legacy
`ChannelModelRatio`/`ChannelCompletionRatio` flat override for a model, `quota.Compute` (:80–:103)
uses that flat value as the base input ratio and does **not** replace it with the windowed
`eff.InputRatio`; windowed **tiers/cache/modality** values still apply via the resolved config.
This preserves today's precedence exactly. **Operator guidance:** to use time windows for a
model, configure its base price in the `model_configs` `ModelConfig` (not the legacy flat
`model_ratio` override) so the window can act on the base ratio. Covered by an explicit test
(flat-override + window).

### 3.3 Billing plumbing

| File | Change |
| --- | --- |
| [`relay/quota/quota.go`](../../relay/quota/quota.go#L18) | Add `RequestTime time.Time` to `ComputeInput`; pass it into `ResolveModelConfigRatioOnly` (:53) and into the `resolveCompletionRatio` → `GetCompletionRatioWithThreeLayers` fallback (:238/:251). No other math changes — `resolveCompletionRatio` already prefers the (now windowed) resolved `CompletionRatio`, and tiers operate on the windowed config at :75. |
| [`relay/controller/helper.go`](../../relay/controller/helper.go) (:194,:292,:411), [`response_billing.go`](../../relay/controller/response_billing.go#L136), [`claude_messages_billing.go`](../../relay/controller/claude_messages_billing.go#L89), [`mcp_helpers.go`](../../relay/controller/mcp_helpers.go#L197), [`controller/realtime.go`](../../controller/realtime.go#L221) | Set `RequestTime: meta.StartTime` on every `quota.Compute` call. |
| [`relay/controller/output_billing_context.go`](../../relay/controller/output_billing_context.go), [`output_image_billing.go`](../../relay/controller/output_image_billing.go), `output_audio_billing.go`, `output_video_billing.go`, [`image.go`](../../relay/controller/image.go#L317), `audio.go`, `video.go` | Thread `meta.StartTime` into modality pricing: **image** via `ResolveImagePricing(..., at)`, **audio** via `ResolveAudioPricing(..., at)` and the token fallback via `ResolveModelRatioAt`, **video** by resolving the full windowed config (`ResolveModelConfig(..., at)`) and reading `.Video` (since `GetVideoPricingWithThreeLayers` lacks `channelConfigs`). |

Note: the rerank/ocr/audio/video "legacy" billing paths use `PostConsumeQuotaWithLog`. With the
scalar-path routing above, their **prices are windowed correctly**; only the billing *log* lacks
`StartTime` (so the active-window *name* is not recorded in those logs). Migrating them to
`PostConsumeQuotaDetailed` to capture window provenance is **out of scope** here (Phase 3); the
interim limitation is called out in Acceptance Criteria.

### 3.4 Storage, validation, normalization, migration

| File | Change |
| --- | --- |
| [`model/channel.go:282`](../../model/channel.go#L282) `normalizeModelConfigLocal` | Add `normalizeTimeWindowsLocal(windows []TimeWindowLocal) ([]TimeWindowLocal, error)` — validates each window (tz/ranges/dates), trims/defaults (`timezone` → `"UTC"`), preserves window order (order = precedence), recurses normalization into each `Overlay`, and returns the normalized slice or a wrapped error; call it from `normalizeModelConfigLocal` and carry the result into the rebuilt `normalized` struct. **Adjacent fix:** the function rebuilds field-by-field and currently **drops `Embedding`** (it is never assigned to `normalized`); add `if cfg.Embedding != nil { normalized.Embedding = cfg.Embedding }` alongside the new `TimeWindows` so neither is silently lost on save. (Note: `ModelConfigLocal` has no `PerCall` field today — channel-level per-call overrides/windows are a separate gap, see §1.5.) |
| [`model/channel.go:874`](../../model/channel.go#L874) `validateModelPriceConfigs` | Validate each window: IANA timezone parses (`time.LoadLocation`), each `Start`/`End` parses as `"15:04"` (so `"24:00"` is rejected; `Start == End` is allowed and means all-day), ≥1 range, `DaysOfWeek ∈ [0,6]`, `DateFrom`/`DateTo` parse as `"2006-01-02"` with `From < To`, `Overlay` carries ≥1 pricing field, `Overlay.TimeWindows` is empty (reject recursion), overlay ratios obey the same sign rules as base/tier validation. |
| [`model/channel.go:1364`](../../model/channel.go#L1364)/[`:1382`](../../model/channel.go#L1382) `Get/SetModelPriceConfigs` | No structural change — `omitempty` on `time_windows` gives transparent backward compat (old rows → `nil`). |

No DB migration: the `ModelConfigs` column is already `type:text` JSON.

### 3.5 Display API

| File | Change |
| --- | --- |
| [`controller/model.go:294`](../../controller/model.go#L294) `ModelDisplayInfo` | Add `TimeWindows []TimeWindowDisplay `json:"time_windows,omitempty"`` and `ActiveTimeWindow string `json:"active_time_window,omitempty"``. `ActiveTimeWindow` = the `Name` of the **first** window matching at server `time.Now()` (empty if none), used for an "off-peak active now" badge. It is computed at *display* time and is intentionally decoupled from any past request's billing window (which keys on that request's `StartTime`, see §2.5). |
| [`controller/model.go:900-1053`](../../controller/model.go#L900) `buildChannelModels` | Convert resolved `ModelConfig.TimeWindows` into display form (schedule + the overlay rendered as prices via the existing `convertRatioToPrice` at :690); compute `ActiveTimeWindow`. |
| [`router/api.go:21`](../../router/api.go#L21) `GET /api/models/display` (+ `:105`/`:110` channel pricing routes) | No route change; payload gains the new fields. |

### 3.6 Frontend (three mirrors) + i18n

Channel editing uses a **raw `model_configs` JSON textarea** in all three frontends, so admins
can author windows immediately. Required work is validation tolerance + read-only display.

| Frontend | Files | Change |
| --- | --- | --- |
| **modern** (primary) | [`web/modern/src/pages/channels/components/ChannelModelSettings.tsx`](../../web/modern/src/pages/channels/components/ChannelModelSettings.tsx) (textarea + `sanitizeJsonInput`), `schemas.ts`/`helpers.ts` | Accept and (lightly) validate `time_windows` in the JSON; update placeholder/example. |
| **modern** read-only | [`web/modern/src/pages/models/ModelPricingModal.tsx`](../../web/modern/src/pages/models/ModelPricingModal.tsx) (`ModelDisplayData` iface + a new pricing section) | Render a "Time-of-day pricing" section: each window's schedule, timezone, day/date bounds, overlaid prices; highlight the active one. |
| **air** | [`web/air/src/pages/Channel/EditChannel.js`](../../web/air/src/pages/Channel/EditChannel.js) (`validateModelConfigs` :20–123), `web/air/src/pages/Models/index.js` | Tolerate `time_windows` in validation; optionally surface an "off-peak" indicator in the price table. |
| **berry** | [`web/berry/src/views/Channel/component/EditModal.js`](../../web/berry/src/views/Channel/component/EditModal.js) (Yup `model_configs` :76–128), `web/berry/src/views/Models/index.js` | Same: tolerate the field; optionally surface an indicator. |
| **i18n** | `web/modern/src/i18n/locales/{en,es,fr,ja,zh}/models.json` | Add a fixed key set under `models.detail.*`: `time_pricing` (section header), `window_name`, `window_schedule`, `window_timezone`, `window_days`, `window_dates`, `window_active` (badge), and `weekday_0`…`weekday_6` (translated day names). **All five locales** (en, zh, es, fr, ja) must carry every key — enforced by the i18n lint/test. Day names and times are rendered via the i18n library's locale-aware formatters, not hardcoded layouts. |

Frontend validators today are field-presence checks, not key allowlists, so an unknown
`time_windows` key is not rejected even before the validation work lands (safe rollout).

### 3.7 Docs

- Document the window schema, merge rules, precedence, timezone/DST/midnight semantics, and the
  DeepSeek worked example in the model-pricing / channel-configuration docs. **Required**, not optional.
- Update the `README` pricing section to note time-of-day windows are supported (it enumerates
  per-model config fields — see the memory rule on keeping README model/pricing lists current).

---

## 4. Test Matrix

| # | Category | Case | Where |
| --- | --- | --- | --- |
| 1 | Schedule match | normal range in-window / out-of-window | `relay/pricing/timewindow_test.go` |
| 2 | Schedule match | midnight-crossing range (DeepSeek 16:30–00:30 UTC): 23:00 in, 09:00 out | timewindow_test |
| 3 | Schedule match | timezone correctness: same instant in/out depending on `Asia/Shanghai` vs `UTC` | timewindow_test |
| 4 | Schedule match | DST zone (`America/New_York`): "00:30–08:30 local" tracks the offset shift across a DST boundary | timewindow_test |
| 5 | Schedule match | `DaysOfWeek` restriction; **asymmetry** with a midnight-crossing range (`Mon 23:30` matches, `Tue 05:30` does not for `DaysOfWeek=[Mon]`) | timewindow_test |
| 6 | Schedule match | `DateFrom`/`DateTo` bounds evaluated as **local calendar dates** in `w.TimeZone` (instant `2026-06-29T23:59:59Z` ⇒ `2026-06-30` in Shanghai); inclusive From / exclusive To edges | timewindow_test |
| 7 | Precedence | overlapping windows → **first match wins**; ordering changes result | timewindow_test |
| 8 | Merge — scalars | overlay non-zero overrides; `0` inherits; negative cached-ratio preserved as "free" | timewindow_test |
| 9 | Merge — completion | overlay overrides `ratio` only → output price drops via inherited `completion_ratio` | timewindow_test |
| 10 | Merge — tiers | non-empty overlay tiers replace base tiers wholesale; **empty** overlay tiers inherit base; replaced tier with `Ratio:0` inherits the **merged** scalar base (not the original) | timewindow_test |
| 11 | Merge — modality | `Image`/`Video`/`Audio`/`Embedding`/`PerCall` field+map merge: numeric sub-fields `0`-inherit, string sub-fields empty-inherit, map keys union (overlay wins) | timewindow_test |
| 12 | Fast path | `TimeWindows == nil` → `ApplyTimeWindow` returns identical config, no alloc | timewindow_test + benchmark |
| 13 | Clone safety | `cloneModelConfig` now deep-clones `PerCall`; overlay-merged config does not mutate cache/source (`-race`) | `relay/pricing/global_test.go` |
| 14 | Resolver | `ResolveModelConfig*` / `ResolveImagePricing` / `ResolveAudioPricing` / `GetVideoPricingWithThreeLayers` honor `at` (in-window vs out) across all three layers (channel / adaptor-default / global) | `relay/pricing/resolver_test.go` |
| 15 | Billing | `quota.Compute` with `RequestTime` inside vs outside window → correct quota for input, cached input, cache-write, output | `relay/quota/quota_test.go` |
| 16 | Billing | streaming request crossing a boundary uses `StartTime` → single rate end-to-end | quota/integration test |
| 17 | Billing | legacy `ChannelModelRatio` flat override + window: base input ratio stays the flat value, windowed tiers/cache still apply (documented precedence) | quota_test |
| 17b | Billing | **adaptor-default-only** model (no channel config) with adaptor-shipped windows is windowed via the `hasResolvedModelCfg==true` branch | quota_test |
| 17c | Billing | **scalar-shortcut paths**: rerank per-call and audio token fallback honor the window via `ResolveModelRatioAt`; video per-second via full windowed config `.Video` | `rerank_test.go` / `output_audio_billing_test.go` / `output_video_billing_test.go` |
| 17d | Billing | embedding multimodal cost (`computeEmbeddingPromptCost`) applies the windowed `Embedding` block through `quota.Compute` | quota_test |
| 18 | Storage | `Set`→`Get` round-trips `time_windows` (incl. nested overlay) intact | `model/channel_field_migration_test.go` |
| 19 | Storage | old JSON without `time_windows` deserializes to `nil` (backward compat) | `model/channel_migration_integration_test.go` |
| 20 | Storage | `normalizeModelConfigLocal` carries `TimeWindows` **and** `Embedding` (regression for the adjacent drop) | channel_test |
| 21 | Validation | reject: bad timezone, bad `HH:MM`, empty ranges, day ∉ [0,6], `From ≥ To`, empty overlay, recursive `Overlay.TimeWindows` | channel_test |
| 22 | Display | `/api/models/display` payload includes `time_windows` + `active_time_window`; prices rendered via `convertRatioToPrice` | `controller/model_display_test.go` |
| 23 | Frontend | modern `ModelPricingModal` renders windows + active badge; all 3 editors accept the field; i18n keys present in all 5 locales | web unit / lint |
| 24 | Concurrency | concurrent resolution with windows under `-race`; tz-cache has no data race | timewindow_test |
| 25 | Rollback/compat (§7) | old-binary simulation: a `ModelConfigLocal`-shaped struct **without** `TimeWindows` unmarshals new JSON without error and re-marshals dropping only `time_windows` (C3/D1); a window-less config serializes byte-identically old↔new (C2); new binary reads pre-feature JSON with unchanged billing (C5) | `model/channel_field_migration_test.go` |

---

## 5. Acceptance Criteria

1. **Default unchanged**: with no `time_windows`, every existing pricing/billing test passes
   unchanged, and `ApplyTimeWindow` is a no-op (benchmark within noise of baseline).
2. **Correct windowed billing**: for the DeepSeek R1 fixture, a request at 03:00 Asia/Shanghai
   is billed at −75% (input, cached input, output) and a request at 12:00 at the base rate;
   verified end-to-end through `quota.Compute`.
3. **Single-rate streaming**: a request starting in-window and finishing out-of-window (and vice
   versa) is billed entirely at the `StartTime` window.
4. **Incremental override honored**: an overlay that sets only `ratio` leaves tiers, cache, and
   modality prices inherited; an overlay block merges per-field/per-key without dropping base entries.
5. **Timezone/DST/midnight correctness**: tests 2–6 pass, including a DST-shifting zone.
6. **Precedence deterministic**: first-match-wins verified; reordering windows changes the result.
7. **Backward-compatible storage**: old rows load as no-windows; round-trip preserves windows;
   `Embedding` no longer dropped by normalize.
8. **Validation**: all malformed-window cases (test 21) are rejected at `SetModelPriceConfigs`
   with wrapped errors (`github.com/Laisky/errors/v2`).
9. **Clone safety**: `-race` suite clean; `PerCall` deep-cloned; no overlay merge mutates shared state.
10. **Display + i18n**: `/api/models/display` exposes windows and the active window; modern UI
    renders them; i18n keys exist in **en, zh, es, fr, ja**.
11. **Scalar-shortcut correctness**: tests 17b–17d pass — adaptor-default-only, rerank/audio/video
    direct-scalar, and embedding paths are all windowed; no billing path silently uses base prices
    while a window is active.
12. **Rollback/compat (§7)**: a struct without `TimeWindows` unmarshals new JSON without error and
    re-marshals dropping only `time_windows`; a window-less config serializes byte-identically
    old↔new; the new binary reads pre-feature JSON with identical billing (test 25).
13. **`make lint` / `make test` green**; no bare returned errors (all wrapped).

> **Interim limitation (accepted):** the legacy `PostConsumeQuotaWithLog` paths (rerank/ocr/audio/
> video) bill at the correct windowed price but do **not** record the active window's *name* in the
> billing log. Window provenance in logs lands in Phase 3 when those paths move to
> `PostConsumeQuotaDetailed`.

### Phase gate

- **Phase 1 (core, behind nothing)**: types + `ApplyTimeWindow` + resolver threading + `quota`
  plumbing + storage/validation + unit/billing tests. Ship-able; default behavior identical.
- **Phase 2 (visibility)**: display API + modern read-only UI + i18n + frontend validation tolerance.
- **Phase 3 (optional, separate)**: window-builder form UI; migrate legacy billing paths to
  `PostConsumeQuotaDetailed`; optional adaptor-shipped default schedules (e.g. DeepSeek).

---

## 6. Locked Decisions & Risks

| Topic | Decision |
| --- | --- |
| Overlay sparseness | `0`/`nil` = inherit; non-zero = override; negative = free (matches Tiers). |
| Tiers in overlay | non-empty overlay tiers **replace** base tiers wholesale. |
| Window precedence | ordered list, **first match wins**. |
| Timestamp | `meta.StartTime` (request start), threaded as explicit `at time.Time`. |
| Timezone default | empty `timezone` = `"UTC"`; resolved via cached `time.LoadLocation`. |
| Recursion | `Overlay.TimeWindows` must be empty (validation-enforced). |
| Storage | reuse JSON `ModelConfigs` column; `omitempty`; no migration. |
| Legacy flat override | `ChannelModelRatio` flat override keeps today's precedence over the windowed base ratio; windowed tiers/cache/modality still apply. Operators should price via `ModelConfig` (not the flat override) to window the base ratio. |
| Scalar-shortcut paths | direct-scalar billing (rerank, audio token fallback, video) routes through windowed full-config resolution (`ResolveModelRatioAt` / `ResolveModelConfig(..., at)`); the legacy `Get*WithThreeLayers` scalar funcs stay for non-billing/display. |
| `PerCall` overlays | only adaptor-shipped windows can overlay `PerCall` today (`ModelConfigLocal` has no `PerCall` field); channel-level `PerCall` is a separate gap (§1.5). |

**Risks & mitigations**

- *Hot-path cost*: gated by `len(TimeWindows)==0` fast path; tz objects cached; ratio-only merge
  avoids building modality blocks for token billing. → benchmark gate (test 12).
- *Multiple resolver entry points*: a single `ApplyTimeWindow`/`mergePricing` helper is reused by
  all of them; explicit `at` parameter makes the compiler enforce threading. → tests 13–14.
- *Boundary-crossing ambiguity*: resolved by always using `StartTime`. → test 16.
- *Cross-midnight + day-of-week corner*: documented (weekday evaluated at the instant); covered by
  tests 2 & 5.
- *Frontend drift across three mirrors*: validation tolerance lands first (safe), display is
  additive; i18n enforced across all 5 locales in acceptance.

---

## 7. Compatibility & Rollback Safety

This change is **purely additive** and **forward/backward compatible**. A site can upgrade,
write `time_windows`, and later **roll back to the old binary without any data or business-logic
corruption**. The guarantees below are each backed by a verified codebase fact.

| # | Guarantee | Why it holds (verified) |
| --- | --- | --- |
| C1 | **No DB schema change.** | Windows live inside the existing `Channel.ModelConfigs *string gorm:"type:text"` JSON column ([`model/channel.go:48`](../../model/channel.go#L48)). No column is added or dropped, so `AutoMigrate` under either binary is unaffected. (GORM never drops columns regardless, but we add none.) |
| C2 | **No existing field changes.** | Every current `ModelConfig`/`ModelConfigLocal` field keeps its name, type, and meaning. `TimeWindows` is new, `omitempty`, and appended at the **end** of the struct (§3.1) — so a config without windows serializes **byte-identically** to today. Existing stored JSON is never rewritten until an admin explicitly edits that channel. |
| C3 | **Old binary ignores the new key (no error).** | Verified: the repo contains **zero** `json.DisallowUnknownFields` usages. `GetModelPriceConfigs` ([`:1364`](../../model/channel.go#L1364)) and the migration check ([`:719`](../../model/channel.go#L719)) both use lenient `json.Unmarshal`. An old binary decoding new JSON into the old `ModelConfigLocal` (which lacks `TimeWindows`) silently drops the unknown `time_windows` key — no parse error, no failed channel load, no billing break. |
| C4 | **Old binary bills exactly as pre-feature.** | With `time_windows` ignored, the old binary resolves prices from the default `ModelConfig` as it always did. It simply applies no time discount/surcharge (which it never could). Billing stays correct under the old rules — never over/undercharged beyond prior behavior. |
| C5 | **New binary tolerates old data.** | Old rows have no `time_windows` → `nil` → `ApplyTimeWindow` is a no-op → behavior identical to today. New `normalize`/`validate` skip all window checks when `len(TimeWindows)==0`, so they never reject a pre-existing config. |
| C6 | **Nested overlay invisible to old code.** | `time_windows[].overlay` is nested; old code iterates only **top-level** model keys (still model names) and never descends into it. |
| C7 | **API/frontend additive both ways.** | Display payload gains only `omitempty` fields; JSON-over-HTTP consumers ignore unknown fields in both directions (old FE ↔ new BE, new FE ↔ old BE). Old frontend validators are field-presence checks, not key allowlists, so an admin editing JSON that still contains `time_windows` is not blocked. |
| C8 | **Mixed (rolling) deployment is safe.** | A channel saved by a new node (with windows) read by an old node bills at default; a channel saved by an old node (windows stripped, see D1) read by a new node also bills at default. Both paths are crash-free and internally consistent. |

This is the **same additive pattern** by which `Tiers`, `Video`, `Audio`, `Image`, and `Embedding`
were each added to `ModelConfigLocal`; the compatibility behavior is established precedent, not novel.

### The one documented caveat (graceful degradation, **not** corruption)

| # | Behavior | Characterization & mitigation |
| --- | --- | --- |
| D1 | **An old binary that re-SAVES an edited channel strips that channel's `time_windows`.** | Because the old `ModelConfigLocal` struct has no such field, `SetModelPriceConfigs` → `normalizeModelConfigLocal` re-marshals without it. This is **loss of the new feature's config for that one channel**, *not* corruption of any business logic. It happens **only** on an explicit admin save under the old binary — merely running the old binary (read/bill/display) leaves the stored JSON untouched. It is bounded (only the edited channel, only the `time_windows` key) and idempotent. Mitigation: if a rollback is anticipated, avoid editing windowed channels under the old binary; on re-upgrade, re-enter windows. This is identical to how every prior additive field behaves on old-binary save. |

**Net:** no scenario causes data corruption or breaks old business logic. The worst case (D1) is a
bounded, explicit-action-only loss of *new-feature* configuration, with the old system continuing
to bill correctly at default prices throughout.

---

## Appendix A — More overlay examples

Peak surcharge (Zhipu-style; default is the cheap baseline, peak window multiplies up):

```json
{
  "glm-x": {
    "ratio": 0.000001,
    "time_windows": [
      { "name": "peak", "timezone": "Asia/Shanghai",
        "ranges": [{ "start": "14:00", "end": "18:00" }],
        "overlay": { "ratio": 0.000003 } }
    ]
  }
}
```

Bounded promo taking precedence over a recurring off-peak (first-match-wins ordering):

```json
{
  "model-y": {
    "ratio": 0.000002,
    "time_windows": [
      { "name": "launch-promo", "timezone": "UTC", "date_from": "2026-07-01", "date_to": "2026-08-01",
        "ranges": [{ "start": "00:00", "end": "00:00" }], "overlay": { "ratio": 0.000001 } },
      { "name": "nightly-offpeak", "timezone": "UTC",
        "ranges": [{ "start": "22:00", "end": "06:00" }], "overlay": { "ratio": 0.0000015 } }
    ]
  }
}
```

## Appendix B — Key file:line index

| Concern | Anchor |
| --- | --- |
| Canonical config | [`relay/adaptor/interface.go:21`](../../relay/adaptor/interface.go#L21) (`ModelConfig`), `:252` (`ModelRatioTier`) |
| Local/persisted config | [`model/channel.go:116`](../../model/channel.go#L116) (`ModelConfigLocal`), `:48` (column) |
| Normalize / validate / get / set | `model/channel.go` `:282` / `:874` / `:1364` / `:1382` |
| Three-layer resolve | [`relay/pricing/resolver.go:22`](../../relay/pricing/resolver.go#L22)/`:48`/`:81`/`:240` |
| Scalar/video resolve + clone | [`relay/pricing/global.go:348`](../../relay/pricing/global.go#L348)/`:381`/`:409`/`:431` (clone) |
| Tier application | [`relay/pricing/global.go:504`](../../relay/pricing/global.go#L504) |
| Billing compute | [`relay/quota/quota.go:43`](../../relay/quota/quota.go#L43) (`Compute`), `:18` (`ComputeInput`) |
| Request start time | [`relay/meta/relay_meta.go:41`](../../relay/meta/relay_meta.go#L41)/`:117` |
| Display | [`controller/model.go:294`](../../controller/model.go#L294)/`:900` |
| Display route | [`router/api.go:21`](../../router/api.go#L21) |
| Frontend editors | modern `ChannelModelSettings.tsx`; air `EditChannel.js`; berry `EditModal.js` |
| Frontend display | modern `ModelPricingModal.tsx`; air/berry `Models/index.js` |
| i18n | `web/modern/src/i18n/locales/{en,es,fr,ja,zh}/models.json` |
