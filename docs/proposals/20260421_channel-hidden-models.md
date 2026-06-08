# Proposal: Channel Hidden Models

- Status: Draft
- Author: @Laisky
- Created: 2026-04-21
- Owners: backend, frontend

## 1. Background

Today a channel declares its supported/requestable model names in
`Channel.Models` and can rewrite incoming model names via
`Channel.ModelMapping`. Every non-empty name in `Channel.Models` is
currently treated as:

1. Registered as an `Ability` so it can be selected by the load balancer.
2. Advertised through user-facing model discovery paths that are backed by
   abilities, such as `GET /v1/models` and `/api/models/display`.
3. Accepted as a direct `model=<name>` request from end users.

Operators want to expose a **single public alias** while still running
different underlying upstream models across channels, e.g.:

| Channel | Upstream models | `ModelMapping` | Public alias |
|---------|-----------------|----------------|--------------|
| 1       | `A`             | `C -> A`       | `C`          |
| 2       | `B`             | `C -> B`       | `C`          |

For this to route today, each channel must also include the public alias
in `Channel.Models` (for example `Models="A,C"` and `ModelMapping={"C":"A"}`),
because mapping entries alone do not create ability rows. The upstream
target (`A`) must stay in `Channel.Models` so channel testing and explicit
channel routing can still verify that the channel really serves it.

The desired behavior:

- `GET /v1/models` returns only `C` (plus any other non-hidden models).
- A request with `model=C` is load-balanced across Channel 1 and 2 and
  rewritten to `A`/`B` before hitting the upstream.
- A request with `model=A` or `model=B` is rejected (no matching ability).
- Administrators can still test Channel 1 with `model=A` when they pick
  the channel explicitly (admin token suffix or proxy channel route),
  because the upstream model name remains valid for the channel.

Today this can only be approximated by removing `A`/`B` from
`Channel.Models`, which also erases the upstream-capability record and
disables admin-side testing against the underlying model. We want a
dedicated field that captures "serve this model, but do not expose it
publicly".

This proposal implements **Option A** from the design discussion:
`HiddenModels` affects `/v1/models` visibility and **direct** user
requests, but does **not** interfere with `ModelMapping` targets or
`SpecificChannelId` admin flows.

Important endpoint split in the current implementation:

- `GET /v1/models` uses `controller.ListModels`, `TokenAuth`, and
  group abilities. This is the primary public availability contract.
- `GET /api/models/display` is an optional-auth model browser used by
  Modern pages and must not leak hidden upstream names to anonymous or
  normal users.
- `GET /api/models` (`DashboardListModels`) and
  `GET /api/channel/models` (`ListAllModels`) are admin/catalog helpers,
  not routing authority. In the current router `/api/models` uses
  `UserAuth`, so this proposal must either tighten it to admin-only access
  or filter the union of hidden model names for non-admin callers.

## 2. Goals / Non-goals

### Goals
- Allow each channel to declare a set of models that are served but not
  publicly discoverable or directly invocable.
- Preserve load balancing across channels that share a public alias.
- Keep backwards compatibility: channels without `HiddenModels` behave
  identically to today.
- Allow admins to still test a channel against a hidden upstream model
  via `SpecificChannelId`.
- Make hidden models completely invisible to non-admin users. Users may
  call them only through public `ModelMapping` aliases, and no
  user-facing API response, UI page, error body, token helper, pricing
  display, or user log view should reveal the hidden upstream name.

### Non-goals
- Per-token hidden model overrides (tokens already have their own
  allowlist mechanism).
- Per-group hidden model overrides.
- Hiding hidden model names from administrator-only channel
  configuration, channel tests, or internal operational logs.
- Changing the existing mapping/alias semantics for channels that do not
  opt in.

## 3. Design Summary

Add a new channel-level field `HiddenModels` (JSON array of strings)
storing the names the channel serves but should not expose publicly.

Invariants:

1. `HiddenModels` ⊆ `Models` in practice. We will not enforce this with
   a database constraint. The Modern UI warns when a hidden entry is not
   in `Models`, and API validation rejects malformed JSON. Entries that
   are not in `Models` are treated as no-ops after normalization.
2. A user-facing model is exposed iff it appears in `Models` **and not**
   in `HiddenModels`.
3. An ability is created iff the model is in `Models` **and not** in
   `HiddenModels` (so the load balancer never selects a channel for a
   hidden model name).
