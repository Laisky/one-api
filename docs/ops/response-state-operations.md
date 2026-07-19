# Operations Runbook: Gateway Responses-State Feature

- Area: relay routing, Responses API, shared state storage (Redis)
- Audience: platform operators running one-api in production
- Design reference: [`20260719_stateful-responses-format-conversion.md`](../proposals/20260719_stateful-responses-format-conversion.md)
- Implements: task **ST-015** (operations rollout, dashboards, limits, runbooks, rollback). Covers acceptance rows O01-O10, PERF01-PERF06, S12, SEC01-SEC08, L06-L10.

This runbook is operator-facing. It does not describe internal code paths; it
describes how to enable, size, observe, roll out, roll back, and audit the
gateway Responses-state feature in a live deployment. Every configuration value
and default below is taken directly from
[`common/config/config.go`](../../common/config/config.go).

> Terminology: "gateway state" is the durable, encrypted, Redis-backed store of
> virtual response IDs (`resp_...`), conversations (`conv_...`), item ledgers,
> checkpoints, and provider bindings that one-api owns on behalf of clients. It
> is distinct from any upstream provider's own state.

---

## 1. Overview and enablement

### What the feature is

The gateway state layer lets one-api own Responses-API state (`previous_response_id`
chains, Conversations, item replay, retrieval, and deletion) across all client
and upstream API formats, instead of blindly forwarding raw upstream IDs. When
it is off, one-api behaves exactly as it does today (acceptance **row O01** —
feature disabled means current behavior is unchanged, with no state-store
dependency).

### It is OFF by default

The feature ships **off**. It turns on in only two ways:

1. **Explicit**: `RESPONSE_STATE_ENABLED=true`. An explicit value (`true` or
   `false`) always wins over auto-enable.
2. **Auto-enable**: `RESPONSE_STATE_ENABLED` unset **and** both prerequisites
   present:
   - a **stable** encryption key — either `RESPONSE_STATE_ENCRYPTION_KEYS` is
     configured, or `SESSION_SECRET` was set **explicitly** by the operator (an
     auto-generated per-boot `SESSION_SECRET` does **not** count, because it
     would orphan durable ciphertext on the next restart); **and**
   - a **healthy Redis** (`REDIS_CONN_STRING` configured and the health-check
     ping succeeds).

If `RESPONSE_STATE_ENABLED=true` is set but Redis is unavailable or no stable
key is configured, startup **fails** (the feature refuses to degrade to an
in-process store). It never silently falls back.

### The single startup log line to look for

`Init` emits exactly **one INFO line** at boot describing the resolved state.
Grep your startup logs for `gateway response state`:

| Log message | Meaning |
| --- | --- |
| `gateway response state ENABLED` | Feature is on. Fields: `encryption_key_present`, `redis_ready`, `explicitly_set`, `shadow_mode`, `legacy_passthrough`. |
| `gateway response state DISABLED` | Feature is off. Fields include a content-free `reason` (e.g. `auto-enable prerequisite missing: Redis (REDIS_CONN_STRING)`). |
| `gateway response state DISABLED: initialization error` | Requested on but a prerequisite failed; the process returns a startup error. |

```bash
# confirm the resolved state at boot
grep -m1 "gateway response state" /var/log/one-api/one-api.log
```

The line is bounded and content-free: it never logs a key, a prompt, or a
response ID.

---

## 2. Configuration reference

Every variable below is read once at startup. Names and defaults are
authoritative as of this document; see
[`common/config/config.go`](../../common/config/config.go) for the source of
truth. A **non-positive** value disables that particular bound where the column
says so.

