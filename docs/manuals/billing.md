# Billing Administration Guide

This manual is for One-API administrators who can manage channels, users, and tokens.
It is aligned with the current implementation in `controller/`, `model/`, and `relay/`.

## Menu

- [Billing Administration Guide](#billing-administration-guide)
  - [Menu](#menu)
  - [Scope and Permission Model](#scope-and-permission-model)
  - [Billing Pipeline (Code-Accurate)](#billing-pipeline-code-accurate)
  - [Pricing Resolution and Formula](#pricing-resolution-and-formula)
    - [Units](#units)
    - [Four-Layer Resolution](#four-layer-resolution)
    - [Final Quota Formula](#final-quota-formula)
    - [Tier and Cache Behavior](#tier-and-cache-behavior)
    - [How Tier Pricing Is Defined](#how-tier-pricing-is-defined)
  - [Settings Reference by Admin Page](#settings-reference-by-admin-page)
    - [Channels Page (Billing-Related Fields)](#channels-page-billing-related-fields)
      - [Model Configs Property Reference](#model-configs-property-reference)
        - [Video model](#video-model)
        - [Audio model](#audio-model)
        - [Image model](#image-model)
      - [Tooling Property Reference](#tooling-property-reference)
    - [Users Page (Billing-Related Fields)](#users-page-billing-related-fields)
    - [Tokens Page (Billing-Related Fields)](#tokens-page-billing-related-fields)
  - [Global Billing Options (System-Level)](#global-billing-options-system-level)
  - [Operational Workflows](#operational-workflows)
    - [1. Inspect Defaults Before Creating/Editing a Channel](#1-inspect-defaults-before-creatingediting-a-channel)
    - [2. Apply Channel Pricing and Tooling Overrides](#2-apply-channel-pricing-and-tooling-overrides)
    - [3. Reconcile Request Cost by Request ID](#3-reconcile-request-cost-by-request-id)
    - [4. Record External Consumption](#4-record-external-consumption)
    - [`/api/token/consume` field semantics](#apitokenconsume-field-semantics)
  - [Reference API Surface](#reference-api-surface)
  - [API Field Reference (Detailed)](#api-field-reference-detailed)
    - [`GET /api/channel/pricing/:id` response fields](#get-apichannelpricingid-response-fields)
    - [`PUT /api/channel/pricing/:id` request fields](#put-apichannelpricingid-request-fields)
    - [`GET /api/channel/default-pricing?type=...` response fields](#get-apichanneldefault-pricingtype-response-fields)
    - [`PUT /api/user/` billing-related fields](#put-apiuser-billing-related-fields)
    - [`PUT /api/token/` billing-related fields](#put-apitoken-billing-related-fields)
    - [`GET /api/cost/request/:request_id` response fields](#get-apicostrequestrequest_id-response-fields)
  - [Implementation Caveats (Important)](#implementation-caveats-important)

## Scope and Permission Model

- `GET/PUT /api/channel/*`, `GET/PUT /api/user/*`, `GET /api/group/*` require admin auth.
- Token management (`/api/token/*`) is user auth, but admins can still reason about token billing behavior.
- Request-cost lookup endpoint `GET /api/cost/request/:request_id` is currently exposed without auth middleware.

## Billing Pipeline (Code-Accurate)

1. **Pre-consume reservation**
   - Controllers pre-consume quota for many requests (`PreConsumeTokenQuota`), deducting from both user and token (unless token is unlimited).
2. **Streaming incremental charging**
   - For stream requests, `relay/streaming/tracker.go` flushes quota at interval `STREAMING_BILLING_INTERVAL` (default 3s).
3. **Final reconciliation**
   - `relay/quota.Compute` calculates final total quota from usage + pricing.
   - Delta vs pre-consumed amount is applied by `PostConsumeTokenQuota`.
4. **Accounting records**
   - Usage logs are written (`RecordConsumeLog`).
   - Per-request cost table (`user_request_costs`) is upserted by request ID for `/api/cost/request/:request_id`.

## Pricing Resolution and Formula

### Units

- Internal quota unit conversion is fixed: `QuotaPerUsd = 500000`.
- UI and docs should treat model ratios as **USD per 1M tokens** semantics mapped into quota math.

### Four-Layer Resolution

For model input/output ratio resolution (`relay/pricing/global.go`):

1. Channel override (from channel `model_configs` / override maps)
2. Provider adapter default (`GetDefaultModelPricing`)
3. Global pricing map (merged from configured adapters; default list size is 13)
4. Final fallback
   - input ratio fallback: `2.5 * 0.000001`
   - completion ratio fallback: `1.0`

### Final Quota Formula

`relay/quota/quota.go` computes final quota as:

$$
TotalQuota=\lceil C_{prompt\_noncached}+C_{prompt\_cached}+C_{completion}+C_{cachewrite5m}+C_{cachewrite1h}\rceil+ToolsCost
$$

Where:

- `normal_input_price = used_model_ratio * group_ratio`
- `normal_output_price = used_model_ratio * used_completion_ratio * group_ratio`
- Cached and cache-write prices can override normal input price when present.
- If model/group ratio is non-zero and computed quota <= 0, minimum billed quota is 1.

Tooling cost is appended **after** token cost and is already in quota units.

### Tier and Cache Behavior

- Tier and cache ratios are resolved from adaptor `ModelConfig` via `ResolveEffectivePricing`.
- Cache semantics:
  - negative cached/cache-write ratio => free
  - zero cache-write ratio => fallback to normal input ratio
- Cache-write token buckets (`cache_write_5m`, `cache_write_1h`) are capped so they do not exceed non-cached prompt tokens.

### How Tier Pricing Is Defined

Tier pricing schema exists in runtime adaptor config (`relay/adaptor/interface.go`):

```json
{
  "ratio": 0.001,
  "completion_ratio": 4,
  "cached_input_ratio": 0.0005,
  "cache_write_5m_ratio": 0.0008,
  "cache_write_1h_ratio": 0.0007,
  "tiers": [
    {
      "input_token_threshold": 200000,
      "ratio": 0.0008,
      "completion_ratio": 4,
      "cached_input_ratio": 0.0004,
      "cache_write_5m_ratio": 0.0006,
      "cache_write_1h_ratio": 0.0005
    },
    {
      "input_token_threshold": 1000000,
      "ratio": 0.0006
    }
  ]
}
```

Field meaning (runtime tier object):

- `input_token_threshold`: this tier becomes active when request prompt tokens are `>= threshold`.
- `ratio`: input token price for this tier.
- `completion_ratio`: output multiplier for this tier; if omitted/zero, inherited from previous effective value.
- `cached_input_ratio`: cached-read input price for this tier.
- `cache_write_5m_ratio`, `cache_write_1h_ratio`: cache-write prices for this tier.

Important implementation constraints:

1. Tier pricing is fully supported by runtime billing (`ResolveEffectivePricing` + `quota.Compute`).
2. Channel persisted `model_configs` currently does **not** store `tiers` or cache ratio fields.
3. Therefore, admin UI/API channel overrides cannot define tier tables today; tier tables come from provider defaults/global pricing (or code-level adapter updates).

## Settings Reference by Admin Page

### Channels Page (Billing-Related Fields)

In Modern UI (`web/modern/src/pages/channels`), billing-relevant fields are:

1. **Groups (`groups`)**
   - Purpose: controls which user groups can route traffic to this channel.
   - Billing impact: indirect. User group also determines `group_ratio` multiplier in billing.
   - Example:

```json
{
  "group": "default,vip"
}
```

1. **Model Configs (`model_configs`)**
   - Stored in channel `model_configs` JSON.
   - Supported persisted fields in current channel model:
     - `ratio`
     - `completion_ratio`
     - `max_tokens`
     - `video` (`per_second_usd`, `base_resolution`, `resolution_multipliers`)
     - `audio` (`prompt_ratio`, `completion_ratio`, `prompt_tokens_per_second`, `completion_tokens_per_second`, `usd_per_second`)
     - `image` (`price_per_image_usd`, `prompt_ratio`, size/quality multipliers, min/max images, etc.)
   - Example:

```json
{
  "gpt-4o": {
    "ratio": 0.00275,
    "completion_ratio": 4,
    "max_tokens": 128000
  },
  "gpt-4o-mini-audio-preview": {
    "ratio": 0.00015,
    "completion_ratio": 2,
    "audio": {
      "prompt_tokens_per_second": 10,
      "completion_ratio": 2
    }
  }
}
```

#### Model Configs Property Reference

Per-model object keys currently accepted/persisted by channel config (`model.ModelConfigLocal`):

- `ratio` (`number`, `>=0`): base input ratio.
- `completion_ratio` (`number`, `>=0`): output/input multiplier.
- `max_tokens` (`integer`, `>=0`): optional model max token cap metadata.
- `video` (`object`):
  - `per_second_usd` (`number`, `>=0`): base USD/sec equivalent metadata.
  - `base_resolution` (`string`): normalized resolution key (e.g. `1280x720`).
  - `resolution_multipliers` (`map[string]number>0`): resolution multipliers.
- `audio` (`object`):
  - `prompt_ratio` (`number`, `>=0`)
  - `completion_ratio` (`number`, `>=0`)
  - `prompt_tokens_per_second` (`number`, `>=0`)
  - `completion_tokens_per_second` (`number`, `>=0`)
  - `usd_per_second` (`number`, `>=0`)
- `image` (`object`):
  - `price_per_image_usd` (`number`, `>=0`)
  - `prompt_ratio` (`number`, `>=0`)
  - `default_size` (`string`)
  - `default_quality` (`string`)
  - `prompt_token_limit` (`integer`, `>=0`)
  - `min_images` (`integer`, `>=0`)
  - `max_images` (`integer`, `>=0`, and `max_images >= min_images` when both set)
  - `size_multipliers` (`map[string]number>0`)
  - `quality_multipliers` (`map[string]number>0`)
  - `quality_size_multipliers` (`map[quality][size]number>0`)

Validation behavior you should expect:

- Negative numbers are rejected for pricing/count fields.
- Empty model name is rejected.
- A model config entry must contain at least one meaningful field.
- Modern frontend currently requires at least one of `ratio`, `completion_ratio`, or `max_tokens` in each model object.

Practical examples by modality:

##### Video model

```json
{
  "gpt-video-1": {
    "ratio": 0.0002,
    "completion_ratio": 1,
    "video": {
      "per_second_usd": 0.03,
      "base_resolution": "1280x720",
      "resolution_multipliers": {
        "1280x720": 1,
        "1920x1080": 1.5,
        "3840x2160": 2
      }
    }
  }
}
```

##### Audio model

```json
{
  "gpt-4o-mini-audio-preview": {
    "ratio": 0.00015,
    "completion_ratio": 2,
    "audio": {
      "prompt_tokens_per_second": 10,
      "completion_tokens_per_second": 10,
      "prompt_ratio": 1,
      "completion_ratio": 2
    }
  }
}
```

##### Image model

```json
{
  "gpt-image-1": {
    "ratio": 0.0001,
    "completion_ratio": 1,
    "image": {
      "price_per_image_usd": 0.04,
      "default_size": "1024x1024",
      "default_quality": "standard",
      "min_images": 1,
      "max_images": 10,
      "size_multipliers": {
        "1024x1024": 1,
        "1792x1024": 1.5
      },
      "quality_multipliers": {
        "standard": 1,
        "hd": 2
      }
    }
  }
}
```

1. **Tooling Config (`tooling`)**
   - Shape: `{ whitelist: string[], pricing: { tool: { usd_per_call|quota_per_call } } }`
   - Cost conversion: if `usd_per_call` is set, tool call cost is `ceil(usd_per_call * QuotaPerUsd)`.
   - Example:

```json
{
  "whitelist": ["web_search"],
  "pricing": {
    "web_search": { "usd_per_call": 0.025 }
  }
}
```

#### Tooling Property Reference

- `whitelist` (`string[]`): allowed built-in tool names for this channel.
  - Empty/omitted means no explicit whitelist restriction.
- `pricing` (`object`): per-tool cost map.
  - Each key is a tool name.
  - Value supports:
    - `usd_per_call` (`number`, `>=0`), converted by `ceil(usd_per_call * 500000)`.
    - `quota_per_call` (`int64`, `>=0`), used directly.

Priority/override behavior:

- Channel tool pricing overrides provider default pricing for the same tool.
- Tool is considered billable/allowed only when effective pricing metadata exists.
- If whitelist is present, tool must also be in whitelist.

1. **Models (`models`)**
   - Primarily routing capability, but affects which model name is billed and therefore which pricing row resolves.

Additional spend-control fields on channel edit page:

- **Rate Limit (`ratelimit`)**
  - Requests/minute guardrail per channel.
  - Not a price multiplier, but directly controls spend velocity.
- **Priority (`priority`)** and **Weight (`weight`)**
  - Routing behavior among eligible channels.
  - Impacts which priced channel receives traffic when multiple channels support same model/group.

### Users Page (Billing-Related Fields)

In `web/modern/src/pages/users/EditUserPage.tsx`:

1. **Quota (`quota`)**
   - User-level total available quota budget.
   - Must be non-negative.
   - Billing always checks user quota; unlimited token does not bypass user quota checks.

2. **Group (`group`)**
   - Maps to `relay/billing/ratio/group.go` group ratio table (`GetGroupRatio`).
   - This multiplier is applied in the final quota formula.

3. **Used Quota (`used_quota`)**
   - Not edited here, but important for audit and dashboards.

Additional admin-relevant behavior:

- User `group` must exist in configured groups and should have a corresponding `GroupRatio` entry, otherwise runtime falls back to multiplier `1`.
- `PUT /api/user/` updates quota/group explicitly; this is the reliable admin path for billing updates.

Example user update payload:

```json
{
  "id": 123,
  "quota": 2000000,
  "group": "vip"
}
```

### Tokens Page (Billing-Related Fields)

In `web/modern/src/pages/tokens/EditTokenPage.tsx`:

1. **Remain Quota (`remain_quota`)**
   - Token-local budget.
   - If `unlimited_quota=false`, requests fail when exhausted.

2. **Unlimited Quota (`unlimited_quota`)**
   - When true, token-level quota checks/deductions are skipped.
   - User-level quota still applies.

3. **Expired Time (`expired_time`)**
   - Expired tokens are not billable because they cannot be used.

4. **Models (`models`)**
   - Optional model allowlist. Restricts spend scope by model.

5. **Status (`status`)**
   - `enabled`, `disabled`, `expired`, `exhausted` influence whether token can spend quota.
   - Update logic can auto-recover status when quota/expiry becomes valid again.

6. **Access constraints (`subnet`)**
   - Not a pricing field itself, but limits where billing-capable requests can originate.

Example token payload:

```json
{
  "name": "prod-token",
  "remain_quota": 1000000,
  "unlimited_quota": false,
  "expired_time": -1,
  "models": "gpt-4o,gpt-4o-mini"
}
```

## Global Billing Options (System-Level)

Besides channel/user/token pages, these global options influence billing behavior:

- `GroupRatio` (option key): JSON map of user-group multipliers.
  Example:

```json
{
  "default": 1,
  "vip": 0.8,
  "svip": 0.6
}
```

- `QuotaPerUnit` (display/conversion setting in options).
- `QuotaRemindThreshold` (quota reminder trigger).
- `PreConsumedQuota` (reservation behavior for certain request paths).

When onboarding from scratch, confirm these global options first, then configure channel/user/token.

## Operational Workflows

### 1. Inspect Defaults Before Creating/Editing a Channel

1. Determine channel type.
2. Call `GET /api/channel/default-pricing?type=<channel_type>`.
3. Parse `data.model_configs` JSON string for default model config map.

### 2. Apply Channel Pricing and Tooling Overrides

Two supported paths:

- **Main channel update** (Modern page uses this): `PUT /api/channel/` with `model_configs` and `tooling`.
- **Pricing endpoint**: `PUT /api/channel/pricing/:id` with `model_configs` and/or legacy maps.

Recommended full channel billing payload example:

```json
{
  "id": 88,
  "name": "openai-compatible-prod",
  "type": 50,
  "group": "default,vip",
  "models": "gpt-4o,gpt-4o-mini,gpt-image-1",
  "model_configs": "{\"gpt-4o\":{\"ratio\":0.00275,\"completion_ratio\":4,\"max_tokens\":128000},\"gpt-4o-mini\":{\"ratio\":0.00015,\"completion_ratio\":2},\"gpt-image-1\":{\"ratio\":0.0001,\"completion_ratio\":1,\"image\":{\"price_per_image_usd\":0.04,\"default_size\":\"1024x1024\",\"default_quality\":\"standard\"}}}",
  "tooling": "{\"whitelist\":[\"web_search\"],\"pricing\":{\"web_search\":{\"usd_per_call\":0.025}}}"
}
```

Alternative pricing-endpoint payload example (narrow override only):

```json
{
  "model_configs": {
    "gpt-4o": {
      "ratio": 0.0025,
      "completion_ratio": 4,
      "max_tokens": 128000
    }
  },
  "tooling": {
    "whitelist": ["web_search"],
    "pricing": {
      "web_search": { "quota_per_call": 12000 }
    }
  }
}
```

### 3. Reconcile Request Cost by Request ID

1. Capture response header `X-Oneapi-Request-Id`.
2. Query `GET /api/cost/request/:request_id`.
3. Response includes `quota` and computed `cost_usd = quota / 500000`.

### 4. Record External Consumption

Use `POST /api/token/consume` (token-authenticated). Current implementation supports lifecycle phases:

- `single` (default)
- `pre`
- `post`
- `cancel`

Minimal single-step example:

```json
{
  "add_used_quota": 1200,
  "add_reason": "external_service_a"
}
```

Two-phase example:

```json
{ "phase": "pre", "add_used_quota": 2000, "add_reason": "job-42" }
```

```json
{ "phase": "post", "transaction_id": "<id>", "final_used_quota": 1600, "add_reason": "job-42" }
```

### `/api/token/consume` field semantics

- `add_used_quota` (`uint64`): quota amount for `single`/`pre` phase, or fallback final amount for `post`.
- `add_reason` (`string`, required): business source label.
- `phase` (`single|pre|post|cancel`, optional): defaults to `single` when omitted.
- `transaction_id` (`string`): required for `post` and `cancel`.
- `final_used_quota` (`uint64`): explicit reconciled final amount for `post`.
- `timeout_seconds` (`int64`): optional hold timeout for `pre` transaction.
- `elapsed_time_ms` (`int64`): optional latency metadata.

## Reference API Surface

| Purpose                       | Method & Endpoint                                      | Notes                                                                                     |
| ----------------------------- | ------------------------------------------------------ | ----------------------------------------------------------------------------------------- |
| Fetch channel pricing         | `GET /api/channel/pricing/:id`                         | Returns `model_ratio`, `completion_ratio`, `model_configs`, `tooling`.                    |
| Fetch adapter/global defaults | `GET /api/channel/default-pricing?type=<channel_type>` | `data.*` fields are JSON strings; OpenAI-compatible channel types use global pricing map. |
| Update channel pricing        | `PUT /api/channel/pricing/:id`                         | Replaces saved channel pricing map with provided `model_configs`.                         |
| Update channel (full)         | `PUT /api/channel/`                                    | Modern UI path; can include `model_configs` and `tooling`.                                |
| Inspect user quota            | `GET /api/user/:id`                                    | Admin endpoint.                                                                           |
| Inspect token quota           | `GET /api/token/:id`                                   | Token owner endpoint.                                                                     |
| Record external billing       | `POST /api/token/consume`                              | Supports `single/pre/post/cancel` phase model.                                            |
| Request cost lookup           | `GET /api/cost/request/:request_id`                    | Returns request-level quota and `cost_usd`.                                               |
| Debug channel merged config   | `POST /api/debug/channel/:id/debug`                    | Channel-level config debug view.                                                          |
| Validate all channels         | `GET /api/debug/channels/validate`                     | Bulk validation for malformed configs.                                                    |

## API Field Reference (Detailed)

### `GET /api/channel/pricing/:id` response fields

- `model_ratio`: map from model to ratio (derived from saved `model_configs`).
- `completion_ratio`: map from model to completion ratio (derived from saved `model_configs`).
- `model_configs`: saved per-model config object.
- `tooling`: parsed tooling config object (whitelist + pricing).

### `PUT /api/channel/pricing/:id` request fields

- `model_configs`: preferred format (`map[string]ModelConfigLocal`).
- `model_ratio`, `completion_ratio`: legacy format (auto-converted into `model_configs`).
- `tooling`: object or JSON string; parser accepts both.

### `GET /api/channel/default-pricing?type=...` response fields

- `data.model_ratio` (JSON string)
- `data.completion_ratio` (JSON string)
- `data.model_configs` (JSON string)
- `data.tooling` (JSON string, optional)

### `PUT /api/user/` billing-related fields

- `id` (required for update)
- `quota` (non-negative)
- `group` (non-empty, max length validation)

### `PUT /api/token/` billing-related fields

- `id` (required)
- `remain_quota`
- `unlimited_quota`
- `expired_time`
- `status`
- `models`

### `GET /api/cost/request/:request_id` response fields

- `request_id`
- `quota`
- `cost_usd` (derived as `quota / 500000`)

## Implementation Caveats (Important)

1. **Channel `model_configs` does not currently persist tier/cache ratio fields**
   - Current channel-side struct (`model.ModelConfigLocal`) has no `tiers`, `cached_input_ratio`, `cache_write_5m_ratio`, `cache_write_1h_ratio` fields.
   - Those fields still exist in adapter/global defaults and are used by runtime billing.

2. **`GET /api/channel/default-pricing` response shape is stringified JSON**
   - `model_ratio`, `completion_ratio`, `model_configs`, and `tooling` are returned as strings.
   - Current `model_configs` construction in this endpoint includes `ratio`, `completion_ratio`, `max_tokens`, and `video`; it does not currently project audio/image/cache/tier fields.
   - Parse JSON before using.

3. **`PUT /api/channel/pricing/:id` does not deep-merge per model**
   - Provided `model_configs` becomes the saved override map for the channel.
   - Models omitted from override fall back to adapter/global pricing.

4. **Tool allowlist and pricing are coupled at runtime**
   - Built-in tools are considered allowed only when pricing metadata exists (provider default or channel override), then whitelist further restricts.

5. **Streaming billing is incremental and can cut off mid-stream when quota runs out**
   - Flush interval defaults to 3 seconds and is configurable by `STREAMING_BILLING_INTERVAL`.

6. **Admin Create User currently ignores quota/group fields in request body**
   - `POST /api/user/` currently creates user mainly from username/password/display_name flow, then applies default quota/token bootstrap from server config.
   - To set billing quota/group explicitly, update the user afterward via `PUT /api/user/`.

7. **Modern channel form validator expects ratio/completion/max_tokens presence in model configs**
   - Backend supports richer model config fields, but frontend validation currently requires at least one of `ratio`, `completion_ratio`, or `max_tokens` for each model entry.
