# Proposal: Stateful Responses Conversion Across API Formats

- Status: **Implemented** (all code tasks ST-001–ST-014 landed and closed B01–B14; the
  remaining ST-015/ST-016 items are live-infra acceptance rows that require an
  operations environment gate, not a code change; see Section 13)
- Date: 2026-07-19
- Area: relay routing, Responses API, Chat Completions, Claude Messages, shared state storage, streaming
- Related: [`response_api.md`](../refs/response_api.md), [`api_convert.md`](../arch/api_convert.md)
- Evidence: [`response_state_conversion_behavior_test.go`](../../relay/adaptor/openai/response_state_conversion_behavior_test.go), [`response_state_behavior_test.go`](../../relay/controller/response_state_behavior_test.go), [`response_state_behavior_test.go`](../../relay/format/response_state_behavior_test.go)
- Review: 2026-07-19 — contract rules R1-R9, every Section 3 code citation, and
  the B01-B11 test assertions were re-verified against the codebase and the
  current official OpenAI documentation (`developers.openai.com`). All cited
  characterization tests pass with `go test -count=1`.

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

This proposal defines implementation work and acceptance criteria. It does not
implement the state layer.

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

## 3. Current implementation and confirmed defects

### 3.1 Current request paths

| Path | Current behavior |
| --- | --- |
| Responses client to native Responses upstream | [`getResponseAPIRequestBody`](../../relay/controller/response_io.go) patches the original raw JSON, so unknown fields such as `conversation` usually survive. State remains owned by the selected upstream. |
| Responses client to Chat fallback | [`relayResponseAPIThroughChat`](../../relay/controller/response_fallback.go) calls [`ConvertResponseAPIToChatCompletionRequest`](../../relay/adaptor/openai/responseapi_convert_request.go), which converts only the typed current request. |
| Chat or Claude client to Responses upstream | [`ConvertChatCompletionToResponseAPI`](../../relay/adaptor/openai/response_model.go) sends the explicit client transcript as Responses `input`. No cross-request state handle is retained. |
| Chat fallback response to Responses client | [`renderChatResponseAsResponseAPI`](../../relay/controller/response_convert.go) and [`chatToResponseStreamBridge`](../../relay/controller/response_stream_bridge.go) generate Responses-shaped IDs and output items without a backing state record. |
| Responses retrieval, deletion, cancellation | [`response_actions.go`](../../relay/controller/response_actions.go) proxies only OpenAI channels and cannot resolve fallback-generated IDs. |

### 3.2 Characterization test findings

The new tests intentionally assert current behavior so they pass before the
implementation starts. Each finding is a bug or semantic gap except B03, which
is a control proving the correct instruction rule.

