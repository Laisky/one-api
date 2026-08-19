# Change Manual: External UUIDv7 Resource Identifiers

- Status: Proposed (rev. 3 — backward-compatibility ladder + acceptance overhaul)
- Date: 2026-07-03 (rev. 3: 2026-07-04)
- Area: data model / migrations / management API / auth / caching / frontend (modern, air, berry) / observability / docs
- Related: security posture (IDOR / enumeration hardening), [`docs/security/api_security_audit.md`](../security/api_security_audit.md) (which already recommends "Use UUIDs and validate ownership")

> **Revision note.** Every file/line anchor and structural claim below was verified against the working tree — three independent verification rounds; the third re-verified every anchor with five parallel sweeps (§11.2). §11 is a changelog of what changed versus earlier drafts and *why*. Read §3.6 (migration hazards), §3.9 (compatibility ladder), §3.10 (bind-safety), and §5 (phasing) before implementing — they contain the correctness-critical mechanics. Backward compatibility is a first-class rule: **no JSON key ever changes type; fields are only added (dual-emit) and later removed (strict), per resource, each in its own release** (D6).

---

## 1. Background

### 1.1 Motivation

Every important model in one-api (`users`, `channels`, `tokens`, `redemptions`,
`logs`, …) exposes its **integer auto-increment primary key** as the external
identifier — in JSON bodies, in management URLs (`/api/channel/:id`), and in
request bodies (`{ id: 42 }`). Sequential integer identifiers leak information
and enable abuse:

- **Enumeration / IDOR** — sequential IDs make every table trivially walkable
  (`/api/channel/1`, `/2`, `/3`…). Any single authorization gap becomes a full
  table disclosure.
