# GPT Image 2.5/2.6 billing

Applies to `gpt-image-2.5-sunburst`, `gpt-image-2.5-flare`, their `2026-09-08`
snapshots, `gpt-image-2.6-sunburst`, and `gpt-image-2.6-flare` on the existing
Images API path (generations and edits).

These models are billed from provider token usage. The catalog holds OpenAI's
official token prices (USD per 1M tokens: text input 5, cached text input 1.25,
image input 8, cached image input 2, image output 30), not a per-image price.
Every request still needs a **per-image reservation/fallback amount**. The relay
uses it twice: it reserves the amount before the paid upstream request, and it
charges that amount if the receipt has no usable usage. It is never added on top
of the token charge as a render fee.

## Default reservation: OpenAI's token calculator

When the channel's `model_configs` sets no positive `image.price_per_image_usd`
for the model, and no megapixel tariff applies, the relay derives the per-image
amount from the request:

```text
per_image_usd = output_tokens × output_rate / 1e6
              + prompt_bytes × text_input_rate / 1e6 / n
```

- `output_tokens` is OpenAI's published cost-calculator estimate of the image
  output tokens for one image at the request's size and quality.
- `output_rate` is the effective image-output token price at request start. The
  channel's `ratio × completion_ratio` override applies when set. Otherwise the
  catalog price of $30 per 1M applies.
- `prompt_bytes` is the prompt's UTF-8 byte count. It is an upper bound on the
  prompt's text-token count.
- `text_input_rate` is the effective text-input price. The channel's `ratio`
  applies when set. Otherwise the catalog price of $5 per 1M applies.
- `n` is the billed image count. The prompt is counted once per request and is
  spread across the images.

Time-of-day pricing windows are resolved at request start. A usage tier is
selected from the estimate's token counts, the same way settlement selects one
from a receipt.

The derived amount then follows the same path as a configured price. The
reservation is `ceil(per_image_usd × group multiplier × quota/USD)` per image,
multiplied by `n`. Size/quality tier multipliers (`size_multipliers`,
`quality_multipliers`, `quality_size_multipliers`, `$image-tier:` overrides)
scale a configured per-image price only. The derived default already prices
size and quality, so it ignores them and can never be discounted by them.

### What the provider renders

The default prices what the provider will actually render:

- When the relay re-encodes the request (native OpenAI and Azure generations,
  mapped JSON requests, converted channel types), the provider receives the
  normalized request. The hold uses its size, quality, and clamped `n`,
  including any channel `default_size` or `default_quality`.
- When the relay forwards the client body verbatim (native OpenAI edits,
  OpenAI-compatible channel types), the provider receives the client's values.
  An omitted size or quality is the provider's own default, `auto`, whatever
  the channel defaults say. The client's unclamped `n` is held, and each
  requested streamed partial image (`partial_images`, at most 3) adds the
  documented 100 output tokens per image.

### Calculator formula

Source: <https://developers.openai.com/api/docs/guides/image-generation#calculating-costs>
(verified 2026-11-23). For a quality base `B` and a `W×H` output:

```text
d      = round(B / (long_edge / short_edge))        (float64; an exact .5 rounds to even)
tokens = ceil(B × d × (2,000,000 + W×H) / 4,000,000)
```

| Quality | `B` (2.5 and 2.6) | GPT Image 2.6 factor |
|---------|-------------------|----------------------|
| `low` | 16 | 4.2 |
| `medium` | 24 | 3.3 |
| `high` | 48 | 1 |
| `xhigh` | 64 | 1 |
| `max` | 96 | 1 |

For GPT Image 2.6, the token count is multiplied by the factor and rounded to
the nearest integer. Sunburst and Flare are priced identically. The same formula
with GPT Image 2's bases reproduces OpenAI's published GPT Image 2 per-image
price table. `relay/adaptor/openai/image_token_estimate_test.go` pins this.

The calculator does not price every request, so anything it does not cover
resolves to its most expensive case:

- An `auto`, omitted, unparseable, or out-of-bounds size becomes `2880x2880`.
  This is the valid size with the most output tokens at every quality. The bounds
  are a maximum edge of 3840 px, at most a 3:1 aspect ratio, and at most
  8,294,400 pixels.
- An `auto`, omitted, or unknown quality becomes the most expensive quality for
  that size (`max`).

### Example holds

These are per-image holds at catalog prices, for image output only. The prompt
term is added on top, and the tier and group multipliers have not been applied.

