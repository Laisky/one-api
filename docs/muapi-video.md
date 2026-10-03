# MuAPI video generation and durable asynchronous tasks

MuAPI implements a model-scoped submit/poll protocol. one-api keeps that native
capability separate from the existing OpenAI video job protocol, and can wait
for a durable task to finish when a caller needs a synchronous result.

## Interfaces and channel setup

| Interface | Behavior |
| --- | --- |
| `POST /v1/async/videos` | Atomically reserve quota and persist a task, then return `202` with a gateway task ID. |
| `GET /v1/async/videos/{id}` | Read the authenticated owner's persisted task and normalized result. Never submits work or charges again. |
| `POST /v1/videos/generations` | For a registered native async provider, wait for its durable task and return final video URLs. Existing synchronous providers keep their existing response behavior on this same endpoint. |
| `/v1/videos` and its existing CRUD routes | Preserve the existing provider-compatible job protocol. MuAPI is not a candidate for these routes. |

Create a channel with type **MuAPI**, base URL `https://api.muapi.ai`, its API key,
and the desired video model slugs. Its native endpoint setting is
**`mu_async_videos`** (`channeltype.MuAsyncEndpointVideos`), not `videos`.
The distributor explicitly bridges that capability to the two creation surfaces
above. Disabling the native capability disables new task admission, including
through the synchronous bridge. First selection and retry selection use the same
capability checks.

The base URL is an administrator-owned HTTP(S) origin, optionally including a
literal proxy prefix and a `/v1` or `/api/v1` suffix. Query strings, fragments,
embedded user credentials, encoded path separators and dot-relative prefixes
are rejected before network I/O. Model and task IDs must be bounded literal path
segments; `.` and `..` are not accepted. Normal dotted/versioned model slugs and
configured private proxy origins remain supported. Only trusted administrators
should configure a provider origin: it determines where the provider API key goes.

The public `async_videos` capability and MuAPI's native `mu_async_videos`
capability have distinct IDs. New relay-mode IDs are appended; existing IDs are
not renumbered. MuAPI is the first implementation of the shared
`asyncvideo.Provider` contract. Other provider-native job protocols are **not**
automatically converted by declaring them compatible: their adapters must
implement the typed submit/poll contract and register a native capability.

Configure current model slugs from MuAPI's catalog rather than treating the
representative discovery list as an allowlist. Creation parameters still follow
the selected model's schema; the gateway normalizes duration aliases and removes
the gateway-only `model` field before the upstream call.

## Synchronous result example

Use a new, stable idempotency key for each intended generation. Keep the **same**
key and request parameters when recovering a disconnected or timed-out request.

```sh
curl --max-time 150 "$ONE_API_URL/v1/videos/generations" \
  -H "Authorization: Bearer $ONE_API_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: example-video-001" \
  -d '{
    "model": "veo3-fast",
    "prompt": "A slow cinematic shot over a mountain lake at sunrise",
    "duration": 8,
    "aspect_ratio": "16:9"
  }'
```

A completed native async task returns HTTP `200`:

```json
{
  "created": 1790956800,
  "data": [{"url": "https://example.com/generated-video.mp4"}]
}
```

The `Location` and `X-Async-Task-Id` response headers identify the persisted task.
The gateway waits 120 seconds by default. A wait timeout returns HTTP `504` with
`error.code = "async_video_wait_timeout"` and `error.task_id`; it does **not** cancel,
refund, or resubmit the task. Repeating the same request with the same
`Idempotency-Key` waits for the original task without another quote or charge.
Alternatively, retrieve it through the explicit task API.

Client, load-balancer, and reverse-proxy timeouts can end the connection before
the gateway can deliver its headers. Persist the idempotency key on the client;
receiving a task ID is not the only way to recover. Do not use a different key
merely because a response was lost.

## Explicit task API example

```sh
curl "$ONE_API_URL/v1/async/videos" \
  -H "Authorization: Bearer $ONE_API_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: example-video-002" \
  -d '{"model":"veo3-fast","prompt":"A mountain lake","duration":8}'
```