| Environment variable | Default | Meaning |
| --- | --- | --- |
| `RESPONSE_STATE_ENABLED` | `false` (or auto-`true` when Redis + a stable key are both present) | Master switch. Explicit value wins over auto-enable. Startup errors if forced on without prerequisites. |
| `RESPONSE_STATE_SHADOW` | `false` | Shadow mode: compute hydration/portability and emit mismatch metrics, but do **not** alter routing or upstream payloads (row O02). |
| `RESPONSE_STATE_ALLOWLIST` | `""` (all identities in scope) | Comma-separated scope filter. Entries are `user:<id>`, `token:<id>`, `channel:<id>`; a bare number is treated as a user ID (row O03). |
| `RESPONSE_STATE_LEGACY_PASSTHROUGH` | `false` | When on, an **unknown** incoming response ID on GET/DELETE/cancel is forwarded upstream exactly as today (OpenAI-type channels only). When off (default), unknown IDs return the standard not-found error and are never forwarded (rows R08, SEC04). |
| `RESPONSE_STATE_ENCRYPTION_KEYS` | `""` (falls back to an explicit `SESSION_SECRET`; else feature cannot enable) | Versioned AES-256 keys, **newest first**, as `<version>:<base64-key>` entries separated by commas or whitespace. See §3 for rotation (SEC02, O06). |
| `RESPONSE_STATE_MAX_CHAIN_DEPTH` | `64` | Max `previous_response_id` chain depth before `state_limit_exceeded`. Non-positive disables. |
| `RESPONSE_STATE_MAX_ITEM_COUNT` | `2048` | Max hydrated item count per turn / per conversation. Non-positive disables. |
| `RESPONSE_STATE_MAX_RECORD_BYTES` | `8388608` (8 MiB) | Max bytes of a single stored record before rejection. Non-positive disables. |
| `RESPONSE_STATE_MAX_HYDRATED_BYTES` | `33554432` (32 MiB) | Max bytes of a fully hydrated transcript before rejection. Non-positive disables. |
| `RESPONSE_STATE_MAX_HYDRATED_TOKENS` | `1000000` | Max estimated hydrated token budget before `state_limit_exceeded`. Non-positive disables. |
| `RESPONSE_STATE_RESPONSE_TTL_DAYS` | `30` | Default lifetime of a stored response node. Conversations do **not** inherit this TTL (rows S02, S03). |
| `RESPONSE_STATE_MAX_RESPONSES_PER_USER` | `20000` | Per-user cap on stored response records. Overflow prunes the user's **oldest** records first (TTL+LRU); an evicted parent degrades to `previous_response_not_found` (row L06). `0` disables. |
| `RESPONSE_STATE_MAX_CONVERSATIONS_PER_USER` | `2000` | Per-user cap on active conversations. Creating beyond the cap fails with `state_limit_exceeded` (413); existing conversations are untouched. Silent eviction is forbidden (row L07). `0` disables. |
| `RESPONSE_STATE_CONVERSATION_IDLE_TTL_DAYS` | `0` (retain until explicit deletion) | Sliding idle TTL for conversations; every read/append refreshes it. Next access to an expired conversation returns `conversation_not_found` (row L08). `0` disables. |
| `CONVERSATION_RATE_LIMIT` | `240` | Max `/v1/conversations` API calls one authenticated token may make per window (row L09). Non-positive disables. |
| `CONVERSATION_RATE_LIMIT_DURATION` | `60` | Conversations API rate-limit window, in seconds. |

> `DEBUG` is logging-only and never toggles any state behavior. Do not use it to
> influence this feature.

---

## 3. Redis capacity and policy (row O05, Section 5.4)

The state feature requires a healthy Redis and stores durable, encrypted
records there. Because **conversations have no automatic TTL by default**
(`RESPONSE_STATE_CONVERSATION_IDLE_TTL_DAYS=0`), the Redis instance or logical
database holding gateway state must be configured deliberately.

### Required Redis policy

The state Redis MUST run with:

```
maxmemory-policy noeviction
```

and **persistence enabled** — AOF (`appendonly yes`) or RDB snapshots, or both.

Verify:

```bash
redis-cli config get maxmemory-policy      # expect: noeviction
redis-cli config get appendonly            # expect: yes  (or confirm RDB save points)
redis-cli config get save                  # RDB snapshot schedule if AOF is off
```

### Why `noeviction` is mandatory

Under any eviction policy (`allkeys-lru`, `volatile-lru`, etc.), Redis would
silently drop a state key when memory fills. For this feature that is **silent
data loss, not a cache miss**:

- An evicted response node turns a valid continuation into a spurious
  `previous_response_not_found`, or worse, an inconsistent chain.
- An evicted conversation corrupts continuation semantics — items would vanish
  mid-thread with no tombstone, so the gateway cannot even tell a client the
  conversation was deleted versus never existed.
