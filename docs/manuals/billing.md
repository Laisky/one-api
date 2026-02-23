# Billing Administration Guide

One-API’s billing layer is built for multi-tenant deployments that broker requests across dozens of upstream AI vendors. This guide documents the goals behind the system, the core concepts administrators must understand, and the day-to-day workflows for operating it safely.

## Menu

- [Billing Administration Guide](#billing-administration-guide)
  - [Menu](#menu)
  - [Design Goals](#design-goals)
  - [Core Concepts](#core-concepts)
    - [Pricing Units \& Ratios](#pricing-units--ratios)
    - [Four-Layer Pricing Resolution](#four-layer-pricing-resolution)
    - [Tiered \& Cached Pricing](#tiered--cached-pricing)
    - [Multimedia Pricing](#multimedia-pricing)
    - [Tool \& Quota Buckets](#tool--quota-buckets)
  - [Daily Operations](#daily-operations)
    - [1. Inspect Default Pricing](#1-inspect-default-pricing)
    - [2. Override Pricing for a Channel](#2-override-pricing-for-a-channel)
    - [3. Monitor Quota Health](#3-monitor-quota-health)
    - [4. Audit Request-Level Costs](#4-audit-request-level-costs)
    - [5. Manage Prompt Caching Budgets](#5-manage-prompt-caching-budgets)
    - [6. Aggregate External Consumption](#6-aggregate-external-consumption)
  - [Reference API Surface](#reference-api-surface)
  - [Operational Tips](#operational-tips)

## Design Goals

- **Predictable costs**: Every adapter publishes per-1M-token pricing so downstream quotas map directly to currency budgets.
- **Layered overrides**: Channel owners can override adapter defaults, while the platform falls back to merged global pricing when needed.
- **Multimedia parity**: Text, audio, image, and video models all share the same billing primitives so cost controls behave consistently.
- **Cache awareness**: Prompt caching discounts (cache read and cache write windows) must be represented explicitly to avoid double billing.
- **Auditability**: Each request receives a `X-Oneapi-Request-Id`, enabling per-request cost lookups and reconciliation with third-party ledgers.

## Core Concepts

### Pricing Units & Ratios

- All ratios are expressed as **USD per 1M tokens**. Internally the platform converts USD to quota using `QuotaPerUsd = 500,000`.
- `ratio` controls input tokens, `completion_ratio` scales outputs, and group-level multipliers (per user group) apply last.

### Four-Layer Pricing Resolution

1. **Channel overrides** (`model_configs`) – highest priority.
2. **Adapter defaults** (`GetDefaultModelPricing`).
3. **Global pricing manager** – merged catalog from 13+ mature adapters; keeps OpenAI-compatible channels from drifting.
4. **Final fallback** – a conservative USD default that prevents requests from failing when pricing is unknown.

### Tiered & Cached Pricing

- Each model can advertise token thresholds via `tiers[]`. When a request crosses the specified `input_token_threshold`, the system swaps in the tier’s ratios (input, completion, cached read, cache write 5m/1h).
- `CachedInputRatio`, `CacheWrite5mRatio`, and `CacheWrite1hRatio` encode prompt caching math. Zero means “inherit base price,” negative numbers mark the bucket as free.
- `ResolveEffectivePricing` records which tier was used so you can debug large jobs.

### Multimedia Pricing

- `VideoPricingConfig` bills per rendered second at a base resolution and exposes `resolution_multipliers` (e.g., charge 2× for 4K vs 720p).
- `AudioPricingConfig` maps seconds to text tokens via `prompt_tokens_per_second` and can use per-second USD overrides for transcription models.
- `ImagePricingConfig` defines per-image prices plus `quality`/`size` multiplier tables and min/max batch sizes. When `ratio` is zero, only per-image fees apply.

### Tool & Quota Buckets

- Channels may also whitelist built-in provider tools and set per-call USD or quota via `tooling.pricing`. Tool charges are added after token usage.
- Quotas exist at three levels: **user**, **token**, and **request reservation**. Unlimited tokens bypass checks but still log usage for analytics.

## Daily Operations

### 1. Inspect Default Pricing

1. Determine the channel type (e.g., OpenAI-compatible = `type=1`).
2. Call `GET /api/channel/default-pricing?type=<channel_type>` with an admin token.
3. Review `model_configs` in the response—each entry contains ratios, tiers, cache settings, and multimedia metadata pulled from the adapter or global map.

### 2. Override Pricing for a Channel

- Retrieve current values: `GET /api/channel/pricing/<channel_id>`.
- Edit only the models that require overrides. Example payload:

```json
{
  "model_configs": {
    "gpt-4o": {
      "ratio": 0.00275,
      "completion_ratio": 4,
      "cached_input_ratio": 0.001375,
      "tiers": [
        { "input_token_threshold": 200000, "ratio": 0.00225 },
        { "input_token_threshold": 1000000, "ratio": 0.00175 }
      ]
    },
    "gpt-4o-mini-audio-preview": {
      "audio": {
        "prompt_tokens_per_second": 10,
        "completion_ratio": 2
      }
    }
  }
}
```

- Submit via `PUT /api/channel/pricing/<channel_id>`. The server merges only the provided models, leaving others on adapter/global defaults.
- Optional: include `tooling` overrides to price per-call tools.

### 3. Monitor Quota Health

- **Users**: `GET /api/user/<id>` shows `quota`, `used_quota`, and request counts. Use `/api/user/search` to filter by email or name.
- **Tokens**: `GET /api/token/<id>` displays `remain_quota`, `used_quota`, and whether the token is unlimited.
- **Batch updater**: Enable `BATCH_UPDATE_ENABLED=true` plus `BATCH_UPDATE_INTERVAL=<seconds>` to defer quota writes under heavy load. Monitor `logs/` for flush anomalies.

### 4. Audit Request-Level Costs

1. Ask clients to capture the `X-Oneapi-Request-Id` header returned by every response.
2. Query `GET /api/cost/request/<request_id>` to retrieve the `quota` field plus derived USD.
3. Compare against upstream invoices; the platform already subtracts cached tokens and applies tier discounts.

### 5. Manage Prompt Caching Budgets

- Anthropic and OpenAI cache-aware models emit usage buckets for normal input, cache reads, and cache writes.
- To lower cache-write spend, create a channel override that sets `cache_write_5m_ratio` or `cache_write_1h_ratio` to a discounted value (or negative for free promotional tiers).
- If upstream starts returning zero cached metrics, inspect `relay/controller/claude_messages.go` logs; the billing system will fall back to normal ratios but still note the missing metrics.

### 6. Aggregate External Consumption

- When other systems spend budget on behalf of One-API users, call `POST /api/token/consume` (authenticated with the external token) to add quota usage. This keeps `user_request_costs` aligned even for out-of-band workloads.

## Reference API Surface

| Purpose                       | Method & Endpoint                                      | Notes                                                          |
| ----------------------------- | ------------------------------------------------------ | -------------------------------------------------------------- |
| Fetch channel pricing         | `GET /api/channel/pricing/:id`                         | Returns `model_configs`, legacy ratios, and tooling metadata.  |
| Fetch adapter/global defaults | `GET /api/channel/default-pricing?type=<channel_type>` | OpenAI-compatible channels receive merged global pricing.      |
| Update pricing                | `PUT /api/channel/pricing/:id`                         | Accepts unified configs, legacy ratios, and tooling overrides. |
| Inspect user quota            | `GET /api/user/:id`                                    | Requires admin privileges.                                     |
| Inspect token quota           | `GET /api/token/:id`                                   | Requires token owner or admin.                                 |
| Record manual consumption     | `POST /api/token/consume`                              | Body: `{ "add_used_quota": <int>, "add_reason": "..." }`.      |
| Request cost lookup           | `GET /api/cost/request/:request_id`                    | Response includes quota units and `cost_usd`.                  |
| Debug channel configs         | `POST /api/debug/channel/:id/debug`                    | Validates merged pricing for a single channel.                 |

## Operational Tips

- Run `go test -race ./...` after upgrading adapters that touch pricing to ensure tier resolution logic stays stable.
- Use `/api/debug/channels/validate` before rolling out large pricing migrations; it scans every channel for malformed configs.
- When adding a new provider, populate its `ModelRatios` map first, then rely on the global pricing manager to expose those models to generic OpenAI-compatible channels.
- Keep documentation and UI labels in **USD per 1M tokens**; mixing units confuses quota planning.
- Streamed responses flush billing every three seconds (`STREAMING_BILLING_INTERVAL`). If users report premature cutoffs, verify they still have quota at those checkpoints.

With these routines in place, administrators can confidently introduce tiered pricing, multimedia workloads, and cache discounts without losing track of quota integrity or spend visibility.