HTTP `202` means **one-api has durably admitted the job**, not that the provider
has already accepted or completed it. The initial public state is `queued`:

```json
{
  "id": "at_00000000-0000-0000-0000-000000000000",
  "object": "video.task",
  "model": "veo3-fast",
  "status": "queued",
  "created_at": 1790956800,
  "billing_status": "held",
  "result": null,
  "error": null
}
```

Poll using the returned **one-api ID**, not MuAPI's upstream `request_id`:

```sh
curl "$ONE_API_URL/v1/async/videos/$TASK_ID" \
  -H "Authorization: Bearer $ONE_API_TOKEN"
```

A completed task has `status: "completed"`, `billing_status: "settled"`, and
`result: {"videos": [{"url": "https://example.com/generated-video.mp4"}]}`.
Only this typed result is exposed. Provider IDs, channel identities, input
payloads, upstream cost fields, and arbitrary provider fields are not forwarded.
Result URLs are bounded, require HTTP(S), and cannot contain URL credentials.

Other task states include `running`, `failed`, `cancelled`,
`submission_unknown`, and `reconciliation_required`. A failed task can still have
`billing_status: "held"` while waiting for authoritative refund confirmation.
Public errors have fixed gateway codes; raw upstream responses are not exposed.
Listing, cancellation, deletion, and `/content` downloads are not emulated for
this new contract. Fetch the result URL directly.

## Idempotency and authorization

The database records a unique hash of the owner's immutable identity and the
idempotency key, together with a canonical hash of the original JSON request.
Reordering JSON object fields does not create a different request. Reusing a key
with different request content returns HTTP `409`. Keys are at most 256 bytes.
Identical requests can reattach through either synchronous or asynchronous
creation, even after the selected channel is disabled or the token runs out of
quota. An exhausted token cannot use this exception to create a new task.

A missing key intentionally means an independent generation. Idempotency does
not deduplicate calls across different users, different keys, or expired task
receipts. Completed/refunded receipts are retained for at least 30 days after
financial completion and are pruned only once their usage outbox is acknowledged.

Task access always requires a valid, nonexpired, non-disabled token, an enabled
owner, and the current token's model permissions. Ownership uses numeric ID **and
immutable UUID**. Another valid token of the same owner can retrieve permitted
work. Token deletion does not prevent accounting for already prepaid work, and
refunds never credit a replacement token that happens to reuse the numeric ID.

## Persistence, billing, and recovery

`async_tasks` is the source of truth, not an in-memory goroutine or the old
`AsyncTaskBinding` table. Existing bindings remain available for legacy protocols.
The database migration creates the new task table and an
`async_task_log_receipts` table in the configured log database.

The task path is:

```text
Validate and quote
    -> one transaction: task receipt + full owner/token quota reservation
    -> worker claims a fenced lease
    -> one provider submission
    -> persist provider receipt
    -> safe, repeated GET polling
    -> one transaction: terminal task observation + settlement or confirmed refund
    -> retryable usage-log outbox
```

MuAPI's request-specific `estimate-cost` is obtained before admission. Missing or
invalid positive provider quotes fail closed; explicit administrator pricing
settings remain authoritative. The reservation stores the quoted quota and an
immutable quota/USD + group-ratio conversion snapshot. For dynamic provider
pricing, documented `cost.amount_usd` from submission/polling raises the debit to
at least the actual charge, rounded upward to whole quota units. Later lower or
stale cost observations do not reduce an existing debit. Restart or later group
price edits cannot change the captured conversion. Explicit administrator fixed
per-call/video tariffs keep their configured customer price rather than adopting
the provider's wallet price. The worker refuses to
start reserved work older than five minutes, releasing its reservation because
no provider submission was attempted. This is a gateway queue-age limit, not a
claim that an upstream quote is a guaranteed final invoice.

Full quota is reserved transactionally even for accounts with a large balance;
parallel admission cannot rely on an advisory quota cache. Successful completion
settles that reservation rather than debiting it again. A higher observed dynamic
charge is collected transactionally before a successful result becomes visible.
If the wallet is already exhausted, the supplement becomes explicit negative
balance/debt; it is not silently forgiven. Numeric errors or overflow retain the
hold and withhold success instead of publishing an unaccounted result.