- Eviction is indistinguishable from a legitimate delete, so idempotency and
  ownership guarantees break.

With `noeviction`, a full Redis instead returns errors on write, which one-api
surfaces as a retryable `state_store_unavailable` (503) — a loud, correct
failure rather than silent corruption (row O05).

### What keeps `noeviction` safe

`noeviction` means Redis will not self-trim, so **the per-user caps and TTLs are
the operative upper bound** on growth (rows L06-L10). Keep them enabled:

| Bound | Variable | Effect |
| --- | --- | --- |
| Response records per user | `RESPONSE_STATE_MAX_RESPONSES_PER_USER` (default 20000) | Oldest-first TTL+LRU pruning; growth per user is bounded. |
| Active conversations per user | `RESPONSE_STATE_MAX_CONVERSATIONS_PER_USER` (default 2000) | Hard-reject on create beyond the cap (no silent eviction). |
| Conversation idle TTL | `RESPONSE_STATE_CONVERSATION_IDLE_TTL_DAYS` (default 0 = off) | Sliding expiry of abandoned conversations. **Consider setting this to a non-zero value in production** so idle conversations do not accumulate forever under `noeviction`. |
| Response node TTL | `RESPONSE_STATE_RESPONSE_TTL_DAYS` (default 30) | Response nodes self-expire after 30 days. |
| Conversations rate limit | `CONVERSATION_RATE_LIMIT` / `CONVERSATION_RATE_LIMIT_DURATION` | Throttles the quota-free conversation write path (row L09). |

Without these bounds, any authenticated token could grow gateway state until
Redis OOMs, taking down every state-dependent request (a cheap, durable denial
of service). Do not disable them in production.

### Capacity-planning formula

Estimate the working-set size before sizing `maxmemory`:

```
expected_bytes ≈ (responses_per_user  × avg_record_bytes         × active_users)
              +  (conversations_per_user × avg_conversation_bytes × active_users)
```

- `responses_per_user`, `conversations_per_user` — the *typical* live counts you
  expect, not the caps.
- `avg_record_bytes`, `avg_conversation_bytes` — measure these against your real
  traffic (transcripts with tool calls and reasoning are far larger than plain
  text). Encryption and Redis key overhead add a small fixed amount per key.

Apply a hard **upper bound** using the configured caps:

```
max_possible_bytes ≤ active_users × ( RESPONSE_STATE_MAX_RESPONSES_PER_USER      × RESPONSE_STATE_MAX_RECORD_BYTES
                                    + RESPONSE_STATE_MAX_CONVERSATIONS_PER_USER × avg_conversation_bytes )
```

With defaults, a single user can theoretically hold
`20000 × 8 MiB = 160 GiB` of response records — the caps are generous, so the
*expected* formula (not the worst case) should drive `maxmemory`, and you should
tighten `RESPONSE_STATE_MAX_RESPONSES_PER_USER` / `RESPONSE_STATE_MAX_RECORD_BYTES`
if your `maxmemory` budget is small. Provision `maxmemory` above the expected
working set with headroom, and alert well before it is reached (see §4).

Worked example: 5,000 active users, 50 live responses each at 40 KiB average,
10 live conversations each at 200 KiB average:

```
(50 × 40 KiB × 5000) + (10 × 200 KiB × 5000)
  = 10 GiB (responses) + 10 GiB (conversations)
  ≈ 20 GiB working set  →  provision maxmemory ≈ 30-40 GiB with headroom.
```

### Encryption at rest and key rotation

- State payloads are AES-256-GCM encrypted **before** they reach Redis (SEC01).
  Redis keys contain only opaque gateway IDs — never prompts, model names,
  owner-readable data, or upstream credentials. You can confirm this by
  inspecting keys directly:
  ```bash
  redis-cli --scan --pattern 'resp:*' | head   # keys are opaque IDs only
  redis-cli get <one key>                       # value is ciphertext, not JSON
  ```
