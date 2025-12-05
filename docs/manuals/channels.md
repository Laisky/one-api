# Channel Administration Guide

This guide explains how One-API channels work, how to set them up, and what every option on the Modern UI **Edit Channel** page does. It is written for administrators who manage provider connections, quotas, and built-in tools.

## Menu

- [Channel Administration Guide](#channel-administration-guide)
  - [Menu](#menu)
  - [1. Channel Fundamentals](#1-channel-fundamentals)
  - [2. Creating or Editing a Channel](#2-creating-or-editing-a-channel)
    - [2.1 Basic Information](#21-basic-information)
    - [2.2 Provider Credentials \& Config (`config` block)](#22-provider-credentials--config-config-block)
    - [2.3 Advanced JSON Fields](#23-advanced-json-fields)
    - [2.4 Operational Settings](#24-operational-settings)
  - [3. Model Pricing \& Quotas](#3-model-pricing--quotas)
  - [4. Tooling Policy](#4-tooling-policy)
    - [Tooling Config JSON Schema](#tooling-config-json-schema)
  - [5. Groups and Routing](#5-groups-and-routing)
  - [6. Testing \& Monitoring](#6-testing--monitoring)
  - [7. Editing Tips \& Validation Rules](#7-editing-tips--validation-rules)
  - [8. Troubleshooting Checklist](#8-troubleshooting-checklist)
  - [9. Glossary of Data Fields](#9-glossary-of-data-fields)
  - [10. Best Practices](#10-best-practices)
  - [11. API Endpoint Forwarding \& Compatibility](#11-api-endpoint-forwarding--compatibility)
    - [11.1 How Endpoint Routing Works](#111-how-endpoint-routing-works)
    - [11.2 Configurable Endpoint Support](#112-configurable-endpoint-support)
    - [11.3 Endpoint Support Matrix](#113-endpoint-support-matrix)
    - [11.4 OpenAI-Compatible Channel Behavior](#114-openai-compatible-channel-behavior)
    - [11.5 Setting Up Rerank with Custom Providers](#115-setting-up-rerank-with-custom-providers)
    - [11.6 URL Path Construction](#116-url-path-construction)
    - [11.7 Troubleshooting Endpoint Issues](#117-troubleshooting-endpoint-issues)
    - [11.8 Future Considerations](#118-future-considerations)
  - [12. Proxy Channel Type](#12-proxy-channel-type)
    - [12.1 How Proxy Channels Work](#121-how-proxy-channels-work)
    - [12.2 Setting Up a Proxy Channel](#122-setting-up-a-proxy-channel)
    - [12.3 Use Cases](#123-use-cases)
    - [12.4 Proxy Channel Limitations](#124-proxy-channel-limitations)
    - [12.5 Example: Forwarding to a Custom Embedding Service](#125-example-forwarding-to-a-custom-embedding-service)
    - [12.6 Security Considerations](#126-security-considerations)

## 1. Channel Fundamentals

- **What is a channel?** A channel is a configured connection to an upstream AI provider (OpenAI, Azure OpenAI, Anthropic, proxy services, etc.). Channels determine where requests are sent, which models are available, pricing, credentials, rate limits, and tool policies.
- **Why multiple channels?** You can balance traffic, separate staging from production, or expose different model catalogs and pricing to user groups.
- **Lifecycle overview:**
  1. Create channel → supply credentials, base URL, and model list.
  2. Assign pricing, tooling policy, and optional overrides.
  3. Associate channel with user groups / routing logic.
  4. Monitor usage and update status or quotas as needed.

## 2. Creating or Editing a Channel

Open **Channels → Create Channel** or select an existing channel and choose **Edit**. The Modern template renders the same React form for both flows; fields marked with `*` are required.

### 2.1 Basic Information

| Field                | Description                                                                                                                                                                     |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Channel Name\***   | Human-readable label (e.g., `Azure GPT-4 Prod`). Used throughout the dashboard.                                                                                                 |
| **Channel Type\***   | Provider preset. Determines default base URLs, adapter behavior, specialized fields, and available models. You must choose this before other provider-specific controls unlock. |
| **API Key**          | Provider credential. Leave blank while editing to keep the stored secret. Some providers (AWS, Vertex, Coze OAuth) build the key automatically from other fields.               |
| **Base URL**         | Endpoint root. Optional unless the provider demands it (Azure, OpenAI-compatible). Trailing slashes are trimmed automatically.                                                  |
| **Group Membership** | Multi-select of logical user groups (default is always included). Channels are eligible only for the groups you assign here.                                                    |
| **Models**           | Explicit allowlist of model IDs routed through this channel. Empty list means “all supported models”. The helper dialog offers search, bulk add, and clear actions.             |
| **Model Mapping**    | JSON object translating external model names to upstream model IDs (string → string). Useful when clients send `gpt-4` but upstream expects a deployment alias.                 |

### 2.2 Provider Credentials & Config (`config` block)

`config` stores provider-specific metadata as JSON. The UI renders dedicated inputs based on the channel type:

- **Azure OpenAI**: region endpoint, API version (defaults to `2024-03-01-preview` if blank).
- **AWS Bedrock**: region plus access/secret keys (channel key is derived as `AK|SK|Region`).
- **Vertex AI**: region, project ID, and service account JSON.
- **Coze**: choose between Personal Access Token (entered in API Key field) or OAuth JWT JSON blob.
- **Cloudflare**, **plugin** providers, and others expose single-purpose fields like Account ID or plugin parameters.

Any new provider that requires extra configuration will appear in this section after selecting the channel type.

### 2.3 Advanced JSON Fields

| Field                         | Purpose                                                                                                                                                                                                  |
| ----------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Model Configs**             | JSON describing per-model pricing ratios and max tokens. Example structure: `{"gpt-4o": {"ratio": 0.03, "completion_ratio": 2.0, "max_tokens": 128000}}`. Empty → pricing inferred from global defaults. |
| **Tooling Config (JSON)**     | Defines built-in tool policy and pricing. See [Section 4](#4-tooling-policy).                                                                                                                            |
| **System Prompt**             | Optional default system message injected into every request when the upstream supports it.                                                                                                               |
| **Inference Profile ARN Map** | AWS Bedrock only. JSON map of model → Inference Profile ARN.                                                                                                                                             |

All JSON fields accept formatted input; the **Format** buttons auto-indent valid JSON, and empty strings are saved as `null`.

### 2.4 Operational Settings

| Field                                  | Description                                                                                                            |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| **Priority**                           | Higher values are preferred when multiple channels serve the same model and group.                                     |
| **Weight**                             | Legacy load-balancing hint. Unless you rely on historical behavior, set `0`.                                           |
| **Rate Limit**                         | Requests per minute allowed for this channel. `0` means unlimited (subject to upstream throttling).                    |
| **Testing Model** (optional API field) | Preferred model for health checks. When blank, One-API chooses the cheapest configured model.                          |
| **Status**                             | Edited via the channel list (Enable / Disable). Disabled channels stay in the database but are skipped during routing. |

## 3. Model Pricing & Quotas

One-API meters usage in unified quota units. Channel-level pricing can override global defaults:

1. **Model Configs JSON** (recommended): Set `ratio`, `completion_ratio`, and optional `max_tokens` per model. Ratios are expressed as USD per 1M tokens; they are converted automatically to quota units.
2. **Legacy fields** (`model_ratio`, `completion_ratio`): still respected during migration but replaced by `model_configs` in the UI.

When pricing data is missing, One-API falls back to adapter defaults (see `relay/adaptor/*/constants.go`). For accurate billing, provide explicit values that match your provider contract.

**Balance & Usage:** Additional readonly fields (visible in the table, not the form) track balance, last update time, and consumed quota.

## 4. Tooling Policy

Built-in tools (e.g., `web_search`, `code_interpreter`, `file_search`) funnel through a consistent policy engine:

1. **Effective allowlist** is computed from provider defaults + channel overrides.
2. **Pricing** must exist for any allowed tool. If no pricing entry exists, requests are rejected.
3. **Whitelist behavior:**
   - No whitelist → any tool with pricing is allowed.
   - Empty whitelist → identical to “no whitelist”; pricing still controls access.
   - Administrator-specified whitelist → only those tools are allowed, even if provider defaults include others.

### Tooling Config JSON Schema

```json
{
  "whitelist": ["web_search", "code_interpreter"],
  "pricing": {
    "web_search": { "usd_per_call": 0.01 },
    "code_interpreter": { "usd_per_call": 0.03 }
  }
}
```

- `whitelist` entries are case-insensitive and trimmed.
- Pricing can use either `usd_per_call` or `quota_per_call`. When both are absent or zero, the tool is considered unpriced and therefore blocked.
- Clear the field (submit an empty string) to revert to provider defaults.

The UI offers helper buttons:

- **Format**: Reformat JSON using canonical order and indentation.
- **Load Defaults**: Pull adapter defaults (when available) for the selected channel type.
- **Add Tool**: Quick-add a whitelist entry; the UI will prefill pricing from defaults or prompt for manual input.

## 5. Groups and Routing

- **Groups** map to user segments. Each channel must include `default`; you can add more (e.g., `enterprise`, `beta`, `internal`).
- Routing logic selects channels based on user group, model requested, channel priority, and health status.
- For deterministic routing, restrict a channel to a single group and model combination.

## 6. Testing & Monitoring

- **Test Channel** button (on edit page) issues a diagnostic request using the configured testing model. Successful tests confirm credentials and base URL.
- **Status column** in the channel list shows response time, last test timestamp, balance, and auto-disable reasons. Channels auto-disable after repeated errors or quota exhaustion.
- Traces are recorded in `logs/` and the database for auditing.

## 7. Editing Tips & Validation Rules

- Every JSON field is validated client-side with human-readable error messages. Invalid JSON blocks submission.
- Numeric fields (priority, weight, rate limit) accept integers only; blank values become `0`.
- Coze OAuth JWT requires a full JSON object with `client_type`, `client_id`, `coze_www_base`, `coze_api_base`, `private_key`, and `public_key_id`.
- Azure `other` field defaults to the latest supported API version if left blank.
- Clearing sensitive fields: leaving the API key empty when editing keeps the stored value. To remove credentials entirely, disable or delete the channel.

## 8. Troubleshooting Checklist

| Symptom                                                            | Possible Cause                                          | Remedy                                                                         |
| ------------------------------------------------------------------ | ------------------------------------------------------- | ------------------------------------------------------------------------------ |
| Requests still reach `web_search` after removing it from whitelist | Pricing entry left intact while whitelist omitted       | Ensure whitelist contains only approved tools or clear pricing entries as well |
| Channel edit form loads with empty tooling JSON                    | Stored config is `null` or invalid JSON                 | Re-enter JSON, use **Format** to validate                                      |
| 401/403 from provider                                              | API key malformed, missing base URL, or wrong auth type | Re-enter credentials; verify channel type matches provider                     |
| Users hit “no enabled channel” errors                              | Channel disabled, group mismatch, or model not in list  | Re-enable channel, adjust groups/models                                        |

## 9. Glossary of Data Fields

- **Balance / Balance Updated Time**: Optional manual tracking for providers without APIs. Populated by scheduled jobs or manual refresh.
- **Used Quota**: Accumulated quota units consumed by the channel; resets via maintenance or database operations.
- **Model Mapping**: Facilitates backwards compatibility when client model names differ from provider deployments.
- **Config JSON**: Structured storage for adapter-specific metadata (region, auth type, plugin parameters, etc.).

## 10. Best Practices

- Keep at least one fallback channel per critical model with a distinct provider to improve resiliency.
- Match channel names to deployment environments (`Prod`, `Staging`, `EU`, `US`) for easy identification.
- Review tooling policies quarterly. Providers may introduce new built-in tools requiring explicit pricing.
- Before editing a production channel, duplicate it, apply changes to the copy, test thoroughly, then swap traffic.

For additional technical reference, inspect the React implementation (`web/modern/src/pages/channels/EditChannelPage.tsx`) and backend controller (`controller/channel.go`). They align exactly with the options described above.

## 11. API Endpoint Forwarding & Compatibility

One-API supports multiple API formats (OpenAI ChatCompletion, Claude Messages, Response API) and various endpoints (chat, embeddings, rerank, audio, video, images). However, **not every endpoint is supported by every channel type**. This section explains how endpoint routing works and which channel types support which endpoints.

### 11.1 How Endpoint Routing Works

When a request arrives at One-API, the system:

1. **Determines the relay mode** from the request path (e.g., `/v1/rerank` → `Rerank`, `/v1/embeddings` → `Embeddings`)
2. **Maps the channel type to an API type** using internal adapter logic
3. **Selects the appropriate adapter** based on the API type
4. **Checks adapter capabilities** — some endpoints require adapters to implement specific interfaces (e.g., `RerankAdaptor`)
5. **Checks channel endpoint configuration** — each channel can have custom endpoint settings that override defaults
6. **Constructs the upstream URL** using the channel's Base URL and the adapter's `GetRequestURL()` logic

### 11.2 Configurable Endpoint Support

Each channel type has default supported endpoints, but administrators can customize which endpoints a specific channel supports. This is useful when:

- A custom upstream provider only supports a subset of endpoints
- You want to restrict certain channels to specific use cases
- You need to enable endpoints that aren't in the default set for a channel type

**Configuration Options:**

- **Use Defaults**: Leave the endpoint configuration empty to use the channel type's default endpoints
- **Custom Selection**: Select specific endpoints to enable on the channel edit page
- **Reset to Defaults**: Click "Reset to Defaults" to revert to the channel type's default endpoints

**Backward Compatibility**: Existing channels without endpoint configuration will continue to work exactly as before, using the default endpoints for their channel type. No database migration is required.

### 11.3 Endpoint Support Matrix

The following table shows the **default** endpoints supported by each channel type:

| Endpoint                                             | OpenAI | Azure | OpenAI-Compatible | Cohere | Ollama | AWS Bedrock | Vertex AI | Gemini |
| ---------------------------------------------------- | ------ | ----- | ----------------- | ------ | ------ | ----------- | --------- | ------ |
| **Chat Completions** (`/v1/chat/completions`)        | ✅     | ✅    | ✅                | ✅     | ✅     | ✅          | ✅        | ✅     |
| **Response API** (`/v1/responses`)                   | ✅     | ✅    | ✅\*              | ❌     | ❌     | ❌          | ❌        | ❌     |
| **Claude Messages** (`/v1/messages`)                 | ✅     | ✅    | ✅                | ✅     | ❌     | ✅          | ✅        | ❌     |
| **Embeddings** (`/v1/embeddings`)                    | ✅     | ✅    | ✅                | ❌     | ✅     | ✅          | ✅        | ✅     |
| **Rerank** (`/v1/rerank`)                            | ❌     | ❌    | ❌                | ✅     | ❌     | ❌          | ❌        | ❌     |
| **Audio Speech** (`/v1/audio/speech`)                | ✅     | ✅    | ✅                | ❌     | ❌     | ❌          | ❌        | ❌     |
| **Audio Transcription** (`/v1/audio/transcriptions`) | ✅     | ✅    | ✅                | ❌     | ❌     | ❌          | ❌        | ❌     |
| **Images** (`/v1/images/generations`)                | ✅     | ✅    | ✅                | ❌     | ❌     | ❌          | ❌        | ❌     |
| **Video** (`/v1/videos`)                             | ✅     | ❌    | ✅                | ❌     | ❌     | ❌          | ❌        | ❌     |

_Notes:_

- ✅ = Supported by default
- ❌ = Not supported by default
- \* = OpenAI-Compatible Response API support depends on the `API Format` configuration
- Administrators can override these defaults on a per-channel basis

### 11.4 OpenAI-Compatible Channel Behavior

**Important**: When you create an **OpenAI-Compatible** channel with a custom Base URL, it uses the OpenAI adapter internally. This means:

1. **Chat Completions, Embeddings, Audio, Images** → Requests are forwarded to `{Base URL}{request path}` (e.g., `https://your-api.com/v1/chat/completions`)

2. **Rerank** → **NOT SUPPORTED BY DEFAULT**. Even if your upstream provider supports rerank at `/v1/rerank`, One-API's OpenAI adapter does not implement the `RerankAdaptor` interface. The request will fail with:

   ```text
   rerank requests are not supported by adaptor openai
   ```

3. **Response API** → Supported if you configure `API Format: response` in the channel settings. Otherwise, requests are converted to Chat Completions format.

### 11.5 Setting Up Rerank with Custom Providers

If you need to use rerank with a Cohere-compatible API at a custom URL:

1. Create a **Cohere** channel (not OpenAI-Compatible)
2. Set the **Base URL** to your provider's endpoint (e.g., `https://your-cohere-proxy.com`)
3. Add your rerank models to the channel's model list
4. The Cohere adapter will forward rerank requests to `{base URL}/v2/rerank`

### 11.6 URL Path Construction

Different adapters construct upstream URLs differently:

| Channel Type      | Chat Completions Path                   | Embeddings Path                                | Rerank Path        |
| ----------------- | --------------------------------------- | ---------------------------------------------- | ------------------ |
| OpenAI            | `{base}/v1/chat/completions`            | `{base}/v1/embeddings`                         | ❌                 |
| Azure             | `{base}/openai/deployments/{model}/...` | `{base}/openai/deployments/{model}/embeddings` | ❌                 |
| OpenAI-Compatible | `{base}/v1/chat/completions` or custom  | `{base}/v1/embeddings`                         | ❌                 |
| Cohere            | `{base}/v1/chat`                        | ❌                                             | `{base}/v2/rerank` |
| Ollama            | `{base}/api/chat`                       | `{base}/api/embed`                             | ❌                 |

### 11.7 Troubleshooting Endpoint Issues

| Symptom                                          | Cause                                                        | Solution                                                      |
| ------------------------------------------------ | ------------------------------------------------------------ | ------------------------------------------------------------- |
| "rerank requests are not supported by adaptor X" | The selected channel type's adapter doesn't implement rerank | Use a Cohere channel for rerank endpoints                     |
| Requests go to wrong URL path                    | Base URL may include `/v1` suffix causing duplication        | Remove `/v1` from Base URL; the adapter adds it automatically |
| Embeddings work but rerank fails                 | OpenAI-Compatible channels support embeddings but not rerank | Create separate Cohere channel for rerank models              |
| Audio/video endpoints return 404                 | Upstream provider doesn't support these endpoints            | Verify your provider offers these APIs                        |

### 11.8 Future Considerations

The endpoint support matrix is determined by each adapter's implementation. To add rerank support for OpenAI-Compatible channels, the adapter would need to:

1. Implement the `RerankAdaptor` interface
2. Define the appropriate request conversion logic
3. Handle the `relaymode.Rerank` case in `GetRequestURL()`

Community contributions to extend adapter capabilities are welcome via pull requests.

## 12. Proxy Channel Type

The **Proxy** channel type is a special channel designed for transparent request forwarding. Unlike other channel types that convert requests between API formats, the Proxy channel passes requests through to an upstream service with minimal transformation.

### 12.1 How Proxy Channels Work

When you create a Proxy channel:

1. **Request Path**: Requests must use the special path format: `/v1/oneapi/proxy/{channel_id}/{upstream_path}`
2. **URL Construction**: The proxy adaptor strips the prefix and forwards to `{Base URL}{upstream_path}`
3. **Headers**: All request headers are forwarded (except Host, Content-Length, Accept-Encoding, Connection)
4. **No Quota Consumption**: Proxy requests are logged but do not consume user quota

### 12.2 Setting Up a Proxy Channel

1. **Create the channel**:

   - Channel Type: `Proxy`
   - Base URL: The target upstream service (e.g., `https://internal-api.example.com`)
   - API Key: The authorization token to use with the upstream service
   - Models: Optional, not used for routing

2. **Note the Channel ID**: After creation, note the channel's numeric ID (visible in the channel list)

3. **Make requests** using the format:

   ```text
   POST /v1/oneapi/proxy/{channel_id}/any/path/you/want
   Authorization: Bearer your-oneapi-token
   ```

   This will be forwarded to:

   ```text
   POST https://internal-api.example.com/any/path/you/want
   Authorization: {channel's API Key}
   ```

### 12.3 Use Cases

- **Internal APIs**: Forward requests to internal services that don't conform to standard AI API formats
- **Custom Endpoints**: Access provider-specific endpoints not covered by standard adapters
- **API Composition**: Chain multiple services through One-API's authentication layer
- **Legacy Integration**: Wrap older APIs with One-API's token and quota management

### 12.4 Proxy Channel Limitations

| Feature                    | Status                         |
| -------------------------- | ------------------------------ |
| Request Format Conversion  | ❌ Not supported               |
| Response Format Conversion | ❌ Not supported               |
| Streaming                  | ✅ Transparent pass-through    |
| Quota Billing              | ❌ Always 0 (free)             |
| Token Counting             | ❌ Not performed               |
| Model Routing              | ❌ Uses channel ID in URL path |
| Retry on Failure           | ❌ Not applicable              |

### 12.5 Example: Forwarding to a Custom Embedding Service

```bash
# 1. Create Proxy channel with:
#    - Base URL: https://embeddings.internal.com
#    - API Key: sk-internal-key
#    - Note: Channel ID is 42

# 2. Call through One-API:
curl https://your-oneapi-server/v1/oneapi/proxy/42/embed \
  -H "Authorization: Bearer $ONEAPI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"text": "Hello world"}'

# This forwards to:
# POST https://embeddings.internal.com/embed
# Authorization: sk-internal-key
```

### 12.6 Security Considerations

- Proxy channels expose the upstream service through One-API's authentication
- Ensure users authorized for the Proxy channel should have access to the upstream service
- Consider using group restrictions to limit which users can access Proxy channels
- The upstream API key is stored in the channel configuration; users cannot see or modify it