Only a provably pre-dispatch local rejection, or an explicit
`cost.refunded: true` with a matching task ID on a failed/cancelled MuAPI job,
releases funds exactly once. Once an HTTP submission is dispatched, **no HTTP
status alone (including 400/401/402/403/404/413/422/429)** proves that the provider
did not create paid work. A usable accepted ID is preserved even on non-2xx
responses. A known submit charge is preserved even if the task ID is missing.
A failed poll, lost acknowledgement, disconnected socket or HTTP wait timeout is
not evidence of an upstream refund.

The worker pool has bounded concurrency, per-operation deadlines, and fenced
leases. It resumes safe polling after restart, including when an accepted task's
channel is disabled. It reloads credentials rather than copying API keys into
task rows, and fences changes to channel UUID, type, and base URL. Missing or
changed routing for accepted work retains its debit and retries resolution with
bounded backoff, never failing over to a different provider account. If it remains
unresolved at the task-age limit, it requires operator reconciliation. Credential rotation must preserve access to the
original provider account; the gateway cannot independently infer account
identity from a new key.

Expired **submission** leases are not submitted again. A timeout, truncated
success response, provider 5xx, or a database outage after provider acceptance can
leave the task in `submission_unknown`. Polling failures use capped exponential
backoff. Known jobs still unresolved after 24 hours move to
`reconciliation_required`. Outstanding quota holds are never discarded by the
retention sweep.

**Important limitation:** without upstream idempotency or a provider-supported
lookup by a client-generated identifier, a crash between upstream acceptance and
local receipt persistence cannot be resolved automatically with certainty. This
implementation preserves an explicit hold and avoids duplicate paid work; it
does not claim exactly-once upstream execution. These exceptional tasks need
operator reconciliation against the provider's records. This PR does not add an
operator reconciliation UI/API. Do not manually retry submission or issue a
refund without confirming the provider outcome.

Held, unknown and terminal tasks all produce nonzero financial records unless a
refund has actually committed. The outbox updates the **same** consume-log row as
its task moves from held to settled/refunded; it does not sum a hold and final
charge as two purchases. Financial revisions fence delayed workers so stale hold
records cannot overwrite a later supplement or refund. Log delivery and its
receipt share one LOG_DB transaction; the primary acknowledgement is separate
and repeat-safe. Each failed outbox item has independent backoff, so one bad row
or an entire 32-row poison batch cannot indefinitely starve later receipts.
The request-cost surface mirrors the locked current task, not a stale snapshot.
These surfaces are eventually consistent; the primary task/wallet transaction is
the immediate financial authority. Cache refresh or log failure never refunds,
rolls back a committed debit, or replays provider generation.

MuAPI's documented `X-MuAPI-Cost-USD` header is also financial evidence. The
adapter retains the greater valid charge from the body and bounded header values,
using exact decimal comparison. A short, malformed or oversized body cannot erase
an already received header charge. Redirects are not followed: the original
response remains available for its accepted ID/cost, but never authorizes result
publication or a refund merely because it includes a redirect or refund header.
Only a valid matching failed/cancelled body receipt can authorize a refund.

Observed task IDs and maximum USD charges are committed in a fenced evidence
transaction **before** the financial transaction. The private `observed_cost_usd`,
`evidence_pending` and `evidence_version` fields distinguish known evidence from
money that actually committed. A failure during token/user supplemental debit
therefore cannot erase the provider ID or a known higher invoice. No result or
financially completed state is exposed by the evidence transaction.

After lease expiry, a submitting task with a durable provider ID resumes GET
polling, never POST creation. Anonymous pending cost evidence is reconciled without
calling any provider endpoint, then stops being runnable. Completion collects the
largest persisted charge even when a later provider poll omits cost or quotes a
lower amount. Evidence and accounting are both durable only once their respective
transactions commit; a total database outage before evidence storage still needs
provider/operator reconciliation, while the original prepaid reservation remains.