- **Key rotation (SEC02, O06)** uses `RESPONSE_STATE_ENCRYPTION_KEYS` as a
  versioned, **newest-first** list. To rotate:
  1. Prepend a new `<version>:<base64-key>` entry, keeping the old entries in
     the list.
  2. Deploy. New writes use the newest key; existing ciphertext still decrypts
     against the older versions during the mixed-version window (row O06).
  3. Once all records older than `RESPONSE_STATE_RESPONSE_TTL_DAYS` have expired
     (and conversations are re-encrypted or deleted), you may drop the retired
     key entry.
- **Do not** rely on `SESSION_SECRET` derivation for durable ciphertext unless
  `SESSION_SECRET` is set explicitly and never auto-generated. Rotating
  `SESSION_SECRET` changes the derived key version, so old ciphertext fails
  cleanly with "unknown key version" rather than being mis-decrypted — that is a
  data-loss event, not a rotation. Prefer explicit `RESPONSE_STATE_ENCRYPTION_KEYS`.

---

## 4. Dashboards and alerts (rows O08, OBS01-OBS07)

### Metric source

All gateway state decisions are recorded on one Prometheus counter:

```
oneapi_response_state_events_total{category, outcome}
```

Both labels come from a fixed compile-time vocabulary — the metric is
**low-cardinality and content-free**. A gateway response/conversation ID, a
prompt, a model name, or an error message is **never** used as a label. Do not
build dashboards that expect such labels.

Label vocabulary:

| Category | Outcomes | Means |
| --- | --- | --- |
| `path` | `hydrated`, `conversation`, `stateless` | How a turn's effective context was resolved. |
| `portability` | `portable`, `sidecar_dropped`, `not_portable` | Per-item lowering decision on a fallback route. |
| `commit` | `committed`, `commit_failed`, `no_store` | Response-node commit outcome (`no_store` = `store=false`, no shared write). |
| `affinity` | `pinned`, `unpinned` | Pre-routing provider-affinity decision. |
| `miss` | `not_found`, `store_error` | State lookup miss classification. |

All outcomes above are emitted as of the ST-023 observability pass: `portable`
on every carried fallback item (OBS03) and `unpinned` on every selector-present
affinity miss (OBS05), so the portability-success and affinity-pin ratios are
directly derivable.

Lease conflicts (`conversation_conflict`, a 409) are **not** on the state
counter; they surface as HTTP status on the conversations routes via
`one_api_http_requests_total{path, method, status_code}`.

### Recommended alerts

Tune thresholds to your traffic; these are starting points.

| Signal | PromQL sketch | Suggested threshold |
| --- | --- | --- |
| **Store error rate** (miss backend failures) | `rate(oneapi_response_state_events_total{category="miss",outcome="store_error"}[5m])` | Page if sustained > 0 for 5m; any nonzero rate means Redis is failing required lookups. |
| **Commit failures** | `rate(oneapi_response_state_events_total{category="commit",outcome="commit_failed"}[5m]) / clamp_min(rate(oneapi_response_state_events_total{category="commit"}[5m]),1)` | Warn > 0.5%, page > 2% of commits failing over 10m. |
| **Commit lag / store health** | Correlate `commit_failed` spikes with `one_api_relay_request_duration_seconds` p95 on Responses paths; a dedicated store-latency histogram (OBS04) is a planned add. | Page on p95 relay latency regression on Responses routes coinciding with commit failures. |
| **Portability failures** | `rate(oneapi_response_state_events_total{category="portability",outcome="not_portable"}[15m])` | Warn on a sustained rise — clients are hitting non-portable state across routes; may indicate a routing/allowlist misconfiguration. |
| **Sidecar drops** | `rate(oneapi_response_state_events_total{category="portability",outcome="sidecar_dropped"}[15m])` | Informational; a spike means reasoning/thinking is being degraded to display-only on fallback. |
| **Affinity pin ratio** | `pinned / (pinned + unpinned)` over the `affinity` category | Track as a gauge; a sudden drop in `pinned` means bound channels are going ineligible (health flapping). |
| **Lease conflicts** | `rate(one_api_http_requests_total{path=~"/v1/conversations.*",status_code="409"}[5m])` | Warn on a sustained rise — concurrent writers contending for the same conversation lease. |
| **Redis memory** (protects `noeviction`) | `redis_memory_used_bytes / redis_memory_max_bytes` | Warn at 70%, page at 85%. Under `noeviction` a full instance 503s all state writes. |

