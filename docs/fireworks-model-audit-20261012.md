# Fireworks model audit — 2026-10-12

The dedicated Standard serverless price table and main V4.1 model-card price
agree on USD 0.30 / 0.006 / 1.20 per million input / cached input / output tokens.
Update the existing `accounts/fireworks/models/deepseek-v4p1-flash` definition
in place, replacing 0.22 / 0.007 / 0.66. The model card's older FAQ still prints
the old tariff: record the disagreement, preferring the dedicated current price
table corroborated by the card's main pricing block.

Sources:
- https://docs.fireworks.ai/serverless/pricing
- https://fireworks.ai/models/deepseek-ai/deepseek-v4p1-flash

Tests independently check all ten individually priced Standard text/vision
models in the dedicated table and preserve dated V4 Flash and Pro prices.
Priority, Fast, US-only, Batch and on-demand GPU deployment prices are separate
service modes, not new guessed model IDs or ordinary default-token multipliers.
No first-party DeepSeek peak/off-peak schedule is copied to this host. No exact
upstream tariff-change timestamp was published in the inspected pages, so this
is a current-default correction, not a fabricated historical pricing schedule.
Existing account-specific channel overrides are not changed.

The original multimodal/tool request regression retains exact payload checks
with the intentionally corrected V4.1 rate. The isolated qualification requires
old-code assertion failures and full Fireworks/OpenAI/pricing/format race suites
before publishing. See PR receipts for actual outcomes; no paid live call was
made. Revert this provider commit for rollback. No database, credential or
production-workflow changes are included.