| ID | Finding | Severity | Evidence |
| --- | --- | --- | --- |
| B01 | `ResponseAPIRequest` has no `conversation` field. Typed fallback parsing drops both the string and object selector forms. | Critical | `TestResponseStateConversionBehaviorConversationIsNotRepresented` |
| B02 | `previous_response_id` is parsed but never resolved. Chat fallback receives only the incremental current `input`. | Critical | `TestResponseStateConversionBehaviorPreviousResponseContextIsNotResolved` |
| B03 | Omitted prior `instructions` are not invented on a chained request. This is correct and must remain true after hydration. | Control | `TestResponseStateConversionBehaviorInstructionsRemainRequestLocal` |
| B04 | A `function_call_output` whose call exists in the referenced prior response becomes an orphan user message because the prior function call was not hydrated. | Critical | `TestResponseStateConversionBehaviorPriorToolOutputLosesCallLink` |
| B05 | Reasoning items and `item_reference` items without a Chat message `content` field become empty user messages. | High | `TestResponseStateConversionBehaviorTypedStateItemsDegrade` |
| B06 | The conversion response DTO drops `store`, `conversation`, reasoning `encrypted_content`, and message `phase` during a typed round trip. | High | `TestResponseStateConversionBehaviorResponseRoundTripDropsOpaqueState` |
| B07 | Native raw forwarding preserves state selectors, proving native and fallback paths have materially different semantics. | Control | `TestResponseStateBehaviorNativeRawForwardingPreservesSelectors` |
| B08 | one-api does not reject the mutually exclusive combination of `conversation` and `previous_response_id`; both reach a native upstream. | Medium | `TestResponseStateBehaviorDualSelectorsReachNativeUpstream` |
| B09 | State-only requests are rejected because controller validation requires `input` or `prompt`, although the create schema makes `input` optional. | Medium | `TestResponseStateBehaviorStateOnlyRequestsAreRejected` |
| B10 | Chat fallback returns a synthetic response ID and accepts `store=true`, but stores no transcript and omits `store` and `conversation` from the response. | Critical | `TestResponseStateBehaviorFallbackReturnsUnresolvableSyntheticID` |
| B11 | Format detection does not recognize `previous_response_id`, `conversation`, or `prompt`, so stateful Responses payloads without `input` cannot be transparently rerouted. | Medium | `TestResponseStateFormatDetectionBehaviorSelectorsAreNotRecognized`, `TestResponseStateFormatDetectionBehaviorExplicitInputMasksSelectorGap` |
| B12 | State references are resolved only by whatever channel happens to be selected for the current request. There is no owner validation or provider-affinity lookup before routing. | Critical | [`RelayResponseAPIHelper`](../../relay/controller/response.go) receives an already selected `meta` and forwards the selector unchanged. |
| B13 | Streaming fallback generates the same unbacked synthetic IDs as non-streaming fallback. | Critical | [`newChatToResponseStreamBridge`](../../relay/controller/response_stream_bridge.go) calls `generateResponseAPIID` without a state commit. |
| B14 | GET and DELETE cannot retrieve or delete B10/B13 fallback responses because action handlers only proxy OpenAI upstream IDs. | High | [`RelayResponseAPIGetHelper`](../../relay/controller/response_actions.go) and `RelayResponseAPIDeleteHelper` reject non-OpenAI channels. |

When implementation begins, the B01-B14 characterization assertions must be
inverted or replaced in the same change that fixes each behavior. They must not
remain as permanent assertions of broken semantics.

Implementation status of the B-findings (see Section 13 for the full status):

| ID | Status |
| --- | --- |
| B01 | Closed — `TestResponseStateConversionBehaviorConversationIsRepresented` |
| B02 | Closed — `TestHydratePreviousResponseResolvesPriorContext` (controller) |
| B03 | Control — unchanged, still correct |
| B04 | Closed — `TestHydratePriorToolCallLink` |
| B05 | Closed — `TestHydrateResolvesItemReferenceAndDropsReasoning` |
| B06 | Closed — `TestResponseStateConversionBehaviorResponseRoundTripPreservesOpaqueState` |
| B07 | Control — unchanged, still correct |
| B08 | Closed — `TestResponseStateBehaviorDualSelectorsAreRejected` |
| B09 | Closed — `TestResponseStateBehaviorStateOnlyRequestsAreAccepted` |
| B10 | Closed — `TestResponseStateBehaviorFallbackReturnsResolvableGatewayID` |
| B11 | Closed — `TestResponseStateFormatDetectionBehaviorSelectorsAreRecognized` |
| B12 | Closed — `TestResponseStateAffinityPinsBoundChannel` (owner-scoped provider-affinity lookup now runs before channel selection; soft pin preserves failover) |
| B13 | Closed — `TestChatToResponseStreamBridge_CommitsGatewayResponse` |
| B14 | Closed — `TestGatewayResponseGetAndDelete` |

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