Logging/trace hygiene (OBS06, OBS07): logs use structured fields only and must
not contain prompts, encrypted content, API keys, raw state payloads, or raw
provider IDs. A gateway response ID may appear only where policy treats it as
non-secret; raw upstream provider IDs are excluded.

---

## 5. Rollout gates (Section 9)

Advance through five gates in order. Do not skip: each gate depends on the
previous one being observably healthy. Gate 1 is complete in code; gates 2-5 are
operator-driven and depend on the Section 14 P0 tasks being done.

| Gate | What it enables | How to turn on | What to observe before advancing |
| --- | --- | --- | --- |
| **1. Contract** | Target tests, DTO fidelity, store conformance — **feature disabled**. | Nothing; baseline. Confirm the startup line reads `DISABLED` and behavior matches row O01. | CI green; no state-store dependency. |
| **2. Shadow** | Resolve and classify state, but keep current routing/payloads. | `RESPONSE_STATE_SHADOW=true` (with the feature enabled). | `oneapi_response_state_events_total{category="path"}` populates; mismatch metrics stay low; **no** change to upstream payloads or latency (PERF01). |
| **3. Native-affinity** | Virtualize IDs; pin same-provider native continuations for an allowlist. | `RESPONSE_STATE_ALLOWLIST=user:<id>,...` (start with a few internal users/tokens). | `affinity` `pinned` events appear; GET/DELETE on gateway IDs already work; retrieval and deletion verified for the allowlisted scope. |
| **4. Fallback-hydration** | Responses-to-Chat/Claude hydration for portable item sets, then explicit portability errors. | Widen `RESPONSE_STATE_ALLOWLIST`; keep monitoring `portability`. | `path=hydrated` events succeed; `not_portable` only where genuinely non-portable; billing includes hydrated context (BIL01). |
| **5. Checkpoint + Conversations** | Stateless-client checkpoint optimization and the gateway Conversations API. | Remove the allowlist (empty = all) after race/fault/retention tests pass; confirm `CONVERSATION_RATE_LIMIT` is set. | Conversation lease conflicts (409) are rare and self-clearing; retention audit (§8) passes; no Redis memory-pressure alerts. |

Roll each gate to a small allowlist first, watch the dashboards for at least one
full traffic cycle, then widen. Any store-error or commit-failure alert is a
stop-and-investigate signal.

---

## 6. Rollback (Section 9, row O04)

Rolling back means **stopping new state use while keeping already-issued gateway
IDs usable until they expire**. It does not mean deleting data.

### Valid rollback

1. Set `RESPONSE_STATE_ENABLED=false` (or move the allowlist back to a narrow
   scope to shrink the blast radius).
2. Keep Redis and the encryption keys **in place and unchanged**. Reads,
   deletion, and continuation for IDs already issued continue to work until each
   record's TTL expires (`RESPONSE_STATE_RESPONSE_TTL_DAYS`, and conversation
   idle TTL where configured).
3. Let the TTL window drain naturally before decommissioning the store.

Because `RESPONSE_STATE_LEGACY_PASSTHROUGH` defaults **off**, an unknown ID is
never forwarded upstream during or after rollback — it returns the standard
not-found error. If you need old raw upstream IDs to keep working on OpenAI-type
channels during a transition, set `RESPONSE_STATE_LEGACY_PASSTHROUGH=true`
deliberately and only for the transition window.

### NOT a valid rollback

- **Deleting the Redis store** while live gateway IDs exist. Clients holding
  `resp_...`/`conv_...` IDs would get not-found for state they legitimately own.
- **Reverting ID resolution** (going back to raw synthetic IDs) while gateway
  IDs are in flight. The two ID schemes are not interchangeable.
- **Rotating away the only encryption key** — that orphans all ciphertext.

Row O04: disabling new writes must leave already-issued gateway IDs readable
until their retention window expires.

---

## 7. Data-deletion runbook (row O09)

To remove **all** gateway state for a user or owner on request (e.g. account
deletion, GDPR erasure), delete across every state class for that owner scope.
Deletes are **tombstoned**, and item indexes (both the gateway ID and the
upstream ID) are purged so nothing resolves after deletion.

What must be removed for an owner:

