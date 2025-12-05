# Channels: Technical Implementation Guide

- [Channels: Technical Implementation Guide](#channels-technical-implementation-guide)
  - [Overview](#overview)
  - [Data model and cache](#data-model-and-cache)
  - [Channel selection algorithm](#channel-selection-algorithm)
  - [Retry](#retry)
  - [Failure handling](#failure-handling)
  - [Temporary suspension vs auto-disable policy](#temporary-suspension-vs-auto-disable-policy)
  - [Metrics integration](#metrics-integration)
  - [Configuration knobs](#configuration-knobs)
  - [Operational guidance](#operational-guidance)
  - [Known limitations](#known-limitations)
  - [API Endpoint Routing Architecture](#api-endpoint-routing-architecture)
    - [Channel Type to API Type Mapping](#channel-type-to-api-type-mapping)
    - [Adaptor Resolution Flow](#adaptor-resolution-flow)
    - [Endpoint Interface Requirements](#endpoint-interface-requirements)
    - [Rerank Endpoint Deep Dive](#rerank-endpoint-deep-dive)
    - [URL Construction by Channel Type](#url-construction-by-channel-type)
      - [OpenAI Adaptor (used by OpenAI, OpenAI-Compatible, Azure, etc.)](#openai-adaptor-used-by-openai-openai-compatible-azure-etc)
      - [OpenAI-Compatible URL Helper](#openai-compatible-url-helper)
    - [Audio/Video/Image Endpoints](#audiovideoimage-endpoints)
    - [Endpoint Support Summary](#endpoint-support-summary)
    - [Configurable Channel Endpoint Support](#configurable-channel-endpoint-support)
      - [Configuration Storage](#configuration-storage)
      - [Endpoint Validation Flow](#endpoint-validation-flow)
      - [Default Endpoints by Channel Type](#default-endpoints-by-channel-type)
      - [API Endpoints](#api-endpoints)
      - [Frontend Integration](#frontend-integration)
      - [Backward Compatibility](#backward-compatibility)
    - [Implementation Gap: Rerank for OpenAI-Compatible](#implementation-gap-rerank-for-openai-compatible)

## Overview

This document describes how One API routes requests across channels and how failures trigger retries and channel state changes. It maps directly to the implementation in:

- `controller/relay.go` (request flow, retry driver)
- `model/cache.go` (in‑memory channel selection)
- `model/ability.go` (per group/model/channel capability and suspension)
- `monitor/*` (auto disable decisions and notifications)

Terminology:

- Channel: a provider connection (row in `channels` table) that supports one or more models for one or more groups.
- Ability: tuple (group, model, channel_id) that is enabled/disabled and can be temporarily suspended.

## Data model and cache

- Channels are stored in `channels` with fields including `status`, `group`, `models` (CSV), and `priority`.
- Abilities are stored in `abilities` with fields: `group`, `model`, `channel_id`, `enabled`, `priority`, `suspend_until`.
- The in‑memory cache (`model.InitChannelCache`) builds `group2model2channels`:
  - Only channels with `status = enabled` are considered.
  - Only abilities where `enabled = true` and `suspend_until` is nil or in the past are included.
  - For each (group, model), channels are sorted by priority descending (higher number = higher priority).
- Cache refresh runs every `SYNC_FREQUENCY` seconds; see [Configuration knobs](#configuration-knobs).

## Channel selection algorithm

Selection uses the in‑memory cache when `MEMORY_CACHE_ENABLED` is true; otherwise DB queries are used by the non‑cache variants.

- Default selection (outside of the retry handler) prefers highest priority:
  - Among available candidates for (group, model), pick highest priority (max value); if multiple at that priority, choose randomly.
  - If `DEFAULT_USE_MIN_MAX_TOKENS_MODEL` is true, within the highest priority tier, prefer the smallest `max_tokens` (from `Model Configs`) and randomize among ties. This optimizes for lower capacity usage by default.
- Retry selection (see next section) alters the target tier and/or filters by `max_tokens` depending on error class.

Priority semantics (as implemented): higher integer value = higher priority. “Ignore first priority” in code means “skip the current highest priority tier and try lower tiers.”

## Retry

The retry driver lives in `controller/relay.go::Relay` and executes after an initial attempt fails.

Entry and gating

- Initial attempt: `relayHelper` performs the request against the selected channel.
- If it fails with `bizErr != nil`, we may retry unless a specific channel was forced:
  - If `SpecificChannelId` is present in the request context, retries are disabled and the error is returned immediately.

Retry budget

- Base attempts derive from `RETRY_TIMES` (default 0 = no generic retries).
- Special handling by error class:
  - 429 Too Many Requests (rate limit): if `RETRY_TIMES > 0`, the attempt budget is doubled to probe more alternatives.
  - 413 Request Entity Too Large (capacity): attempt budget becomes “all other channels” for the same (group, model), i.e., `len(channels) - 1`. If the cache lookup fails, fall back to 1 retry. This path activates regardless of `RETRY_TIMES`.
  - 5xx or network transport errors (server/transport transient): keep base budget; avoid reusing the same ability first.
  - Client request errors (4xx due to user input, e.g., schema/validation): no generic retries.

Per‑attempt selection strategy

- A failed channel is added to an in‑memory exclusion set for the duration of this request. Subsequent selections avoid these channels.
- Strategy depends on the classified error of the most recent attempt:
  - Rate‑limit (429):
    - Prefer lower priority tiers first to escape localized throttling. If no lower tier available, fall back to the highest priority tier among remaining candidates.
  - Capacity (413):
    - Prefer channels whose `max_tokens` for the requested model differs from the failed ones (channels with no `max_tokens` limit are also eligible). This avoids immediately retrying channels with the same capacity constraint that just failed.
  - Server/transport transient (5xx/network):
    - Avoid the exact (channel, model) ability first; probe other abilities in the same tier to maintain performance, then drop to lower tiers if needed.
  - Client request errors (4xx due to user input):
    - Do not retry; surface the error.

Request replay

- The original request body is cached (`common.GetRequestBody`) and the HTTP request body is reset for each retry.

Attempt accounting and exit

- Each attempt (initial and retries) records Prometheus metrics; on success the handler returns immediately.
- If selection fails (no candidates) or we exhaust the budget, the last error is returned. For 429 after exhausting retries, the message is rewritten to be more actionable:
  - If multiple channels were tried: “All available channels (N) for this model are currently rate limited, please try again later”.
  - Otherwise: “The current group load is saturated, please try again later”.
- The response always appends the `request_id` for traceability.

## Failure handling

Error classification and side‑effects (per attempt)

- After each failure, `processChannelRelayError` runs asynchronously and classifies the error origin. Actions are scoped to the specific (group, model, channel_id) “ability” unless the issue is proven to be channel‑wide/fatal.

Classification (examples):

- Client request error:
  - 400 and similar schema/validation errors; vendor type `invalid_request_error`.
  - Action: no suspension, no auto‑disable. Emit failure metric only.
- Rate limit (channel‑origin transient):
  - 429, or vendor‑specific rate‑limit types.
  - Action: suspend the ability for `CHANNEL_SUSPEND_SECONDS_FOR_429`.
- Capacity (model/token/context window limits):
  - 413, or explicit vendor messages for token/context overflow.
  - Action: no suspension by default; rely on retry strategy to pick channels with larger `max_tokens`.
- Server/transport transient (channel‑origin):
  - 5xx responses; network timeouts/EOF/connection reset; upstream gateway failures.
  - Action: suspend the ability for a short window `CHANNEL_SUSPEND_SECONDS_FOR_5XX`.
- Auth/permission/quota (potentially channel‑wide):
  - 401/403; error types `authentication_error`, `permission_error`, `insufficient_quota`; known vendor strings like “API key not valid/expired”, “organization restricted”, “已欠费/余额不足”.
  - Action: suspend the ability for `CHANNEL_SUSPEND_SECONDS_FOR_AUTH`. If `monitor.ShouldDisableChannel` deems the condition fatal (e.g., invalid API key or deactivated account), auto‑disable the entire channel.

Debugging aids

- With `DEBUG=true`, the retry loop logs the exclusion set and attempt ordering. If selection fails, it queries the DB for the excluded channels’ suspension status to help diagnose cache vs. DB discrepancies.

Concurrency note

- There is a known race on `bizErr` mutation before response serialization; this is safe in practice because the mutation occurs on the last reference prior to write, but it is noted in the code.

## Temporary suspension vs auto-disable policy

Suspension (temporary, per ability)

- Scope: always the ability (group, model, channel_id), not the entire channel.
- Triggers: channel‑origin/transient classes (rate‑limit 429, server/transport 5xx/network), and optionally auth/quota/permission depending on severity.
- Actions:
  - 429: set `abilities.suspend_until = now + CHANNEL_SUSPEND_SECONDS_FOR_429`.
  - 5xx/network: set `abilities.suspend_until = now + CHANNEL_SUSPEND_SECONDS_FOR_5XX`.
  - Auth/quota/permission: set `abilities.suspend_until = now + CHANNEL_SUSPEND_SECONDS_FOR_AUTH`, unless immediately escalated to auto‑disable by policy.
- Effect: the ability is excluded from selection until suspension expires and the cache refreshes.

Auto‑disable (persistent, per channel)

- Gate: `AUTOMATIC_DISABLE_CHANNEL_ENABLED` must be true.
- Reserved for fatal channel‑wide conditions verified by `monitor.ShouldDisableChannel`:
  - Invalid API key, account deactivated, hard permission denials, permanent organization restrictions, clear vendor policy violations.
- Action: `monitor.DisableChannel` sets `channels.status = auto_disabled` and sends a notification (email or message pusher).

## Metrics integration

- For each request:
  - `RecordChannelRequest` increments in‑flight counters by channel/type and decrements later.
  - `RecordRelayRequest` records success/failure, usage, user quota, and per‑model latency when successful.
  - Post‑refactor: failures should be tagged with error class (client/rate‑limit/capacity/server/auth) to aid dashboards and alerting.

## Configuration knobs

Environment variables (see `common/config/config.go`):

- `CHANNEL_SUSPEND_SECONDS_FOR_429` (int seconds, default 60): ability suspension window after 429.
- `CHANNEL_SUSPEND_SECONDS_FOR_5XX` (int seconds, default 30): ability suspension window after transient 5xx/network errors.
- `CHANNEL_SUSPEND_SECONDS_FOR_AUTH` (int seconds, default 300): ability suspension window after auth/quota/permission errors, unless escalated to channel‑wide auto‑disable.
- `MEMORY_CACHE_ENABLED` (bool): enable in‑memory channel cache. Auto‑enabled when Redis is enabled.
- `SYNC_FREQUENCY` (int seconds, default 600): cache refresh interval for channels and abilities.
- `DEBUG` (bool): verbose retry diagnostics and DB suspension dumps.
- `ENABLE_PROMETHEUS_METRICS` (bool, default true): enable Prometheus metrics.
- `AUTOMATIC_DISABLE_CHANNEL_ENABLED` (bool, default false): allow auto‑disabling channels on fatal errors.
- `DEFAULT_USE_MIN_MAX_TOKENS_MODEL` (bool, default false): default selection prefers smaller `max_tokens` within top priority tier.

## Operational guidance

- Priority assignment: higher integer = higher priority. Place primary channels at higher numbers; backups use lower numbers. The retry engine will intentionally drop to lower tiers first on 429 to escape local rate limits.
- Max tokens configuration: populate `Model Configs` with realistic `max_tokens` per model. This enables the 413 path to move to channels with larger capacity.
- Pinning to a specific channel disables retries: if a request carries a specific channel id (populated into `SpecificChannelId`), the system will not try alternatives.
- Cache consistency: suspensions take effect for new requests after the next cache refresh; the current request already excludes the failed channel via its local exclusion set.
- Prefer ability‑level suspension for transient issues; reserve channel‑wide disable for fatal vendor/account problems.

## Known limitations

- With `RETRY_TIMES = 0`, only 413 errors will trigger multi‑channel retries (due to explicit override). Other errors will not retry unless you set a positive budget.
- The in‑memory cache is eventually consistent (refresh interval). A freshly suspended ability may remain in the cache until the next sync; selection still avoids it within the same request.
- Error string heuristics for classification/auto‑disable are provider‑dependent and may need updates as provider messages evolve.

## API Endpoint Routing Architecture

This section documents how One-API routes different API endpoints to upstream providers, including the adaptor selection process and URL construction logic.

### Channel Type to API Type Mapping

The `ToAPIType()` function in `relay/channeltype/helper.go` maps channel types to API types:

```go
func ToAPIType(channelType int) int {
    apiType := apitype.OpenAI  // Default fallback
    switch channelType {
    case Anthropic: return apitype.Anthropic
    case Cohere:    return apitype.Cohere
    case VertextAI: return apitype.VertexAI
    // ... other explicit mappings
    }
    return apiType  // OpenAI for unmapped types (incl. OpenAICompatible)
}
```

**Critical insight**: `OpenAICompatible` (type 50) does NOT have an explicit mapping—it falls through to `apitype.OpenAI`. This means OpenAI-Compatible channels use the OpenAI adaptor, which has implications for endpoint support.

### Adaptor Resolution Flow

```text
Request Path → relaymode.GetByPath() → RelayMode
Channel Type → channeltype.ToAPIType() → API Type
API Type → relay.GetAdaptor() → Adaptor Instance
```

Example for a rerank request to an OpenAI-Compatible channel:

1. `/v1/rerank` → `relaymode.Rerank`
2. `channeltype.OpenAICompatible` → `apitype.OpenAI`
3. `apitype.OpenAI` → `openai.Adaptor{}`

### Endpoint Interface Requirements

Each relay mode requires specific adaptor interface implementations:

| Relay Mode      | Required Interface              | Implementing Adaptors                |
| --------------- | ------------------------------- | ------------------------------------ |
| ChatCompletions | `Adaptor` (base)                | All adaptors                         |
| Embeddings      | `Adaptor` (base)                | All adaptors (varies by support)     |
| Rerank          | `RerankAdaptor`                 | `cohere.Adaptor` only                |
| ClaudeMessages  | `ConvertClaudeRequest()` method | OpenAI, Cohere, VertexAI, AWS        |
| ResponseAPI     | Handled via conversion          | OpenAI (native), others via fallback |

### Rerank Endpoint Deep Dive

The rerank flow in `relay/controller/rerank.go`:

```go
func prepareRerankRequestBody(c *gin.Context, meta *metalib.Meta,
    adaptorImpl adaptor.Adaptor, request *relaymodel.RerankRequest) (io.Reader, error) {

    // Check if adaptor implements RerankAdaptor interface
    if rerankAdaptor, ok := adaptorImpl.(adaptor.RerankAdaptor); ok {
        converted, err := rerankAdaptor.ConvertRerankRequest(c, request.Clone())
        // ... process
    }

    // If not implemented, fail with error
    return nil, errors.Errorf("rerank requests are not supported by adaptor %s",
        adaptorImpl.GetChannelName())
}
```

**Only Cohere adaptor implements `RerankAdaptor`**:

```go
// relay/adaptor/cohere/adaptor.go
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, request *model.RerankRequest) (any, error)
func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
    switch meta.Mode {
    case relaymode.Rerank:
        return fmt.Sprintf("%s/v2/rerank", meta.BaseURL), nil
    // ...
    }
}
```

### URL Construction by Channel Type

The `GetRequestURL()` method constructs upstream URLs:

#### OpenAI Adaptor (used by OpenAI, OpenAI-Compatible, Azure, etc.)

```go
// relay/adaptor/openai/adaptor.go
func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
    switch meta.ChannelType {
    case channeltype.Azure:
        // Azure-specific: /openai/deployments/{model}/{task}?api-version=...
    case channeltype.OpenAICompatible:
        // Preserves request path, handles /v1 deduplication
        return GetFullRequestURL(meta.BaseURL, requestPath+query, meta.ChannelType), nil
    default:
        // Standard OpenAI: {BaseURL}{RequestURLPath}
        return GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
    }
}
```

#### OpenAI-Compatible URL Helper

```go
// relay/adaptor/openai/helper.go
func GetFullRequestURL(baseURL string, requestURL string, channelType int) string {
    if channelType == channeltype.OpenAICompatible {
        trimmedBase := strings.TrimRight(baseURL, "/")
        path := requestURL
        // Handle /v1 prefix to avoid duplication
        if strings.HasSuffix(trimmedBase, "/v1") {
            path = strings.TrimPrefix(path, "/v1")
        }
        return trimmedBase + path
    }
    // ... other channel types
}
```

### Audio/Video/Image Endpoints

These endpoints in `relay/controller/audio.go` and `relay/controller/video.go` handle URL construction differently:

**Audio**: Constructs URL directly without going through adaptor:

```go
fullRequestURL := openai.GetFullRequestURL(baseURL, requestURL, channelType)
if channelType == channeltype.Azure {
    // Azure-specific audio endpoint construction
    fullRequestURL = fmt.Sprintf("%s/openai/deployments/%s/audio/...", ...)
}
```

**Video**: Uses proxy helper for non-POST requests, otherwise direct forwarding:

```go
if c.Request.Method != http.MethodPost {
    return RelayProxyHelper(c, relaymode.Videos)
}
// ... direct request construction for POST
```

### Endpoint Support Summary

| Channel Type      | Chat | Embeddings | Rerank | Audio | Video | Images |
| ----------------- | ---- | ---------- | ------ | ----- | ----- | ------ |
| OpenAI            | ✅   | ✅         | ❌     | ✅    | ✅    | ✅     |
| Azure             | ✅   | ✅         | ❌     | ✅    | ❌    | ✅     |
| OpenAI-Compatible | ✅   | ✅         | ❌     | ✅    | ✅    | ✅     |
| Cohere            | ✅   | ❌         | ✅     | ❌    | ❌    | ❌     |
| Anthropic         | ✅   | ❌         | ❌     | ❌    | ❌    | ❌     |
| AWS Bedrock       | ✅   | ✅         | ❌     | ❌    | ❌    | ❌     |
| Vertex AI         | ✅   | ✅         | ❌     | ❌    | ❌    | ❌     |
| Ollama            | ✅   | ✅         | ❌     | ❌    | ❌    | ❌     |
| Gemini            | ✅   | ✅         | ❌     | ❌    | ❌    | ❌     |

Note: This table shows **default** endpoints per channel type. Administrators can customize supported endpoints per-channel (see below).

### Configurable Channel Endpoint Support

Channel endpoint support is now configurable on a per-channel basis. This allows administrators to:

1. **Restrict endpoints**: Limit a channel to only specific endpoints (e.g., chat-only)
2. **Expand endpoints**: Enable endpoints not in the channel type's default set
3. **Override defaults**: Customize behavior for specific upstream providers

#### Configuration Storage

Endpoint configuration is stored in the `config` JSON field of the channel:

```json
{
  "supported_endpoints": ["chat_completions", "embeddings", "response_api"]
}
```

When `supported_endpoints` is empty or absent, the channel uses its type's default endpoints (backward compatible).

#### Endpoint Validation Flow

```text
Request → Distributor Middleware → channelSupportsEndpoint() → Route or Skip
```

The `channelSupportsEndpoint()` function in `middleware/distributor.go`:

```go
func channelSupportsEndpoint(channel *model.Channel, relayMode int) bool {
    // Get endpoint name from relay mode
    endpointName := channeltype.RelayModeToEndpointName(relayMode)

    // Get effective supported endpoints
    supportedEndpoints := channel.GetEffectiveSupportedEndpoints()

    // Check if endpoint is supported
    return channeltype.IsEndpointSupportedByName(endpointName, supportedEndpoints)
}
```

#### Default Endpoints by Channel Type

Defined in `relay/channeltype/endpoints.go`:

```go
var defaultEndpointsMap = map[int][]Endpoint{
    OpenAI: {
        EndpointChatCompletions, EndpointCompletions, EndpointEmbeddings,
        EndpointModerations, EndpointImagesGenerations, EndpointImagesEdits,
        EndpointAudioSpeech, EndpointAudioTranscription, EndpointAudioTranslation,
        EndpointResponseAPI, EndpointClaudeMessages, EndpointRealtime, EndpointVideos,
    },
    Cohere: {
        EndpointChatCompletions, EndpointClaudeMessages, EndpointRerank,
    },
    Anthropic: {
        EndpointChatCompletions, EndpointClaudeMessages,
    },
    // ... other channel types
}
```

#### API Endpoints

**GET `/api/channel/metadata?type={channelType}`** returns:

```json
{
  "default_base_url": "https://api.openai.com",
  "base_url_editable": true,
  "default_endpoints": ["chat_completions", "embeddings", "response_api", ...],
  "all_endpoints": [
    {"id": 1, "name": "chat_completions", "description": "Chat Completions", "path": "/v1/chat/completions"},
    {"id": 3, "name": "embeddings", "description": "Embeddings", "path": "/v1/embeddings"},
    // ... all available endpoints
  ]
}
```

#### Frontend Integration

The channel edit page (`web/modern/src/pages/channels/EditChannelPage.tsx`) includes:

1. **ChannelEndpointSettings** component with checkbox toggles for each endpoint
2. "Reset to Defaults" button to revert to channel type defaults
3. "Select All" and "Minimal" quick actions
4. Documentation modal with cURL examples for each endpoint

#### Backward Compatibility

- Existing channels without `supported_endpoints` in config use defaults
- Empty array `[]` is treated as "use defaults" (not "no endpoints")
- No database migration required—uses existing `config` JSON column

### Implementation Gap: Rerank for OpenAI-Compatible

To add rerank support for OpenAI-Compatible channels, the following would be required:

1. **Implement `RerankAdaptor` interface** in the OpenAI adaptor

2. **Add rerank mode handling** in `GetRequestURL()`:

   ```go
   case channeltype.OpenAICompatible:
       if meta.Mode == relaymode.Rerank {
           return fmt.Sprintf("%s/v1/rerank", meta.BaseURL), nil
       }
   ```

3. **Add `ConvertRerankRequest()` method** with proper request mapping

This would allow OpenAI-Compatible channels to forward rerank requests to providers like Jina or other Cohere-API-compatible services at custom URLs.
