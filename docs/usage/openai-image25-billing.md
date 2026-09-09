# GPT Image 2.5 billing

Applies to `gpt-image-2.5-sunburst`, `gpt-image-2.5-flare`, and their
`2026-09-08` snapshots on the existing Images API path.

## Configure before enabling traffic

The catalog contains official token prices, not an official per-image price.
OpenAI warns against copying GPT Image 2 per-image estimates to these models.
A one-token estimate is not a safe image reservation.

For each enabled ID, explicitly set its `image.price_per_image_usd` in the
channel's existing `model_configs`. For this family only, this field is an
**operator-defined advance reservation and incomplete-usage fallback tariff**,
not an additional render fee. Choose it for the image counts, sizes, and quality
levels offered by the channel. No default dollar amount is invented by the relay.
A price-only override preserves the catalog request defaults (`auto` size/quality,
32,000-character prompt, and 1–10 images) instead of reverting to legacy defaults.
An absent, nonpositive, nonfinite, or unrepresentable reservation produces
`503 image_billing_not_configured` before any paid upstream request.

Reservation is `ceil(tariff × tier × group multiplier × quota/USD)` per image,
multiplied by the requested count. Both user and limited-token quota use the
existing pre-consumption path before HTTP dispatch. This is an advance estimate,
not a provider cost ceiling: actual usage can exceed it and require an additional
debit under the existing quota policy. Configure adequate reserves and balances.

## Settlement

With billable output usage, the token charge **replaces** the advance reservation.
Unused reserved quota is refunded using a signed post-consumption adjustment.
No fixed render fee is added. The original model rates and settlement rules for
older image models remain unchanged.

The Images response decoder preserves `cached_tokens` and
`cached_tokens_details.{text_tokens,image_tokens}`. Explicit cache buckets are
used as reported, not apportioned by the ratio of total text/image input.
Single-modality aggregate cache counts can be allocated exactly. Mixed-modality
cached input with no split, inconsistent counts, or missing output usage is not
presented as a measured token charge: settlement retains the configured fallback.
Logs distinguish `billing_source=provider_tokens` from `configured_fallback` and
record the reserved amount separately.

For compatible endpoints with input totals but incomplete modality details,
unclassified input is billed at the text-input rate instead of being discarded.
This is a compatibility pricing fallback, not a claim about the actual modality;
accurate mixed-modality billing requires a complete upstream breakdown.

Existing upstream-error/refund handling is preserved. This change does not add
streaming image generation or the Responses image-generation tool.

## Behavioral regression coverage

`image25_review_test.go` covers wire JSON conversion, asymmetric caches,
totals-only/partial input, ambiguous usage, and reservation replacement.
`image25_relay_review_test.go` uses a local HTTP upstream and SQLite to verify
admission, physical pre-debits, signed refunds, and persisted log/request costs.
`image25_policy_test.go` covers invalid reservation values and delta boundaries.

Run the focused CI gate locally with the repository's Go toolchain:

```sh
go test -race -count=1 -timeout=5m ./relay/adaptor/openai ./relay/controller -run 'Test(GPTImage25|Image)'
```

The dedicated workflow does not replace the existing full vet, race-test, or
frontend gates. Tests use local fixtures; no paid OpenAI request is required.

Sources checked 2026-09-09:
- https://developers.openai.com/api/docs/models/gpt-image-2.5-sunburst
- https://developers.openai.com/api/docs/models/gpt-image-2.5-flare
- https://developers.openai.com/api/reference/resources/images/methods/generate
- https://developers.openai.com/api/docs/guides/image-generation