| State class | What to delete |
| --- | --- |
| Response records | Every `resp_...` node owned by the user/token, including chained children. |
| Conversations | Every `conv_...` object and its ordered item ledger. |
| Conversation items | All items under those conversations (gateway item IDs). |
| Checkpoints | Every stored continuation checkpoint keyed to that owner scope. |
| Provider mappings | Provider bindings and both the gateway-ID and upstream-ID item-index entries. |

Procedure:

1. Prefer the API surface where it exists — `DELETE /v1/responses/{id}`,
   `DELETE /v1/conversations/{id}`, `DELETE /v1/conversations/{id}/items/{item_id}`
   — because those paths write tombstones and invalidate dependent checkpoints
   correctly. A deleted parent then returns the same external result as an
   unknown parent (rows C05, C11, CON06), preventing stale fallback.
2. For a bulk owner wipe, enumerate the owner's keys and delete them, ensuring
   the **upstream-ID index entry is removed too** (not only the gateway-ID
   entry), so no item resolves via its upstream handle afterward.
3. Verify: a subsequent GET on any deleted `resp_`/`conv_` ID returns not-found;
   continuation against a deleted parent returns `previous_response_not_found` /
   `conversation_not_found`; and no checkpoint replays the deleted transcript.

> Note (development status): ST-018 tracks completing tombstone consultation and
> upstream-ID index cleanup across both backends. Until it lands, verify
> deletions with the checks in step 3 rather than assuming index purge.

---

## 8. Retention audit (row O10)

Periodically prove that TTL and `store=false` behavior actually hold. Each check
below is a black-box assertion you can automate.

| Property | Expected | How to verify |
| --- | --- | --- |
| Response node TTL (S02) | A stored response expires ~`RESPONSE_STATE_RESPONSE_TTL_DAYS` (30 by default) after creation, within clock tolerance. | Create a response with `store=true`; check the Redis key TTL (`redis-cli ttl <key>`) is ≈ 30 days; confirm GET returns not-found after expiry (use a short TTL in a staging run). |
| Conversation TTL (S03, L08) | Conversations do **not** inherit the 30-day response TTL. With idle TTL off (`=0`) they persist until deletion; with idle TTL set, they slide on read/append and expire only when abandoned. | Create a conversation, confirm no 30-day TTL is attached; with idle TTL set, touch it and confirm the TTL refreshes; leave it idle past the window and confirm the next access returns `conversation_not_found`. |
| Checkpoint retention (S04) | A checkpoint never outlives the response node it references. | Confirm checkpoint TTL ≤ the referenced response node TTL. |
| `store=false` (A06, SEC06) | No shared-store record is written. | Send a Responses request with `store=false`; confirm **no** `resp_...` key appears in Redis and the `commit` metric shows `no_store`, not `committed`. |

Run this as a scheduled synthetic test against a non-production instance, or as
a periodic assertion job. Row O10 requires an automated test/report, not a
manual spot-check.

---

## 9. Multi-instance operation (row S12)

Gateway state lives in **shared Redis**, so it is not instance-local:

- A response created on instance **A** can be continued, retrieved, and deleted
  on instance **B**. Provider affinity, conversation leases, and idempotency
  markers are all Redis-backed, so they hold across the fleet.
- Per-user caps and counters (`RESPONSE_STATE_MAX_RESPONSES_PER_USER`,
  `RESPONSE_STATE_MAX_CONVERSATIONS_PER_USER`) are maintained in Redis, so they
  survive restarts and are enforced consistently across instances.
- Conversation write serialization uses a **renewable per-conversation lease**
  in Redis; a conflicting concurrent write on any instance gets 409
  (`conversation_conflict`) before an upstream call.

The **only** non-shared state is the active Responses **WebSocket
connection-local `store=false` state**: a `store=false` continuation is valid
only on the same live socket that produced the prior response, and that state is
never promoted to Redis (rows SEC06, WS01, WS08). A WebSocket reconnect — even
to the same instance — cannot resume `store=false` continuation; it returns
`previous_response_not_found`. This is by design: `store=false` means no durable
state, and there is no gateway connection-local cache to share.

Operational implication: you can scale one-api horizontally behind a normal load
balancer without sticky sessions for HTTP Responses traffic. The state store,
not the instance, is the source of truth.