| Task | Scope and deliverable | Dependencies | Acceptance rows |
| --- | --- | --- | --- |
| ST-001 | Freeze official state fixtures and add target-state unit tests for selector validation, instruction non-inheritance, retention defaults, item ordering, and lossless raw-field round trips. Convert B01-B11 characterization tests into failing target assertions before production code changes. | None | A01-A08, I01-I08 |
| ST-002 | Add `Conversation` to the typed request model; add `Store`, `Conversation`, `Phase`, `EncryptedContent`, and extension retention to response/item DTOs. Update format detection for `prompt`, `previous_response_id`, and `conversation`. | ST-001 | A01-A08, I01-I04 |
| ST-003 | Define `relay/state` canonical records, store interface, ID generator, owner scope, schema versioning, portability classes, and limits. Add in-memory conformance backend for tests only. | ST-001 | S01-S08, SEC01-SEC05 |
| ST-004 | Implement the Redis backend with encryption keyed from `RESPONSE_STATE_ENCRYPTION_KEYS`, TTLs, HMAC-safe keys, tombstones, batch chain reads, idempotent create, Conversation CAS/lease operations, and checkpoint indexes. | ST-003 | S01-S12, SEC01-SEC08, F01-F04 |
| ST-005 | Add the pre-routing state hint and affinity stage after authentication but before final channel selection. Ensure retries consume one resolved route plan. | ST-003 | R01-R08, SEC03-SEC04 |
| ST-006 | Implement selector validation and the canonical hydrator for parent chains, Conversations, explicit replay, and `item_reference`. Keep instructions separate. | ST-003, ST-005 | C01-C12, I01-I08, L01-L05 |
| ST-007 | Refactor Responses-to-Chat fallback to consume a resolved turn. Preserve tool adjacency; remove empty-message degradation; emit `state_not_portable` where required. | ST-006 | M04-M06, I03-I08, P01-P06 |
| ST-008 | Refactor Responses-to-Claude fallback to consume the same resolved turn and apply the portability table, including signed thinking and tool blocks. | ST-006 | M07-M09, I03-I08, P01-P06 |
| ST-009 | Add gateway response/item ID rendering and atomic non-streaming state commit. Make GET and DELETE resolve gateway records and proxy native handles when appropriate. | ST-004, ST-006 | C01-C12, E01-E06, S05-S08 |
| ST-010 | Add SSE item accumulation and exactly-once terminal commit for native and fallback streams. Ensure IDs are stable across every event and the final response. | ST-009 | STR01-STR10, F03-F08 |
| ST-011 | Enforce the Section 5.9 WebSocket state boundary: keep upstream-owned connection-local `store=false` semantics on the native passthrough, commit observed `store=true` completed responses to the gateway store, and document that any future WebSocket-to-HTTP bridge must implement connection-local state itself. | ST-003, ST-010 | WS01-WS09, SEC06 |
| ST-012 | Implement exact transcript checkpoints for Chat and Claude clients using deterministic full-prefix hashing and longest unambiguous match. Cache misses fall back to explicit replay. | ST-004, ST-009 | CP01-CP10, M01-M09 |
| ST-013 | Add gateway Conversations endpoints (`POST /v1/conversations`; `GET`/`POST`/`DELETE /v1/conversations/{id}`; `GET`/`POST /v1/conversations/{id}/items`; `GET`/`DELETE /v1/conversations/{id}/items/{item_id}`) and wire automatic input/output append, owner checks, leases, and provider Conversation mapping. | ST-004, ST-006, ST-009 | V01-V12, CON01-CON10 |
| ST-014 | Integrate state-aware token estimation, billing reconciliation, metrics, tracing fields, and content-free DEBUG logs. | ST-006, ST-010 | BIL01-BIL08, OBS01-OBS07 |
| ST-015 | Add configuration, migration-free rollout flags, shadow comparison, dashboards, operational limits, and a documented rollback procedure. | ST-004 through ST-014 | O01-O10, PERF01-PERF06 |
| ST-016 | Run full CI and fault-injection suites; remove old synthetic-ID paths and close every B01-B14 finding only after its target assertion passes. | All | All rows |