Graceful cancellation gives already observed provider receipts a bounded detached
write inside the joined worker. Hard process termination relies on the database:
prepaid reservations survive; known tasks resume GET polling; ambiguous submitted
work retains its debit and is not re-created. See the
[billing fault matrix](research/20261002_async_video_billing_fault_matrix.md) for
actual socket, SIGKILL, lost-COMMIT and concurrent-accounting tests.

## Runtime settings and validation

| Setting | Default | Range / meaning |
| --- | --- | --- |
| `ASYNC_VIDEO_WAIT_SECONDS` | `120` | `1..600`; maximum synchronous HTTP wait, not provider execution lifetime. |
| `ASYNC_VIDEO_WORKERS` | `4` | `1..32` per replica; database leases coordinate replicas. |

The worker pool participates in application shutdown and is joined before the
database closes. Notification channels only reduce admission latency; database
scanning recovers missed notifications. Tasks accept bounded JSON requests of at
most 1 MiB. Production and regression tests use the same auth, routing, pricing,
transaction, lease, provider, and result-conversion code.

The regression suite covers native/legacy routing isolation, replay vetoes,
original-body preservation before failover, concurrent idempotent reservation,
HTTP disconnect/timeouts, database failure around acceptance, restart and lease
fencing, authoritative refunds, owner/token ID reuse, token removal, split-log
outbox replay, and safe retention. Provider traffic in tests goes to local HTTP
fixtures; validation does not submit a paid MuAPI generation.

## Provider references

- [MuAPI API reference](https://muapi.ai/de/docs/api-reference)
- [MuAPI model catalog](https://api.muapi.ai/api/v1/models)
- [MuAPI authentication](https://muapi.ai/docs/authentication)
- [MuAPI pricing](https://muapi.ai/de/docs/pricing)


### Quote routing consistency

The request-specific quote uses the server-selected channel's configured endpoint
and key. Inconsistent channel ID, UUID, type, account/proxy path, or credentials
in relay metadata are rejected before quoting or reserving a paid task. Custom
administrator-configured proxies remain supported; caller-supplied model names
cannot select the destination authority. This gate reuses the selected channel
snapshot, without an additional database query.

## Safe operational rollback

Stop new admission before rolling back to a release without the durable worker.
Disable the MuAPI native capability/channel for new requests, but retain at least
one worker from this release while already accepted tasks finish polling and
accounting. Disabling an accepted task's channel does not authorize failover or
refund. Existing durable task retrieval remains available to authorized owners.

On the primary database, inspect outstanding work with a read-only query:

```sql
SELECT state, billing_state, COUNT(*) AS tasks
FROM async_tasks
WHERE billing_state = 'held'
   OR evidence_pending = true
   OR log_recorded = false
GROUP BY state, billing_state;
```

Do not remove the last compatible worker while this reports unresolved work.
Tasks with irretrievably missing provider receipts require provider/operator
reconciliation; do not make the query empty by deleting rows or manually clearing
holds. Preserve the primary database, separate log database, task identities,
financial receipts and routing configuration through rollback and backup restore.
This is an operational precondition, not a claim that older binaries can resume
new task records or that a reconciliation administration API is included.


## Scope after the non-adaptor audit

The durable bridge uses `async_tasks` exclusively. Legacy OpenAI/xAI task binding
behavior remains unchanged from the merge base: this feature does not introduce
a second legacy binding retry table, global pending map or detached per-request
retry goroutines. Earlier development revisions included that optional mechanism;
preserve and inspect any existing `async_task_binding_retries` rows before an
experimental deployment switches revisions. This release does not drop that
unused table or silently migrate its contents into the financial task ledger.

An `Idempotency-Key` on a traditional synchronous video request does not impose
the durable JSON/1-MiB schema unless it actually reattaches an existing durable
task. New native async admissions continue to enforce that schema and full quota
reservation. Running previews are not published as final results; output requires
both a completed task and committed settlement. A malformed cost cannot discard
an independently valid accepted task ID. Old unresolved log receipts are retained
without blocking cleanup of later orphan receipts.

See `docs/research/20261002_pr440_nonadaptor_audit.md` for every cross-cutting file,
its necessity, retained boundaries, removed scope and regression mapping.
