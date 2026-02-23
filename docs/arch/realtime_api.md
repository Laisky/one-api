# OpenAI Realtime API — Architecture & Implementation Guide

## Overview

Goal: Add first-class support for the OpenAI Realtime API via pass-through proxying. A client connects to one-api using a WebSocket at `/v1/realtime?model=...` with a one-api access token. one-api authenticates, selects a channel, records billing/logs, then establishes an upstream WebSocket to the OpenAI Realtime endpoint and pipes events unchanged. No request/response conversion is performed. This mirrors how the Response API is proxied when conversion is not required.

Scope (Phase 1): WebSocket transport only, pass-through of events and media (text/audio). WebRTC and ephemeral token minting are explicitly out of scope for the initial release; see Future Work.

## User Story

As a developer building low-latency, multimodal voice agents:

- I want to connect to one-api at `/v1/realtime?model=gpt-4o-realtime-preview-2025-06-03` using my one-api token so that one-api authenticates me, chooses a valid upstream channel, and transparently forwards my realtime session to OpenAI.
- I expect full event fidelity: whatever I send/receive over the socket is forwarded unchanged so Realtime tool calling, audio frames, deltas, and control events work as in OpenAI’s docs.
- I want usage to be billed and visible in one-api logs/statistics just like other endpoints, using the shared pricing for OpenAI Realtime models (per 1M tokens unit).
- I don’t want to manage upstream credentials in clients; one-api should use the configured channel key, apply rate limits, and centralize observability and quotas.

Non-goals (Phase 1):

- WebRTC peer connection proxying and ephemeral token minting for browser clients.
- Any transformation of frames or events (pure pass-through only).

## Architecture

### High-level flow

Client (WebSocket) → one-api `/v1/realtime` → AuthN/AuthZ → Channel selection → Upstream WS to OpenAI `/v1/realtime` → Full-duplex frame pump → Usage accumulation → Log + bill → Close

Key principles:

- Pass-through: Don’t modify event payloads, message ordering, or binary frames.
- Compatibility: Preserve Realtime subprotocols and headers required by OpenAI.
- Observability: Track lifecycle, errors, and usage to align with standard logs/billing.

## Interfaces and Endpoints

- Public relay entry: `GET /v1/realtime` (WebSocket upgrade)
  - Query: `model` (required). Example: `gpt-4o-realtime-preview-2025-06-03`, `gpt-4o-mini-realtime-preview-2024-12-17`.
  - Headers from client:
    - `Authorization: Bearer <one-api-token>` (required; one-api TokenAuth)
    - `Sec-WebSocket-Protocol: openai-realtime-v1` (recommended; forwarded to upstream if present)
  - one-api applies: CORS, rate limits (global + channel), distribution, channel auth.

- Upstream target: `wss://api.openai.com/v1/realtime?model=<model>`
  - Headers set by one-api:
    - `Authorization: Bearer <upstream-channel-key>`
    - `Sec-WebSocket-Protocol: openai-realtime-v1` (if client requested, mirror/forward)
    - Any additional OpenAI-required headers for Realtime beta (transparent forward if needed)

Note: The OpenAI Realtime API also supports WebRTC and ephemeral tokens. For Phase 1 we only support the WebSocket path and do not issue OpenAI ephemeral tokens from one-api (that would bypass one-api’s logging/billing).

## Technical Implementation

This section maps required changes into this repository’s existing patterns to minimize risk and keep consistency with other adaptors.

### 1) Relay mode plumbing

- Add a new relay mode constant: `Realtime`.
  - File: `relay/relaymode/define.go`
  - Also update `relay/relaymode/helper.go` to map the URL path `/v1/realtime` → `relaymode.Realtime`.
- Why: The router/middleware stack and adaptors rely on `meta.Mode` for routing, rate-limiting, and billing behavior.

### 2) Router and controller

- Router: extend `router/relay.go`
  - In the `/v1` group (protected by TokenAuth + Distribute + rate limits), add a WebSocket endpoint:
    - `relayV1Router.GET("/realtime", controller.RelayRealtime)`
  - Keep consistent middlewares: `RelayPanicRecover`, `TokenAuth`, `Distribute`, `GlobalRelayRateLimit`, `ChannelRateLimit`.

- Controller: implement `controller.RelayRealtime`
  - Responsibilities:
    - Validate `model` query param.
    - Build `meta.Meta` using existing helpers (ensuring `Mode = relaymode.Realtime`).
    - Delegate to the OpenAI adaptor’s Realtime handler which performs the upstream dial and bidirectional pump.
    - Ensure trace timestamps are recorded (upstream start/complete), and errors are wrapped via `github.com/Laisky/errors/v2`.

### 3) OpenAI adaptor (upstream pass-through)

- New handler in `relay/adaptor/openai` (e.g., `realtime.go`) with two main functions:
  - `RealtimeHandler(c *gin.Context, meta *meta.Meta) (*model.ErrorWithStatusCode, *model.Usage)`
    - Upgrade the client connection to a WebSocket (via `gorilla/websocket`).
    - Create an upstream WebSocket client to OpenAI Realtime endpoint with the selected channel’s base URL and API key.
    - Mirror required headers/subprotocols (notably `Sec-WebSocket-Protocol: openai-realtime-v1`).
    - Start full-duplex pumps:
      - Client → Upstream: forward text/binary frames, pong/ping, close frames.
      - Upstream → Client: forward frames identically.
    - Accumulate usage from upstream “completion”/“response.completed” events when present. If usage objects include `input_tokens` and `output_tokens`, sum them into `model.Usage`.
      - If usage is not present, usage remains zero (see Billing). No speculative token counting in Phase 1.
    - On teardown, finalize usage and return.
  - Reuse patterns from existing streaming handlers (see `relay/adaptor/xunfei/main.go`) for pump/cleanup structure.

