# API Format Conversion Guide

one-api provides transparent, bidirectional conversion across the three chat-style APIs it exposes:

- **OpenAI Chat Completions** (`/v1/chat/completions`)
- **OpenAI Responses** (`/v1/responses`)
- **Claude Messages** (`/v1/messages`)

Regardless of the entrypoint a caller chooses, the platform delivers the reply using the same format the caller used while freely routing the upstream call to whatever protocol the target model or channel actually understands.

## Menu

- [API Format Conversion Guide](#api-format-conversion-guide)
  - [Menu](#menu)
  - [1. Supported Conversions at a Glance](#1-supported-conversions-at-a-glance)
  - [2. Request Routing Overview](#2-request-routing-overview)
    - [2.1 Default Endpoint Configuration](#21-default-endpoint-configuration)
  - [3. Automatic Format Detection](#3-automatic-format-detection)
    - [3.1 Format Detection Logic](#31-format-detection-logic)
    - [3.2 Configuration](#32-configuration)
    - [3.3 Handling Modes](#33-handling-modes)
    - [3.4 Implementation](#34-implementation)
  - [4. Entry Point Details](#4-entry-point-details)
    - [4.1 Chat Completions (`/v1/chat/completions`)](#41-chat-completions-v1chatcompletions)
    - [4.2 Responses (`/v1/responses`)](#42-responses-v1responses)
    - [4.3 Claude Messages (`/v1/messages`)](#43-claude-messages-v1messages)
  - [5. Capability Detection \& Sanitisation](#5-capability-detection--sanitisation)
  - [6. Streaming Behaviour](#6-streaming-behaviour)
  - [7. Error Handling \& Billing](#7-error-handling--billing)
  - [8. Context Keys \& Runtime Flags](#8-context-keys--runtime-flags)
  - [9. Testing Coverage](#9-testing-coverage)
  - [10. Summary \& Further Reading](#10-summary--further-reading)

## 1. Supported Conversions at a Glance

| User Request Format | Possible Upstream Formats                                 | Response Back to User   |
| ------------------- | --------------------------------------------------------- | ----------------------- |
| Chat Completions    | Chat Completions • Responses • Claude Messages            | Always Chat Completions |
| Responses           | Responses • Chat Completions (fallback) • Claude Messages | Always Responses        |
| Claude Messages     | Claude Messages • Chat Completions • Responses            | Always Claude Messages  |

Key points:

- Native-capable channels are contacted using their preferred protocol (for example, OpenAI GPT-4o via Responses, Anthropic Claude via Claude Messages).
- Channels lacking native Responses support automatically fall back to Chat Completions while the controller rebuilds a Responses payload for the caller.
- Cross-family access (Claude client → OpenAI model, OpenAI client → Claude model, etc.) works without user code changes.

## 2. Request Routing Overview

```text
Incoming request --> Auto-detect API format (if misrouted)
                  --> Identify controller (Chat / Response / Claude)
                  --> Parse model + channel metadata
                  --> Apply capability gates + sanitizers
                  --> Convert request if upstream protocol differs
                  --> Call adaptor
                  --> Convert response/stream back to caller format
                  --> Reconcile usage + quota
```

Important building blocks:

- `relay/format` – fast format detection based on payload structure.
- `middleware.APIFormatAutoDetect` – middleware that re-routes mismatched requests.
- `meta.Meta` stores routing facts (channel, model mapping, URL path, fallback flags).
- `relay/channeltype/endpoints.go` – defines default supported endpoints for each channel type.
- Conversion utilities live primarily in `relay/adaptor/openai` and `relay/adaptor/openai_compatible`.
- Controllers (`relay/controller/*.go`) coordinate conversion, quota, and response rewriting.

### 2.1 Default Endpoint Configuration

Every chat-capable channel type is configured with default endpoints in `relay/channeltype/endpoints.go`. The `DefaultEndpointsForChannelType` function returns the supported endpoints for each channel type. **All chat-capable channels support these three core APIs:**

- `EndpointChatCompletions` – Chat Completions API (`/v1/chat/completions`)
- `EndpointResponseAPI` – Response API (`/v1/responses`)
- `EndpointClaudeMessages` – Claude Messages API (`/v1/messages`)

This ensures users can freely call any of these three API formats regardless of the upstream channel type. The platform transparently converts requests and responses as needed.

## 3. Automatic Format Detection

Some popular AI clients (e.g., Cursor) may mistakenly send requests to the wrong endpoint (for example, Response API format to `/v1/chat/completions`). one-api includes automatic format detection to handle these cases transparently.

### 3.1 Format Detection Logic

The `relay/format.DetectFormat` function analyzes the request payload and identifies the format based on distinguishing fields:

| Format          | Key Indicators                                                                                                               |
| --------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Response API    | Has `input` field (without `messages`), or `instructions`, or `max_output_tokens`                                            |
| Claude Messages | Has `messages` with Claude-specific indicators: `system` field, `tool_use`/`tool_result` content, or `input_schema` in tools |
| ChatCompletion  | Has `messages` with standard OpenAI format (no Claude-specific indicators)                                                   |

### 3.2 Configuration

Control auto-detection behavior via environment variables:

| Variable                        | Default       | Description                                                                                          |
| ------------------------------- | ------------- | ---------------------------------------------------------------------------------------------------- |
| `AUTO_DETECT_API_FORMAT`        | `true`        | Enable/disable automatic format detection                                                            |
| `AUTO_DETECT_API_FORMAT_ACTION` | `transparent` | Action when mismatch detected: `transparent` (re-route internally) or `redirect` (HTTP 302 redirect) |

### 3.3 Handling Modes

- **Transparent (default):** The middleware rewrites the request path and re-dispatches to the correct handler internally. The client receives the response in the format they actually sent.

- **Redirect:** Returns an HTTP 302 redirect to the correct endpoint. Query parameters are preserved in the redirect URL. Note that most API clients may not follow POST redirects correctly, so `transparent` mode is recommended.

### 3.4 Implementation

The middleware (`middleware.APIFormatAutoDetect`) runs early in the request chain:

1. Reads and buffers the request body
2. Calls `format.DetectFormat` to identify the actual payload format
3. Compares with the expected format based on the URL path
4. On mismatch, either re-routes transparently or returns a redirect

## 4. Entry Point Details

### 4.1 Chat Completions (`/v1/chat/completions`)

1. Controller parses the payload into `relay/model.GeneralOpenAIRequest`.
2. If the downstream channel is OpenAI **and** `IsModelsOnlySupportedByChatCompletionAPI` returns `false` **and** the original URL was `/v1/chat/completions`, the adaptor upgrades the request to a Responses payload via `ConvertChatCompletionToResponseAPI`.
3. The converted request is cached under `ctxkey.ConvertedRequest` so the response path knows it must translate the upstream payload back into Chat Completion format.
4. When the adaptor detects that the channel only offers Chat Completions (search models or third-party compatibles), it forwards the original request unchanged.
5. Response handling mirrors the request decision: Responses bodies are converted with `ResponseAPIHandler`, while vanilla Chat Completion bodies use the standard handler.

### 4.2 Responses (`/v1/responses`)

1. The controller parses the JSON into `openai.ResponseAPIRequest`, then runs `sanitizeResponseAPIRequest` to clear unsupported parameters (for example, reasoning models drop `temperature`/`top_p`).
2. `normalizeResponseAPIRawBody` rewrites the raw JSON in-place so that forbidden fields are removed before the request ever leaves the proxy. This keeps upstream validation errors from leaking to callers.
3. `supportsNativeResponseAPI(meta)` decides whether the channel can speak Responses directly. Native Response API support is enabled for:
   - Official OpenAI endpoints (`api.openai.com`)
   - Azure channels using GPT-5 models (via `AzureRequiresResponseAPI`)
   - XAI channels (native support)
   - OpenAI-compatible channels with explicit `response_api` format configuration
     All other channels fall back to Chat Completions.
4. When falling back, the controller converts the request with `ConvertResponseAPIToChatCompletionRequest`, updates `meta.RequestURLPath` to `/v1/chat/completions`, and sets `meta.ResponseAPIFallback = true` to avoid recursive reconversion later in the pipeline.
5. The adaptor call proceeds. If a fallback was used, the upstream Chat Completion response is transformed back into a Responses envelope via `ResponseAPIHandler` (non-streaming) or `ResponseAPIStreamHandler` (streaming). The helper registered under `ctxkey.ResponseRewriteHandler` performs the final rewrite before bytes are flushed to the client.
6. `normalizeResponseAPIRawBody` also deletes `temperature`/`top_p` keys from the raw payload when the sanitized struct dropped them, ensuring double coverage for channels that reject those parameters outright.

### 4.3 Claude Messages (`/v1/messages`)

1. `RelayClaudeMessagesHelper` inspects the requested model to determine the target channel.
2. Anthropic-native channels set `ctxkey.ClaudeMessagesNative` and forward the request untouched.
3. OpenAI-compatible, Gemini, and other providers convert the Claude payload into their preferred format via adaptor-specific `ConvertClaudeRequest` implementations. Most OpenAI compatibles share `openai_compatible.ConvertClaudeRequest`.
4. During response handling, adaptors check `ctxkey.ClaudeMessagesConversion`. When present, they transform the upstream response (or SSE stream) back into Claude Messages events using utilities such as `openai_compatible.HandleClaudeMessagesResponse` and `ConvertOpenAIStreamToClaudeSSE`.
5. The controller always returns Claude-flavoured JSON/SSE to the caller, regardless of the intermediate protocols.

## 5. Capability Detection & Sanitisation

| Concern                                 | Implementation                                                                        | Notes                                                                                             |
| --------------------------------------- | ------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| Model supports only Chat Completions    | `IsModelsOnlySupportedByChatCompletionAPI` (`relay/adaptor/openai/response_model.go`) | Matches search/audio GPT variants; skips Responses conversion.                                    |
| Channel supports native Responses       | `supportsNativeResponseAPI` (`relay/controller/response.go`)                          | OpenAI first-party, Azure GPT-5, XAI, and configured OpenAI-compatible. Others use Chat fallback. |
| Reasoning models reject sampling params | `sanitizeResponseAPIRequest` + `sanitizeChatCompletionRequest`                        | Clears `temperature` and `top_p` for GPT-5 / o-series models.                                     |
| Raw payload must match sanitised struct | `normalizeResponseAPIRawBody`                                                         | Removes stripped keys from the outbound JSON to avoid upstream 400s.                              |
| Fallback recursion protection           | `meta.ResponseAPIFallback`                                                            | Prevents a Chat fallback request from being re-upgraded to Responses downstream.                  |

These safeguards execute before every upstream call, so the same rules apply to retries and multi-channel failovers.

## 6. Streaming Behaviour

- **Responses → Chat fallback:** `ResponseAPIStreamHandler` rebuilds SSE sequences (`response.created`, `response.output_text.delta`, etc.) from Chat Completion chunks and emits the `response.completed` summary once usage is known. The helper also ensures a terminating `data: [DONE]` envelope for clients.
- **Chat → Responses upgrade:** When `/v1/chat/completions` requests are upgraded to Responses, `ResponseAPIDirectStreamHandler` passes through upstream Responses SSE untouched.
- **Claude Messages:** `ConvertOpenAIStreamToClaudeSSE` produces Claude-native event types (`message_start`, `content_block_delta`, …) while accumulating text and tool call arguments for billing.

## 7. Error Handling & Billing

1. Controllers pre-consume quota using the same logic regardless of protocol. Response fallback calls use the Chat Completion quota helpers but reconcile against the final Responses usage once conversion completes.
2. All adaptor errors are wrapped with `openai.ErrorWrapper` (or the channel equivalent) so HTTP status codes and machine-readable error bodies survive conversions.
3. Token accounting prioritises upstream usage. When upstream omits it, the system estimates totals from streamed text, tool call arguments, and prompt size.
4. Billing post-processing funnels through `billing.PostConsumeQuotaDetailed`, which now receives the original Responses model name even after a fallback path.
5. Pricing adaptor resolution is API-first: we resolve by `APIType` before `ChannelType` so pricing defaults follow the protocol family (OpenAI, Claude, Gemini, etc.). This avoids mismatches when a channel type is a generic proxy or does not map 1:1 to a pricing table.

## 8. Context Keys & Runtime Flags

| Key                                                               | Purpose                                                                                                 |
| ----------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `ctxkey.ConvertedRequest`                                         | Stores the converted request (Responses or Chat) so the response side can pick the proper formatter.    |
| `ctxkey.ConvertedResponse`                                        | Holds a fully converted response object awaiting controller flush (used heavily by Claude conversions). |
| `ctxkey.ResponseRewriteHandler`                                   | Callback used in Responses fallback to stream Chat output back as Responses SSE.                        |
| `ctxkey.ResponseAPIRequestOriginal`                               | Snapshot of the user’s original Responses payload for metadata echoes.                                  |
| `ctxkey.ClaudeMessagesConversion` / `ctxkey.ClaudeMessagesNative` | Flags describing the Claude pipeline path.                                                              |
| `meta.ResponseAPIFallback`                                        | Marks that the active request already fell back to Chat Completions.                                    |

Refer to `common/ctxkey/key.go` and `relay/meta/relay_meta.go` for the authoritative list.

## 9. Testing Coverage

Relevant test suites validating the behaviour above:

- `relay/format/detect_test.go` – comprehensive tests for API format detection heuristics.
- `middleware/api_format_detect_test.go` – validates automatic format detection middleware behavior.
- `relay/adaptor/openai/channel_conversion_test.go` – ensures Chat ↔ Responses conversion toggles correctly for OpenAI, Azure, GPT-OSS, etc.
- `relay/controller/response_fallback_test.go`
  - `TestRelayResponseAPIHelper_FallbackStreaming` exercises SSE rewriting.
  - `TestNormalizeResponseAPIRawBody_RemovesUnsupportedParams` guards sanitisation.
- `relay/adaptor/openai/response_model_test.go` – bidirectional conversion tests for Chat ↔ Responses, including function calling and streaming.
- `relay/controller/claude_messages_test.go` plus adaptor-specific suites – validate Claude ↔ OpenAI/Gemini conversions.
- `relay/channeltype/endpoints_test.go` – verifies default endpoint configuration for all channel types.
- `test/adaptor_comprehensive_test.go` – comprehensive tests for all adaptors covering:
  - `TestAdapterChatCompletionSupport` – validates ChatCompletion support for all adaptors.
  - `TestAdapterClaudeMessagesSupport` – validates Claude Messages API support via `ConvertClaudeRequest`.
  - `TestClaudeMessagesRequestConversion` – tests complex Claude request conversions.
  - `TestResponseConversionIntegration` – end-to-end response conversion testing.
- `cmd/test` regression sweep – cross-api smoke tests that hit every public adaptor.

Run everything with:

```bash
GOFLAGS=-race go test ./...
```

(Controllers and adaptors can also be targeted individually for faster iteration.)

## 10. Summary & Further Reading

- All three chat-style APIs can be used interchangeably from the client's perspective; one-api will translate on the fly.
- Automatic format detection handles clients that send requests to incorrect endpoints (configurable via `AUTO_DETECT_API_FORMAT` and `AUTO_DETECT_API_FORMAT_ACTION`).
- Capability detection ensures each upstream sees only the fields it supports, falling back to Chat Completions when Responses is unavailable.
- Streaming and billing remain accurate across conversions thanks to shared handlers and usage reconciliation.
- Internal flags (`ConvertedRequest`, `ResponseAPIFallback`, `ClaudeMessagesConversion`, etc.) tie the request/response lifecycles together.

For deeper implementation insight, explore:

- `relay/format/detect.go` – format detection logic
- `middleware/api_format_detect.go` – auto-detection middleware
- `relay/channeltype/endpoints.go` – default endpoint configuration per channel type
- `relay/controller/response.go`
- `relay/adaptor/openai/adaptor.go`
- `relay/controller/claude_messages.go`
- `relay/adaptor/openai_compatible/claude_messages.go`

These modules contain the authoritative logic that keeps the three API formats in sync.
