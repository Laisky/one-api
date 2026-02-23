---
applyTo: '**/*'
---

# Memory

Authoritative, abstract, and current guidance for everyone working in this repository. Keep it lean, remove stale details, and surface subtle decisions that are easy to miss elsewhere.

## Concepts

One‑API is a **single‑endpoint gateway** spanning many AI SaaS providers. Clients may submit OpenAI ChatCompletion, Claude Messages, or Response API payloads interchangeably; adapters normalize traffic, reconcile quotas, and reply in the caller’s format. Advanced features—function calling, tool use, structured or multimodal content, streaming—must behave consistently regardless of the upstream model.

## Claude Prompt Caching & Billing (2025-08)

- Three-bucket billing: normal input, cache-read, and cache-write tokens. Cache-write tokens subtract from normal input to avoid double charges, clamping at zero when necessary.
- Cached completion/output tokens are never billed. `cached_completion_tokens` stays only for compatibility and should remain zero for Claude.
- Pricing config for Claude (and Vertex Claude) must expose `CachedInputRatio`, `CacheWrite5mRatio`, `CacheWrite1hRatio`. Any new Claude model needs these fields.
- Mapping: Anthropic’s `Ephemeral5mInputTokens`, `Ephemeral1hInputTokens`, or legacy `CacheCreationInputTokens` flow into the cache-write buckets.
- Keep backend, API, UI, and docs synchronized. Always run `go test -race ./...`; allow float tolerances in assertions when needed.

## Structured Output & Claude Messages (2025-10)

### Conversion pipeline

- `ConvertClaudeRequest` turns Claude Messages payloads into OpenAI ChatCompletion requests; `HandleClaudeMessagesResponse` reverses the mapping.
- Context markers (`ctxkey.ClaudeMessagesConversion`, `ctxkey.OriginalClaudeRequest`) ensure streaming, quota reconciliation, and logging stay aligned.
- Response API fallback (`relay/controller/response.go`) reuses the same conversion layer. Guard with the adaptor/controller unit suites and `go test -race ./...`.

### Structured-output promotion heuristics

- Promotion to OpenAI `response_format=json_schema` now occurs only when every condition passes:
  1. Exactly one tool with `additionalProperties=false` in its schema.
  2. No existing `tool_use`/`tool_result` messages.
  3. `tool_choice` matches the tool name case-insensitively.
  4. Tool description or conversation text references JSON/structured/schema semantics.
- When promoted we drop `tools`/`tool_choice`, set `response_format` with `strict=true`, and treat the payload as structured output.
- Regression coverage: `TestConvertClaudeRequest_StructuredToolPromoted` and `TestConvertClaudeRequest_ToolNotPromoted`.

### Streaming JSON preservation

- `Message.StringContent` and `Message.ParseContent` now aggregate `output_json` / `output_json_delta` fragments into a single JSON string so Claude SSE conversions can forward structured content. Covered by `TestMessageStringContent_OutputJSON`.

### Provider caveats (E2E sweep 2025‑10‑24)

- **Azure gpt-5-nano:** Structured requests return empty content (even with streams). Current heuristics still promote, leading to “structured output fields missing.” Consider gating promotion per provider/model.
- **OpenAI gpt-5-mini (Claude Structured stream=true):** Upstream emits only usage deltas, no JSON chunks → “stream missing structured output fields.” Needs either improved stream handling or promotion disablement for this combo.
- **DeepSeek Chat (Response API structured):** Rejects `text.format.type=json_schema` with `invalid_request_error`. Today we surface the 400; future improvements may add capability negotiation or bypass structured mode for DeepSeek.

## Response API Fallback & Rewriter (2025-10)

- Non-OpenAI channels receiving `/v1/responses` requests are converted via `ConvertResponseAPIToChatCompletionRequest`, executed as ChatCompletions, then rewrapped through `ctxkey.ResponseRewriteHandler`.
- Text-only inputs collapse back to single message strings; multimodal segments remain structured. Tool definitions, reasoning config, and JSON schema descriptors must round-trip intact.
- Shares billing/quota reconciliation with the ChatCompletion flow. Regression tests live in `relay/adaptor/openai` and `relay/controller`.

## Pricing & Billing Architecture

- Pricing unit is always **per 1M tokens**. Keep backend, UI, and docs aligned.
- `ModelRatios` is the canonical pricing map; do not invent adapter-local overrides.
- Fallback precedence: channel overrides → adapter defaults → global defaults → final fallback. Never bypass this stack.

## Models Display Permissions (2025-09)

- `/api/models/display` exposes only models explicitly configured on each channel. Anonymous users see public models only; authenticated users receive the intersection of channel abilities and group entitlements (deduplicated & sorted before pricing lookups).
- Controller tests isolate SQLite, disable Redis, and reset caches (`anonymousModelsDisplayCache`, `singleflight`). Follow the same pattern to keep tests deterministic.

## General Backend Practices

- Wrap errors with `github.com/Laisky/errors/v2`; never return bare errors. Use `gmw.GetLogger(c)` once per request to retain context.
- Prefer SQL for read-heavy paths; use `gorm.io/gorm` for writes. All timestamps are UTC.
- Context keys are defined in `common/ctxkey/key.go`; do not introduce ad-hoc strings.
- Every code change needs unit tests. No temporary scripts. Run `go test -race ./...` prior to handoff.

## Frontend Snapshot (2025-08)

- All API calls use explicit `/api/...` paths (no Axios baseURL). Verification script: `grep -r "api\.(get|post|put|delete)" web | grep -v "/api/"`.
- Testing stack: Vitest + `jsdom`; always use `vi.mock`/`vi.fn`.
- Responsive tables rely on `useResponsive`, `ResponsivePageContainer`, `AdaptiveGrid`. Mobile layouts use `data-label` and ≥44px tap targets. Login/registration flows enforce stricter TOTP and email validation.

## Recent Developments & Logs (through 2025-10-24)

- Structured-output heuristics and JSON aggregation landed in `relay/adaptor/openai_compatible/claude_messages.go` and `relay/model/message.go`; formatting handled via `gofmt`.
- Regression matrix (160 requests) now passes for previously failing OpenAI Claude tool cases. Known red rows stem from upstream gaps (Azure gpt-5-nano structured, DeepSeek Response structured, OpenAI gpt-5-mini streaming structured Claude).

## Handover Checklist

- Key files: `relay/adaptor/openai_compatible/claude_messages.go`, `relay/model/message.go`, `relay/controller/claude_messages.go`, `docs/arch/api_convert.md`, `docs/arch/billing.md`.
- Test cadence: `go test ./relay/adaptor/openai_compatible ./relay/model`, full `go test -race ./...`, and when conversion logic changes, `API_BASE=... API_TOKEN=... go run -race ./cmd/test`.
- Keep this document synchronized with reality—prune obsolete notes, surface subtle behaviors, and record provider quirks discovered during regression runs.