| Model | Size | Quality | Output tokens | Per-image hold |
|-------|------|---------|---------------|----------------|
| 2.5 or 2.6 | `auto` | `auto` | 23,719 | ≈ $0.7116 |
| 2.5 or 2.6 | `1024x1024` | `max` | 7,024 | ≈ $0.2107 |
| 2.5 or 2.6 | `1024x1024` | `high` | 1,756 | ≈ $0.0527 |
| 2.5 or 2.6 | `1536x1024` | `xhigh` | 2,459 | ≈ $0.0738 |
| 2.5 | `1024x1024` | `low` | 196 | ≈ $0.0059 |
| 2.6 | `1024x1024` | `low` | 823 | ≈ $0.0247 |
| 2.5 | `1024x1024` | `medium` | 439 | ≈ $0.0132 |
| 2.6 | `1024x1024` | `medium` | 1,449 | ≈ $0.0435 |

A request with `n` = 10, `auto` size, and `auto` quality therefore reserves
about $7.12 before the prompt term. A balance that cannot cover the hold is
refused at admission under the existing quota policy.

The default does not cover reference-image input tokens on edits, because
OpenAI publishes no rule for estimating them. A complete receipt still bills
them. The hold is an advance estimate, not a provider cost ceiling. If actual
usage exceeds it, the difference is debited under the existing quota policy.

## Explicit per-image price (optional override)

A positive `image.price_per_image_usd` in the channel's `model_configs` still
overrides the default, so existing configurations keep their behavior. Use it to
hold a different amount. For example, a channel that only offers small images
can hold less than the calculator's worst case for `auto`. An opt-in megapixel
tariff also overrides the default. For these models, the field is an
operator-defined advance reservation and fallback for incomplete usage. It is
not an official OpenAI per-image price and not an extra render fee.

A price-only override keeps the catalog request defaults: `auto` size and
quality, a 32,000-character prompt, and 1–10 images. It does not fall back to
the legacy defaults. The prompt limit counts Unicode code points, not UTF-8
bytes. Configuring only a reservation does not replace the official token rates
with zeros.

## Admission and operator-priced hosts

The relay answers `503 image_billing_not_configured` before any paid upstream
request only when no positive, finite, representable reservation exists. That
happens when there is neither a positive per-image price nor an image-output
token price to derive the default from, or when the configured or derived amount
overflows the quota range. The error text is:

> GPT Image 2.5/2.6 requires a positive reservation/fallback: price the model's
> image output tokens so the default is derived from OpenAI's token calculator,
> or set a positive image.price_per_image_usd in channel model_configs

On native OpenAI channels, the catalog prices normally provide a default. Azure
and the AIProxy library declare the two GPT Image 2.6 IDs operator-priced, so
only the channel's own token prices can price them there. The AIProxy library
has no image endpoint, so image requests are never routed to it. On those hosts:

- With no channel token prices, the request is still refused. The pricing
  readiness check reports both the `token_tariff` gap and the
  `image_price_per_image_usd` gap.
- Once the channel sets `ratio` and `completion_ratio`, the default is derived
  from the channel's output price, and no per-image price is needed.

A free group (group multiplier 0) is served without a reservation and settles at
zero. Admission still requires a positive per-image amount, either configured or
derived, to exist.

The Modern channel editor runs the same check when a channel is saved, through
`POST /api/channel/pricing-readiness`. The check is evaluated at the most
expensive request: `auto` size, `auto` quality, one image, tier 1, and group 1.
The `image_price_per_image_usd` requirement is reported only when a Supported
Model (after Model Mapping) has neither a positive per-image price nor an
image-output token price. The save dialog names the model and the fields to set.
The warning never blocks saving: "Save Anyway" keeps the channel as entered.

## Settlement and channel pricing

Settlement does not depend on whether the hold was configured or derived. The
relevant code is `finalizeImageQuota` in `relay/controller/image_token_billing.go`
and `computeStrictGPTImageTokenQuote` / `strictGPTImageReceiptCeiling` in
`relay/controller/image25_billing.go`.

- **Complete receipt.** A complete receipt has positive input and output counts,
  consistent buckets, and a positive token charge. Its token charge **replaces**
  the hold. A signed post-consumption adjustment refunds unused reserved quota or
  debits the shortfall. No fixed render fee is added.
