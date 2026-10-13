# Moonshot/Kimi audit — 2026-10-12

Official sources:

- https://platform.kimi.com/docs/models
- https://platform.kimi.com/docs/guide/kimi-k3-quickstart
- https://platform.kimi.com/docs/pricing/chat

All four current public model IDs are already present. Seven retained catalogue
entries (K2.5 and six V1 variants) are now explicitly retired on August 31, rather
than appearing current or describing the retirement as future. Historical IDs,
prices and capabilities remain available for existing operator configurations.
No retired V1-auto or K2 identifier is newly reintroduced.

K3's documented max_completion_tokens and reasoning_effort controls are advertised;
the existing conversion path preserves them. Fixed sampling settings remain
excluded. Its output ceiling is 1,048,576, not the default 131,072. Public image
URLs are not supported upstream; base64 or ms:// references are required.

## Pricing evidence limit

The current pricing page describes separately billed five-minute and one-hour
cache writes, but its numeric tariff table was absent from the retrieved HTML.
Do not infer rates from Anthropic or substitute the international USD tariff.
The existing CNY input/cache-read/output numbers are preserved, not claimed as
freshly verified. Cache-write prices and usage parsing remain an unresolved
billing gap; the K3 description states this instead of implying free writes.
No automatic model migration, date-based price change, or new billing default is
introduced without that evidence.

Tests cover retained retired IDs, current IDs not being marked retired, historical
amounts, and conversion of K3 output/effort controls. Validation is in PR #560.
Rollback by reverting this provider commit; no schema or deployment change.