4. Mapping targets are resolved after channel selection and do not go
   through the hidden-model check, so `C -> A` still rewrites correctly.
5. Admin paths that supply `SpecificChannelId` continue to call
   `SupportsModel`, which ignores `HiddenModels`; this preserves admin
   testing for upstream models.
6. A public alias that should be routed must be present in `Models` and
   absent from `HiddenModels`; hiding a mapping source such as `C` makes
   that alias unreachable through normal load balancing.
7. User-facing representations use the public/origin model name. The
   mapped hidden target can be used internally for routing, upstream URL
   construction, and billing calculation, but it must not be returned to
   non-admin users in model lists, token helpers, user logs, pricing
   displays, error responses, or playground suggestions.

## 4. Change List

### 4.1 Backend — data model

- [model/channel.go](../../model/channel.go)
  - Add `HiddenModels *string \`json:"hidden_models" gorm:"type:text"\`` to
    the `Channel` struct (JSON array of strings, nullable for backward
    compatibility).
  - Add `GetHiddenModels() map[string]struct{}`. It returns lowercase
    keys for case-insensitive membership, trims whitespace, skips empty
    strings, and tolerates `NULL`/empty/`[]`.
  - Add `IsModelHidden(name string) bool`.
  - Ensure new functions have comments that start with the function name,
    matching the repo's Go style.
  - Add a small validation/normalization helper for API payloads so
    `hidden_models` must be a JSON array of strings when provided; store
    the normalized JSON string with duplicates removed and original casing
    preserved.
  - Leave `SupportsModel` unchanged so `SpecificChannelId` paths continue
    to work for hidden upstream names.
  - Extend `channelSortFields` only if we later sort by hidden state (not
    in this change).

- Schema migration: add a `hidden_models TEXT` column. GORM's
  `AutoMigrate` covers MySQL/Postgres; SQLite requires the additive
  column, which `AutoMigrate` also handles. No back-fill needed —
  `NULL`/empty string means "no hidden models".

### 4.2 Backend — ability registration

- [model/ability.go](../../model/ability.go) `Channel.AddAbilities`
  (line 73): skip model names present in `GetHiddenModels()`. The
  current `utils.DeDuplication` helper does **not** trim names or remove
  empty strings, so build abilities from `GetSupportedModelNames()` (or
  explicitly trim/drop empty values) before applying the hidden filter.
  `UpdateAbilities` delegates to `AddAbilities`, so only one call site
  needs to change.

- No change to `Ability` schema; hidden models simply never produce
  rows. On any subsequent edit, `UpdateAbilities` deletes and re-adds,
  so the database rows are correct immediately.

- Add a cache invalidation helper used by `Insert`, `Update`,
  `BatchInsertChannels`, status changes, and ability rebuilds:
  - Rebuild `group2model2channels` via `InitChannelCache()`.
  - Clear the in-process `getGroupModelsV2Cache` entries for affected
    groups (or replace the cache if per-key delete is unavailable).
  - Delete Redis keys `group_models:<group>` and
    `group_models_v2:<group>` when Redis is enabled.
  - Treat a channel group edit as an old-groups-plus-new-groups
    invalidation. If the previous `Group` value cannot be loaded safely,
    fall back to clearing all group-model caches to avoid stale models in
    groups the channel no longer serves.

Without this, `GET /v1/models`, `/api/user/available_models`, and the
Modern playground can show hidden models until `GroupModelsCacheSeconds`
expires (`config.SyncFrequency`, 120 seconds by default).

### 4.3 Backend — public listing endpoints