- URL generation and channel selection:
  - Follow the existing OpenAI adaptor URL logic, but for `relaymode.Realtime` build `wss://.../v1/realtime?model=...` using `meta.BaseURL`.
  - Do not apply ChatCompletion/Response conversions.

### 4) Billing and pricing

- Pricing source: `relay/adaptor/openai/constants.go` already includes realtime models, for example:
  - `gpt-4o-realtime-preview[-2025-06-03]`
  - `gpt-4o-mini-realtime-preview[-2024-12-17]`
    These use the standardized “per 1M tokens” ratios and integrate with the global pricing map.

- Usage accumulation:
  - Prefer upstream-provided usage fields from Realtime events (e.g., final response completed/summary). If present, map to `Usage.PromptTokens` and `Usage.CompletionTokens`; compute `TotalTokens` accordingly.
  - If upstream usage isn’t delivered in the session, bill 0 for Phase 1 (explicit design trade-off to avoid guessing). This is consistent with the philosophy of not over-billing. Operators may configure a minimum charge via channel overrides if desired.
  - Prompt caching (Claude-specific) does not apply to OpenAI Realtime; treat cached fields as zero.

- Logs:
  - Record model, upstream channel, duration, close status, and usage. Keep timestamps in UTC.
  - For data safety, do not persist raw audio or full event payloads; only store minimal metadata needed for debugging (first error string, close codes) and billing.

### 5) Rate limits and quotas

- Existing `GlobalRelayRateLimit` and `ChannelRateLimit` middleware will apply to `/v1/realtime`.
- For long-lived sockets, rate-limit enforcement happens at accept time (connection count) and optionally at message level in the handler if needed. Phase 1 can rely on connection gating only.

### 6) Security

- Authentication: `TokenAuth` is required; only authenticated users can open Realtime sockets.
- Authorization: Standard distribution/selection ensures the user’s token has access to the requested model per policy.
- Header hygiene: Do not echo sensitive upstream headers back to clients. Only pass required subprotocols and Realtime-specific headers.

### 7) Testing strategy

- Unit tests:
  - Mode mapping: `/v1/realtime` → `relaymode.Realtime`.
  - Router integration: ensures the handler is registered and protected by TokenAuth and rate limits.
  - Usage parsing: synthetic upstream events with usage fields map into `model.Usage` correctly.

- Integration tests (optional/CI-guarded):
  - Connect to a local mock WS server standing in for OpenAI; verify bidirectional pump (text + binary frames), correct close handling, and no payload mutation.

### 8) Observability & errors

- Use `github.com/Laisky/errors/v2` for wrapping. No bare errors.
- Trace timestamps: mark upstream start/complete using the same helpers used by other adaptors so the UI shows consistent timings.
- Log upstream close codes and reasons for troubleshooting.

## Compatibility and Edge Cases

- Subprotocol negotiation: If clients specify `Sec-WebSocket-Protocol: openai-realtime-v1`, forward it to upstream; if absent, many clients still connect, but mirroring is recommended.
- Binary frames: Audio and other binary payloads must be forwarded as-is; do not attempt to reinterpret or re-chunk.
- Large/long sessions: Ensure pumps are non-blocking and use bounded buffers to avoid memory spikes. Close gracefully on either side terminating.
- Timeouts: Consider idle/read deadlines to prevent hanging connections; surface as clean closes where possible.
- Errors mid-session: Return an error frame if appropriate, then close; ensure partial usage (if any) is still recorded when upstream sent it.

## Minimal client example (WebSocket)

Pseudo-code (client connects to one-api, not OpenAI directly):

```javascript
const url = 'wss://<your-one-api-host>/v1/realtime?model=gpt-4o-realtime-preview-2025-06-03';
const ws = new WebSocket(url, ['openai-realtime-v1'], {
  headers: { Authorization: `Bearer ${ONE_API_TOKEN}` },
});

ws.onopen = () => {
  // Send Realtime events/frames exactly as OpenAI expects
};
ws.onmessage = (evt) => {
  // Handle Realtime events (text/audio/tool-calls, etc.)
};
ws.onerror = console.error;
ws.onclose = console.log;
```

## Future Work

- WebRTC proxying: Support browser peer connections. This implies session bootstrap endpoints, TURN/ICE negotiation pass-through, and possibly media relaying.
- Ephemeral token minting: Provide a one-api endpoint to mint one-api–scoped ephemeral tokens, not OpenAI ones, to ensure billing/logging still flow through one-api.
- Fine-grained rate limits: Per-message/byte quotas for sustained sessions.
- Enhanced usage heuristics: Optional fallback billing when upstream does not emit usage (configurable caps, duration-based heuristics).

## Acceptance Criteria

- A user can establish a WebSocket session at `/v1/realtime` with a one-api token and model query parameter.
- For supported OpenAI Realtime models in `relay/adaptor/openai/constants.go`, one-api proxies the connection to the correct upstream URL and pipes all frames unchanged.
- one-api records a log entry (UTC timestamps) and bills using upstream-provided usage when available. If usage is absent, no tokens are charged in Phase 1.
- No conversion logic is applied; function/tool events and audio frames retain full fidelity.

## References

- OpenAI Realtime docs (summary in `docs/refs/openai_realtime.md`).
- Existing streaming proxy examples: `relay/adaptor/xunfei/main.go` (WebSocket dial + pump), OpenAI Response API streaming handlers in `relay/adaptor/openai/main.go`.
- Pricing: OpenAI model ratios for realtime models in `relay/adaptor/openai/constants.go` (per 1M tokens standard).