- **Business-intelligence leakage** — [`GetMaxUserId()`](../../model/user.go#L76)
  and "latest row" queries hand any observer the total user / channel / token
  counts, growth rate, and record ordering. (Note the residual leak in §10.5:
  OAuth default usernames are minted as `github_<GetMaxUserId()+1>`.)

The goal is to expose an opaque, non-enumerable **UUIDv7** as the sole external
identifier for every model that has an integer primary key, while keeping the
integer primary key authoritative internally (FK joins, caches, relay hot path,
billing).

### 1.2 Decisions already locked

| # | Decision | Value |
| --- | --- | --- |
| D1 | Identifier scheme | **UUIDv7**, canonical **hyphenated** form (`018f…-…-…`) |
| D2 | Internal primary keys | **Unchanged** — integer `Id` stays the authoritative PK; all FK joins/caches/metrics/relay stay int |
| D3 | Request compatibility | Management endpoints accept **both**: a value containing `-` is a UUID, otherwise a legacy integer id |
| D4 | Response identifiers | **End state:** responses return UUID only — no integer `id` and no integer foreign-key field crosses the API boundary. Reached per resource through a **dual-emit window** (§3.9, §5), never a flag-day flip |
| D5 | Model scope | **Every table with an integer PK** (see §2.1) |
| D6 | Type stability (new in rev. 3) | **No JSON key ever changes type.** UUIDs ship under **new keys** (`uuid`, `user_uuid`, `channel_uuid`, `token_uuid`, `inviter_uuid`, `server_uuid`, `log_uuid`); the legacy int keys are later **dropped**, never re-typed. A removal fails loudly in clients; an in-place `user_id: 42 → "018f…"` flip fails silently (`parseInt(uuid)` → `NaN`) and is forbidden |

> **D3 discriminator** is safe for whole-value inputs (path params, query
> params, JSON body ids): integer ids are digits-only; canonical UUIDv7 always
> contains hyphens; one shared helper (`strings.Contains(s, "-")`) implements the
> branch. **One exception, verified:** the admin "token-key channel suffix" is
> parsed by splitting the key on **`-`** (not `:` — the "`token_key:channel_id`"
> comment in the source is misleading; the code is
> [`strings.Split(key, "-")`](../../middleware/utils.go#L268), then
> [`parts[1]`](../../middleware/auth.go#L300)). A hyphenated channel UUID appended
> there shatters into fragments, so **that one site cannot accept a UUID under the
> current key grammar** — keep it int-only (or change its delimiter). The relay
> `:channelid` path param ([`auth.go:314`](../../middleware/auth.go#L314)) is a URL
> segment and *does* accept a UUID. See §3.2 and Appendix A.

> **D1 caveat** (documented, not blocking): UUIDv7 embeds a 48-bit creation
> timestamp and is time-ordered, so it still leaks *creation time* and rough
> ordering. It closes enumeration/IDOR but not timing inference. Accepted.

### 1.3 Existing infrastructure we reuse

- UUIDv7 generator: [`random.GetUUIDWithHyphens()`](../../common/random/main.go#L23)
  returns `gutils.UUID7()` **with hyphens** (plain [`random.GetUUID()`](../../common/random/main.go#L16)
  strips them — we must use the hyphenated variant so D3 holds). go-utils pin:
  `github.com/Laisky/go-utils/v6 v6.2.3-…` ([`go.mod:10`](../../go.mod#L10)).
- Opaque external secrets are already UUID/random and need no change:
  `User.AccessToken`, `User.AffCode` (a 4-char code resolved server-side via
  [`GetUserIdByAffCode`](../../model/user.go#L148)), API `Token.Key`,
  `Redemption.Key`, the per-request id, and the signed session cookie (stores
  the int id server-side only).
- **Proven post-backfill unique-index migration template:**
  [`MigrateUserRequestCostEnsureUniqueRequestID`](../../model/cost.go#L131) —
  dedup-then-create-index, cross-dialect, `HasIndex`-guarded, batched. This is
  the template we follow for the uuid unique index (§3.6), *not* the AutoMigrate
  struct-tag path.
- Cross-dialect ALTER precedents: [`trace_migration.go`](../../model/trace_migration.go#L21),
  [`ability_migration.go`](../../model/ability_migration.go#L20).
- SQLite busy-retry wrapper: [`sqlite_retry.go`](../../model/sqlite_retry.go#L21).

### 1.4 The core complication: "UUID only" cascades into foreign keys

Because D4's end state forbids **any** integer id in a response, embedded
foreign-key fields must also be emitted as the referenced row's UUID — under new
`*_uuid` keys per D6, with the legacy int keys dropped only at that resource's
strict cutover (S2, §3.9). Verified FK fields that currently serialize as
integers:

| Model | Own `id` JSON | Embedded FK JSON fields (int today) |
| --- | --- | --- |
| User | `id` | `inviter_id` (no frontend UI reads it — [`model/user.go:55`](../../model/user.go#L55)) |
| Token | `id` | `user_id` — **via `MarshalJSON` DTO, not struct tags** ([`model/token.go:69-70`](../../model/token.go#L69)) |
| Redemption | `id` | `user_id` |
| Log | `id` | `user_id`, **`channel`** (JSON name of `ChannelId`), `token_id` |
| MCPTool | `id` | `server_id` (own id not route-addressable — see §2.1) |
| TokenTransaction | `id` | `token_id`, `user_id`, `log_id` (own id not route-addressable) |
| PasskeyCredential | `id` | `user_id` |
| AsyncTaskBinding | `id` | `user_id`, `token_id`, `channel_id` — **no HTTP surface at all** (§2.2) |

> **Correction vs first draft:** `dto.EnabledAbility.ChannelId`
> ([`dto/ability.go:7`](../../dto/ability.go#L7)) is **not** a response leak. All
> consumers ([`controller/model.go:1487,1553,1664,1720,1783`](../../controller/model.go))
> read it internally to build display maps; no handler serializes an
> `EnabledAbility`. It is removed from scope.

This FK translation is the largest, highest-risk part of the work; §3.3 decides
the strategy and §4 sizes it.

---

## 2. Scope

### 2.1 In-scope models

All 12 integer-PK models are `AutoMigrate`d in [`migrateDB()`](../../model/main.go#L276)
(the migrate list actually holds **14** structs — the 12 below plus `Option`
(string PK) and `Ability` (composite PK), both correctly out of scope per §2.2):

`User`, `Token`, `Channel`, `Redemption`, `Log`, `TokenTransaction`,
`UserRequestCost`, `Trace`, `AsyncTaskBinding`, `MCPServer`, `MCPTool`,
`PasskeyCredential`.

**They are not equal.** The security value (enumeration/IDOR hardening) lives
entirely in the models whose *own integer PK is accepted as a route parameter*.
Verified route surface splits them into three tiers, which drives how much work
each needs:

| Tier | Models | Route-addressable by own int PK? | Work implied |
| --- | --- | --- | --- |
| **T-A: addressable** | User, Channel, Token, Redemption, **Log** (via [`/api/trace/log/:log_id`](../../router/api.go#L175) + `?id=` drilldown), MCPServer, PasskeyCredential | **Yes** | Full: uuid column + `GetXByUUID` + request-side resolver (D3) + response cutover (D4) |
| **T-B: serialized-only** | MCPTool (listed by `server_id`), TokenTransaction (scoped to auth token), UserRequestCost (addressed by `request_id` string), Trace (addressed by `trace_id` string) | No — own int id never taken as a param | D4 only: uuid column optional; emit uuid **or omit** the int fields (§3.3) |
| **T-C: internal-only** | AsyncTaskBinding | No HTTP surface whatsoever ([confirmed](../../model/async_task.go): only `SaveAsyncTaskBinding`/`GetAsyncTaskBindingByTaskID`) | Column is **defensive-only**; zero external/frontend work |

D5 keeps all 12 uniform (a uuid column everywhere), but recognizing the tiers
avoids wasted effort: T-B/T-C need **no request-side resolver and no frontend
change**, and their D4 compliance can be met by *omitting* redundant int fields
the client already knows (its own token/user) rather than denormalizing a uuid.

### 2.2 Explicitly out of scope

| Item | Why |
| --- | --- |
| `Option` ([`model/option.go:17`](../../model/option.go#L17)) | PK is a **string** `Key`. No option value stores a channel/user id (verified: only flags/secrets/quotas/ratios/links). |
| `Ability` ([`model/ability.go:21`](../../model/ability.go#L21)) | **Composite** PK `(Group, Model, ChannelId)`, no surrogate int PK. Relay hot path resolves group+model→`*Channel` by int and never needs a uuid. Stays int. |
| Internal FK joins, caches, in-memory routing maps | D2 — stay int behind the resolver (§3.5). |
| Prometheus / OTEL `channel_id` labels ([`monitor/prometheus/recorder.go`](../../monitor/prometheus/recorder.go), [`monitor/otel/recorder.go`](../../monitor/otel/recorder.go)) | Internal ops telemetry, admin-bounded cardinality, never serialized to a client. Stays int. |
| zap log fields | Internal diagnostics. Stays int. |
| Session cookie / WebAuthn handle | Cookie stores int server-side ([`SetupLogin`](../../controller/user.go#L159)). [`WebAuthnUser.WebAuthnID()`](../../model/passkey_webauthn.go#L20) binds `uint64(User.Id)`; int PK unchanged ⇒ passkey login unaffected. |
| Relay passthrough routes (`router/relay.go`) | OpenAI-compatible surface, not management resources. (Admin `:channelid` proxy param is handled in §3.2.) |
| Root-user email notifications embedding `#%d` ([`monitor/channel.go:41,57,74`](../../monitor/channel.go#L41)) | Recipient is the operator (admin), not an end user. Internal. |

---

## 3. Design & key decisions

### 3.1 The `uuid` column (index strategy is deliberately *not* the struct tag)

Add to every in-scope struct:

```go
UUID string `json:"uuid" gorm:"type:char(36);index;column:uuid"` // NULLABLE — no not-null, no default
```

- **The column MUST be nullable — do not add `not null` or `default:''`.** This
  is the actual hard rule (see §3.6.1). GORM's `ADD COLUMN` on an existing table
  produces a nullable, no-default column, so every pre-existing row backfills to
  `NULL`, not `''`. A `not null`/`default:''` tag would instead give every row an
  identical `''` and *then* a unique index would collide — so simply keeping it
  nullable is what avoids that.
- **`index`, NOT `uniqueIndex`, in the tag (conservative default, not a
  crash-fix).** A unique index over many `NULL`s is actually legal on MySQL/PG/
  SQLite (NULLs are distinct), so a nullable `uniqueIndex` tag would *not* crash
  at migrate time. We still add the UNIQUE constraint in a dedicated post-backfill
  migration (cost.go pattern) for three concrete reasons: (a) the codebase already
  distrusts AutoMigrate for cross-dialect unique-index creation — that is exactly
  why [`MigrateUserRequestCostEnsureUniqueRequestID`](../../model/cost.go#L131)
  exists; (b) SQLite's flaky DDL introspection ([`main.go:217-222`](../../model/main.go#L217));
  (c) it decouples the constraint from the backfill so a generation bug surfaces
  as a controlled migration error, not a startup `Fatal`. A plain index already
  serves `GetXByUUID` lookups.
- `char(36)` (hyphenated) on all three dialects.
- **The tag makes the field bindable, too.** Go JSON tags are symmetric
  (marshal *and* unmarshal), so every struct a handler `ShouldBindJSON`s into now
  accepts client-supplied `uuid`/`*_uuid` values. §3.10 defines the mandatory
  bind-safety rule (zero them after bind; exclude them from `Updates`).

### 3.2 Request-side resolver (D3) — complete verified inventory

One shared helper package `common/idresolve`:

```go
// Resolve returns the internal integer PK for s, which is either a legacy
// integer id (digits) or a UUID (contains '-'), looking up the uuid when needed.
// Empty/garbage → ErrInvalidRef (400); unknown uuid → ErrNotFound (404).
func Resolve(lookup func(uuid string) (int, error), s string) (int, error)
```

Per-model typed wrappers (`ResolveUserRef`, `ResolveChannelRef`, …) call the
matching `GetXByUUID`. **Every** site below (verified by direct read of the
router + controllers) switches from `strconv.Atoi(...)` to a wrapper. See
Appendix A for the full table; the counts:

- **Path `:id` params (27 sites):** token `121/220/1108`, tracing `71` (`log_id`),
  passkey `399/420`, channel_debug `16/51/100`, channel_billing `382`,
  channel_testing `315`, user `337/1172/1735`, mcp_server `94/140/230/249/285/323`,
  redemption `89/160`, channel `275/295/389/479/523` (= 3+1+2+3+1+1+3+6+2+5 = **27**).
- **Admin channel selectors (2 sites, only 1 UUID-capable):** the relay
  `:channelid` path param ([`auth.go:314`](../../middleware/auth.go#L314)) is a URL
  segment → resolver applies, accepts uuid. The **token-key suffix**
  ([`auth.go:300`](../../middleware/auth.go#L300)) parses via `strings.Split(key,
  "-")` and therefore **stays int-only** (a hyphenated uuid would shatter — §1.2);
  either leave it int or change its delimiter first. This is the one site D3 does
  *not* cover.
- **Query params (4 sites):** [`controller/log.go:28,259,280`](../../controller/log.go#L28)
  `strconv.Atoi(c.Query("channel"))` — **server-side int coercion** (the frontend
  passes the raw string; the *backend* is what breaks), [`mcp_tool.go:35`](../../controller/mcp_tool.go#L35)
  (`server_id`), [`user.go:403`](../../controller/user.go#L403) (`user_id`, already
  a string), [`token.go:1051`](../../controller/token.go#L1051) (`user_id`).
- **JSON body ids (5 sites):** `UpdateUser` (`/api/user/` PUT, `payload.Id`),
  `UpdateChannel` (`channel.Id`), `UpdateToken` (`token.Id`), `UpdateRedemption`
  (`redemption.Id`), and — **missed by the first draft** —
  [`AdminTopUp`](../../controller/user.go#L1455) (`/api/topup` POST, `req.UserId`).

**Body-site mechanics (D3 without a type flip).** The bound struct keeps its
legacy int `id` field and gains the (already-added) `uuid` field; the handler
resolves `payload.UUID` when non-empty, else falls back to `payload.Id`. Legacy
writers keep sending int `id` completely unchanged through S2 (§3.9). After
resolution the handler zeroes every uuid field before the struct reaches GORM
(bind-safety, §3.10).

**Downstream note:** the resolver converts at the boundary only. Internal
consumers of the already-parsed int channel id (`ctxkey.SpecificChannelId`
readers in [`distributor.go:212`](../../middleware/distributor.go#L212),
[`relay_retry.go:18`](../../controller/relay_retry.go#L18),
[`video_task_binding.go:72`](../../middleware/video_task_binding.go#L72)) are
unaffected per D2.

### 3.3 Foreign-key rendering strategy — **RECOMMENDED: denormalized uuid columns (Option A), with omit-for-T-B**

**Option A — denormalize (recommended for T-A).** Store the FK's uuid as its own
column, populated at write time, emitted under a **new `*_uuid` JSON key** (D6).
The legacy int key stays during the dual-emit window and flips to `json:"-"` only
at that resource's strict cutover — a removal, never an in-place type change:

```go
// S1 (dual-emit): both keys present
UserId   int    `json:"user_id"   gorm:"index;column:user_id"`            // legacy key, dropped at S2
UserUUID string `json:"user_uuid" gorm:"type:char(36);column:user_uuid"`  // permanent external key
// S2 (strict): UserId's tag flips to json:"-"; user_uuid is the only identifier
```

Three reasons it fits one-api better than the alternative:

1. **It survives the split log database.** `Log` lives on a *separate* `LOG_DB`
   when `LOG_SQL_DSN` is set ([`InitLogDB`](../../model/main.go#L339); only `Log`
   is migrated there, [`migrateLOGDB`](../../model/main.go#L375)). A read-time
   translator would have to reach across databases (log rows on `LOG_DB`, their
   users/channels on `DB`) — a JOIN is impossible and a per-page cross-DB
   `IN (…)` fan-out is required. Denormalizing the uuid **onto the log row at
   write time** eliminates every cross-DB read.
2. **Controllers serialize raw GORM structs** (`data: logs`) almost everywhere,
   so a tag flip covers them with no per-endpoint rewrite and far less chance of
   a missed leak.
3. **Every FK here is set-once/immutable** (a token never changes owner, a log's
   user/channel is historical, transactions are immutable) → zero drift, no sync
   triggers. (Nuance, sharpened in §3.10: `User.Update`/`Channel.Update`/
   `MCPServer.Update` use `Updates(struct)` with no `Select` allowlist
   ([`user.go:231`](../../model/user.go#L231), [`channel.go:1929`](../../model/channel.go#L1929),
   [`mcp_server_store.go:101`](../../model/mcp_server_store.go#L101)), so a
   populated string field *is* written. Server-side that is harmless — GORM skips
   zero values and the uuids are immutable — but these structs are **bound from
   client JSON**, so a client-supplied non-zero uuid *would* clobber. The
   bind-safety rule (§3.10) zeroes uuid fields after bind *and* adds `Omit`
   of the uuid columns to those three `Update` methods. Verified:
   token/redemption/log/txn updates use `Select`/map allowlists that already
   exclude uuid columns.)

**The real cost of Option A is write-site population.** The FK uuid cannot be
filled by a generic `BeforeCreate` hook (the hook doesn't know the referenced
row's uuid). Every `Log`/`TokenTransaction` creation site must set it — **23
distinct log rows + 2 TokenTransaction sites** (enumerated in Appendix C).

> **Correction vs rev. 2 (verified).** The detached billing snapshot does **not**
> hold `*User`/`*Channel`: [`billingIdentity`](../../relay/controller/billing_ctx.go#L20)
> captures only `requestID`/`provisionalLogID`/`traceID`/`toolSummary`, and every
> relay/billing log site holds only the **int** ids carried by
> [`meta.Meta`](../../relay/meta/relay_meta.go#L41) (`ChannelId`/`TokenId`/`UserId`).
> Naive per-write lookups would add DB reads to the hot billing path (~15 of the
> 23 log rows).

**Hot-path population without extra queries — enrich `meta.Meta`.** Add
`UserUUID`/`TokenUUID`/`ChannelUUID string` to `relay/meta.Meta`, populated where
the rows are *already fetched*: auth middleware loads the token + user, the
distributor loads the channel. Relay write sites then copy strings out of `meta`
— zero added queries. The off-hot-path helpers that take bare ints
(`RecordLog`/`RecordTopupLog`/`RecordManageLog` — topup/gift/manage logs) either
gain a cheap internal lookup (they already call `GetUsernameById` per row) or
leave the FK uuid NULL for the idempotent sweep (§3.6.3) to fill. Rolling-upgrade
caveat: cached user/token objects serialized by an **old** binary lack the uuid
field, so a freshly upgraded node can briefly write NULL FK uuids — also swept.

**For Tier T-B, prefer omit over denormalize.** `GetTokenTransactions` is scoped
to the caller's own token; its `id`/`token_id`/`user_id` are redundant to the
caller. The minimal D4 fix is to drop them (`json:"-"`, at that resource's S2 —
removals follow the ladder like everything else) and denormalize only
`log_uuid` (to preserve the trace drilldown). `UserRequestCost` is addressed by
`request_id`; drop `id`/`user_id`. `Trace`'s own `id` is never needed externally;
drop it. This avoids building denormalized columns nobody reads.

**Option B — translate at read (rejected as the primary strategy).** Keep FKs
int; per serialization site, batch `SELECT id, uuid … WHERE id IN (…)` and remap.
Single source of truth, no schema growth — but it rewrites every list/detail
handler (logs return thousands of rows → a mandatory batch resolver), forces a
cross-DB fan-out for logs, and is easy to under-cover. Higher churn, higher leak
risk. The change list in §4 is sized for **Option A**.

> This remains the one design fork worth an explicit sign-off before starting.

### 3.4 uuid generation: `BeforeCreate` hook for own uuid

No GORM hooks exist in the model package today, and **every** create goes through
`DB.Create` / `LOG_DB.Create` (no raw-SQL `INSERT`, no `CreateInBatches` with
conflict clauses — verified). So a shared `BeforeCreate` that assigns
`GetUUIDWithHyphens()` when `UUID == ""` covers 100% of creation paths, including
`BatchInsertChannels` (per-row hook) and the root seed. Implement it once via an
embedded mixin (e.g. `type UUIDModel struct { UUID string … }` with a
`BeforeCreate`) embedded in each in-scope struct, or a per-model hook.

- **Own uuid → `BeforeCreate` hook** (all paths, including old code you forget).
- **FK uuid → explicit population at the write sites** (Appendix C), because the
  referenced uuid is external to the row being created.

### 3.5 uuid → int resolver + cache

Auth resolves the internal int id from the **session cookie** or the **opaque
token key** — never from a user-facing uuid — so the existing `user_obj:%d` /
`user_group:%d` / `token:%s` / channel caches ([`model/cache.go`](../../model/cache.go))
stay int and unchanged. Only the new **request-side** uuid inputs need
translation:

- DB: `GetUserByUUID`, `GetChannelByUUID`, `GetTokenByUUID`, `GetRedemptionByUUID`,
  `GetMCPServerByUUID`, `GetLogByUUID`, `GetPasskeyCredentialByUUID` — each backed
  by the `uuid` index. (T-B/T-C need no `GetXByUUID`.)
- Optional Redis cache `<model>_uuid:<uuid> → id`, mirroring the `token:%s`
  recipe (string key, `SyncFrequency` TTL). Invalidation mirrors
  [`clearTokenCache`](../../model/token.go#L105). **Corrected claim (verified):**
  the user-scoped caches (`user_obj:%d`, `user_group:%d`, `user_quota:%d`, …) are
  TTL-only today — *nothing* `RedisDel`s them; only token and group-model caches
  have active invalidation. The uuid→id cache adds its own `RedisDel` on the
  delete paths (uuid→id never changes, so staleness only means a deleted row
  resolves for up to `SyncFrequency`; delete-path invalidation is cheap and keeps
  404s immediate).

### 3.6 Migration mechanics — **the correctness-critical section**

`InitDB()` runs migrations only on the master node
([`main.go:207`](../../model/main.go#L207)) in three ordered steps:
STEP 1 `migrateDB()` AutoMigrate ([`:223`](../../model/main.go#L223)),
STEP 2 column/data normalizers ([`:234-256`](../../model/main.go#L234)),
STEP 3 data-format migrations ([`:258-271`](../../model/main.go#L258)).

Five hazards, each with its required handling:

1. **Keep the uuid column nullable; add the UNIQUE constraint after backfill.**
   AutoMigrate (STEP 1) adds the `uuid` column *nullable* (§3.1), so existing
   rows are `NULL` and even a `uniqueIndex` tag would not crash (multi-NULL is
   legal in a unique index on all three dialects). The genuine failure mode is
   only if the column is declared `not null`/`default:''` — then every row shares
   `''` and a unique index collides. **Handling:** tag is nullable plain `index`
   (§3.1); a new `model/uuid_migration.go` creates the UNIQUE index in a
   dialect-aware, `HasIndex`-guarded step, exactly like
   [`cost.go:317-334`](../../model/cost.go#L317), for the conservatism reasons in
   §3.1. The unique promotion is gated on "no NULL **own** uuids remaining" (see
   hazards 3 & 5).

2. **The `Log` uuid + FK-uuid backfill cannot use `UPDATE … JOIN`.** When
   `LOG_SQL_DSN` is set, `logs` is on `LOG_DB` while `users`/`channels` are on
   `DB` — no cross-DB JOIN exists. **Handling:** the backfill for `Log` runs
   against `LOG_DB`, generates each row's own uuid in Go, and fills
   `user_uuid`/`channel_uuid` from an in-memory `id → uuid` map built by reading
   `DB.users`/`DB.channels` (batched). When `LOG_SQL_DSN` is empty
   (`LOG_DB == DB`, [`main.go:341`](../../model/main.go#L341)) the same code path
   works against the single handle.

3. **Rolling upgrades / slaves write NULL-uuid rows.** Migrations are
   master-only, but *every* node (slaves included) writes `Log`/`TokenTransaction`/
   quota rows directly to the shared DB, and an old binary during a rolling
   deploy has no `BeforeCreate` hook — so it inserts rows with NULL
   uuid/FK-uuid *after* the master backfilled. **Handling:** (a) the backfill is
   idempotent, batched (`deleteBatchSize`-style, 1000/loop), `IsMasterNode`-gated,
   and **re-runnable every master start** so it sweeps rows created by old
   binaries; (b) the write path and reads must tolerate NULL uuid during the
   window; (c) the UNIQUE-index promotion (hazard 1) is a **later release**,
   after all nodes run the new binary and a backfill pass reports no NULL own
   uuids. Until then a plain index serves lookups and correctness is preserved.

4. **Hard-deleted channels/tokens orphan FK backfill (verified).**
   `channel.Delete()` ([`channel.go:2016`](../../model/channel.go#L2016)) and token
   delete are **hard** deletes — no `DeletedAt` on `Channel`/`Token` — so a
   historical `Log`/`TokenTransaction` can reference a channel/token whose row no
   longer exists, and the §3.6.2 `id → uuid` map (built from live rows) cannot
   fill its `channel_uuid`/`token_uuid`. (`user.Delete()`
   [`user.go:249`](../../model/user.go#L249) is a *soft* delete — sets
   `Status=Deleted`, keeps the row — so `user_uuid` is always fillable.)
   **Handling:** orphaned FK uuids stay `NULL` and are emitted as JSON `null`
   (never a bare `""`); the D4 gate and T12 must tolerate a `null` FK uuid on
   pre-existing orphaned rows; the empty-sweep predicate (hazard 5) keys on the
   **own** uuid only, so orphans do not keep it re-scanning forever.

5. **SQLite AutoMigrate-once + a terminating sweep.** Keep AutoMigrate to a single
   invocation per process — note (verified) the "guard" at
   [`main.go:217-222`](../../model/main.go#L217) is a **comment/convention, not a
   runtime flag**; the only safety net is `shouldIgnoreDuplicateColumn`
   ([`main.go:331`](../../model/main.go#L331)), so do not add a second AutoMigrate
   call. Route backfill writes through
   [`sqlite_retry.go`](../../model/sqlite_retry.go); guard
   every step with `HasColumn`/`HasIndex`. The re-runnable backfill's "is there
   work left" predicate must test **own** uuid IS NULL (fillable), never FK uuid
   (may be permanently NULL for orphans, hazard 4), so it converges. SQLite
   type-change is a no-op — rely on AutoMigrate for the new column.

6. **Deploy order: master first (verified failure mode).** Migrations are
   master-only, and GORM is only *partially* tolerant of a missing column: plain
   `First`/`Find` render `SELECT *` and survive, but **every write** (INSERT and
   UPDATE enumerate all struct columns) and **every `Omit(...)` read**
   ([`user.go:83,121,123,140`](../../model/user.go#L83) `Omit("password")`,
   [`channel.go:489,534,547`](../../model/channel.go#L489) `Omit("key")` — these
   enumerate every schema column *except* the omitted one) reference the new
   `uuid` columns explicitly and **hard-fail** against a schema the master has
   not migrated yet. A new-binary slave started before the new master therefore
   breaks on log writes and on user/channel list reads. **Handling:** the rollout
   runbook (§5) requires upgrading the master node first (it AutoMigrates at
   startup), then the slaves — the same latent constraint as any column-adding
   release here, now stated explicitly and exercised by T18.

### 3.7 Root-account seeding

[`CreateRootAccountIfNeed()`](../../model/main.go#L31) creates the root `User`
([`main.go:53`](../../model/main.go#L53)) and — only when
`config.InitialRootToken != ""` — `Token{Id: 1}`
([`main.go:68`](../../model/main.go#L68)). Both seed rows use **raw `DB.Create`**,
bypassing `User.Insert`/`Token.Insert` — so FK-uuid population must not live only
inside the `Insert()` wrappers. The `BeforeCreate` hook (§3.4) is GORM-level and
still fires, assigning both own uuids automatically; the token's `user_uuid` must
be set explicitly at the seed site (the user is created first, so its uuid is
available).

### 3.8 Custom serialization sites (do **not** inherit struct-tag changes) — verified inventory

The tag flip fixes raw `c.JSON(data: <gormStruct>)` handlers. The following
hand-built maps / DTOs / custom marshallers do **not** inherit it and must be
edited by hand — **twice, per the ladder (§3.9): S1 adds the uuid keys alongside
the ints; that resource's S2 removes the int keys.** This list is the acceptance
gate's target (full detail in Appendix B):

| Site | File:line | Fields to convert |
| --- | --- | --- |
| `Token.MarshalJSON` → `tokenDTO` | [`model/token.go:69-70`](../../model/token.go#L69) | `id`, `user_id` — **highest impact**: overrides tags for ~8 token endpoints + `ConsumeToken`'s `data: updatedToken` |
| `GetSelfByToken` | [`controller/user.go:681,708,731,733`](../../controller/user.go#L681) | `id`(user), `id`(token), **`uid`**, **`token_id`** (last two missed by first draft) |
| `GetTraceByTraceId` | [`controller/tracing.go:53`](../../controller/tracing.go#L53) | `id`(trace) |
| `GetTraceByLogId` | [`controller/tracing.go:132,143,144`](../../controller/tracing.go#L132) | `id`(trace), `id`(log), `user_id`(log) |
| `buildTransactionResponse` (via `ConsumeToken`) | [`controller/token.go:746,748`](../../controller/token.go#L746) | `id`(txn), `token_id` |
| `PasskeyRegisterFinish` | [`controller/passkey.go:251`](../../controller/passkey.go#L251) | `id`(cred) |
| `PasskeyList` → `passkeyInfo` DTO | [`controller/passkey.go:383`](../../controller/passkey.go#L383) | `id`(cred) |
| `DuplicateChannel` | [`controller/channel.go:317`](../../controller/channel.go#L317) | `id`(new channel) |
| `GetChannelMigrationStatus` | [`controller/channel_debug.go:115`](../../controller/channel_debug.go#L115) | `channel_id` |
| `GetToolsDisplay` → `MCPServerDisplayInfo` | [`controller/mcp_server.go:405`](../../controller/mcp_server.go#L405) | `id`(server) |
| `GetDashboardUsers` → `UserOption` | [`controller/user.go:591,599`](../../controller/user.go#L591) | `id`(user); also the literal `Id:0` "All Users" sentinel → needs a non-int sentinel (`""`/`"all"`) |
| `GetUserDashboard` stat DTOs | [`dto/log_statistics.go:20,34,57,66`](../../dto/log_statistics.go#L20) | `UserId` — **no json tag**, serializes as capitalized `"UserId"` ×4 DTOs. **Not a passive field flip:** these come from `GROUP BY user_id` queries ([`model/log.go:1180,1195,1227,1243,1333,1352,1390,1410`](../../model/log.go#L1180)); emitting a uuid means adding `logs.user_uuid` to both `SELECT` and `GROUP BY` (sequenced *after* the Log `user_uuid` denormalization lands), and it must be grouped, not just selected, to satisfy MySQL `ONLY_FULL_GROUP_BY`. |

**Shared login response is tag-flippable** but high-fan-out: `SetupLogin` builds
`cleanUser := model.User{…}` ([`controller/user.go:192-198`](../../controller/user.go#L192), `c.JSON` at `:199`)
returned by password `Login`, **all** OAuth logins (github/oidc/lark/wechat →
`SetupLogin`), and `PasskeyLoginFinish` — one `model.User.Id` tag flip covers them
all. No response header, SSE, WebSocket, or relay payload leaks a resource int id
(verified).

### 3.9 Compatibility ladder — how D3/D4/D6 compose without a flag-day break

Per resource, the external contract moves through four stages; **each stage
transition is its own release**, and a JSON key is never re-typed (D6) — fields
are only *added* (S1) and later *removed* (S2/S3):

| Stage | Requests (D3) | Responses (D4/D6) | Breaking? |
| --- | --- | --- | --- |
| **S0** (today) | int only | int only | — |
| **S1 dual** | accept int **and** uuid | emit int **and** uuid keys (`id`+`uuid`, `user_id`+`user_uuid`, …) | **No** — purely additive; legacy readers and writers untouched |
| **S2 strict-out** | accept int and uuid | **uuid keys only** (int keys dropped) | Readers of int keys break **loudly** (missing key), per resource, release-noted |
| **S3 strict-in** (end state) | **uuid only** (int ids rejected) | uuid only | Writers still sending int ids break; completes enumeration/IDOR hardening |

- Enumeration hardening is only *complete* at S3: while S2 still accepts integer
  ids (the D3 window), an attacker can still probe by int. S1/S2 already remove
  the BI/count leakage from responses, and frontends/scripts migrate during S1.
- The three bundled frontends switch to reading/sending uuid during S1 (they ship
  with the same binary, so by a resource's S2 they no longer touch its int keys).
- External consumers experience: additive keys at S1 → a release-noted removal at
  S2 → a major-version removal at S3 (open Q3 sets the concrete windows).
- Mapping to struct tags: S1 = int tag kept + uuid field tagged; S2 = int tag →
  `json:"-"`. Both are compile-time, per-resource flips (§5). There is **no
  runtime config flag** — that would require runtime-conditional serialization in
  every handler (rejected for churn; revisit only if a deployment provably cannot
  tolerate any S2 date).

### 3.10 Bind-safety (mass-assignment guard) — mandatory with the new fields

Go JSON tags are symmetric, so every `json:"uuid"` / `json:"*_uuid"` field is
also **bindable** wherever a handler `ShouldBindJSON`s into the model struct
(`AddChannel`, `UpdateChannel`, `AddToken`, `UpdateToken`, …). Without a guard:

- **Create paths** would accept a *client-chosen* uuid — `BeforeCreate` fills
  only when empty — making uuids spoofable and unique-collisions forceable.
- **Update paths** on the three no-allowlist models (`User`/`Channel`/`MCPServer`
  use `Updates(struct)` — §3.3) would **write** a client-supplied non-zero uuid,
  clobbering own or FK uuids.

Rule (enforced by T24): every handler that binds client JSON into an in-scope
struct **zeroes all uuid fields after bind** — after first using `payload.UUID`
for D3 resolution where applicable (§3.2) — and the three no-allowlist `Update`
methods additionally `Omit` the uuid columns as defense-in-depth. The
allowlisted update paths (token/redemption/log/txn) already exclude uuid
columns; keep them that way.

---

## 4. Change list

Grouped by layer. File references are anchors, not exhaustive line lists.

### 4.1 Model & schema (`model/`)

| Change | Location |
| --- | --- |
| Add `UUID string gorm:"type:char(36);index"` (plain index) + `BeforeCreate` mixin | 12 in-scope structs; root seed [`main.go:31`](../../model/main.go#L31) |
| Add denormalized FK uuid columns (Option A), emitted under new `*_uuid` keys (D6) | `token.go` (`user_uuid`), `log.go` (`user_uuid`, `channel_uuid`, `token_uuid`), `token_transaction.go` (`log_uuid`; drop id/token_id/user_id at S2 per §3.3), `user.go` (`inviter_uuid`), `mcp_tool.go` (`server_uuid`), `cost.go`/`trace.go` (omit int fields at S2, no FK uuid needed) |
| S1: add uuid JSON keys alongside the ints; per-resource S2: flip that resource's int tags to `json:"-"` | same files + the §3.8 custom sites (edited twice, once per stage) |
| Bind-safety (§3.10): zero uuid fields after every `ShouldBindJSON` into in-scope structs; `Omit` uuid columns in the three no-allowlist `Update` methods | create/update handlers; [`user.go:231`](../../model/user.go#L231), [`channel.go:1929`](../../model/channel.go#L1929), [`mcp_server_store.go:101`](../../model/mcp_server_store.go#L101) |
| `GetXByUUID` lookups (T-A only) | `user.go`, `channel.go`, `token.go`, `redemption.go`, `log.go`, `mcp_server_store.go`, `passkey.go` |
| New `model/uuid_migration.go`: backfill own+FK uuids (Go-side, batched, idempotent, `IsMasterNode`, **`LOG_DB`-aware for `Log`**, re-runnable); **later** phase promotes plain index → UNIQUE (cost.go pattern) | wired into STEP 3 of [`InitDB()`](../../model/main.go#L258) |

### 4.2 Backend infra

| Change | Location |
| --- | --- |
| `common/idresolve` (`-` discriminator + typed wrappers, 400/404 semantics) | new package |
| Optional `<model>_uuid:<uuid> → id` Redis cache + invalidation | `model/cache.go` |
| Enrich `relay/meta.Meta` with `UserUUID`/`TokenUUID`/`ChannelUUID`, populated in auth/distributor where the rows are already fetched (§3.3) | `relay/meta/relay_meta.go`, `middleware/auth.go`, `middleware/distributor.go` |
| Populate FK uuid at the 23 log rows + 2 txn sites (Appendix C): relay sites copy from `meta`; int-only `Record*` helpers use an internal lookup or leave NULL for the sweep | billing/relay controllers + `model/log.go` record helpers |

### 4.3 Controllers & routers

Request-side: swap the 27 path params + the relay `:channelid` selector + 4 query
params + 5 body ids (§3.2 / Appendix A) to the resolver (the token-key suffix at
`auth.go:300` stays int-only — §1.2). Response-side, per the ladder (§3.9): S1
adds uuid keys (struct tags for `data: <model>` handlers plus the 12 custom sites
in §3.8); each resource's S2 removes its int keys (tag flip to `json:"-"` plus a
second pass over its custom sites). Note the four `log_statistics` DTOs are a
**query rewrite** (GROUP BY `user_uuid`), not a tag flip, and depend on the Log
`user_uuid` denormalization landing first (§3.8).

> **Correction:** `/api/channel/batch` (referenced by the air frontend) has **no
> backend route** — no `POST("/batch")` exists anywhere in `router/`/`controller/`.
> There is no batch endpoint to update. See §4.4 and §11.

### 4.4 Frontends (all three shipped: modern, air, berry)

Distinguish **resource-row ids** (migrate to string) from **channel-TYPE enums**
(`CHANNEL_OPTIONS`/`CHANNEL_TYPES` — numeric, **unchanged**). Guard against a
blanket `id: number → string` sweep: several `id: number` fields are *not*
migrated-model ids and must stay numeric.

**modern** ([`web/modern`](../../web/modern), TS/Vite — most surface):

- Flip `id: number → string` in per-page interfaces, the `LogEntry` type in
  [`src/types/log.ts`](../../web/modern/src/types/log.ts) (`id`, `user_id`,
  `channel`), the **persisted** auth store
  ([`lib/stores/auth.ts:6`](../../web/modern/src/lib/stores/auth.ts#L6) — cached to
  `localStorage`; see the persisted-state note below), MCP `server_id`, the local
  `MCPTool` interface ([`EditMCPServerPage.tsx:22`](../../web/modern/src/pages/mcp/EditMCPServerPage.tsx#L22)
  — **missed by rev. 2**), `useClipboardManager` `Record<number,…>` **key type**,
  and the balance-refresh `Set<number>` of channel ids
  ([`ChannelsPage.tsx:119`](../../web/modern/src/pages/channels/ChannelsPage.tsx#L119)
  — **missed by rev. 2**). Admin gating uses `role`, never `id` — safe.
- Remove four update-body `parseInt` casts: channel
  [`pages/channels/hooks/useChannelForm.ts:621`](../../web/modern/src/pages/channels/hooks/useChannelForm.ts#L621)
  (path corrected — the hook lives under `pages/channels/hooks/`, not
  `src/hooks/`), token `EditTokenPage.tsx:253`, user `EditUserPage.tsx:213`,
  redemption `EditRedemptionPage.tsx:156`.
- Three comparison/parse breakages the other apps lack: `LogsPage.tsx:141`
  `parseInt(idStr)` tracing (`parseInt(uuid)`→NaN never matches — compare raw
  string); `EditUserPage.tsx:92` `Number(userId)` self-vs-target compare;
  dashboard [`pages/dashboard/hooks/useDashboardData.ts:153/168/190/200`](../../web/modern/src/pages/dashboard/hooks/useDashboardData.ts#L153)
  `Number(row.UserId)` on the stat rows (the **filter** input is already
  string-safe — the break is row parsing, contra the first draft's "dashboard
  user_id filter").
- Keep numeric: `getChannelTypeColor` (channel *type*), `schemas.ts` `EndpointInfo.id`
  (endpoint catalog, not a migrated model), chat message keys (numeric
  `messageIndex` array indices — the `Message` type has no id field), local UI
  row ids.
- **Persisted-state migration:** the auth store caches the whole user object
  (`id: number`) in `localStorage` (zustand `persist` plus explicit
  `localStorage.setItem('user', …)`), so after an S2 cutover a stale persisted
  entry still holds an int id and no `uuid`. Type the field `string | number`
  during the window **and** bump the persist `version` with a `migrate` that
  refreshes stale entries; audit air and berry the same way (both keep `user`
  in localStorage).

**air** ([`web/air`](../../web/air), JS/CRA/Semi):

- Four update-body `parseInt`s: `EditChannel.js:548`, `EditToken.js:150`,
  `EditUser.js:96`, `EditRedemption.js:63`.
- New-record sentinels are `undefined` (`if (userId)` / `!== undefined`), not
  numeric-0 → survive the migration.
- **No batch work:** `ChannelsTable.js:550-552` already pushes raw `channel.id` (no
  `parseInt`) to a `/api/channel/batch` endpoint that **does not exist** on the
  backend. Nothing to change here (and the endpoint's absence is a pre-existing
  bug, out of scope).
- Log `channel` filter is a plain text input → passes the uuid through; the
  backend `strconv.Atoi` is what needs the resolver (§3.2).

**berry** ([`web/berry`](../../web/berry), JS/CRA/MUI):

- Four update-body `parseInt`s: `Channel/component/EditModal.js:369`,
  `Token/component/EditModal.js:72`, `User/component/EditModal.js:77`,
  `Redemption/component/EditModal.js:53`.
- **Numeric-0 sentinels → `''`/`null`:** `useState(0)` + `handleOpenModal(0)` in
  `views/{Channel,User,Redemption,Token}/index.js`, and the explicit
  `channelId === 0` "new record" check in `Channel/component/EditModal.js:843`.
  The `if (xId)` truthiness guards survive only if the new sentinel is falsy — a
  stray `0` silently routes edits into the create branch.
- Row keys `key={row.id}` are fine as strings.

Leave untouched everywhere: `row.id` from react-table/MUI/Semi, route `:id`
params + `navigate(...${id})` + URL templates (auto-coerce), `sort=id`/`p`/`size`
(column/pagination names), channel-TYPE enums, and `/api/user/manage` (keyed by
**username**). **Correction:** `/api/user/totp/disable/:id` is **id-keyed**, not
username-keyed — its call sites (`modern UsersPage.tsx:391`/`EditUserPage.tsx:315`,
`air EditUser.js:72`, `berry User/EditModal.js:137`) are in scope.

### 4.5 Docs & observability (D4-strict, optional)

- Update [`docs/manuals/api_references.md`](../../docs/manuals/api_references.md#L247)
  (line 247 literally states "Resource ids are integers") and the served
  `web/modern/public/openapi.json` ([`router/web.go:44`](../../router/web.go#L44)).
- Client-facing relay error strings embedding raw channel id —
  `"Channel #%d does not support…"` ([`middleware/distributor.go:227,233,241`](../../middleware/distributor.go#L227)),
  `"channel #%d does not list support…"` ([`model/ability.go:70,379`](../../model/ability.go#L70)),
  `"Invalid Channel Id: %s"` ([`middleware/auth.go:302,317`](../../middleware/auth.go#L302)).
  Diagnostic text, not identifiers; sanitize (or emit the channel uuid/name) only
  if "no int id ever crosses the boundary" is enforced strictly. See open Q2.
- Optionally decide whether `SearchUsers`/`SearchChannels`/`SearchRedemptions`
  (which match a numeric keyword against the `id` column —
  [`user.go:121`](../../model/user.go#L121), [`channel.go:534`](../../model/channel.go#L534),
  [`redemption.go:67`](../../model/redemption.go#L67); PG `SearchUsers` already
  omits id) should also match a uuid. Minor admin QoL.

---

## 5. Phasing / rollout

Schema risk (backfill, 3 dialects, split LOG_DB) is decoupled from API-cutover
risk; API cutover is decoupled per **resource** and per **ladder stage** (§3.9);
and the risky UNIQUE-index promotion is decoupled from all of it. Every phase is
independently shippable and revertible (§9).

| Phase | Content | Ladder stage | Breaking? |
| --- | --- | --- | --- |
| **P0 — Schema (additive)** | uuid + FK-uuid columns (**plain index**) on all 12 tables; `BeforeCreate` mixin; `meta.Meta` enrichment + FK population at the write sites (Appendix C); bind-safety guard (§3.10); root seed; backfill migration (Go-side, batched, idempotent, `IsMasterNode`, **LOG_DB-aware**, re-runnable). `uuid`/`*_uuid` keys start appearing in responses (additive). | S1 (emit side) | **No** — new keys only |
| **P1 — Infra** | `idresolve`, `GetXByUUID` (T-A), uuid→id cache; wire request-side **dual-parse** everywhere (§3.2, incl. the body mechanics). | S1 complete | **No** |
| **P2 — Pilot strict-out: `token`** | Flip token int keys off (tags + `tokenDTO` fields → removed); all 3 frontends' token flows already on uuid from S1. Proves own uuid + FK uuid + the hand-written `MarshalJSON` DTO + dual-parse (path & body) + the 3-frontend recipe on the smallest blast radius. | token → S2 | Yes, for int-key **readers** of token payloads (release-noted) |
| **P3..N — Roll out strict-out** | Per resource: `channel`, `user`, `redemption`, `log`, `mcp_server`; then T-B D4 (`mcp_tool`, `token_transaction`, `cost`, `trace` — omit/denormalize per §3.3), `passkey`. Each gets its own release-note entry. | per-resource S2 | Yes, per resource, announced |
| **P-final** | **After all nodes on the new binary + a backfill pass reports no NULL own uuids:** promote plain index → UNIQUE (cost.go pattern). Then error-string decision (Q2); docs/openapi; the automated no-int-leak gate (§6.2) flips to strict for all resources. | — | No |
| **P-S3 — strict-in (separate, major version)** | Reject integer ids request-side; remove the `idresolve` int branch. Completes enumeration/IDOR hardening. Ships only after the D3 deprecation window (Q3) lapses and request logs show no legacy-int traffic. | S3 | Yes, for int-id **writers** |

**Rollout runbook rule (every phase):** upgrade the **master first** (it runs
AutoMigrate at startup), then slaves — a new-binary slave against an un-migrated
schema fails on writes and `Omit()` reads (§3.6.6, T18).

Pilot rationale: `token` exercises every mechanism (own uuid, a FK uuid, the
hand-written `MarshalJSON` DTO, path + body dual-parse, all three frontends) on
the smallest surface.

---

## 6. Test matrix

| # | Layer | Scenario | Assertion |
| --- | --- | --- | --- |
| T1 | Migration | Fresh DB, each of sqlite / mysql / postgres | uuid column + plain index created; new rows get a hyphenated UUIDv7 via `BeforeCreate` |
| T2 | Migration | Pre-populated DB (existing int rows) | every row backfilled with a unique uuid; FK uuid columns backfilled to the referenced row's uuid |
| T3 | Migration | **Split `LOG_DB` (`LOG_SQL_DSN` set)** | `logs` backfilled on `LOG_DB`; `user_uuid`/`channel_uuid` filled from the `DB` id→uuid map (no cross-DB JOIN attempted) |
| T4 | Migration | Re-run backfill (idempotency) + simulate an old-binary insert (NULL own uuid) then re-run | no error/duplicate/overwrite; the NULL-uuid row is swept and filled; no second `AutoMigrate` on sqlite |
| T5 | Migration | UNIQUE-index promotion with a residual NULL **own**-uuid row present | promotion is **skipped/deferred** (gated on "no NULL own uuids"), no failure; succeeds once all own uuids filled — and is *not* blocked by permanently-NULL orphan FK uuids |
| T5b | Migration | **Orphaned FK** (hard-deleted channel/token, logs persist) | `channel_uuid`/`token_uuid` stay NULL, emitted as JSON `null` (never `""`); the re-runnable sweep keys on own-uuid so it converges and never re-scans orphans forever |
| T6 | Migration | Root-account seeding | root `User` and `Token{Id:1}` get uuids; `Token.user_uuid` = root user uuid (both seed branches, incl. `InitialRootToken==""`) |
| T7 | Unit | `idresolve.Resolve` | `"42"`→int; `"018f-…"`→uuid lookup; empty/garbage→400; unknown uuid→404; a hyphenless uuid is (correctly) treated as int and 400s |
| T8 | Model | `GetXByUUID` for each T-A model | correct row; wrong/unknown uuid → not-found; uniqueness holds post-promotion |
| T9 | API request | Every §3.2 site (27 path + relay `:channelid` + 4 query + 5 body) with a **UUID** | 200 + operates on the correct row (incl. relay `:channelid` uuid, logs `?channel=uuid`, `AdminTopUp` body `user_id` uuid). The **token-key suffix** (`auth.go:300`) is the one excluded site — asserts it stays int-only (a uuid suffix is rejected/misparsed, per §1.2) |
| T10 | API request | Same sites with a **legacy integer** id (D3 back-compat) | 200 + identical result |
| T11 | API response | Every `data:` list/detail body, stage-aware (§6.2) | **S1:** int and uuid keys coexist and are consistent (the uuid resolves to the same row as the int); **S2:** no integer `id`, no integer FK key anywhere |
| T12 | API response | FK rendering under the new keys (D6): token→`user_uuid`, log→`user_uuid`+`channel_uuid`+`token_uuid`, txn→`log_uuid` (others omitted at S2), user→`inviter_uuid`, mcp_tool→`server_uuid` | each equals the referenced row's uuid (or is absent for omitted T-B fields); a log whose channel/token was hard-deleted emits `null` (not `""`, not an int); legacy int keys present at S1, absent at S2 |
| T13 | API response | **All 12 §3.8 custom sites**: `tokenDTO`, `GetSelfByToken` (incl. `uid`,`token_id`), both tracing handlers, `buildTransactionResponse`, `PasskeyRegisterFinish`, `PasskeyList`, `DuplicateChannel`, `GetChannelMigrationStatus`, `GetToolsDisplay`, `GetDashboardUsers` (incl. the `Id:0` sentinel), the 4 `log_statistics` DTOs | emit uuid (or omit), no int leak; `UserId` capitalized key no longer an int |
| T14 | API response | Shared login (`SetupLogin cleanUser`) for password + each OAuth provider + passkey | returns user uuid, no int id |
| T15 | Auth | Session-cookie + API-token login; both admin channel selectors | internal int ctx keys unaffected; the relay `:channelid` **path param** accepts a uuid; the **token-key suffix** stays int-only and keeps working with an int (consistent with T9's exclusion — this row previously contradicted §1.2) |
| T16 | Cache | uuid→id resolver + invalidation on delete | correct id; stale entry cleared after delete |
| T17 | Relay hot path | Ability model selection + billing under load (`-race`) | unchanged behavior/perf; abilities stay int; quota accounting correct; Log/txn rows carry FK uuids **copied from the enriched `meta.Meta` with zero added DB reads** (assert query counts on the billing path) |
| T18 | Multi-node | Master + slave against shared DB; **old-binary + new-binary** rolling window, **both upgrade orders** | slave-written rows tolerated with NULL uuid; next master backfill pass fills them; unique promotion deferred until no NULL own uuids; new-binary slave before master migration **reproduces the write/`Omit`-read failure** (documents the master-first runbook rule, §3.6.6); stale cached objects from old binaries (no uuid) tolerated and swept |
| T19 | Frontend ×3 | Per resource: list, create, edit, delete, navigate, search-select | works with uuid; no `parseInt`/`Number()` breakage; berry `useState(0)`→falsy sentinel; modern tracing/self-compare/dashboard-row parsing fixed; persisted auth-store entries from the previous version migrate (persist `version` bump) without breaking sessions |
| T20 | Frontend | Blanket-sweep guard | channel-TYPE enums, `EndpointInfo.id`, chat message ids stay numeric; `/api/user/totp/disable/:id` (id-keyed) works; `/api/user/manage` (username-keyed) unchanged |
| T21 | Regression | Full existing suite `go test -race ./...` | green |
| T22 | Security | Enumeration via sequential ints against a cut-over endpoint | still resolvable only within the D3 deprecation window; no BI count/ordering derivable from responses |
| T23 | Docs | openapi.json + api_references.md | describe uuid string ids; the "ids are integers" line removed |
| T24 | Security | **Bind-safety (§3.10):** create/update every in-scope resource with a client-supplied `uuid`/`*_uuid` in the body | server-generated uuid unchanged; client value ignored on create; no clobber via the three no-allowlist `Updates(struct)` paths |
| T25 | Contract | **Type-stability snapshot (D6)** per endpoint per stage | across S0→S1→S2 no JSON key ever changes type; S1 only adds keys, S2 only removes keys (golden-file diff per endpoint) |
| T26 | Compat | **Legacy-client simulation** through S1/S2: a reader consuming only int keys; a writer sending only int ids (no uuid field) | reader works fully at S1; writer works fully through S2; run against the pilot resource first, then each cutover |
| T27 | Migration | **Backfill scale/perf:** synthetic ≥1M-row `logs` (+100k tokens) on each dialect | batched (1000/loop) with no long transaction or table lock; completes within the startup budget or defers cleanly to the next sweep; progress logged |
| T28 | Security | **Enumeration probe at S3** (int ids rejected): sequential-int walk of every T-A endpoint | 400/404 for every int id; uuid guessing infeasible (UUIDv7 random bits documented); no BI count/ordering signal derivable from any response |
| T29 | Ops | **Rollback drill (§9):** revert the pilot resource S2→S1 (restore int tags), then re-apply | legacy readers work again post-revert; no schema change needed; re-cutover is clean |

DB guardrails from prior billing/migration work apply: `SetMaxOpenConns(1)` for
concurrent sqlite tests; assert observed `ctx.Err()` (sqlite commits on cancelled
ctx); high-concurrency `-race` stress + DB accounting for any billing-adjacent
path.

### 6.1 Phase gates — what must be green before each phase ships

| Gate | Guards | Required green |
| --- | --- | --- |
| G-P0 | merging P0 | T1–T6, T17, T21, T24, T27 (sqlite in CI; mysql + pg via the compose harness, §6.3) |
| G-P1 | merging P1 | T7–T10, T15, T16, T21 |
| G-P2 | flipping `token` to S2 | T11(S1-mode), T25, T26 green **on token**; all three frontends' token flows pass the manual dev-server pass (§6.3); release note drafted |
| G-P3..N | each further resource's S2 | the G-P2 bundle scoped to that resource (its T11–T14 rows, T19, T25, T26) |
| G-Pfinal | UNIQUE promotion | T5, T5b, T18 green; a backfill report from the target deployment (or staging clone) shows **zero NULL own uuids** across all 12 tables incl. `LOG_DB` |
| G-PS3 | rejecting int request ids | deprecation window lapsed (Q3); request logs show no legacy-int traffic; T28 |

### 6.2 The automated no-int-leak gate (AC-1) — concrete implementation

A table-driven Go integration test (`controller/uuid_contract_test.go`, httptest
+ sqlite) that logs in as root, seeds one row per resource **plus one orphaned
log** (hard-deleted channel/token), calls **every** endpoint in Appendix A/B,
and walks each JSON response recursively:

- **Key denylist** (must not hold an integer once the resource is at S2): `id`,
  `user_id`, `channel`, `token_id`, `inviter_id`, `server_id`, `log_id`,
  `channel_id`, `uid`, `UserId`.
- **Key allowlist** (legitimately numeric, never flagged): channel `type`,
  `status`, quotas/counts/timestamps, pagination fields, `priority`/`weight`,
  and the JSON-RPC echo `id` inside MCP proxy payloads.
- **Stage awareness:** each endpoint row in the test table declares its current
  stage. At S1 the walker asserts *coexistence + consistency* (int key and uuid
  key both present; the uuid resolves to the same row as the int). At S2 it
  asserts *absence* of the denylisted int keys. Flipping a resource's stage in
  the table is part of that resource's cutover PR — the gate fails whenever code
  and table disagree, and the T25 golden files catch the reverse direction.
- Runs in CI as part of `go test ./controller/...`; P-final flips every row's
  expectation to S2-strict.

### 6.3 Verification environments & commands

| What | How |
| --- | --- |
| Unit/integration (sqlite) | `go test -race ./...` — CI default; `SetMaxOpenConns(1)` for concurrent sqlite per the house guardrail |
| MySQL + Postgres dialects | compose harness: run the migration suite against `mysql:8` and `postgres:16` containers (`SQL_DSN` matrix); assert T1–T6 + T27 per dialect |
| Split log DB | same harness with `LOG_SQL_DSN` pointing at a second database (T3) |
| Multi-node rolling window | two binaries (previous release tag + new build) against one MySQL; run both upgrade orders (T18) |
| Frontend unit | `cd web/modern && npm test` (vitest); air/berry have no unit suites — `npm run build` each + the manual pass below |
| Manual E2E per cutover | dev server (`make dev` + the documented backend run command): per resource, run list/create/edit/delete/search/navigate in all three UIs; one relay request with an admin `:channelid` uuid override |
| Docs gate | `openapi.json` schema asserts uuid string ids at P-final (T23) |

---

## 7. Acceptance criteria & traceability

Each criterion names the tests that prove it and the phase gate (§6.1) it blocks.

| # | Criterion | Proven by | Blocks gate |
| --- | --- | --- | --- |
| AC-1 | **No integer resource id crosses the API boundary at S2+.** The automated gate (§6.2) walks every endpoint, including the 12 custom §3.8 sites and the capitalized `UserId` keys | T11–T14, T25 | each S2 flip; strict at G-Pfinal |
| AC-2 | **Every request-side site accepts both** a UUID and a legacy integer id through S2 — all 27 path params, the relay `:channelid` selector, the 4 query params, the 5 body ids incl. `AdminTopUp`. The **one documented exception** is the token-key channel suffix (`auth.go:300`), int-only because its parser splits on `-` (§1.2) | T9, T10, T26 | G-P1 |
| AC-3 | **Backward compatibility (D3/D6):** S1 is purely additive — legacy readers *and* writers run unmodified; no JSON key ever changes type across the whole migration; every break is a *removal*, per resource, in an announced release | T25, T26, T19 | every S2 flip |
| AC-4 | **Migration is idempotent, all-dialect, split-DB-safe, and scale-tested:** backfills every row (own + FK uuids), handles `LOG_DB` without cross-DB JOINs, covers root seeding, re-runs to sweep old-binary rows, never double-runs sqlite `AutoMigrate`; UNIQUE index only after zero NULL own uuids | T1–T6, T18, T27 | G-P0, G-Pfinal |
| AC-5 | **Internal integrity untouched:** integer PKs, FK joins, `*:%d` caches, routing maps, abilities, Prometheus/OTEL labels stay int; relay + billing behavior and perf unchanged under `-race`; FK uuids come from the enriched `meta.Meta` with **zero added hot-path queries** | T15–T18 | G-P0 |
| AC-6 | **Bind-safety:** a client can never set or change any server uuid via create/update bodies (§3.10) | T24 | G-P0 |
| AC-7 | **All three frontends** perform full CRUD + navigation on every migrated resource using uuids — no `parseInt`/`Number()`/sentinel regressions, persisted stores migrated, channel-TYPE enums and non-model `id:number` fields untouched, id-keyed TOTP route handled | T19, T20 | each S2 flip |
| AC-8 | **Rollout safety:** master-first deploy order documented with its failure mode demonstrated; per-resource rollback (S2→S1) rehearsed | T18, T29 | G-P2 |
| AC-9 | **`go test -race ./...` green**; no new lint; errors wrapped per house rule (`github.com/Laisky/errors/v2`) | T21 | every merge |
| AC-10 | UUIDs are canonical **hyphenated** UUIDv7 (the `-` discriminator holds); docs/openapi describe uuid string ids | T7, T23 | G-P1, G-Pfinal |

---

## 8. Risks & mitigations

| Risk | Mitigation |
| --- | --- |
| **Declaring the uuid column `not null`/`default:''` → unique index collides on `''`** | Keep the column **nullable** (§3.1); rows backfill to `NULL`, and multi-NULL is legal in a unique index. UNIQUE promotion is still a dedicated post-backfill, `HasIndex`-guarded, dialect-aware migration (cost.go pattern), deferred to P-final — for dialect/SQLite-introspection conservatism, not to avoid a crash. (§3.6.1, T5) |
| **Split `LOG_DB` breaks FK backfill JOINs** | Log backfill runs on `LOG_DB` with a Go-side `id→uuid` map read from `DB`; no cross-DB JOIN. (§3.6.2, T3) |
| **Hard-deleted channel/token orphans a log's FK uuid → empty forever + non-terminating sweep** | Orphaned FK uuid stays `NULL`, emitted as `null`; the sweep's "work-left" predicate keys on **own** uuid only, so it converges; T12/T5b tolerate null FK. (§3.6.4, T5b) |
| **Old binaries / slaves insert NULL-uuid rows mid-rollout** | `BeforeCreate` covers new binaries; backfill is re-runnable each master start and sweeps the window; reads tolerate NULL uuid; unique promotion waits for "no NULL own uuids". (§3.6.3, T18) |
| Missed response site still leaks an int id | Tag flip centralizes most; the §3.8 verified custom-site checklist + automated gate (AC-1) catch the rest; the sweep already found the sites the first draft missed (`uid`, `token_id`, txn response, passkey, migration-status, dashboard-users, dashboard stat DTOs). |
| FK-uuid write-site omission | Every one of the 23 log rows + 2 txn sites in Appendix C enumerated; relay sites copy from the enriched `meta.Meta`; a Log/txn row with empty FK uuid is swept by the re-runnable backfill and caught by T12. |
| **Client-supplied uuid mass-assignment** (create paths, or the three no-allowlist `Updates(struct)` paths) | Bind-safety rule (§3.10): zero uuid fields post-bind + `Omit` uuid columns in `User`/`Channel`/`MCPServer.Update`; enforced by T24. |
| **New-binary slave against un-migrated schema fails writes and `Omit()` reads** | Master-first deploy order in the runbook (§5); failure mode reproduced and pinned by T18 (§3.6.6). |
| **In-place key type flip breaks JS/script parsers silently** | D6: uuids ship under new keys; int keys are removed, never re-typed; enforced by T25 contract snapshots. |
| **Stale caches during rolling upgrade** (old-binary-serialized user/token objects lack uuid) | Write sites tolerate empty FK uuid (row swept by the re-runnable backfill); user caches are TTL-bounded (`SyncFrequency`); the uuid→id cache adds delete-path invalidation (§3.5). |
| Denormalized FK uuid drift | FKs are set-once/immutable; the three no-`Select` struct updates don't clobber (GORM skips zero values); assert in T2/T12. The only "drift" is the orphan case above (referent hard-deleted after the log was written) — handled as a permanent `null`, not stale data. |
| Backfill slow on large `logs`/`token_transactions` | Batched (1000/loop), resumable/idempotent, `IsMasterNode`-gated, off the hot path. |
| Breaking external admin scripts reading `id` from responses | D4 is a deliberate break; phased per-resource rollout + release-note the int-id deprecation; D3 keeps request-side back-compat. `docs/skills/admin/scripts/oneapi` works under D3. |
| WebAuthn handle tied to int `User.Id` | Int PK unchanged → passkey login unaffected (verified). |

## 9. Rollback

- **P0/P1 additive** (columns + accept-both parsing): rollback = stop
  populating/reading uuid; columns stay inert; the plain (non-unique) index is
  harmless.
- **P2+ per-resource cutover (S2)** is revertible one resource at a time by
  restoring that resource's int JSON tags (and frontend) — i.e. dropping back to
  S1, which stays fully functional because int and uuid keys coexist by
  construction; no schema rollback since int PK/FK columns are never dropped.
  T29 rehearses this on the pilot before any further resource cuts over.
- **UNIQUE-index promotion (P-final)** is the only non-trivial-to-reverse step;
  it ships last, after the system has proven stable on uuid — a `DROP INDEX`
  reverts it.

## 10. Open questions

1. **§3.3 FK strategy** — confirm **Option A (denormalize)** for T-A + **omit** for
   T-B, vs Option B (translate-at-read). Change list is sized for A.
2. **§4.5 strict mode** — sanitize the `"Channel #%d"` client-facing relay error
   strings (or emit uuid/name), or accept them as diagnostic text?
   *Recommendation:* switch to channel **name + uuid** as part of the channel
   resource's S2 — cheap, and it keeps the strict AC-1 gate honest.
3. **Deprecation window** — how long do endpoints keep accepting legacy integer
   ids (D3) before removal, and does removal warrant a major-version bump?
   *Recommendation:* S1 ships at least one minor release before the first S2
   flip; S2 completes across resources over ≥2 minor releases; S3 (reject int
   ids) is a **major-version** release at least one release after the last S2,
   gated on request logs showing no remaining legacy-int traffic (G-PS3).
4. **T-B/T-C uniformity** — do we ship the uuid column on `TokenTransaction`/
   `UserRequestCost`/`Trace`/`AsyncTaskBinding` for D5 uniformity even though
   their external D4 need is met by omitting int fields, or skip the column to
   cut churn? (Recommendation: ship the column for `Trace`/txn only if a future
   route will address them; skip for `AsyncTaskBinding`.)
5. **The dead `/api/channel/batch`** the air frontend calls — fix air (remove/
   repoint) or add the missing backend batch route? Out of this change's scope
   but surfaced here.

---

## Appendix A — request-side parse-site inventory (verified)

| Resource | Handler | Site (file:line) | Kind |
| --- | --- | --- | --- |
| User | GetUser | [`user.go:337`](../../controller/user.go#L337) | path `:id` |
| User | DeleteUser | [`user.go:1172`](../../controller/user.go#L1172) | path `:id` |
| User | AdminDisableUserTotp | [`user.go:1735`](../../controller/user.go#L1735) | path `:id` (string) |
| User | UpdateUser | `/api/user/` PUT `payload.Id` | body |
| User | AdminTopUp | [`user.go:1455`](../../controller/user.go#L1455) `req.UserId` | body |
| User | GetUserDashboard / AdminGetAllTokens | [`user.go:403`](../../controller/user.go#L403), [`token.go:1051`](../../controller/token.go#L1051) | query `user_id` |
| Channel | GetChannel / DeleteChannel / DuplicateChannel / (balance) | [`channel.go:275,295,389,479,523`](../../controller/channel.go#L275) | path `:id` |
| Channel | GetChannelPricing / UpdateChannelPricing / UpdateChannelBalance | [`channel_billing.go:382`](../../controller/channel_billing.go#L382) | path `:id` |
| Channel | TestChannel | [`channel_testing.go:315`](../../controller/channel_testing.go#L315) | path `:id` |
| Channel | Debug / Fix / MigrationStatus | [`channel_debug.go:16,51,100`](../../controller/channel_debug.go#L16) | path `:id` |
| Channel | UpdateChannel | `/api/channel/` PUT `channel.Id` | body |
| Channel | GetAllLogs / GetLogsStat | [`log.go:28,259,280`](../../controller/log.go#L28) | query `channel` (int-coerced) |
| Channel | admin selector (relay path) | [`auth.go:314`](../../middleware/auth.go#L314) | `:channelid` — **uuid-capable** |
| Channel | admin selector (token suffix) | [`auth.go:300`](../../middleware/auth.go#L300) | `strings.Split(key,"-")` → **int-only**, not D3-capable (§1.2) |
| Token | GetToken / DeleteToken / AdminGetToken | [`token.go:121,220,1108`](../../controller/token.go#L121) | path `:id` |
| Token | UpdateToken | `/api/token/` PUT `token.Id` | body |
| Redemption | GetRedemption / DeleteRedemption | [`redemption.go:89,160`](../../controller/redemption.go#L89) | path `:id` |
| Redemption | UpdateRedemption | `/api/redemption/` PUT | body |
| MCPServer | Get/Update/Delete/Sync/Test/ListTools | [`mcp_server.go:94,140,230,249,285,323`](../../controller/mcp_server.go#L94) | path `:id` |
| MCPTool | GetMCPTools | [`mcp_tool.go:35`](../../controller/mcp_tool.go#L35) | query `server_id` |
| Log/Trace | GetTraceByLogId | [`tracing.go:71`](../../controller/tracing.go#L71) | path `:log_id` |
| Passkey | PasskeyDelete / PasskeyRename | [`passkey.go:399,420`](../../controller/passkey.go#L399) | path `:id` |

## Appendix B — response-side leak inventory (verified sweep)

**Tag-flippable (struct):** `model.User` (esp. `SetupLogin cleanUser`), `model.Log`
(`Id`,`UserId`,`ChannelId`,`TokenId`), `model.Redemption`, `model.Channel`,
`model.MCPServer` (`sanitizeMCPServer`), `model.MCPTool` (`Id`,`MCPServerId`).
Only `Token` among returned gorm models has a custom marshaller.

**Hand-built maps / custom DTOs (manual edit — the gate's target):** see the §3.8
table. Cleared false-positives: JSON-RPC echo `id` (`mcp_proxy.go:352,363`), OAuth
**app** `client_id` (`misc.go`, `auth/*`), string `transaction_id`/`request_id`/
`trace_id`, upstream `SiliconFlowUsageResponse.Data.ID`, `OpenAIModels.Id`
(=model name), request payloads (`adminTopUpRequest`, `UserAdminUpdatePayload`),
and all `model.Log`/`TokenTransaction` **input** struct literals. No response
header / SSE / WebSocket / relay-body int-id leak exists.

## Appendix C — FK-uuid write-site population (Option A)

**Log — 23 distinct rows** (the `token.go` 384/396 and 654/666 pairs are one
build+write each): [`billing.go:108,111,248,311`](../../relay/billing/billing.go#L108),
[`proxy.go:72`](../../relay/controller/proxy.go#L72),
[`image.go:584,599`](../../relay/controller/image.go#L584),
`video.go:196`, `rerank.go:353`, `ocr.go:339`, `audio.go:361`,
[`billing_safety.go:433`](../../relay/controller/billing_safety.go#L433),
`realtime_billing.go:117`, `channel_testing.go:205`,
[`token.go:384,654`](../../controller/token.go#L384) (+ tool logs `396,666`),
`mcp_proxy.go:297`, [`redemption.go:157`](../../model/redemption.go#L157),
[`user.go:184,189,193`](../../model/user.go#L184) (gifts),
`controller/user.go:1069,1471,1778`. Most record helpers funnel through
[`recordLogHelper`](../../model/log.go#L423) (`LOG_DB.Create`); the two that call
`LOG_DB.Create` directly are `RecordToolLogs` ([`log.go:642`](../../model/log.go#L642))
and `RecordProvisionalConsumeLog` ([`log.go:689`](../../model/log.go#L689))
(`RecordTestLog`/`RecordTestLogWithIDs` *do* funnel through `recordLogHelper`).
Whichever path, each `Log` needs `user_uuid`/`channel_uuid`. **Population source
(corrected in rev. 3):** the relay/billing sites hold only int ids
([`meta.Meta`](../../relay/meta/relay_meta.go#L41) carries `ChannelId`/`TokenId`/
`UserId int` + `TokenName`) and the detached billing snapshot
([`billingIdentity`](../../relay/controller/billing_ctx.go#L20)) holds **no**
`*User`/`*Channel` — nothing is "free" there. They copy the strings from the
enriched `meta` (§3.3). The int-only helpers (`RecordLog`/`RecordTopupLog`/
`RecordManageLog` — topup/gift/manage paths, off the hot path; they already do a
per-row `GetUsernameById`) use an internal lookup or leave NULL for the sweep.

**TokenTransaction (2 sites)** — [`controller/token.go:416,692`](../../controller/token.go#L416)
via [`CreateTokenTransaction`](../../model/token_transaction.go#L88): set
`log_uuid` (and, if kept, `token_uuid`/`user_uuid`). Note `LogId` is nil whenever
`IsLogConsumeEnabled()` is off (`RecordToolLog` early-returns —
[`log.go:548`](../../model/log.go#L548)); `log_uuid` is NULL in exactly the same
cases and is emitted as JSON `null`.

**Other FK-uuid write paths** — `MCPTool.server_uuid`: rows are created only in
[`UpsertMCPTools`](../../model/mcp_tool_store.go#L87), which receives `serverID
int`; its sole caller ([`relay/mcp/sync.go:53`](../../relay/mcp/sync.go#L53))
holds the full `*MCPServer`, so extend the signature to pass the server uuid.
`Token.user_uuid`: [`Token.Insert`](../../model/token.go#L286) callers hold the
int only ([`controller/token.go:207`](../../controller/token.go#L207)) or the
full user ([`model/user.go:207`](../../model/user.go#L207)); the **root-seed
token bypasses `Insert` entirely** ([`main.go:68`](../../model/main.go#L68)) —
set it at the seed site (§3.7). `User.inviter_uuid`: only `inviterId int` is in
scope at [`user.go:193`](../../model/user.go#L184) — lookup (registration path,
off the hot path).

## 11. Review changelog (what changed vs the first draft, and why)

**Blocking bug fixed.** The first draft told AutoMigrate to create the
`uniqueIndex` from the struct tag, then backfill. On any populated MySQL/PG
deployment that fails at startup — AutoMigrate builds a unique index over the
freshly-added column while every existing row holds `''`. The codebase's own
`cost.go` migration exists precisely to build such an index *after* dedup. Fixed
to plain-index-then-post-backfill-UNIQUE (§3.6.1, §5 P-final).

**Two architectural realities the draft missed.** (1) `Log` lives on a separate
`LOG_DB` when `LOG_SQL_DSN` is set, so the proposed `UPDATE … JOIN` FK backfill is
impossible — replaced with a Go-side `id→uuid` map (§3.6.2). (2) Slaves and
old-binary nodes write rows directly during a rolling upgrade, so the backfill
must be re-runnable and the unique index deferred (§3.6.3).

**Factual corrections.** `dto.EnabledAbility.ChannelId` is *not* a response leak
(never serialized) — removed. `/api/channel/batch` has no backend route — the air
"batch ids → strings" work item and its test were chasing dead code; removed.
`/api/user/totp/disable/:id` is **id-keyed**, not username-keyed — the draft's
§4.4 parenthetical was wrong and internally inconsistent with its own §4.3.
`AdminTopUp` (`/api/topup`, body `user_id`) was a missing request-side site. The
logs `channel` query param is **server-side** int-coerced (`strconv.Atoi`), so the
work is backend, not frontend.

**Response inventory expanded by a full sweep.** The draft's §3.6 listed 5 custom
sites; the verified sweep found 12, adding `GetSelfByToken`'s `uid`+`token_id`
(the draft caught only `id`s), `buildTransactionResponse`, `PasskeyRegisterFinish`,
`PasskeyList`, `DuplicateChannel`, `GetChannelMigrationStatus`, `GetToolsDisplay`,
and `GetDashboardUsers` (including an `Id:0` "All Users" sentinel that mirrors
berry's numeric-0 problem). `GetSelfByToken` is at `user.go:660`, not 680.

**Scope refined into tiers.** Only 7 models are route-addressable by their own int
PK (the actual enumeration surface); 4 more are serialized-only (D4 via omit); 1
(`AsyncTaskBinding`) has no HTTP surface at all. This concentrates the
resolver/frontend work where it matters and lets T-B meet D4 by dropping
redundant fields instead of building denormalized columns nobody reads.

**Frontend nuances corrected.** The dashboard break is stat-row parsing
(`Number(row.UserId)`), not the (already string-safe) filter; the modern auth
store `User.id` is persisted to `localStorage`; `useClipboardManager`'s
`Record<number,…>` key type must flip; and a blanket `id:number→string` sweep
would wrongly convert channel-TYPE enums, `EndpointInfo.id`, and chat message ids
— all called out as must-stay-numeric.

### 11.1 Second-pass corrections (adversarial verification round)

A follow-up verification workflow (per-claim anchor checks + a completeness
critic) caught five more issues, now folded in:

- **Blocking — D3 vs the token-key delimiter.** The admin token-key channel
  suffix is parsed by `strings.Split(key, "-")`
  ([`utils.go:268`](../../middleware/utils.go#L268) → [`auth.go:300`](../../middleware/auth.go#L300)),
  *not* `:` as the first draft's D3 note claimed. A hyphenated UUID appended there
  shatters, so that single site stays int-only. Fixed §1.2, §3.2, Appendix A, T9,
  AC-2. (The relay `:channelid` path param is unaffected and remains uuid-capable.)
- **Overstated migration crash walked back.** The "AutoMigrate builds a unique
  index over empty strings and fails deterministically" justification was wrong:
  the added column is *nullable*, rows backfill to `NULL` (not `''`), and
  multi-NULL is legal in a unique index on all three dialects — so it would not
  crash. The real rule is **keep the column nullable**; the post-backfill UNIQUE
  promotion stays, but justified by dialect/SQLite-introspection conservatism, not
  crash-avoidance. Fixed §3.1, §3.6.1, §8.
- **Major — hard-delete FK orphans.** `channel.Delete()`/token delete are hard
  deletes (no `DeletedAt`), so logs referencing a deleted channel/token cannot be
  FK-backfilled; `user.Delete()` is soft, so `user_uuid` is always fillable. Added
  the orphan policy (null FK uuid; own-uuid-only sweep predicate) to §3.6.4/§3.6.5,
  §8, T5b, T12.
- **Major — log-statistics is a SQL rewrite.** The four `UserId` stat DTOs come
  from `GROUP BY user_id`; emitting a uuid requires grouping+selecting
  `logs.user_uuid` (after the Log denormalization lands) and satisfying
  `ONLY_FULL_GROUP_BY`, not a passive field flip. Fixed §3.8.
- **Minor anchor/count fixes.** Path-param count reconciled 24 → **27**;
  `SetupLogin cleanUser` anchor corrected to `user.go:192-198`; the "test log
  helpers call `LOG_DB.Create` directly" note corrected (only `RecordToolLogs` +
  `RecordProvisionalConsumeLog` do; the test helpers funnel through
  `recordLogHelper`).

### 11.2 Third-pass revision (backward-compatibility & acceptance overhaul)

A third verification round — five parallel sweeps re-checking every claim
(request sites, response sites, migration/model behavior, all three frontends,
FK write sites) — plus a compatibility-focused design review produced:

- **Corrected — the billing-snapshot claim was wrong.** `billingIdentity`
  ([`billing_ctx.go:20-25`](../../relay/controller/billing_ctx.go#L20)) holds no
  `*User`/`*Channel`; every relay/billing log site carries only the int ids in
  `meta.Meta`. Hot-path FK-uuid population is redesigned as **`meta.Meta`
  enrichment** — uuids captured in auth/distributor middleware where those rows
  are already fetched — instead of "free from the snapshot" (§3.3, Appendix C).
  Log-row count corrected ~20 → **23 distinct rows** (the `token.go` 384/396 and
  654/666 pairs are one row each).
- **New locked rule D6 — no key type flip.** Rev. 2's Option A re-used `user_id`
  as the uuid's JSON key: an in-place int→string type change that breaks JS
  parsers *silently* (`parseInt(uuid)` → `NaN`). UUIDs now ship under new
  `uuid`/`*_uuid` keys; int keys are dropped at S2, never re-typed (§1.2, §3.3,
  §3.9, T25).
- **Explicit compatibility ladder S0→S3 (§3.9)** replacing the implicit jump to
  uuid-only responses: S1 dual-emit/dual-parse is purely additive (legacy readers
  *and* writers unaffected); S2 strict-out per resource; S3 strict-in (reject int
  request ids) as a separate major-version phase (P-S3). Body-site dual-parse
  mechanics spelled out (§3.2): resolve `payload.UUID` first, fall back to the
  legacy int `id`, then zero the uuid fields.
- **New — bind-safety / mass-assignment guard (§3.10, T24).** The uuid json tags
  are bindable: create paths would accept client-chosen uuids, and the three
  no-allowlist `Updates(struct)` paths (`user.go:231`, `channel.go:1929`,
  `mcp_server_store.go:101`) would write a client-supplied non-zero uuid.
  Handlers zero uuid fields post-bind; those three `Update`s gain `Omit`.
- **New rolling-upgrade hazard (§3.6.6, T18).** A new-binary slave against an
  un-migrated schema hard-fails on all writes and on `Omit()` reads
  (`user.go:83,121,123,140`, `channel.go:489,534,547`) — only plain `SELECT *`
  reads survive. Master-first deploy order is now an explicit runbook rule (§5).
- **Cache and guard-claim corrections (§3.5, §3.6.5).** The user-scoped caches
  (`user_obj:%d` etc.) are TTL-only — nothing `RedisDel`s them today; only token
  and group-model caches have active invalidation. The sqlite "AutoMigrate-once
  guard" is a comment/convention, not a runtime flag. Old-binary-serialized
  cached objects lack the uuid field → tolerated and swept.
- **Write-path nuances (Appendix C).** Root-seed user/token use raw `DB.Create`,
  bypassing the `Insert()` wrappers (§3.7); `UpsertMCPTools` receives only
  `serverID int` (signature extension needed); `TokenTransaction.LogId` — and
  therefore `log_uuid` — is legitimately NULL when consume-logging is disabled;
  the int-only `Record*` helpers already do per-row `GetUsernameById` lookups,
  so an analogous uuid lookup off the hot path is precedented.
- **Frontend inventory refreshed against the current tree.** Path drift:
  `useChannelForm.ts` → `pages/channels/hooks/`, `useDashboardData.ts` →
  `pages/dashboard/hooks/`; the log type is named `LogEntry`; berry's parseInt
  lines pinned (`Channel/component/EditModal.js:369`, Token `:72`, User `:77`,
  Redemption `:53`). Two missed sites added: modern `ChannelsPage.tsx:119`
  (balance-refresh `Set<number>` of channel ids) and `EditMCPServerPage.tsx:22`
  (local `MCPTool.id: number`). New requirement: persisted-store migration for
  the localStorage-cached user object (persist `version` bump).
- **Acceptance made operational.** Phase gates with entry conditions (§6.1); a
  concrete, stage-aware implementation of the no-int-leak gate (§6.2);
  verification environments and commands, incl. the mysql/pg/split-`LOG_DB`
  matrices and the manual three-frontend pass (§6.3); six new tests (T24–T29:
  bind-safety, type-stability snapshots, legacy-client simulation, backfill
  scale, S3 enumeration probe, rollback drill); and a criteria↔tests↔gates
  traceability matrix (§7).
- **Recommendations recorded on open questions:** relay error strings switch to
  channel name + uuid at the channel S2 (Q2); concrete deprecation windows and a
  major-version S3 (Q3).
