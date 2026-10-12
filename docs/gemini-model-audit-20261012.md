# Gemini model audit checkpoint — 2026-10-12

## Implemented: lifecycle descriptions

Official source: https://ai.google.dev/gemini-api/docs/changelog

- Since October 8, `gemini-3.7-flash` is deprecated in the Gemini Developer API;
  Google automatically routes that ID to `gemini-3.8-flash`.
- Since October 8, `gemini-3.5-flash` is deprecated in the Gemini Developer API;
  Google automatically routes that ID to `gemini-3.6-flash`.
- Since October 6, `gemini-3.1-flash-image` is deprecated, with no announced
  shutdown date. Google's recommended replacement is `gemini-nano-banana-2.1`.

The shared catalogue initialization applies these descriptions without changing
its model list. Native Gemini and Vertex consume the same metadata. Wording is
explicitly scoped to the Developer API; it does not certify Vertex entitlement
or an identical Vertex lifecycle.

No model ID, request URL, numeric tariff, capability, or override is changed.
The legacy 3.5 tariff is explicitly labelled historical: callers should select
3.6 or 3.8 and its current price rather than assume that an upstream automatic
redirect preserves the old 3.5 charge. A billing-effective alias migration still
needs backend-specific verification; this checkpoint does not claim to fix it.
The September access restriction on Gemini 2.5 remains correctly described as
**not** a deprecation.

## New-model gap requiring transport work

Official model card and pricing:

- https://ai.google.dev/gemini-api/docs/models/gemini-nano-banana-2.1
- https://ai.google.dev/gemini-api/docs/pricing

Nano Banana 2.1 became GA on October 6. It has a 131,072 input-token limit and
32,768 output-token limit. Standard input costs USD 1.50/million; text/thinking
output costs USD 7.50/million and image output USD 30/million. The advertised
image sizes are 1K, 2K, and 4K, not 512 pixels. Its 4K output uses 3,780 tokens
(USD 0.1134 before the provider's display rounding), unlike Nano Banana 2's
2,520-token 4K output. Do not clone the old model's image-size tariffs or its
structured-output/function-calling metadata.

This model is **not added** yet. The existing `GetModelModalities` helper cannot
parse the named family as numeric Gemini 3.x and does not find `-image`, so it
would request TEXT-only output. The native image sampling cleanup and the
model-specific search tariff selector also use numeric/name assumptions.
Before admission, validate native image conversion, output modality selection,
resolution-specific billing, search pricing, system instructions, and the
OpenAI-compatible/Vertex transport boundaries with behavior tests. A catalogue
entry alone is not a working image integration.

## Validation and rollback

`TestGeminiOctoberLifecycle` checks the shared/native initialized catalogues,
exact migration dates, no invented image shutdown date, retained historical
prices, and the unrelated Gemini 2.5 non-deprecation wording. Run the complete
Gemini shared/native and Vertex package suites with the repository's Go 1.27
compiler. Refer to the PR validation receipts for executed checks. No live
provider calls or complete all-provider audit is claimed.

Revert this provider commit to restore prior descriptions. No database migration,
credential change, or deployment configuration change is required.