- **Incomplete modality or cache split.** If the provider attributes only part of
  the input to text or image, the unclassified remainder is priced at the higher
  modality rate. Uncached unclassified tokens use the larger of the text and
  image input rates, and cached unclassified tokens use the larger of the two
  cached rates. If the receipt gives an aggregate `cached_tokens` count without
  `cached_tokens_details`, the cache reads are credited to buckets in order of
  smallest cache discount (uncached rate minus cached rate) first. The result is
  the most expensive charge consistent with the receipt's totals. It is recorded
  with estimate reason `gpt_image_receipt_modality_upper_bound` and
  `billing_source=provider_tokens_upper_bound`. The flag is omitted when the
  split cannot change the charge: every input token is cached, or every
  non-empty bucket has the same cache discount, as with a single-modality
  aggregate. Explicit `cached_tokens_details.{text_tokens,image_tokens}` are used
  as reported. They must not exceed their modality's input, and they must sum to
  the aggregate when one is given. Cache reads are never apportioned by the
  ratio of text to image input.
- **Incomplete or inconsistent receipt.** This covers a receipt that is missing
  its input or output count, or whose buckets contradict its totals. It settles
  `max(hold, ceiling)`. The ceiling prices every reported token at the highest
  applicable uncached rate. The input count is the largest of `input_tokens`,
  `total_tokens − output_tokens`, text plus image input, and cached input. It is
  priced at the largest of the four input rates. Output is priced at the output
  rate. When the ceiling exceeds the hold, the charge is flagged
  `gpt_image_receipt_incomplete_upper_bound`
  (`billing_source=provider_tokens_upper_bound`).
- **No usable usage.** Settlement keeps the hold. A derived default is labeled
  `gpt_image_receipt_unusable_default_reservation` with
  `billing_source=default_fallback`. An operator price is labeled
  `gpt_image_receipt_unusable_configured_fallback` with
  `billing_source=configured_fallback`.

Wherever the input count is used, it is the larger of `input_tokens` and
`total_tokens − output_tokens`. A receipt that omits `input_tokens` is therefore
not billed as output only. Logs record `billing_source=provider_tokens` for an
exact token charge, and they record the reserved amount separately
(`reserved_usd`, `fallback_unit_usd`).

Channel token prices are resolved at request start, including time-window rules.
Usage-based tiers are selected during settlement. Channel overrides map to the
buckets as follows:

- `ratio` prices text input.
- `completion_ratio` multiplies `ratio` to give the image-output price.
- `cached_input_ratio` prices cached text.
- The existing `image.prompt_ratio` multiplies both the uncached and cached text
  rates to give the image-input buckets. The catalog default is 8/5.

This schema has no independent cached-image price field. Missing optional
overrides inherit the provider defaults. Negative cached-input pricing keeps the
existing free-cache convention. The tariff tables and settlement formulas for
older image models are unchanged.

Existing upstream-error and refund handling is preserved. This billing path does
not add streaming image generation or the Responses image-generation tool.

## Behavioral regression coverage

- `relay/adaptor/openai/image_token_estimate_test.go` pins the calculator
  against OpenAI's published GPT Image 2 prices, the per-model estimates, and the
  worst-case `2880x2880` size.
- `image_default_reservation_relay_test.go` drives the derived default through
  the relay: `auto` and explicit holds, multi-image requests, channel
  token-price overrides, and the missing-usage settlement label.
- `image25_review_test.go` covers wire JSON conversion, asymmetric caches,
  totals-only and partial input, ambiguous usage, and reservation replacement.
- `image25_relay_review_test.go` uses a local HTTP upstream and SQLite to verify
  admission, physical pre-debits, signed refunds, channel token-price overrides,
  and persisted log and request costs.
- `image25_unicode_review_test.go` covers ASCII, CJK, and emoji at and beyond the
  character boundary.
- `image25_policy_test.go` covers invalid reservation values, defaults, and delta
  boundaries.
- `image_modality_upper_bound_test.go` covers the upper-bound and
  incomplete-receipt settlement.
- `channel_pricing_readiness_image_test.go` matches the readiness verdict
  against `RelayImageHelper`.

Run the focused tests locally with the repository's Go toolchain:

```sh
go test -race -count=1 -timeout=5m ./relay/adaptor/openai ./relay/controller -run 'Test(GPTImage|Image|StrictGPTImage|EstimateGPTImage)'
```

This focused run does not replace the full vet, race-test, or frontend
qualification described in `docs/ci-testing.md`. The tests use local fixtures,
and no paid OpenAI request is required.

Sources checked 2026-09-09, with the cost calculator re-verified 2026-11-23:

- <https://developers.openai.com/api/docs/models/gpt-image-2.5-sunburst>
- <https://developers.openai.com/api/docs/models/gpt-image-2.5-flare>
- <https://developers.openai.com/api/docs/models/gpt-image-2.6-sunburst>
- <https://developers.openai.com/api/docs/models/gpt-image-2.6-flare>
- <https://developers.openai.com/api/reference/resources/images/methods/generate>
- <https://developers.openai.com/api/docs/guides/image-generation>
