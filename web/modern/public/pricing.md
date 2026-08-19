# One API Pricing And Quota

One API is open-source software. A hosted deployment may configure its own quota, channel ratios, group ratios, model pricing, and top-up policy.

## Free Sandbox

Laisky One API exposes a no-key sandbox at [https://oneapi.laisky.com/sandbox](https://oneapi.laisky.com/sandbox). Sandbox calls are free because they return static mock responses and do not call upstream model providers.

Real provider calls require a relay API key and account quota.

## Quota Model

- Usage is billed against user or token quota.
- Channel configuration can define model ratios, completion ratios, cache read/write ratios, tiered pricing, time windows, and media pricing.
- Streaming requests keep one consistent rate for the request even when a configured time window changes during the stream.
- Public model display can expose pricing metadata at `/api/models/display` when configured.

## Agent Guidance

- Query available models before making calls when exact pricing matters.
- Do not assume all upstream providers price the same model identically.
- Treat quota and balance responses as sensitive account data.
