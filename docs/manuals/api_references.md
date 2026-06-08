# One API — HTTP API Reference Manual

> **Scope** — the complete HTTP API of the One API gateway: the OpenAI-compatible `/v1` inference surface, the Anthropic-compatible `/v1/messages` surface, and the `/api` management/administration API.
> **API version** — `v1` (inference) · `/api` (management). This manual documents the gateway as of **2026-06-08**, verified against the `main` source tree.
> **Placeholders** — every example uses `$BASE_URL` (your gateway origin, e.g. `https://oneapi.laisky.com` or `http://localhost:3000`), `$API_KEY` (a relay key, `sk-…`), and `$ACCESS_TOKEN` (a management access token). Never paste a real secret into a shared document, and always use HTTPS for token-bearing requests.

## 1. Overview

One API is an open-source, self-hostable **unified AI gateway** — an open implementation of the "OpenRouter" idea. It places a single, stable HTTP surface in front of many upstream model providers (OpenAI, Anthropic, Azure OpenAI, Google Vertex, DeepSeek, Replicate, AWS Bedrock, OpenRouter, and more) and handles authentication, routing, format translation, quota/billing, rate limiting, and observability on their behalf.

It exposes two distinct API surfaces:

| Surface | Prefixes | Audience | Credential |
|---|---|---|---|
| **Inference / relay** | `/v1/*`, `/v2/rerank`, `/mcp`, `/dashboard/billing/*`, `/api/paas/v4/*`, `/openrouter/v1/*` | Applications making AI calls | Relay **API key** (`sk-…`) |
| **Management / admin** | `/api/*` | The web dashboard, operators, and automation | Session cookie **or** management **access token** |

Key capabilities relevant to API consumers:

1. **Multi-provider aggregation** — chat, responses, embeddings, rerank, images, audio (STT/TTS), video, moderations, OCR, and realtime, all behind one base URL.
2. **Transparent format translation** — clients may speak **Chat Completion** (`/v1/chat/completions`), **Response API** (`/v1/responses`), or **Claude Messages** (`/v1/messages`); the gateway converts to each upstream's native format. A request sent to the "wrong" endpoint for its body shape is auto-detected and re-routed.
3. **Multi-tenant management** — users, roles, per-tenant quotas and permissions.
4. **Sub-API-keys** — each tenant can mint multiple relay keys, each optionally scoped to specific models, an IP subnet, an expiry, and its own quota.

A public instance is available at `https://oneapi.laisky.com` (log in with `test` / `12345678`).

### How to read this manual

The manual is **reference documentation**: it is organized to be consulted, not read front-to-back. Start with [Authentication & Authorization](#2-authentication--authorization) to understand the three credential types, skim [Conventions](#3-conventions), then jump to the specific endpoint you need via the [Endpoint Index](#endpoint-index). The [Quickstart](#quickstart) gives you a working call in under a minute.


## Document scope

**This manual covers** the HTTP request/response contract of every route registered by the gateway router (`router/api.go`, `router/relay.go`, `router/dashboard.go`): method, path, required credential, request parameters, response shape, a runnable `curl` example, and notable errors. It is grouped into three tiers — **inference**, **user-level management**, and **admin/root management** — plus public endpoints.

**This manual does not cover** (see the sibling manuals in `docs/manuals/`):

| Topic | See |
|---|---|
| Quota math, pricing ratios, the 4-layer pricing fallback | [`billing.md`](billing.md) |
| Channel object schema, `model_configs`, tooling policy | [`channels.md`](channels.md) |
| Charging an external/upstream account | [`external_billing.md`](external_billing.md) |
| The MCP aggregator concept | [`mcp_aggregator.md`](mcp_aggregator.md) |
| Rerank pricing specifics | [`rerank.md`](rerank.md) |
| OpenRouter upstream provider integration | [`openrouter_provider.md`](openrouter_provider.md) |
| Prometheus metrics / OpenTelemetry | [`PROMETHEUS.md`](PROMETHEUS.md), [`open_telemetry.md`](open_telemetry.md) |
| Deployment / Kubernetes | [`k8s.md`](k8s.md) |

The full, per-provider request/response schemas for OpenAI- and Anthropic-shaped endpoints are owned upstream; for those endpoints this manual documents the **gateway-relevant** parameters (model selection, streaming, auth, billing) and links the body/response shape to the upstream specification rather than restating it in full.


## Contents

**Foundations**

- [1. Overview](#1-overview) · [Document scope](#document-scope) · [Quickstart](#quickstart)
- [2. Authentication & Authorization](#2-authentication--authorization)
- [3. Conventions](#3-conventions)
- [4. Errors & status codes](#4-errors--status-codes)
- [5. Rate limits](#5-rate-limits)
- [6. Billing & quota model](#6-billing--quota-model)

**Inference / relay API** (relay API key)

- [Chat Completions, Text Completions & Responses API](#chat-completions-text-completions--responses-api)
- [Claude Messages & Moderations](#claude-messages--moderations)
- [Realtime API (WebSocket)](#realtime-api-websocket)
- [Embeddings & Rerank](#embeddings--rerank)
- [Images, Audio & Video](#images-audio--video)
- [OCR, MCP, Channel Proxy, Model Discovery & OpenRouter Listing](#ocr-mcp-channel-proxy-model-discovery--openrouter-listing)
- [Usage, Billing Dashboard & API-Key Introspection](#usage-billing-dashboard--api-key-introspection)

**User-level management API** (session or access token)

- [Authentication & Account Lifecycle](#authentication--account-lifecycle)
- [Self-Service Account, Access Token, 2FA, Passkeys, Logs, Trace & Cost](#self-service-account-access-token-2fa-passkeys-logs-trace--cost)
- [API Key (Token) Management](#api-key-token-management)

**Admin & root management API**

- [User Administration & Top-up](#user-administration--top-up)
- [Channel Administration & Diagnostics](#channel-administration--diagnostics)
- [Redemptions, Groups, Logs, Admin Token Visibility & Model Catalog](#redemptions-groups-logs-admin-token-visibility--model-catalog)
- [MCP Server & Tool Administration](#mcp-server--tool-administration)
- [System Options (Root) & Public Endpoints](#system-options-root--public-endpoints)

**Reference**

- [Endpoint index](#endpoint-index)


## Quickstart

### Make your first inference call (relay API key)

If you already hold a relay key (`sk-…`), you can call the gateway exactly like the OpenAI API — just change the base URL:

```bash
curl -s "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "Say hello in one word."}]
  }'
```

Point any OpenAI SDK at the gateway by overriding `base_url`:

```python
from openai import OpenAI
client = OpenAI(base_url="https://oneapi.laisky.com/v1", api_key="sk-...")
client.chat.completions.create(model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Say hello in one word."}])
```

For Anthropic clients, point `base_url` at `$BASE_URL` and call `/v1/messages`.

### Provision a relay key from scratch (no browser)

This is the headless flow that GitHub issue #349 asks about — create a key purely over the API. It uses **two different credentials**, which is the single most important thing to understand about this gateway (see [Authentication](#2-authentication--authorization)):

```bash
# Step 1 — obtain a management ACCESS TOKEN.
# Requires an existing session OR a previously issued access token.
# (First-time bootstrap: log in via POST /api/user/login to get a session cookie,
#  then call this with that cookie. Each call ROTATES the access token.)
curl -s "$BASE_URL/api/user/token" \
  -H "Authorization: $ACCESS_TOKEN"
# -> {"success":true,"message":"","data":"<new 32-char access token>"}

# Step 2 — mint a relay API KEY (sk-...) with that access token.
curl -s -X POST "$BASE_URL/api/token/" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"my-app-key","expired_time":-1,"unlimited_quota":false,"remain_quota":500000}'
# -> {"success":true,"message":"","data":{"id":123,"key":"sk-XXXXXXXX...","name":"my-app-key",...}}

# Step 3 — use data.key from step 2 as the inference credential.
curl -s "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer sk-XXXXXXXX..." \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}'
```

> The full key (`data.key`) is returned **only once**, at creation time. Store it immediately.

See [API Key (Token) Management](#api-key-token-management) for every field accepted at creation.


## 2. Authentication & Authorization

The gateway recognizes **three credential types**. They are not interchangeable, and confusing them is the most common integration mistake.

| Credential | Looks like | Authenticates | Sent as | Validated by |
|---|---|---|---|---|
| **Relay API key** | `sk-` + 48 chars | Inference (`/v1/*`, `/v2/rerank`, `/mcp`, `/dashboard/billing/*`) and the metered `/api/token/*` introspection endpoints | `Authorization: Bearer sk-…` | `TokenAuth` |
| **Management access token** | bare 32-char UUID (no prefix) | The management/admin API under `/api` | `Authorization: <token>` | `ValidateAccessToken` (session fallback) |
| **Session cookie** | HTTP cookie | The same management API, from the web dashboard | browser `Cookie` header | `UserAuth`/`AdminAuth`/`RootAuth` |

### 2.1 Relay API key (`sk-…`)

The credential your applications use for AI calls. It is a **token resource** owned by a user, created via [`POST /api/token`](#api-key-token-management). It carries its own controls: optional model allow-list, IP subnet restriction, expiry, and quota.

Accepted headers (first match wins):

1. `Authorization: Bearer sk-…` — OpenAI style (also accepts the bare key without `Bearer`).
2. `X-Api-Key: sk-…` — Anthropic style.
3. `Api-Key: sk-…` — Azure / GitHub Copilot BYOK style.
4. `Sec-WebSocket-Protocol: openai-insecure-api-key.sk-…` — for the Realtime WebSocket endpoint.

The `sk-` prefix is presentation only — it is configurable (`TOKEN_KEY_PREFIX`, default `sk-`) and is stripped/re-applied at the edge; the stored key is the bare 48 chars.

**Admin channel pinning** — an admin may append a channel id to force routing through one channel: `Authorization: Bearer sk-{key}-{channel_id}`. For non-admin users this is rejected with `403`. (Because parsing splits on `-`, a non-admin key whose text contains extra `-`-delimited segments will also trip this path and `403`; this matters when debugging third-party BYOK clients.)

### 2.2 Management access token

A bare 32-character UUID stored on the **user** record (one per user). It is the credential for the management API when you are not in a browser session. Obtain or rotate it with [`GET /api/user/token`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost):

```bash
curl -s "$BASE_URL/api/user/token" -H "Authorization: $ACCESS_TOKEN"
# -> {"success":true,"message":"","data":"<32-char uuid>"}
```

Send it on subsequent management calls as `Authorization: <token>`. An exact, case-sensitive leading `Bearer ` is stripped if present (so the bare UUID and `Bearer <uuid>` both work, but a lowercase `bearer ` would not be stripped); no `sk-` prefix applies. **Each call to `GET /api/user/token` overwrites the previous access token** — rotating it invalidates the old value everywhere.

> The access token is *not* an `sk-` key and cannot call `/v1/*`; conversely an `sk-` key cannot call the management API. To bootstrap the very first access token without any existing credential, log in via [`POST /api/user/login`](#authentication--account-lifecycle) (which sets a session cookie) and call `GET /api/user/token` with that cookie.

### 2.3 Roles

Every authenticated principal has a role; management endpoints require a minimum role:

| Role | Value | Grants |
|---|---|---|
| Guest | `0` | unauthenticated |
| Common user | `1` | `UserAuth` endpoints (own account, own tokens, own logs) |
| Admin | `10` | `AdminAuth` endpoints (users, channels, redemptions, logs, MCP servers) |
| Root | `100` | `RootAuth` endpoints (system options) |

`OptionalUserAuth` endpoints never reject: an anonymous caller is served a reduced view, an authenticated caller a personalized one.

### 2.4 401 vs 403

The gateway distinguishes these deliberately:

- **`401 Unauthorized`** — the credential is missing, malformed, or unknown. A relay key that is already **expired or exhausted** (`remain_quota` depleted) is also rejected at authentication time with `401`.
- **`403 Forbidden`** — the credential is valid, but the action is not allowed: insufficient role, banned user, model not in the key's allow-list, source IP outside the key's subnet, or **insufficient quota for this request** (detected at billing pre-consume → `insufficient_user_quota` / `insufficient_token_quota`).

### 2.5 Security guidance

- Always use **HTTPS**; a bearer credential grants full access to whoever holds it.
- Mint **one relay key per application/teammate** for blast-radius isolation and clean audit logs; scope keys with `models`, `subnet`, `expired_time`, and `remain_quota`.
- Store credentials in environment variables or a secrets manager; never commit them.
- Rotate the management access token (re-`GET /api/user/token`) if it may have leaked, and disable/delete compromised relay keys via the token management API.


## 3. Conventions

### Base URL & transport

All paths are relative to your gateway origin, written `$BASE_URL`. Inference paths live under `/v1` (plus `/v2/rerank`, `/mcp`, `/dashboard/billing/*`, `/api/paas/v4/*`, `/openrouter/v1/*`); management paths live under `/api`. Request and response bodies are `application/json` unless noted (file uploads use `multipart/form-data`; TTS returns binary audio; the Realtime endpoint upgrades to WebSocket).

### Two response envelopes

The gateway uses **different envelopes for the two surfaces** — know which one you are talking to:

**Management API** (`/api/*`) — a wrapper object, almost always returned with **HTTP 200** even on logical failure (inspect the `success` field, not only the status code):

```json
{ "success": true, "message": "", "data": { "id": 3, "username": "alice", "quota": 250000 } }
```

```json
{ "success": false, "message": "Token name is required" }
```

`data` may be an object, an array, or `null`. Session/access-token **auth** failures are the exception that *do* set a real `401`/`403` status alongside `{"success":false,...}`.

**Inference / relay API** (`/v1/*`, etc.) — passes the upstream response through on success, and on failure emits an **OpenAI-style error object with the real HTTP status code**:

```json
{ "error": { "message": "…", "type": "one_api_error", "param": "", "code": "…" } }
```

### Identifiers & timestamps

- Resource ids are integers (`id`, `user_id`, `channel_id`).
- Token timestamps `created_time`, `accessed_time`, `expired_time` are **Unix seconds** (`expired_time: -1` means "never"). GORM bookkeeping fields `created_at`/`updated_at` are millisecond timestamps.
- Field names are `snake_case` in JSON. Use them verbatim — never paraphrase a field name.

### Lists, pagination & filtering

List endpoints accept pagination and filter query parameters; the exact names are documented per endpoint (commonly a page index such as `p`/`page` plus `page_size`, and filters such as `keyword`, `type`, `model_name`, `start_timestamp`, `end_timestamp`). Empty results return an empty array in `data`, not an error.

### Request tracing

Each request is assigned a request id (surfaced in logs and recoverable via [`GET /api/cost/request/:request_id`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) and the trace endpoints). Quote it when reporting an issue so operators can correlate your call with server logs and the per-request cost record.

### Streaming (SSE)

Chat/Responses/Claude endpoints support `stream: true`. The response is `text/event-stream`: a sequence of `data: {json}` chunks. OpenAI-shaped streams send incremental `choices[].delta` chunks, a final chunk carrying `usage`, and terminate with the literal `data: [DONE]` sentinel. Claude-shaped streams (`/v1/messages`) use Anthropic event types (`message_start`, `content_block_delta`, `message_delta`, `message_stop`). Errors that occur mid-stream are emitted as an error event in the stream rather than changing the already-sent HTTP status.


## 4. Errors & status codes

### Error shapes

See [Conventions](#two-response-envelopes) for the two envelopes. In short:

- **Management** errors: `{"success": false, "message": "<reason>"}` (usually HTTP 200; auth failures use a real `401`/`403`).
- **Relay** errors: `{"error": {"message", "type", "param", "code"}}` with the real HTTP status. `type` is one of the upstream error types or `one_api_error` for gateway-originated errors; `code` is a machine-readable string (e.g. `insufficient_user_quota`, `api_not_implemented`).

### HTTP status codes

| Code | Meaning in this gateway | Client action |
|---|---|---|
| `200 OK` | Success. Also the transport status for most management *logical errors* — check `success`. Relay success passes the upstream 200 through. | Proceed; for `/api`, read `success`. |
| `400 Bad Request` | Malformed request, schema validation failure, or model unsupported by the chosen channel. | Fix the request; do not blind-retry. |
| `401 Unauthorized` | Missing/invalid/expired credential (bad session, bad access token, or bad/disabled relay key). | Re-authenticate; check the key. |
| `403 Forbidden` | Valid credential, not allowed: banned user, insufficient role, model not permitted for the key, subnet restriction, or insufficient quota (`insufficient_user_quota` / `insufficient_token_quota`). | Adjust scope/role, or top up quota. |
| `404 Not Found` | Unknown route or resource. Relay returns an OpenAI-style `invalid_request_error`. | Check the path/id. |
| `408 Request Timeout` | Client-side timeout/cancellation. | Retry if appropriate. |
| `413 Payload Too Large` | Request body exceeds capacity. | Reduce payload. |
| `429 Too Many Requests` | Gateway rate limit hit, or upstream 429. After retries are exhausted across channels: "All available channels … are currently rate limited". | Back off and retry; see [Rate limits](#5-rate-limits). |
| `500 Internal Server Error` | Gateway/infrastructure failure. | Retry with backoff; report with the request id. |
| `501 Not Implemented` | Endpoint registered but unimplemented (e.g. `images/variations`, `files`, `fine_tuning`, `assistants`, `threads`). Returns `type: one_api_error`, `code: api_not_implemented`. | Do not call. |
| `502/503/504` | Upstream errors, or `503` "No available channels" when no channel serves the requested model+group. | Retry; verify model availability. |

> **No client-facing rate-limit headers** are emitted — the gateway does not set `X-RateLimit-*` or `Retry-After`. Treat `429` as the signal and apply exponential backoff.

### Channel suspension (relay only)

For relay calls, certain upstream statuses (notably `401`, `429`, and non-retryable `5xx`) cause the gateway to temporarily suspend or disable the affected channel/model ability and transparently retry on another eligible channel before surfacing an error. This is invisible to a well-behaved client beyond slightly higher latency on a bad channel.


## 5. Rate limits

Rate limiting uses fixed-window counters, backed by Redis when configured (otherwise in-process). Each limiter is a no-op when its limit is `<= 0` or when `RATE_LIMIT_DISABLED` is set. `DEBUG` never changes rate-limit behavior. Defaults below are the out-of-the-box values and are operator-configurable.

| Limiter | Applies to | Keyed by | Default |
|---|---|---|---|
| Global API | management API + billing dashboard (`/api`, `/dashboard`) | client IP | 480 / 180s |
| Global Web | web dashboard pages | client IP | 240 / 180s |
| Critical | login, register, OAuth, email verification, password reset | client IP | 20 / 1200s |
| Global Relay | inference (`/v1`, `/v2`, …) | **hashed API key** | 480 / 180s |
| Low-balance Relay | inference, when the user's live balance is below `LOW_BALANCE_RATE_LIMIT_THRESHOLD` (default ≈ $0.5) | user id | off unless tightened |
| Channel | per channel, only if `GLOBAL_CHANNEL_RATE_LIMIT=true` | hashed key + channel id | 1 / window |
| TOTP | 2FA verification | user id | 1 / s |

Notes:

- The relay limiter is keyed on the **API key**, not the IP — so concurrency budgets are per-key.
- Admins and sufficiently-funded users bypass the low-balance limiter; when a low-balance user is throttled the `429` message explains the balance/threshold and advises topping up.
- On breach, relay endpoints return `429` with the OpenAI-style error envelope; management endpoints return `429` likewise. There are **no** `X-RateLimit-*` or `Retry-After` response headers.
- Redis-error behavior differs by limiter: the **low-balance and TOTP** limiters **fail open** (allow the request, log a warning), but the **Global API / Global Web / Critical / Global Relay / Channel** limiters **fail closed** — a Redis error surfaces as `500`. Operators should monitor Redis availability.


## 6. Billing & quota model

### The quota unit

Balances are tracked as an internal integer "quota" unit. The conversion is fixed:

> **500,000 quota = 1 USD** (so 1 quota = $0.000002). For reference, $1 = 7 RMB in the built-in exchange rate.

Two balances exist: the **user balance** (`User.quota`) and an optional **per-key balance** (`Token.remain_quota`). The dashboard renders quota as currency by dividing by 500,000.

### How a request is charged

For token-metered models the charge is, in quota units:

```
quota = ceil( (prompt_tokens * model_ratio
               + completion_tokens * model_ratio * completion_ratio)
              * group_ratio )
```

where `model_ratio` is expressed in milli-token-USD units (a model priced at $2.50 / 1M tokens has `model_ratio = 1.25`), `completion_ratio` scales output tokens, and `group_ratio` is the user's group multiplier (default `1`). Per-call / per-second models (rerank, OCR, image, video) charge a fixed quota instead of counting tokens. Full pricing rules, cached-token discounts, and the 4-layer pricing fallback (channel override → provider default → global pricing → safe default) are in [`billing.md`](billing.md).

### Two-balance enforcement

Every billed relay request is charged in two phases:

1. **Pre-consume** (before the upstream call) reserves the estimated quota. The **owning user's** balance is checked first → `403 insufficient_user_quota` if too low; then, if the key is *not* unlimited and its `remain_quota` is too low → `403 insufficient_token_quota`. The reserved amount is deducted from the user (always) and the key (unless unlimited).
2. **Post-consume** (after the response) reconciles the difference: a positive delta deducts more, a negative delta refunds. Again applied to the user always, and to the key only when it is not unlimited.

`unlimited_quota: true` on a key means the key is **exempt from its own per-key cap** — its `remain_quota` is neither checked nor decremented — **but the owning user's balance is still enforced and still drained.** So an unlimited key cannot spend beyond the user account's balance.

A non-unlimited key whose `remain_quota` has reached `0` fails authentication (status transitions to *Exhausted*). A low-balance reminder email may fire at a configurable threshold but does not block requests.

### Inspecting usage

- [`GET /dashboard/billing/subscription`](#usage-billing-dashboard--api-key-introspection) and [`/usage`](#usage-billing-dashboard--api-key-introspection) — OpenAI-compatible balance/usage, authenticated with the relay key.
- [`GET /api/token/balance`](#usage-billing-dashboard--api-key-introspection), `/transactions`, `/logs` — gateway-native key introspection.
- [`GET /api/cost/request/:request_id`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) — the exact recorded cost of one request.


## Endpoint index

Every route the gateway serves, grouped by section. Each row links to its section. **Auth** legend: **API key** = relay `sk-…` key (`TokenAuth`); **Access token / session** = management credential (`UserAuth`); **Admin** = `AdminAuth`; **Root** = `RootAuth`; **Public** = no credential required.

_Total: 154 active routes across 15 sections (plus 38 reserved OpenAI endpoints that return `501 Not Implemented`)._

**[Chat Completions, Text Completions & Responses API](#chat-completions-text-completions--responses-api)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `POST` | [`/v1/chat/completions`](#chat-completions-text-completions--responses-api) | API key | Primary chat completion inference; supports streaming, tools, reasoning. |
| `POST` | [`/v1/completions`](#chat-completions-text-completions--responses-api) | API key | Legacy single-prompt text completion. |
| `POST` | [`/v1/edits`](#chat-completions-text-completions--responses-api) | API key | Legacy edits; apply an instruction to optional input text (Edits relay mode). |
| `POST` | [`/v1/responses`](#chat-completions-text-completions--responses-api) | API key | Create a Response API response; native pass-through or chat fallback. |
| `GET` | [`/v1/responses`](#chat-completions-text-completions--responses-api) | API key | Same body-based handler as POST /v1/responses (RelayResponseAPIHelper); stream toggled by body field, not q… |
| `GET` | [`/v1/responses/:response_id`](#chat-completions-text-completions--responses-api) | API key | Retrieve a stored response (OpenAI channels only); stream query param supported. |
| `DELETE` | [`/v1/responses/:response_id`](#chat-completions-text-completions--responses-api) | API key | Delete a stored response (OpenAI channels only); upstream body/status forwarded verbatim. |
| `POST` | [`/v1/responses/:response_id/cancel`](#chat-completions-text-completions--responses-api) | API key | Cancel an in-progress background response (OpenAI channels only). |

**[Claude Messages & Moderations](#claude-messages--moderations)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `POST` | [`/v1/messages`](#claude-messages--moderations) | API key | Relay an Anthropic Messages-format request to a Claude-capable channel; supports direct pass-through and SS… |
| `POST` | [`/v1/moderations`](#claude-messages--moderations) | API key | Relay an OpenAI-format content moderation request; classifies input text against policy categories. Model d… |

**[Realtime API (WebSocket)](#realtime-api-websocket)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | [`/v1/realtime`](#realtime-api-websocket) | API key | WebSocket upgrade that proxies a full OpenAI Realtime audio/text session; pre-consumes a conservative quota… |
| `POST` | [`/v1/realtime/sessions`](#realtime-api-websocket) | API key | Proxies upstream Realtime Sessions to mint an ephemeral client token for WebRTC; enforces the body model ag… |

**[Embeddings & Rerank](#embeddings--rerank)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `POST` | [`/v1/embeddings`](#embeddings--rerank) | API key | Create embedding vectors for input text (OpenAI Embeddings schema). |
| `POST` | [`/v1/engines/:model/embeddings`](#embeddings--rerank) | API key | Legacy engines-form embeddings; model resolved from path when body model is empty. |
| `POST` | [`/v1/rerank`](#embeddings--rerank) | API key | Rerank documents by relevance to a query (Jina/Cohere-style); per-call billing. |
| `POST` | [`/v2/rerank`](#embeddings--rerank) | API key | Cohere/Jina v2 rerank path; identical handler/schema/billing to /v1/rerank. |

**[Images, Audio & Video](#images-audio--video)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `POST` | [`/v1/images/generations`](#images-audio--video) | API key | Generate images from a text prompt (DALL-E, gpt-image, CogView, etc.). |
| `POST` | [`/v1/images/edits`](#images-audio--video) | API key | Edit/extend an uploaded image (multipart) with a prompt and optional mask. |
| `POST` | [`/v1/images/variations`](#images-audio--video) | API key | Not implemented; TokenAuth still required, then returns 501 api_not_implemented. |
| `POST` | [`/v1/audio/transcriptions`](#images-audio--video) | API key | Transcribe uploaded audio to text (multipart); billed by audio duration. |
| `POST` | [`/v1/audio/translations`](#images-audio--video) | API key | Translate uploaded audio to English text (multipart). |
| `POST` | [`/v1/audio/speech`](#images-audio--video) | API key | Text-to-speech; returns a raw binary audio stream, billed by input chars. |
| `POST` | [`/v1/videos`](#images-audio--video) | API key | Create an async video-generation task; billed per second by resolution. |
| `GET` | [`/v1/videos`](#images-audio--video) | API key | List the caller's video tasks; proxied, no per-second billing. |
| `GET` | [`/v1/videos/:video_id`](#images-audio--video) | API key | Poll a single video task's status/metadata; proxied, no per-second billing. |
| `GET` | [`/v1/videos/:video_id/content`](#images-audio--video) | API key | Download rendered video bytes (raw stream); proxied, no per-second billing. |
| `DELETE` | [`/v1/videos/:video_id`](#images-audio--video) | API key | Cancel/delete a video task; proxied, no per-second billing. |

**[OCR, MCP, Channel Proxy, Model Discovery & OpenRouter Listing](#ocr-mcp-channel-proxy-model-discovery--openrouter-listing)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `POST` | [`/api/paas/v4/layout_parsing`](#ocr-mcp-channel-proxy-model-discovery--openrouter-listing) | API key | Zhipu-compatible document OCR / layout parsing; per-call billing; upstream body forwarded verbatim |
| `ANY` | [`/mcp`](#ocr-mcp-channel-proxy-model-discovery--openrouter-listing) | API key | MCP Streamable HTTP transport: POST=JSON-RPC 2.0 (initialize/tools.list/tools.call/ping/notifications); GET… |
| `ANY` | [`/v1/oneapi/proxy/:channelid/*target`](#ocr-mcp-channel-proxy-model-discovery--openrouter-listing) | API key | Raw passthrough pinning one channel; strips proxy prefix, swaps Authorization for channel credential, mirro… |
| `GET` | [`/v1/models`](#ocr-mcp-channel-proxy-model-discovery--openrouter-listing) | API key | OpenAI-compatible list of models accessible to the key's user group (hidden-model filtered, sorted by id) |
| `GET` | [`/v1/models/:model`](#ocr-mcp-channel-proxy-model-discovery--openrouter-listing) | API key | OpenAI-compatible single-model descriptor; case-insensitive; 200+model_not_found error when unknown/forbidden |
| `GET` | [`/openrouter/v1/models`](#ocr-mcp-channel-proxy-model-discovery--openrouter-listing) | Public | Public OpenRouter provider model catalog, deduped by lowercased id and sorted by lowercased name |

**[Usage, Billing Dashboard & API-Key Introspection](#usage-billing-dashboard--api-key-introspection)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | [`/dashboard/billing/subscription`](#usage-billing-dashboard--api-key-introspection) | API key | OpenAI-style billing subscription view of the calling key's quota ceiling (shares GetSubscription). |
| `GET` | [`/v1/dashboard/billing/subscription`](#usage-billing-dashboard--api-key-introspection) | API key | Same as /dashboard/billing/subscription, /v1 alias. |
| `GET` | [`/dashboard/billing/usage`](#usage-billing-dashboard--api-key-introspection) | API key | OpenAI-style usage view: consumed quota in cents (shares GetUsage). |
| `GET` | [`/v1/dashboard/billing/usage`](#usage-billing-dashboard--api-key-introspection) | API key | Same as /dashboard/billing/usage, /v1 alias. |
| `POST` | [`/api/token/consume`](#usage-billing-dashboard--api-key-introspection) | API key | Record an external/tool billing event (pre/post/cancel/single phases) deducting quota from the calling key. |
| `GET` | [`/api/token/balance`](#usage-billing-dashboard--api-key-introspection) | API key | Calling key's remain_quota/used_quota/unlimited_quota. |
| `GET` | [`/api/token/transactions`](#usage-billing-dashboard--api-key-introspection) | API key | Paginated external billing transaction history for the calling key (capped at retained-history limit). |
| `GET` | [`/api/token/logs`](#usage-billing-dashboard--api-key-introspection) | API key | Paginated consumption/billing logs scoped to the calling key's token_name. |
| `GET` | [`/api/available_models`](#usage-billing-dashboard--api-key-introspection) | API key | Models the calling key may invoke (key restrictions intersected with group-visible models). |
| `GET` | [`/api/user/get-by-token`](#usage-billing-dashboard--api-key-introspection) | API key | Owning user + calling key metadata, with flattened top-level convenience fields. |

**[Authentication & Account Lifecycle](#authentication--account-lifecycle)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `POST` | [`/api/user/register`](#authentication--account-lifecycle) | Public | Create a local account (username+password, optional email verification code); common-user role. |
| `POST` | [`/api/user/login`](#authentication--account-lifecycle) | Public | Password (+optional TOTP) login; issues the session cookie and returns the sanitized user object. |
| `GET` | [`/api/user/logout`](#authentication--account-lifecycle) | Public | Clear the current browser session (server store + cookie); missing session is a no-op success. |
| `POST` | [`/api/user/passkey/login/begin`](#authentication--account-lifecycle) | Public | Begin a discoverable WebAuthn passkey login; returns PublicKeyCredentialRequestOptions, stores ceremony in… |
| `POST` | [`/api/user/passkey/login/finish`](#authentication--account-lifecycle) | Public | Finish passkey login by verifying the assertion; resolves user from userHandle and issues the session cookie. |
| `GET` | [`/api/oauth/github`](#authentication--account-lifecycle) | Public | GitHub OAuth callback; logs in or provisions (or binds if session has username); requires oauth_state. |
| `GET` | [`/api/oauth/oidc`](#authentication--account-lifecycle) | Public | Generic OIDC callback; username from preferred_username else oidc_<n>; login/provision/bind; requires oauth… |
| `GET` | [`/api/oauth/lark`](#authentication--account-lifecycle) | Public | Lark/Feishu OAuth callback; login/provision/bind; requires oauth_state; no feature-disabled guard. |
| `GET` | [`/api/oauth/wechat`](#authentication--account-lifecycle) | Public | WeChat sign-in callback; resolves WeChat id from code; login/provision; does NOT validate oauth_state. |
| `GET` | [`/api/oauth/state`](#authentication--account-lifecycle) | Public | Generate and store a 12-char anti-CSRF state in the session and return it for OAuth redirects. |
| `GET` | [`/api/oauth/wechat/bind`](#authentication--account-lifecycle) | Access token / session | Bind a WeChat identity to the authenticated account; success returns empty message. |
| `GET` | [`/api/oauth/email/bind`](#authentication--account-lifecycle) | Access token / session | Bind/change account email gated by verification code; root also updates system root email. |
| `GET` | [`/api/verification`](#authentication--account-lifecycle) | Public | Issue an email verification code; uniform success after ~1s delay, async whitelist/occupancy check and send. |
| `GET` | [`/api/reset_password`](#authentication--account-lifecycle) | Public | Send a password-reset link if email registered; uniform success, async registration check and send. |
| `POST` | [`/api/user/reset`](#authentication--account-lifecycle) | Public | Complete password reset by validating the emailed token; returns effective password; token single-use. |

**[Self-Service Account, Access Token, 2FA, Passkeys, Logs, Trace & Cost](#self-service-account-access-token-2fa-passkeys-logs-trace--cost)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | [`/api/user/self`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Read the caller's own profile (password/access_token stripped; totp_secret present when set). |
| `PUT` | [`/api/user/self`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Update own username/display_name/password; partial payloads supported. |
| `DELETE` | [`/api/user/self`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Delete own account (root cannot self-delete). |
| `GET` | [`/api/user/dashboard`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Per-day usage breakdowns plus quota/status; root may target a user or all. |
| `GET` | [`/api/user/dashboard/users`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Root | User-selector list for the dashboard (root only). |
| `GET` | [`/api/user/aff`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Get/lazily generate the 4-char affiliate code. |
| `POST` | [`/api/user/topup`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Redeem a redemption code; returns credited quota. |
| `GET` | [`/api/user/available_models`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Sorted list of model names the user may call. |
| `GET` | [`/api/user/token`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Mint/rotate the 32-char management access token. |
| `GET` | [`/api/user/totp/status`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Report whether TOTP 2FA is enabled. |
| `GET` | [`/api/user/totp/setup`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Begin TOTP enrollment; returns secret + otpauth URI (temp secret in session). |
| `POST` | [`/api/user/totp/confirm`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Confirm TOTP code and activate 2FA; rate-limited. |
| `POST` | [`/api/user/totp/disable`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Disable TOTP after verifying a current code; rate-limited. |
| `GET` | [`/api/user/passkey`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | List the user's WebAuthn passkey credentials. |
| `POST` | [`/api/user/passkey/register/begin`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Begin passkey registration; returns creation options (challenge in session). |
| `POST` | [`/api/user/passkey/register/finish`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Finish passkey registration; optional ?name label; persists credential. |
| `DELETE` | [`/api/user/passkey/:id`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Delete one of the caller's passkeys (scoped to owner). |
| `PUT` | [`/api/user/passkey/:id`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Rename a passkey (1-128 chars; ownership verified). |
| `GET` | [`/api/log/self`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Paginated own request logs with filters/sorting (30-day cap when sorting+range). |
| `GET` | [`/api/log/self/search`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Keyword search over own logs, paginated. |
| `GET` | [`/api/log/self/stat`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Summed quota across own logs matching filters (username from session). |
| `GET` | [`/api/trace/log/:log_id`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Trace for a log id with computed durations and log summary. |
| `GET` | [`/api/trace/:trace_id`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Access token / session | Trace by trace id (timestamps only, no durations/log). |
| `GET` | [`/api/cost/request/:request_id`](#self-service-account-access-token-2fa-passkeys-logs-trace--cost) | Public | Raw cost object for a request id; no auth/ownership check, not enveloped. |

**[API Key (Token) Management](#api-key-token-management)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | [`/api/token/`](#api-key-token-management) | Access token / session | List the caller's tokens, paginated and sortable; envelope adds top-level total. |
| `GET` | [`/api/token/search`](#api-key-token-management) | Access token / session | Search the caller's tokens by name prefix (name LIKE 'keyword%'); returns data array plus total. |
| `GET` | [`/api/token/:id`](#api-key-token-management) | Access token / session | Get one token owned by the caller; other users' tokens treated as not found. |
| `POST` | [`/api/token/`](#api-key-token-management) | Access token / session | Mint a new server-generated 48-char relay API key; client supplies only metadata (name required). |
| `PUT` | [`/api/token/`](#api-key-token-management) | Access token / session | Update an editable token by body id; ?status_only= switches to status-only update. |
| `DELETE` | [`/api/token/:id`](#api-key-token-management) | Access token / session | Permanently delete a token owned by the caller; data omitted on success. |

**[User Administration & Top-up](#user-administration--top-up)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | [`/api/user/`](#user-administration--top-up) | Admin | List non-deleted users with pagination and optional sorting; returns data array plus total. |
| `GET` | [`/api/user/search`](#user-administration--top-up) | Admin | Search users by keyword (id exact / username,email,display_name prefix); no pagination. |
| `GET` | [`/api/user/:id`](#user-administration--top-up) | Admin | Fetch a single user by id (password and access_token omitted); role gate vs target. |
| `POST` | [`/api/user/`](#user-administration--top-up) | Admin | Create a user; role field only rejects role>=caller, quota/group applied as post-create overrides. |
| `POST` | [`/api/user/manage`](#user-administration--top-up) | Admin | Lifecycle action by username: enable/disable/promote/demote/delete; echoes resulting role+status. |
| `PUT` | [`/api/user/`](#user-administration--top-up) | Admin | Partial user update; absent/null fields untouched (mcp_tool_blacklist null clears); records quota log. |
| `DELETE` | [`/api/user/:id`](#user-administration--top-up) | Admin | Soft-delete a user by id; requires caller role strictly > target (no root self-exemption). |
| `POST` | [`/api/user/totp/disable/:id`](#user-administration--top-up) | Admin | Clear target user's TOTP/2FA secret; logged to management log. |
| `POST` | [`/api/topup`](#user-administration--top-up) | Admin | Admin credit a user's quota and record a top-up log entry; distinct from self /api/user/topup. |

**[Channel Administration & Diagnostics](#channel-administration--diagnostics)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | [`/api/channel/`](#channel-administration--diagnostics) | Admin | List channels with pagination/sort; returns array + total count. |
| `GET` | [`/api/channel/search`](#channel-administration--diagnostics) | Admin | Keyword search across channels; returns all matches, no pagination. |
| `GET` | [`/api/channel/models`](#channel-administration--diagnostics) | Admin | Admin catalog of all known models in OpenAI list shape (NOT management envelope). |
| `GET` | [`/api/channel/metadata`](#channel-administration--diagnostics) | Admin | Type metadata: default base URL, editability, default/all endpoints. |
| `GET` | [`/api/channel/:id`](#channel-administration--diagnostics) | Admin | Get one channel by ID (secrets masked, optional tooling string). |
| `GET` | [`/api/channel/test`](#channel-administration--diagnostics) | Admin | Start async background test sweep across channels; one at a time. |
| `GET` | [`/api/channel/test/:id`](#channel-administration--diagnostics) | Admin | Synchronously probe one channel; flat {success,message,time,modelName} (no data envelope). |
| `GET` | [`/api/channel/update_balance`](#channel-administration--diagnostics) | Admin | All-channel balance refresh trigger; inline body disabled, returns success immediately. |
| `GET` | [`/api/channel/update_balance/:id`](#channel-administration--diagnostics) | Admin | Query upstream billing for one channel; flat balance field (USD), supported types only. |
| `GET` | [`/api/channel/pricing/:id`](#channel-administration--diagnostics) | Admin | Effective pricing: derived ratios, unified model_configs, tooling. |
| `GET` | [`/api/channel/default-pricing`](#channel-administration--diagnostics) | Admin | Adapter default pricing for a type; fields are JSON-encoded strings. |
| `POST` | [`/api/channel/`](#channel-administration--diagnostics) | Admin | Create one or more channels (newline-split key = bulk create). |
| `POST` | [`/api/channel/:id/duplicate`](#channel-administration--diagnostics) | Admin | Clone a channel; resets identity/usage, appends ' Copy'. |
| `PUT` | [`/api/channel/`](#channel-administration--diagnostics) | Admin | Update channel; full update or status_only mode. |
| `PUT` | [`/api/channel/pricing/:id`](#channel-administration--diagnostics) | Admin | Replace channel pricing (unified model_configs preferred; legacy ratios accepted). |
| `DELETE` | [`/api/channel/disabled`](#channel-administration--diagnostics) | Admin | Delete all disabled channels; returns deleted row count. |
| `DELETE` | [`/api/channel/:id`](#channel-administration--diagnostics) | Admin | Delete one channel by ID. |
| `POST` | [`/api/debug/channel/:id/debug`](#channel-administration--diagnostics) | Admin | Log model-config diagnostics for one channel (4xx/5xx on error). |
| `GET` | [`/api/debug/channels`](#channel-administration--diagnostics) | Admin | Log model-config summary across all channels. |
| `POST` | [`/api/debug/channel/:id/fix`](#channel-administration--diagnostics) | Admin | Repair one channel's model config; logged. |
| `GET` | [`/api/debug/channels/validate`](#channel-administration--diagnostics) | Admin | Validate model config of every channel; logged. |
| `POST` | [`/api/debug/channels/remigrate`](#channel-administration--diagnostics) | Admin | Re-run model-config migration for all channels. |
| `GET` | [`/api/debug/channel/:id/migration-status`](#channel-administration--diagnostics) | Admin | Structured migration state for one channel (returns data, no message). |
| `POST` | [`/api/debug/channels/clean`](#channel-administration--diagnostics) | Admin | Clean channels with mixed/legacy model data. |

**[Redemptions, Groups, Logs, Admin Token Visibility & Model Catalog](#redemptions-groups-logs-admin-token-visibility--model-catalog)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | [`/api/redemption/`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | List redemption codes with offset pagination (id desc); data is array, plus top-level total (full table cou… |
| `GET` | [`/api/redemption/search`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | Keyword search redemptions by exact id or name prefix, paginated and optionally sorted; total is filtered c… |
| `GET` | [`/api/redemption/:id`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | Fetch one redemption by numeric id; data is a single redemption object. |
| `POST` | [`/api/redemption/`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | Create a batch of 1-100 redemption codes (name<=20 bytes); data is array of generated 32-char code strings. |
| `PUT` | [`/api/redemption/`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | Update name+quota by default, or status only when status_only query is present; data is updated object. |
| `DELETE` | [`/api/redemption/:id`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | Permanently delete a redemption by id; no data payload. id=0/non-numeric returns 'id is empty!'. |
| `GET` | [`/api/group/`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | List configured billing group names; data is an unordered array of strings. |
| `GET` | [`/api/log/`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | List usage/audit logs across all users with filters, pagination, sort (sort/sort_by, order/sort_order, size… |
| `DELETE` | [`/api/log/`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | Purge logs older than required non-zero target_timestamp; data is deleted row count. |
| `GET` | [`/api/log/stat`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | Return summed quota over filtered logs; data is {quota}. |
| `GET` | [`/api/log/search`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | Keyword search across all users' logs, paginated and optionally sorted; total is matched count. |
| `GET` | [`/api/admin/tokens/`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | List relay tokens across all users read-only, optional user_id filter; key field is raw 48-char (not sk- pr… |
| `GET` | [`/api/admin/tokens/search`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | Keyword search tokens across users by name prefix, read-only, paginated/sorted. |
| `GET` | [`/api/admin/tokens/:id`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | Fetch any token by id regardless of owner, read-only; non-numeric id returns 'invalid token id: ...'. |
| `GET` | [`/api/models`](#redemptions-groups-logs-admin-token-visibility--model-catalog) | Admin | Return full channel-type-ID to model-name-list catalog; data is an object keyed by channel type ID. |

**[MCP Server & Tool Administration](#mcp-server--tool-administration)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | [`/api/mcp_servers/ (and /api/mcp_servers)`](#mcp-server--tool-administration) | Admin | List MCP servers with pagination/sorting; each entry pairs the masked server with its tool count, plus total. |
| `GET` | [`/api/mcp_servers/:id`](#mcp-server--tool-administration) | Admin | Get full config for one MCP server (api_key masked). |
| `POST` | [`/api/mcp_servers/ (and /api/mcp_servers)`](#mcp-server--tool-administration) | Admin | Create a new MCP server (validated, api_key encrypted); does not fetch tools. |
| `PUT` | [`/api/mcp_servers/:id`](#mcp-server--tool-administration) | Admin | Update an MCP server; only body-present keys are written (incl. zero/empty clears); ****** keeps api_key. |
| `DELETE` | [`/api/mcp_servers/:id`](#mcp-server--tool-administration) | Admin | Permanently delete an MCP server; no data field returned. |
| `POST` | [`/api/mcp_servers/:id/sync`](#mcp-server--tool-administration) | Admin | Manually sync and persist the upstream tool catalogue; returns tool_count, updates last_sync_* fields. |
| `POST` | [`/api/mcp_servers/:id/test`](#mcp-server--tool-administration) | Admin | 15s connectivity test that lists upstream tools without persisting; returns tool_count and protocol, update… |
| `GET` | [`/api/mcp_servers/:id/tools`](#mcp-server--tool-administration) | Admin | List stored tools for one server (non-paginated) with pricing overrides applied and null schemas normalized. |
| `GET` | [`/api/mcp_tools/ (and /api/mcp_tools)`](#mcp-server--tool-administration) | Admin | List synchronized tools across all servers with pagination/sorting and optional server_id/status filters; r… |

**[System Options (Root) & Public Endpoints](#system-options-root--public-endpoints)**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | [`/api/option/`](#system-options-root--public-endpoints) | Root | List all stored system config options, omitting sensitive keys (Token/Secret/Password). |
| `PUT` | [`/api/option/`](#system-options-root--public-endpoints) | Root | Upsert a single config option by key, with feature-toggle prerequisite validation and sensitive-key empty-v… |
| `GET` | [`/api/status`](#system-options-root--public-endpoints) | Public | Return app metadata and public feature/auth toggles for the web UI. |
| `GET` | [`/api/status/channel`](#system-options-root--public-endpoints) | Public | Return a paginated, sanitized view of channel health and last connectivity-test metrics. |
| `GET` | [`/api/models/display`](#system-options-root--public-endpoints) | Public (optional) | Return the model catalog grouped by channel with pricing/capability metadata; anonymous sees all, authentic… |
| `GET` | [`/api/tools/display`](#system-options-root--public-endpoints) | Public | Return enabled MCP servers and their enabled tools with normalized schemas and per-call pricing. |
| `GET` | [`/api/notice`](#system-options-root--public-endpoints) | Public | Return the stored Notice option string. |
| `GET` | [`/api/about`](#system-options-root--public-endpoints) | Public | Return the stored About option string. |
| `GET` | [`/api/home_page_content`](#system-options-root--public-endpoints) | Public | Return the stored HomePageContent option string. |



## Chat Completions, Text Completions & Responses API

These are the core inference (relay) endpoints. They accept OpenAI-compatible payloads, select the upstream channel by the `model` field, meter usage against your key's quota, and forward the upstream's response (with gateway-specific normalization). Inbound formats are auto-detected and, if a request is sent to the "wrong" path (e.g. a Response API body posted to `/v1/chat/completions`), transparently re-routed to the correct handler. All endpoints in this section authenticate with a Relay API Key via `TokenAuth` and return OpenAI-style error envelopes (`{"error": {...}}`) with the real HTTP status. Every response carries an `X-Oneapi-Request-Id` header you can use to correlate logs and billing; that same request id is also appended to client-facing error messages.

> Auth (all endpoints in this section): Relay API Key. Send as `Authorization: Bearer $API_KEY`. The gateway also accepts `X-Api-Key: $API_KEY` and `Api-Key: $API_KEY` (the `Bearer` scheme is matched case-insensitively and is optional on the latter two). A 401 means the key is missing/invalid; a 403 means the key is valid but not permitted (banned, model not allowed for the key, subnet restriction, or insufficient quota).

> Schema note: request bodies and success responses follow the upstream OpenAI schema. The tables below document the gateway-relevant fields with field names taken verbatim from the gateway request structs, plus one representative example each, rather than re-listing the entire upstream schema. Any additional OpenAI-recognized field is passed through to the upstream.

> Streaming note: when `stream` is `true` the response is `text/event-stream`. Each SSE event is a line beginning with `data: ` followed by one JSON chunk; the stream ends with a literal `data: [DONE]` sentinel. For Chat/Text Completions, send `"stream_options": {"include_usage": true}` to force a usage object on the final pre-`[DONE]` chunk.

### POST /v1/chat/completions

Generates a chat completion from a list of messages. This is the primary inference endpoint; use it for conversational and tool-calling workloads.

**Auth:** Relay API Key - `Authorization: Bearer $API_KEY`.

**Request body** (OpenAI Chat Completions schema; gateway-relevant fields):

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Model | `model` | string | Yes | - | Model id; selects the upstream channel. |
| Messages | `messages` | array | Yes | - | Conversation messages (must be non-empty). |
| Stream | `stream` | bool | No | `false` | If `true`, responds with SSE chunks ending in `[DONE]`. |
| Stream options | `stream_options` | object | No | - | `{"include_usage": true}` emits a usage object on the final chunk. |
| Max tokens | `max_tokens` | int | No | - | Max output tokens (validated `0 <= n <= MaxInt32/2`; most open models still use this field). |
| Max completion tokens | `max_completion_tokens` | int | No | - | Preferred output-token cap; validated and preferred over `max_tokens`. |
| Temperature | `temperature` | number | No | - | Sampling temperature. |
| Top P | `top_p` | number | No | - | Nucleus sampling. |
| Tools | `tools` | array | No | - | Tool/function definitions the model may call. |
| Tool choice | `tool_choice` | string/object | No | - | How tools are selected. |
| Response format | `response_format` | object | No | - | e.g. `{"type": "json_object"}` or a `json_schema`. |
| Reasoning effort | `reasoning_effort` | string | No | - | One of `low`/`medium`/`high`/`minimal` (reasoning models). |
| User | `user` | string | No | - | Stable end-user identifier. |

```json
{
  "model": "gpt-4o-mini",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Say hello in one short sentence."}
  ],
  "temperature": 0.7,
  "max_tokens": 64,
  "stream": false
}
```

**Response:** `200 OK`. The body is the upstream OpenAI chat completion object.

Non-streaming (`stream: false`):

```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1733650000,
  "model": "gpt-4o-mini",
  "choices": [
    {
      "index": 0,
      "message": {"role": "assistant", "content": "Hello there, nice to meet you!"},
      "finish_reason": "stop"
    }
  ],
  "usage": {"prompt_tokens": 19, "completion_tokens": 8, "total_tokens": 27}
}
```

Streaming (`stream: true`): the response is `Content-Type: text/event-stream`. Each event is one delta chunk; the final data line is `[DONE]`. With `stream_options.include_usage = true`, the chunk immediately before `[DONE]` carries a populated `usage` field (and an empty `choices` array):

```text
data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1733650000,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1733650000,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":" there"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1733650000,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1733650000,"model":"gpt-4o-mini","choices":[],"usage":{"prompt_tokens":19,"completion_tokens":8,"total_tokens":27}}

data: [DONE]
```

**Example:**

```bash
curl -sS $BASE_URL/v1/chat/completions \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [
      {"role": "user", "content": "Say hello in one short sentence."}
    ],
    "stream": false
  }'
```

**Errors:**

| Status | Meaning |
|--------|---------|
| 400 | `invalid_text_request` - body failed validation (empty `messages`, missing `model`, invalid `max_tokens`/`max_completion_tokens`) or contained an unknown parameter. |
| 401 | Missing or invalid API key. |
| 403 | `insufficient_user_quota` / `insufficient_token_quota` - quota exhausted (may surface mid-stream when streaming billing detects the cap); also `tool_not_allowed` for a built-in tool the key/model may not use. |
| 413 | Request body too large; the gateway retries on other channels before returning this. |
| 429 | All eligible channels for the model are rate limited; returned only after retries are exhausted. |

### POST /v1/completions

Legacy text completion from a single `prompt`. Use only for older models that do not support chat messages; prefer `/v1/chat/completions` otherwise.

**Auth:** Relay API Key - `Authorization: Bearer $API_KEY`.

**Request body** (OpenAI Completions schema; gateway-relevant fields):

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Model | `model` | string | Yes | - | Model id; selects the upstream channel. |
| Prompt | `prompt` | string | Yes | - | The text to complete (must be non-empty). |
| Stream | `stream` | bool | No | `false` | If `true`, responds with SSE chunks ending in `[DONE]`. |
| Stream options | `stream_options` | object | No | - | `{"include_usage": true}` emits a usage object on the final chunk. |
| Max tokens | `max_tokens` | int | No | - | Max output tokens. |
| Temperature | `temperature` | number | No | - | Sampling temperature. |

```json
{
  "model": "gpt-3.5-turbo-instruct",
  "prompt": "Write a one-line greeting:",
  "max_tokens": 32,
  "temperature": 0.7,
  "stream": false
}
```

**Response:** `200 OK`. Upstream OpenAI completion object.

```json
{
  "id": "cmpl-xyz789",
  "object": "text_completion",
  "created": 1733650100,
  "model": "gpt-3.5-turbo-instruct",
  "choices": [
    {"text": "Hello and welcome!", "index": 0, "finish_reason": "stop", "logprobs": null}
  ],
  "usage": {"prompt_tokens": 6, "completion_tokens": 5, "total_tokens": 11}
}
```

Streaming behaves as described in the section preamble: `text_completion` chunks followed by `data: [DONE]`.

**Example:**

```bash
curl -sS $BASE_URL/v1/completions \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-3.5-turbo-instruct",
    "prompt": "Write a one-line greeting:",
    "max_tokens": 32
  }'
```

**Errors:**

| Status | Meaning |
|--------|---------|
| 400 | `invalid_text_request` - `prompt` is empty, `model` is missing, `max_tokens` is invalid, or an unknown parameter was sent. |
| 401 | Missing or invalid API key. |
| 403 | `insufficient_user_quota` / `insufficient_token_quota` - quota exhausted. |

### POST /v1/edits

Legacy edits endpoint. Applies an `instruction` to optional `input` text. Routed through the same text relay handler (Edits mode); retained for OpenAI compatibility - most modern models should use `/v1/chat/completions` instead.

**Auth:** Relay API Key - `Authorization: Bearer $API_KEY`.

**Request body** (OpenAI Edits schema; gateway-relevant fields):

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Model | `model` | string | Yes | - | Model id; selects the upstream channel. |
| Instruction | `instruction` | string | Yes | - | The edit instruction (must be non-empty). |
| Input | `input` | string | No | `""` | The text to be edited. |
| Temperature | `temperature` | number | No | - | Sampling temperature. |

```json
{
  "model": "gpt-4o-mini",
  "input": "What day of the wek is it?",
  "instruction": "Fix the spelling mistakes."
}
```

**Response:** `200 OK`. Upstream edits object.

```json
{
  "object": "edit",
  "created": 1733650200,
  "choices": [
    {"text": "What day of the week is it?", "index": 0}
  ],
  "usage": {"prompt_tokens": 25, "completion_tokens": 32, "total_tokens": 57}
}
```

**Example:**

```bash
curl -sS $BASE_URL/v1/edits \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "input": "What day of the wek is it?",
    "instruction": "Fix the spelling mistakes."
  }'
```

**Errors:**

| Status | Meaning |
|--------|---------|
| 400 | `invalid_text_request` - `instruction` is empty, `model` is missing, or an unknown parameter was sent. |
| 401 | Missing or invalid API key. |
| 403 | `insufficient_user_quota` / `insufficient_token_quota` - quota exhausted. |

### POST /v1/responses

Creates a model response using the OpenAI Response API. Use it for the Responses workflow (server-side conversation chaining via `previous_response_id`, built-in tools, background runs). For channels with native Response API support the body is passed through; channels without it (and requests using MCP built-in tools) are transparently served through a Chat Completions fallback.

**Auth:** Relay API Key - `Authorization: Bearer $API_KEY`.

**Request body** (OpenAI Response API schema; gateway-relevant fields):

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Model | `model` | string | Yes | - | Model id; selects the upstream channel. |
| Input | `input` | string/array | Conditional | - | Text/image/file input. Provide exactly one of `input` or `prompt`. |
| Prompt | `prompt` | object | Conditional | - | Prompt-template config. Mutually exclusive with `input`. |
| Instructions | `instructions` | string | No | - | System message inserted first in the model's context. |
| Stream | `stream` | bool | No | `false` | If `true`, streams SSE response events ending in `[DONE]`. |
| Max output tokens | `max_output_tokens` | int | No | - | Upper bound on generated tokens. |
| Background | `background` | bool | No | `false` | Run the response asynchronously in the background. |
| Previous response id | `previous_response_id` | string | No | - | Chain onto a prior stored response. |
| Store | `store` | bool | No | - | Whether to persist the generated response. |
| Tools | `tools` | array | No | - | Tools (including built-ins) the model may call. |
| Tool choice | `tool_choice` | string/object | No | - | How tools are selected. |
| Reasoning | `reasoning` | object | No | - | Reasoning-model config (`effort`, `summary`). |
| Temperature | `temperature` | number | No | - | Sampling temperature. |
| Include | `include` | array | No | - | Additional output data to include. |

```json
{
  "model": "gpt-4o-mini",
  "input": "Write a haiku about the sea.",
  "max_output_tokens": 128,
  "stream": false
}
```

**Response:** `200 OK`. Upstream Response API `response` object.

```json
{
  "id": "resp_abc123",
  "object": "response",
  "created_at": 1733650300,
  "status": "completed",
  "model": "gpt-4o-mini",
  "output": [
    {
      "id": "msg_001",
      "type": "message",
      "role": "assistant",
      "content": [
        {"type": "output_text", "text": "Endless rolling waves\nwhispering to the gray shore\nsalt upon the wind", "annotations": []}
      ]
    }
  ],
  "usage": {"input_tokens": 12, "output_tokens": 19, "total_tokens": 31}
}
```

When `stream: true`, the response is `text/event-stream`. Each event is a typed Response API event (e.g. `response.created`, `response.output_text.delta`, `response.completed`) delivered as a `data:` line; the stream terminates with `data: [DONE]`.

**Example:**

```bash
curl -sS $BASE_URL/v1/responses \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "input": "Write a haiku about the sea.",
    "max_output_tokens": 128
  }'
```

**Errors:**

| Status | Meaning |
|--------|---------|
| 400 | `invalid_response_api_request` - missing `model`, or neither/both of `input` and `prompt` provided. |
| 400 | `tool_not_allowed` - a requested built-in tool is not permitted for this model/channel. |
| 401 | Missing or invalid API key. |
| 403 | `insufficient_user_quota` / `insufficient_token_quota` - quota exhausted. |

### GET /v1/responses

Same handler and request semantics as `POST /v1/responses` (both route to `controller.Relay` in Response API mode, i.e. `RelayResponseAPIHelper`). The gateway reads and validates the same JSON body, and streaming is still controlled by the `stream` field in that body, not by a query parameter. This GET form exists only so that upstream clients which issue the create/stream call as a GET with a body are accepted.

**Auth:** Relay API Key - `Authorization: Bearer $API_KEY`.

**Request body:** identical to `POST /v1/responses` (`model` plus exactly one of `input`/`prompt`; `stream` toggles SSE).

**Response:** `200 OK`. Same `response` object shape as `POST /v1/responses`.

**Example:**

```bash
curl -sS -X GET $BASE_URL/v1/responses \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "input": "Write a haiku about the sea.",
    "stream": false
  }'
```

**Errors:**

| Status | Meaning |
|--------|---------|
| 400 | `invalid_response_api_request` - missing `model`, or neither/both of `input` and `prompt` provided. |
| 401 | Missing or invalid API key. |
| 403 | `insufficient_user_quota` / `insufficient_token_quota` - quota exhausted. |

### GET /v1/responses/:response_id

Retrieves a previously created Response API response by id (handler `controller.RelayResponseGet`). Pass-through to the OpenAI Responses retrieve endpoint. Supported only when the resolved channel is an OpenAI channel.

**Auth:** Relay API Key - `Authorization: Bearer $API_KEY`.

**Path parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `response_id` | string | Yes | The id of the response to retrieve (e.g. `resp_abc123`). |

**Query parameters:**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `stream` | bool | No | `false` | When `true` (parsed via `strconv.ParseBool`), stream the response events as SSE. |

**Response:** `200 OK`. The upstream `response` object (same shape as the create response).

```json
{
  "id": "resp_abc123",
  "object": "response",
  "created_at": 1733650300,
  "status": "completed",
  "model": "gpt-4o-mini",
  "output": [
    {
      "id": "msg_001",
      "type": "message",
      "role": "assistant",
      "content": [
        {"type": "output_text", "text": "Endless rolling waves", "annotations": []}
      ]
    }
  ],
  "usage": {"input_tokens": 12, "output_tokens": 19, "total_tokens": 31}
}
```

**Example:**

```bash
curl -sS $BASE_URL/v1/responses/resp_abc123 \
  -H "Authorization: Bearer $API_KEY"
```

**Errors:**

| Status | Meaning |
|--------|---------|
| 400 | `unsupported_channel` - "Response API is only supported for OpenAI channels"; the resolved channel is not an OpenAI channel. |
| 400 | `invalid_query_parameter` - the `stream` query value could not be parsed as a boolean. |
| 401 | Missing or invalid API key. |
| 404 | The response id does not exist upstream (propagated from OpenAI). |

### DELETE /v1/responses/:response_id

Deletes a stored Response API response (handler `controller.RelayResponseDelete`). Pass-through to the OpenAI Responses delete endpoint; the upstream body, status, and headers are forwarded verbatim. Supported only for OpenAI channels.

**Auth:** Relay API Key - `Authorization: Bearer $API_KEY`.

**Path parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `response_id` | string | Yes | The id of the response to delete. |

**Response:** `200 OK`. The upstream deletion confirmation object.

```json
{
  "id": "resp_abc123",
  "object": "response.deleted",
  "deleted": true
}
```

**Example:**

```bash
curl -sS -X DELETE $BASE_URL/v1/responses/resp_abc123 \
  -H "Authorization: Bearer $API_KEY"
```

**Errors:**

| Status | Meaning |
|--------|---------|
| 400 | `unsupported_channel` - only OpenAI channels support this action. |
| 401 | Missing or invalid API key. |
| 404 | The response id does not exist upstream. |

### POST /v1/responses/:response_id/cancel

Cancels an in-progress background Response API response (handler `controller.RelayResponseCancel`). Pass-through to the OpenAI Responses cancel endpoint. Applicable only to responses created with `background: true`, and only for OpenAI channels.

**Auth:** Relay API Key - `Authorization: Bearer $API_KEY`.

**Path parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `response_id` | string | Yes | The id of the background response to cancel. |

**Response:** `200 OK`. The upstream `response` object with its status set to `cancelled`.

```json
{
  "id": "resp_abc123",
  "object": "response",
  "created_at": 1733650300,
  "status": "cancelled",
  "model": "gpt-4o-mini",
  "output": [],
  "usage": null
}
```

**Example:**

```bash
curl -sS -X POST $BASE_URL/v1/responses/resp_abc123/cancel \
  -H "Authorization: Bearer $API_KEY"
```

**Errors:**

| Status | Meaning |
|--------|---------|
| 400 | `unsupported_channel` - only OpenAI channels support this action. |
| 401 | Missing or invalid API key. |
| 404 | The response id does not exist upstream. |


## Claude Messages & Moderations

This section covers the two relay endpoints that accept non–Chat-Completion request schemas: the Anthropic-native Claude Messages endpoint (`/v1/messages`) and the OpenAI-compatible content moderation endpoint (`/v1/moderations`). Both are inference/relay endpoints dispatched through `controller.Relay` and protected by `TokenAuth`, so they require a Relay API KEY (`$API_KEY`). Their request and response bodies follow the upstream provider schemas (Anthropic for Claude Messages, OpenAI for Moderations); this section documents the gateway-relevant fields, the prefix-rewrite aliases used by Claude Code clients, and one complete example each. Errors use the OpenAI-style relay error envelope `{"error": {"message": "...", "type": "...", "param": "...", "code": "..."}}` returned with the real HTTP status code.

### POST /v1/messages

Relays a request in the Anthropic Messages format to a Claude-capable channel. The gateway auto-detects the inbound format, maps the requested model to the resolved channel/model, optionally sanitizes thinking blocks for pass-through, and either forwards the upstream Claude response verbatim (direct pass-through) or converts a non-Anthropic upstream response back into Claude Messages format. Requests in other formats sent to this path are transparently re-routed; conversely, the following aliases are rewritten to `/v1/messages` before auth runs (for Claude Code clients that prepend gateway prefixes):

- `POST /v1/v1/messages`
- `POST /openai/v1/messages`
- `POST /openai/v1/v1/messages`
- `POST /api/v1/v1/messages`

**Auth:** Relay API KEY via `Authorization: Bearer $API_KEY` (also accepted: `X-Api-Key: $API_KEY`, `Api-Key: $API_KEY`).

**Request body**: follows the Anthropic Messages schema. Gateway-recognized top-level fields:

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Model | `model` | string | Yes | — | Requested model name; remapped to the resolved channel model. |
| Max tokens | `max_tokens` | integer | Yes | — | Maximum tokens to generate; must be greater than 0. |
| Messages | `messages` | array | Yes | — | Conversation turns; each has `role` (`user` or `assistant`) and `content` (non-empty string or non-empty array of content blocks). |
| System | `system` | string or array | No | — | System prompt (string or array of text blocks). |
| Temperature | `temperature` | number | No | — | Sampling temperature. |
| Top P | `top_p` | number | No | — | Nucleus sampling. |
| Top K | `top_k` | integer | No | — | Top-k sampling. |
| Stream | `stream` | boolean | No | `false` | When `true`, responds with Anthropic SSE events. |
| Stop sequences | `stop_sequences` | array[string] | No | — | Sequences that halt generation. |
| Tools | `tools` | array | No | — | Tool definitions; each has `name` (required), optional `description`, `input_schema`, `type`, `defer_loading`. |
| Tool choice | `tool_choice` | object/string | No | — | Tool selection directive. |
| Thinking | `thinking` | object | No | — | Extended thinking config: `type` (string) and `budget_tokens` (integer, min 1024). |
| Metadata | `metadata` | object | No | — | Opaque request metadata. |
| Extra body | `extra_body` | object | No | — | Allowlisted provider-specific params merged into the upstream payload. |

```json
{
  "model": "claude-sonnet-4-5",
  "max_tokens": 1024,
  "system": "You are a concise assistant.",
  "messages": [
    {
      "role": "user",
      "content": "In one sentence, what is the capital of France?"
    }
  ],
  "stream": false
}
```

**Response**: HTTP 200. For direct pass-through, the upstream Anthropic JSON body and headers are forwarded verbatim. The non-streaming response follows the Anthropic Messages schema:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Message identifier. |
| `type` | string | Always `message`. |
| `role` | string | Always `assistant`. |
| `model` | string | Model that produced the response. |
| `content` | array | Content blocks; each has `type` (`text`, `tool_use`, `thinking`, …) and type-specific fields. |
| `stop_reason` | string | Why generation stopped (e.g. `end_turn`, `max_tokens`, `tool_use`). |
| `stop_sequence` | string or null | Matched stop sequence, if any. |
| `usage` | object | Token usage: `input_tokens`, `output_tokens`, optional `cache_read_input_tokens`, `cache_creation_input_tokens` / `cache_creation`. |

```json
{
  "id": "msg_01ABCdefGhIJKlmnOPqrstuv",
  "type": "message",
  "role": "assistant",
  "model": "claude-sonnet-4-5",
  "content": [
    {
      "type": "text",
      "text": "The capital of France is Paris."
    }
  ],
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "usage": {
    "input_tokens": 18,
    "output_tokens": 9
  }
}
```

When `stream` is `true`, the response is `Content-Type: text/event-stream` carrying Anthropic-style SSE events. Each line is `event: <type>` followed by `data: <json>`. The event sequence is: `message_start`, then for each content block `content_block_start` -> one or more `content_block_delta` -> `content_block_stop`, then `message_delta` (carries `stop_reason` and cumulative `usage`), and finally `message_stop`. Periodic `ping` events may be interleaved. A representative chunk:

```text
event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Paris"}}
```

Note: the Anthropic stream is terminated by the `message_stop` event; unlike the OpenAI relay endpoints, it does not emit a `[DONE]` sentinel.

**Example**

```bash
curl -X POST "$BASE_URL/v1/messages" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 1024,
    "system": "You are a concise assistant.",
    "messages": [
      {"role": "user", "content": "In one sentence, what is the capital of France?"}
    ],
    "stream": false
  }'
```

**Errors**

| Status | Meaning |
|--------|---------|
| 400 | `invalid_claude_messages_request` — body is not valid Claude Messages JSON, or a required field is invalid (`model` empty, `max_tokens` <= 0, `messages` empty, or a message has a bad `role`/empty `content`). |
| 401 | Missing or invalid API key. |
| 403 | Valid key but the requested model is not permitted for this token, or a quota/subnet restriction applies. |

### POST /v1/moderations

Relays an OpenAI-format content-moderation request to a moderation-capable channel to classify input text against policy categories. Use it to screen text before or after generation.

**Auth:** Relay API KEY via `Authorization: Bearer $API_KEY` (also accepted: `X-Api-Key: $API_KEY`, `Api-Key: $API_KEY`).

**Request body**: follows the OpenAI Moderations schema. Gateway-recognized fields:

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Input | `input` | string or array[string] | Yes | — | Text to classify. Must be non-empty (the gateway rejects an empty string). |
| Model | `model` | string | No | `text-moderation-latest` | Moderation model; defaults to `text-moderation-latest` when omitted. |

```json
{
  "model": "text-moderation-latest",
  "input": "I want to learn how to bake bread at home."
}
```

**Response**: HTTP 200. Body follows the OpenAI Moderations schema.

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Moderation request identifier. |
| `model` | string | Model that performed the classification. |
| `results` | array | One result per input; each has `flagged` (boolean), `categories` (map of category -> boolean), and `category_scores` (map of category -> number). |

```json
{
  "id": "modr-XXXXXXXXXXXXXXXXXXXXXXXX",
  "model": "text-moderation-007",
  "results": [
    {
      "flagged": false,
      "categories": {
        "sexual": false,
        "hate": false,
        "harassment": false,
        "self-harm": false,
        "violence": false
      },
      "category_scores": {
        "sexual": 0.0000012,
        "hate": 0.0000004,
        "harassment": 0.0000007,
        "self-harm": 0.0000002,
        "violence": 0.0000009
      }
    }
  ]
}
```

**Example**

```bash
curl -X POST "$BASE_URL/v1/moderations" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "text-moderation-latest",
    "input": "I want to learn how to bake bread at home."
  }'
```

**Errors**

| Status | Meaning |
|--------|---------|
| 400 | `field input is required` — `input` is missing or an empty string. |
| 401 | Missing or invalid API key. |
| 403 | Valid key but the requested model is not permitted for this token, or a quota/subnet restriction applies. |


## Realtime API (WebSocket)

These endpoints proxy the OpenAI Realtime API for live, low-latency audio/text conversations. `GET /v1/realtime` upgrades the connection to a WebSocket and bidirectionally pumps frames between the client and the upstream provider, parsing usage from `response.done` events for billing. `POST /v1/realtime/sessions` mints an ephemeral session token for browser WebRTC clients. Both routes require a Relay API KEY (validated by `TokenAuth`), and the requested model must be passed (as the `?model=` query parameter for the WebSocket route, or as the `model` body field for session creation) and must match the model bound to the channel. Because audio tokens cost significantly more than text tokens, realtime sessions pre-consume a conservative quota estimate (a 120-second session at audio token rates) before the WebSocket is upgraded; the charge is reconciled to actual usage when the session closes. If upstream never reports usage, the pre-consumed amount is kept as the final charge rather than refunded. Trusted users whose remaining quota greatly exceeds the estimate skip pre-consumption entirely and are billed only at reconciliation.

### GET /v1/realtime

Upgrades the HTTP connection to a WebSocket and proxies a full OpenAI Realtime session (audio/text frames in both directions) to the upstream provider. Use this for live, streaming voice or text conversations over a persistent socket.

**Auth:** Relay API KEY (`TokenAuth`). Send as `Authorization: Bearer $API_KEY`. Browsers cannot set custom headers on WebSocket connections, so the key may instead be supplied via the `Sec-WebSocket-Protocol` subprotocol value `openai-insecure-api-key.$API_KEY` (the `X-Api-Key` and `Api-Key` headers are also accepted for non-browser clients).

**Query parameters**

| Name  | Type   | Required | Default | Description |
|-------|--------|----------|---------|-------------|
| model | string | Yes      | —       | The realtime model to use (e.g. `gpt-4o-realtime-preview`). Required; the upgrade is rejected if missing. The gateway maps this to the channel-bound model before dialing upstream, so a client-supplied alias is replaced with the actual model name. |

**WebSocket handshake**

This is a WebSocket upgrade request, not a normal HTTP call. The client sends the standard upgrade headers (`Upgrade: websocket`, `Connection: Upgrade`, `Sec-WebSocket-Key`, `Sec-WebSocket-Version: 13`). The gateway recommends including `OpenAI-Beta: realtime=v1`; if omitted it is added automatically when dialing upstream. Any `Sec-WebSocket-Protocol` subprotocols sent by the client are mirrored back during negotiation, except the `openai-insecure-api-key.*` auth subprotocol, which is consumed for authentication and never echoed.

**Response**

On success the server responds with HTTP `101 Switching Protocols` and the socket stays open. After the upgrade, the gateway transparently relays Realtime protocol events in both directions (e.g. `session.update`, `input_audio_buffer.append`, `response.create`, `response.done`); the message shapes follow the upstream OpenAI Realtime schema. Client-to-upstream `session.update` frames that attempt to change the session `model` are rejected: the gateway sends back an `error` event (`{"type":"error","error":{"type":"invalid_request_error","code":"model_switch_denied","message":"..."}}`) and then closes the socket with WebSocket close code `1008` (policy violation) and reason `model_switch_denied`.

If the handshake or the upstream dial fails the gateway does not return a JSON body over the socket in the normal way:
- Upgrade failure: the WebSocket upgrade response carries HTTP `400`; the controller records `{"error": {"message": "websocket upgrade failed: ...", "type": "one_api_error", "code": "ws_upgrade_failed"}}`.
- Upstream connect failure: the (already-upgraded) socket is closed with WebSocket close code `1013` (try again later); the controller records the failure as a `502` (`upstream_connect_failed`, type `one_api_error`) and refunds the pre-consumed quota.

**Example**

Using a WebSocket client such as `wscat` with header-based auth:

```bash
wscat \
  --connect "$BASE_URL/v1/realtime?model=gpt-4o-realtime-preview" \
  --header "Authorization: Bearer $API_KEY" \
  --header "OpenAI-Beta: realtime=v1"
```

For a browser-style client that cannot set headers, pass the key via the subprotocol (note `https://` becomes `wss://`):

```bash
wscat \
  --connect "$BASE_URL/v1/realtime?model=gpt-4o-realtime-preview" \
  --subprotocol "realtime" \
  --subprotocol "openai-insecure-api-key.$API_KEY" \
  --subprotocol "openai-beta.realtime-v1"
```

Equivalent raw upgrade handshake (illustrative headers):

```bash
curl -i -N \
  -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" \
  -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  -H "Sec-WebSocket-Protocol: realtime, openai-insecure-api-key.$API_KEY, openai-beta.realtime-v1" \
  "$BASE_URL/v1/realtime?model=gpt-4o-realtime-preview"
```

**Errors**

| Status | Meaning |
|--------|---------|
| 400 (`missing required query parameter: model`) | The `?model=` query parameter was not supplied (rejected by auth/distribution middleware before the handler). |
| 400 (`ws_upgrade_failed`) | The WebSocket upgrade handshake failed. |
| 403 (`insufficient_quota`) | User quota is below the pre-consume estimate for a realtime session. |
| 403 (`pre_consume_failed`) | Pre-consuming the estimated quota from the token failed. |
| 500 (`one_api_error`) | The gateway could not read the user's quota before starting the session. |
| 502 (`upstream_connect_failed`) | The gateway could not establish the upstream WebSocket connection; pre-consumed quota is refunded. |
| 1008 (WS close) | A client `session.update` frame attempted to change the session model (`model_switch_denied`). |
| 1013 (WS close) | The upstream WebSocket connection could not be established (pairs with the `502 upstream_connect_failed` record). |

### POST /v1/realtime/sessions

Proxies to the upstream OpenAI Realtime Sessions endpoint to create a session and mint an ephemeral client token for WebRTC browser clients. Use this when establishing a Realtime session over WebRTC rather than the gateway's WebSocket relay.

**Auth:** Relay API KEY (`TokenAuth`). Send as `Authorization: Bearer $API_KEY` (the `X-Api-Key` and `Api-Key` headers are also accepted).

**Request body**

The body follows the upstream OpenAI Realtime Sessions schema and is forwarded as-is after model enforcement. The only field the gateway inspects is `model`; all other fields (e.g. `voice`, `modalities`, `instructions`) are passed through to upstream unchanged.

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| model | `model` | string | No | channel-bound model | The realtime model for the session. If empty or omitted, the channel-bound model is injected (and if the body is empty, a body of `{"model": <channel model>}` is synthesized). If it equals the channel's user-facing alias it is rewritten to the actual model. Any other value is rejected with `400 model_switch_denied` before the request reaches upstream (prevents minting an ephemeral token for an unbilled model). Enforcement is skipped only on the legacy path where the gateway could not resolve a bound model. |

Other fields, such as the following, are forwarded verbatim:

```json
{
  "model": "gpt-4o-realtime-preview",
  "voice": "verse",
  "modalities": ["audio", "text"]
}
```

**Response**

On success the gateway returns the upstream status (e.g. `200`/`201`), copies the upstream response headers, and relays the upstream JSON body verbatim, including the ephemeral `client_secret`. The shape follows the upstream OpenAI Realtime Sessions schema; a representative response:

```json
{
  "id": "sess_001",
  "object": "realtime.session",
  "model": "gpt-4o-realtime-preview",
  "modalities": ["audio", "text"],
  "voice": "verse",
  "client_secret": {
    "value": "ek_abc123",
    "expires_at": 1717000000
  }
}
```

**Example**

```bash
curl -X POST "$BASE_URL/v1/realtime/sessions" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-realtime-preview",
    "voice": "verse",
    "modalities": ["audio", "text"]
  }'
```

**Errors**

Failures are returned as the relay/OpenAI-style envelope `{"error": {"message": "...", "type": "...", "code": "..."}}` with the listed HTTP status.

| Status | Meaning |
|--------|---------|
| 400 (`invalid_request_body`) | The request body was present but not valid JSON. |
| 400 (`model_switch_denied`) | The body `model` did not match the channel-bound model (or its alias); the request was not forwarded upstream. |
| 502 (`upstream_request_failed`) | The upstream Realtime Sessions request could not be sent (network/dial failure). |
| 502 (`read_upstream_failed`) | The upstream responded but its body could not be read. |
| upstream status (>= 400) | Non-2xx upstream responses are surfaced with the upstream status code and the upstream body is relayed verbatim. |


## Embeddings & Rerank

This section covers the vector and ranking relay endpoints. Embeddings follow the OpenAI Embeddings schema; rerank uses the Jina/Cohere-style schema. All of these are inference/relay endpoints (routed through `controller.Relay` and protected by `TokenAuth`), so they require a Relay API KEY and return the OpenAI-style relay error envelope (`{"error": {...}}`) with the real HTTP status code. The gateway auto-detects the relay mode from the request path. Embeddings are billed by token usage; rerank is billed per call (one charge per request, independent of document count — see the per-call quota logic in `relay/controller/rerank.go`).

### POST /v1/embeddings

Creates one or more embedding vectors for the supplied input text(s), following the OpenAI Embeddings API. Use it to convert text into vectors for search, clustering, or retrieval.

**Auth:** Relay API KEY. `Authorization: Bearer $API_KEY` (also accepted: `X-Api-Key: $API_KEY`, `Api-Key: $API_KEY`).

The request and response bodies follow the upstream OpenAI Embeddings schema. The gateway-relevant fields are documented below; unknown JSON fields are logged and dropped during deserialization (they are not forwarded unless one-api models them).

**Request body**

| Field | JSON key | Type | Required | Default | Description |
| --- | --- | --- | --- | --- | --- |
| Model | `model` | string | Yes | — | The embedding model to use (e.g. `text-embedding-3-small`). |
| Input | `input` | string or array | Yes | — | Text to embed. A single string, or an array of strings/token arrays for batch embedding. |
| Encoding format | `encoding_format` | string | No | `float` | Output vector encoding. `float` returns numeric arrays; `base64` returns a base64-encoded float32 blob. |
| Dimensions | `dimensions` | integer | No | model default | Number of dimensions for the output vector (supported by newer embedding models only). |

```json
{
  "model": "text-embedding-3-small",
  "input": "The quick brown fox jumps over the lazy dog",
  "encoding_format": "float"
}
```

**Response**

`200 OK`. The body is an OpenAI-style embedding list.

| Field | Type | Description |
| --- | --- | --- |
| `object` | string | Always `list`. |
| `data` | array | One item per input. Each item has `object` (`embedding`), `index`, and `embedding` (numeric array). |
| `model` | string | The model that produced the vectors. |
| `usage` | object | Token accounting (`prompt_tokens`, `total_tokens`) used for billing. |

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "index": 0,
      "embedding": [0.0023064255, -0.009327292, 0.015797347]
    }
  ],
  "model": "text-embedding-3-small",
  "usage": {
    "prompt_tokens": 10,
    "total_tokens": 10
  }
}
```

**Example**

```bash
curl -X POST "$BASE_URL/v1/embeddings" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "text-embedding-3-small",
    "input": "The quick brown fox jumps over the lazy dog",
    "encoding_format": "float"
  }'
```

**Errors** (relay envelope `{"error": {...}}`)

| Status | Meaning |
| --- | --- |
| 400 | `model is required` — neither the body `model` nor a path model was supplied. |
| 401 | Missing or invalid Relay API KEY. |
| 403 | Model not permitted for this key, or insufficient quota. |

### POST /v1/engines/:model/embeddings

Legacy OpenAI "engines" form of the embeddings endpoint. Identical behavior to `POST /v1/embeddings`, except the model is taken from the `:model` path segment when the JSON body omits `model` (see `relay/controller/helper.go`: `textRequest.Model = c.Param("model")`). Use it only for clients that still target the legacy engines path.

**Auth:** Relay API KEY. `Authorization: Bearer $API_KEY` (also accepted: `X-Api-Key: $API_KEY`, `Api-Key: $API_KEY`).

**Path parameters**

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `model` | string | Yes | Embedding model id. Used as the request model when the body's `model` field is empty. |

**Request body**

Same schema as `POST /v1/embeddings`. The `model` field may be omitted here because it is resolved from the path. All other fields (`input`, `encoding_format`, `dimensions`) behave identically.

```json
{
  "input": "The quick brown fox jumps over the lazy dog",
  "encoding_format": "float"
}
```

**Response**

`200 OK`. Identical shape to `POST /v1/embeddings`.

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "index": 0,
      "embedding": [0.0023064255, -0.009327292, 0.015797347]
    }
  ],
  "model": "text-embedding-3-small",
  "usage": {
    "prompt_tokens": 10,
    "total_tokens": 10
  }
}
```

**Example**

```bash
curl -X POST "$BASE_URL/v1/engines/text-embedding-3-small/embeddings" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "input": "The quick brown fox jumps over the lazy dog",
    "encoding_format": "float"
  }'
```

### POST /v1/rerank

Reranks a list of documents by relevance to a query and returns them in scored order. Use it for search re-ranking, RAG candidate selection, and information retrieval. This route and `POST /v2/rerank` are equivalent (Jina/Cohere-style); both route to `controller.Relay` and the same rerank handler.

**Auth:** Relay API KEY. `Authorization: Bearer $API_KEY` (also accepted: `X-Api-Key: $API_KEY`, `Api-Key: $API_KEY`).

**Request body**

The canonical request DTO is `relaymodel.RerankRequest`. Fields below are the only ones modeled; any other JSON key is logged as unknown and dropped during deserialization (it is not forwarded upstream).

| Field | JSON key | Type | Required | Default | Description |
| --- | --- | --- | --- | --- | --- |
| Model | `model` | string | Yes | — | The rerank model to use (e.g. `rerank-v3.5`). |
| Query | `query` | string | Yes | — | The search query to rank documents against. If empty, the gateway derives it from a legacy `input` field; if still empty, the request is rejected. |
| Documents | `documents` | array of strings | Yes | — | Documents to rank. Up to ~1,000 documents per request. |
| Top N | `top_n` | integer | No | all | Number of top-ranked results to return. When present, must be greater than 0. |
| Max tokens per doc | `max_tokens_per_doc` | integer | No | 4096 | Maximum tokens considered per document. When present, must be >= 0. |
| Priority | `priority` | integer | No | 0 | Request priority hint. When present, must be between 0 and 999. |
| Input | `input` | any | No | — | Legacy compatibility field. Used only to derive `query` when `query` is absent. |

```json
{
  "model": "rerank-v3.5",
  "query": "What is the capital of the United States?",
  "top_n": 3,
  "documents": [
    "Carson City is the capital city of the American state of Nevada.",
    "The Commonwealth of the Northern Mariana Islands is a group of islands in the Pacific Ocean. Its capital is Saipan.",
    "Washington, D.C. is the capital of the United States. It is a federal district.",
    "Capitalization in English grammar is the use of a capital letter at the start of a word.",
    "Capital punishment has existed in the United States since before the United States was a country."
  ]
}
```

**Response**

`200 OK`. The upstream rerank response is forwarded transparently (Jina/Cohere shape); there is no canonical response struct.

| Field | Type | Description |
| --- | --- | --- |
| `results` | array | Ranked results. Each item has `index` (position in the original `documents` array) and `relevance_score` (higher is more relevant). Some upstreams may also include `document`. |
| `id` | string | Upstream request id. |
| `meta` | object | Upstream metadata, e.g. `api_version` and `billed_units`. |

```json
{
  "results": [
    { "index": 2, "relevance_score": 0.999 },
    { "index": 4, "relevance_score": 0.78 },
    { "index": 0, "relevance_score": 0.32 }
  ],
  "id": "some-unique-id",
  "meta": {
    "api_version": { "version": "2", "is_experimental": false },
    "billed_units": { "search_units": 1 }
  }
}
```

**Example**

```bash
curl -X POST "$BASE_URL/v1/rerank" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "rerank-v3.5",
    "query": "What is the capital of the United States?",
    "top_n": 3,
    "documents": [
      "Carson City is the capital city of the American state of Nevada.",
      "The Commonwealth of the Northern Mariana Islands is a group of islands in the Pacific Ocean. Its capital is Saipan.",
      "Washington, D.C. is the capital of the United States. It is a federal district.",
      "Capitalization in English grammar is the use of a capital letter at the start of a word.",
      "Capital punishment has existed in the United States since before the United States was a country."
    ]
  }'
```

**Errors** (relay envelope `{"error": {...}}`)

| Status | Meaning |
| --- | --- |
| 400 | `invalid_rerank_request` — validation failed (`model is required`, `field query is required`, `field documents is required`, `top_n must be greater than 0`, `max_tokens_per_doc must be >= 0`, or `priority must be between 0 and 999`). |
| 401 | Missing or invalid Relay API KEY. |
| 403 | `insufficient_user_quota` — the per-call quota exceeds the user's remaining balance; also returned when the model is not permitted for the key. |

### POST /v2/rerank

Cohere/Jina v2 rerank path. Equivalent to `POST /v1/rerank` in every respect — same `controller.Relay` handler, request DTO, response shape, per-call billing, and errors. Use whichever path your client library expects.

**Auth:** Relay API KEY. `Authorization: Bearer $API_KEY` (also accepted: `X-Api-Key: $API_KEY`, `Api-Key: $API_KEY`).

**Request body**

Identical to `POST /v1/rerank` (see that entry for the full field table).

```json
{
  "model": "rerank-v3.5",
  "query": "What is the capital of the United States?",
  "top_n": 3,
  "documents": [
    "Carson City is the capital city of the American state of Nevada.",
    "Washington, D.C. is the capital of the United States. It is a federal district.",
    "Capital punishment has existed in the United States since before the United States was a country."
  ]
}
```

**Response**

`200 OK`. Identical to `POST /v1/rerank`.

```json
{
  "results": [
    { "index": 1, "relevance_score": 0.999 },
    { "index": 2, "relevance_score": 0.41 },
    { "index": 0, "relevance_score": 0.32 }
  ],
  "id": "some-unique-id",
  "meta": {
    "api_version": { "version": "2", "is_experimental": false },
    "billed_units": { "search_units": 1 }
  }
}
```

**Example**

```bash
curl -X POST "$BASE_URL/v2/rerank" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "rerank-v3.5",
    "query": "What is the capital of the United States?",
    "top_n": 3,
    "documents": [
      "Carson City is the capital city of the American state of Nevada.",
      "Washington, D.C. is the capital of the United States. It is a federal district.",
      "Capital punishment has existed in the United States since before the United States was a country."
    ]
  }'
```


## Images, Audio & Video

These endpoints relay multimodal generation and recognition requests to the configured upstream provider. They are OpenAI-compatible: request and response bodies follow the upstream OpenAI schema, and the gateway adds quota accounting, model mapping, and per-second/per-image/per-token billing on top. Image generation/edit and audio transcription/translation accept `multipart/form-data`; text-to-speech returns a raw binary audio stream; and the `/v1/videos` family implements an asynchronous task flow (create, list, poll, download, cancel). All routes require a Relay API key (validated by `TokenAuth` before the handler runs). The gateway auto-detects the inbound format, so a request landing on the "wrong" path is transparently re-routed. Quota is metered in internal units where 500000 quota = 1 USD.

Failures return the OpenAI-style relay error envelope with the real HTTP status code:

```json
{
  "error": {
    "message": "...",
    "type": "...",
    "param": "",
    "code": "..."
  }
}
```

### POST /v1/images/generations

Generates one or more images from a text prompt, relaying to the upstream image model (DALL·E, gpt-image, grok-2-image, Zhipu CogView, Replicate, etc.).

**Auth:** Relay API key. Header: `Authorization: Bearer $API_KEY` (also accepted: `X-Api-Key: $API_KEY` or `Api-Key: $API_KEY`).

**Request body** (`application/json`; follows the upstream OpenAI Images schema — gateway-relevant fields below):

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Model | `model` | string | No | `dall-e-2` | Target image model. Defaults to `dall-e-2` when omitted. |
| Prompt | `prompt` | string | Yes | — | Text description of the desired image. Required and non-empty; length capped per model. |
| N | `n` | integer | No | `1` | Number of images to generate. Clamped to the model's allowed range. |
| Size | `size` | string | No | model-specific | e.g. `1024x1024`, `1024x1536`. Default depends on model (gpt-image family defaults to `1024x1536`, dall-e to `1024x1024`). Must be a size the model supports. |
| Quality | `quality` | string | No | model-specific | e.g. `standard`, `hd`, `high`. dall-e-3 accepts only `standard` or `hd`. gpt-image family defaults to `high`; dall-e family defaults to `standard`. |
| ResponseFormat | `response_format` | string | No | provider default | `url` or `b64_json`. Ignored (forced null) for `gpt-image-*` models. |
| Style | `style` | string | No | — | Provider-specific style hint (e.g. dall-e-3 `vivid`/`natural`). |
| User | `user` | string | No | — | Opaque end-user identifier forwarded upstream. |

```json
{
  "model": "dall-e-3",
  "prompt": "A red panda coding at a desk, watercolor style",
  "n": 1,
  "size": "1024x1024",
  "quality": "standard",
  "response_format": "url"
}
```

**Response:** `200 OK`. The body is the upstream OpenAI image response.

| Field | Type | Description |
|-------|------|-------------|
| `created` | integer | Unix timestamp of generation. |
| `data` | array | One object per image, each with `url` and/or `b64_json` and optional `revised_prompt`. |
| `usage` | object | Token usage (present for token-billed models such as gpt-image); includes `total_tokens`, `input_tokens`, `output_tokens`, `input_tokens_details`. |

```json
{
  "created": 1735689600,
  "data": [
    {
      "url": "https://upstream.example.com/generated/abc123.png",
      "revised_prompt": "A red panda in watercolor, seated at a wooden desk typing on a laptop"
    }
  ],
  "usage": {
    "total_tokens": 0,
    "input_tokens": 0,
    "output_tokens": 0,
    "input_tokens_details": {
      "text_tokens": 0,
      "image_tokens": 0
    }
  }
}
```

**Example:**

```bash
curl -X POST "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "dall-e-3",
    "prompt": "A red panda coding at a desk, watercolor style",
    "n": 1,
    "size": "1024x1024",
    "quality": "standard",
    "response_format": "url"
  }'
```

**Errors:**

| Status | Meaning |
|--------|---------|
| 400 | `prompt_missing` (empty prompt), `size_not_supported`, `prompt_too_long`, `n_not_within_range`, `invalid_value` (bad dall-e-3 quality), or `invalid_image_request`. |
| 403 | `insufficient_user_quota` (estimated cost exceeds remaining user quota). |

### POST /v1/images/edits

Edits or extends an existing image given an image (and optional mask) plus a prompt. Uses `multipart/form-data` so the source image bytes can be uploaded.

**Auth:** Relay API key. Header: `Authorization: Bearer $API_KEY`.

**Request body** (`multipart/form-data`; follows the upstream OpenAI image-edit schema):

| Field | Form key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Image | `image` | file | Yes | — | Source image to edit (e.g. PNG). |
| Mask | `mask` | file | No | — | Optional mask; transparent areas mark the region to edit. |
| Prompt | `prompt` | string | Yes | — | Text describing the desired edit. |
| Model | `model` | string | No | `dall-e-2` | Target model. |
| N | `n` | integer | No | `1` | Number of images. |
| Size | `size` | string | No | model-specific | Output size. |
| ResponseFormat | `response_format` | string | No | provider default | `url` or `b64_json` (ignored for `gpt-image-*`). |

**Response:** `200 OK`, same shape as `/v1/images/generations`.

```json
{
  "created": 1735689600,
  "data": [
    { "url": "https://upstream.example.com/edited/def456.png" }
  ],
  "usage": {
    "total_tokens": 0,
    "input_tokens": 0,
    "output_tokens": 0,
    "input_tokens_details": { "text_tokens": 0, "image_tokens": 0 }
  }
}
```

**Example:**

```bash
curl -X POST "$BASE_URL/v1/images/edits" \
  -H "Authorization: Bearer $API_KEY" \
  -F model="gpt-image-1" \
  -F image="@/path/to/source.png" \
  -F mask="@/path/to/mask.png" \
  -F prompt="Replace the background with a starry night sky" \
  -F n=1 \
  -F size="1024x1024"
```

**Errors:** Same image-validation and quota errors as `/v1/images/generations` (400 for invalid request fields, 403 for `insufficient_user_quota`).

### POST /v1/images/variations

Not implemented by this gateway. The route is registered behind the standard relay middleware, so a valid Relay API key is still required; once authenticated, the handler unconditionally returns the not-implemented error.

**Auth:** Relay API key. Header: `Authorization: Bearer $API_KEY`. A missing or invalid key is rejected by `TokenAuth` with `401` before the handler runs; a valid key reaches the handler and receives the `501` below.

**Response:** `501 Not Implemented` with the relay error envelope.

```json
{
  "error": {
    "message": "API not implemented",
    "type": "one_api_error",
    "param": "",
    "code": "api_not_implemented"
  }
}
```

**Example:**

```bash
curl -X POST "$BASE_URL/v1/images/variations" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"dall-e-2","image":"<base64>"}'
```

### POST /v1/audio/transcriptions

Transcribes uploaded audio into text in the audio's source language (Whisper-style). Uses `multipart/form-data`. Billed by audio duration, not output length.

**Auth:** Relay API key. Header: `Authorization: Bearer $API_KEY`.

**Request body** (`multipart/form-data`; follows the upstream OpenAI transcription schema):

| Field | Form key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| File | `file` | file | Yes | — | Audio file to transcribe (mp3, wav, m4a, etc.). |
| Model | `model` | string | No | `whisper-1` | Transcription model. Falls back to `whisper-1` if the form omits it. |
| Language | `language` | string | No | auto | ISO-639-1 hint for the input language. |
| Prompt | `prompt` | string | No | — | Optional text to guide the model's style/vocabulary. |
| ResponseFormat | `response_format` | string | No | `json` | One of `json`, `text`, `srt`, `verbose_json`, `vtt`. |
| Temperature | `temperature` | number | No | `0` | Sampling temperature. |
| TimestampGranularity | `timestamp_granularity` | string[] | No | — | e.g. `segment`, `word` (with `verbose_json`). |

**Response:** `200 OK`. The body is passed through verbatim from upstream in the requested `response_format`. For `json`:

```json
{
  "text": "Hello, this is a transcription of the uploaded audio."
}
```

**Example:**

```bash
curl -X POST "$BASE_URL/v1/audio/transcriptions" \
  -H "Authorization: Bearer $API_KEY" \
  -F model="whisper-1" \
  -F file="@/path/to/audio.mp3" \
  -F response_format="json"
```

**Errors:**

| Status | Meaning |
|--------|---------|
| 403 | `insufficient_user_quota` (audio-duration cost exceeds remaining quota) or `pre_consume_token_quota_failed`. |
| 500 | `count_audio_tokens_failed` (could not read/decode the uploaded audio to estimate duration). |

### POST /v1/audio/translations

Translates uploaded audio into English text. Identical multipart contract to `/v1/audio/transcriptions`; the upstream produces English output regardless of the source language.

**Auth:** Relay API key. Header: `Authorization: Bearer $API_KEY`.

**Request body** (`multipart/form-data`):

| Field | Form key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| File | `file` | file | Yes | — | Audio file to translate. |
| Model | `model` | string | No | `whisper-1` | Translation model. |
| Prompt | `prompt` | string | No | — | Optional style/vocabulary hint (in English). |
| ResponseFormat | `response_format` | string | No | `json` | One of `json`, `text`, `srt`, `verbose_json`, `vtt`. |
| Temperature | `temperature` | number | No | `0` | Sampling temperature. |

**Response:** `200 OK`, passed through from upstream. For `json`:

```json
{
  "text": "Hello, this is the English translation of the uploaded audio."
}
```

**Example:**

```bash
curl -X POST "$BASE_URL/v1/audio/translations" \
  -H "Authorization: Bearer $API_KEY" \
  -F model="whisper-1" \
  -F file="@/path/to/audio.mp3" \
  -F response_format="json"
```

**Errors:** Same as `/v1/audio/transcriptions` (403 `insufficient_user_quota` or `pre_consume_token_quota_failed`, 500 `count_audio_tokens_failed`).

### POST /v1/audio/speech

Synthesizes speech (text-to-speech) from input text and returns a raw binary audio stream. Billed by input character count.

**Auth:** Relay API key. Header: `Authorization: Bearer $API_KEY`.

**Request body** (`application/json`; follows the upstream OpenAI TTS schema):

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Model | `model` | string | Yes | — | TTS model (e.g. `tts-1`, `tts-1-hd`, `gpt-4o-mini-tts`). |
| Input | `input` | string | Yes | — | Text to synthesize. Maximum 4096 characters. |
| Voice | `voice` | string | Yes | — | Voice name (e.g. `alloy`, `echo`, `fable`, `onyx`, `nova`, `shimmer`). |
| Speed | `speed` | number | No | `1.0` | Playback speed multiplier. |
| ResponseFormat | `response_format` | string | No | `mp3` | Audio container: `mp3`, `opus`, `aac`, `flac`, `wav`, `pcm`. |

```json
{
  "model": "tts-1",
  "input": "Hello, welcome to the gateway.",
  "voice": "alloy",
  "speed": 1.0,
  "response_format": "mp3"
}
```

**Response:** `200 OK`. The body is the raw audio byte stream (not JSON); the upstream `Content-Type` (e.g. `audio/mpeg`) is forwarded to the client. Write the body to a file rather than parsing it.

**Example:**

```bash
curl -X POST "$BASE_URL/v1/audio/speech" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "tts-1",
    "input": "Hello, welcome to the gateway.",
    "voice": "alloy",
    "speed": 1.0,
    "response_format": "mp3"
  }' \
  --output speech.mp3
```

**Errors:**

| Status | Meaning |
|--------|---------|
| 400 | `invalid_json` (malformed body) or `text_too_long` (input over 4096 characters). |
| 403 | `insufficient_user_quota`. |

### POST /v1/videos

Creates an asynchronous video-generation task and returns a task object whose `id` is polled via the GET endpoints. Billed per second of requested duration, scaled by a resolution multiplier and the group ratio.

**Auth:** Relay API key. Header: `Authorization: Bearer $API_KEY`.

**Request body** (`application/json`; follows the upstream video schema — gateway-relevant fields below):

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Model | `model` | string | No | `sora-2` | Video model. Defaults to `sora-2` when omitted (falls back to the request-context model first, then `sora-2`). |
| Prompt | `prompt` | string | No | — | Text description of the video. |
| Seconds | `seconds` | number | Yes* | — | Requested duration in seconds. Used when `duration_seconds` is absent. |
| Duration | `duration` | number | Yes* | — | Alternative duration key. Lowest priority. |
| DurationSeconds | `duration_seconds` | number | Yes* | — | Duration key with the highest priority. |
| Size | `size` | string | No | — | Resolution hint (highest priority), e.g. `1280x720`. |
| Resolution | `resolution` | string | No | — | Resolution hint used if `size` is absent. |
| AspectRatio | `aspect_ratio` | string | No | — | e.g. `16:9`. |
| RemixID | `remix_id` | string | No | — | ID of a prior video to remix. |
| ReferenceID | `reference_id` | string | No | — | ID of a reference asset. |

\* At least one of `duration_seconds`, `seconds`, or `duration` must resolve to a positive number (priority: `duration_seconds` > `seconds` > `duration`); otherwise the request is rejected with `invalid_video_duration`.

```json
{
  "model": "sora-2",
  "prompt": "A timelapse of a city skyline at sunset",
  "seconds": 8,
  "size": "1280x720"
}
```

**Response:** `200 OK`. The body is the upstream video-task object passed through verbatim; the gateway does not reshape it. A representative shape:

```json
{
  "id": "video_abc123",
  "object": "video",
  "model": "sora-2",
  "status": "queued",
  "created_at": 1735689600,
  "seconds": 8
}
```

**Example:**

```bash
curl -X POST "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sora-2",
    "prompt": "A timelapse of a city skyline at sunset",
    "seconds": 8,
    "size": "1280x720"
  }'
```

**Errors:**

| Status | Meaning |
|--------|---------|
| 400 | `invalid_video_request` (malformed body), `invalid_video_duration` (no positive duration), or `video_pricing_missing` (no price configured for the model). |
| 403 | `insufficient_user_quota` (per-second cost exceeds remaining quota) or `pre_consume_token_quota_failed`. |

### GET /v1/videos

Lists the caller's video tasks. The gateway proxies the request to the upstream provider (non-POST video methods skip per-second quota accounting and are forwarded as-is).

**Auth:** Relay API key. Header: `Authorization: Bearer $API_KEY`.

**Response:** `200 OK`. The upstream list payload is passed through verbatim. Representative shape:

```json
{
  "object": "list",
  "data": [
    {
      "id": "video_abc123",
      "object": "video",
      "model": "sora-2",
      "status": "completed",
      "created_at": 1735689600
    }
  ]
}
```

**Example:**

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $API_KEY"
```

### GET /v1/videos/:video_id

Retrieves the status and metadata of a single video task (the polling endpoint). Proxied to upstream; non-POST video methods skip per-second quota accounting.

**Auth:** Relay API key. Header: `Authorization: Bearer $API_KEY`.

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `video_id` | string | Yes | The task ID returned by `POST /v1/videos`. |

**Response:** `200 OK`. The upstream task object is passed through verbatim. Representative shape:

```json
{
  "id": "video_abc123",
  "object": "video",
  "model": "sora-2",
  "status": "completed",
  "created_at": 1735689600,
  "seconds": 8
}
```

**Example:**

```bash
curl "$BASE_URL/v1/videos/video_abc123" \
  -H "Authorization: Bearer $API_KEY"
```

### GET /v1/videos/:video_id/content

Downloads the rendered video bytes once the task is complete. The response body is the raw binary asset stream; the upstream `Content-Type` (e.g. `video/mp4`) is forwarded. Proxied to upstream; non-POST video methods skip per-second quota accounting.

**Auth:** Relay API key. Header: `Authorization: Bearer $API_KEY`.

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `video_id` | string | Yes | The task ID whose rendered content to download. |

**Response:** `200 OK` with the raw video byte stream (not JSON). Write the body to a file. If the task is not yet complete, the upstream may return an error status that is forwarded as-is.

**Example:**

```bash
curl "$BASE_URL/v1/videos/video_abc123/content" \
  -H "Authorization: Bearer $API_KEY" \
  --output video.mp4
```

### DELETE /v1/videos/:video_id

Cancels or deletes a video task. Proxied to upstream; non-POST video methods skip per-second quota accounting.

**Auth:** Relay API key. Header: `Authorization: Bearer $API_KEY`.

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `video_id` | string | Yes | The task ID to cancel/delete. |

**Response:** `200 OK`. The upstream deletion confirmation is passed through verbatim. Representative shape:

```json
{
  "id": "video_abc123",
  "object": "video",
  "deleted": true
}
```

**Example:**

```bash
curl -X DELETE "$BASE_URL/v1/videos/video_abc123" \
  -H "Authorization: Bearer $API_KEY"
```


## OCR, MCP, Channel Proxy, Model Discovery & OpenRouter Listing

This section documents the auxiliary relay and discovery surfaces: the Zhipu-compatible document OCR endpoint, the Model Context Protocol (MCP) Streamable HTTP proxy, the channel-pinning raw passthrough, the per-key model discovery endpoints, and the public OpenRouter provider listing. All endpoints except the OpenRouter listing require the Relay API KEY (`$API_KEY`) validated by `TokenAuth`; they share the routing layer's auto-detection and quota machinery. Quotas are internal integers where 500000 quota = 1 USD.

### POST /api/paas/v4/layout_parsing

Performs document OCR / layout parsing on a single file using the Zhipu `/api/paas/v4` `layout_parsing` request schema. The request is auto-detected as OCR mode by path, converted by the channel adaptor, billed per call, and the upstream OCR response body is forwarded verbatim to the caller.

**Auth:** Relay API KEY. Header `Authorization: Bearer $API_KEY` (or `X-Api-Key: $API_KEY` / `Api-Key: $API_KEY`).

**Request body**

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Model | `model` | string | Yes | - | OCR model name (e.g. a Zhipu layout-parsing model). Subject to per-key model permission and channel model mapping. |
| File | `file` | string | Yes | - | The document to parse. Per the Zhipu schema this is a file reference (URL or base64-encoded content) accepted by the upstream channel. |
| RequestID | `request_id` | string | No | omitted | Optional client-supplied request identifier, forwarded upstream. |
| UserID | `user_id` | string | No | omitted | Optional end-user identifier, forwarded upstream. |
| ReturnCropImages | `return_crop_images` | boolean | No | omitted (upstream default) | Whether the upstream should return cropped region images. |
| NeedLayoutVisualization | `need_layout_visualization` | boolean | No | omitted (upstream default) | Whether the upstream should return a layout visualization. |
| StartPageID | `start_page_id` | integer | No | omitted (upstream default) | First page (inclusive) to parse. |
| EndPageID | `end_page_id` | integer | No | omitted (upstream default) | Last page (inclusive) to parse. |

```json
{
  "model": "glm-ocr",
  "file": "https://example.com/sample-invoice.pdf",
  "request_id": "req-20260608-001",
  "return_crop_images": true,
  "need_layout_visualization": false,
  "start_page_id": 0,
  "end_page_id": 4
}
```

**Response**

`200 OK`. The upstream OCR result is forwarded verbatim (the gateway only decodes it to extract `usage` for billing). Following the Zhipu `layout_parsing` schema, the body carries Markdown results, optional structured layout/visualization blobs, and a `usage` block:

| Field | JSON key | Type | Description |
|-------|----------|------|-------------|
| ID | `id` | string | Upstream result id. |
| Created | `created` | integer | Unix timestamp. |
| Model | `model` | string | The resolved (mapped) model name. |
| MdResults | `md_results` | string | Parsed document content rendered as Markdown. |
| LayoutDetails | `layout_details` | object/array | Structured layout blocks (omitted when not produced). |
| LayoutVisualization | `layout_visualization` | object/array | Layout visualization payload (present when `need_layout_visualization` is set). |
| DataInfo | `data_info` | object | Additional upstream metadata (omitted when empty). |
| RequestID | `request_id` | string | Echoes the client/upstream request id. |
| Usage | `usage` | object | Token accounting reported by upstream (`prompt_tokens`, `completion_tokens`, `total_tokens`). |

Billing is per-call: a flat model unit (`modelRatio`) times the group ratio, rounded up to at least 1 quota when the model unit is non-zero. It is settled asynchronously after the response is sent.

```json
{
  "id": "ocr-9f1c...",
  "created": 1749350400,
  "model": "glm-ocr",
  "md_results": "# INVOICE\n\n| Item | Qty | Price |\n| ---- | --- | ----- |\n...",
  "layout_details": [
    {
      "type": "title",
      "text": "INVOICE",
      "bbox": [72, 64, 320, 96]
    }
  ],
  "request_id": "req-20260608-001",
  "usage": {
    "prompt_tokens": 0,
    "completion_tokens": 0,
    "total_tokens": 0
  }
}
```

**Example**

```bash
curl -X POST "$BASE_URL/api/paas/v4/layout_parsing" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "glm-ocr",
    "file": "https://example.com/sample-invoice.pdf",
    "return_crop_images": true
  }'
```

**Errors** (OpenAI-style `{"error": {...}}` with the real status code)

| Status | `code` | Meaning |
|--------|--------|---------|
| 400 | `invalid_ocr_request` | Body is not valid JSON, or required `model` / `file` is missing. |
| 400 | `ocr_not_supported` | The routed channel's adaptor does not implement OCR. |
| 403 | `insufficient_user_quota` | The per-call quota exceeds the user's remaining balance. |
| 500 | `convert_request_failed` / `do_request_failed` | The adaptor failed to convert the request or reach upstream. |
| 4xx/5xx | (upstream `code`) | Upstream OCR errors are surfaced with the upstream status. |

### ANY /mcp

Single-endpoint Model Context Protocol (MCP) Streamable HTTP transport backed by the gateway's configured MCP servers. The handler dispatches by HTTP method: `POST` carries JSON-RPC 2.0 messages; `GET` (would-be server-initiated SSE) and `DELETE` (would-be session termination) are not implemented by this stateless proxy and return `405`.

**Auth:** Relay API KEY. Header `Authorization: Bearer $API_KEY` (or `X-Api-Key: $API_KEY` / `Api-Key: $API_KEY`). The authenticated user determines which MCP servers/tools are visible (after applying the user's MCP tool blacklist).

**Methods on this endpoint**

| Method | Behavior |
|--------|----------|
| `POST` | Processes a single JSON-RPC 2.0 request or notification (see methods below). |
| `GET` | `405 Method Not Allowed` (server-initiated SSE stream is not offered; empty body). |
| `DELETE` | `405 Method Not Allowed` (no server-side session to terminate; the proxy is stateless; empty body). |
| other | `405 Method Not Allowed`. |

**Supported JSON-RPC methods (POST body `method`)**

| `method` | Result |
|----------|--------|
| `initialize` | Returns the advertised `protocolVersion` (`2025-06-18`), `capabilities` (`tools.listChanged: false`), and `serverInfo` (`name`: `one-api-mcp-proxy`, `version`: `1.0.0`). |
| `tools/list` | Returns `tools`: the array of MCP tool descriptors the user may call. Server-qualified names use the form `<serverName>.<toolName>`. Always an array (`[]` when none). |
| `tools/call` | Invokes a tool (params `name`, `arguments`, optional `signature`), applies the tool's per-call quota, and returns the `CallToolResult`. |
| `ping` | Returns an empty result object. |
| `notifications/initialized`, `notifications/cancelled`, `notifications/progress`, `notifications/roots/list_changed` | Acknowledged with HTTP `202` and an empty body. |

**Request body**

A JSON-RPC 2.0 envelope. A request carries an `id`; a notification omits `id` (the server then replies `202` with no body). An unknown `method` without an `id` is also acknowledged with `202`.

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| JSONRPC | `jsonrpc` | string | Yes | - | Protocol version; `"2.0"`. |
| ID | `id` | string \| number \| null | No | null | Request correlation id. Omit (or `null`) to send a notification. |
| Method | `method` | string | Yes | - | One of the supported methods above. |
| Params | `params` | object | Conditional | - | Method parameters. For `tools/call`: `name` (string, required; `<server>.<tool>` or bare tool name), `arguments` (object), `signature` (string, optional). |

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "search.web_search",
    "arguments": {
      "query": "one-api mcp transport"
    }
  }
}
```

**Response**

`200 OK` for both results and JSON-RPC-level errors (the HTTP status stays 200; protocol errors travel in the `error` envelope). `202 Accepted` with an empty body for notifications. `405 Method Not Allowed` for `GET`/`DELETE`/other.

A successful result envelope:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Top result: ..."
      }
    ],
    "isError": false
  }
}
```

A JSON-RPC error envelope (HTTP still 200):

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "error": {
    "code": -32601,
    "message": "unsupported method tools/invoke"
  }
}
```

JSON-RPC error codes: `-32700` parse error, `-32600` invalid request, `-32601` method not found, `-32602` invalid params, `-32603` internal error.

**Example**

```bash
# tools/list
curl -X POST "$BASE_URL/mcp" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

**Errors**

| Status | Meaning |
|--------|---------|
| 401 | Missing/invalid relay API key (rejected by `TokenAuth` before the handler runs). |
| 405 | The request used `GET`, `DELETE`, or any method other than `POST`. |
| 200 + `error.code` -32700 | Request body is not valid JSON. |
| 200 + `error.code` -32602 | Invalid `tools/call` params. |
| 200 + `error.code` -32601 | Unknown `method` (when an `id` is present). |
| 200 + `error.code` -32603 | Internal error, e.g. failure to resolve the user's MCP servers/tools or an upstream tool-call failure. |

### ANY /v1/oneapi/proxy/:channelid/*target

Raw passthrough that forwards the request to one specific channel, identified by `:channelid`, with the remainder of the path (`*target`) appended verbatim to that channel's base URL. This is the channel-pinning feature: the gateway does not interpret the body or model; it relays bytes and copies the upstream status, headers, and body back to the caller. Proxy traffic is logged but consumes zero quota.

**Auth:** Relay API KEY. Header `Authorization: Bearer $API_KEY` (or `X-Api-Key: $API_KEY` / `Api-Key: $API_KEY`). Validated by `TokenAuth`; the `:channelid` path form pins the channel for any authenticated key (unlike the `sk-xxx:<channelid>` token-suffix form, which is admin-only). Before forwarding, the gateway replaces the inbound `Authorization` header with the pinned channel's own upstream credential.

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `channelid` | integer | Yes | Numeric id of the channel to forward to. Must parse as an integer or the request is rejected `400`. |
| `target` | string (wildcard) | Yes | The remaining path. The `/v1/oneapi/proxy/:channelid` prefix is stripped; everything after it is appended to the channel base URL (the original query string is preserved). |

**Request body**: any. The original request body is streamed to the upstream unchanged; the method (`GET`, `POST`, `PUT`, `DELETE`, etc.) is preserved because the route is registered for any HTTP method.

**Response**

The upstream response is mirrored: the original HTTP status code, response headers, and body bytes are copied back to the caller unmodified. There is no gateway envelope on the success path and no quota is charged (proxy requests are logged with `ModelName = "proxy"`, quota `0`).

**Example**

```bash
# Forward GET /v1/models to channel 42's upstream base URL
curl "$BASE_URL/v1/oneapi/proxy/42/v1/models" \
  -H "Authorization: Bearer $API_KEY"
```

```bash
# Forward a POST with a body verbatim to channel 42
curl -X POST "$BASE_URL/v1/oneapi/proxy/42/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ping"}]}'
```

**Errors** (gateway-side failures use the OpenAI-style `{"error": {...}}` envelope with the real status)

| Status | Meaning |
|--------|---------|
| 400 | `:channelid` is not a valid integer (`Invalid Channel Id: <value>`, raised during auth), or the channel id does not resolve to a channel (`Invalid Channel Id`, raised during distribution). |
| 403 | The pinned channel exists but is disabled (`The channel has been disabled`). |
| 4xx/5xx | Any upstream status is passed through as-is once the request is forwarded (the gateway does not rewrite upstream errors on this route). |

### GET /v1/models

Lists the models the authenticated key may access, in the OpenAI-compatible models-list format. The result is the intersection of the gateway's known model catalog and the abilities enabled for the key's user group, with hidden-model channels filtered out, sorted by `id`.

**Auth:** Relay API KEY. Header `Authorization: Bearer $API_KEY` (or `X-Api-Key: $API_KEY` / `Api-Key: $API_KEY`).

**Response**

`200 OK`. An object with `object: "list"` and `data`, an array of model entries (sorted by `id`).

| Field | Type | Description |
|-------|------|-------------|
| `object` | string | Always `"list"`. |
| `data[].id` | string | Model name. |
| `data[].object` | string | Always `"model"`. |
| `data[].created` | integer | Unix timestamp. |
| `data[].owned_by` | string | The owning channel/adaptor name (or `channel-<id>` when unnamed). |
| `data[].permission` | array | OpenAI-style permission records. |
| `data[].root` | string | Equals `id`. |
| `data[].parent` | string \| null | Always `null`. |

```json
{
  "object": "list",
  "data": [
    {
      "id": "gpt-4o-mini",
      "object": "model",
      "created": 1626777600,
      "owned_by": "openai",
      "permission": [
        {
          "id": "modelperm-LwHkVFn8AcMItP432fKKDIKJ",
          "object": "model_permission",
          "created": 1626777600,
          "allow_create_engine": true,
          "allow_sampling": true,
          "allow_logprobs": true,
          "allow_search_indices": false,
          "allow_view": true,
          "allow_fine_tuning": false,
          "organization": "*",
          "group": null,
          "is_blocking": false
        }
      ],
      "root": "gpt-4o-mini",
      "parent": null
    }
  ]
}
```

**Example**

```bash
curl "$BASE_URL/v1/models" \
  -H "Authorization: Bearer $API_KEY"
```

### GET /v1/models/:model

Retrieves the OpenAI-compatible descriptor for one model, if it is accessible to the authenticated key. Lookup is case-insensitive and limited to models enabled for the key's user group (after hidden-model filtering).

**Auth:** Relay API KEY. Header `Authorization: Bearer $API_KEY` (or `X-Api-Key: $API_KEY` / `Api-Key: $API_KEY`).

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `model` | string | Yes | The model id to retrieve (case-insensitive). |

**Response**

`200 OK` with a single model object (same shape as one `data[]` entry from `GET /v1/models`).

```json
{
  "id": "gpt-4o-mini",
  "object": "model",
  "created": 1626777600,
  "owned_by": "openai",
  "permission": [
    {
      "id": "modelperm-LwHkVFn8AcMItP432fKKDIKJ",
      "object": "model_permission",
      "created": 1626777600,
      "allow_create_engine": true,
      "allow_sampling": true,
      "allow_logprobs": true,
      "allow_search_indices": false,
      "allow_view": true,
      "allow_fine_tuning": false,
      "organization": "*",
      "group": null,
      "is_blocking": false
    }
  ],
  "root": "gpt-4o-mini",
  "parent": null
}
```

**Example**

```bash
curl "$BASE_URL/v1/models/gpt-4o-mini" \
  -H "Authorization: Bearer $API_KEY"
```

**Errors**

| Status | Meaning |
|--------|---------|
| 200 + `error` body | When the model is unknown or not permitted for the key, the response is `200 OK` carrying an OpenAI-style error: `{"error": {"message": "The model '<id>' does not exist", "type": "invalid_request_error", "param": "model", "code": "model_not_found"}}`. |

### GET /openrouter/v1/models

Public model catalog in the OpenRouter upstream-provider listing schema. OpenRouter scrapes this during provider onboarding and periodic refresh; it enumerates every model advertised by the gateway's adaptors (plus the OpenAI-compatible channels), deduplicated by case-insensitive id and sorted by lowercased name.

**Auth:** Public - no auth.

**Response**

`200 OK`. An object with a `data` array of OpenRouter `Model` entries. Required fields are always emitted (even when zero/empty); fields marked optional are omitted when empty (`omitempty`).

| Field | JSON key | Type | Required | Description |
|-------|----------|------|----------|-------------|
| Id | `id` | string | always | Model identifier. |
| HuggingFaceID | `hugging_face_id` | string | always | HuggingFace id when known (`""` otherwise). |
| Name | `name` | string | always | Display name. |
| Created | `created` | integer | always | Unix timestamp. |
| Description | `description` | string | optional | Short description (omitted when empty). |
| InputModalities | `input_modalities` | string[] | always | Supported input modalities (e.g. `text`, `image`). |
| OutputModalities | `output_modalities` | string[] | always | Supported output modalities. |
| Quantization | `quantization` | string | always | Quantization label (`""` when unset). |
| ContextLength | `context_length` | integer | always | Total context window. |
| MaxOutputLength | `max_output_length` | integer | always | Max output tokens. |
| Pricing | `pricing` | object | always | USD-per-unit pricing (see below). |
| SupportedSamplingParameters | `supported_sampling_parameters` | string[] | always | Accepted sampling parameters. |
| SupportedFeatures | `supported_features` | string[] | always | Capability flags. |
| Pricing.Prompt | `pricing.prompt` | string | always | USD per prompt token (`"0"` when none). |
| Pricing.Completion | `pricing.completion` | string | always | USD per completion token (`"0"` when none). |
| Pricing.Image | `pricing.image` | string | optional | USD per image (omitted when none). |
| Pricing.Request | `pricing.request` | string | optional | USD per request (omitted when none). |
| Pricing.InputCacheRead | `pricing.input_cache_read` | string | optional | USD per cached-input token (omitted when none). |

```json
{
  "data": [
    {
      "id": "gpt-4o-mini",
      "hugging_face_id": "",
      "name": "gpt-4o-mini",
      "created": 1749350400,
      "input_modalities": ["text", "image"],
      "output_modalities": ["text"],
      "quantization": "",
      "context_length": 128000,
      "max_output_length": 16384,
      "pricing": {
        "prompt": "0.00000015",
        "completion": "0.0000006"
      },
      "supported_sampling_parameters": ["temperature", "top_p"],
      "supported_features": ["tools"]
    }
  ]
}
```

**Example**

```bash
curl "$BASE_URL/openrouter/v1/models"
```

**Errors**

| Status | Meaning |
|--------|---------|
| 500 | The gateway failed to assemble the model catalog (OpenAI-style `{"error": {...}}`). |


## Usage, Billing Dashboard & API-Key Introspection

These endpoints let a holder of a Relay API KEY inspect the key's own quota, usage, model permissions, owning user, and external billing history. They are all guarded by `TokenAuth`, so they are authenticated with the API KEY itself (`Authorization: Bearer $API_KEY`), never with the management access token. Two families coexist here: an OpenAI-compatible billing view (`/dashboard/billing/*`, mirrored under `/v1/dashboard/billing/*`) that mimics the upstream OpenAI subscription/usage shape, and the gateway-specific introspection endpoints under `/api/token/*`, `/api/available_models`, and `/api/user/get-by-token` that return the management-style `{"success", "message", "data"}` envelope. The single write endpoint, `POST /api/token/consume`, deducts (or reserves/refunds) quota from the calling key for external/tool billing events. Unless noted, all quota figures are internal integer quota units where 500000 quota = 1 USD.

Because `TokenAuth` accepts the API key from any of three headers, every endpoint below accepts the key as `Authorization: Bearer $API_KEY`, `X-Api-Key: $API_KEY`, or `Api-Key: $API_KEY`. A missing or unknown key yields `401`; a valid key that is banned, subnet-restricted, or otherwise disallowed yields `403`.

### GET /dashboard/billing/subscription
### GET /v1/dashboard/billing/subscription

Returns the calling key's quota ceiling rendered in the OpenAI "billing subscription" shape, for compatibility with OpenAI billing clients. Both paths share `controller.GetSubscription`.

**Auth:** Relay API KEY. Header: `Authorization: Bearer $API_KEY` (or `X-Api-Key: $API_KEY` / `Api-Key: $API_KEY`).

**Response:** `200 OK`. The three "limit" fields all carry the same value. When the server runs with token-level stats enabled (`DisplayTokenStatEnabled`, the default), the value is `RemainQuota + UsedQuota` for the calling key; otherwise it is the owning user's current remaining quota. If `DisplayInCurrencyEnabled` (the default) the value is divided by 500000 to express USD; an unlimited-quota key reports a fixed sentinel of `100000000`. `access_until` is the key's expiry as a Unix timestamp in seconds (0 = never expires). `has_payment_method` is always `true`.

| Field | JSON key | Type | Description |
|---|---|---|---|
| Object | `object` | string | Always `"billing_subscription"`. |
| HasPaymentMethod | `has_payment_method` | boolean | Always `true`. |
| SoftLimitUSD | `soft_limit_usd` | number | Quota ceiling (USD if currency display enabled). |
| HardLimitUSD | `hard_limit_usd` | number | Same value as the soft limit. |
| SystemHardLimitUSD | `system_hard_limit_usd` | number | Same value as the soft limit. |
| AccessUntil | `access_until` | integer | Key expiry, Unix seconds; 0 = never. |

```json
{
  "object": "billing_subscription",
  "has_payment_method": true,
  "soft_limit_usd": 20,
  "hard_limit_usd": 20,
  "system_hard_limit_usd": 20,
  "access_until": 0
}
```

**Example:**

```bash
curl "$BASE_URL/v1/dashboard/billing/subscription" \
  -H "Authorization: Bearer $API_KEY"
```

**Errors:** On a backend lookup failure the body is the relay-style `{"error": {...}}` envelope (still HTTP 200) with `type` `"upstream_error"`, e.g. `{"error":{"message":"record not found","type":"upstream_error"}}`.

### GET /dashboard/billing/usage
### GET /v1/dashboard/billing/usage

Returns total quota already consumed, in the OpenAI "usage" shape. Both paths share `controller.GetUsage`.

**Auth:** Relay API KEY. Header: `Authorization: Bearer $API_KEY` (or `X-Api-Key` / `Api-Key`).

**Response:** `200 OK`. `total_usage` is the consumed quota expressed in hundredths of a US dollar (i.e. cents): the used quota is converted to USD (divide by 500000 when currency display is enabled) and then multiplied by 100. With token-level stats enabled the figure is the calling key's `used_quota`; otherwise it is the owning user's used quota.

| Field | JSON key | Type | Description |
|---|---|---|---|
| Object | `object` | string | Always `"list"`. |
| TotalUsage | `total_usage` | number | Consumed amount in units of 0.01 USD (cents). |

```json
{
  "object": "list",
  "total_usage": 357.5
}
```

**Example:**

```bash
curl "$BASE_URL/v1/dashboard/billing/usage" \
  -H "Authorization: Bearer $API_KEY"
```

**Errors:** On a backend lookup failure the body is `{"error": {...}}` (relay-style, HTTP 200) with `type` `"one_api_error"`.

### POST /api/token/consume

Records an external/tool billing event against the calling key, deducting quota from it. It supports a two-phase reserve-then-finalize flow (`pre` then `post`), an explicit `cancel`, and a legacy one-shot `single` mode. Each call first auto-confirms any of the key's pending transactions that have passed their timeout. Handler: `controller.ConsumeToken`.

**Auth:** Relay API KEY. Header: `Authorization: Bearer $API_KEY` (or `X-Api-Key` / `Api-Key`).

**Request body:**

| Field | JSON key | Type | Required | Default | Description |
|---|---|---|---|---|---|
| AddUsedQuota | `add_used_quota` | integer (uint64) | Conditional | 0 | Quota units to reserve (`pre`) or to charge (`single`). Required (> 0) for `pre`; for `post` it is the fallback when `final_used_quota` is omitted; for `single` a value of 0 records a free zero-quota audit entry. |
| AddReason | `add_reason` | string | Yes | — | Human-readable source/label of the billing event. Must be non-empty after trimming; surfaces as the log/transaction reason. |
| ElapsedTimeMs | `elapsed_time_ms` | integer | No | null | Upstream processing latency in milliseconds; recorded when > 0. |
| Phase | `phase` | string | No | `single` | Lifecycle stage: `pre`, `post`, `cancel`, or `single`. Case-insensitive; empty/omitted means `single`. |
| TransactionID | `transaction_id` | string | Conditional | — | Required for `post` and `cancel` (identifies the prior `pre`). Ignored/auto-generated for `pre`; auto-generated for `single` when omitted. |
| FinalUsedQuota | `final_used_quota` | integer (uint64) | No | null | Authoritative reconciled quota for `post`; the delta versus the reserved amount is charged or refunded. Falls back to `add_used_quota` when null. |
| TimeoutSeconds | `timeout_seconds` | integer | No | server default (600s) | Auto-confirm window for a `pre` hold, in seconds; clamped to the server's configured maximum (default 3600s). |

Reserve quota (`pre`):

```json
{
  "phase": "pre",
  "add_used_quota": 5000,
  "add_reason": "websearch-tool",
  "timeout_seconds": 600
}
```

Finalize that reservation (`post`):

```json
{
  "phase": "post",
  "transaction_id": "f3b9c0e2-7a4d-4c1e-9b2a-8d6f0a1c3e54",
  "final_used_quota": 4200,
  "add_reason": "websearch-tool",
  "elapsed_time_ms": 1830
}
```

**Response:** `200 OK`. The envelope is `{"success", "message", "data"}` where `data` is the updated token object (its `remain_quota`/`used_quota` reflect the deduction), plus a top-level `transaction` object describing the affected transaction.

| Field | JSON key | Type | Description |
|---|---|---|---|
| — | `success` | boolean | `true` on success. |
| — | `message` | string | Empty on success. |
| — | `data` | object | The refreshed token record for the calling key. |
| transaction.id | `transaction.id` | integer | Internal transaction row id. |
| transaction.transaction_id | `transaction.transaction_id` | string | The (possibly generated) external transaction id. |
| transaction.token_id | `transaction.token_id` | integer | Owning key id. |
| transaction.status_code | `transaction.status_code` | integer | 1=pending, 2=confirmed, 3=auto_confirmed, 4=canceled. |
| transaction.status | `transaction.status` | string | Label for the status code. |
| transaction.pre_quota | `transaction.pre_quota` | integer | Reserved quota. |
| transaction.final_quota | `transaction.final_quota` | integer / null | Reconciled quota; null while pending. |
| transaction.auto_confirmed | `transaction.auto_confirmed` | boolean | True when finalized by the timeout flow. |
| transaction.expires_at | `transaction.expires_at` | integer | Auto-confirm deadline (Unix seconds); 0 once terminal. |
| transaction.reason | `transaction.reason` | string | The `add_reason`. |
| transaction.request_id | `transaction.request_id` | string | Originating request id. |
| transaction.trace_id | `transaction.trace_id` | string | Originating trace id. |
| transaction.confirmed_at | `transaction.confirmed_at` | integer | Present when confirmed (Unix seconds). |
| transaction.canceled_at | `transaction.canceled_at` | integer | Present when canceled (Unix seconds). |
| transaction.log_id | `transaction.log_id` | integer | Associated consumption log id, when present. |
| transaction.elapsed_time_ms | `transaction.elapsed_time_ms` | integer | Present when latency was supplied. |

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 42,
    "user_id": 7,
    "key": "sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
    "status": 1,
    "name": "tool-key",
    "remain_quota": 95800,
    "used_quota": 4200,
    "unlimited_quota": false,
    "expired_time": -1
  },
  "transaction": {
    "id": 1001,
    "transaction_id": "f3b9c0e2-7a4d-4c1e-9b2a-8d6f0a1c3e54",
    "token_id": 42,
    "status_code": 2,
    "status": "confirmed",
    "pre_quota": 5000,
    "final_quota": 4200,
    "auto_confirmed": false,
    "expires_at": 0,
    "reason": "websearch-tool",
    "request_id": "2026060812000000000000000",
    "trace_id": "a1b2c3d4e5f6",
    "confirmed_at": 1749384000,
    "log_id": 55012,
    "elapsed_time_ms": 1830
  }
}
```

**Example:**

```bash
curl -X POST "$BASE_URL/api/token/consume" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "phase": "single",
    "add_used_quota": 5000,
    "add_reason": "websearch-tool",
    "elapsed_time_ms": 1830
  }'
```

**Errors:** Validation and lifecycle failures return `{"success": false, "message": "<reason>"}` (HTTP 200). Examples: `add_reason cannot be empty`; `add_used_quota must be greater than 0 for pre phase`; `transaction_id is required for post phase`; `transaction_id is required for cancel phase`; `final_used_quota or add_used_quota must be provided for post phase`; `transaction <id> is already confirmed`; `transaction <id> cannot be canceled because it is <state>`; `unsupported phase: <value>`. The key must be enabled, unexpired, and have quota, otherwise: `API Key is not enabled`, `The token has expired and cannot be used...`, or `The available quota of the token has been used up...`.

### GET /api/token/balance

Returns the calling key's current quota figures. Handler: `controller.GetTokenBalance`.

**Auth:** Relay API KEY. Header: `Authorization: Bearer $API_KEY` (or `X-Api-Key` / `Api-Key`).

**Response:** `200 OK`, management envelope; `data` carries the three balance fields (quota units).

| Field | JSON key | Type | Description |
|---|---|---|---|
| — | `data.remain_quota` | integer | Quota remaining on the key. |
| — | `data.used_quota` | integer | Quota consumed by the key. |
| — | `data.unlimited_quota` | boolean | Whether the key has no quota cap. |

```json
{
  "success": true,
  "message": "",
  "data": {
    "remain_quota": 95800,
    "used_quota": 4200,
    "unlimited_quota": false
  }
}
```

**Example:**

```bash
curl "$BASE_URL/api/token/balance" \
  -H "Authorization: Bearer $API_KEY"
```

**Errors:** If the key record cannot be loaded, returns `{"success": false, "message": "<reason>"}` (HTTP 200).

### GET /api/token/transactions

Returns a paginated history of the calling key's external billing transactions (those created via `POST /api/token/consume`), newest first. Handler: `controller.GetTokenTransactions`.

**Auth:** Relay API KEY. Header: `Authorization: Bearer $API_KEY` (or `X-Api-Key` / `Api-Key`).

**Query parameters:**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| p | integer | No | 0 | Zero-based page index; negatives are clamped to 0. |
| size | integer | No | 10 | Page size; capped at the server's max items per page (default 100). |

**Response:** `200 OK`, management envelope. `data` is an array of transaction objects (the `TokenTransaction` model); `total` is the transaction count, capped at the server's configured maximum retained history (`TokenTransactionsMaxHistory`, default 1000). Requesting a page whose start index is at/beyond that cap returns an empty `data` array with `total` equal to the cap.

| Field | JSON key | Type | Description |
|---|---|---|---|
| — | `data[].id` | integer | Internal row id. |
| — | `data[].transaction_id` | string | External transaction id. |
| — | `data[].token_id` | integer | Owning key id. |
| — | `data[].user_id` | integer | Owning user id. |
| — | `data[].status` | integer | 1=pending, 2=confirmed, 3=auto_confirmed, 4=canceled. |
| — | `data[].pre_quota` | integer | Reserved quota. |
| — | `data[].final_quota` | integer / null | Reconciled quota; null while pending. |
| — | `data[].reason` | string | Event reason. |
| — | `data[].request_id` | string | Originating request id. |
| — | `data[].trace_id` | string | Originating trace id. |
| — | `data[].expires_at` | integer | Auto-confirm deadline (Unix seconds); 0 once terminal. |
| — | `data[].confirmed_at` | integer / null | Confirmation time (Unix seconds). |
| — | `data[].canceled_at` | integer / null | Cancellation time (Unix seconds). |
| — | `data[].auto_confirmed` | boolean | Finalized by timeout flow. |
| — | `data[].log_id` | integer / null | Associated consumption log id. |
| — | `data[].elapsed_time_ms` | integer / null | Upstream latency, when recorded. |
| — | `data[].created_at` | integer | Creation time (Unix milliseconds). |
| — | `data[].updated_at` | integer | Last update time (Unix milliseconds). |
| — | `total` | integer | Total transactions (capped at the retained history limit). |

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 1001,
      "transaction_id": "f3b9c0e2-7a4d-4c1e-9b2a-8d6f0a1c3e54",
      "token_id": 42,
      "user_id": 7,
      "status": 2,
      "pre_quota": 5000,
      "final_quota": 4200,
      "reason": "websearch-tool",
      "request_id": "2026060812000000000000000",
      "trace_id": "a1b2c3d4e5f6",
      "expires_at": 0,
      "confirmed_at": 1749384000,
      "canceled_at": null,
      "auto_confirmed": false,
      "log_id": 55012,
      "elapsed_time_ms": 1830,
      "created_at": 1749383990000,
      "updated_at": 1749384000000
    }
  ],
  "total": 1
}
```

**Example:**

```bash
curl "$BASE_URL/api/token/transactions?p=0&size=10" \
  -H "Authorization: Bearer $API_KEY"
```

### GET /api/token/logs

Returns paginated consumption/billing log entries for the calling key, scoped to the authenticated key's name (i.e. the owning user's logs filtered to this key's `token_name`). Handler: `controller.GetTokenLogs`.

**Auth:** Relay API KEY. Header: `Authorization: Bearer $API_KEY` (or `X-Api-Key` / `Api-Key`).

**Query parameters:**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| p | integer | No | 0 | Zero-based page index; negatives are clamped to 0. |
| size | integer | No | server default (10) | Page size; capped at the server's max items per page (default 100). |
| type | integer | No | 0 | Log-type filter: 0=all, 1=topup, 2=consume, 3=manage, 4=system. |
| start_timestamp | integer | No | 0 | Lower bound on `created_at` (Unix seconds); 0 = no lower bound. |
| end_timestamp | integer | No | 0 | Upper bound on `created_at` (Unix seconds); 0 = no upper bound. |
| model_name | string | No | — | Exact model-name filter. |

**Response:** `200 OK`, management envelope. `data` is an array of log objects (the `Log` model); `total` is the matching count. Provisional (unreconciled pre-consume) rows are excluded.

| Field | JSON key | Type | Description |
|---|---|---|---|
| — | `data[].id` | integer | Log row id. |
| — | `data[].user_id` | integer | Owning user id. |
| — | `data[].created_at` | integer | Event time (Unix seconds). |
| — | `data[].type` | integer | Log type (see `type` param). |
| — | `data[].content` | string | Human-readable description. |
| — | `data[].username` | string | Owning username. |
| — | `data[].token_name` | string | Key name this log belongs to. |
| — | `data[].model_name` | string | Billed model name. |
| — | `data[].origin_model_name` | string | Client-requested model before mapping. |
| — | `data[].quota` | integer | Quota charged. |
| — | `data[].prompt_tokens` | integer | Prompt tokens. |
| — | `data[].completion_tokens` | integer | Completion tokens. |
| — | `data[].channel` | integer | Channel id used. |
| — | `data[].request_id` | string | Request id. |
| — | `data[].trace_id` | string | Trace id. |
| — | `data[].updated_at` | integer | Last update time (Unix milliseconds). |
| — | `data[].elapsed_time` | integer | Latency in milliseconds. |
| — | `data[].is_stream` | boolean | Whether the request streamed. |
| — | `data[].system_prompt_reset` | boolean | Whether the system prompt was reset. |
| — | `data[].cached_prompt_tokens` | integer | Cached prompt tokens. |
| — | `data[].metadata` | object | Provider-specific attributes (omitted when empty). |
| — | `total` | integer | Total matching logs. |

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 55012,
      "user_id": 7,
      "created_at": 1749384000,
      "type": 2,
      "content": "Model price ...",
      "username": "alice",
      "token_name": "tool-key",
      "model_name": "gpt-4o",
      "origin_model_name": "gpt-4o",
      "quota": 4200,
      "prompt_tokens": 1200,
      "completion_tokens": 340,
      "channel": 3,
      "request_id": "2026060812000000000000000",
      "trace_id": "a1b2c3d4e5f6",
      "updated_at": 1749384000000,
      "elapsed_time": 1830,
      "is_stream": false,
      "system_prompt_reset": false,
      "cached_prompt_tokens": 0
    }
  ],
  "total": 1
}
```

**Example:**

```bash
curl "$BASE_URL/api/token/logs?p=0&size=20&type=2" \
  -H "Authorization: Bearer $API_KEY"
```

### GET /api/available_models

Returns the list of models the calling key may invoke, intersecting the key's configured model restrictions with the models actually visible to the owning user's group. Handler: `controller.GetAvailableModelsByToken`.

**Auth:** Relay API KEY. Header: `Authorization: Bearer $API_KEY` (or `X-Api-Key` / `Api-Key`).

**Response:** `200 OK`. `data.available` is the array of permitted model names (de-duplicated, preserving the key's configured order, restricted to models visible to the user's group); `data.enabled` reflects whether the key's status is enabled. If the key has no model restriction configured (or the configured list is empty), the call returns `success: false` with message `the token has no available models` and `data.available: null` (still HTTP 200).

| Field | JSON key | Type | Description |
|---|---|---|---|
| — | `success` | boolean | True when a restricted model list is resolved. |
| — | `data.available` | array / null | Permitted model names, or null when none/error. |
| — | `data.enabled` | boolean | Whether the calling key is enabled. |

```json
{
  "success": true,
  "data": {
    "available": [
      "gpt-4o",
      "claude-sonnet-4-20250514"
    ],
    "enabled": true
  }
}
```

**Example:**

```bash
curl "$BASE_URL/api/available_models" \
  -H "Authorization: Bearer $API_KEY"
```

**Errors:** If the key cannot be loaded (or the group/ability lookup fails), returns `400 Bad Request` with `{"success": false, "message": "<reason>", "data": {"available": null, "enabled": false}}`.

### GET /api/user/get-by-token

Returns the user that owns the calling key together with that key's own metadata, letting a key holder identify itself without the management access token. Handler: `controller.GetSelfByToken`.

**Auth:** Relay API KEY. Header: `Authorization: Bearer $API_KEY` (or `X-Api-Key` / `Api-Key`).

**Response:** `200 OK`, management envelope. `data.user` describes the owning user and `data.token` describes the calling key; the same values are additionally flattened as top-level `uid`, `username`, and `token_*` fields for convenience. `data.token.models` and `data.token.subnet` are the raw configured restriction strings (null when unset or blank), while `available_models` is the comma-joined effective model list from the auth context.

| Field | JSON key | Type | Description |
|---|---|---|---|
| — | `data.user.id` | integer | Owning user id. |
| — | `data.user.username` | string | Username. |
| — | `data.user.display_name` | string | Display name. |
| — | `data.user.role` | integer | 1=common, 10=admin, 100=root. |
| — | `data.user.status` | integer | User status code. |
| — | `data.user.group` | string | User group. |
| — | `data.user.quota` | integer | User remaining quota. |
| — | `data.user.used_quota` | integer | User total used quota. |
| — | `data.user.created_at` | integer | User creation time (Unix milliseconds). |
| — | `data.user.updated_at` | integer | User update time (Unix milliseconds). |
| — | `data.token.id` | integer | Calling key id. |
| — | `data.token.name` | string | Key name. |
| — | `data.token.status` | integer | Key status code. |
| — | `data.token.remain_quota` | integer | Key remaining quota. |
| — | `data.token.used_quota` | integer | Key used quota. |
| — | `data.token.unlimited_quota` | boolean | Whether the key is uncapped. |
| — | `data.token.expired_time` | integer | Key expiry (Unix seconds); -1 = never. |
| — | `data.token.accessed_time` | integer | Last-used time (Unix seconds). |
| — | `data.token.created_time` | integer | Key creation time (Unix seconds). |
| — | `data.token.created_at` | integer | Row creation time (Unix milliseconds). |
| — | `data.token.updated_at` | integer | Row update time (Unix milliseconds). |
| — | `data.token.models` | string / null | Configured model restriction, raw. |
| — | `data.token.subnet` | string / null | Configured subnet restriction, raw. |
| — | `data.token.available_models` | string | Effective model list, comma-joined. |
| — | `uid` | integer | Flattened owning user id. |
| — | `username` | string | Flattened username. |
| — | `token_id` | integer | Flattened key id. |
| — | `token_name` | string | Flattened key name. |
| — | `token_status` | integer | Flattened key status. |
| — | `token_used_quota` | integer | Flattened key used quota. |
| — | `token_remain_quota` | integer | Flattened key remaining quota. |
| — | `token_unlimited_quota` | boolean | Flattened uncapped flag. |
| — | `token_created_time` | integer | Flattened key creation time (Unix seconds). |
| — | `token_updated_at` | integer | Flattened row update time (Unix milliseconds). |
| — | `token_accessed_time` | integer | Flattened last-used time (Unix seconds). |
| — | `token_expired_time` | integer | Flattened expiry (Unix seconds); -1 = never. |
| — | `token_available_models` | string | Flattened effective model list, comma-joined. |

```json
{
  "success": true,
  "message": "",
  "data": {
    "user": {
      "id": 7,
      "username": "alice",
      "display_name": "Alice",
      "role": 1,
      "status": 1,
      "group": "default",
      "quota": 9580000,
      "used_quota": 420000,
      "created_at": 1735689600000,
      "updated_at": 1749384000000
    },
    "token": {
      "id": 42,
      "name": "tool-key",
      "status": 1,
      "remain_quota": 95800,
      "used_quota": 4200,
      "unlimited_quota": false,
      "expired_time": -1,
      "accessed_time": 1749384000,
      "created_time": 1735689600,
      "created_at": 1735689600000,
      "updated_at": 1749384000000,
      "models": "gpt-4o,claude-sonnet-4-20250514",
      "subnet": null,
      "available_models": "gpt-4o,claude-sonnet-4-20250514"
    }
  },
  "uid": 7,
  "username": "alice",
  "token_id": 42,
  "token_name": "tool-key",
  "token_status": 1,
  "token_used_quota": 4200,
  "token_remain_quota": 95800,
  "token_unlimited_quota": false,
  "token_created_time": 1735689600,
  "token_updated_at": 1749384000000,
  "token_accessed_time": 1749384000,
  "token_expired_time": -1,
  "token_available_models": "gpt-4o,claude-sonnet-4-20250514"
}
```

**Example:**

```bash
curl "$BASE_URL/api/user/get-by-token" \
  -H "Authorization: Bearer $API_KEY"
```

**Errors:** `400 Bad Request` with `{"success": false, "message": "missing token context"}` if the authenticated key context is incomplete (`uid` or `token_id` resolves to 0).


## Authentication & Account Lifecycle

This section documents the public and session-bound endpoints that create, authenticate, and recover One API accounts. These endpoints establish the credentials described in the global authentication overview: `POST /api/user/register` creates a password account, `POST /api/user/login` and the passkey/OAuth flows mint the browser **session cookie**, and the `/api/oauth/wechat/bind` and `/api/oauth/email/bind` endpoints attach external identities to an already-authenticated account. All of these are management-API endpoints and therefore return the management envelope (`{"success": ..., "message": ..., "data": ...}` with HTTP 200 in the common case); they do not use the OpenAI-style relay error envelope. Sensitive endpoints are protected by `CriticalRateLimit` (a per-client rate limiter that returns HTTP 429 when exceeded) and, where noted, by `TurnstileCheck` (Cloudflare Turnstile, active only when the operator enables it). The exact credential names, headers, and roles referenced below are defined in the global overview.

### POST /api/user/register

Creates a new local account using username + password (plus an email verification code when email verification is enabled). The account is created with the common-user role.

**Auth:** Public - no auth. Protected by `CriticalRateLimit` and `TurnstileCheck`. Rejected with an error message when the operator has disabled registration (`RegisterEnabled`) or password registration (`PasswordRegisterEnabled`).

**Request body**

| Field | JSON key | Type | Required | Default | Description |
|---|---|---|---|---|---|
| Username | `username` | string | yes | - | Login identity. Validated `max=30` (max 30 characters); a non-empty unique value is required by the account insert. |
| Password | `password` | string | yes | - | Validated `min=8,max=20` (8-20 characters). |
| Email | `email` | string | only if email verification is enabled | - | Required together with `verification_code` when `EmailVerificationEnabled`. Validated `max=50`. |
| Verification code | `verification_code` | string | only if email verification is enabled | - | The 6-digit code previously issued by `GET /api/verification`. Not persisted to the DB. |
| Affiliate code | `aff_code` | string | no | - | The inviter's affiliate code (not the new user's own code). |

```json
{
  "username": "alice",
  "password": "s3cretpassw0rd",
  "email": "alice@example.com",
  "verification_code": "123456",
  "aff_code": "Ab12"
}
```

**Response**

HTTP 200. On success the envelope carries no payload (`data` is omitted).

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -X POST "$BASE_URL/api/user/register" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "alice",
    "password": "s3cretpassw0rd",
    "email": "alice@example.com",
    "verification_code": "123456",
    "aff_code": "Ab12"
  }'
```

**Errors**

| Condition | Envelope `message` (HTTP 200, `success:false`) |
|---|---|
| Registration disabled | `The administrator has turned off new user registration` |
| Password registration disabled | `The administrator has turned off registration via password. Please use the form of third-party account verification to register` |
| Email verification on but email/code missing | `The administrator has turned on email verification, please enter the email address and verification code` |
| Bad or expired code | `Verification code error or expired` |
| Field validation failed (e.g. password length) | invalid-input error |

### POST /api/user/login

Authenticates a username/password (optionally TOTP) pair and, on success, issues the **session cookie** used by the web dashboard and management endpoints.

**Auth:** Public - no auth. Protected by `CriticalRateLimit`. Turnstile becomes required for a username after a recent failed login when `TurnstileCheckEnabled` is set; pass the token via the `turnstile` query parameter.

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `turnstile` | string | conditional | - | Cloudflare Turnstile token. Required only when a prior login for this username failed and Turnstile is enabled. |

**Request body**

| Field | JSON key | Type | Required | Default | Description |
|---|---|---|---|---|---|
| Username | `username` | string | yes | - | Account login name (an email address is also accepted). |
| Password | `password` | string | yes | - | Account password. |
| TOTP code | `totp_code` | string | conditional | - | Six-digit TOTP code; required only when the account has TOTP enabled. |

```json
{
  "username": "alice",
  "password": "s3cretpassw0rd",
  "totp_code": "654321"
}
```

**Response**

HTTP 200. On success a `Set-Cookie` session header is returned and `data` holds a sanitized user object.

| Field | Type | Description |
|---|---|---|
| `data.id` | int | User id. |
| `data.username` | string | Login name. |
| `data.display_name` | string | Display name. |
| `data.role` | int | 1 = common, 10 = admin, 100 = root. |
| `data.status` | int | 1 = enabled, 2 = disabled, 3 = deleted. |

```json
{
  "message": "",
  "success": true,
  "data": {
    "id": 42,
    "username": "alice",
    "display_name": "Alice",
    "role": 1,
    "status": 1
  }
}
```

When the account has TOTP enabled and `totp_code` was omitted, login is not completed and the server signals that a second factor is required:

```json
{
  "success": false,
  "message": "totp_required",
  "data": {
    "totp_required": true
  }
}
```

A Turnstile challenge is similarly signaled with `"data": {"turnstile_required": true}`. Note that when `TurnstileCheckEnabled` is set, any failed credential validation (wrong username/password) also returns `data.turnstile_required: true` so the client knows to present a challenge on the next attempt.

**Example**

```bash
curl -X POST "$BASE_URL/api/user/login" \
  -H "Content-Type: application/json" \
  -c cookies.txt \
  -d '{
    "username": "alice",
    "password": "s3cretpassw0rd"
  }'
```

**Errors**

| Condition | Behavior |
|---|---|
| Missing username or password | `success:false`, invalid-parameter message |
| Wrong credentials | `success:false` with the validation error; when Turnstile is enabled, `data.turnstile_required:true`, and subsequent logins for the username require Turnstile |
| TOTP enabled, code missing | `success:false`, `message:"totp_required"`, `data.totp_required:true` |
| Invalid TOTP code | `success:false`, `Invalid TOTP code` |
| Too many TOTP attempts | HTTP 429, `Too many TOTP verification attempts. Please wait before trying again.` |
| Password login disabled for non-root | `The administrator has disabled password login. Please use a third-party authentication method (e.g. OIDC) to log in.` |

### GET /api/user/logout

Clears the current browser session (server-side session store and cookie).

**Auth:** Public - no auth (operates on whatever session cookie is presented; a missing session is a no-op success).

**Response**

HTTP 200.

```json
{
  "message": "",
  "success": true
}
```

**Example**

```bash
curl -X GET "$BASE_URL/api/user/logout" \
  -b cookies.txt -c cookies.txt
```

### POST /api/user/passkey/login/begin

Starts a discoverable WebAuthn (passkey) login ceremony. No user identifier is supplied up front; the server generates an assertion challenge and stores its session data server-side. Returns the challenge options the browser passes to `navigator.credentials.get`.

**Auth:** Public - no auth. Protected by `CriticalRateLimit`. Requires that a session cookie can be set, since the ceremony state is held in the session between begin and finish.

**Request body**

None. (The body is ignored.)

**Response**

HTTP 200. `data` is the WebAuthn `PublicKeyCredentialRequestOptions` object produced by the server (a `publicKey` wrapper containing `challenge`, `rpId`, `timeout`, `userVerification`, etc.). The shape follows the WebAuthn specification; the gateway-relevant point is that the entire `data` object is forwarded verbatim to the browser API.

```json
{
  "success": true,
  "data": {
    "publicKey": {
      "challenge": "k5p2QF7mE0u3...",
      "timeout": 60000,
      "rpId": "oneapi.laisky.com",
      "userVerification": "preferred"
    }
  }
}
```

**Example**

```bash
curl -X POST "$BASE_URL/api/user/passkey/login/begin" \
  -H "Content-Type: application/json" \
  -c cookies.txt
```

### POST /api/user/passkey/login/finish

Completes the discoverable passkey login by verifying the authenticator assertion against the challenge stored in the session, resolves the account from the credential's user handle, and on success issues the **session cookie** (the same login setup as password login).

**Auth:** Public - no auth. Protected by `CriticalRateLimit`. Requires the session cookie set by `passkey/login/begin`.

**Request body**

The serialized WebAuthn assertion (`PublicKeyCredential` with `response.authenticatorData`, `response.clientDataJSON`, `response.signature`, `response.userHandle`, plus `id`, `rawId`, `type`). This is the object returned by `navigator.credentials.get` JSON-serialized; it follows the WebAuthn specification and is parsed from the raw HTTP request body.

```json
{
  "id": "AY3b...",
  "rawId": "AY3b...",
  "type": "public-key",
  "response": {
    "authenticatorData": "SZYN5YgOj...",
    "clientDataJSON": "eyJ0eXBlIjoid2Vi...",
    "signature": "MEUCIQ...",
    "userHandle": "AAAAAAAAACo="
  }
}
```

**Response**

HTTP 200. On success returns the same sanitized user object and `Set-Cookie` session as `POST /api/user/login`.

```json
{
  "message": "",
  "success": true,
  "data": {
    "id": 42,
    "username": "alice",
    "display_name": "Alice",
    "role": 1,
    "status": 1
  }
}
```

**Example**

```bash
curl -X POST "$BASE_URL/api/user/passkey/login/finish" \
  -H "Content-Type: application/json" \
  -b cookies.txt -c cookies.txt \
  --data @assertion.json
```

**Errors**

| Condition | Envelope `message` |
|---|---|
| No begin step / expired session | `no login session found, please start again` |
| Assertion verification failed | `login failed: <reason>` |
| Resolved account disabled | `login failed: user account is disabled` |

### GET /api/oauth/github

OAuth callback for GitHub sign-in. The browser is redirected here by GitHub with `code` and `state`. If the session already carries a logged-in username, the request is treated as a bind instead (see Binding identities below); otherwise it logs in an existing GitHub-linked account or, when registration is enabled, provisions a new one. On success it issues the **session cookie**.

**Auth:** Public - no auth. Protected by `CriticalRateLimit`. Requires a session containing the `oauth_state` previously set by `GET /api/oauth/state`.

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `code` | string | yes | - | GitHub authorization code, exchanged server-side for an access token. |
| `state` | string | yes | - | Anti-CSRF state; must equal the value stored in the session by `/api/oauth/state`. |

**Response**

HTTP 200. On success returns the sanitized user object and the session cookie, identical in shape to `POST /api/user/login`. For a newly provisioned account the username defaults to `github_<n>`.

```json
{
  "message": "",
  "success": true,
  "data": {
    "id": 51,
    "username": "github_52",
    "display_name": "Alice",
    "role": 1,
    "status": 1
  }
}
```

**Example**

```bash
curl -X GET "$BASE_URL/api/oauth/github?code=GITHUB_CODE&state=OAUTH_STATE" \
  -b cookies.txt -c cookies.txt
```

**Errors**

| Condition | Behavior |
|---|---|
| Missing/mismatched `state` | HTTP 403, `state is empty or not same` |
| GitHub login disabled | `The administrator did not turn on login and registration via GitHub` |
| New user but registration disabled | `The administrator has turned off new user registration` |
| Account banned | `User has been banned` |

### GET /api/oauth/oidc

OAuth callback for a generic OIDC provider. Exchanges `code` at the configured token endpoint, fetches userinfo, then logs in or provisions the account (username taken from `preferred_username` when present, otherwise `oidc_<n>`). If the session already carries a logged-in username, the request becomes an OIDC bind. On success it issues the **session cookie**.

**Auth:** Public - no auth. Protected by `CriticalRateLimit`. Requires the session `oauth_state` from `GET /api/oauth/state`.

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `code` | string | yes | - | OIDC authorization code. |
| `state` | string | yes | - | Anti-CSRF state; must equal the session value from `/api/oauth/state`. |

**Response**

HTTP 200, same shape as `POST /api/user/login`.

```json
{
  "message": "",
  "success": true,
  "data": {
    "id": 60,
    "username": "alice",
    "display_name": "Alice",
    "role": 1,
    "status": 1
  }
}
```

**Example**

```bash
curl -X GET "$BASE_URL/api/oauth/oidc?code=OIDC_CODE&state=OAUTH_STATE" \
  -b cookies.txt -c cookies.txt
```

**Errors**

| Condition | Behavior |
|---|---|
| Missing/mismatched `state` | HTTP 403, `state is empty or not same` |
| OIDC disabled | `Administrator has not enabled OIDC Log in and Sign up` |
| New user but registration disabled | `The administrator has turned off new user registration` |
| Account banned | `User has been banned` |

### GET /api/oauth/lark

OAuth callback for Lark / Feishu sign-in. Exchanges `code` at Feishu's token endpoint, fetches userinfo, then logs in or provisions the account (username derived from the email local-part, otherwise `lark_<n>`). If the session already carries a logged-in username, the request becomes a Lark bind. On success it issues the **session cookie**.

**Auth:** Public - no auth. Protected by `CriticalRateLimit`. Requires the session `oauth_state` from `GET /api/oauth/state`.

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `code` | string | yes | - | Lark authorization code. |
| `state` | string | yes | - | Anti-CSRF state; must equal the session value from `/api/oauth/state`. |

**Response**

HTTP 200, same shape as `POST /api/user/login`.

```json
{
  "message": "",
  "success": true,
  "data": {
    "id": 71,
    "username": "alice",
    "display_name": "Alice",
    "role": 1,
    "status": 1
  }
}
```

**Example**

```bash
curl -X GET "$BASE_URL/api/oauth/lark?code=LARK_CODE&state=OAUTH_STATE" \
  -b cookies.txt -c cookies.txt
```

**Errors**

| Condition | Behavior |
|---|---|
| Missing/mismatched `state` | HTTP 403, `state is empty or not same` |
| New user but registration disabled | `The administrator has turned off new user registration` |
| Account banned | `User has been banned` |

> Note: unlike the GitHub and OIDC callbacks, the Lark login path has no dedicated "feature disabled" guard before the token exchange; a misconfiguration surfaces as an upstream connect/parse error from Feishu.

### GET /api/oauth/wechat

WeChat sign-in callback. Resolves a WeChat id from `code` via the configured WeChat auth server, then logs in or provisions the account (username defaults to `wechat_<n>`). On success it issues the **session cookie**. Unlike the GitHub/OIDC/Lark callbacks, this endpoint does not validate an `oauth_state` parameter.

**Auth:** Public - no auth. Protected by `CriticalRateLimit`.

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `code` | string | yes | - | WeChat verification code (QR scan code), exchanged for the WeChat id. |

**Response**

HTTP 200, same shape as `POST /api/user/login`.

```json
{
  "message": "",
  "success": true,
  "data": {
    "id": 80,
    "username": "wechat_81",
    "display_name": "WeChat User",
    "role": 1,
    "status": 1
  }
}
```

**Example**

```bash
curl -X GET "$BASE_URL/api/oauth/wechat?code=WECHAT_CODE" \
  -b cookies.txt -c cookies.txt
```

**Errors**

| Condition | Behavior |
|---|---|
| WeChat login disabled | `The administrator has not enabled login and registration via WeChat` |
| Empty / invalid code | `Verification code error or expired` |
| New user but registration disabled | `The administrator has turned off new user registration` |
| Account banned | `User has been banned` |

### GET /api/oauth/state

Generates a random anti-CSRF `state` value, stores it in the session, and returns it. Call this before redirecting the user to a GitHub / OIDC / Lark authorization URL, then pass the returned value back as the `state` query parameter on the corresponding callback.

**Auth:** Public - no auth. Protected by `CriticalRateLimit`. Sets a session cookie that the callback must later present.

**Response**

HTTP 200. `data` is the opaque 12-character state string.

```json
{
  "success": true,
  "message": "",
  "data": "Xa9KmZ3qLp0w"
}
```

**Example**

```bash
curl -X GET "$BASE_URL/api/oauth/state" \
  -c cookies.txt
```

### GET /api/oauth/wechat/bind

Binds a WeChat identity to the currently authenticated account. Resolves the WeChat id from `code` and attaches it to the caller's user record.

**Auth:** Management ACCESS TOKEN as `Authorization: $ACCESS_TOKEN`, or a valid session cookie. Requires UserAuth (role >= 1). Protected by `CriticalRateLimit`.

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `code` | string | yes | - | WeChat verification code resolved to the WeChat id to bind. |

**Response**

HTTP 200, no payload.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -X GET "$BASE_URL/api/oauth/wechat/bind?code=WECHAT_CODE" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Condition | Behavior |
|---|---|
| Missing/invalid credential | HTTP 401 |
| WeChat login disabled | `The administrator has not enabled login and registration via WeChat` |
| WeChat id already linked to another account | `The WeChat account has been bound` |

### GET /api/oauth/email/bind

Binds (or changes) the email address on the currently authenticated account, gated by a verification code previously sent to that address. When the caller is the root user, the system's root email is also updated.

**Auth:** Management ACCESS TOKEN as `Authorization: $ACCESS_TOKEN`, or a valid session cookie. Requires UserAuth (role >= 1). Protected by `CriticalRateLimit`.

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `email` | string | yes | - | The email address to bind. |
| `code` | string | yes | - | The verification code sent to that email by `GET /api/verification`. |

**Response**

HTTP 200, no payload.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -X GET "$BASE_URL/api/oauth/email/bind?email=alice%40example.com&code=123456" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Condition | Behavior |
|---|---|
| Missing/invalid credential | HTTP 401 |
| Bad or expired code | `Verification code error or expired` |

### GET /api/verification

Issues an email verification code for the supplied address (used for registration and for `GET /api/oauth/email/bind`). To resist user enumeration and timing attacks the response is always a uniform success (after a fixed ~1s delay) and the actual whitelist/occupancy check plus email delivery happen asynchronously; a code is only generated when the address passes the domain whitelist and is not already taken.

**Auth:** Public - no auth. Protected by `CriticalRateLimit` and `TurnstileCheck`.

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `email` | string | yes | - | The recipient email address. Validated `required,email` (must be a syntactically valid email). |

**Response**

HTTP 200. The message is intentionally uniform regardless of whether an email is actually sent.

```json
{
  "success": true,
  "message": "If the email is valid and not already registered, you will receive a verification code shortly."
}
```

**Example**

```bash
curl -X GET "$BASE_URL/api/verification?email=alice%40example.com"
```

**Errors**

| Condition | Behavior |
|---|---|
| Missing or malformed email | `success:false`, invalid-parameter message |

### GET /api/reset_password

Sends a password-reset link (containing `email` and a `token`) to the supplied address when that address is registered. Like `GET /api/verification`, the response is a uniform success and the registration check plus email delivery run asynchronously to resist enumeration. The emailed link points at `<ServerAddress>/user/reset?email=...&token=...`; the `token` is then submitted to `POST /api/user/reset`.

**Auth:** Public - no auth. Protected by `CriticalRateLimit` and `TurnstileCheck`.

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `email` | string | yes | - | The account email to send the reset link to. Validated `required,email` (must be a syntactically valid email). |

**Response**

HTTP 200.

```json
{
  "success": true,
  "message": "If the email is registered, you will receive a password reset link shortly."
}
```

**Example**

```bash
curl -X GET "$BASE_URL/api/reset_password?email=alice%40example.com"
```

**Errors**

| Condition | Behavior |
|---|---|
| Missing or malformed email | `success:false`, invalid-parameter message |

### POST /api/user/reset

Completes a password reset by validating the emailed token for the address. If `password` is provided it becomes the new password; if omitted (legacy clients) the server generates a random password and returns it in `data`. The token is single-use and is deleted on success.

**Auth:** Public - no auth. Protected by `CriticalRateLimit`.

**Request body**

| Field | JSON key | Type | Required | Default | Description |
|---|---|---|---|---|---|
| Email | `email` | string | yes | - | The account email being reset. |
| Token | `token` | string | yes | - | The reset token from the emailed link. |
| Password | `password` | string | no | random | New password. When omitted, the server generates one and returns it in `data`. |

```json
{
  "email": "alice@example.com",
  "token": "a1b2c3d4e5f6",
  "password": "myNewPassw0rd"
}
```

**Response**

HTTP 200. `data` is the effective password: the value supplied by the caller, or the server-generated random password when `password` was omitted.

```json
{
  "success": true,
  "message": "",
  "data": "myNewPassw0rd"
}
```

**Example**

```bash
curl -X POST "$BASE_URL/api/user/reset" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "alice@example.com",
    "token": "a1b2c3d4e5f6",
    "password": "myNewPassw0rd"
  }'
```

**Errors**

| Condition | Envelope `message` |
|---|---|
| Missing email or token | invalid-parameter message |
| Invalid or expired token | `Reset link is illegal or expired` |
| Password locked by administrator | `Password is locked by administrator` |

---

**Binding identities via the OAuth callbacks.** `GET /api/oauth/github`, `/api/oauth/oidc`, and `/api/oauth/lark` double as bind endpoints: when the active **session cookie** already identifies a logged-in user (the session carries a `username`), the same callback links the external identity to that account instead of logging in, returning `{"success": true, "message": "bind"}`. (The GitHub/OIDC callbacks still validate `oauth_state` before dispatching to the bind path.) Binding fails with the corresponding "account has been bound" / "already been bound" message (`The GitHub account has been bound`, `This OIDC account has already been bound`, `This Lark account has already been bound`) if the external identity is already linked elsewhere. WeChat and email use dedicated bind endpoints (`/api/oauth/wechat/bind`, `/api/oauth/email/bind`) which require UserAuth rather than reusing the login callback; on success `/api/oauth/wechat/bind` returns an empty `message` (not `"bind"`).

Relevant source files (absolute paths):
- `/home/laisky/repo/laisky/one-api/controller/user.go` (Register, Login, SetupLogin, Logout, EmailBind)
- `/home/laisky/repo/laisky/one-api/controller/misc.go` (SendEmailVerification, SendPasswordResetEmail, ResetPassword)
- `/home/laisky/repo/laisky/one-api/controller/passkey.go` (PasskeyLoginBegin, PasskeyLoginFinish)
- `/home/laisky/repo/laisky/one-api/controller/auth/github.go`, `oidc.go`, `lark.go`, `wechat.go` (OAuth + GenerateOAuthCode + WeChatBind)
- `/home/laisky/repo/laisky/one-api/router/api.go` (route + middleware wiring)
- `/home/laisky/repo/laisky/one-api/model/user.go` (User struct validation tags)
- `/home/laisky/repo/laisky/one-api/common/helper/helper.go` (RespondError / RespondErrorWithStatus envelope)
- `/home/laisky/repo/laisky/one-api/middleware/auth.go` (UserAuth: session-then-access-token, 401/403 semantics)


## Self-Service Account, Access Token, 2FA, Passkeys, Logs, Trace & Cost

This section documents the endpoints a signed-in user calls to manage their own account: reading and updating the profile, deleting the account, minting the management access token, viewing dashboards and affiliate codes, redeeming top-up keys, enrolling 2FA (TOTP) and passkeys, querying their own request logs and usage statistics, and inspecting per-request traces and costs. Every endpoint here is reached under `/api` and—except for `GET /api/cost/request/:request_id`—is protected by `UserAuth` (role >= 1). When there is no browser session, authenticate these calls with the management **access token** in the `Authorization` header (see `GET /api/user/token` below for how to mint it). Unless stated otherwise, responses use the management envelope `{"success": bool, "message": string, "data": ...}` returned with HTTP 200; error bodies from these handlers are also HTTP 200 with `{"success": false, "message": "<reason>"}` unless a specific status code is called out.

> **Quota unit reminder:** quota is an internal integer where `500000 quota = 1 USD` (1 quota = $0.000002).

### GET /api/user/self

Returns the authenticated user's own profile record.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Response:** HTTP 200. `data` is the user object. The `password` and `access_token` columns are stripped server-side (the record is loaded with `Omit("password", "access_token")`). `totp_secret` is `omitempty`, so it is absent when empty — but note that when TOTP is enabled it **is** present in this payload. `metadata.password_locked` is `omitempty`, so an account with the lock cleared serializes `metadata` as `{}`.

| Field | JSON key | Type | Description |
|---|---|---|---|
| Id | `id` | integer | User ID. |
| Username | `username` | string | Login identity (unique). |
| DisplayName | `display_name` | string | Human-friendly name. |
| Role | `role` | integer | 1 = common, 10 = admin, 100 = root. |
| Status | `status` | integer | 1 = enabled, 2 = disabled, 3 = deleted. |
| Email | `email` | string | Bound email, if any. |
| Quota | `quota` | integer | Remaining quota (internal units). |
| UsedQuota | `used_quota` | integer | Lifetime consumed quota. |
| RequestCount | `request_count` | integer | Lifetime request count. |
| Group | `group` | string | Pricing/permission group. |
| AffCode | `aff_code` | string | Affiliate code (may be empty until first requested). |
| Metadata | `metadata` | object | Account metadata; `password_locked` appears only when `true`. |
| CreatedAt | `created_at` | integer | Creation time (ms epoch). |
| UpdatedAt | `updated_at` | integer | Last update time (ms epoch). |

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 42,
    "username": "alice",
    "display_name": "Alice",
    "role": 1,
    "status": 1,
    "email": "alice@example.com",
    "github_id": "",
    "wechat_id": "",
    "lark_id": "",
    "oidc_id": "",
    "quota": 4500000,
    "used_quota": 500000,
    "request_count": 128,
    "group": "default",
    "aff_code": "Ab3D",
    "inviter_id": 0,
    "mcp_tool_blacklist": null,
    "metadata": {},
    "created_at": 1716000000000,
    "updated_at": 1717000000000
  }
}
```

**Example**

```bash
curl -s "$BASE_URL/api/user/self" \
  -H "Authorization: $ACCESS_TOKEN"
```

### PUT /api/user/self

Updates the authenticated user's own profile. Only `username`, `display_name`, and `password` are honored by this endpoint; any other fields in the body are ignored. Partial payloads are supported: an omitted or empty `username` is silently restored from the current record (so the login identity can never be blanked through self-update), an omitted `display_name` is restored while an explicitly provided empty `display_name` clears it, and an empty/omitted `password` leaves the password unchanged.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Request body**

| Field | JSON key | Type | Required | Default | Description |
|---|---|---|---|---|---|
| Username | `username` | string | No | current value | New login name. Max 30 characters. An empty value is ignored (restored from current). |
| DisplayName | `display_name` | string | No | current value | New display name. Max 20 characters. Provide `""` to clear it. |
| Password | `password` | string | No | unchanged | New password, 8–20 characters. Rejected if the account has `password_locked` set by an admin. |

```json
{
  "username": "alice",
  "display_name": "Alice Liddell",
  "password": "newSecret123"
}
```

**Response:** HTTP 200. No `data` payload on success.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -s -X PUT "$BASE_URL/api/user/self" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"display_name":"Alice Liddell"}'
```

**Errors** (all HTTP 200 with `success:false`)

| Condition | Meaning |
|---|---|
| `Password is locked by administrator` | Account has `password_locked` set; password cannot be self-changed. |
| `Input is illegal ...` | Validation failed (e.g. username > 30 chars, display_name > 20 chars, password not 8–20 chars). |

### DELETE /api/user/self

Permanently deletes the authenticated user's own account.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Response:** HTTP 200.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -s -X DELETE "$BASE_URL/api/user/self" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Condition | Meaning |
|---|---|
| `Cannot delete super administrator account` | A root (role 100) user cannot self-delete. |

### GET /api/user/dashboard

Returns per-day usage breakdowns (by model, by user, by token, and by MCP tool) plus quota and account status for charting. Date range is an inclusive whole-UTC-day range via `from_date`/`to_date` (interpreted internally as a half-open `[from 00:00 UTC, to+1 00:00 UTC)` interval); common users may request at most 7 days, root users up to 365 days. When the range is omitted it defaults to the last 7 days (today-6 .. today). Root users default to site-wide aggregates and may target a specific user or `all`.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `from_date` | string `YYYY-MM-DD` | No | today-6 | Inclusive start day (UTC). Must be paired with `to_date`; supplying only one is ignored and the default range applies. |
| `to_date` | string `YYYY-MM-DD` | No | today | Inclusive end day (UTC). |
| `user_id` | string | No | self (root: site-wide) | Root only. A numeric user ID, or `all` for site-wide stats. A non-root caller supplying this gets `success:false` with `No permission to view other users' dashboard data`. |

**Response:** HTTP 200. `data` aggregates several per-day arrays plus quota fields.

| Field | JSON key | Type | Description |
|---|---|---|---|
| Logs | `logs` | array | Per-day, per-model usage rows. |
| User logs | `user_logs` | array | Per-day, per-user usage rows. |
| Token logs | `token_logs` | array | Per-day, per-token usage rows. |
| Tool logs | `tool_logs` | array | Per-day, per-tool MCP usage rows. |
| Tool user logs | `tool_user_logs` | array | Per-day, per-user MCP tool usage. |
| Tool token logs | `tool_token_logs` | array | Per-day, per-token MCP tool usage. |
| Total quota | `total_quota` | integer | Remaining quota (user) or site-wide total. |
| Used quota | `used_quota` | integer | Consumed quota. |
| Status | `status` | string | `Active` / `Disabled` / `Deleted` / `Unknown` for a single user; aggregate status for site-wide. |

```json
{
  "success": true,
  "message": "",
  "data": {
    "logs": [
      { "Day": "2026-06-07", "ModelName": "gpt-4o-mini", "RequestCount": 12, "Quota": 2400, "PromptTokens": 1500, "CompletionTokens": 800 }
    ],
    "user_logs": [],
    "token_logs": [],
    "tool_logs": [],
    "tool_user_logs": [],
    "tool_token_logs": [],
    "total_quota": 4500000,
    "used_quota": 500000,
    "status": "Active"
  }
}
```

**Example**

```bash
curl -s "$BASE_URL/api/user/dashboard?from_date=2026-06-01&to_date=2026-06-07" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/user/dashboard/users

Returns a flat list of users (id, username, display_name) for populating the dashboard's user-selector dropdown, prefixed by an "All Users (Site-wide)" option with id 0. Root-only.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie). Requires role 100 (root); non-root callers receive `{"success": false, "message": "No permission to access user list"}` (HTTP 200).

**Response:** HTTP 200. `data` is an array of user options (up to 1000 users plus the leading "all" entry).

```json
{
  "success": true,
  "message": "",
  "data": [
    { "id": 0, "username": "all", "display_name": "All Users (Site-wide)" },
    { "id": 42, "username": "alice", "display_name": "Alice" }
  ]
}
```

**Example**

```bash
curl -s "$BASE_URL/api/user/dashboard/users" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/user/aff

Returns the user's affiliate (invitation) code, lazily generating a 4-character code on first call.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Response:** HTTP 200. `data` is the affiliate code string.

```json
{
  "success": true,
  "message": "",
  "data": "Ab3D"
}
```

**Example**

```bash
curl -s "$BASE_URL/api/user/aff" \
  -H "Authorization: $ACCESS_TOKEN"
```

### POST /api/user/topup

Redeems a redemption (gift/recharge) code, crediting its quota to the authenticated user's balance.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Request body**

| Field | JSON key | Type | Required | Default | Description |
|---|---|---|---|---|---|
| Key | `key` | string | Yes | — | The redemption code to redeem. |

```json
{
  "key": "REDEEM-XXXX-YYYY-ZZZZ"
}
```

**Response:** HTTP 200. `data` is the quota amount credited by this redemption (internal units).

```json
{
  "success": true,
  "message": "",
  "data": 500000
}
```

**Example**

```bash
curl -s -X POST "$BASE_URL/api/user/topup" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"key":"REDEEM-XXXX-YYYY-ZZZZ"}'
```

**Errors**

| Condition | Meaning |
|---|---|
| `message` describing an invalid/used/expired code | The redemption code is unknown, already consumed, or disabled (returned by `model.Redeem`). |

### GET /api/user/available_models

Lists the model names the authenticated user is allowed to call, derived from the user's group and visible channel abilities. Use this to discover which `model` values are valid for relay requests.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Response:** HTTP 200. `data` is a sorted array of model name strings (an empty array when none are accessible).

```json
{
  "success": true,
  "message": "",
  "data": [
    "claude-3-5-sonnet-20241022",
    "gpt-4o",
    "gpt-4o-mini"
  ]
}
```

**Example**

```bash
curl -s "$BASE_URL/api/user/available_models" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/user/token

Mints (and rotates) the management **access token** — the 32-character UUID credential used to authenticate the management/admin REST API headlessly when there is no browser session. **Each call generates a brand-new token and overwrites the previous one**, immediately invalidating any access token issued earlier. Because this endpoint itself requires `UserAuth`, the very first token must be obtained via a logged-in web session (cookie) from `POST /api/user/login`; thereafter the returned token authenticates subsequent management calls.

**Auth:** Web session cookie (typical for the first issuance), or an existing management access token — `Authorization: $ACCESS_TOKEN`.

**Response:** HTTP 200. `data` is the newly generated access token string (a 32-char hyphen-free UUID, no `sk-` prefix).

```json
{
  "success": true,
  "message": "",
  "data": "9f8b1c2d3e4f5a6b7c8d9e0f1a2b3c4d"
}
```

**Example — mint the token (using a session cookie), then use it for a follow-up management call**

```bash
# 1) Mint/rotate the access token (authenticated by the dashboard session cookie).
ACCESS_TOKEN=$(curl -s "$BASE_URL/api/user/token" -b cookies.txt \
  | sed -n 's/.*"data":"\([^"]*\)".*/\1/p')

# 2) Use the returned token to authenticate a headless management call.
curl -s "$BASE_URL/api/user/self" \
  -H "Authorization: $ACCESS_TOKEN"
```

> **Note:** This is distinct from a Relay API KEY (`sk-...`). The access token authenticates management endpoints under `/api`; it cannot be used as a relay/inference key.

### GET /api/user/totp/status

Reports whether time-based one-time-password (TOTP) two-factor authentication is currently enabled for the authenticated user.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Response:** HTTP 200.

```json
{
  "success": true,
  "message": "",
  "data": {
    "totp_enabled": false
  }
}
```

**Example**

```bash
curl -s "$BASE_URL/api/user/totp/status" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/user/totp/setup

Begins TOTP enrollment: generates a fresh Base32 secret and an `otpauth://` provisioning URI (for rendering a QR code in an authenticator app). The secret is stored in the server-side session (`temp_totp_secret`) pending confirmation; enrollment is not active until `POST /api/user/totp/confirm` succeeds. The route is guarded by `UserAuth`, but because the pending secret lives in the session this step in practice relies on the **web session cookie**.

**Auth:** `UserAuth` plus the web session cookie (the temp secret is written to the session).

**Response:** HTTP 200. Note this endpoint's success envelope omits `message`.

| Field | JSON key | Type | Description |
|---|---|---|---|
| Secret | `secret` | string | Base32 TOTP secret to register manually if the QR cannot be scanned. |
| QR code | `qr_code` | string | `otpauth://totp/...` provisioning URI to encode as a QR image. |

```json
{
  "success": true,
  "data": {
    "secret": "JBSWY3DPEHPK3PXP",
    "qr_code": "otpauth://totp/One%20API:alice?secret=JBSWY3DPEHPK3PXP&issuer=One%20API"
  }
}
```

**Example**

```bash
curl -s "$BASE_URL/api/user/totp/setup" -b cookies.txt
```

**Errors**

| Condition | Meaning |
|---|---|
| `MFA enrollment is locked by administrator` | The account has `password_locked` set, which also locks MFA enrollment. |

### POST /api/user/totp/confirm

Confirms TOTP enrollment by verifying a code generated from the pending secret created at `/api/user/totp/setup`, then persists the secret and activates 2FA. Subject to per-user TOTP rate limiting and replay protection.

**Auth:** `UserAuth` plus the web session cookie (the pending secret is read from the session).

**Request body**

| Field | JSON key | Type | Required | Default | Description |
|---|---|---|---|---|---|
| TotpCode | `totp_code` | string | Yes | — | The current 6-digit code from the authenticator app. |

```json
{
  "totp_code": "123456"
}
```

**Response:** HTTP 200.

```json
{
  "success": true,
  "message": "TOTP has been successfully enabled"
}
```

**Example**

```bash
curl -s -X POST "$BASE_URL/api/user/totp/confirm" -b cookies.txt \
  -H "Content-Type: application/json" \
  -d '{"totp_code":"123456"}'
```

**Errors**

| Condition | Status / meaning |
|---|---|
| `Too many TOTP verification attempts. Please wait before trying again.` | HTTP 429 — TOTP rate limit exceeded. |
| `TOTP code is required` | HTTP 200 — `totp_code` was empty. |
| `No TOTP setup session found. Please start setup again.` | HTTP 200 — the pending secret expired or the session changed; restart at `/totp/setup`. |
| `Invalid TOTP code` | HTTP 200 — the supplied code did not verify (or was a replay). |
| `MFA enrollment is locked by administrator` | HTTP 200 — account `password_locked` set. |

### POST /api/user/totp/disable

Disables TOTP for the authenticated user after verifying a current code. Subject to per-user TOTP rate limiting. This endpoint reads the user from the auth context and the stored secret from the database, so it does **not** depend on a setup session.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Request body**

| Field | JSON key | Type | Required | Default | Description |
|---|---|---|---|---|---|
| TotpCode | `totp_code` | string | Yes | — | A current valid TOTP code, required to authorize disabling. |

```json
{
  "totp_code": "123456"
}
```

**Response:** HTTP 200.

```json
{
  "success": true,
  "message": "TOTP has been successfully disabled"
}
```

**Example**

```bash
curl -s -X POST "$BASE_URL/api/user/totp/disable" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"totp_code":"123456"}'
```

**Errors**

| Condition | Status / meaning |
|---|---|
| `Too many TOTP verification attempts. Please wait before trying again.` | HTTP 429 — rate limit. |
| `TOTP is not enabled for this user` | HTTP 200 — nothing to disable. |
| `Invalid TOTP code` | HTTP 200 — code did not verify (an empty `totp_code` also yields this). |

### GET /api/user/passkey

Lists the WebAuthn passkey credentials registered to the authenticated user.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Response:** HTTP 200. The success envelope omits `message`; `data` is an array of passkey summaries.

| Field | JSON key | Type | Description |
|---|---|---|---|
| Id | `id` | integer | Credential row ID (used in delete/rename paths). |
| Credential name | `credential_name` | string | User-assigned label. |
| Sign count | `sign_count` | integer | Authenticator signature counter. |
| Created at | `created_at` | integer | Creation time (ms epoch). |

```json
{
  "success": true,
  "data": [
    {
      "id": 7,
      "credential_name": "YubiKey 5C",
      "sign_count": 12,
      "created_at": 1716500000000
    }
  ]
}
```

**Example**

```bash
curl -s "$BASE_URL/api/user/passkey" \
  -H "Authorization: $ACCESS_TOKEN"
```

### POST /api/user/passkey/register/begin

Starts a WebAuthn registration ceremony, returning the credential-creation options to pass to the browser's `navigator.credentials.create()`. The challenge/session is stored server-side in the session, so this flow in practice requires the **web session cookie**. No request body is needed.

**Auth:** `UserAuth` plus the web session cookie (the registration challenge is kept in the session).

**Response:** HTTP 200. The success envelope omits `message`; `data` is the WebAuthn credential-creation options object produced by the server (passed verbatim to the browser API). The exact structure follows the WebAuthn spec; a representative shape:

```json
{
  "success": true,
  "data": {
    "publicKey": {
      "challenge": "k9aR...base64url...",
      "rp": { "name": "One API", "id": "oneapi.laisky.com" },
      "user": { "id": "AAAAAAAAACo", "name": "alice", "displayName": "Alice" },
      "pubKeyCredParams": [ { "type": "public-key", "alg": -7 } ],
      "authenticatorSelection": { "residentKey": "required" },
      "excludeCredentials": []
    }
  }
}
```

**Example**

```bash
curl -s -X POST "$BASE_URL/api/user/passkey/register/begin" -b cookies.txt
```

**Errors**

| Condition | Meaning |
|---|---|
| `WebAuthn not available` | Server WebAuthn (RP) configuration could not be initialised. |
| `MFA enrollment is locked by administrator` | Account `password_locked` set. |

### POST /api/user/passkey/register/finish

Completes the WebAuthn registration ceremony, verifying the authenticator's attestation against the challenge stored at the begin step and persisting the credential. The request body is the browser's `navigator.credentials.create()` result (a `PublicKeyCredential`/attestation JSON), posted as-is. An optional credential label is supplied via the `name` query parameter.

**Auth:** `UserAuth` plus the web session cookie (the registration challenge is read from the session).

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | No | `Passkey` | Label for the new credential. Trimmed; truncated to 128 characters. |

**Request body:** the WebAuthn attestation object returned by the browser. Follows the WebAuthn `PublicKeyCredential` JSON schema; a representative example:

```json
{
  "id": "AQIDBAUGBwgJCgsMDQ4PEA",
  "rawId": "AQIDBAUGBwgJCgsMDQ4PEA",
  "type": "public-key",
  "response": {
    "attestationObject": "o2NmbXRkbm9uZ...base64url...",
    "clientDataJSON": "eyJ0eXBlIjoid2ViYXV0aG4uY3JlYXRlIiwiY2hhbGxlbmdlIjoiazlhUiJ9"
  },
  "clientExtensionResults": {}
}
```

**Response:** HTTP 200. `data` carries the new credential's id and name.

```json
{
  "success": true,
  "message": "Passkey registered successfully",
  "data": {
    "id": 7,
    "name": "YubiKey 5C"
  }
}
```

**Example**

```bash
curl -s -X POST "$BASE_URL/api/user/passkey/register/finish?name=YubiKey%205C" -b cookies.txt \
  -H "Content-Type: application/json" \
  -d @attestation.json
```

**Errors**

| Condition | Meaning |
|---|---|
| `WebAuthn not available` | Server WebAuthn (RP) configuration could not be initialised. |
| `no registration session found, please start again` | The begin step's session expired or is missing. |
| `registration failed ...` | Attestation verification failed. |
| `MFA enrollment is locked by administrator` | Account `password_locked` set. |

### DELETE /api/user/passkey/:id

Removes one of the authenticated user's passkey credentials. Deletion is scoped to the caller via `(id, user_id)`, so passing another user's credential id deletes nothing.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Path parameters**

| Name | Type | Required | Description |
|---|---|---|---|
| `id` | integer | Yes | Passkey credential row id (from `GET /api/user/passkey`). |

**Response:** HTTP 200.

```json
{
  "success": true,
  "message": "Passkey deleted successfully"
}
```

**Example**

```bash
curl -s -X DELETE "$BASE_URL/api/user/passkey/7" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Condition | Meaning |
|---|---|
| `invalid credential id` | The path `id` was not an integer. |

### PUT /api/user/passkey/:id

Renames one of the authenticated user's passkey credentials. Ownership is verified before the rename is applied.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Path parameters**

| Name | Type | Required | Description |
|---|---|---|---|
| `id` | integer | Yes | Passkey credential row id. |

**Request body**

| Field | JSON key | Type | Required | Default | Description |
|---|---|---|---|---|---|
| Name | `name` | string | Yes | — | New label, 1–128 characters (trimmed). |

```json
{
  "name": "Work Laptop Touch ID"
}
```

**Response:** HTTP 200.

```json
{
  "success": true,
  "message": "Passkey renamed successfully"
}
```

**Example**

```bash
curl -s -X PUT "$BASE_URL/api/user/passkey/7" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Work Laptop Touch ID"}'
```

**Errors**

| Condition | Meaning |
|---|---|
| `invalid credential id` | The path `id` was not an integer. |
| `name must be 1-128 characters` | Empty or over-length name. |
| `passkey not found ...` | The credential does not exist or is not owned by the caller. |

### GET /api/log/self

Lists the authenticated user's own request logs, paginated, with optional filters and sorting.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `p` | integer | No | 0 | Zero-based page index. Negative values are clamped to 0. |
| `size` | integer | No | server default | Page size; clamped to the server maximum (`<=0` falls back to the default). |
| `type` | integer | No | 0 | Log type filter (0 = all; e.g. 1 = top-up, 2 = consume, 3 = manage, 4 = system, 5 = test). |
| `model_name` | string | No | — | Filter by model name. |
| `token_name` | string | No | — | Filter by token name. |
| `start_timestamp` | integer (Unix sec) | No | 0 | Lower bound on log time. |
| `end_timestamp` | integer (Unix sec) | No | 0 | Upper bound on log time. |
| `sort` / `sort_by` | string | No | — | Sort column (`sort_by` preferred; `sort` is the fallback). e.g. `created_at`, `quota`, `prompt_tokens`, `elapsed_time`. |
| `order` / `sort_order` | string | No | `desc` | `asc` or `desc` (`order` overrides `sort_order`). |

> When a sort column is requested together with both timestamps, the date range may not exceed 30 days, otherwise the call returns `Date range for sorting cannot exceed 30 days`.

**Response:** HTTP 200. `data` is the log array; `total` is the unpaginated match count.

| Field | JSON key | Type | Description |
|---|---|---|---|
| Id | `id` | integer | Log row id. |
| User id | `user_id` | integer | Owning user id. |
| Type | `type` | integer | Log type. |
| Content | `content` | string | Human-readable description / billing note. |
| Username | `username` | string | Owner username. |
| Model name | `model_name` | string | Billed model. |
| Origin model name | `origin_model_name` | string | Client-requested model before mapping. |
| Token name | `token_name` | string | Token used. |
| Quota | `quota` | integer | Quota consumed (internal units). |
| Prompt/Completion tokens | `prompt_tokens` / `completion_tokens` | integer | Token counts. |
| Cached prompt tokens | `cached_prompt_tokens` | integer | Cached input tokens. |
| Channel | `channel` | integer | Channel id. |
| Request id | `request_id` | string | Per-request id (use with `/api/cost/request/:request_id`). |
| Trace id | `trace_id` | string | Trace id (use with `/api/trace/:trace_id`). |
| Elapsed time | `elapsed_time` | integer | Latency in ms. |
| Is stream | `is_stream` | boolean | Whether the request streamed. |
| Created at | `created_at` | integer | Log time (Unix sec). |

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 90211,
      "user_id": 42,
      "created_at": 1717000000,
      "type": 2,
      "content": "Model fee $0.0048",
      "username": "alice",
      "token_name": "default",
      "model_name": "gpt-4o-mini",
      "origin_model_name": "gpt-4o-mini",
      "quota": 2400,
      "prompt_tokens": 1500,
      "completion_tokens": 800,
      "channel": 3,
      "request_id": "2026060812000042-aBcD",
      "trace_id": "9f8b1c2d3e4f5a6b",
      "elapsed_time": 1320,
      "is_stream": true,
      "cached_prompt_tokens": 0
    }
  ],
  "total": 1
}
```

**Example**

```bash
curl -s "$BASE_URL/api/log/self?p=0&size=20&type=2&model_name=gpt-4o-mini" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/log/self/search

Full-text searches the authenticated user's own logs by keyword and returns paginated results.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `keyword` | string | No | — | Search term matched across log fields. |
| `p` | integer | No | 0 | Zero-based page index. Negative values are clamped to 0. |
| `size` | integer | No | server default | Page size; clamped to the server maximum. |
| `sort` | string | No | — | Sort column. |
| `order` | string | No | `desc` | `asc` or `desc`. |

**Response:** HTTP 200. Same log-row shape as `GET /api/log/self`; `total` is the match count.

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 90211,
      "user_id": 42,
      "created_at": 1717000000,
      "type": 2,
      "content": "Model fee $0.0048",
      "model_name": "gpt-4o-mini",
      "token_name": "default",
      "quota": 2400,
      "request_id": "2026060812000042-aBcD",
      "trace_id": "9f8b1c2d3e4f5a6b"
    }
  ],
  "total": 1
}
```

**Example**

```bash
curl -s "$BASE_URL/api/log/self/search?keyword=gpt-4o&size=20" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/log/self/stat

Returns the total quota consumed by the authenticated user across logs matching the supplied filters. The username is taken from the authenticated session, not from a query parameter.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| `type` | integer | No | 0 | Log type filter. |
| `model_name` | string | No | — | Filter by model name. |
| `token_name` | string | No | — | Filter by token name. |
| `channel` | integer | No | 0 | Filter by channel id. |
| `start_timestamp` | integer (Unix sec) | No | 0 | Lower time bound. |
| `end_timestamp` | integer (Unix sec) | No | 0 | Upper time bound. |

**Response:** HTTP 200. `data.quota` is the summed quota (internal units).

```json
{
  "success": true,
  "message": "",
  "data": {
    "quota": 124500
  }
}
```

**Example**

```bash
curl -s "$BASE_URL/api/log/self/stat?start_timestamp=1716000000&end_timestamp=1717000000" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/trace/log/:log_id

Returns the request trace associated with a specific log entry, resolved via the log's `trace_id`, and enriched with computed durations between trace milestones plus a summary of the originating log. There is no per-user ownership check on the log id beyond `UserAuth`.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Path parameters**

| Name | Type | Required | Description |
|---|---|---|---|
| `log_id` | integer | Yes | The log row id (the `id` field from `/api/log/self`). |

**Response:** HTTP 200. The success envelope omits `message`. `data` holds the trace, its parsed `timestamps`, derived `durations` (milliseconds between milestones), and the source `log` summary.

| Field (under `data`) | Type | Description |
|---|---|---|
| `id` | integer | Trace row id. |
| `trace_id` | string | Trace identifier. |
| `url` | string | Request URL. |
| `method` | string | HTTP method. |
| `body_size` | integer | Request body size in bytes. |
| `status` | integer | Final HTTP status. |
| `created_at` / `updated_at` | integer | Trace row create/update time (ms epoch). |
| `timestamps` | object | Milestone times: `request_received`, `request_forwarded`, `first_upstream_response`, `first_client_response`, `upstream_completed`, `request_completed`, plus optional `external_calls[]`. All fields are omitted when unset. |
| `durations` | object | Computed gaps (ms), each present only when both endpoints exist: `processing_time`, `upstream_response_time`, `response_processing_time`, `streaming_time`, `total_time`. |
| `log` | object | `id`, `user_id`, `username`, `content`, `type` of the source log. |

```json
{
  "success": true,
  "data": {
    "id": 5521,
    "trace_id": "9f8b1c2d3e4f5a6b",
    "url": "/v1/chat/completions",
    "method": "POST",
    "body_size": 412,
    "status": 200,
    "created_at": 1717000000000,
    "updated_at": 1717000001320,
    "timestamps": {
      "request_received": 1717000000000,
      "request_forwarded": 1717000000040,
      "first_upstream_response": 1717000000700,
      "first_client_response": 1717000000720,
      "upstream_completed": 1717000001300,
      "request_completed": 1717000001320
    },
    "durations": {
      "processing_time": 40,
      "upstream_response_time": 660,
      "response_processing_time": 20,
      "streaming_time": 580,
      "total_time": 1320
    },
    "log": {
      "id": 90211,
      "user_id": 42,
      "username": "alice",
      "content": "Model fee $0.0048",
      "type": 2
    }
  }
}
```

**Example**

```bash
curl -s "$BASE_URL/api/trace/log/90211" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Condition | Status / meaning |
|---|---|
| `invalid log_id parameter` | HTTP 400 — non-integer `log_id`. |
| `log not found` | HTTP 404 — no such log. |
| `no trace information available for this log entry` | HTTP 404 — the log has no `trace_id`. |
| `trace information not found` | HTTP 404 — the referenced trace is missing. |

### GET /api/trace/:trace_id

Returns the request trace for a given trace id directly (without computed durations or the log summary that the by-log variant adds).

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (or a web session cookie).

**Path parameters**

| Name | Type | Required | Description |
|---|---|---|---|
| `trace_id` | string | Yes | The trace identifier (the `trace_id` field on a log). |

**Response:** HTTP 200. The success envelope omits `message`. `data` holds the trace and its parsed `timestamps`.

```json
{
  "success": true,
  "data": {
    "id": 5521,
    "trace_id": "9f8b1c2d3e4f5a6b",
    "url": "/v1/chat/completions",
    "method": "POST",
    "body_size": 412,
    "status": 200,
    "created_at": 1717000000000,
    "updated_at": 1717000001320,
    "timestamps": {
      "request_received": 1717000000000,
      "request_forwarded": 1717000000040,
      "first_upstream_response": 1717000000700,
      "first_client_response": 1717000000720,
      "upstream_completed": 1717000001300,
      "request_completed": 1717000001320,
      "external_calls": [
        {
          "tool": "web_search",
          "source": "mcp",
          "server_label": "search",
          "started_at": 1717000000100,
          "ended_at": 1717000000400,
          "duration_ms": 300
        }
      ]
    }
  }
}
```

**Example**

```bash
curl -s "$BASE_URL/api/trace/9f8b1c2d3e4f5a6b" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Condition | Status / meaning |
|---|---|
| `trace_id parameter is required` | HTTP 400 — empty trace id. |
| `trace not found` | HTTP 404 — no trace with that id. |

### GET /api/cost/request/:request_id

Returns the recorded cost for a single relayed request, identified by its `request_id` (as found on a log entry). This route is registered under the `/api/cost` group with **no auth middleware**, and the handler performs **no credential or ownership check** — any caller who knows a valid `request_id` can read its cost; the `request_id` itself is the only access control. Unlike the other endpoints in this section, a successful response is the **raw cost object** and is *not* wrapped in the `{"success", "message", "data"}` envelope.

**Auth:** Public — no auth (knowledge of the `request_id` is required).

**Path parameters**

| Name | Type | Required | Description |
|---|---|---|---|
| `request_id` | string | Yes | The per-request identifier (stored with max length 32; the `request_id` field on a log). |

**Response:** HTTP 200. On success the body is the cost record itself.

| Field | JSON key | Type | Description |
|---|---|---|---|
| Id | `id` | integer | Cost row id. |
| Created time | `created_time` | integer | Record creation time (Unix sec). |
| User ID | `user_id` | integer | Owning user id. |
| Request ID | `request_id` | string | The request identifier. |
| Quota | `quota` | integer | Quota charged (internal units). |
| Cost (USD) | `cost_usd` | number | `quota / 500000`, i.e. the USD-equivalent cost (computed at read time; not stored). |
| Created at | `created_at` | integer | Insert time (ms epoch). |
| Updated at | `updated_at` | integer | Last update time (ms epoch). |

```json
{
  "id": 30144,
  "created_time": 1717000000,
  "user_id": 42,
  "request_id": "2026060812000042-aBcD",
  "quota": 2400,
  "cost_usd": 0.0048,
  "created_at": 1717000000000,
  "updated_at": 1717000000000
}
```

**Example**

```bash
curl -s "$BASE_URL/api/cost/request/2026060812000042-aBcD"
```

**Errors**

| Condition | Meaning |
|---|---|
| `failed to get cost by request id ...` | No cost record exists for that `request_id`. Returned in the management error envelope (`{"success": false, "message": ...}`, HTTP 200). |

> **Implementation note:** the handler does not early-return after writing an error body, so on a lookup failure it emits the error envelope and may then also attempt to encode an empty/zero cost object onto the same response. Treat a `success:false` body as "no cost record found".

---

**Source files used (absolute paths):** `/home/laisky/repo/laisky/one-api/router/api.go`, `/home/laisky/repo/laisky/one-api/controller/user.go` (GetSelf, UpdateSelf, DeleteSelf, GetUserDashboard, GetDashboardUsers, GenerateAccessToken, GetAffCode, TopUp, SetupTotp, ConfirmTotp, DisableTotp, GetTotpStatus), `/home/laisky/repo/laisky/one-api/controller/model.go` (GetUserAvailableModels), `/home/laisky/repo/laisky/one-api/controller/passkey.go`, `/home/laisky/repo/laisky/one-api/controller/log.go` (GetUserLogs, SearchUserLogs, GetLogsSelfStat), `/home/laisky/repo/laisky/one-api/controller/tracing.go`, `/home/laisky/repo/laisky/one-api/controller/token.go` (GetRequestCost), `/home/laisky/repo/laisky/one-api/common/helper/helper.go` (RespondError), `/home/laisky/repo/laisky/one-api/model/user.go`, `/home/laisky/repo/laisky/one-api/model/user_metadata.go`, `/home/laisky/repo/laisky/one-api/model/log.go`, `/home/laisky/repo/laisky/one-api/model/trace.go`, `/home/laisky/repo/laisky/one-api/model/cost.go`, `/home/laisky/repo/laisky/one-api/common/random/main.go`, `/home/laisky/repo/laisky/one-api/common/helper/time.go`.


## API Key (Token) Management

This section covers the management endpoints that create and administer **relay API keys** — the `sk-`-prefixed credentials your applications send to the inference endpoints (`/v1/chat/completions`, `/v1/responses`, `/v1/messages`, etc.). These are **not** the same as the management access token. To call any endpoint in this section you must authenticate as a logged-in user, using either a **management access token** (the 32-char UUID from `GET /api/user/token`, sent as `Authorization: $ACCESS_TOKEN`) or a browser session cookie. The relay API key these endpoints mint is a distinct, 48-character credential (16 random characters followed by a 32-character UUID-derived tail), returned with the configured prefix (default `sk-`). The two are easy to confuse: the **access token** lets you *manage* tokens via this REST API; the **relay API key** is what an end user puts in their OpenAI/Anthropic SDK to *make inference calls*. The full quick-start flow (mint a key, then use it) is at the end of `POST /api/token/`.

All routes below are registered under the `/api/token` group guarded by `UserAuth` (role >= common user, 1). Each operates only on tokens owned by the authenticated caller: `user_id` is always taken from the caller's identity, never from the request. Responses use the management envelope `{"success": bool, "message": string, "data": ...}` with HTTP 200; on failure the envelope is `{"success": false, "message": "<reason>"}`, also with HTTP 200. The `key` field, wherever a token object is serialized, is rewritten at response time by a custom `MarshalJSON` to strip any stored legacy prefix (`sk-`/`laisky-`) and apply the configured prefix — so it always comes back as `<prefix><48 chars>` (e.g. `sk-...`).

The `Token` object shape returned in `data` (verbatim JSON keys):

| JSON key | Type | Description |
| --- | --- | --- |
| `id` | int | Token ID (used in `/api/token/:id` paths). |
| `user_id` | int | Owner; always the caller. |
| `key` | string | The relay API key, serialized with the configured prefix (e.g. `sk-...`). |
| `status` | int | 1 = enabled, 2 = disabled, 3 = expired, 4 = exhausted. |
| `name` | string | Token name (max 30 chars). |
| `created_time` | int64 | Unix seconds. |
| `accessed_time` | int64 | Unix seconds, last use. |
| `expired_time` | int64 | Unix seconds; `-1` = never expires. |
| `remain_quota` | int64 | Remaining quota units (500000 = $1). |
| `unlimited_quota` | bool | If true, `remain_quota` is not enforced. |
| `used_quota` | int64 | Server-maintained; consumed quota units. |
| `created_at` | int64 | Unix milliseconds. |
| `updated_at` | int64 | Unix milliseconds. |
| `models` | string \| null | Comma-separated allow-list of model names; `null` = all models the user can access. |
| `subnet` | string \| null | Comma-separated CIDR allow-list; `null`/empty = no IP restriction. |

### GET /api/token/

Lists the calling user's tokens with pagination and sorting.

**Auth:** Management access token via `Authorization: $ACCESS_TOKEN` (a leading `Bearer ` is also accepted), or a session cookie.

**Query parameters**

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `p` | int | No | 0 | Zero-based page index; negative values are clamped to 0. |
| `size` | int | No | server default | Page size; values `<= 0` use the configured default, values above the configured max are clamped to the max. |
| `sort` | string | No | (none) | Sort field. One of `id`, `name`, `status`, `expired_time`, `remain_quota`, `used_quota`, `created_at`, `updated_at`. Unknown values fall back to `id desc`. |
| `order` | string | No | `desc` | Sort direction (`asc`/`desc`). Also used as a legacy single-field order (`remain_quota`, `used_quota`) when `sort` is empty. |

**Response**: HTTP 200. The envelope carries an extra top-level `total` (total token count for the user, for pagination). `data` is an array of token objects.

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 12,
      "user_id": 3,
      "key": "sk-abcd1234ABCD5678EFGH9012ijkl3456MNOP7890qrst1234",
      "status": 1,
      "name": "production",
      "created_time": 1717000000,
      "accessed_time": 1717200000,
      "expired_time": -1,
      "remain_quota": 500000,
      "unlimited_quota": false,
      "used_quota": 12500,
      "created_at": 1717000000000,
      "updated_at": 1717200000000,
      "models": null,
      "subnet": null
    }
  ],
  "total": 1
}
```

**Example**

```bash
curl -s "$BASE_URL/api/token/?p=0&size=20&sort=created_at&order=desc" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/token/search

Searches the calling user's tokens by name prefix.

**Auth:** Management access token via `Authorization: $ACCESS_TOKEN` (a leading `Bearer ` is also accepted), or a session cookie.

**Query parameters**

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `keyword` | string | No | (empty) | Token-name prefix to match (`name LIKE 'keyword%'`). Empty matches all. |
| `p` | int | No | 0 | Zero-based page index; negatives clamped to 0. |
| `size` | int | No | server default | Page size; `<= 0` uses default, over-max is clamped. |
| `sort` | string | No | (none) | Same fields as `GET /api/token/`. |
| `order` | string | No | `desc` | Sort direction. |

**Response**: HTTP 200, same shape as `GET /api/token/`. `data` is the matched array; `total` is the count of matches.

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 12,
      "user_id": 3,
      "key": "sk-abcd1234ABCD5678EFGH9012ijkl3456MNOP7890qrst1234",
      "status": 1,
      "name": "prod-eu",
      "created_time": 1717000000,
      "accessed_time": 1717200000,
      "expired_time": -1,
      "remain_quota": 500000,
      "unlimited_quota": false,
      "used_quota": 0,
      "created_at": 1717000000000,
      "updated_at": 1717000000000,
      "models": "gpt-4o,gpt-4o-mini",
      "subnet": null
    }
  ],
  "total": 1
}
```

**Example**

```bash
curl -s "$BASE_URL/api/token/search?keyword=prod&p=0&size=20" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/token/:id

Retrieves a single token owned by the caller.

**Auth:** Management access token via `Authorization: $ACCESS_TOKEN` (a leading `Bearer ` is also accepted), or a session cookie.

**Path parameters**

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | int | Yes | Token ID. The lookup is scoped to the caller's `user_id`; another user's token is treated as not found. |

**Response**: HTTP 200; `data` is a single token object.

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 12,
    "user_id": 3,
    "key": "sk-abcd1234ABCD5678EFGH9012ijkl3456MNOP7890qrst1234",
    "status": 1,
    "name": "production",
    "created_time": 1717000000,
    "accessed_time": 1717200000,
    "expired_time": -1,
    "remain_quota": 500000,
    "unlimited_quota": false,
    "used_quota": 12500,
    "created_at": 1717000000000,
    "updated_at": 1717200000000,
    "models": null,
    "subnet": null
  }
}
```

**Example**

```bash
curl -s "$BASE_URL/api/token/12" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status | Meaning |
| --- | --- |
| 200 with `success:false` | `id` not parseable as an integer, `id` is `0`, or no token with that `id` is owned by the caller (another user's token is treated as not found). The body carries `"message"` describing the failure. |

### POST /api/token/

Creates (mints) a new relay API key for the calling user. **This is the endpoint to use to create an `sk-` key.** The server generates the key value itself; you supply only metadata.

**Auth:** Management access token via `Authorization: $ACCESS_TOKEN` (a leading `Bearer ` is also accepted), or a session cookie. (Note: you authenticate here with the *access token*, and you receive back a separate *relay API key*.)

**Request body**

| Field | JSON key | Type | Required | Default | Description |
| --- | --- | --- | --- | --- | --- |
| Name | `name` | string | Yes | — | Display name. Must be non-empty (whitespace-only is rejected) and at most 30 characters. |
| Expiry | `expired_time` | int64 | No | `-1` | Unix seconds at which the key expires. Use `-1` for "never expires". To set a real expiry, send a future Unix-seconds timestamp. If omitted, the database default (`-1`, never expires) applies. |
| Remaining quota | `remain_quota` | int64 | No | `0` | Quota units available to this key (500000 = $1). Ignored for enforcement when `unlimited_quota` is true. |
| Unlimited | `unlimited_quota` | bool | No | `false` | If true, this key is not limited by `remain_quota` (it is still bounded by the owning user's quota). |
| Models allow-list | `models` | string \| null | No | `null` | Comma-separated list of model names this key may call (e.g. `"gpt-4o,gpt-4o-mini"`). `null` or omitted = no per-key model restriction. |
| Subnet allow-list | `subnet` | string \| null | No | `null` | Comma-separated CIDR list restricting which client IPs may use the key (e.g. `"10.0.0.0/8,192.168.0.0/16"`). Validated server-side; invalid CIDRs are rejected. `null`/empty = no IP restriction. |

Fields you cannot set: `user_id` is forced to the caller; `key` is server-generated; `status`, `used_quota`, `created_time`, `accessed_time`, `created_at`, `updated_at` are server-maintained. Any values you send for those are ignored.

```json
{
  "name": "production",
  "expired_time": -1,
  "remain_quota": 500000,
  "unlimited_quota": false,
  "models": "gpt-4o,gpt-4o-mini",
  "subnet": null
}
```

**Response**: HTTP 200. `data` is the full created token object, including the generated `key` with the configured prefix applied. The same `key` is also returned by later list/get responses, but treat creation as the natural moment to capture and store it.

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 27,
    "user_id": 3,
    "key": "sk-7Kp2mQ9xZ1aB3cD4ABCD1234EFGH5678IJKL9012MNOP3456",
    "status": 1,
    "name": "production",
    "created_time": 1717400000,
    "accessed_time": 1717400000,
    "expired_time": -1,
    "remain_quota": 500000,
    "unlimited_quota": false,
    "used_quota": 0,
    "created_at": 1717400000000,
    "updated_at": 1717400000000,
    "models": "gpt-4o,gpt-4o-mini",
    "subnet": null
  }
}
```

**Example (full quick-start: mint a key and use it)**

Step 1 — obtain a management access token (`GET /api/user/token` returns the freshly (re)generated 32-char UUID in `data`; this endpoint is itself guarded by `UserAuth`, so it requires an existing session cookie or a prior access token):

```bash
curl -s "$BASE_URL/api/user/token" \
  -H "Authorization: $ACCESS_TOKEN"
```

Step 2 — mint the relay API key (capture `data.key`, the `sk-` value):

```bash
curl -s -X POST "$BASE_URL/api/token/" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "production",
    "expired_time": -1,
    "remain_quota": 500000,
    "unlimited_quota": false,
    "models": null,
    "subnet": null
  }'
```

Step 3 — use the minted `sk-` key on the relay API (here `$API_KEY` is the `key` from step 2):

```bash
curl -s -X POST "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

**Key formatting and the admin channel suffix**

- The `sk-` prefix is applied at serialization, not stored, so the same key is recognized whether the client sends it with the configured prefix, the legacy `sk-`/`laisky-` prefix, or no prefix at all. On relay calls the key is also accepted via `X-Api-Key: <key>` (Anthropic-style) and `Api-Key: <key>` (Azure-style), in addition to `Authorization: Bearer <key>`.
- After prefix-stripping, the credential is split on `-`. A relay key may carry a trailing channel selector in the form `<prefix>{token}-{channelid}` (e.g. `sk-...3456-42`), which pins the request to a specific channel. This is an **admin-only** feature (role >= admin, 10): a non-admin sending a key with extra `-`-separated segments is rejected with **403** (`Ordinary users do not support specifying channels`). The generated key body itself contains no `-`, so ordinary keys are unaffected.

**Errors**

| Status | Meaning |
| --- | --- |
| 200 with `success:false` | `name` empty/whitespace; `name` longer than 30 chars; invalid `subnet` CIDR; malformed JSON body. The body carries `"message"` describing the failure. |

### PUT /api/token/

Updates an existing token owned by the caller. The token is identified by the `id` field in the body. By default this is a full-field update of editable fields; pass `?status_only=` (with any non-empty value) to change only the status.

**Auth:** Management access token via `Authorization: $ACCESS_TOKEN` (a leading `Bearer ` is also accepted), or a session cookie.

**Query parameters**

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `status_only` | string | No | (empty) | When present and non-empty, only the `status` field is applied; all other fields (including `name`) are ignored and the empty-name check is skipped. When absent/empty, a full editable-field update is performed and `name` must be non-empty. |

**Request body**

| Field | JSON key | Type | Required | Description |
| --- | --- | --- | --- | --- |
| Token ID | `id` | int | Yes | Which token to update; must belong to the caller. |
| Name | `name` | string | Required unless `status_only` | Max 30 chars; non-empty when not `status_only`. |
| Status | `status` | int | No | 1 = enabled, 2 = disabled, 3 = expired, 4 = exhausted. Re-enabling (`1`) is rejected if the token is still expired or still has no quota; conversely the server may auto-correct a status of exhausted/expired to enabled when the new quota/expiry makes it usable. |
| Expiry | `expired_time` | int64 | No | Unix seconds; `-1` = never. Applied only on full update. |
| Remaining quota | `remain_quota` | int64 | No | Quota units. Applied only on full update. |
| Unlimited | `unlimited_quota` | bool | No | Applied only on full update. |
| Models allow-list | `models` | string \| null | No | Applied only on full update. |
| Subnet allow-list | `subnet` | string \| null | No | Validated CIDR list; applied only on full update. |

`used_quota` is server-maintained and cannot be set here (the persisted update only writes `name`, `status`, `expired_time`, `remain_quota`, `unlimited_quota`, `models`, `subnet`). The relay key value cannot be changed.

```json
{
  "id": 27,
  "name": "production-renamed",
  "status": 1,
  "expired_time": -1,
  "remain_quota": 1000000,
  "unlimited_quota": false,
  "models": "gpt-4o,gpt-4o-mini",
  "subnet": "10.0.0.0/8"
}
```

**Response**: HTTP 200; `data` is the updated token object.

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 27,
    "user_id": 3,
    "key": "sk-7Kp2mQ9xZ1aB3cD4ABCD1234EFGH5678IJKL9012MNOP3456",
    "status": 1,
    "name": "production-renamed",
    "created_time": 1717400000,
    "accessed_time": 1717400000,
    "expired_time": -1,
    "remain_quota": 1000000,
    "unlimited_quota": false,
    "used_quota": 0,
    "created_at": 1717400000000,
    "updated_at": 1717450000000,
    "models": "gpt-4o,gpt-4o-mini",
    "subnet": "10.0.0.0/8"
  }
}
```

**Example (full update)**

```bash
curl -s -X PUT "$BASE_URL/api/token/" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "id": 27,
    "name": "production-renamed",
    "status": 1,
    "expired_time": -1,
    "remain_quota": 1000000,
    "unlimited_quota": false,
    "models": "gpt-4o,gpt-4o-mini",
    "subnet": "10.0.0.0/8"
  }'
```

**Example (status-only: disable a key)**

```bash
curl -s -X PUT "$BASE_URL/api/token/?status_only=1" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id": 27, "status": 2}'
```

**Errors**

| Status | Meaning |
| --- | --- |
| 200 with `success:false` | `name` empty when not `status_only`; `name` over 30 chars; invalid `subnet`; `id` not found for caller (or `id` is `0`); attempting to enable a token that is still expired or still has zero/depleted quota. The body carries `"message"` describing the failure. |

### DELETE /api/token/:id

Permanently deletes a token owned by the caller. The owner check prevents deleting another user's token.

**Auth:** Management access token via `Authorization: $ACCESS_TOKEN` (a leading `Bearer ` is also accepted), or a session cookie.

**Path parameters**

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | int | Yes | Token ID to delete; must belong to the caller. |

**Response**: HTTP 200. `data` is omitted; only `success` and `message` are returned.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -s -X DELETE "$BASE_URL/api/token/27" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status | Meaning |
| --- | --- |
| 200 with `success:false` | `id` not found for the caller (including when it belongs to another user), or `id` is `0`/unparseable. The body carries `"message"` describing the failure. |


## User Administration & Top-up

These endpoints comprise the administrative slice of the management REST API for inspecting, creating, mutating, and crediting user accounts. Every route in this section requires an administrator credential (role >= 10) enforced by `AdminAuth`; root-only constraints (role 100) are applied per-action inside the handlers. Send the management **access token** as `Authorization: $ACCESS_TOKEN` (a leading `Bearer ` is tolerated), or rely on a browser **session cookie** from `POST /api/user/login`; the relay API key (`sk-...`) is not accepted here. All responses use the management envelope `{"success": bool, "message": string, "data": <payload>}` with HTTP 200, including error cases (controllers respond through `helper.RespondError`, which always writes HTTP 200 with `{"success": false, "message": "<reason>"}`). Quota values are internal integers where 500000 quota = 1 USD.

A cross-cutting permission rule applies to most handlers: an admin may only act on users strictly below their own role. An admin (role 10) cannot read, update, delete, or otherwise manage another admin or a root user; only a root user (role 100) may act on peers at the same level. Violations return `{"success": false}` with an explanatory message. (The one exception is `DELETE /api/user/:id`, which checks `caller role > target role` with no root self-exemption.)

> **Token exposure note:** the list (`GET /api/user/`) and search (`GET /api/user/search`) responses omit only the `password` column, so each user object carries its populated 32-char `access_token` (the management credential). The single-user reads (`GET /api/user/:id`) omit both `password` and `access_token`. `totp_secret` is omitted from JSON whenever empty.

### GET /api/user/

Lists user accounts (excluding soft-deleted ones) with pagination and optional sorting. Use it to render the admin user table.

**Auth:** Management access token - `Authorization: $ACCESS_TOKEN` (AdminAuth, role >= 10).

**Query parameters**

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `p` | integer | No | 0 | Zero-based page index. Negative values are clamped to 0. The DB offset is `p * size`. |
| `size` | integer | No | 10 | Page size. Values <= 0 fall back to the default (`DefaultItemsPerPage`, 10); values above the server cap (`MaxItemsPerPage`, default 100) are clamped to the cap. |
| `sort` | string | No | (none) | Column to sort by. Allowed: `id`, `username`, `email`, `display_name`, `role`, `status`, `quota`, `used_quota`, `request_count`, `created_at`, `updated_at`. Unknown values fall back to `id desc`. |
| `order` | string | No | `desc` | Sort direction (`asc`/`desc`), used together with `sort`. When `sort` is empty this same value is also interpreted as a legacy sort key: `quota`, `used_quota`, or `request_count` (each sorted descending); any other value (including the `desc` default) orders by `id desc`. |

**Response**: HTTP 200. `data` is an array of user objects (the `password` column is omitted; `access_token` is present). A top-level `total` field carries the total non-deleted user count for pagination.

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 1,
      "username": "root",
      "display_name": "Root",
      "role": 100,
      "status": 1,
      "email": "root@example.com",
      "github_id": "",
      "wechat_id": "",
      "lark_id": "",
      "oidc_id": "",
      "access_token": "1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d",
      "quota": 500000,
      "used_quota": 12000,
      "request_count": 42,
      "group": "default",
      "aff_code": "a1b2",
      "inviter_id": 0,
      "mcp_tool_blacklist": null,
      "metadata": {},
      "created_at": 1716200000000,
      "updated_at": 1716300000000
    }
  ],
  "total": 1
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/user/?p=0&size=20&sort=quota&order=desc" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/user/search

Searches users by a single keyword across id, username, email, and display name. Use it for the admin search box. Returns all matches (no pagination).

**Auth:** Management access token - `Authorization: $ACCESS_TOKEN` (AdminAuth, role >= 10).

**Query parameters**

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `keyword` | string | No | (empty) | Search term. On non-PostgreSQL backends it matches an exact `id = keyword` OR a prefix (`LIKE keyword%`) of `username`/`email`/`display_name`; on PostgreSQL only the prefix match on those three text columns applies. An empty keyword matches by the same rules (prefix `%` effectively returns all). |
| `sort` | string | No | (none) | Sort column, same allowed set as `GET /api/user/`. Invalid values fall back to `id desc`. |
| `order` | string | No | `desc` | Sort direction (`asc`/`desc`). |

**Response**: HTTP 200. `data` is an array of user objects (same shape as `GET /api/user/`: `password` omitted, `access_token` present). No `total` field.

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 7,
      "username": "alice",
      "display_name": "Alice",
      "role": 1,
      "status": 1,
      "email": "alice@example.com",
      "access_token": "0f1e2d3c4b5a69788796a5b4c3d2e1f0",
      "quota": 500000,
      "used_quota": 0,
      "request_count": 0,
      "group": "default",
      "aff_code": "z9y8",
      "inviter_id": 0,
      "mcp_tool_blacklist": null,
      "metadata": {},
      "created_at": 1716200000000,
      "updated_at": 1716200000000
    }
  ]
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/user/search?keyword=alice&sort=created_at&order=desc" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/user/:id

Fetches a single user by numeric id. Use it to load a user detail view.

**Auth:** Management access token - `Authorization: $ACCESS_TOKEN` (AdminAuth, role >= 10).

**Path parameters**

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | integer | Yes | Target user id. |

**Response**: HTTP 200. `data` is the user object (`password` and `access_token` both omitted).

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 7,
    "username": "alice",
    "display_name": "Alice",
    "role": 1,
    "status": 1,
    "email": "alice@example.com",
    "quota": 500000,
    "used_quota": 0,
    "request_count": 0,
    "group": "default",
    "aff_code": "z9y8",
    "inviter_id": 0,
    "mcp_tool_blacklist": null,
    "metadata": {},
    "created_at": 1716200000000,
    "updated_at": 1716200000000
  }
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/user/7" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Condition | Envelope |
| --- | --- |
| `id` is not an integer, or no such user | `{"success": false, "message": "..."}` (HTTP 200) |
| Caller role <= target role and caller is not root | `{"success": false, "message": "No permission to get information of users at the same level or higher"}` |

### POST /api/user/

Creates a new user account. Use it to provision users administratively.

**Auth:** Management access token - `Authorization: $ACCESS_TOKEN` (AdminAuth, role >= 10).

**Request body**

| Field | JSON key | Type | Required | Default | Description |
| --- | --- | --- | --- | --- | --- |
| Username | `username` | string | Yes | - | Login identity. Must be non-empty; validated to max 30 characters. |
| Password | `password` | string | Yes | - | Plaintext password; validated to 8-20 characters and stored hashed. |
| Display name | `display_name` | string | No | = `username` | Validated to max 20 characters; if omitted, defaults to the username. Must not be whitespace-only if provided. |
| Email | `email` | string | No | "" | Optional email; validated to max 50 characters. |
| Quota | `quota` | integer | No | account default | Applied as an override after creation only when > 0 (the create path first resets quota to the new-user default, then applies this). |
| Group | `group` | string | No | `default` | Applied as an override after creation only when non-empty. |
| Role | `role` | integer | No | 1 | Used only to validate against the caller: rejected if `role >= caller role`. The created account's role is NOT set from this field. |

The created account's role is not set from `role` in the body; the field is only used to reject requests where `role >= caller role`. To assign a higher role after creation, use `PUT /api/user/` or `POST /api/user/manage`.

```json
{
  "username": "bob",
  "password": "s3cretpass",
  "display_name": "Bob",
  "email": "bob@example.com",
  "quota": 1000000,
  "group": "vip"
}
```

**Response**: HTTP 200, no `data` payload.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -sS -X POST "$BASE_URL/api/user/" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username":"bob","password":"s3cretpass","display_name":"Bob","email":"bob@example.com","quota":1000000,"group":"vip"}'
```

**Errors**

| Condition | Envelope |
| --- | --- |
| Missing/empty `username` or `password`, or malformed body | `{"success": false, "message": "invalid parameter"}` |
| Validation failure (length constraints) | `{"success": false, "message": "Invalid input, please check your input"}` |
| Whitespace-only username / display name | `{"success": false, "message": "Username cannot be empty"}` / `"Display name cannot be empty if provided"` |
| `role` >= caller role | `{"success": false, "message": "Unable to create users with permissions greater than or equal to your own"}` |

### POST /api/user/manage

Performs a single lifecycle action on a user identified by username: enable, disable, promote, demote, or delete. Use it for the per-row admin action buttons.

**Auth:** Management access token - `Authorization: $ACCESS_TOKEN` (AdminAuth, role >= 10).

**Request body**

| Field | JSON key | Type | Required | Default | Description |
| --- | --- | --- | --- | --- | --- |
| Username | `username` | string | Yes | - | Username of the target user. |
| Action | `action` | string | Yes | - | One of `enable`, `disable`, `promote`, `demote`, `delete`. |

Action semantics:
- `enable` -> sets status to 1 (enabled).
- `disable` -> sets status to 2 (disabled); rejected for a root user.
- `promote` -> sets role to admin (10); root-only operation; rejected if the user is already admin or higher.
- `demote` -> sets role to common user (1); rejected for a root user or a user who is already a common user.
- `delete` -> soft-deletes the user; rejected for a root user.

```json
{
  "username": "bob",
  "action": "disable"
}
```

**Response**: HTTP 200. `data` echoes the resulting role and status (a `User` object where only `role` and `status` are set; all other fields are zero-valued/omitted).

```json
{
  "success": true,
  "message": "",
  "data": {
    "role": 1,
    "status": 2
  }
}
```

**Example**

```bash
curl -sS -X POST "$BASE_URL/api/user/manage" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username":"bob","action":"disable"}'
```

**Errors**

| Condition | Envelope |
| --- | --- |
| Malformed body | `{"success": false, "message": "invalid parameter"}` |
| Unknown username | `{"success": false, "message": "User does not exist"}` |
| Caller role <= target role and caller not root | `{"success": false, "message": "No permission to update user information with the same permission level or higher permission level"}` |
| `disable`/`delete`/`demote` on a root user | `{"success": false, "message": "Unable to disable super administrator user"}` / `"Unable to delete super administrator user"` / `"Unable to downgrade super administrator user"` |
| `promote` by a non-root admin | `{"success": false, "message": "Ordinary administrator users cannot promote other users to administrators"}` |
| `promote` of an existing admin | `{"success": false, "message": "The user is already an administrator"}` |
| `demote` of a common user | `{"success": false, "message": "The user is already an ordinary user"}` |

### PUT /api/user/

Partially updates an existing user. Only fields present in the JSON body are changed; absent fields and explicit `null` values are left untouched (except `mcp_tool_blacklist`, where explicit `null` clears the list). Use it for the admin edit form.

**Auth:** Management access token - `Authorization: $ACCESS_TOKEN` (AdminAuth, role >= 10).

**Request body**

| Field | JSON key | Type | Required | Default | Description |
| --- | --- | --- | --- | --- | --- |
| Id | `id` | integer | Yes | - | Target user id. Must be non-zero. |
| Username | `username` | string | No | unchanged | Trimmed; must be non-empty and 3-30 characters. Renaming a user at the same or higher level is forbidden unless caller is root. Explicit `null` is rejected. |
| Display name | `display_name` | string | No | unchanged | Trimmed; max 20 characters. `null` => no change. |
| Email | `email` | string | No | unchanged | Trimmed; max 50 characters; validated as an email when non-empty. `null` => no change. |
| Password | `password` | string | No | unchanged | Trimmed; non-empty; 8-20 characters; stored hashed. Rejected if the target user is password-locked. `null` => no change. |
| Group | `group` | string | No | unchanged | Trimmed; non-empty; max 32 characters. `null` => no change. |
| Quota | `quota` | integer | No | unchanged | Must be non-negative. A change is recorded in the management log. `null` => no change. |
| Role | `role` | integer | No | unchanged | Cannot set a role >= caller's own unless caller is root. `null` => no change. |
| Status | `status` | integer | No | unchanged | One of 1 (enabled), 2 (disabled), 3 (deleted). Disabling bans the user; enabling unbans. `null` => no change. |
| MCP tool blacklist | `mcp_tool_blacklist` | string array | No | unchanged | Explicit `null` clears the list; an array replaces it. |
| Metadata | `metadata` | object | No | unchanged | Object with `password_locked` (boolean), merged into existing metadata. Changing `password_locked` is root-only. `null` => no change. |

If the body contains only `id` (no mutable fields, i.e. no field produced an update), the call is a no-op success.

```json
{
  "id": 7,
  "display_name": "Alice Smith",
  "quota": 2000000,
  "status": 1,
  "group": "vip"
}
```

**Response**: HTTP 200, no `data` payload.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -sS -X PUT "$BASE_URL/api/user/" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":7,"display_name":"Alice Smith","quota":2000000,"status":1,"group":"vip"}'
```

**Errors**

| Condition | Envelope |
| --- | --- |
| Malformed body or `id` == 0 | `{"success": false, "message": "invalid parameter"}` |
| No such user | `{"success": false, "message": "..."}` |
| Caller role <= target role and caller not root | `{"success": false, "message": "No permission to update user information with the same permission level or higher permission level"}` |
| Explicit `null` username | `{"success": false, "message": "Username cannot be null"}` |
| Rename of same/higher-level user by non-root | `{"success": false, "message": "No permission to rename this user"}` |
| `quota` < 0 | `{"success": false, "message": "Quota must be non-negative"}` |
| `status` not in {1,2,3} | `{"success": false, "message": "Invalid status provided"}` |
| `role` >= caller role (non-root) | `{"success": false, "message": "No permission to promote other users to a permission level greater than or equal to your own"}` |
| `password` set while user is password-locked | `{"success": false, "message": "Password is locked for this user"}` |
| `metadata.password_locked` changed by a non-root admin | `{"success": false, "message": "Only root admin can change password lock"}` |

### DELETE /api/user/:id

Soft-deletes a user by id. Use it to remove an account from the admin user list.

**Auth:** Management access token - `Authorization: $ACCESS_TOKEN` (AdminAuth, role >= 10).

**Path parameters**

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | integer | Yes | Target user id. |

**Response**: HTTP 200, no `data` payload.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -sS -X DELETE "$BASE_URL/api/user/7" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Condition | Envelope |
| --- | --- |
| `id` is not an integer, or no such user | `{"success": false, "message": "..."}` |
| Caller role <= target role | `{"success": false, "message": "No permission to delete users with the same permission level or higher permission level"}` (this path requires the caller's role to be strictly greater than the target's, with NO root exemption) |

### POST /api/user/totp/disable/:id

Disables (clears) the TOTP/2FA secret for the target user. Use it to recover a user locked out of their authenticator. The action is recorded in the management log.

**Auth:** Management access token - `Authorization: $ACCESS_TOKEN` (AdminAuth, role >= 10).

**Path parameters**

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | integer | Yes | Target user id. |

**Response**: HTTP 200, no `data` payload.

```json
{
  "success": true,
  "message": "TOTP has been successfully disabled for the user"
}
```

**Example**

```bash
curl -sS -X POST "$BASE_URL/api/user/totp/disable/7" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Condition | Envelope |
| --- | --- |
| `id` is not an integer | `{"success": false, "message": "Invalid user ID"}` |
| No such user | `{"success": false, "message": "..."}` |
| Caller role <= target role and caller not root | `{"success": false, "message": "No permission to modify user with the same or higher permission level"}` |
| Target has no TOTP enabled | `{"success": false, "message": "TOTP is not enabled for this user"}` |

### POST /api/topup

Credits (increases) a user's quota balance administratively and records a top-up log entry. Use it to grant quota without a redemption code. Quota is an internal integer (500000 = 1 USD). Note this is the admin route registered directly at `/api/topup`; it is distinct from the self-service `POST /api/user/topup` (redemption-code redemption).

**Auth:** Management access token - `Authorization: $ACCESS_TOKEN` (AdminAuth, role >= 10).

**Request body**

| Field | JSON key | Type | Required | Default | Description |
| --- | --- | --- | --- | --- | --- |
| User id | `user_id` | integer | Yes | - | Target user whose quota is increased. |
| Quota | `quota` | integer | Yes | - | Amount of quota to add (internal units). |
| Remark | `remark` | string | No | auto-generated | Note stored on the top-up log. If empty, defaults to a generated string like `Recharged via API $X.XX`. |

```json
{
  "user_id": 7,
  "quota": 500000,
  "remark": "Manual grant for Q2 promo"
}
```

**Response**: HTTP 200, no `data` payload.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -sS -X POST "$BASE_URL/api/topup" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id":7,"quota":500000,"remark":"Manual grant for Q2 promo"}'
```

**Errors**

| Condition | Envelope |
| --- | --- |
| Malformed body / bind failure | `{"success": false, "message": "..."}` |
| Quota increase fails (e.g. no such user) | `{"success": false, "message": "..."}` |


## Channel Administration & Diagnostics

This section documents the management endpoints that create, inspect, test, price, and repair upstream provider channels. Every route here is mounted under `/api/channel` or `/api/debug` and is protected by `AdminAuth` (role >= 10): pass the management **access token** as `Authorization: $ACCESS_TOKEN` (a leading `Bearer ` is also accepted), or rely on a logged-in session cookie. These are management-API endpoints, so most use the management envelope (`{"success": ..., "message": ..., "data": ...}` with HTTP 200); the few that deviate (`GET /api/channel/models`, `GET /api/channel/test/:id`, `GET /api/channel/update_balance/:id`) are called out explicitly. A channel record is a large object; the create/update entries below document the operationally important fields and defer to `docs/manuals/channels.md` for the exhaustive schema. Channel `status` values are: `1` = enabled, `2` = manually disabled, `3` = auto-disabled. Quota fields are internal integers (500000 quota = 1 USD).

Note on HTTP status: the channel and most debug handlers report errors via `helper.RespondError`, which returns HTTP 200 with `{"success": false, "message": ...}`. The debug handlers that use `helper.RespondErrorWithStatus` (the `/api/debug/*` routes) return real 4xx/5xx codes; those are noted per endpoint.

### GET /api/channel/

Lists channel records with pagination and optional sorting. Secret fields (keys) are returned in masked/limited form.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `p` | integer | No | 0 | Zero-based page index. Negative values are clamped to 0. |
| `size` | integer | No | 10 | Page size. Values <= 0 fall back to the default (`DefaultItemsPerPage`); capped at `MaxItemsPerPage` (default 100). |
| `sort` | string | No | (none) | Column to sort by (e.g. `id`, `priority`, `name`). Empty means default DB ordering. |
| `order` | string | No | `desc` | Sort direction: `asc` or `desc`. Empty falls back to `desc`. |

**Response**: HTTP 200. `data` is an array of channel objects; `total` is the full unpaginated count.

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 12,
      "type": 1,
      "key": "",
      "status": 1,
      "name": "OpenAI Prod",
      "weight": 0,
      "created_time": 1717000000,
      "test_time": 1717100000,
      "response_time": 432,
      "base_url": "https://api.openai.com",
      "balance": 0,
      "balance_updated_time": 0,
      "models": "gpt-4o,gpt-4o-mini",
      "group": "default",
      "used_quota": 1500000,
      "priority": 10,
      "ratelimit": 0,
      "testing_model": "gpt-4o-mini"
    }
  ],
  "total": 37
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/channel/?p=0&size=20&sort=priority&order=desc" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/channel/search

Performs a keyword search across channels (name, models, key fragment, etc.) and returns all matches without pagination.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `keyword` | string | No | (empty) | Search term. Empty keyword returns the unfiltered set. |
| `sort` | string | No | (none) | Column to sort by. |
| `order` | string | No | `desc` | Sort direction: `asc` or `desc`. Empty falls back to `desc`. |

**Response**: HTTP 200. `data` is an array of channel objects (same shape as `GET /api/channel/`, no `total`).

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 12,
      "type": 1,
      "name": "OpenAI Prod",
      "status": 1,
      "models": "gpt-4o,gpt-4o-mini",
      "group": "default",
      "priority": 10
    }
  ]
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/channel/search?keyword=openai&order=asc" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/channel/models

Returns every model known across all enabled channels, in the OpenAI `GET /v1/models` list shape. This is the admin catalog view and ignores per-user permissions. Note: unlike the other endpoints in this section, the response is NOT wrapped in the management envelope; it is the bare OpenAI list object. On internal failure it returns HTTP 500 (via `middleware.AbortWithError`).

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Response**: HTTP 200. Top-level `object` is `"list"`; `data` is an array of model descriptors.

```json
{
  "object": "list",
  "data": [
    {
      "id": "gpt-4o",
      "object": "model",
      "created": 1626777600,
      "owned_by": "openai",
      "permission": [],
      "root": "gpt-4o",
      "parent": null
    }
  ]
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/channel/models" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/channel/metadata

Returns server-side metadata for a given channel type: the default base URL, whether the base URL is user-editable, the default supported endpoints, and the catalog of all available endpoints. Used by the UI to drive the channel-edit form.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `type` | integer | Yes | — | Channel type code (e.g. `1` = OpenAI). |

**Response**: HTTP 200.

| Field | Type | Description |
|-------|------|-------------|
| `data.default_base_url` | string | Default upstream base URL for the type (may be empty). |
| `data.base_url_editable` | bool | Whether the operator may override the base URL. |
| `data.default_endpoints` | string[] | Endpoint names enabled by default for the type. |
| `data.all_endpoints` | object[] | All endpoints, each `{id, name, description, path}`. The `id` is the relay-mode integer (e.g. `chat_completions` = 1, `claude_messages` = 14). |

```json
{
  "success": true,
  "message": "",
  "data": {
    "default_base_url": "https://api.openai.com",
    "base_url_editable": true,
    "default_endpoints": [
      "chat_completions",
      "completions",
      "embeddings",
      "moderations",
      "images_generations",
      "images_edits",
      "audio_speech",
      "audio_transcription",
      "audio_translation",
      "response_api",
      "claude_messages",
      "realtime",
      "videos"
    ],
    "all_endpoints": [
      {
        "id": 1,
        "name": "chat_completions",
        "description": "Chat Completions API",
        "path": "/v1/chat/completions"
      },
      {
        "id": 14,
        "name": "claude_messages",
        "description": "Claude Messages API",
        "path": "/v1/messages"
      }
    ]
  }
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/channel/metadata?type=1" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status | Meaning |
|--------|---------|
| 200 `{"success": false, "message": "type is required"}` | `type` query parameter omitted/empty. |
| 200 `{"success": false, "message": "invalid type"}` | `type` is not an integer. |

### GET /api/channel/:id

Retrieves one channel by ID. Secret fields are masked; if a tooling config exists it is serialized into a `tooling` string field.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | integer | Yes | Channel ID. |

**Response**: HTTP 200. `data` is a channel object (see `docs/manuals/channels.md` for the full field list) plus an optional `tooling` JSON string (present only when a tooling config exists).

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 12,
    "type": 1,
    "key": "",
    "status": 1,
    "name": "OpenAI Prod",
    "base_url": "https://api.openai.com",
    "models": "gpt-4o,gpt-4o-mini",
    "model_mapping": "{\"gpt-4\":\"gpt-4o\"}",
    "model_configs": "{\"gpt-4o\":{\"ratio\":2.5,\"completion_ratio\":4}}",
    "group": "default",
    "priority": 10,
    "weight": 0,
    "ratelimit": 0,
    "testing_model": "gpt-4o-mini",
    "tooling": "{\"whitelist\":[\"web_search\"]}"
  }
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/channel/12" \
  -H "Authorization: $ACCESS_TOKEN"
```

### POST /api/channel/

Creates one or more channels from a posted channel object. The `key` field is split on newlines and one channel is inserted per non-empty key (all other fields shared); empty key segments are skipped.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Request body**: a channel object. Key fields below; the full schema is in `docs/manuals/channels.md`.

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Name | `name` | string | Yes | — | Human-readable label. Must be non-empty (after trimming). |
| Type | `type` | integer | No | 0 | Provider type code (e.g. `1` = OpenAI). |
| Key | `key` | string | No | "" | Upstream credential. Newline-separated to bulk-create multiple channels. |
| Base URL | `base_url` | string | No | type default | Upstream origin; auto-filled from the type default when blank. |
| Models | `models` | string | No | "" | Comma-separated model names served by the channel. |
| Group | `group` | string | No | `default` | User group(s) this channel serves. |
| Model mapping | `model_mapping` | string (JSON) | No | null | JSON map of inbound model name → upstream model name. |
| Model configs | `model_configs` | string (JSON) | No | null | Unified per-model pricing/config JSON. |
| Priority | `priority` | integer | No | 0 | Selection priority (higher = preferred). |
| Weight | `weight` | integer | No | 0 | Load-balancing weight within a priority tier. |
| Status | `status` | integer | No | 1 | `1` enabled, `2` manually disabled, `3` auto-disabled. |
| Testing model | `testing_model` | string | No | null | Model used for health checks; cleared at create time if not in `models`. |
| Inference profile ARN map | `inference_profile_arn_map` | string (JSON) | No | null | AWS Bedrock model→ARN map; validated when non-empty. |
| Tooling | `tooling` | string or object | No | null | Channel tooling policy (whitelist/pricing). Accepts a JSON object or a JSON string. |

```json
{
  "name": "OpenAI Prod",
  "type": 1,
  "key": "sk-upstreamkeyA\nsk-upstreamkeyB",
  "base_url": "https://api.openai.com",
  "models": "gpt-4o,gpt-4o-mini",
  "group": "default",
  "model_mapping": "{\"gpt-4\":\"gpt-4o\"}",
  "model_configs": "{\"gpt-4o\":{\"ratio\":2.5,\"completion_ratio\":4}}",
  "priority": 10,
  "weight": 0,
  "status": 1,
  "testing_model": "gpt-4o-mini"
}
```

**Response**: HTTP 200. No `data` payload on success.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -sS -X POST "$BASE_URL/api/channel/" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"OpenAI Prod","type":1,"key":"sk-upstreamkey","base_url":"https://api.openai.com","models":"gpt-4o,gpt-4o-mini","group":"default","priority":10,"status":1}'
```

**Errors**

| Status | Meaning |
|--------|---------|
| 200 `{"success": false, "message": "Channel name is required"}` | Blank `name`. |
| 200 `{"success": false, "message": "Invalid inference profile ARN map: ..."}` | `inference_profile_arn_map` is not valid JSON. |
| 200 `{"success": false, "message": "Invalid tooling config: ..."}` | `tooling` payload cannot be parsed. |

### POST /api/channel/:id/duplicate

Clones an existing channel server-side. The duplicate copies configuration and credentials but resets identity and usage fields (id, timestamps, balance, used quota) and appends ` Copy` to the name (a blank source name becomes `Channel Copy`).

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | integer | Yes | Source channel ID. |

**Response**: HTTP 200. `data` contains the new channel's `id` and `name`.

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 41,
    "name": "OpenAI Prod Copy"
  }
}
```

**Example**

```bash
curl -sS -X POST "$BASE_URL/api/channel/12/duplicate" \
  -H "Authorization: $ACCESS_TOKEN"
```

### PUT /api/channel/

Updates a channel. Two modes: a full update (default), or a status-only update when the `status_only` query flag is set (safer, touches only the `status` column).

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `status_only` | string | No | (empty) | When non-empty, only `id` and `status` are applied; all other fields are ignored. |

**Request body**: a channel object including `id`. For a full update the same fields as `POST /api/channel/` apply, and `name` must be non-empty. For a status-only update, only `id` and `status` are required. Note: `inference_profile_arn_map` (when non-empty) is validated before the `status_only` branch, so an invalid ARN map is rejected even in status-only mode.

```json
{
  "id": 12,
  "name": "OpenAI Prod",
  "type": 1,
  "base_url": "https://api.openai.com",
  "models": "gpt-4o,gpt-4o-mini,o3-mini",
  "group": "default",
  "priority": 20,
  "status": 1,
  "testing_model": "gpt-4o-mini"
}
```

**Response**: HTTP 200. A full update returns the updated channel object in `data` (with optional `tooling` string); a status-only update returns no `data`.

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 12,
    "name": "OpenAI Prod",
    "type": 1,
    "status": 1,
    "models": "gpt-4o,gpt-4o-mini,o3-mini",
    "priority": 20
  }
}
```

**Example**

```bash
# Full update
curl -sS -X PUT "$BASE_URL/api/channel/" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":12,"name":"OpenAI Prod","type":1,"models":"gpt-4o,gpt-4o-mini","group":"default","priority":20,"status":1}'

# Status-only update (disable channel 12)
curl -sS -X PUT "$BASE_URL/api/channel/?status_only=1" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":12,"status":2}'
```

**Errors**

| Status | Meaning |
|--------|---------|
| 200 `{"success": false, "message": "Channel id is required"}` | Status-only mode with `id` of 0. |
| 200 `{"success": false, "message": "Channel name cannot be empty"}` | Full update with blank `name`. |
| 200 `{"success": false, "message": "Invalid inference profile ARN map: ..."}` | Bad `inference_profile_arn_map` JSON. |
| 200 `{"success": false, "message": "Invalid tooling config: ..."}` | `tooling` payload cannot be parsed (full update only). |

### DELETE /api/channel/:id

Deletes a single channel by ID.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | integer | Yes | Channel ID to delete. |

**Response**: HTTP 200. No `data`.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -sS -X DELETE "$BASE_URL/api/channel/12" \
  -H "Authorization: $ACCESS_TOKEN"
```

### DELETE /api/channel/disabled

Deletes all channels currently in a disabled state (manually or auto-disabled) and returns how many rows were removed.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Response**: HTTP 200. `data` is the number of deleted rows.

```json
{
  "success": true,
  "message": "",
  "data": 4
}
```

**Example**

```bash
curl -sS -X DELETE "$BASE_URL/api/channel/disabled" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/channel/test

Starts a background test sweep across a set of channels. Returns immediately; results are applied asynchronously (channels may be auto-disabled or re-enabled depending on server configuration, e.g. `AutomaticDisableChannelEnabled`). Only one sweep runs at a time.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `scope` | string | No | `all` | Which channels to test. Empty falls back to `all`; passed through to the channel selector. |

**Response**: HTTP 200 acknowledging the sweep started. No `data`.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/channel/test?scope=all" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status | Meaning |
|--------|---------|
| 200 `{"success": false, "message": "Test is already running"}` | A sweep is already in progress. |

### GET /api/channel/test/:id

Synchronously runs one live chat-completion request against a single channel to verify availability, and returns the upstream reply text plus elapsed time. Records a test log and updates the channel's stored response time.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | integer | Yes | Channel ID to test. |

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `model` | string | No | (stored testing model, else cheapest supported) | Model to send the probe request to. Trimmed; falls back to `testing_model` (if still supported) then the channel's cheapest supported model. |

**Response**: HTTP 200. Note: this endpoint does not use the standard `data` envelope; it returns flat fields. On success `message` holds the upstream reply text; on failure `success` is `false` and `message` holds the error, with `time` set to 0.

| Field | Type | Description |
|-------|------|-------------|
| `success` | bool | Whether the probe succeeded. |
| `message` | string | Reply text on success, or error message on failure. |
| `time` | number | Elapsed seconds (0 on failure). |
| `modelName` | string | The model actually used for the probe. |

```json
{
  "success": true,
  "message": "Hello! How can I help you today?",
  "time": 0.842,
  "modelName": "gpt-4o-mini"
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/channel/test/12?model=gpt-4o-mini" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/channel/update_balance

Triggers a refresh of all channel balances. Note: in the current implementation the synchronous body is disabled (commented out), so this returns success immediately without performing the refresh inline; the periodic background updater handles balances.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Response**: HTTP 200. No `data`.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/channel/update_balance" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/channel/update_balance/:id

Refreshes the remaining balance for one channel by querying the upstream provider's billing API, then returns the resolved balance in USD. Only certain provider types support balance queries (OpenAI and OpenAI-compatible / Custom, CloseAI, OpenAI-SB, AIProxy, API2GPT, AIGC2D, SiliconFlow, DeepSeek, OpenRouter); others (e.g. Azure) return "Not yet implemented".

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | integer | Yes | Channel ID. |

**Response**: HTTP 200. Note: the balance is returned in a flat `balance` field, not under `data`.

| Field | Type | Description |
|-------|------|-------------|
| `success` | bool | Whether the query succeeded. |
| `message` | string | Empty on success; error text on failure. |
| `balance` | number | Remaining balance in USD (present on success). |

```json
{
  "success": true,
  "message": "",
  "balance": 42.17
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/channel/update_balance/12" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status | Meaning |
|--------|---------|
| 200 `{"success": false, "message": "Not yet implemented"}` | Channel type (e.g. Azure) does not support balance queries. |
| 200 `{"success": false, "message": "...: status code: <n>"}` | Upstream billing endpoint returned a non-200 response (the `status code: <n>` is wrapped with a context prefix such as `get OpenAI subscription response`). |

### GET /api/channel/pricing/:id

Returns the effective pricing for a channel: derived model and completion ratios (computed from the unified configs), the unified per-model config map, and any tooling config.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | integer | Yes | Channel ID. |

**Response**: HTTP 200.

| Field | Type | Description |
|-------|------|-------------|
| `data.model_ratio` | object | model name → input price ratio (derived from configs). |
| `data.completion_ratio` | object | model name → completion price ratio (derived from configs). |
| `data.model_configs` | object | model name → unified config object (`ratio`, `completion_ratio`, optional `max_tokens`, media pricing, etc.). |
| `data.tooling` | object \| null | Channel tooling policy, or null. |

```json
{
  "success": true,
  "message": "",
  "data": {
    "model_ratio": {
      "gpt-4o": 2.5
    },
    "completion_ratio": {
      "gpt-4o": 4
    },
    "model_configs": {
      "gpt-4o": {
        "ratio": 2.5,
        "completion_ratio": 4,
        "max_tokens": 16384
      }
    },
    "tooling": null
  }
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/channel/pricing/12" \
  -H "Authorization: $ACCESS_TOKEN"
```

### PUT /api/channel/pricing/:id

Replaces a channel's pricing. Prefer the unified `model_configs` map; for backward compatibility a legacy `model_ratio` / `completion_ratio` pair is accepted and converted to `model_configs` automatically. A `tooling` payload can also be set here.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | integer | Yes | Channel ID. |

**Request body**

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Model configs | `model_configs` | object | No | — | Preferred: model name → config (`ratio`, optional `completion_ratio`, `cached_input_ratio`, `max_tokens`, `tiers`, `video`, `audio`, `image`, `embedding`). Used when non-empty. |
| Model ratio | `model_ratio` | object | No | — | Legacy: model name → input ratio. Used only when `model_configs` is empty. |
| Completion ratio | `completion_ratio` | object | No | — | Legacy: model name → completion ratio. |
| Tooling | `tooling` | string or object | No | — | Channel tooling policy. |

If neither `model_configs` nor the legacy ratios are supplied, pricing is left unchanged (a `tooling` payload, if supplied, is still applied).

```json
{
  "model_configs": {
    "gpt-4o": {
      "ratio": 2.5,
      "completion_ratio": 4,
      "max_tokens": 16384
    },
    "gpt-4o-mini": {
      "ratio": 0.15,
      "completion_ratio": 4
    }
  }
}
```

**Response**: HTTP 200. No `data`.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -sS -X PUT "$BASE_URL/api/channel/pricing/12" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model_configs":{"gpt-4o":{"ratio":2.5,"completion_ratio":4,"max_tokens":16384}}}'
```

**Errors**

| Status | Meaning |
|--------|---------|
| 200 `{"success": false, "message": "Failed to set model configs: ..."}` | A `model_configs` entry is invalid. |
| 200 `{"success": false, "message": "Invalid tooling config: ..."}` | `tooling` payload cannot be parsed. |

### GET /api/channel/default-pricing

Returns adapter-provided default pricing for a channel type, used to seed the pricing form when creating a channel. For OpenAI-compatible types it returns the merged global pricing across adapters. Note: `model_ratio`, `completion_ratio`, `model_configs`, and `tooling` are returned as JSON-encoded **strings** (not nested objects).

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `type` | integer | Yes | — | Channel type code. |

**Response**: HTTP 200.

| Field | Type | Description |
|-------|------|-------------|
| `data.model_ratio` | string (JSON) | Encoded map of model → input ratio. |
| `data.completion_ratio` | string (JSON) | Encoded map of model → completion ratio. |
| `data.model_configs` | string (JSON) | Encoded unified config map. |
| `data.tooling` | string (JSON) | Encoded default tooling config (empty string when none). |

```json
{
  "success": true,
  "message": "",
  "data": {
    "model_ratio": "{\"gpt-4o\":2.5,\"gpt-4o-mini\":0.15}",
    "completion_ratio": "{\"gpt-4o\":4,\"gpt-4o-mini\":4}",
    "model_configs": "{\"gpt-4o\":{\"ratio\":2.5,\"completion_ratio\":4}}",
    "tooling": ""
  }
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/channel/default-pricing?type=1" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status | Meaning |
|--------|---------|
| 200 `{"success": false, "message": "Invalid channel type: ..."}` | `type` is missing or not an integer. |
| 200 `{"success": false, "message": "Unsupported channel type"}` | No adapter exists for a non-OpenAI-compatible type. |

### POST /api/debug/channel/:id/debug

Emits detailed model-config diagnostics for one channel to the application logs. The response only confirms the logging occurred; inspect the server logs for the actual output.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | integer | Yes | Channel ID. |

**Response**: HTTP 200.

```json
{
  "success": true,
  "message": "Debug information logged. Check application logs for details."
}
```

**Example**

```bash
curl -sS -X POST "$BASE_URL/api/debug/channel/12/debug" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status | Meaning |
|--------|---------|
| 400 `{"success": false, "message": "Invalid channel ID"}` | `id` is not an integer. |
| 500 `{"success": false, "message": "Debug failed: ..."}` | Diagnostics could not be generated. |

### GET /api/debug/channels

Logs a summary of model-config state across all channels. The response confirms logging; details go to the application logs.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Response**: HTTP 200.

```json
{
  "success": true,
  "message": "Debug summary logged. Check application logs for details."
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/debug/channels" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status | Meaning |
|--------|---------|
| 500 `{"success": false, "message": "Debug failed: ..."}` | Summary could not be generated. |

### POST /api/debug/channel/:id/fix

Attempts to repair one channel's model configuration (e.g. normalize/migrate its config representation). Outcome details are written to the logs.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | integer | Yes | Channel ID. |

**Response**: HTTP 200.

```json
{
  "success": true,
  "message": "Channel model configs fixed. Check application logs for details."
}
```

**Example**

```bash
curl -sS -X POST "$BASE_URL/api/debug/channel/12/fix" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status | Meaning |
|--------|---------|
| 400 `{"success": false, "message": "Invalid channel ID"}` | `id` is not an integer. |
| 500 `{"success": false, "message": "Fix failed: ..."}` | Repair could not be applied. |

### GET /api/debug/channels/validate

Validates the model configuration of every channel and reports any issues to the application logs.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Response**: HTTP 200.

```json
{
  "success": true,
  "message": "Validation completed. Check application logs for details."
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/debug/channels/validate" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status | Meaning |
|--------|---------|
| 500 `{"success": false, "message": "Validation failed: ..."}` | Validation could not complete. |

### POST /api/debug/channels/remigrate

Re-runs the model-config migration for all channels (converts legacy ratio fields into the unified `model_configs` representation). Progress and results are logged.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Response**: HTTP 200.

```json
{
  "success": true,
  "message": "Re-migration completed. Check application logs for details."
}
```

**Example**

```bash
curl -sS -X POST "$BASE_URL/api/debug/channels/remigrate" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status | Meaning |
|--------|---------|
| 500 `{"success": false, "message": "Re-migration failed: ..."}` | Migration could not complete. |

### GET /api/debug/channel/:id/migration-status

Reports the migration state of one channel: whether it has unified configs and/or legacy ratio fields, the model names/counts in each, and a derived status label. Unlike the other debug endpoints, this returns structured `data`.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `id` | integer | Yes | Channel ID. |

**Response**: HTTP 200. Note: the success body omits `message` and returns only `success` + `data`.

| Field | Type | Description |
|-------|------|-------------|
| `data.channel_id` | integer | Channel ID. |
| `data.channel_name` | string | Channel name. |
| `data.channel_type` | integer | Channel type code. |
| `data.has_model_configs` | bool | Whether unified `model_configs` is populated. |
| `data.has_model_ratio` | bool | Whether legacy `model_ratio` is populated. |
| `data.has_completion_ratio` | bool | Whether legacy `completion_ratio` is populated. |
| `data.model_configs_models` | string[] | Model names in `model_configs` (present only when populated). |
| `data.model_configs_count` | integer | Count of those models (present only when populated). |
| `data.model_ratio_models` | string[] | Model names in legacy `model_ratio` (present only when populated). |
| `data.model_ratio_count` | integer | Count of those models (present only when populated). |
| `data.migration_status` | string | One of `migrated`, `migrated_with_legacy`, `needs_migration`, `empty`, `unknown`. |

```json
{
  "success": true,
  "data": {
    "channel_id": 12,
    "channel_name": "OpenAI Prod",
    "channel_type": 1,
    "has_model_configs": true,
    "has_model_ratio": false,
    "has_completion_ratio": false,
    "model_configs_models": ["gpt-4o", "gpt-4o-mini"],
    "model_configs_count": 2,
    "migration_status": "migrated"
  }
}
```

**Example**

```bash
curl -sS "$BASE_URL/api/debug/channel/12/migration-status" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status | Meaning |
|--------|---------|
| 400 `{"success": false, "message": "Invalid channel ID"}` | `id` is not an integer. |
| 404 `{"success": false, "message": "Channel not found"}` | No channel with that ID. |

### POST /api/debug/channels/clean

Cleans channels that hold mixed/legacy model data (e.g. both unified configs and stale legacy ratios). Results are logged.

**Auth:** Management access token — `Authorization: $ACCESS_TOKEN` (AdminAuth).

**Response**: HTTP 200.

```json
{
  "success": true,
  "message": "Mixed model data cleaned. Check application logs for details."
}
```

**Example**

```bash
curl -sS -X POST "$BASE_URL/api/debug/channels/clean" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status | Meaning |
|--------|---------|
| 500 `{"success": false, "message": "Cleaning failed: ..."}` | Cleanup could not complete. |


## Redemptions, Groups, Logs, Admin Token Visibility & Model Catalog

This section documents the administrative endpoints for managing redemption (gift) codes, listing billing groups, browsing and purging usage logs across all users, inspecting any user's API keys read-only, and fetching the full channel-to-model catalog. Every route here is mounted under `/api` and protected by `AdminAuth` (role >= admin, 10). All calls use the management **ACCESS TOKEN** (`Authorization: $ACCESS_TOKEN`, a leading `Bearer ` is also accepted) or an equivalent session cookie, and return the standard management envelope `{"success", "message", "data"}` with HTTP 200; several list endpoints add a top-level `"total"` field for pagination. On a handler error the envelope is `{"success": false, "message": "<reason>"}`, also returned with HTTP 200 in these controllers. The quota unit throughout is the internal integer where 500000 quota = 1 USD.

### GET /api/redemption/

Lists redemption codes with offset pagination, newest first (ordered by `id desc`).

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| p | integer | No | 0 | Zero-based page index; negative values are clamped to 0. Offset = `p * size`. |
| size | integer | No | DefaultItemsPerPage | Items per page; non-positive falls back to the default, and values above MaxItemsPerPage are clamped down. |

**Response:** HTTP 200. `data` is an array of redemption objects; `total` is the full row count (the count is over the whole table, not the filtered page).

| Field | Type | Description |
|-------|------|-------------|
| id | integer | Redemption code identifier. |
| user_id | integer | ID of the admin who created the code. |
| key | string | The 32-char redeem code (UUID with dashes removed). |
| status | integer | 1 = enabled, 2 = disabled, 3 = used. |
| name | string | Human-readable batch name. |
| quota | integer | Quota granted when redeemed (500000 = 1 USD). |
| created_time | integer | Unix seconds of creation. |
| redeemed_time | integer | Unix seconds when redeemed (0 if unused). |
| created_at | integer | Unix milliseconds (ORM timestamp). |
| updated_at | integer | Unix milliseconds (ORM timestamp). |

Note: the model also carries a write-only `count` field (used only on create, not persisted); it is not part of the stored row.

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 12,
      "user_id": 1,
      "key": "9f2c1ab84d5e4f0b9c7a3e21d6b8f0aa",
      "status": 1,
      "name": "launch-promo",
      "quota": 500000,
      "created_time": 1717000000,
      "redeemed_time": 0,
      "created_at": 1717000000123,
      "updated_at": 1717000000123
    }
  ],
  "total": 1
}
```

**Example**

```bash
curl -s "$BASE_URL/api/redemption/?p=0&size=20" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/redemption/search

Keyword-searches redemption codes and returns paginated, optionally sorted results. The keyword matches an exact `id` OR a `name` prefix (`name LIKE keyword%`).

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| keyword | string | No | "" | Matched as exact `id` or `name` prefix. Empty returns all rows. |
| p | integer | No | 0 | Zero-based page index. Offset = `p * size`. |
| size | integer | No | DefaultItemsPerPage | Items per page; clamped to MaxItemsPerPage. |
| sort | string | No | "" | Sort column. Allowed: id, name, status, quota, created_time, redeemed_time, created_at, updated_at. Unknown values fall back to `id desc`. |
| order | string | No | desc | Sort direction: asc or desc. |

**Response:** HTTP 200. Same object shape as `GET /api/redemption/`; `total` is the count over the matched (filtered) rows.

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 12,
      "user_id": 1,
      "key": "9f2c1ab84d5e4f0b9c7a3e21d6b8f0aa",
      "status": 1,
      "name": "launch-promo",
      "quota": 500000,
      "created_time": 1717000000,
      "redeemed_time": 0,
      "created_at": 1717000000123,
      "updated_at": 1717000000123
    }
  ],
  "total": 1
}
```

**Example**

```bash
curl -s "$BASE_URL/api/redemption/search?keyword=promo&sort=quota&order=desc" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/redemption/:id

Fetches a single redemption code by its numeric identifier.

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| id | integer | Yes | Redemption code identifier. |

**Response:** HTTP 200. `data` is a single redemption object (shape as above).

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 12,
    "user_id": 1,
    "key": "9f2c1ab84d5e4f0b9c7a3e21d6b8f0aa",
    "status": 1,
    "name": "launch-promo",
    "quota": 500000,
    "created_time": 1717000000,
    "redeemed_time": 0,
    "created_at": 1717000000123,
    "updated_at": 1717000000123
  }
}
```

**Example**

```bash
curl -s "$BASE_URL/api/redemption/12" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

- A non-numeric `:id` returns `{"success": false, "message": "..."}` (a `strconv.Atoi` parse error).
- An id that does not exist returns `{"success": false, "message": "get redemption by id <id>: record not found"}`.

### POST /api/redemption/

Creates a batch of redemption codes; each code grants the same quota when redeemed. The codes themselves are generated server-side as 32-char UUIDs (dashes removed). The creating admin's user ID is recorded as `user_id` on each code.

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Request body**

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Name | name | string | Yes | - | Batch name; must be non-blank and at most 20 bytes. |
| Quota | quota | integer | No | 0 | Quota each generated code grants (500000 = 1 USD). If omitted, the JSON zero value (0) is used; the model default of 100 applies only at the DB layer when the column is unset, but `Insert()` writes the bound value, so omitting it stores 0. |
| Count | count | integer | Yes | - | Number of codes to generate; must be 1-100. |

```json
{
  "name": "launch-promo",
  "quota": 500000,
  "count": 3
}
```

**Response:** HTTP 200. `data` is an array of the newly generated 32-char code strings (one per `count`).

```json
{
  "success": true,
  "message": "",
  "data": [
    "9f2c1ab84d5e4f0b9c7a3e21d6b8f0aa",
    "1b7d4e90a2c34f5e8b0d6a13c9e2f7bb",
    "44e0c8a1f3b24d67a9e5b2c0d8f1a6cc"
  ]
}
```

**Example**

```bash
curl -s -X POST "$BASE_URL/api/redemption/" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"launch-promo","quota":500000,"count":3}'
```

**Errors**

- `"Redemption name is required"` - blank/whitespace name.
- `"The length of the redemption code name must be between 1-20"` - name longer than 20 bytes.
- `"The number of redemption codes must be greater than 0"` - count <= 0.
- `"The number of redemption codes generated in a batch cannot be greater than 100"` - count > 100.
- If an insertion fails partway, the response is `{"success": false, "message": "<db error>", "data": [<codes created so far>]}` (HTTP 200).

### PUT /api/redemption/

Updates a redemption code. By default it updates `name` and `quota`; when `status_only` is present it updates only the `status`. The handler first loads the existing row by `id`, applies the changes, then persists.

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| status_only | string | No | "" | If non-empty, only `status` from the body is applied; `name`/`quota` are ignored. |

**Request body**

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Id | id | integer | Yes | - | Identifier of the redemption code to update; must reference an existing row. |
| Name | name | string | Conditional | - | New name; required (non-blank) in default mode, ignored in status-only mode. |
| Quota | quota | integer | No | - | New quota; applied in default mode. |
| Status | status | integer | Conditional | - | New status (1/2/3); applied only in status-only mode. |

```json
{
  "id": 12,
  "name": "launch-promo-renamed",
  "quota": 1000000
}
```

**Response:** HTTP 200. `data` is the updated redemption object.

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 12,
    "user_id": 1,
    "key": "9f2c1ab84d5e4f0b9c7a3e21d6b8f0aa",
    "status": 1,
    "name": "launch-promo-renamed",
    "quota": 1000000,
    "created_time": 1717000000,
    "redeemed_time": 0,
    "created_at": 1717000000123,
    "updated_at": 1717000050456
  }
}
```

**Example**

```bash
# Default update (name + quota)
curl -s -X PUT "$BASE_URL/api/redemption/" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":12,"name":"launch-promo-renamed","quota":1000000}'

# Status-only update (disable the code)
curl -s -X PUT "$BASE_URL/api/redemption/?status_only=1" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":12,"status":2}'
```

**Errors**

- `"Redemption name cannot be empty"` - blank name in default (non status-only) mode.
- `"get redemption by id <id>: record not found"` - the `id` does not exist.

### DELETE /api/redemption/:id

Permanently deletes a redemption code by ID.

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| id | integer | Yes | Redemption code identifier. |

**Response:** HTTP 200. No `data` payload.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -s -X DELETE "$BASE_URL/api/redemption/12" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

- A non-numeric or `0` `:id` parses to `0`, yielding `{"success": false, "message": "id is empty!"}`.
- A non-existent id returns `{"success": false, "message": "find redemption <id>: record not found"}`.

### GET /api/group/

Returns the list of configured billing group names (the groups a user/token can belong to, each mapping to a pricing multiplier). The list is built by iterating a map, so order is not guaranteed.

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Response:** HTTP 200. `data` is an array of group-name strings.

```json
{
  "success": true,
  "message": "",
  "data": [
    "default",
    "vip",
    "svip"
  ]
}
```

**Example**

```bash
curl -s "$BASE_URL/api/group/" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/log/

Lists usage/audit logs across all users with filtering, pagination, and optional sorting.

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| p | integer | No | 0 | Zero-based page index; negatives clamped to 0. Offset = `p * size`. |
| type | integer | No | 0 | Log type filter: 0 = all/unknown, 1 = topup, 2 = consume, 3 = manage, 4 = system. |
| username | string | No | "" | Filter by username. |
| token_name | string | No | "" | Filter by token name. |
| model_name | string | No | "" | Filter by model name. |
| channel | integer | No | 0 | Filter by channel ID (0 = any). |
| start_timestamp | integer | No | 0 | Lower bound (Unix seconds). |
| end_timestamp | integer | No | 0 | Upper bound (Unix seconds). |
| size | integer | No | DefaultItemsPerPage | Items per page; legacy alias `items_per_page` accepted (only used if `size` is absent); clamped to MaxItemsPerPage. |
| sort | string | No | "" | Sort column; preferred alias `sort_by` takes precedence if present. |
| order | string | No | desc | Sort direction asc/desc; preferred alias `sort_order`; if `order` is supplied it overrides `sort_order`. |

Note: when a sort column is supplied together with both `start_timestamp` and `end_timestamp` (both > 0), the range must not exceed 30 days, otherwise the request is rejected.

**Response:** HTTP 200. `data` is an array of log objects; `total` is the matching row count. The full `Log` struct is returned; the fields most relevant to consumers:

| Field | Type | Description |
|-------|------|-------------|
| id | integer | Log row ID. |
| user_id | integer | Owning user ID. |
| created_at | integer | Unix seconds of the event. |
| type | integer | Log type (see filter values above). |
| content | string | Human-readable description. |
| username | string | User who triggered the event. |
| token_name | string | Token used. |
| model_name | string | Model invoked (after any channel model mapping; used for billing). |
| origin_model_name | string | Model name as requested by the client before mapping. |
| quota | integer | Quota consumed/granted. |
| prompt_tokens | integer | Prompt token count. |
| completion_tokens | integer | Completion token count. |
| cached_prompt_tokens | integer | Cached prompt tokens. |
| channel | integer | Channel ID used. |
| request_id | string | Request identifier. |
| trace_id | string | Trace identifier. |
| updated_at | integer | Unix milliseconds (ORM timestamp). |
| elapsed_time | integer | Upstream latency in milliseconds. |
| is_stream | boolean | Whether the relay was streamed. |
| system_prompt_reset | boolean | Whether the system prompt was reset for this request. |
| metadata | object | Provider-specific attributes (e.g. cache write tokens); omitted when empty. |

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 90871,
      "user_id": 7,
      "created_at": 1717003600,
      "type": 2,
      "content": "model rate 0.000015 / completion rate 0.000060",
      "username": "alice",
      "token_name": "prod-key",
      "model_name": "gpt-4o",
      "origin_model_name": "",
      "quota": 1875,
      "prompt_tokens": 1024,
      "completion_tokens": 256,
      "cached_prompt_tokens": 0,
      "channel": 3,
      "request_id": "20260608120000-abc123",
      "trace_id": "4f9c1d2e7a8b",
      "updated_at": 1717003600456,
      "elapsed_time": 2840,
      "is_stream": true,
      "system_prompt_reset": false
    }
  ],
  "total": 1
}
```

**Example**

```bash
curl -s "$BASE_URL/api/log/?type=2&model_name=gpt-4o&start_timestamp=1716998400&end_timestamp=1717084800&p=0&size=20" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

- `"Date range for sorting cannot exceed 30 days"` - a sort column was supplied with a start/end range (both > 0) wider than 30 days.

### DELETE /api/log/

Purges historical log rows older than a given timestamp. Irreversible.

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| target_timestamp | integer | Yes | - | Unix seconds; logs with `created_at` older than this are deleted. Must be non-zero. |

**Response:** HTTP 200. `data` is the number of deleted rows.

```json
{
  "success": true,
  "message": "",
  "data": 1542
}
```

**Example**

```bash
curl -s -X DELETE "$BASE_URL/api/log/?target_timestamp=1704067200" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

- `"target timestamp is required"` - `target_timestamp` missing or 0.

### GET /api/log/stat

Returns the summed quota usage for logs matching the supplied filters (used for dashboard totals).

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| type | integer | No | 0 | Log type filter (see `GET /api/log/`). |
| username | string | No | "" | Filter by username. |
| token_name | string | No | "" | Filter by token name. |
| model_name | string | No | "" | Filter by model name. |
| channel | integer | No | 0 | Filter by channel ID. |
| start_timestamp | integer | No | 0 | Lower bound (Unix seconds). |
| end_timestamp | integer | No | 0 | Upper bound (Unix seconds). |

**Response:** HTTP 200. `data` is an object with a single `quota` field (summed quota over the filtered logs).

```json
{
  "success": true,
  "message": "",
  "data": {
    "quota": 348125
  }
}
```

**Example**

```bash
curl -s "$BASE_URL/api/log/stat?type=2&start_timestamp=1716998400&end_timestamp=1717084800" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/log/search

Full-text keyword search across all users' logs, paginated and optionally sorted.

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| keyword | string | No | "" | Term matched across log fields. |
| p | integer | No | 0 | Zero-based page index. Offset = `p * size`. |
| size | integer | No | DefaultItemsPerPage | Items per page; clamped to MaxItemsPerPage. |
| sort | string | No | "" | Sort column. |
| order | string | No | desc | Sort direction asc/desc. |

**Response:** HTTP 200. `data` is an array of log objects (same shape as `GET /api/log/`); `total` is the matched row count.

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 90871,
      "user_id": 7,
      "created_at": 1717003600,
      "type": 2,
      "content": "model rate 0.000015 / completion rate 0.000060",
      "username": "alice",
      "token_name": "prod-key",
      "model_name": "gpt-4o",
      "origin_model_name": "",
      "quota": 1875,
      "prompt_tokens": 1024,
      "completion_tokens": 256,
      "cached_prompt_tokens": 0,
      "channel": 3,
      "request_id": "20260608120000-abc123",
      "trace_id": "4f9c1d2e7a8b",
      "updated_at": 1717003600456,
      "elapsed_time": 2840,
      "is_stream": true,
      "system_prompt_reset": false
    }
  ],
  "total": 1
}
```

**Example**

```bash
curl -s "$BASE_URL/api/log/search?keyword=gpt-4o&sort=quota&order=desc&p=0&size=20" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/admin/tokens/

Lists API keys (relay tokens) across all users, read-only, for administrative inspection. Write operations on tokens remain on `/api/token` and act on the caller's own tokens.

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| p | integer | No | 0 | Zero-based page index; negatives clamped to 0. Offset = `p * size`. |
| size | integer | No | DefaultItemsPerPage | Items per page; clamped to MaxItemsPerPage. |
| user_id | integer | No | 0 | Filter to one user's tokens; 0 or absent = all users. |
| sort | string | No | "" | Sort column. |
| order | string | No | desc | Sort direction asc/desc. |

**Response:** HTTP 200. `data` is an array of token objects; `total` is the matching row count.

| Field | Type | Description |
|-------|------|-------------|
| id | integer | Token ID. |
| user_id | integer | Owning user ID. |
| key | string | The raw 48-char stored key (NOT prefixed). To call relay endpoints with it, prepend the configured prefix, e.g. `Authorization: Bearer sk-<key>`. |
| status | integer | Token status (1 = enabled). |
| name | string | Token name. |
| created_time | integer | Unix seconds of creation. |
| accessed_time | integer | Unix seconds of last use. |
| expired_time | integer | Unix seconds of expiry; -1 = never. |
| remain_quota | integer | Remaining quota. |
| unlimited_quota | boolean | Whether quota is unlimited. |
| used_quota | integer | Quota consumed so far. |
| created_at | integer | Unix milliseconds (ORM). |
| updated_at | integer | Unix milliseconds (ORM). |
| models | string\|null | Comma-separated allowed models, or null for all. |
| subnet | string\|null | Allowed subnet CIDR(s), or null/empty for any. |

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 41,
      "user_id": 7,
      "key": "AbCd1234EfGh5678ijklmnopqrstuvwx0123456789ABCDEF",
      "status": 1,
      "name": "prod-key",
      "created_time": 1716000000,
      "accessed_time": 1717003600,
      "expired_time": -1,
      "remain_quota": 4500000,
      "unlimited_quota": false,
      "used_quota": 348125,
      "created_at": 1716000000123,
      "updated_at": 1717003600456,
      "models": "gpt-4o,claude-sonnet-4",
      "subnet": null
    }
  ],
  "total": 1
}
```

**Example**

```bash
curl -s "$BASE_URL/api/admin/tokens/?user_id=7&p=0&size=20" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/admin/tokens/search

Keyword-searches tokens across all users by name prefix (`name LIKE keyword%`), read-only, paginated and optionally sorted.

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| keyword | string | No | "" | Matched against the token name as a prefix. Empty returns all tokens. |
| p | integer | No | 0 | Zero-based page index. Offset = `p * size`. |
| size | integer | No | DefaultItemsPerPage | Items per page; clamped to MaxItemsPerPage. |
| sort | string | No | "" | Sort column. |
| order | string | No | desc | Sort direction asc/desc. |

**Response:** HTTP 200. `data` is an array of token objects (same shape as `GET /api/admin/tokens/`); `total` is the matched row count.

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 41,
      "user_id": 7,
      "key": "AbCd1234EfGh5678ijklmnopqrstuvwx0123456789ABCDEF",
      "status": 1,
      "name": "prod-key",
      "created_time": 1716000000,
      "accessed_time": 1717003600,
      "expired_time": -1,
      "remain_quota": 4500000,
      "unlimited_quota": false,
      "used_quota": 348125,
      "created_at": 1716000000123,
      "updated_at": 1717003600456,
      "models": "gpt-4o,claude-sonnet-4",
      "subnet": null
    }
  ],
  "total": 1
}
```

**Example**

```bash
curl -s "$BASE_URL/api/admin/tokens/search?keyword=prod&sort=used_quota&order=desc" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/admin/tokens/:id

Fetches any token by ID regardless of owner, read-only.

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Path parameters**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| id | integer | Yes | Token identifier. |

**Response:** HTTP 200. `data` is a single token object (same shape as `GET /api/admin/tokens/`).

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 41,
    "user_id": 7,
    "key": "AbCd1234EfGh5678ijklmnopqrstuvwx0123456789ABCDEF",
    "status": 1,
    "name": "prod-key",
    "created_time": 1716000000,
    "accessed_time": 1717003600,
    "expired_time": -1,
    "remain_quota": 4500000,
    "unlimited_quota": false,
    "used_quota": 348125,
    "created_at": 1716000000123,
    "updated_at": 1717003600456,
    "models": "gpt-4o,claude-sonnet-4",
    "subnet": null
  }
}
```

**Example**

```bash
curl -s "$BASE_URL/api/admin/tokens/41" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

- A non-numeric `:id` returns `{"success": false, "message": "invalid token id: ..."}` (parse error wrapped).
- A non-existent id returns `{"success": false, "message": "failed to get token by id=<id>: record not found"}`.

### GET /api/models

Returns the full channel-to-model catalog: a map keyed by channel type ID, each value being the list of model names that channel exposes. Used by the admin dashboard to populate channel/model selectors. The map is precomputed at startup.

**Auth:** Management ACCESS TOKEN - `Authorization: $ACCESS_TOKEN` (admin role).

**Response:** HTTP 200. `data` is an object whose keys are channel-type IDs (as JSON object string keys) and whose values are arrays of model-name strings.

```json
{
  "success": true,
  "message": "",
  "data": {
    "1": [
      "gpt-4o",
      "gpt-4o-mini",
      "gpt-3.5-turbo"
    ],
    "14": [
      "claude-sonnet-4",
      "claude-opus-4"
    ]
  }
}
```

**Example**

```bash
curl -s "$BASE_URL/api/models" \
  -H "Authorization: $ACCESS_TOKEN"
```


## MCP Server & Tool Administration

These endpoints let administrators register and manage the upstream Model Context Protocol (MCP) servers that One API aggregates, and to inspect the catalogue of tools synchronized from them. One API acts as an MCP aggregator: each registered MCP server is polled (via "sync") for its tool list, and the union of enabled tools is exposed to downstream inference requests as built-in tools (see `docs/manuals/mcp_aggregator.md` for the aggregator concept, tool routing, priority, and billing semantics).

All routes in this section are mounted under `/api/mcp_servers` and `/api/mcp_tools`, and every route is guarded by `AdminAuth` — they require an **admin-or-higher** credential (role >= 10). Authenticate with the management **access token** (`Authorization: $ACCESS_TOKEN`; a leading `Bearer ` is also accepted) obtained from `GET /api/user/token`, or with an active admin **session cookie**. A relay API key (`sk-...`) is NOT accepted here.

All routes follow the management API envelope: `{"success": true, "message": "", "data": <payload>}` on success (HTTP 200). Errors are also returned with **HTTP 200** and `{"success": false, "message": "<reason>"}` (the controllers use `helper.RespondError`, which always writes status 200). List endpoints additionally include a top-level `"total"` count.

**Secret masking.** The MCP server `api_key` is stored encrypted (AES-GCM) and is never returned in plaintext. Read responses mask it: a configured key is returned as the literal `******`, while a server with no key set returns an empty string `""` (the mask helper only masks non-empty values). On create/update, sending the literal `******` back is treated as "leave the stored credential unchanged".

The persisted `MCPServer` object has the following shape (returned by the read/create/update endpoints, with `api_key` masked):

| Field | JSON key | Type | Description |
|---|---|---|---|
| Id | `id` | integer | Server identifier. |
| Name | `name` | string | Unique display name (required, trimmed, `varchar(128)`). |
| Description | `description` | string | Free-form description. |
| Status | `status` | integer | `1` = enabled, `0` = disabled. |
| Priority | `priority` | integer (int64) | Routing priority; higher wins when tool names collide. |
| BaseURL | `base_url` | string | Upstream MCP endpoint URL (required, must be `http`/`https`). |
| Protocol | `protocol` | string | Transport protocol, lowercased on save; currently `streamable_http`. |
| AuthType | `auth_type` | string | One of `none`, `bearer`, `api_key`, `custom_headers` (lowercased on save). |
| APIKey | `api_key` | string | Upstream credential; masked as `******` (or `""` when unset) in responses. |
| Headers | `headers` | object (string→string) | Custom request headers sent upstream; serializes as `{}` when empty. |
| ToolWhitelist | `tool_whitelist` | array of strings | If non-empty, only these tool names are exposed; serializes as `[]` when empty. |
| ToolBlacklist | `tool_blacklist` | array of strings | Tool names to exclude; serializes as `[]` when empty. |
| ToolPricing | `tool_pricing` | object (string→pricing) | Per-tool price overrides; serializes as `{}` when empty. Each value is `{"usd_per_call": <float>, "quota_per_call": <int>}`, where each field is omitted when zero. |
| AutoSyncEnabled | `auto_sync_enabled` | boolean | Whether the server is synced automatically. |
| AutoSyncIntervalMinutes | `auto_sync_interval_minutes` | integer | Auto-sync interval in minutes; must be between 5 and 1440. |
| LastSyncAt | `last_sync_at` | integer | Epoch milliseconds of the last sync attempt (`0` if never synced). |
| LastSyncStatus | `last_sync_status` | string | `ok`, `error`, or empty (never synced). |
| LastSyncError | `last_sync_error` | string | Error message from the last failed sync. |
| LastTestAt | `last_test_at` | integer | Epoch milliseconds of the last connectivity test (`0` if never tested). |
| LastTestStatus | `last_test_status` | string | `ok`, `error`, or empty. |
| LastTestError | `last_test_error` | string | Error message from the last failed test. |
| CreatedAt | `created_at` | integer | Epoch milliseconds. |
| UpdatedAt | `updated_at` | integer | Epoch milliseconds. |

> Note on pricing serialization: `usd_per_call` and `quota_per_call` are both tagged `omitempty`, so a zero value for either is omitted from the JSON. A pricing entry that is entirely zero serializes as `{}`.

### GET /api/mcp_servers/ (and /api/mcp_servers)

Lists registered MCP servers with pagination and sorting; each entry pairs the (secret-masked) server object with its synchronized tool count.

**Auth:** Admin access token via `Authorization: $ACCESS_TOKEN` (or an active admin session cookie). Requires role admin (>=10).

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| p | integer | No | 0 | Zero-based page index. Negative values are clamped to 0. |
| size | integer | No | server default | Page size; non-positive values use the server default, and values above the server max are clamped to it. |
| sort | string | No | (none) | Sort column: one of `id`, `name`, `status`, `priority`, `created_at`, `updated_at`. Unknown/empty values fall back to `id desc`. |
| order | string | No | `desc` | Sort direction: `asc` or `desc` (any other value is treated as `desc`). Ignored when `sort` is unknown (the fallback is always `id desc`). |

**Response**: HTTP 200. `data` is an array of objects, each `{"server": <MCPServer>, "tool_count": <int>}`, where `tool_count` counts all tools for that server regardless of status. `total` is the total server count (not the page size).

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "server": {
        "id": 1,
        "name": "weather-mcp",
        "description": "Weather tools",
        "status": 1,
        "priority": 10,
        "base_url": "https://mcp.example.com/sse",
        "protocol": "streamable_http",
        "auth_type": "bearer",
        "api_key": "******",
        "headers": {},
        "tool_whitelist": [],
        "tool_blacklist": [],
        "tool_pricing": {},
        "auto_sync_enabled": true,
        "auto_sync_interval_minutes": 60,
        "last_sync_at": 1749350400000,
        "last_sync_status": "ok",
        "last_sync_error": "",
        "last_test_at": 1749350000000,
        "last_test_status": "ok",
        "last_test_error": "",
        "created_at": 1749000000000,
        "updated_at": 1749350400000
      },
      "tool_count": 4
    }
  ],
  "total": 1
}
```

**Example**

```bash
curl -s "$BASE_URL/api/mcp_servers/?p=0&size=20&sort=priority&order=desc" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/mcp_servers/:id

Returns the full configuration for a single MCP server (with `api_key` masked).

**Auth:** Admin access token via `Authorization: $ACCESS_TOKEN` (or admin session cookie). Requires role admin (>=10).

**Path parameters**

| Name | Type | Required | Description |
|---|---|---|---|
| id | integer | Yes | MCP server identifier. |

**Response**: HTTP 200. `data` is a single `MCPServer` object (see the table above).

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 1,
    "name": "weather-mcp",
    "description": "Weather tools",
    "status": 1,
    "priority": 10,
    "base_url": "https://mcp.example.com/sse",
    "protocol": "streamable_http",
    "auth_type": "bearer",
    "api_key": "******",
    "headers": {},
    "tool_whitelist": [],
    "tool_blacklist": [],
    "tool_pricing": {},
    "auto_sync_enabled": true,
    "auto_sync_interval_minutes": 60,
    "last_sync_at": 1749350400000,
    "last_sync_status": "ok",
    "last_sync_error": "",
    "last_test_at": 1749350000000,
    "last_test_status": "ok",
    "last_test_error": "",
    "created_at": 1749000000000,
    "updated_at": 1749350400000
  }
}
```

**Example**

```bash
curl -s "$BASE_URL/api/mcp_servers/1" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status / body | Meaning |
|---|---|
| 200 `{"success": false, "message": "get mcp server: record not found"}` | No server with that id (errors use HTTP 200 with `success:false`). |
| 200 `{"success": false, "message": "strconv.Atoi: parsing \"...\": invalid syntax"}` | Non-integer `id` in the path. |

### POST /api/mcp_servers/ (and /api/mcp_servers)

Registers a new MCP server. The payload is normalized and validated (`NormalizeAndValidate`) and the API key is encrypted before the record is persisted. The upstream tool list is **not** fetched by this call; use `POST /api/mcp_servers/:id/sync` afterwards.

**Auth:** Admin access token via `Authorization: $ACCESS_TOKEN` (or admin session cookie). Requires role admin (>=10).

**Request body**: all fields are pointers/optional in the request struct — only fields present in the body are applied; omitted fields take their model/gorm defaults. After applying, `name` and `base_url` must be non-empty or the request is rejected.

| Field | JSON key | Type | Required | Default | Description |
|---|---|---|---|---|---|
| Name | `name` | string | Yes | — | Unique server name (trimmed; must be non-empty after trim). |
| Description | `description` | string | No | `""` | Free-form description. |
| Status | `status` | integer | No | `1` | `1` = enabled, `0` = disabled. |
| Priority | `priority` | integer (int64) | No | `0` | Routing priority for tool-name collisions. |
| BaseURL | `base_url` | string | Yes | — | Upstream endpoint; trimmed; must parse as an `http`/`https` URL. |
| Protocol | `protocol` | string | No | `streamable_http` | Transport; trimmed and lowercased on save; empty defaults to `streamable_http`. |
| AuthType | `auth_type` | string | No | `none` | `none`, `bearer`, `api_key`, or `custom_headers`; trimmed and lowercased; empty defaults to `none`. |
| APIKey | `api_key` | string | No | `""` | Upstream credential; stored encrypted. Sending the literal `******` is ignored (no value applied). |
| Headers | `headers` | object (string→string) | No | `null` | Custom upstream headers. |
| ToolWhitelist | `tool_whitelist` | array of strings | No | `null` | Allow-list of tool names. |
| ToolBlacklist | `tool_blacklist` | array of strings | No | `null` | Deny-list of tool names. |
| ToolPricing | `tool_pricing` | object (string→`{usd_per_call,quota_per_call}`) | No | `null` | Per-tool price overrides; tool names must be non-empty and `usd_per_call`/`quota_per_call` must be non-negative. |
| AutoSyncEnabled | `auto_sync_enabled` | boolean | No | `true` | Enable scheduled auto-sync. |
| AutoSyncIntervalMinutes | `auto_sync_interval_minutes` | integer | No | `60` | Auto-sync interval in minutes; `0` is treated as the default `60`, otherwise must be 5–1440. |

```json
{
  "name": "weather-mcp",
  "description": "Weather tools",
  "status": 1,
  "priority": 10,
  "base_url": "https://mcp.example.com/sse",
  "protocol": "streamable_http",
  "auth_type": "bearer",
  "api_key": "sk-upstream-secret-token",
  "headers": {},
  "tool_whitelist": ["get_forecast", "get_alerts"],
  "tool_blacklist": [],
  "tool_pricing": {
    "get_forecast": { "usd_per_call": 0.002 }
  },
  "auto_sync_enabled": true,
  "auto_sync_interval_minutes": 60
}
```

**Response**: HTTP 200. `data` is the created `MCPServer` (with `api_key` masked and `id` assigned). Note that zero pricing fields are omitted in the echoed `tool_pricing` (here `quota_per_call` was omitted because it was 0).

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 2,
    "name": "weather-mcp",
    "description": "Weather tools",
    "status": 1,
    "priority": 10,
    "base_url": "https://mcp.example.com/sse",
    "protocol": "streamable_http",
    "auth_type": "bearer",
    "api_key": "******",
    "headers": {},
    "tool_whitelist": ["get_forecast", "get_alerts"],
    "tool_blacklist": [],
    "tool_pricing": {
      "get_forecast": { "usd_per_call": 0.002 }
    },
    "auto_sync_enabled": true,
    "auto_sync_interval_minutes": 60,
    "last_sync_at": 0,
    "last_sync_status": "",
    "last_sync_error": "",
    "last_test_at": 0,
    "last_test_status": "",
    "last_test_error": "",
    "created_at": 1749350400000,
    "updated_at": 1749350400000
  }
}
```

**Example**

```bash
curl -s -X POST "$BASE_URL/api/mcp_servers/" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "weather-mcp",
    "base_url": "https://mcp.example.com/sse",
    "protocol": "streamable_http",
    "auth_type": "bearer",
    "api_key": "sk-upstream-secret-token",
    "auto_sync_enabled": true,
    "auto_sync_interval_minutes": 60
  }'
```

**Errors** (all HTTP 200 with `success:false`; validation messages are wrapped, so the actual `message` is prefixed with `normalize and validate mcp server: ` and pricing errors additionally with `validate tool pricing: `):

| Status / body (message contains) | Meaning |
|---|---|
| 200 `...mcp server name is required` | `name` missing or blank after trim. |
| 200 `...mcp server base_url is required` | `base_url` missing or blank after trim. |
| 200 `...mcp server base_url must use http or https` | `base_url` is set but not an `http`/`https` URL. |
| 200 `...auto_sync_interval_minutes must be between 5 and 1440` | Interval out of range (and non-zero). |
| 200 `...tool <name> usd_per_call cannot be negative` | A `tool_pricing` `usd_per_call` value is negative. |
| 200 `...tool <name> quota_per_call cannot be negative` | A `tool_pricing` `quota_per_call` value is negative. |
| 200 `...tool pricing contains empty tool name` | A `tool_pricing` key is blank after trim. |
| 200 `decode mcp server: ...` | Request body is not valid JSON. |

### PUT /api/mcp_servers/:id

Updates an existing MCP server. The handler tracks which JSON keys were physically present in the raw request body, so only those columns are written — including explicit zero/empty values (e.g. sending `"description": ""` or `"tool_whitelist": []` clears that column, which GORM's struct-based update would otherwise skip). Fields omitted from the body are left untouched. Sending `"api_key": "******"` leaves the stored credential unchanged (the masked-secret placeholder is dropped from the provided-fields set). The merged record is re-validated by `NormalizeAndValidate` before saving.

**Auth:** Admin access token via `Authorization: $ACCESS_TOKEN` (or admin session cookie). Requires role admin (>=10).

**Path parameters**

| Name | Type | Required | Description |
|---|---|---|---|
| id | integer | Yes | MCP server identifier. |

**Request body**: same fields as `POST /api/mcp_servers/`; all optional. Only the fields physically present in the body are applied. The following JSON keys are honored for per-column updates: `name`, `description`, `status`, `priority`, `base_url`, `protocol`, `auth_type`, `api_key`, `headers`, `tool_whitelist`, `tool_blacklist`, `tool_pricing`, `auto_sync_enabled`, `auto_sync_interval_minutes`.

```json
{
  "priority": 20,
  "status": 0,
  "tool_blacklist": ["debug_tool"],
  "auto_sync_interval_minutes": 120
}
```

**Response**: HTTP 200. `data` is the updated `MCPServer` (with `api_key` masked).

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 1,
    "name": "weather-mcp",
    "description": "Weather tools",
    "status": 0,
    "priority": 20,
    "base_url": "https://mcp.example.com/sse",
    "protocol": "streamable_http",
    "auth_type": "bearer",
    "api_key": "******",
    "headers": {},
    "tool_whitelist": [],
    "tool_blacklist": ["debug_tool"],
    "tool_pricing": {},
    "auto_sync_enabled": true,
    "auto_sync_interval_minutes": 120,
    "last_sync_at": 1749350400000,
    "last_sync_status": "ok",
    "last_sync_error": "",
    "last_test_at": 1749350000000,
    "last_test_status": "ok",
    "last_test_error": "",
    "created_at": 1749000000000,
    "updated_at": 1749351000000
  }
}
```

**Example**

```bash
curl -s -X PUT "$BASE_URL/api/mcp_servers/1" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"priority": 20, "status": 0, "auto_sync_interval_minutes": 120}'
```

**Errors**: same validation errors as `POST` (name/base_url required, base_url scheme, interval range, pricing non-negative, empty pricing tool name). A non-existent `id` returns `{"success": false, "message": "get mcp server: record not found"}`; a non-integer `id` returns a `strconv.Atoi` parse error. Because the merged record is re-validated, a previously-stored server still cannot be saved with an out-of-range interval or invalid base_url.

### DELETE /api/mcp_servers/:id

Permanently deletes an MCP server record by id. Its synchronized tools become orphaned and are no longer exposed downstream.

**Auth:** Admin access token via `Authorization: $ACCESS_TOKEN` (or admin session cookie). Requires role admin (>=10).

**Path parameters**

| Name | Type | Required | Description |
|---|---|---|---|
| id | integer | Yes | MCP server identifier. |

**Response**: HTTP 200. No `data` field is returned.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -s -X DELETE "$BASE_URL/api/mcp_servers/1" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status / body | Meaning |
|---|---|
| 200 `{"success": false, "message": "mcp server id is invalid"}` | `id` <= 0. |
| 200 `{"success": false, "message": "strconv.Atoi: parsing \"...\": invalid syntax"}` | Non-integer `id` in the path. |

### POST /api/mcp_servers/:id/sync

Triggers an immediate, manual refresh of the server's tool catalogue: One API connects upstream, lists the available tools, and persists them (replacing the previously stored tool set for that server). The server's `last_sync_at`/`last_sync_status`/`last_sync_error` fields are updated to reflect the attempt (including on failure, where the status is set to `error`).

**Auth:** Admin access token via `Authorization: $ACCESS_TOKEN` (or admin session cookie). Requires role admin (>=10).

**Path parameters**

| Name | Type | Required | Description |
|---|---|---|---|
| id | integer | Yes | MCP server identifier. |

**Response**: HTTP 200. `data.tool_count` is the number of tools synchronized and persisted.

```json
{
  "success": true,
  "message": "",
  "data": {
    "tool_count": 4
  }
}
```

**Example**

```bash
curl -s -X POST "$BASE_URL/api/mcp_servers/1/sync" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status / body | Meaning |
|---|---|
| 200 `{"success": false, "message": "get mcp server: record not found"}` | No server with that id. |
| 200 `{"success": false, "message": "<upstream sync error>"}` | Upstream sync failed (e.g. unreachable host, auth rejected); before responding, the server's `last_sync_status` is set to `error` with the message. |

### POST /api/mcp_servers/:id/test

Performs a connectivity check against the upstream MCP server using a short-lived (15s) Streamable HTTP client that lists the server's tools. Updates `last_test_at`/`last_test_status`/`last_test_error`. Unlike `/sync`, this does **not** persist the discovered tools; it only reports reachability and how many tools the server advertised.

**Auth:** Admin access token via `Authorization: $ACCESS_TOKEN` (or admin session cookie). Requires role admin (>=10).

**Path parameters**

| Name | Type | Required | Description |
|---|---|---|---|
| id | integer | Yes | MCP server identifier. |

**Response**: HTTP 200. `data.tool_count` is the number of tools the upstream returned; `data.protocol` echoes the server's stored transport protocol.

```json
{
  "success": true,
  "message": "",
  "data": {
    "tool_count": 4,
    "protocol": "streamable_http"
  }
}
```

**Example**

```bash
curl -s -X POST "$BASE_URL/api/mcp_servers/1/test" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status / body | Meaning |
|---|---|
| 200 `{"success": false, "message": "get mcp server: record not found"}` | No server with that id. |
| 200 `{"success": false, "message": "<connection error>"}` | The upstream could not be reached or rejected the request within 15s; before responding, `last_test_status` is set to `error`. |

### GET /api/mcp_servers/:id/tools

Lists the tools currently stored for a single MCP server. The server's per-tool pricing overrides are applied to each matching tool's `default_pricing` (matched case-insensitively by tool name), and any tool whose stored `input_schema` is the serialized literal `"null"` is normalized to an empty string before rendering. This endpoint is **not** paginated and returns the full stored tool set (a non-nil, possibly empty array).

**Auth:** Admin access token via `Authorization: $ACCESS_TOKEN` (or admin session cookie). Requires role admin (>=10).

**Path parameters**

| Name | Type | Required | Description |
|---|---|---|---|
| id | integer | Yes | MCP server identifier. |

**Response**: HTTP 200. `data` is an array of `MCPTool` objects (no `total` field for this endpoint).

| Field | JSON key | Type | Description |
|---|---|---|---|
| Id | `id` | integer | Tool record identifier. |
| ServerId | `server_id` | integer | Owning MCP server id. |
| Name | `name` | string | Tool name (normalized to lowercase on storage). |
| DisplayName | `display_name` | string | Human-readable name. |
| Description | `description` | string | Tool description. |
| InputSchema | `input_schema` | string | JSON Schema for the tool's arguments, serialized as a string (empty string when the upstream provided none or `null`). |
| DefaultPricing | `default_pricing` | object | `{usd_per_call, quota_per_call}` (each field omitted when zero, so this is `{}` when both are zero); reflects the server's per-tool pricing override when one matches. |
| Status | `status` | integer | `1` = enabled, `0` = disabled. |
| CreatedAt | `created_at` | integer | Epoch milliseconds. |
| UpdatedAt | `updated_at` | integer | Epoch milliseconds. |

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 11,
      "server_id": 1,
      "name": "get_forecast",
      "display_name": "Get Forecast",
      "description": "Return the weather forecast for a location.",
      "input_schema": "{\"type\":\"object\",\"properties\":{\"location\":{\"type\":\"string\"}},\"required\":[\"location\"]}",
      "default_pricing": { "usd_per_call": 0.002 },
      "status": 1,
      "created_at": 1749350400000,
      "updated_at": 1749350400000
    }
  ]
}
```

**Example**

```bash
curl -s "$BASE_URL/api/mcp_servers/1/tools" \
  -H "Authorization: $ACCESS_TOKEN"
```

**Errors**

| Status / body | Meaning |
|---|---|
| 200 `{"success": false, "message": "get mcp server: record not found"}` | No server with that id (the handler loads the server before its tools). |

### GET /api/mcp_tools/ (and /api/mcp_tools)

Lists synchronized MCP tools across all servers (or filtered to one server and/or status), with pagination and sorting. Use this for a global view of the aggregated tool catalogue. Unlike `GET /api/mcp_servers/:id/tools`, this endpoint does **not** apply server pricing overrides or normalize `null` schemas — it returns the raw stored `MCPTool` rows.

**Auth:** Admin access token via `Authorization: $ACCESS_TOKEN` (or admin session cookie). Requires role admin (>=10).

**Query parameters**

| Name | Type | Required | Default | Description |
|---|---|---|---|---|
| p | integer | No | 0 | Zero-based page index. Negative values are clamped to 0. |
| size | integer | No | server default | Page size; non-positive values use the server default, values above the server max are clamped to it. |
| sort | string | No | (none) | Sort column: one of `id`, `name`, `status`, `created_at`, `updated_at`. Unknown/empty values fall back to `id desc`. |
| order | string | No | `desc` | Sort direction: `asc` or `desc` (any other value is treated as `desc`). |
| server_id | integer | No | 0 (all servers) | Filter to tools belonging to this MCP server id; only applied when > 0. |
| status | integer | No | (no filter) | Filter by tool status (`1` enabled, `0` disabled). Omit or send empty for no filter; non-integer values are ignored (treated as no filter). |

**Response**: HTTP 200. `data` is an array of `MCPTool` objects (see the field table under `GET /api/mcp_servers/:id/tools`). `total` is the count of rows matching the `server_id`/`status` filter.

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 11,
      "server_id": 1,
      "name": "get_forecast",
      "display_name": "Get Forecast",
      "description": "Return the weather forecast for a location.",
      "input_schema": "{\"type\":\"object\",\"properties\":{\"location\":{\"type\":\"string\"}}}",
      "default_pricing": {},
      "status": 1,
      "created_at": 1749350400000,
      "updated_at": 1749350400000
    }
  ],
  "total": 1
}
```

**Example**

```bash
curl -s "$BASE_URL/api/mcp_tools/?server_id=1&status=1&p=0&size=50&sort=name&order=asc" \
  -H "Authorization: $ACCESS_TOKEN"
```


## System Options (Root) & Public Endpoints

This section documents the root-only system configuration endpoints and the public/optional-auth endpoints that power the web dashboard (status banner, model catalog, MCP tools catalog, notice, about, and homepage content). The two `/api/option/` endpoints require a root-level credential (the management access token under `Authorization`, or a root session cookie) and read/write the server's key/value option store. The remaining endpoints are public (or never-reject optional auth) and return management-style envelopes `{"success", "message", "data"}` with HTTP 200. Sensitive option keys (any key ending in `Token`, `Secret`, or `Password`) are stripped from reads and protected from accidental empty-value overwrites on writes.

> Auth note: none of the endpoints in this section accept the relay API key (`sk-...`). The two write/read option endpoints use the management **access token** (32-char UUID from `GET /api/user/token`) or a session cookie; `GET /api/models/display` optionally accepts the same access token / session but never the relay key; the rest are fully public.

### GET /api/option/

Returns the full set of stored system configuration options as key/value pairs, omitting any sensitive key (suffix `Token`/`Secret`/`Password`).

**Auth:** Root access token. Header: `Authorization: $ACCESS_TOKEN` (a leading `Bearer ` is also accepted), or a root session cookie. Requires role >= 100. Missing/invalid credential -> `401`; valid credential below root -> `403`.

**Response:** `200 OK`. `data` is an array of option objects.

| Field | JSON key | Type | Description |
|-------|----------|------|-------------|
| Key | `key` | string | Option name |
| Value | `value` | string | Option value, always serialized as a string |
| CreatedAt | `created_at` | integer | Always `0` here (the handler builds options from the in-memory map and does not populate timestamps) |
| UpdatedAt | `updated_at` | integer | Always `0` here (see above) |

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "key": "SystemName",
      "value": "One API",
      "created_at": 0,
      "updated_at": 0
    },
    {
      "key": "Theme",
      "value": "modern",
      "created_at": 0,
      "updated_at": 0
    },
    {
      "key": "DisplayInCurrencyEnabled",
      "value": "true",
      "created_at": 0,
      "updated_at": 0
    },
    {
      "key": "QuotaPerUnit",
      "value": "500000",
      "created_at": 0,
      "updated_at": 0
    }
  ]
}
```

**Example**

```bash
curl -s "$BASE_URL/api/option/" \
  -H "Authorization: $ACCESS_TOKEN"
```

### PUT /api/option/

Persists a single configuration option (upsert by key). Some keys gate a feature toggle and are validated against prerequisite configuration before being saved.

**Auth:** Root access token. Header: `Authorization: $ACCESS_TOKEN` (a leading `Bearer ` is also accepted), or a root session cookie. Requires role >= 100.

**Request body**

| Field | JSON key | Type | Required | Default | Description |
|-------|----------|------|----------|---------|-------------|
| Key | `key` | string | yes | - | Option name to set |
| Value | `value` | string | yes | - | New value (always a string; booleans are `"true"`/`"false"`, numbers are decimal strings) |

```json
{
  "key": "SystemName",
  "value": "My Gateway"
}
```

Behavior notes for specific keys:
- `Theme`: must be a known theme; the legacy value `"default"` is rewritten to `"modern"`.
- `GitHubOAuthEnabled`: cannot be set to `"true"` unless the GitHub Client Id is already configured.
- `WeChatAuthEnabled`: cannot be set to `"true"` unless the WeChat server address is already configured.
- `TurnstileCheckEnabled`: cannot be set to `"true"` unless the Turnstile site key is already configured.
- `EmailDomainRestrictionEnabled`: cannot be set to `"true"` unless an email domain whitelist is already configured.
- Sensitive keys (suffix `Token`/`Secret`/`Password`): an empty/whitespace `value` is ignored (treated as "no change") to avoid wiping a stored secret; the response then reports `"empty value ignored for sensitive option"` with `success: true`.

**Response:** `200 OK`.

```json
{
  "success": true,
  "message": ""
}
```

**Example**

```bash
curl -s -X PUT "$BASE_URL/api/option/" \
  -H "Authorization: $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"key":"SystemName","value":"My Gateway"}'
```

**Errors**

| Status | Body `message` | Meaning |
|--------|----------------|---------|
| 400 | invalid parameter | Request body is not valid JSON |
| 200 | invalid theme | `Theme` value is not a recognized theme |
| 200 | Unable to enable ... please fill in ... first! | Toggling a feature on without its prerequisite configuration (GitHub OAuth / email domain restriction / WeChat / Turnstile) |
| 200 | (db error text) | Persisting the option to the store failed |

> Note: business-logic errors (invalid theme, prerequisite missing, db failure) are returned via the standard error helper as HTTP `200` with `{"success": false, "message": "..."}`. Only the malformed-JSON case returns HTTP `400`.

### GET /api/status

Returns application metadata and the public feature/auth toggles consumed by the web UI at load time.

**Auth:** Public - no auth.

**Response:** `200 OK`. `data` is an object of metadata and toggles.

| Field | JSON key | Type | Description |
|-------|----------|------|-------------|
| version | `version` | string | Build version |
| start_time | `start_time` | integer | Process start time (Unix seconds) |
| email_verification | `email_verification` | boolean | Email verification required for registration |
| github_oauth | `github_oauth` | boolean | GitHub OAuth login enabled |
| github_client_id | `github_client_id` | string | GitHub OAuth client id (public) |
| lark_client_id | `lark_client_id` | string | Lark OAuth client id (public) |
| system_name | `system_name` | string | Display name of the deployment |
| logo | `logo` | string | Logo URL |
| footer_html | `footer_html` | string | Custom footer HTML |
| wechat_qrcode | `wechat_qrcode` | string | WeChat QR code image URL |
| wechat_login | `wechat_login` | boolean | WeChat login enabled |
| server_address | `server_address` | string | Canonical server address |
| turnstile_check | `turnstile_check` | boolean | Cloudflare Turnstile check enabled |
| turnstile_site_key | `turnstile_site_key` | string | Turnstile site key (public) |
| top_up_link | `top_up_link` | string | External top-up URL |
| chat_link | `chat_link` | string | External chat UI URL |
| quota_per_unit | `quota_per_unit` | number | Quota units per 1 USD (default 500000) |
| display_in_currency | `display_in_currency` | boolean | Show prices in currency rather than raw quota |
| oidc | `oidc` | boolean | OIDC login enabled |
| oidc_client_id | `oidc_client_id` | string | OIDC client id (public) |
| oidc_well_known | `oidc_well_known` | string | OIDC discovery URL |
| oidc_authorization_endpoint | `oidc_authorization_endpoint` | string | OIDC authorization endpoint |
| oidc_token_endpoint | `oidc_token_endpoint` | string | OIDC token endpoint |
| oidc_userinfo_endpoint | `oidc_userinfo_endpoint` | string | OIDC userinfo endpoint |
| password_login | `password_login` | boolean | Username/password login enabled |
| password_register | `password_register` | boolean | Username/password registration enabled |

```json
{
  "success": true,
  "message": "",
  "data": {
    "version": "v1.0.0",
    "start_time": 1717804800,
    "email_verification": false,
    "github_oauth": false,
    "github_client_id": "",
    "lark_client_id": "",
    "system_name": "One API",
    "logo": "",
    "footer_html": "",
    "wechat_qrcode": "",
    "wechat_login": false,
    "server_address": "https://oneapi.laisky.com",
    "turnstile_check": false,
    "turnstile_site_key": "",
    "top_up_link": "",
    "chat_link": "",
    "quota_per_unit": 500000,
    "display_in_currency": true,
    "oidc": false,
    "oidc_client_id": "",
    "oidc_well_known": "",
    "oidc_authorization_endpoint": "",
    "oidc_token_endpoint": "",
    "oidc_userinfo_endpoint": "",
    "password_login": true,
    "password_register": true
  }
}
```

**Example**

```bash
curl -s "$BASE_URL/api/status"
```

### GET /api/status/channel

Returns a paginated, sanitized view of channel health and the most recent connectivity-test metrics, used by the public status/monitoring page.

**Auth:** Public - no auth.

**Query parameters**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| p | integer | no | 0 | Zero-based page index; negative values are clamped to 0 |
| size | integer | no | 6 | Page size; values <= 0 fall back to 6 and are capped at the server's max items per page |

**Response:** `200 OK`. Top-level `data` is an array of channel-status objects; `total` is the total channel count for pagination. (Note: this handler does not include a `message` field on success.)

| Field | JSON key | Type | Description |
|-------|----------|------|-------------|
| name | `name` | string | Channel name |
| status | `status` | string | One of `enabled`, `manually_disabled`, `auto_disabled`, `unknown` |
| enabled | `enabled` | boolean | True only when status is `enabled` |
| response_time_ms | `response.response_time_ms` | integer | Last test latency in milliseconds |
| test_time | `response.test_time` | integer | Last test timestamp (Unix seconds) |
| created_time | `response.created_time` | integer | Channel creation timestamp (Unix seconds) |
| total | `total` | integer | Total number of channels across all pages (for pagination) |

```json
{
  "success": true,
  "data": [
    {
      "name": "openai-main",
      "status": "enabled",
      "enabled": true,
      "response": {
        "response_time_ms": 142,
        "test_time": 1717804800,
        "created_time": 1716000000
      }
    },
    {
      "name": "anthropic-backup",
      "status": "auto_disabled",
      "enabled": false,
      "response": {
        "response_time_ms": 0,
        "test_time": 1717800000,
        "created_time": 1716100000
      }
    }
  ],
  "total": 2
}
```

**Example**

```bash
curl -s "$BASE_URL/api/status/channel?p=0&size=6"
```

**Errors**

| Status | Body | Meaning |
|--------|------|---------|
| 500 | `{"success": false, "message": "..."}` | Database error while listing channels or counting them |

### GET /api/models/display

Returns the model catalog grouped by channel, with per-model pricing and capability metadata, for the web UI's model browser. Anonymous callers see all models across all enabled channels; authenticated callers see only the models their user group is allowed to use.

**Auth:** Optional auth (`OptionalUserAuth` never rejects). Authentication, when present, is by **management access token or session cookie** - NOT the relay API key. To authenticate non-interactively, send the access token (the 32-char UUID from `GET /api/user/token`) as `Authorization: $ACCESS_TOKEN` (a leading `Bearer ` is also accepted). Anonymous requests (no header, no cookie) are allowed and return the full catalog. Note: `X-Api-Key` / `Api-Key` headers are NOT consulted by this endpoint.

**Query parameters**

All filters are optional; CSV and repeated parameters are both accepted. Boolean params accept `1`/`true`/`yes`/`on`.

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| keyword | string | no | (none) | Case-insensitive substring match on model name |
| input_modality | string (CSV/repeatable) | no | (none) | Keep models supporting any listed input modality |
| output_modality | string (CSV/repeatable) | no | (none) | Keep models supporting any listed output modality |
| feature | string (CSV/repeatable) | no | (none) | Keep models supporting all listed feature flags |
| reasoning_effort | string (CSV/repeatable) | no | (none) | Keep models supporting any listed reasoning-effort level |
| channel_type | integer (CSV/repeatable) | no | (none) | Restrict to listed channel type ids |
| min_context_length | integer | no | 0 | Minimum context window |
| max_input_price | number | no | 0 | Maximum input price (USD per 1M tokens) |
| has_image | boolean | no | false | Require image pricing or `image` output modality |
| has_video | boolean | no | false | Require video pricing or `video` output modality |
| has_audio | boolean | no | false | Require audio pricing or `audio` input/output modality |
| has_embedding | boolean | no | false | Require embedding pricing |
| has_reasoning | boolean | no | false | Require the `reasoning` feature flag |
| has_tools | boolean | no | false | Require the `tools` feature flag |
| has_web_search | boolean | no | false | Require the `web_search` feature flag |
| has_structured_outputs | boolean | no | false | Require the `structured_outputs` feature flag |

**Response:** `200 OK`. `data` is a map keyed by `"<channelTypeName>:<channelName>"` (e.g. `"openai:openai-main"`); each entry carries the channel metadata and a `models` map keyed by model name. Prices are USD per 1M tokens unless noted. Most capability/metadata arrays and the various nested pricing objects use `omitempty` and are omitted when empty; `input_price`, `cached_input_price`, `output_price`, and `max_tokens` are always present.

| Field | JSON key | Type | Description |
|-------|----------|------|-------------|
| channel_name | `channel_name` | string | The composed `type:name` key |
| channel_type | `channel_type` | integer | Numeric channel type id |
| models | `models` | object | Map of model name to model display info |
| input_price | `input_price` | number | USD per 1M input tokens (always present) |
| cached_input_price | `cached_input_price` | number | USD per 1M cached input tokens (always present; falls back to input price) |
| output_price | `output_price` | number | USD per 1M output tokens (always present) |
| max_tokens | `max_tokens` | integer | Max token limit; `0` means unlimited (always present) |
| context_length | `context_length` | integer | Total context window (omitted if 0) |
| input_modalities | `input_modalities` | string[] | Supported input modalities (omitted if empty) |
| output_modalities | `output_modalities` | string[] | Supported output modalities (omitted if empty) |
| supported_features | `supported_features` | string[] | Capability flags (omitted if empty) |
| image_price | `image_price` | number | USD per image, image models only (omitted if 0) |
| per_call_pricing | `per_call_pricing` | object | Flat per-invocation pricing `{usd_per_thousand_calls, usd_per_call}` (mutually exclusive with token pricing; omitted if absent) |

Other optional nested fields present where applicable (all `omitempty`): `cache_write_5m_price`, `cache_write_1h_price`, `max_output_tokens`, `max_reasoning_tokens`, `supported_sampling_parameters`, `supported_reasoning_efforts`, `default_reasoning_effort`, `quantization`, `hugging_face_id`, `description`, `tiers`, `video_pricing`, `audio_pricing`, `image_pricing`, `embedding_pricing`.

```json
{
  "success": true,
  "message": "",
  "data": {
    "openai:openai-main": {
      "channel_name": "openai:openai-main",
      "channel_type": 1,
      "models": {
        "gpt-4o": {
          "input_price": 2.5,
          "cached_input_price": 1.25,
          "output_price": 10,
          "max_tokens": 0,
          "context_length": 128000,
          "max_output_tokens": 16384,
          "input_modalities": ["text", "image"],
          "output_modalities": ["text"],
          "supported_features": ["tools", "structured_outputs"]
        }
      }
    }
  }
}
```

**Example**

```bash
# Anonymous: full catalog filtered to text-input models with tools
curl -s "$BASE_URL/api/models/display?input_modality=text&has_tools=true"

# Authenticated: only models the caller's user group may use
# (management access token from GET /api/user/token, NOT the relay sk- key)
curl -s "$BASE_URL/api/models/display" \
  -H "Authorization: $ACCESS_TOKEN"
```

### GET /api/tools/display

Returns all enabled MCP servers and their enabled tools (with normalized input schemas and any server-level pricing overrides) for the public tools catalog.

**Auth:** Public - no auth.

**Response:** `200 OK`. `data` is an array of server entries; each entry has a sanitized `server` object (no secrets) and a `tools` array. Servers with no enabled tools are omitted.

| Field | JSON key | Type | Description |
|-------|----------|------|-------------|
| server.id | `server.id` | integer | MCP server id |
| server.name | `server.name` | string | MCP server name |
| server.status | `server.status` | integer | Server status code |
| server.protocol | `server.protocol` | string | Transport protocol |
| tools[].id | `id` | integer | Tool id |
| tools[].server_id | `server_id` | integer | Owning server id |
| tools[].name | `name` | string | Normalized (lowercased) tool name |
| tools[].display_name | `display_name` | string | Human-facing name |
| tools[].description | `description` | string | Tool description |
| tools[].input_schema | `input_schema` | string | JSON-schema string (a `"null"` schema is normalized to an empty string) |
| tools[].default_pricing | `default_pricing` | object | Per-call pricing `{usd_per_call, quota_per_call}` (both `omitempty`) |
| tools[].status | `status` | integer | Tool status (always `1` here; only enabled tools are returned) |
| tools[].created_at | `created_at` | integer | Creation time (Unix ms) |
| tools[].updated_at | `updated_at` | integer | Update time (Unix ms) |

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "server": {
        "id": 3,
        "name": "web-search",
        "status": 1,
        "protocol": "http"
      },
      "tools": [
        {
          "id": 12,
          "server_id": 3,
          "name": "search",
          "display_name": "Web Search",
          "description": "Search the public web",
          "input_schema": "{\"type\":\"object\",\"properties\":{\"query\":{\"type\":\"string\"}}}",
          "default_pricing": {
            "usd_per_call": 0.005
          },
          "status": 1,
          "created_at": 1716000000000,
          "updated_at": 1717804800000
        }
      ]
    }
  ]
}
```

**Example**

```bash
curl -s "$BASE_URL/api/tools/display"
```

**Errors**

| Status | Body | Meaning |
|--------|------|---------|
| 200 | `{"success": false, "message": "Failed to load MCP servers: ..."}` | Listing enabled MCP servers failed (per-server tool-load failures are skipped silently) |

### GET /api/notice

Returns the configured notice/announcement content (the stored `Notice` option) rendered by the web UI.

**Auth:** Public - no auth.

**Response:** `200 OK`. `data` is the raw notice string (may be empty).

```json
{
  "success": true,
  "message": "",
  "data": "Scheduled maintenance on Sunday 02:00 UTC."
}
```

**Example**

```bash
curl -s "$BASE_URL/api/notice"
```

### GET /api/about

Returns the configured "About" content block (the stored `About` option) for the web UI.

**Auth:** Public - no auth.

**Response:** `200 OK`. `data` is the raw about-page string (may be empty; typically Markdown or HTML).

```json
{
  "success": true,
  "message": "",
  "data": "# About\nThis gateway is operated by Example Corp."
}
```

**Example**

```bash
curl -s "$BASE_URL/api/about"
```

### GET /api/home_page_content

Returns the configured homepage content block (the stored `HomePageContent` option) shown on the dashboard landing page.

**Auth:** Public - no auth.

**Response:** `200 OK`. `data` is the raw homepage content string (may be empty; typically Markdown or HTML, or a URL to embed).

```json
{
  "success": true,
  "message": "",
  "data": "Welcome to the gateway. See the docs to get started."
}
```

**Example**

```bash
curl -s "$BASE_URL/api/home_page_content"
```