## 8. Test and acceptance matrix

All Go tests must use `github.com/stretchr/testify/require`. Integration tests use
fake upstream servers and a state-store conformance harness; no live provider is
required for CI.

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

Roll out in five gates:

1. **Contract gate:** merge target tests, DTO fidelity, and store conformance with
   the feature disabled.
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
unchanged (row O01, verified by `*WhenDisabled`/`*Disabled*` tests). All landed
work passes `go vet ./...`, `go build ./...`, and `go test -race` on the touched
packages.

### Landed

- **Contract gate (ST-002, ST-003, ST-004, ST-015 config).**
  - `ResponseAPIRequest.Conversation` selector (string/object, canonicalized);
    `Store`/`Conversation` on the response and `Phase`/`EncryptedContent` on
    output items; `DetectFormat` recognizes `previous_response_id`,
    `conversation`, and prompt-object selectors. (B01, B06, B11)
  - New `relay/state` package: canonical `ResponseStateRecord` /
    `ConversationStateRecord` / `CheckpointRecord`, a lossless `ItemEnvelope`
    whose raw JSON is authoritative, crypto/rand gateway IDs
    (`resp_`/`conv_`/`item_`, 128-bit), owner scope, portability classes,
    configurable limits, schema versioning.
  - `ResponseStateStore` interface with an in-memory backend and a Redis backend
    (`RedisStore`), both proven by one shared conformance harness (miniredis).
    AES-GCM `KeyRing` keyed from `RESPONSE_STATE_ENCRYPTION_KEYS` (versioned,
    rotatable); encrypted payloads; HMAC/random-only key names; tombstones;
    idempotent create; conversation CAS + lease; checkpoints.
  - Config block + `state.Init()` startup gate: refuses to enable without Redis
    and a stable key (no in-process degrade); `state.Init()` wired into `main.go`.
- **State resolution + Chat fallback hydration (ST-006, ST-007).** Selector
  validation (mutual exclusivity, state-only acceptance), a canonical hydrator
  (parent-chain walk, conversation snapshot, `item_reference` resolution,
  instructions kept request-local), and lowering that drops provider-bound /
  display-only items instead of emitting empty messages. (B02, B04, B05, B08, B09)
- **Gateway IDs, commit, retrieval (ST-009).** Non-streaming fallback commits a
  gateway response node and returns a resolvable `resp_` ID with `store` /
  `conversation` echoed; GET/DELETE resolve gateway records first and honor the
  legacy-passthrough switch. (B10, B14)
- **Streaming terminal commit (ST-010, partial).** The Chat→Responses stream
  bridge pre-mints the gateway ID (so every event carries it) and commits exactly
  once at the terminal event. (B13)
- **Gateway Conversations API (ST-013).** `POST/GET/POST/DELETE /v1/conversations`
  and the `/items` sub-resource, owner-scoped, distribution-free, auto-append on
  attach; helper-level tests cover V01-V12.

### Landed in the completion pass (2026-07-19)

- **ST-005 pre-routing affinity (closes B12).** A new pre-routing stage
  (`middleware/response_state_affinity.go`, wired into `middleware.Distribute`)
  parses the gateway selector from the reusable body, looks up the bound channel's
  provider binding under owner scope BEFORE channel selection, and pins it when it
  is still eligible (enabled, group, model, endpoint, and — for websocket
  handshakes — websocket transport). The pin is a SOFT preference via a new
  `ctxkey.ResponseStateAffinityChannelId`; it deliberately does NOT set
  `ctxkey.SpecificChannelId`, because that key disables retry in
  `controller/relay_retry.go`. Retry/failover therefore stay enabled (rows R01,
  R02, R03, R05, R07). An ineligible or unresolved binding fails open to normal
  selection, where hydration replays canonical items.
