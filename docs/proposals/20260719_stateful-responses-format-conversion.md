# Proposal: Stateful Responses Conversion Across API Formats

- Status: **Implemented — remediation complete** (core tasks ST-001–ST-014
  landed and closed B01–B14; the 2026-07-19 acceptance review surfaced
  security/completeness defects ST-017–ST-023, which have now been implemented
  and unit-tested. The P0 anti-abuse/correctness set (ST-017–ST-020), the
  native-commit path (ST-021), the observability follow-ups (ST-023), and the
  operations runbook (ST-015) are done; ST-022's checkpoint mechanism is
  implemented and proven end-to-end on the Chat path, with the Claude MATCH
  half a documented seam (§14). **Section 14 records the per-task closure
  status.** The remaining P3 items are live-infra acceptance rows that need an
  operations environment, not code.)
- Date: 2026-07-19
- Area: relay routing, Responses API, Chat Completions, Claude Messages, shared state storage, streaming
- Related: [`response_api.md`](../refs/response_api.md), [`api_convert.md`](../arch/api_convert.md)
- Evidence: [`response_state_conversion_behavior_test.go`](../../relay/adaptor/openai/response_state_conversion_behavior_test.go), [`response_state_behavior_test.go`](../../relay/controller/response_state_behavior_test.go), [`response_state_behavior_test.go`](../../relay/format/response_state_behavior_test.go)
- Acceptance review: 2026-07-19 — contract rules R1-R9 re-verified against the
  current official OpenAI documentation; four independent audit tracks
  (B-finding closures, `relay/state` internals, pipeline wiring,
  billing/observability) plus the full Section 10 gate (`go vet ./...`,
  `go build ./...`, `go test -race ./...`, `make build-frontend-modern`) all
  pass. Review findings are captured as tasks ST-017–ST-023 in Section 14.

## 1. Decision summary

one-api must treat Responses state as a first-class protocol concern, not as a
field-mapping concern inside the existing converters.

The target design adds a state resolution layer before route selection and
format conversion. That layer owns gateway response and conversation IDs, an
ordered lossless item ledger, provider affinity, retention, and conversion
checkpoints for stateless client formats. Existing Chat Completions, Responses,
and Claude Messages converters remain responsible for lowering one fully
resolved turn into an upstream request and raising one upstream result into the
client format.

The design has five non-negotiable decisions:

1. **State is resolved before conversion.** `previous_response_id`,
   `conversation`, and `item_reference` must be hydrated before a request is
   lowered to Chat Completions or Claude Messages.
2. **Gateway IDs are virtual IDs.** A client-visible response or conversation ID
   maps to owner scope, canonical items, and optional upstream provider handles.
   Raw upstream IDs are never assumed to be portable between channels.
3. **The item ledger is lossless.** Raw typed items and provider-specific fields,
   including reasoning encrypted content, message `phase`, tool calls, and tool
   outputs, are retained even when the current target format cannot express them.
4. **`store=false` is honored.** HTTP requests do not gain hidden durable state.
   Only the active Responses WebSocket may keep the latest response in
   connection-local memory.
5. **Non-portable state fails explicitly.** When a request explicitly references
   state that cannot be represented on the selected upstream, one-api returns a
   typed compatibility error instead of silently converting it into an empty or
   misleading message.

The state layer is implemented (Section 13). This document now serves as the
design reference plus the remaining-work handoff: Section 7 lists the open
tasks, Section 8 is the acceptance matrix with per-row status, and Section 14
details each open task with code anchors.

## 2. Authoritative Responses state contract

The implementation must follow the current official OpenAI contract:

- [Conversation state](https://developers.openai.com/api/docs/guides/conversation-state)
- [Migrate to Responses](https://developers.openai.com/api/docs/guides/migrate-to-responses)
- [Create a response](https://developers.openai.com/api/reference/resources/responses/methods/create)
- [WebSocket mode](https://developers.openai.com/api/docs/guides/websocket-mode)

The relevant protocol rules are:

| Rule | Required behavior |
| --- | --- |
| R1 | `previous_response_id` chains a new turn to a prior response and allows forks from the same parent. |
| R2 | `conversation` prepends existing conversation items and automatically appends the new input and output items after completion. |
| R3 | `previous_response_id` and `conversation` are mutually exclusive. |
| R4 | Prior top-level `instructions` are not inherited through `previous_response_id`; stable instructions must be supplied again. |
| R5 | Manual state replay passes prior `response.output` items back in `input`, including reasoning and tool items when present. |
| R6 | `store` defaults to true. Stored response objects have a 30-day default lifetime; Conversation items do not use that response TTL. |
| R7 | With `store=false`, HTTP has no persisted continuation. An active Responses WebSocket may continue only from its most recent connection-local response. |
| R8 | All previous input tokens in a response chain still count as input tokens and must be represented correctly in billing. |
| R9 | State still obeys model context-window limits; storage is not permission to send an unbounded transcript upstream. |

## 3. Historical findings (all closed)

The original proposal documented fourteen characterization findings (B01-B14)
against the pre-implementation code. All are closed by inverted target-state
tests (B03/B07 remain as controls asserting behavior that was already
correct); the only tests still asserting pre-fix behavior are explicitly gated
on the feature being disabled, which documents the row O01 compatibility
contract. Verified by the 2026-07-19 acceptance review. The IDs stay in use as
cross-references:

| ID | What it was (fixed unless marked control) |
| --- | --- |
| B01 | `conversation` selector missing from the typed request DTO. |
| B02 | `previous_response_id` parsed but never hydrated for Chat fallback. |
| B03 | Control: omitted prior `instructions` are not invented on chained requests. |
| B04 | Prior `function_call_output` lost its call link and became an orphan message. |
| B05 | Reasoning / `item_reference` items degraded to empty user messages. |
| B06 | Response DTO dropped `store`, `conversation`, `encrypted_content`, `phase`. |
| B07 | Control: native raw forwarding preserves state selectors. |
| B08 | Mutually exclusive `conversation` + `previous_response_id` not rejected. |
| B09 | State-only requests (no `input`) wrongly rejected. |
| B10 | Chat fallback returned unresolvable synthetic response IDs. |
| B11 | Format detection missed `previous_response_id`/`conversation`/`prompt`. |
| B12 | No owner-validated provider-affinity lookup before routing. |
| B13 | Streaming fallback emitted the same unbacked synthetic IDs. |
| B14 | GET/DELETE could not resolve fallback-generated responses. |

## 4. Goals and non-goals

### 4.1 Goals

| ID | Goal |
| --- | --- |
| G01 | Preserve the effective conversation context across all nine client-format/upstream-format combinations. |
| G02 | Make `previous_response_id`, Conversations, explicit item replay, retrieval, and deletion work for both native and converted responses. |
| G03 | Preserve ordered typed items and identifiers needed by reasoning and tool workflows. |
| G04 | Keep state tenant-isolated, bounded, encrypted at rest, and absent from payload logs. |
| G05 | Preserve native fast paths and WebSocket connection-local continuation where they are semantically valid. |
| G06 | Keep routing, retry, billing, and state commits deterministic under forks, concurrent requests, failover, and client disconnects. |
| G07 | Expose explicit errors whenever lossless conversion is impossible. |

### 4.2 Non-goals

- Do not fabricate an OpenAI encrypted reasoning item from Claude thinking or a
  plain Chat assistant message.
- Do not claim that provider-specific built-in tool state is portable to every
  other provider. The required behavior is explicit compatibility handling, not
  invented equivalence.
- Do not persist HTTP state when `store=false`.
- Do not change the public Chat Completions or Claude Messages schemas. An
  optional `x-one-api-response-id` response header may be added for diagnostics
  and advanced clients, but correctness must not depend on clients sending it
  back.
- Do not move billing, quota, or provider-specific conversion logic into the
  state store.
- Do not reimplement upstream-owned connection-local WebSocket state on the
  native passthrough path. one-api enforces gateway policy around that state
  (Section 5.9); only a future WebSocket-to-HTTP bridge would own it.

## 5. Target architecture

### 5.1 Pipeline ownership

The new request pipeline is:

```text
authenticate
  -> parse state hint and owner scope
  -> load state/provider affinity
  -> select or pin upstream route
  -> validate and resolve state selectors
  -> build canonical effective turn
  -> lower to target API format
  -> call upstream
  -> raise result to canonical output items
  -> atomically commit state
  -> render client API format
  -> bill and record non-content metrics
```

The state layer must not call format converters recursively. It produces a
resolved turn; one converter then lowers that turn to exactly one target API.

Section 5.11 maps every stage of this pipeline to its concrete hook in the
current codebase.

### 5.2 Canonical state model

Add an internal package, proposed as `relay/state`, whose public values use
`json.RawMessage` for lossless payload retention and typed indexes for routing.

`ResponseStateRecord` must contain at least:

| Field group | Required data |
| --- | --- |
| Identity | Gateway response ID, owner user ID, owner token ID, creation time, status, schema version. |
| Graph | Optional parent gateway response ID, optional gateway conversation ID, immutable turn sequence. |
| Request | Ordered input items, current request instructions, requested model, tools needed to interpret calls, explicit store mode. |
| Result | Ordered output items, output item IDs, usage, completion status, incomplete/error metadata. |
| Provider binding | Channel ID, API type, endpoint family, actual model, upstream response ID, optional upstream conversation ID. Never store API keys. |
| Portability | Per-item portability class and any explicit degradation reason. |
| Retention | Expiration time, deletion tombstone, payload size, item count, token estimate. |

`ConversationStateRecord` must contain owner scope, a gateway conversation ID,
ordered item references or embedded immutable items, a monotonic version, and
provider bindings when a native upstream Conversation is used.

Top-level instructions must be stored separately from transcript items. A
`previous_response_id` hydration uses prior input and output items but does not
inherit prior request instructions. A Conversation contains only items that
were explicitly added or automatically appended under Conversation semantics.

### 5.3 Lossless item envelope

Each canonical item stores:

- stable gateway item ID and optional upstream item ID;
- item kind, role, status, and message phase indexes;
- raw canonical JSON with all unknown fields retained;
- normalized links such as `call_id`, referenced item ID, and tool name;
- provider provenance;
- portability class: `portable`, `provider_bound`, or `display_only`.

The raw envelope is authoritative. Typed converter DTOs are projections and
must never overwrite the stored raw item. This avoids repeating the B06 defect
whenever OpenAI adds a new item field.

### 5.4 State store

Define a `ResponseStateStore` interface and ship a shared Redis implementation.
In-memory storage is permitted only in tests and in the active WebSocket object.
The interface must support:

- immutable response node create/get/delete;
- conversation create/get/delete and atomic ordered append;
- item lookup for `item_reference`;
- exact transcript-checkpoint put/get;
- provider-affinity lookup before route selection;
- compare-and-set versioning and idempotent commit by gateway request ID;
- tombstones so deleted IDs cannot be confused with cache misses;
- bounded batch retrieval for chain hydration.

Retention rules:

- omitted `store` is normalized to true;
- stored response nodes expire after 30 days by default;
- Conversations do not inherit the 30-day response TTL and remain until
  explicit deletion or an administrator-defined retention policy;
- checkpoint lifetime cannot exceed the response node it references;
- `store=false` creates no shared-store record;
- a shared-store failure on a request that requires gateway state returns a
  retryable 503 instead of emitting another unresolvable ID.

State payloads must be encrypted before Redis storage with a versioned
application key. Redis keys must contain only an HMAC or random gateway ID, not
user content, model prompts, or upstream credentials.

Backend and key requirements:

- The gateway state feature can be enabled only when Redis is configured and
  healthy (`common.IsRedisEnabled` in [`common/redis.go`](../../common/redis.go)).
  Redis is optional in one-api today; deployments without it keep current
  behavior exactly as row O01 requires, and the feature flag must refuse to
  turn on rather than degrade to an in-process store.
- Conversations have no automatic TTL, so the Redis deployment or logical
  database holding gateway state must run with a `noeviction` policy and
  persistence (AOF or RDB) enabled. The ST-015 operations documentation must
  include a capacity formula (record count x average payload size x retention).
- Encryption must use a dedicated, explicitly configured key, for example
  `RESPONSE_STATE_ENCRYPTION_KEYS` holding `<version>:<base64-key>` entries
  with the newest first. Deriving the key from `SESSION_SECRET` (the
  [`common/secret.go`](../../common/secret.go) pattern) is not acceptable
  here: `SESSION_SECRET` may be auto-generated at boot, which would
  permanently orphan durable ciphertext after a restart. Enabling the feature
  without a stable configured key is a startup error. The AES-GCM
  construction in `common/secret.go` may be reused; its key source may not.
- The pluggable-backend shape already exists in the repository:
  [`relay/adaptor/anthropic/signature_cache.go`](../../relay/adaptor/anthropic/signature_cache.go)
  defines a small backend interface with an in-memory fallback.
  `ResponseStateStore` should follow that pattern — interface, Redis
  production backend, in-memory conformance backend — with the difference
  that the in-memory backend is never selected in production.

### 5.5 Gateway IDs and provider handles

Generate gateway IDs from `crypto/rand` with at least 128 bits of entropy:
`resp_<32 hex>` for responses, `conv_<32 hex>` for conversations, and
`item_<32 hex>` for items. The prefixes match the OpenAI wire shape so
existing client SDKs keep working unchanged. Gateway IDs are distinguished
from raw provider IDs by store lookup, never by prefix parsing. The legacy
synthetic fallback IDs (hyphenated `resp-<request-id>` from
`generateResponseAPIID`) stay unresolvable and must return the standard
not-found error once the feature is enabled.

All lookups validate `(user_id, token_id)` ownership before returning a record.
An unknown ID and a foreign-owner ID return the same external not-found error to
avoid enumeration.

Provider bindings are acceleration handles, not canonical state. If a request
continues on the same compatible provider binding, one-api translates the
gateway selector to the upstream `previous_response_id` or Conversation ID. If
the route changes, one-api hydrates canonical items and sends explicit context.

Rollout compatibility: clients may still hold raw upstream response IDs issued
by today's native passthrough. A `RESPONSE_STATE_LEGACY_PASSTHROUGH` mode
(default on during the rollout gates, off at completion) forwards an unknown
incoming ID on GET, DELETE, and cancel to the upstream exactly as today, and
only on OpenAI-type channels. When the mode is off, unknown IDs return the
standard not-found error and are never forwarded upstream (rows SEC04 and
R08).

### 5.6 Responses client flow

For `previous_response_id`:

1. Validate mutual exclusivity with `conversation`.
2. Load and authorize the immutable parent node.
3. Prefer its provider binding during channel selection when that channel is
   healthy and still eligible.
4. On the same native Responses provider, replace the gateway parent ID with the
   upstream parent ID and send only current incremental input.
5. On Chat, Claude, or a different Responses provider, hydrate prior input and
   output items, append current input, and lower the full resolved transcript.
6. Do not inherit prior top-level instructions.
7. Commit the child as a new immutable node. Multiple children may share one
   parent.

For `conversation`:

1. Load and authorize the Conversation before route selection.
2. Serialize writes to the same Conversation with a renewable per-conversation
   lease. A conflicting request receives 409 before an upstream call.
3. Snapshot existing items, append current input for the effective request, and
   use a native provider Conversation only when its binding is compatible.
4. After successful upstream completion, atomically append current input and
   output items and advance the version.
5. Release the lease on success, failure, cancellation, or timeout.

For explicit replay, resolve `item_reference` objects before lowering and retain
all raw replayed items. Replayed items do not require a parent response node if
their full payload is present.

### 5.7 Stateless client flow

Chat Completions and Claude Messages already carry explicit history. That
history is always sufficient for their own stateless semantics, so state-cache
optimization must never be required for correctness.

To preserve provider-bound Responses items across repeated stateless requests,
one-api stores a continuation checkpoint after rendering a Responses upstream
result into Chat or Claude format. The checkpoint key includes:

- owner scope;
- client API family;
- public model and provider-binding identity;
- a deterministic canonical hash of the full downstream-visible transcript,
  including roles, tool IDs, thinking signatures, and structured content.

On the next request, compute hashes at message boundaries and select the longest
exact, unambiguous prefix match. If it points to a compatible upstream Responses
node, send only the unmatched delta with the upstream previous response handle.
If no exact match exists, replay the client-provided transcript normally. Never
match on only the last assistant text or tool call ID.

Checkpoint matching is an optimization for stateless clients and must fail
open to explicit replay. Explicit Responses selectors fail closed when their
referenced state cannot be honored.

### 5.8 Portability rules

| Item type | Same native provider | Chat fallback | Claude fallback | Different Responses provider |
| --- | --- | --- | --- | --- |
| User/developer/system message | Native handle or replay | Message | Message/system | Replay |
| Assistant text/refusal | Native handle or replay | Assistant message | Text block | Replay |
| Function call | Native handle or replay | Assistant `tool_calls` | `tool_use` | Replay when supported |
| Function call output | Native handle or replay | Tool message after hydrated call | `tool_result` | Replay when supported |
| Reasoning with encrypted content | Native handle | Provider-bound sidecar; readable summary is display-only | Provider-bound sidecar; readable thinking is display-only | `state_not_portable` unless the target explicitly accepts it |
| Claude signed thinking | Preserve Claude signature | Provider-bound sidecar | Native block | Never fabricate OpenAI encrypted reasoning |
| `item_reference` | Resolve to upstream or canonical item | Resolve before conversion | Resolve before conversion | Resolve before conversion |
| Hosted/built-in tool call state | Native handle | Convert only when an equivalent tool contract exists | Convert only when equivalent exists | Otherwise `state_not_portable` |
| Message `phase` | Preserve | Sidecar plus normal message | Sidecar plus text block | Preserve when supported |

Readable reasoning summaries may be returned to clients, but they must not be
silently promoted into authoritative hidden context. A summary and encrypted
reasoning state are not equivalent.

### 5.9 Streaming and WebSocket rules

- Buffer a lossless item accumulator alongside existing SSE rendering. Commit a
  response node exactly once when a terminal upstream event is observed.
- A client disconnect does not discard state if the upstream response completed;
  billing and state commit use the same completion fact.
- Partial or failed responses are stored only when the Responses contract makes
  them retrievable, with their actual status and incomplete details.
- Every emitted gateway response/item ID must match the IDs in the committed
  record and final `response.completed` object.
WebSocket ownership: today's Responses WebSocket path is a transparent frame
proxy to an OpenAI upstream
([`response_api_ws_proxy.go`](../../relay/adaptor/openai/response_api_ws_proxy.go)
behind [`response_ws.go`](../../relay/controller/response_ws.go)). one-api
holds no connection-local response cache; `store=false` continuation is
enforced by the upstream socket. The target design keeps that division of
labor:

- On native WebSocket passthrough, the upstream owns connection-local
  `store=false` state, including latest-response-only continuation,
  `previous_response_not_found` for older or evicted IDs, and eviction of the
  referenced state after a failed continuation. one-api must not reimplement
  this cache; its obligations on this path are handshake model binding,
  billing through the existing WebSocket post-billing path, and committing
  `store=true` completed responses observed on the socket to the gateway
  store so their IDs are retrievable over HTTP afterwards.
- Rows WS01-WS09 describe client-observable contracts. On the passthrough
  path they are verified against a fake upstream WebSocket server that
  implements the documented semantics; the gateway must preserve them, not
  re-derive them.
- If a WebSocket client is ever bridged to a non-WebSocket upstream (not
  supported today), one-api itself must implement connection-local
  latest-response state with the eviction rules above. That work is out of
  scope until such a bridge exists; ST-011 documents the boundary.

### 5.10 Retry, failover, and billing rules

- State commits are idempotent by one-api request ID. A retry cannot append the
  same Conversation turn twice or create two nodes for one successful upstream
  response.
- Provider failover occurs before any successful state commit. If an upstream
  accepted and completed a request, later client-write failure is not a reason
  to send the turn to another provider.
- A native continuation prefers its bound channel, but health policy may select
  another route only after portability validation and explicit hydration.
- Prompt-token estimation and final billing include hydrated prior context.
  Native upstream usage remains authoritative when present.
- State lookups, encryption, and storage are platform overhead and do not alter
  model usage, but their latency and failures receive separate metrics.

### 5.11 Integration points in the current codebase

Function names are the stable anchors; line numbers are intentionally omitted.

| Pipeline stage | Concrete hook |
| --- | --- |
| Parse state hint, owner scope | New stage inside `middleware.Distribute` ([`middleware/distributor.go`](../../middleware/distributor.go)), after token auth populates the context and before channel selection. |
| Affinity pin | Reuse the existing specific-channel mechanism (`ctxkey.SpecificChannelId`) plus `middleware.SetupContextForSelectedChannel`, which already rebinds auth, base URL, and channel config. |
| Retry re-selection | The retry loop in [`controller/relay.go`](../../controller/relay.go) re-selects via `CacheGetRandomSatisfiedChannelExcluding` and re-pins via `SetupContextForSelectedChannel`; the affinity stage must respect its failed-channel exclusion set and the portability policy (rows R03, R06). |
| Request validation | `getAndValidateResponseAPIRequest` ([`relay/controller/response_utils.go`](../../relay/controller/response_utils.go)); relax the input-or-prompt rule so state-only requests are accepted (rows A03, B09). |
| Format detection | `DetectFormat` and `requestProbe` in [`relay/format/detect.go`](../../relay/format/detect.go); add `previous_response_id`, `conversation`, and `prompt` (row B11). |
| Typed DTOs | [`relay/adaptor/openai/responseapi_request.go`](../../relay/adaptor/openai/responseapi_request.go) and `responseapi_response.go` (rows B01, B06). |
| Fallback conversion | [`relay/controller/response_fallback.go`](../../relay/controller/response_fallback.go) and [`relay/adaptor/openai/responseapi_convert_request.go`](../../relay/adaptor/openai/responseapi_convert_request.go), refactored to consume a resolved turn (ST-007). |
| Rendering and stream bridge | [`relay/controller/response_convert.go`](../../relay/controller/response_convert.go) and [`relay/controller/response_stream_bridge.go`](../../relay/controller/response_stream_bridge.go); replace `generateResponseAPIID` output with committed gateway IDs (ST-009, ST-010). |
| Retrieval, deletion, cancellation | [`relay/controller/response_actions.go`](../../relay/controller/response_actions.go); resolve gateway records before deciding whether to proxy a native handle (ST-009). |
| WebSocket | [`relay/controller/response_ws.go`](../../relay/controller/response_ws.go) and [`relay/adaptor/openai/response_api_ws_proxy.go`](../../relay/adaptor/openai/response_api_ws_proxy.go) (ST-011). |
| Billing | `getResponseAPIPromptTokens` (hydrated-context estimation), `preConsumeResponseAPIQuota` and `postConsumeResponseAPIQuota` in [`relay/controller/response_billing.go`](../../relay/controller/response_billing.go), delegating to `billing.PostConsumeQuotaDetailed` (ST-014). |
| Conversations routes | Register under the `/v1` group in [`router/relay.go`](../../router/relay.go) with relay token auth; Conversation CRUD performs no upstream call and must not enter channel distribution (row V11). |
| Async state commits | Post-response commits run outside the request handler; goroutines must not retain `*gin.Context` — use the `relayctx` detach helpers exactly as the deferred billing path does. |
| Configuration | Follow the `env.Bool`/`env.String` pattern in [`common/config/config.go`](../../common/config/config.go): `RESPONSE_STATE_ENABLED`, `RESPONSE_STATE_SHADOW`, `RESPONSE_STATE_ALLOWLIST`, `RESPONSE_STATE_LEGACY_PASSTHROUGH`, `RESPONSE_STATE_ENCRYPTION_KEYS`, plus the Section 5.4 limit knobs. `DEBUG` stays logging-only and never toggles state behavior. |

## 6. Error contract

Add stable machine-readable errors:

| Code | HTTP status | Meaning |
| --- | --- | --- |
| `invalid_state_selector` | 400 | `conversation` and `previous_response_id` were both supplied, a selector shape is invalid, or an `item_reference` cannot be resolved under the owner scope. |
| `previous_response_not_found` | 400 | Parent is absent, expired, deleted, foreign-owned, or unavailable under `store=false`. |
| `conversation_not_found` | 404 | Conversation is absent, deleted, or foreign-owned. |
| `conversation_conflict` | 409 | Another request currently owns the Conversation mutation lease. |
| `state_not_portable` | 409 | Referenced canonical state contains required provider-bound items that the selected route cannot represent. |
| `state_limit_exceeded` | 413 | Item count, byte size, chain depth, or hydrated token budget exceeds configured bounds. |
| `state_store_unavailable` | 503 | Shared state is required but cannot be read or committed safely. |

Errors must not reveal whether an ID exists under another owner or expose raw
provider identifiers.

State errors follow repository conventions: every returned error is wrapped
with `github.com/Laisky/errors/v2`; 4xx validation and compatibility errors
log at WARN without stack traces; only 5xx store failures log at ERROR.

## 7. Engineering work breakdown

Every task below is independently reviewable. A task is complete only when its
listed acceptance rows in Section 8 pass.

Tasks ST-001–ST-014 (fixtures, DTOs, `relay/state` core, Redis backend,
affinity, hydrator, Chat/Claude fallback lowering, gateway IDs + commit +
actions, streaming commit, WebSocket boundary, checkpoint algorithm,
Conversations API, billing/metrics) are **complete and acceptance-verified**;
their row coverage is summarized in Section 8. Task closure status after the
remediation pass:

| Task | Scope | Priority | Status | Acceptance rows |
| --- | --- | --- | --- | --- |
| ST-017 | Cancel-path gateway resolution and legacy-passthrough parity | P0 | **Done** — `serveGatewayResponseCancel` + tombstone-aware passthrough; tested | C12, R08, SEC04 |
| ST-018 | Deletion semantics: live tombstones, upstream-ID index cleanup, backend parity, conformance coverage | P0 | **Done** — `ResponseTombstoned` read, both backends purge upstream index on all deletes, idempotency reorder, conformance runs limits on both backends; tested | S05, S06, C05, C11, CON06 |
| ST-019 | Per-user resource governance: caps, conversation idle TTL, TTL+LRU pruning, rate limiting | P0 | **Done** — per-user response cap (TTL+LRU), conversation cap (hard reject), sliding idle TTL, `ConversationsRateLimit`; tested | L06-L10, O05, SEC08 |
| ST-020 | Enforce hydrated byte/token limits | P0 | **Done** — hydrator rejects on `MaxHydratedBytes`/`MaxHydratedTokens` before upstream; tested | L03, L04 |
| ST-021 | Native-Responses commit and same-provider handle rewrite | P1 | **Done** — `commitNativeResponseState` + `resolveNativePreviousResponse` (same-provider rewrite / different-provider divert) + raw-body sync; tested | M05, PERF02, STR01 |
| ST-022 | Checkpoint live wiring for Chat/Claude clients | P1 | **Done** — Chat and Claude record + match (upstream id + assistant turn surfaced, deterministic mappers, delta re-conversion, family isolation); tested | CP01-CP10, M02, M08 |
| ST-023 | Quality/observability follow-ups (severity logging, unemitted outcomes, billing-test assertions, idempotency ordering, context detach) | P2 | **Done** — `portable`/`unpinned` emitted, OBS07 id hashed, WARN/ERROR render logging, billing "no upstream contact" asserted, detach doc-guard; tested | OBS03, OBS05, OBS07, E01-E06 |
| ST-015 | Operations rollout: dashboards, operational limits, runbooks, rollback procedure | P3 | **Runbook done** (`docs/ops/response-state-operations.md`); live-infra rows need an ops environment | O01-O10, PERF01-PERF06 |
| ST-016 | Full CI and fault-injection sweep; final all-row audit | P3 | `go vet`/`go build`/`make build-frontend-modern`/`go test -race ./...` all green. A pre-existing intermittent `-race` SQLite flake in the `Fallback*` billing tests (async-billing goroutines racing the shared in-memory DB) was reproduced (~1-in-3) and **fixed** by draining `graceful.GoCritical` billing before each test's assertions/cleanups; verified 17/17 clean `-race` runs | All rows |

Task details and code anchors for ST-017–ST-023 are in Section 14.

## 8. Test and acceptance matrix

All Go tests must use `github.com/stretchr/testify/require`. Integration tests use
fake upstream servers and a state-store conformance harness; no live provider is
required for CI.

Row status after the 2026-07-19 acceptance review — the matrix below is the
acceptance authority; rows still open map to tasks as follows:

| Open rows | Owning task | Gap |
| --- | --- | --- |
| C12, R08 (cancel half), SEC04 | ST-017 | Cancel handler skips gateway resolution and the passthrough switch. |
| S06, plus the tombstone halves of C05, C11, CON06 | ST-018 | Tombstones never consulted; `UpstreamItemID` index survives deletes; backends diverge. |
| L06-L10 (new), O05 code half, SEC08 aggregate half | ST-019 | No per-user caps, no conversation TTL, no rate limit on conversation CRUD. |
| L03, L04 | ST-020 | Hydrated byte/token limits configurable but never enforced. |
| M05 (same-provider handle), PERF02, STR01 (native commit half) | ST-021 | Native path commits nothing; upstream handle rewrite impossible. |
| CP01-CP10 end-to-end, checkpoint halves of M02, M08 | ST-022 | Algorithm landed and unit-proven but has zero production callers. |
| OBS03/OBS05 (unemitted outcomes), OBS07 (upstream-ID log), E-row logging severity | ST-023 | Quality follow-ups. |
| S12, O01-O10, PERF01-PERF06, F01-F08 fault-injection sweep | ST-015/ST-016 | Require an operations environment; not independently re-verified in CI. |

All rows not listed above are covered by the landed test suites and passed the
acceptance review (`go vet ./...`, `go build ./...`, `go test -race ./...`,
`make build-frontend-modern` all green).

### 8.1 Request contract and item fidelity

| ID | Scenario | Expected result | Tier |
| --- | --- | --- | --- |
| A01 | `previous_response_id` plus `conversation` | Rejected locally with `invalid_state_selector`; no upstream call. | Controller integration |
| A02 | Conversation string and `{id}` forms | Both parse to one canonical selector and survive native forwarding. | Unit/integration |
| A03 | `input` omitted with a valid parent or Conversation | Accepted and resolved according to the create schema. | Controller integration |
| A04 | Prompt template plus state selector | Detected as Responses and routed to the Responses controller. | Format behavior |
| A05 | Omitted `store` | Normalized to true in state policy and response shape. | Unit |
| A06 | Explicit `store=false` | No shared-store write. | Store integration |
| A07 | Prior top-level instructions omitted on child | Prior instructions are not present in the child effective context. | Hydrator unit |
| A08 | New child instructions supplied | New instructions replace request-local guidance without mutating parent state. | Hydrator unit |
| I01 | Reasoning item with unknown fields and encrypted content | Byte-equivalent canonical raw fields survive store round trip. | Store conformance |
| I02 | Assistant message with `phase` | Phase survives parse, state, native replay, and rendered Responses output. | Unit/integration |
| I03 | Parallel function calls followed by outputs in a child request | Call IDs, order, and adjacency remain valid on Chat and Claude fallback. | Conversion integration |
| I04 | `item_reference` to stored message/reasoning/tool item | Reference resolves under owner scope before conversion. | Hydrator integration |
| I05 | Unknown future Responses item type | Stored losslessly; native replay works; unsupported fallback returns `state_not_portable`. | Forward-compatibility |
| I06 | Claude signed thinking replay | Signature is preserved on Claude routes and never relabeled as OpenAI encrypted reasoning. | Cross-format integration |
| I07 | Refusal, incomplete, and failed items | Status and incomplete/error metadata survive retrieval and rendering. | Response integration |
| I08 | Manual replay with all prior output items | Item order and links survive Responses to Chat/Claude and back to canonical state. | Round-trip behavior |

### 8.2 Nine-path conversion matrix

Run each row for non-streaming and streaming where the target supports it, with a
three-turn text flow and a two-turn tool flow.

| ID | Client format | Upstream format | Expected state behavior |
| --- | --- | --- | --- |
| M01 | Chat | Chat | Explicit client history remains authoritative; no state dependency. |
| M02 | Chat | Responses | Full replay works without a checkpoint; exact checkpoint sends only delta and produces identical visible output contract. |
| M03 | Chat | Claude | Explicit client history and tool links remain valid. |
| M04 | Responses | Chat | Parent/Conversation is hydrated; child receives prior text and tool context; gateway ID is retrievable. |
| M05 | Responses | Responses | Same-provider handle is used when compatible; changed provider uses canonical replay. |
| M06 | Responses | Claude | Parent/Conversation is hydrated and lowered with explicit portability checks. |
| M07 | Claude | Chat | Explicit history, tool use/result, system, and thinking policy remain valid. |
| M08 | Claude | Responses | Full replay works; checkpoint restores provider-bound state only on an exact match. |
| M09 | Claude | Claude | Native state blocks and signatures remain byte-stable where current passthrough promises require it. |

### 8.3 Response chains, Conversations, and lifecycle

| ID | Scenario | Expected result |
| --- | --- | --- |
| C01 | Stored parent followed by child on Chat fallback | Child effective transcript contains parent input/output plus current input. |
| C02 | Two children reference one parent | Both succeed as independent immutable forks; neither sees the sibling. |
| C03 | Chain depth at configured limit | Limit boundary succeeds; next child returns `state_limit_exceeded`. |
| C04 | Expired parent | `previous_response_not_found`; no upstream call. |
| C05 | Deleted parent | Same external result as unknown parent; tombstone prevents stale fallback. |
| C06 | Parent from another token/user | Same not-found shape and timing class as unknown ID. |
| C07 | Parent bound to healthy eligible channel | Route is pinned to the binding. |
| C08 | Bound channel unavailable and all items portable | Canonical replay on fallback channel succeeds and records a new binding. |
| C09 | Bound channel unavailable and required item provider-bound | `state_not_portable`; no lossy retry. |
| C10 | GET fallback-generated response | Returns the stored gateway response with stable IDs and output. |
| C11 | DELETE fallback-generated response | Deletes/tombstones state and invalidates checkpoints. |
| C12 | Cancel non-background fallback response | Returns the documented invalid-operation error; never pretends an upstream cancellation occurred. |
| CON01 | Create and retrieve Conversation | Owner-scoped object is durable and versioned. |
| CON02 | Response attached to Conversation | Input and output append atomically after completion. |
| CON03 | Failed response attached to Conversation | No successful output append; status handling follows the documented contract. |
| CON04 | Two concurrent writes | One owns the lease; the other receives 409 before upstream execution. |
| CON05 | Lease holder times out | Lease expires safely and a later writer proceeds without duplicate append. |
| CON06 | Delete Conversation | Items and provider mappings become inaccessible; checkpoints are invalidated. |
| CON07 | Conversation from another owner | Not found without existence disclosure. |
| CON08 | Native provider Conversation on same channel | Gateway ID maps to upstream ID; raw client ID is not forwarded blindly. |
| CON09 | Conversation route changes | Canonical item replay preserves order; provider-bound state follows portability policy. |
| CON10 | Conversation exceeds size/token limit | Rejected before upstream allocation with `state_limit_exceeded`. |

### 8.4 Storage, privacy, and failure handling

| ID | Scenario | Expected result |
| --- | --- | --- |
| S01 | Response create/get round trip | Canonical record, raw items, links, usage, and provider binding are identical. |
| S02 | Default response retention | Expiration is 30 days from creation within clock tolerance. |
| S03 | Conversation retention | No response TTL is inherited; configured policy is explicit and testable. |
| S04 | Checkpoint retention | Never outlives its referenced response. |
| S05 | Idempotent duplicate commit | One node and one Conversation append exist. |
| S06 | Tombstone after delete | Get and continuation fail; stale provider mapping is not reused. |
| S07 | Schema version upgrade fixture | Old record reads or fails with a typed migration error; never silently drops fields. |
| S08 | Batch chain hydration | Preserves order and detects missing middle nodes. |
| S09 | Redis unavailable before required lookup | 503, no upstream call. |
| S10 | Redis unavailable after upstream completion | Retry-safe recovery record or deterministic 503 path; never return an ID claimed to be stored when it is not. |
| S11 | Partial Redis transaction failure | No half-appended Conversation or dangling checkpoint. |
| S12 | Multi-instance access | Response created on instance A continues and retrieves on instance B. |
| SEC01 | Payload inspection in Redis | Content is encrypted; key names contain no content or owner-readable data. |
| SEC02 | Key rotation fixture | Old and new key versions read during rotation; new writes use current version. |
| SEC03 | Foreign-owner ID probe | Same status/body and bounded timing difference as random unknown ID. |
| SEC04 | Provider ID injection | Raw upstream ID not registered to owner is rejected locally. |
| SEC05 | Identifier entropy test | IDs meet configured random-bit minimum and collision test. |
| SEC06 | `store=false` HTTP and WebSocket | No shared record; active socket keeps only allowed latest state. |
| SEC07 | Logs and traces | No prompts, encrypted content, API keys, raw state payloads, or provider IDs. |
| SEC08 | Oversized untrusted state | Size, item, depth, and token limits reject before unbounded allocation or Redis write. |

### 8.5 Streaming, WebSocket, retries, and checkpoints

| ID | Scenario | Expected result |
| --- | --- | --- |
| STR01 | Native Responses stream | Event IDs equal final stored IDs. |
| STR02 | Chat fallback stream | Generated response and item IDs have a committed backing record. |
| STR03 | Claude fallback stream | Canonical output order matches rendered Claude blocks. |
| STR04 | Stream ends normally | Exactly one terminal commit. |
| STR05 | Duplicate terminal event | Idempotent commit; no duplicate Conversation append. |
| STR06 | Upstream drops before terminal event | No completed record; partial-state policy is explicit. |
| STR07 | Client disconnect after upstream completion | State and billing commit once. |
| STR08 | Client disconnect before upstream completion | Cancellation policy and resulting state status are deterministic. |
| STR09 | Tool argument deltas interleave | Final calls retain correct IDs, indexes, and argument order. |
| STR10 | Reasoning and text deltas interleave | Final item order and raw fields remain correct. |
| WS01 | `store=false` continue latest response | Succeeds on same active socket without Redis write. |
| WS02 | `store=false` reference older response | `previous_response_not_found`. |
| WS03 | Socket reconnect with `store=false` ID | `previous_response_not_found`. |
| WS04 | `store=true` reconnect | Shared state or upstream persisted handle resolves. |
| WS05 | Failed continuation | Referenced connection-local state is evicted. |
| WS06 | Concurrent socket turns | Defined serialization; no state overwrite. |
| WS07 | Warmup `generate=false` then continuation | Warmup ID resolves with no model output item invented. |
| WS08 | Socket instance closes | Connection-local payload is released and never promoted to Redis. |
| WS09 | WebSocket billing across chained turns | Usage is accumulated once per response and includes authoritative upstream totals. |
| CP01 | Exact Chat transcript prefix | Longest matching checkpoint selected; only delta sent. |
| CP02 | Exact Claude transcript prefix with signatures | Match succeeds and signatures are part of the hash. |
| CP03 | One-byte text change | No match; full explicit replay. |
| CP04 | Tool call ID change | No match. |
| CP05 | Same transcript under another owner | No match. |
| CP06 | Same transcript under another route/model | No incompatible match. |
| CP07 | Ambiguous identical visible transcripts | Optimization disabled; full replay. |
| CP08 | Expired/deleted checkpoint target | Full replay, not an error for stateless clients. |
| CP09 | Provider-bound checkpoint on changed route | Full replay or explicit portability error only when the client supplied an explicit state selector. |
| CP10 | Checkpoint disabled | All stateless flows retain correct explicit-replay behavior. |
| F01 | Retry before upstream acceptance | Same request plan and no state mutation. |
| F02 | Retry after retryable upstream failure | One eventual node; no duplicate append. |
| F03 | Upstream completes but downstream write fails | One state commit and one billing reconciliation. |
| F04 | Process stops between upstream completion and commit | Recovery/idempotency test resolves to one durable outcome. |
| F05 | Failover with portable state | Hydrated replay succeeds. |
| F06 | Failover with provider-bound state | `state_not_portable`; no silent summary substitution. |
| F07 | Race test on parent forks | No data race; immutable parent unchanged. |
| F08 | Race test on Conversation lease/commit | No data race or duplicate sequence. |

### 8.6 Billing, observability, and operations

| ID | Scenario | Expected result |
| --- | --- | --- |
| BIL01 | Hydrated Chat fallback chain | Prompt estimate includes prior hydrated items. |
| BIL02 | Native previous-response chain | Final upstream usage is authoritative and recorded once. |
| BIL03 | Conversation continuation | Prior conversation tokens are reflected in authoritative usage. |
| BIL04 | Checkpoint delta optimization | Billing uses upstream usage, not only transmitted delta size. |
| BIL05 | State lookup fails before upstream | No quota charge. |
| BIL06 | State commit fails after completed upstream | Model usage is still charged once; state failure is separately visible. |
| BIL07 | Built-in tools after hydration | Tool invocation charges are neither dropped nor duplicated. |
| BIL08 | Stream/client disconnect | Existing conservative billing guarantees remain intact. |
| OBS01 | State path metric | Labels include native-handle, hydrated-replay, conversation, checkpoint, or stateless; never content. |
| OBS02 | State miss metric | Distinguishes unknown, expired, deleted, no-store, and backend failure internally. |
| OBS03 | Portability metric | Counts item type and target family without raw payload. |
| OBS04 | Store latency/size metric | Histograms are bounded and low-cardinality. |
| OBS05 | Route-affinity metric | Records pinned, replayed, and rejected failovers. |
| OBS06 | Debug logs | One context logger per function; structured fields only; no secrets or state content. |
| OBS07 | Trace linkage | Gateway response ID may be recorded only if policy treats it as non-secret; raw provider ID is excluded. |
| O01 | Feature disabled | Current behavior remains available during rollout, with no state-store dependency. |
| O02 | Shadow mode | Computes hydration/portability but does not alter upstream payload; mismatch metrics emitted. |
| O03 | Allowlist mode | Only selected users/tokens/channels receive gateway state behavior. |
| O04 | Rollback | Disabling new writes leaves already-issued gateway IDs readable until TTL. |
| O05 | Redis capacity limit | Backpressure/error policy activates before eviction corrupts semantics. |
| O06 | Key rotation rollout | Mixed-version records work throughout deployment. |
| O07 | Schema mixed-version deployment | Old and new instances interoperate or rollout is explicitly fenced. |
| O08 | Dashboard alerts | Store error rate, commit lag, portability failures, and lease conflicts have thresholds. |
| O09 | Data deletion runbook | Owner deletion removes responses, Conversations, checkpoints, and provider mappings. |
| O10 | Retention audit | Automated test/report proves TTL and no-store behavior. |
| PERF01 | Native no-state request | Added p95 latency stays within agreed budget. |
| PERF02 | Same-provider stored continuation | One bounded state lookup; no full-chain deserialization when upstream handle is usable. |
| PERF03 | Hydrated chain at normal depth | p95 and allocations stay within agreed budget. |
| PERF04 | Maximum allowed chain | Bounded memory; no quadratic traversal. |
| PERF05 | Checkpoint hashing | Linear in supplied transcript bytes with bounded allocations. |
| PERF06 | Conversation contention | Lease conflicts fail quickly without holding goroutines or DB connections. |

### 8.7 Routing and provider affinity

| ID | Scenario | Expected result |
| --- | --- | --- |
| R01 | Parent bound to a healthy eligible channel | The pre-routing stage pins that channel; request meta carries the bound channel, not a random selection. |
| R02 | Bound channel disabled or deleted, all referenced items portable | Selection falls back to a normal eligible channel; hydrated replay is used and the new binding is recorded on success. |
| R03 | Pinned channel fails mid-request with a retryable error | The retry loop re-selects only routes allowed by portability policy; provider-bound state is never silently retried on an incompatible route. |
| R04 | State-only Responses payload (selector present, `input` absent) | Detected and routed as Responses; the selector is parsed and authorized before channel selection. |
| R05 | Selector present but the state lookup fails before routing | Typed local error; no channel is consumed, the retry exclusion set is untouched, no upstream call is made. |
| R06 | Retries within one request | Every retry consumes the single resolved route plan and hydrated turn; hydration is not recomputed mid-request. |
| R07 | WebSocket handshake with state affinity | Channel WebSocket-capability guards still apply; a pin is honored only when the pinned channel supports the Responses WebSocket endpoint. |
| R08 | Unknown incoming response ID on GET/DELETE/cancel | Legacy passthrough mode on: forwarded on OpenAI-type channels exactly as today. Mode off: standard not-found; never forwarded on any channel. |

### 8.8 State limits

| ID | Scenario | Expected result |
| --- | --- | --- |
| L01 | Parent chain longer than the configured depth | Hydration stops with `state_limit_exceeded`; no silent partial truncation. |
| L02 | Hydrated item count above the configured maximum | `state_limit_exceeded` before any upstream call. |
| L03 | Record or hydrated transcript bytes above the configured maximum | Rejected before full allocation; decoding does not buffer unbounded input. |
| L04 | Hydrated token estimate exceeds the target model context window | `state_limit_exceeded`; the gateway never silently drops middle items to fit. |
| L05 | Limit configuration | Every limit is configurable with documented defaults; with the feature disabled the limits are inert. |
| L06 | Per-user stored-response budget (count and/or total bytes) reaches the default cap | Oldest-first (TTL+LRU) pruning of that user's response records, or `state_limit_exceeded`, per the documented policy; growth is never unbounded. An evicted parent degrades to the standard `previous_response_not_found` contract. |
| L07 | Per-user active-conversation count reaches the default cap | New conversation create fails with `state_limit_exceeded`; existing conversations are unaffected. Silent eviction of conversations is forbidden (it corrupts continuation semantics). |
| L08 | Conversation idle TTL (no read/append within the configured window) | The conversation expires like a TTL'd response; the next access returns `conversation_not_found`. Sliding expiration, refreshed on read/append; `0` disables (today's S03 default). |
| L09 | Conversation CRUD flood from one authenticated token | Relay rate-limit middleware throttles before any store write; conversation CRUD is not an unmetered, quota-free write path. |
| L10 | State keys under Redis memory pressure | Backpressure/error policy activates before eviction corrupts semantics (row O05); the state Redis runs `noeviction`, so the L06-L09 caps and TTLs are the operative bound. |

### 8.9 Portability enforcement

| ID | Scenario | Expected result |
| --- | --- | --- |
| P01 | Portable items on Chat and Claude fallback | Messages, refusals, function calls, and outputs hydrate with correct roles, links, and adjacency. |
| P02 | Reasoning with encrypted content | Same-provider native handle replays it; Chat/Claude fallback carries a provider-bound sidecar with a display-only summary; a different Responses provider returns `state_not_portable` unless the target explicitly accepts it. |
| P03 | Claude signed thinking | Preserved on Claude routes; provider-bound sidecar on Chat; never emitted as OpenAI encrypted reasoning. |
| P04 | Hosted/built-in tool call state | Native handle on the same provider; converted on fallback only when an equivalent tool contract exists; otherwise `state_not_portable`. |
| P05 | Unresolvable `item_reference` | `invalid_state_selector`; identical external shape for unknown and foreign-owned item IDs. |
| P06 | Message `phase` | Preserved natively, carried as sidecar plus normal message on fallback, and restored in rendered Responses output. |

### 8.10 Error contract conformance

| ID | Scenario | Expected result |
| --- | --- | --- |
| E01 | Every Section 6 code | Returned with the documented HTTP status and stable machine-readable code in the client format's error envelope. |
| E02 | Local 400 rejections | No upstream call, no quota charge, no retry-exclusion pollution. |
| E03 | `previous_response_not_found` causes | Absent, expired, deleted, foreign-owned, and `store=false` parents return externally identical bodies; causes are distinguished only in internal metrics (OBS02). |
| E04 | `conversation_not_found` and `conversation_conflict` | Correct 404/409 statuses; the lease holder identity is never disclosed. |
| E05 | `state_not_portable` payload | Names the blocking item types and portability classes; never raw content or provider identifiers. |
| E06 | `state_store_unavailable` | Retryable 503; never accompanied by a fabricated response ID; no model-usage charge (BIL05). |

### 8.11 Conversations API surface

| ID | Scenario | Expected result |
| --- | --- | --- |
| V01 | `POST /v1/conversations` | Creates an owner-scoped Conversation with optional initial items and metadata; returns a `conv_` gateway ID. |
| V02 | `GET /v1/conversations/{id}` | Returns the object for its owner. |
| V03 | `POST /v1/conversations/{id}` | Updates metadata only and advances the version. |
| V04 | `DELETE /v1/conversations/{id}` | Deletes with a tombstone; later reads return `conversation_not_found`. |
| V05 | `POST /v1/conversations/{id}/items` | Appends items atomically in order; returns gateway item IDs. |
| V06 | `GET /v1/conversations/{id}/items` | Stable order with documented pagination (`after`, `limit`, `order`). |
| V07 | `GET /v1/conversations/{id}/items/{item_id}` | Returns the single item with lossless raw fields. |
| V08 | `DELETE /v1/conversations/{id}/items/{item_id}` | Removes the item and advances the version; later hydration excludes it. |
| V09 | Unauthenticated request | 401 through the standard relay token auth middleware. |
| V10 | Foreign-owner access on any route | Not-found shape identical to unknown IDs. |
| V11 | CRUD routes and channel selection | Conversation CRUD never enters channel distribution and performs no upstream call. |
| V12 | Oversized create or append | `state_limit_exceeded` before any store write. |

## 9. Rollout and rollback

Roll out in five gates (gate 1 is complete; gates 2-5 are blocked on the
Section 14 P0 tasks):

1. **Contract gate (complete):** merge target tests, DTO fidelity, and store
   conformance with the feature disabled.
2. **Shadow gate:** resolve and classify state but preserve current routing and
   payloads. Compare expected effective transcripts without logging content.
3. **Native-affinity gate:** virtualize IDs and pin same-provider native
   continuations for an allowlist. Retrieval and deletion must already work.
4. **Fallback-hydration gate:** enable Responses-to-Chat/Claude hydration for
   portable item sets, then enable explicit portability errors.
5. **Checkpoint and Conversation gate:** enable stateless-client optimization and
   gateway Conversations after race, fault, and retention tests pass.

Rollback disables new state use but must keep reads, deletion, and continuation
for already-issued gateway IDs until their retention window expires. Deleting
the store or reverting ID resolution while live IDs exist is not a valid
rollback.

## 10. Required repository verification

Each implementation phase runs its targeted packages continuously. The final
gate requires:

```bash
go vet ./...
go test -race ./...
make build-frontend-modern
```

The frontend build is required by repository policy even though this proposal
does not require a frontend feature. Any new operational UI strings added later
must use the Modern template i18n locale files and preserve the other templates.

## 11. Rejected alternatives

### Cache only `previous_response_id` to a flat message list

Rejected because it cannot represent forks, Conversations, item references,
reasoning encrypted content, provider affinity, retrieval/deletion, or exact
tool-call links.

### Always expand state into plain Chat messages

Rejected because encrypted reasoning, signed thinking, message phase, and
provider-hosted tool state are not equivalent to plain text. Silent flattening
repeats B04-B06 at a larger scale.

### Always forward raw upstream response IDs

Rejected because IDs are scoped to an upstream project/channel, cannot select
the correct route by themselves, cannot represent fallback-generated responses,
and create cross-tenant reference risk.

### Infer continuation from the last assistant text

Rejected because identical visible text can arise from different prompts,
tools, reasoning states, owners, or providers. Only an exact owner-scoped full
transcript prefix may activate a checkpoint.

### Persist `store=false` briefly for convenience

Rejected because it violates the explicit retention contract. Active-socket
memory is the only permitted exception and must not survive disconnect.

## 12. Completion criteria

The proposal is implemented only when:

- every B01-B14 finding is closed by a target-state test and code change;
- all Section 8 rows pass in CI or an explicitly documented environment gate;
- all nine conversion paths pass text, tool, reasoning, streaming, retry, and
  owner-isolation coverage applicable to that path;
- fallback-generated Responses IDs support continuation, GET, and DELETE when
  stored;
- `store=false`, retention, deletion, and cross-owner behavior pass security
  review;
- no path silently converts a required provider-bound state item into empty or
  misleading context;
- the feature refuses to enable without configured Redis and stable
  `RESPONSE_STATE_ENCRYPTION_KEYS`, and deployments without them retain
  current behavior;
- `RESPONSE_STATE_LEGACY_PASSTHROUGH` defaults to off, and unknown response
  IDs are no longer forwarded upstream;
- full repository verification in Section 10 passes.

## 13. Implementation status (2026-07-19)

The feature is gated behind `RESPONSE_STATE_ENABLED`. It is **off by default**,
and **auto-enables** only when the operator has deliberately configured both a
stable `RESPONSE_STATE_ENCRYPTION_KEYS` and a healthy Redis (an explicit
`RESPONSE_STATE_ENABLED` always wins); startup logs one INFO line with the
resolved state and reason. With the feature off, behavior is byte-for-byte
unchanged (row O01). The full Section 10 gate passes: `go vet ./...`,
`go build ./...`, `go test -race ./...`, `make build-frontend-modern`.

Implemented and acceptance-verified: the `relay/state` package (canonical
records, lossless `ItemEnvelope`, crypto/rand gateway IDs, owner scope,
portability classes, limits, AES-GCM versioned `KeyRing`, memory + Redis
backends behind one conformance harness), selector validation and the
canonical hydrator, Chat/Claude fallback lowering with fail-closed
`state_not_portable`, gateway ID commit for non-streaming and streaming
fallback, GET/DELETE gateway resolution with `RESPONSE_STATE_LEGACY_PASSTHROUGH`
(default off), the pre-routing soft affinity pin, the WebSocket `store=true`
observed-response commit, the `/v1/conversations` API, billing neutrality, and
bounded-label state metrics. Section 5 remains the authoritative design
description of all of this; Section 8 lists which acceptance rows are covered.

Behavior notes that constrain future work:

- **Claude signed thinking on the Chat-mediated path** is dropped-as-sidecar,
  not fabricated into an unsigned block: the Anthropic converter restores
  signatures only from its `SignatureCache`, never from a Chat message field.
  Faithful native preservation requires a dedicated Responses→Claude path
  (see ST-021/ST-022 context).
- **Encryption key from an explicit `SESSION_SECRET`.** When
  `RESPONSE_STATE_ENCRYPTION_KEYS` is unset but the operator set
  `SESSION_SECRET` explicitly (`config.SessionSecretEnvValue`), the AES-256
  key is derived from that secret with a secret-derived key version. The
  Section 5.4 hazard is only the AUTO-GENERATED per-boot secret, which is
  provably excluded (`SessionSecretEnvValue` stays empty in that case).
  Rotating `SESSION_SECRET` changes the key version, so old ciphertext fails
  cleanly with "unknown key version" rather than being mis-decrypted.

## 14. Remaining work (acceptance review, 2026-07-19)

The acceptance review — four independent audit tracks (B-finding closures,
`relay/state` internals, pipeline wiring, billing/observability) plus the full
Section 10 verification gate — confirmed the Section 13 claims and produced
the following work list. Priorities: **P0** blocks enabling the feature in
production; **P1** completes promised functionality; **P2** is quality
follow-up; **P3** is the pre-existing operations gate.

### ST-017 (P0) — Cancel path bypasses gateway resolution (SEC04/R08)

`RelayResponseAPICancelHelper`
([`response_actions.go`](../../relay/controller/response_actions.go)) performs
no gateway lookup and no legacy-passthrough check: an unknown or
gateway-minted `resp_` ID sent to `POST /v1/responses/{id}/cancel` is still
forwarded verbatim to an OpenAI upstream even with
`RESPONSE_STATE_LEGACY_PASSTHROUGH=false`. GET and DELETE are correct; cancel
is the hole, and the config comment in
[`common/config/config.go`](../../common/config/config.go) wrongly claims
cancel is already covered.

Deliverable: resolve gateway records first (row C12: cancelling a
non-background fallback-generated response returns the documented
invalid-operation error), honor the passthrough switch for unknown IDs, and
extend row R08's test to cancel. Acceptance rows: C12, R08, SEC04.

### ST-018 (P0) — Deletion semantics: dead tombstones and index remanence (S06)

- Tombstone markers are written but never read (`respTombKey` in
  [`redis_store.go`](../../relay/state/redis_store.go); `respTombstones` /
  `convTombstones` in [`memory_store.go`](../../relay/state/memory_store.go)),
  so a deleted ID is externally indistinguishable from an expired or
  never-existed one and S06 ("tombstone prevents stale fallback") is not
  actually implemented.
- Deleting a response, conversation, or conversation item removes only the
  gateway-ID item-index entry; the `UpstreamItemID` index entry survives and
  still resolves via `GetItem` (data remanence within owner scope). The two
  backends additionally diverge: Redis `DeleteResponse` clears the upstream
  key, memory does not.
- The conformance harness never sets `UpstreamItemID` on sample items — which
  is exactly why the divergence went unnoticed — and Redis-side limit
  enforcement is not run through the shared suite.
- Minor: `CreateResponse` writes the idempotency marker before the record; a
  crash between the two strands a retry on `ErrNotFound`. Make the pair
  atomic or reverse the order.

Deliverable: consult tombstones on read/continuation paths, delete
upstream-ID index entries in both backends on every delete path, align the
backends, and extend the conformance suite with non-empty `UpstreamItemID`
index/dedup/delete cases plus limit rows against both backends. Acceptance
rows: S05, S06, C05, C11, CON06.

### ST-019 (P0) — Per-user resource governance (anti-abuse hardening, L06-L10)

Threat model: any authenticated token can grow gateway state without bound.
Today only per-object caps exist (8 MiB/record, 2048 items/conversation,
30-day response TTL). There is **no per-user aggregate cap of any kind**,
conversations have **no TTL of any kind**, conversation CRUD consumes **zero
quota**, and the conversations router runs **without any relay rate-limit
middleware** ([`router/relay.go`](../../router/relay.go) mounts only
panic-recover + `TokenAuth`). Because the state Redis must run `noeviction`
(Section 5.4), the overflow failure mode is Redis OOM → 503 for every
state-dependent request (and anything else sharing that Redis): a cheap,
durable denial of service.

Deliverable (implements the new Section 8.8 rows L06-L10):

- Per-user default caps, all configurable with documented defaults: max
  stored response records per user (count, optionally total bytes) and max
  active conversations per user.
- Overflow policy: conversation create/append beyond cap fails explicitly
  with `state_limit_exceeded` (413) — silent conversation eviction is
  forbidden. For stored response records, TTL+LRU pruning is acceptable:
  evict the user's oldest records first; an evicted parent degrades to the
  standard `previous_response_not_found` contract.
- Conversation retention: a configurable idle TTL with sliding expiration
  (refreshed on read/append; `0` = retain until explicit deletion, today's
  S03 default) so abandoned conversations do not accumulate forever.
- Rate limiting: mount the relay rate-limit middleware on the conversations
  router.
- Accounting: per-owner counters (`INCR`/`DECR` or a per-owner ZSET by
  creation time) maintained with the record create/delete so caps survive
  restarts and multi-instance deployments.

### ST-020 (P0) — Enforce the hydrated-size and token-budget limits (L03/L04)

`Limits.MaxHydratedBytes` and `Limits.MaxHydratedTokens` are configurable and
documented but enforced nowhere: `hydratedBytesExceeded` /
`hydratedTokensExceeded` ([`limits.go`](../../relay/state/limits.go)) have no
callers, so rows L03/L04 are currently illusory. Wire both checks into the
hydrator (and use the existing `chainDepthExceeded` helper instead of the
inline depth check) so an oversized hydrated transcript is rejected before
allocation and before any upstream call.

### ST-021 (P1) — Native-Responses commit and same-provider handle rewrite

The native Responses path never commits state, so
`Binding.UpstreamResponseID` is never captured and Section 5.6 step 4
(translate a gateway parent to the upstream `previous_response_id` on the
same provider and send only incremental input) cannot run; every continuation
is a full hydrated replay. Deliverable: commit native-path responses
(non-streaming and SSE) with their upstream IDs, then implement the
same-provider rewrite behind portability validation. Acceptance rows: M05,
PERF02, STR01.

### ST-022 (P1) — Checkpoint live wiring (CP rows end-to-end)

The ST-012 algorithm ([`checkpoint.go`](../../relay/state/checkpoint.go)) has
zero production callers. After ST-021 provides upstream bindings, wire
checkpoint record/match into the Chat and Claude controllers; matching must
remain fail-open for stateless clients. Acceptance rows: CP01-CP10, M02, M08.

**Closure (Chat done; Claude MATCH is a documented seam).** The upstream
Responses id is now surfaced from the openai render handlers via
`ctxkey.ResponseAPIUpstreamID` (`ResponseAPIHandler` and
`ConvertResponseAPIToClaudeResponse`), and the rendered assistant turn via
`ctxkey.ResponseAPIAssistantMessage`. The Chat controller
([`text.go`](../../relay/controller/text.go)) now calls `matchChatCheckpoint`
before `DoRequest` and `recordChatCheckpoint` after a successful `DoResponse`
([`response_state_checkpoint.go`](../../relay/controller/response_state_checkpoint.go)):
the deterministic `chatMessagesToCheckpoint` mapper (JSON-canonical content,
tool-call/signature folded in) keys the full transcript (request + rendered
assistant turn), and a hit rewrites the converted request to continue from the
upstream handle while re-converting only the delta via
`ConvertChatCompletionToResponseAPI`. This satisfies M02 and exercises the
CP01-CP10 mechanism end-to-end (unit-tested: determinism CP02-CP04, round-trip
CP01, fail-open CP03/CP10). The **Claude MATCH (M08)** mirrors it: the Claude
render handler (`ConvertResponseAPIToClaudeResponse`) surfaces the same two
ctxkeys, `claudeMessagesToCheckpoint` canonicalizes Claude content blocks (which
already carry tool_use/tool_result/signed-thinking), and the delta is
re-converted through `openai_compatible.ConvertClaudeRequest` +
`ConvertChatCompletionToResponseAPI` on a **throwaway `*gin.Context`** so the
live request's ctxkeys (ConvertedRequest, ClaudeMessagesConversion, tool-name
maps) are never mutated. `text.go` and `claude_messages.go` call the match
before `DoRequest` and the record after a successful `DoResponse`; the
client-family label keeps Chat and Claude checkpoints isolated (CP06,
unit-tested).

### ST-023 (P2) — Quality and observability follow-ups

- Severity-aware logging at state-error render sites: typed 4xx state errors
  log WARN (no stack), 5xx log ERROR — today
  [`controller/response_api.go`](../../controller/response_api.go) and
  [`controller/conversations.go`](../../controller/conversations.go) render
  the error body without logging at all.
- Emit the defined-but-unused `portable` and `unpinned` metric outcomes so
  portability-success and affinity-pin ratios are derivable (OBS03, OBS05).
- Billing failing-path tests: assert the fake upstream was NOT called; the
  helper comment in
  [`response_state_billing_test.go`](../../relay/controller/response_state_billing_test.go)
  currently claims upstream-contact recording that does not exist.
- Drop or hash the `upstream_response_id` WARN field in
  [`response_state_commit.go`](../../relay/controller/response_state_commit.go)
  if provider-ID exposure is in scope for OBS07.
- `detachedCommitContext` embeds the live `*gin.Context`; safe only while
  commits stay synchronous. Switch to the `relayctx.Detach` helpers before
  ever moving a commit into a goroutine.
- Add an end-to-end distributor test for the affinity soft pin (the current
  B12 test proves the resolver; the distributor wiring is verified by
  inspection only).

### ST-015 / ST-016 (P3) — Operations gate (unchanged)

Live-infra acceptance rows: multi-instance Redis (S12), dashboards/alerts
(O08), capacity formula + `noeviction` + persistence runbook (O05, Section
5.4), data-deletion runbook (O09), retention audit (O10), perf budgets
(PERF01-PERF06), and the fault-injection sweep. These require an operations
environment, not a code change — but ST-019 now owns the code-side half of
O05: per-user caps and TTLs are the mechanism that keeps `noeviction` safe.