- [controller/model.go](../../controller/model.go)
  - `ListModels` (line 774): this is the real `GET /v1/models` handler.
    It already starts from `CacheGetGroupModelsV2`, which draws from the
    `Ability` table. After 4.2 and cache invalidation, hidden models are
    absent. Add a defensive stale-cache check anyway: before adding an
    ability to the response, load/cache its channel and skip the ability
    when `channel.IsModelHidden(ability.Model)` is true.
  - `RetrieveModel` (line 897): this currently returns static
    `modelsMap` entries before consulting group abilities, so built-in
    hidden upstream names would still be retrievable. Change it to follow
    `ListModels` semantics: resolve the caller's group abilities first,
    require a non-hidden enabled ability for the requested model, and only
    then return metadata from `modelsMap`/`listAllSupportedModels`. For a
    hidden or unavailable name, return the existing `model_not_found`
    response shape (currently HTTP 200 with an error body unless the
    endpoint's status-code behavior is changed globally).
  - `GetUserAvailableModels` (line 927): apply the same stale-cache
    defensive hidden check before returning model names to the Modern
    token/playground UIs.
  - `GetAvailableModelsByToken` (line 963): intersect token-specific
    model restrictions with the user's non-hidden group abilities before
    returning `available`, otherwise a token that explicitly lists a
    hidden upstream name can still leak it through this helper.
  - `GetModelsDisplay` (line 375): filter hidden names in the anonymous
    path because `/api/models/display` is public. The logged-in path
    should already be ability-filtered, but should use the same defensive
    hidden check as `ListModels`.
  - User-facing pricing/model display must expose public aliases only.
    When a request uses `C -> A`, pricing may be calculated from `A`
    internally, but the rendered model key and any response metadata shown
    to users must stay `C`.
  - `listAllSupportedModels` (line 323): do not treat this as the public
    visibility enforcement point. It is also used by `/api/channel/models`
    as an admin/catalog helper, and its built-in `allModels` seed can
    contain names that no enabled channel currently exposes. Filtering DB
    custom hidden entries here is fine for metadata cleanliness, but
    `ListModels` and `RetrieveModel` must still enforce visibility through
    abilities and `HiddenModels`.
  - `DashboardListModels` (line 154): this endpoint is currently
    `UserAuth` but behaves like a management catalog. To honor the hidden
    model contract, either move it behind `AdminAuth` or filter the union
    of enabled channels' hidden model names for non-admin callers. The
    Modern channel editor is admin-only, so admin callers can keep the
    static provider catalog shape.

### 4.4 Backend — user-facing observability

- User-visible logs, billing details, request traces, playground history,
  and error messages must use the original requested model or public alias
  (`OriginModelName` / `RequestModel`), never the mapped hidden target.
- Internal billing, retry, upstream transport, and administrator-only
  diagnostics may use the actual mapped model (`ActualModelName`) when
  needed for correctness.
- Add tests around a `C -> A` request confirming user-facing log and
  billing/model display surfaces show `C` and do not contain `A`.

### 4.5 Backend — request path

- [middleware/distributor.go](../../middleware/distributor.go)
  - Auto-routing path (`CacheGetRandomSatisfiedChannelExcluding`, line
    265): already filtered because abilities no longer include hidden
    models. The unavailable-model path naturally rejects the request.
    When the requested name matches a hidden model, the external response
    must be indistinguishable from an unknown/unavailable model and must
    not reveal hidden status, hidden alternatives, or mapping targets.
  - `SpecificChannelId` path (line 215): unchanged. Admin can still
    test with `model=A` on a channel that hides `A`. In the current auth
    middleware this explicit channel is supplied by an admin token suffix
    (`token:channel_id`) or proxy `:channelid` route parameter, not by a
    normal request-body field.

- Optional but recommended: tighten the error message on the
  auto-routing 503 so that when a hidden model is the cause we add a
  structured DEBUG log such as `lg.Debug("model hidden on all channels",
  zap.String("model", requestModel), zap.String("group", userGroup))`.
  Keep the external message unchanged to avoid enumeration attacks.

### 4.6 Frontend

Only the Modern template is in scope for this change. `web/air` and
`web/berry` remain compatible through the backend CRUD field but do not
need new editor UI.

- [web/modern/src/pages/channels/schemas.ts](../../web/modern/src/pages/channels/schemas.ts)
  - Add `hidden_models: z.array(z.string()).default([])`.
- [web/modern/src/pages/channels/hooks/useChannelForm.ts](../../web/modern/src/pages/channels/hooks/useChannelForm.ts)
  - Parse `data.hidden_models` from the backend JSON string into a string
    array when loading a channel.
  - On submit, send `hidden_models` as a JSON-encoded string (or `null`
    for an empty array) to match the backend field.
  - Keep browser console diagnostics string-only if new diagnostics are
    added.
- [web/modern/src/pages/channels/components/ChannelModelSettings.tsx](../../web/modern/src/pages/channels/components/ChannelModelSettings.tsx)
  - Add a **Hidden Models** `SelectionListManager` near Model Mapping.
    Suggested options are the currently selected `models`; custom entries
    are allowed for paste/typing workflows.
- Add a tooltip: "Models listed here are served by this channel but not
  returned from `/v1/models` and rejected from direct user requests.
  Useful for exposing a unified alias via Model Mapping."
- Validation warning (non-blocking): if a hidden name is not in
  `Models`, show "This name is not currently supported by the channel."
- Validation warning (non-blocking): if a hidden name is a `ModelMapping`
  source, show that the public alias will become unreachable.
- [web/modern/src/i18n/locales/](../../web/modern/src/i18n/locales/)
  - Add all new labels, help text, warnings, and validation messages to
    each Modern locale file.

No hidden badge is required in the public Models page because hidden
upstream names should not be displayed there.

### 4.7 Documentation

- Update the channel configuration section of the manual (if present in
  `docs/manuals`) with an example matching the table in Section 1.
- Add a FAQ entry: "Why does `/v1/models` not list my upstream model?"

## 5. Data Migration & Rollout

- Additive column only; no back-fill.
- Rolling deploy is safe: old binaries ignore the new column; new
  binaries read `NULL` as "no hidden models" and behave identically to
  the old version for existing channels.
- Cache invalidation must be explicit. `UpdateAbilities` refreshes
  database rows and `InitChannelCache()` refreshes routing memory, but
  group model list caches are separate (`getGroupModelsV2Cache` and Redis
  `group_models*` keys). Clear those caches for affected groups during
  channel saves and ability rebuilds so toggling `HiddenModels` is visible
  immediately.

## 6. Test Matrix

Legend: **E2E** = HTTP request against a running server; **UNIT** = Go
test; **FE** = frontend integration test (or manual, if the repo has no
frontend test harness).

| # | Scenario | Setup | Expected | Level |
|---|----------|-------|----------|-------|
| 1 | Happy path — alias routing | Ch1: `Models="A,C"`, `Mapping={C:A}`, `Hidden=[A]`; Ch2: `Models="B,C"`, `Mapping={C:B}`, `Hidden=[B]` | `model=C` gets 200; upstream sees either `A` or `B` | E2E |
| 2 | Load balance distribution | Same as #1, 200 requests for `model=C` | Both channels receive traffic per configured priority/weight | E2E |
| 3 | Direct hidden request rejected | Same as #1, `model=A` | Same generic unavailable/not-found response as an unknown model; no hidden-specific wording or alias suggestions | E2E |
| 4 | `/v1/models` filtered | Same as #1 | Response includes `C`, excludes `A` and `B` | E2E |
| 5 | Public model browser filtered | Same as #1, call `/api/models/display` anonymously | Response includes `C`, excludes `A` and `B` | E2E |
| 6 | `SpecificChannelId` admin bypass | Same as #1, call with an admin token suffix `token:1` or proxy `:channelid` and `model=A` | 200; hits upstream `A` directly | E2E |
| 7 | `RetrieveModel` for hidden name | Same as #1, `GET /v1/models/A` | Existing `model_not_found` response shape | E2E |
| 8 | Backwards compat — empty field | Existing channel, `HiddenModels=NULL` | Identical behavior to pre-change (no abilities removed, `/v1/models` unchanged) | UNIT + E2E |
| 9 | Hidden entry not in `Models` | `Models="C"`, `Hidden=["A"]` | Save succeeds with frontend warning; backend treats `Hidden=["A"]` as a no-op | UNIT |
| 10 | Ability and cache rebuild on toggle | Save channel with `Hidden=["A"]`, then remove → save again | Ability rows for `A` disappear/reappear, memory and Redis group-model caches are refreshed | UNIT |
| 11 | Case-insensitive match | `Models="ModelA"`, `Hidden=["modela"]` | `ModelA` is hidden | UNIT |
| 12 | Token allowlist interaction | Token allows only `C`; hidden `A` on channel | `model=C` works; `model=A` is rejected without hidden-specific wording; token available helpers return only `C` | E2E |
| 13 | Model mapping still targets hidden | `Models="A,C"`, `Mapping={C:A}`, `Hidden=[A]` | `model=C` works; upstream receives `A` | UNIT (mapping) + E2E |
| 14 | Multiple hidden models | `Models="A,B,C"`, `Hidden=[A,B]` | Only `C` advertised; `A` and `B` rejected | E2E |
| 15 | Hidden + DB migration fresh install | Fresh SQLite DB, new binary | Column present, default NULL, no errors on channel CRUD | UNIT |
| 16 | Hidden + DB migration upgrade | Pre-change SQLite DB, new binary | Column added without data loss | UNIT |
| 17 | Channel create via API with `hidden_models` | POST `/api/channel/` with JSON | Persisted correctly, returned on GET | E2E |
| 18 | Modern frontend edit round-trip | Open Modern Edit Channel, add hidden names, save, reopen | Values reappear; abilities reflect change | FE |
| 19 | User-facing catalog invisibility | Same as #1, call `/api/models` as a non-admin user if the route remains user-accessible | Response is forbidden or excludes `A` and `B`; admin-only catalog behavior remains intact | E2E |
| 20 | User-visible logs and billing labels | Same as #1, complete a `model=C` request and view user log/billing/model-display surfaces | User-facing surfaces show `C` and never expose `A`/`B`; internal billing can still price by mapped target | E2E |

## 7. Acceptance Criteria

A reviewer can close this proposal when **all** of the following hold:

1. `Channel` struct, DB migration, and CRUD round-trip a `HiddenModels`
   field with JSON array semantics (Test 17, 18).
2. Abilities are not created for model names listed in `HiddenModels`,
   and `UpdateAbilities` re-applies correctly on toggle (Tests 10, 11).
3. `GET /v1/models` (`ListModels`) omits hidden names while still
   returning the public alias (Test 4).
4. `GET /v1/models/:id` returns the existing `model_not_found` response
   for a name that is hidden on every channel (Test 7).
5. A user request for a hidden model name is rejected like an unknown or
   unavailable model, with no hidden-specific wording or alias hints; the
   explicit admin channel override continues to succeed with the same
   name (Tests 3, 6).
6. Load balancing across multiple channels with the same public alias
   distributes traffic per existing priority/weight rules, and the
   upstream observes the mapped name (Tests 1, 2, 13).
7. `/api/models/display` filters hidden upstream names for anonymous and
   normal users, and `/api/models` is either admin-only or filters hidden
   model names for non-admin callers (Tests 5, 19).
8. Channels with `HiddenModels` unset exhibit identical pre-change
   behavior (Test 8).
9. The Modern frontend exposes an editor for the field with non-blocking
   warnings when a hidden name is missing from `Models` or is used as a
   mapping source (Test 18).
10. Documentation updated with the alias-fan-out example and FAQ entry.
11. New unit tests added under `model/` for `GetHiddenModels`,
    `IsModelHidden`, the `AddAbilities` filter, and group-model cache
    invalidation; controller/relay integration tests cover Tests 1, 3,
    4, 5, 6, 7, 12, 13, 19, and 20.
12. User-facing logs, pricing displays, playground suggestions, token
    helpers, and error responses never expose hidden target names; mapped
    requests are displayed under the public alias (Test 20).

## 8. Risks & Mitigations

- **Risk:** Operators mistakenly hide a model used as a mapping
  *source* (e.g., hiding `C`). Result: the only public alias becomes
  unreachable.
  **Mitigation:** Frontend warning when the hidden name is a mapping
  source. Document clearly.
- **Risk:** User-facing log, billing, or pricing leakage of the hidden
  model name.
  **Mitigation:** Treat hidden targets as internal-only values. Any
  response, user log view, token helper, pricing page, or playground UI
  must display the original request model/public alias. Administrator-only
  diagnostics and upstream telemetry may still contain the real target.
- **Risk:** Stale cache. Routing memory (`group2model2channels`) and
  group model list caches (`getGroupModelsV2Cache`, Redis
  `group_models*`) are separate.
  **Mitigation:** Centralize channel/ability cache invalidation and add a
  test asserting the model disappears from all relevant caches after a
  hidden-only toggle (Test 10).
- **Risk:** Suspended ability leftovers. Existing ability rows can carry
  `suspend_until`; hiding a model must not leave an old suspended row that
  later becomes selectable.
  **Mitigation:** Keep `UpdateAbilities` as delete-all-then-recreate for
  the channel, and add a test with a pre-existing suspended ability for a
  newly hidden model to confirm the row is removed.
- **Risk:** Static catalog leakage. `/api/models` can list provider-known
  upstream names even when every channel hides them.
  **Mitigation:** Make `/api/models` admin-only or filter hidden names for
  non-admin callers; keep `/api/channel/models` admin-only.