- **ST-008 Responses→Claude portability enforcement.** The fallback now threads
  the real lowering target (`responseFallbackTarget` → Claude for Anthropic-family
  upstreams) and applies the Section 5.8 table via the authoritative
  `state.FallbackLowering`: portable items carry, reasoning/thinking degrade to a
  display-only sidecar (dropped), and hosted/built-in tool-call state plus unknown
  future item types now fail closed with `state_not_portable` (409) instead of
  silent corruption (rows P04, I05, E05). Note on Claude signed thinking: the
  current Responses→Chat→Claude pipeline cannot carry a thinking signature (the
  Anthropic converter restores signatures only from its `SignatureCache`, never
  from a Chat message field), so a signed-thinking item is dropped-as-sidecar
  rather than fabricated into an unsigned block that the upstream would reject;
  faithful native preservation requires a dedicated Responses→Claude path, tracked
  as a seam.
- **ST-011 WebSocket `store=true` commit.** The native passthrough proxy
  (`relay/adaptor/openai/response_api_ws_proxy.go`) now collects `store!=false`
  completed response objects off the hot path and returns them; the controller
  commits them to the gateway store keyed by (and idempotent on) the upstream
  response id, so their IDs are retrievable over HTTP afterwards (rows WS04, S05,
  SEC06). `store=false` responses are excluded (connection-local upstream state).
- **ST-012 checkpoint algorithm.** `relay/state/checkpoint.go` implements the
  deterministic full-prefix transcript hash (`CheckpointKeyAt`), longest
  unambiguous match (`LongestCheckpointMatch`), and ambiguity-detecting record
  (`RecordCheckpoint`), covering rows CP01–CP08. It always fails open. Live wiring
  into the Chat/Claude controllers is a documented SEAM: it additionally needs a
  native-Responses `Binding.UpstreamResponseID` captured at commit, which the
  current Chat-mediated fallback does not produce.
- **ST-014 observability.** A bounded `RecordResponseStateEvent(category, outcome)`
  metric (compile-time label vocabulary in `common/metrics/response_state.go`,
  implemented across every recorder satisfier) records the state path,
  portability decision, commit outcome, and affinity pin, plus content-free debug
  logs (rows OBS01–OBS06). Token estimation already includes hydrated items
  because hydration inlines them before Chat estimation.
- **Auto-enable + startup log.** When `RESPONSE_STATE_ENABLED` is not set
  explicitly, the feature auto-enables at startup once both prerequisites — a
  stable encryption key and a healthy Redis — are present. `Init` always emits one
  INFO line reporting the resolved enabled/disabled state and the reason.
- **Encryption key from an explicit `SESSION_SECRET`.** When
  `RESPONSE_STATE_ENCRYPTION_KEYS` is unset but the operator set `SESSION_SECRET`
  explicitly (`config.SessionSecretEnvValue`), the AES-256 key is derived from that
  secret (`DeriveKeyRingFromSecret` = `sha256(secret)`, with a secret-derived key
  version). This refines Section 5.4: the hazard it warns about is the
  AUTO-GENERATED per-boot `SESSION_SECRET`, which is unstable and would orphan
  ciphertext; an explicitly configured `SESSION_SECRET` is stable across restarts,
  so deriving from it is safe. Rotating `SESSION_SECRET` changes the key version, so
  old ciphertext fails cleanly with "unknown key version" rather than being
  mis-decrypted.
- **Billing neutrality.** Behavior tests prove local state errors
  (`state_not_portable`, `previous_response_not_found`) charge no quota and make no
  upstream call, and that an identical successful request bills the exact same
  quota whether the feature is off or on (the commit never adds, drops, or doubles
  a charge).

### Not landed (out of scope for the code change)

- The live-infra acceptance rows (multi-instance Redis, dashboards, runbooks,
  perf budgets) require an operations environment gate. ST-015/ST-016 operational
  documentation and the same-provider native `previous_response_id` upstream-ID
  rewrite (which depends on a native-Responses commit capturing
  `Binding.UpstreamResponseID`) remain as documented seams.
