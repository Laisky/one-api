# Change Manual: Boundary Response DTOs — Retiring Model-Level `MarshalJSON`

- Status: **Implemented** (code landed; verified 2026-07-15 — see §10 Verification results)
- Date: 2026-07-14 (proposed) / 2026-07-15 (verified)
- Area: management API serialization / model layer / caching / security / test & lint infrastructure
- Related: [`20260703_external-uuid-identifiers.md`](20260703_external-uuid-identifiers.md) (defines the S2 strict-out contract this manual **preserves byte-for-byte**), GitHub issue [#353](https://github.com/Laisky/one-api/issues/353) (the production incident that exposed the current design's failure mode)

> **Scope guarantee up front:** this is a **pure refactor of *where* the external
> JSON contract is enforced** — from ambient model-type `MarshalJSON` methods to
> explicit response DTOs invoked at the API boundary. The external contract
> itself (S2 strict-out: UUID keys only, no integer ids, no secrets) does **not
> change by a single byte**. No URLs, no request formats, no DB schema, no
> frontend changes.
>
> **Behavioral invariance is the acceptance bar, not an aspiration.** This
> manual treats *every* observable behavior — success bodies, error bodies,
> status codes, inbound parsing semantics, auth/login flows, cache contents,
> billing outcomes — as frozen. §2.1 enumerates the invariants (I1–I7) and maps
> each to the test that enforces it; §6 includes a black-box differential
> harness (T17) that captures the full endpoint surface before each phase and
> byte-diffs it after, so "architecture changed, behavior didn't" is proven
> mechanically per phase rather than asserted.

---

## 1. Background

### 1.1 The incident (issue #353)

After a deploy, `/v1/chat/completions` intermittently returned
`500 "user id is empty"` with `user_id: 0` in logs. Root cause chain:

1. [`CacheGetUserById`](../../model/cache.go#L84) cached the whole user into
   Redis (`user_obj:<id>`) via `json.Marshal(user)`.
2. `json.Marshal` on a `model.User` invokes the **API-facing**
   [`User.MarshalJSON`](../../model/user_json.go#L37) — a whitelist DTO
   (`userJSON`) that *deliberately* omits the internal integer `Id` (and
   `InviterId`, and secrets), per the S2 strict-out contract.
3. The cached payload therefore had **no `"id"` key**. On a cache **hit**,
   `json.Unmarshal` left `user.Id == 0`.
4. `TokenAuth` then set [`ctxkey.Id = user.Id`](../../middleware/auth.go#L292)
   → `meta.UserId = 0` → quota sync rejected id 0 → 500. Intermittent because
   it fires only on cache hits; flushing `user_obj:*` masked it until the cache
   repopulated.

The hotfix (shipped) mirrors the token cache's existing counter-measure: a
`type plainUser User` alias at [`model/cache.go:115`](../../model/cache.go#L115)
strips the method set so the cache serializes raw fields (with secrets
explicitly scrubbed). The token cache had used `type plainToken Token`
([`model/cache.go:60`](../../model/cache.go#L60)) from the start — meaning the
codebase already knew this trap and defended **one of two** identical sites.
That inconsistency *is* the design smell this manual removes.

### 1.2 The structural defect: the contract is ambient, not boundary-owned

A Go value-receiver `MarshalJSON` participates in **every** `json.Marshal` of
that type — HTTP responses, Redis caches, log encoders, future queue/blob
serialization — with no way to distinguish intent. The S2 contract ("hide
integer ids externally") was implemented as exactly such a method on five model
types, so the *external* contract silently governs *internal* serialization
too. Consequences observed in the tree today:

- **The boundary is not in control.** The login handler builds
  `cleanUser := model.User{Id: user.Id, …}` and returns it
  ([`controller/user.go:194-205`](../../controller/user.go#L194)) — the handler
  explicitly sets `Id`, and the type's marshaler silently drops it on the way
  out. The code *reads* as if the handler decides the shape; it does not.
- **Internal code must remember to opt out.** Every internal
  whole-struct serialization needs a `plainX` alias
  (`plainToken` [cache.go:60](../../model/cache.go#L60), `plainUser`
  [cache.go:115](../../model/cache.go#L115)). Forgetting one is invisible at
  compile time, invisible in code review (the call site looks innocent), and
  surfaces as a production 500 — issue #353 exactly. This is
  **default-dangerous / opt-into-safety**.
- **Embedding promotes the trap.** A wrapper embedding one of these types
  inherits the marshaler; `channelListItem` had to hand-write an override plus
  byte-splicing to add one field
  ([`controller/channel_testing_model.go:86-115`](../../controller/channel_testing_model.go#L86)).
  A prior instance of this exact promotion bug shipped and had to be fixed
  (see memory: the channel wrapper silently dropped `test_models`).

The 2026-consensus arrangement (echoed by Go community practice around
model/view separation, and by this repo's own newer code — see §1.4) is the
inverse: **the persistence/domain struct keeps honest default serialization;
the external shape is an explicit view constructed at the boundary.** Internal
paths are then safe *by default*, and the external contract is enforced by
tests and static analysis rather than by a method with global reach.

### 1.3 Why `json:"-"` on `Id` cannot work (and DTOs can)

> **Corrected 2026-07-15 (verification).** This section originally rejected the
> `json:"-"` alternative on **D3 grounds** — "management endpoints must keep
> *accepting* legacy integer ids in request bodies, so `AddToken`/`UpdateToken`
> still parse legacy writers". **That premise is false against the current
> tree**, and the rejection needed a better reason. Both are set out below. The
> conclusion (DTOs, not tags) is unchanged and in fact strengthened.

**The D3 premise is obsolete.** Legacy integer-id *inbound* acceptance is
already gone — retired by the previous UUID proposal in commit `99c5ed01`, not
by this manual. The evidence is unambiguous:

- [`common/idresolve.Resolve`](../../common/idresolve/idresolve.go#L25) rejects
  any ref lacking a `-`; its doc states "digit-only legacy integer ids are
  rejected".
- [`preferUUIDRef(uuid, id)`](../../controller/id_refs.go#L139) — used by
  `UpdateToken`, `UpdateChannel`, `UpdateRedemption`, `ManageUser` and
  `user.go:1533` — documents its `id` parameter as "**ignored** legacy integer
  id field retained only for decoding old payloads", and returns
  `resource uuid is required` whenever `uuid` is empty.
- The repo's own committed gate asserts the rejection:
  [`TestTokenStrictInResponses`](../../controller/uuid_contract_test.go#L594)
  requires `success == false` for both `GET /api/token/1` and
  `PUT /api/token/ {"id":1,"status":1}`.
- Confirmed live on both binaries: `PUT /api/token/` with `{"id":<int>}`
  returns `resource uuid is required` **identically** pre- and post-refactor.

So the integer-id tags are *decoded and discarded*; no legacy writer works
today, and tagging `Id` as `json:"-"` would break no inbound behavior that
currently functions.

**The real reason `json:"-"` on `Id` is ruled out — the internal cache needs
it.** `Id` must remain serializable because the Redis object cache
round-trips the whole struct: `json.Marshal` → `user_obj:<id>` →
`json.Unmarshal`. Tagging `Id` as `json:"-"` would strip the integer id from
the cached payload and reconstruct `user.Id == 0` on every cache hit — which is
**exactly issue #353**, reintroduced by construction rather than by oversight.
The field must stay honest for internal serialization *and* hidden externally;
tags cannot express that split in either direction, which is precisely why the
external shape has to be an explicit boundary view. This is a stronger argument
than the D3 one it replaces: D3 was a compatibility preference, whereas the
cache requirement is the proposal's own founding bug.

### 1.4 What already exists (this refactor converges, it does not invent)

- **A boundary-DTO style is already in the tree** and is the target idiom:
  `GetSelfByToken` hand-builds a uuid-only `gin.H`
  ([`controller/user.go:701-741`](../../controller/user.go#L701));
  `GetDashboardUsers` uses a `UserOption` DTO; `DuplicateChannel` returns
  `gin.H{uuid,name}`. These sites need **no change**.
- **The `dto` package exists** ([`dto/`](../../dto/)) and already holds
  request/statistics DTOs. Note the dependency direction: **`model` imports
  `dto`** ([`model/ability.go:17`](../../model/ability.go#L17),
  [`model/cache.go:21`](../../model/cache.go#L21)), so `dto` must never import
  `model` — this dictates the mapper placement in §3.1.
- **The enforcement gate exists**:
  [`controller/uuid_contract_test.go`](../../controller/uuid_contract_test.go)
  (769 lines) drives real handlers via httptest and asserts, per endpoint, that
  responses contain `uuid` keys and **do not contain** `id`/`user_id` keys.
  This gate is what makes the migration safe: any handler that loses its
  id-hiding during the refactor fails CI loudly.
- **Static-guardrail infrastructure exists**: `.ast-grep/rules/` +
  [`sgconfig.yml`](../../sgconfig.yml), run by `make ast-grep-scan` and CI
  ([`.github/workflows/lint.yml`](../../.github/workflows/lint.yml)).
- **The regression tests from issue #353 exist**:
  [`model/cache_user_id_test.go`](../../model/cache_user_id_test.go) proves the
  cache round-trip preserves `Id` and scrubs secrets, and
  `TestUserMarshalJSON_StillHidesInternalIntId` guards the outbound contract.

### 1.5 Verified inventory (what must move)

Five model types carry a whitelist `MarshalJSON`; **28 handler sites** rely on
it (full table in Appendix A):

| Entity | Marshaler | Handler sites | Notes |
| --- | --- | --- | --- |
| `model.Redemption` | [`redemption.go:44`](../../model/redemption.go#L44) | 4 (all admin) | smallest, no secrets, no wrappers → **pilot** |
| `model.Log` | [`log_json.go:15`](../../model/log_json.go#L15) | 5 (admin/self/token) | list-heavy; largest row counts |
| `model.Token` | [`token.go:61`](../../model/token.go#L61) | 9 (self/admin/**TokenAuth**) | key-prefix normalization lives in the marshaler; `ConsumeToken` returns `data: updatedToken` to API clients ([`controller/token.go:357`](../../controller/token.go#L357)) |
| `model.User` | [`user_json.go:37`](../../model/user_json.go#L37) | 6 | includes the `SetupLogin` funnel (password + 4 OAuth providers + passkey all serialize at [`user.go:202`](../../controller/user.go#L202)); **secrets live on this struct** |
| `model.Channel` | [`channel_json.go:15`](../../model/channel_json.go#L15) | 4 (all admin) | via 2 shared builders + the `channelListItem` embedding wrapper |

Verified **absent**: no SSE/websocket/CSV-export path serializes these five
types; the only non-`c.JSON` outbound marshal is
[`buildChannelResponsePayload`](../../controller/channel.go#L198) (counted
above). The Redis caches in `model/cache.go` are the only internal
whole-struct JSON round-trips (independently confirmed by the issue-#353
adversarial audit: 22 serialization sites classified, no other instance).

**Frontend readiness (verified across all three themes):** modern, air and
berry key every user/token/channel/redemption/log operation by `uuid` (with a
`uuid || id` fallback), have **no strict response schemas** (modern's zod and
berry's yup are *input*-only, bound to form resolvers — there is no `safeParse`
on any response), and **depend on** no integer FK field. Integer-id residues do
exist and are **out of scope / unaffected**, but note the corrected reasoning
below — both are pre-existing defects inherited from the *previous* UUID
proposal, not consequences of this one:

- The air+berry dashboard "all users" sentinel `id === 0` is **already dead
  code**: `GetDashboardUsers`' `UserOption` emits **no `id` at all**
  ([`controller/user.go:606-610`](../../controller/user.go#L606)) — it was
  removed by commit `99c5ed01` under the 20260703 UUID proposal, and
  [`uuid_contract_test.go:344`](../../controller/uuid_contract_test.go#L344)
  already asserts its absence. So `option.id === 0` evaluates
  `undefined === 0` → always false. Selection still works only via the
  `uuid || id` fallback (`String(user.uuid || user.id)` → `"all"`, driven by
  the `UUID: "all"` sentinel at [`user.go:614-618`](../../controller/user.go#L614)).
  This refactor does not touch it either way.
- Log-row React keys reading `row.id` are cosmetic in modern and berry (berry
  suffixes the row index, so keys stay unique). The one real defect is
  `web/air/src/components/LogsTable.js:308` (`logs[i].key = '' + logs[i].id`),
  which collapses **every** Semi `<Table>` rowKey to the literal string
  `"undefined"` (the table passes no `rowKey` prop, so Semi defaults to
  `rowKey="key"`). That degrades row reconciliation, not merely a console
  warning. It is pre-existing — log rows already omit `id` today — and
  refactor-invariant, but it should be filed separately rather than recorded
  here as working behavior.

---

## 2. Goals and non-goals

### 2.1 Behavioral invariance contract (I1–I7)

Every phase must preserve all seven invariants; each is enforced by named test
rows, never by review alone:

| # | Invariant (frozen behavior) | Enforced by |
| --- | --- | --- |
| I1 | **Success responses**: every endpoint's status code + JSON key-set + values are identical for identical requests and DB state | T1 (golden per entity), T3 (contract gate), **T17 (black-box differential capture of the whole surface)** |
| I2 | **Error responses**: invalid/malformed/unauthorized requests produce the same status code and error body as today (error-path serialization never went through the entity marshalers, but the flipped handlers and new request DTOs touch these paths) | T18, T17 (the capture set includes error cases) |
| I3 | **Inbound semantics**: every request body accepted today is accepted with identical effect (fields parsed, omitted-vs-empty distinctions, unknown-field tolerance, D3 legacy int ids) | T7, T8 |
| I4 | **Auth & session flows**: password login (incl. TOTP branch), all four OAuth providers, passkey, session-cookie reuse, access-token header auth — end-to-end journeys behave identically | T19, T15 |
| I5 | **Program-internal behavior**: Redis cache keys/TTLs unchanged; cached-user round-trip preserves `Id` (the #353 net); relay request → billing → log write produces identical DB rows and quota deltas | T6, T11, T20 |
| I6 | **Secret confinement**: no serialization path emits `password`/`access_token`/`totp_secret`/`verification_code` — before, during, and after the migration | T4, T5, T16 |
| I7 | **Performance envelope**: list-endpoint serialization within ±5% of current ns/op and allocs/op | T13 |

A phase PR that cannot demonstrate all seven (via its gate in §6) does not
merge. Because flip + marshaler removal are a single PR per entity (§3.3),
there is no intermediate deploy state with different behavior.

### Goals

| # | Goal |
| --- | --- |
| G1 | **Byte-identical external contract & full behavioral invariance.** Every migrated endpoint emits exactly the JSON it emits today, and every observable behavior (I1–I7, §2.1) is preserved — this change is an architecture relocation, invisible to every user and every client program. |
| G2 | **Default-safe internal serialization.** After migration, `json.Marshal` on a model struct yields honest, id-bearing JSON — the entire `plainX`-alias class of bug (issue #353) becomes unrepresentable. |
| G3 | **Secrets deny-by-default.** `User.Password`, `User.AccessToken`, `User.TotpSecret` become `json:"-"`: no serialization path — response, cache, or future log/queue — can emit them, even by accident. |
| G4 | **Machine-enforced boundary.** The guarantee "no integer id / secret crosses the API" moves from a type method to: the existing per-endpoint contract gate + per-entity golden tests + a type-aware static analyzer that forbids raw model types in `c.JSON`. |
| G5 | **Per-entity revertibility.** Each entity migrates in its own PR; reverting one entity is a clean `git revert` with no schema or cross-entity coupling. |

### Non-goals

- No change to request handling: D3 dual-parse (uuid or legacy int) stays; all
  inbound tags stay (§1.3).
- No new response fields, no S3 strict-in, no URL changes, no DB migration.
- No change to `GetDashboardUsers`' `UserOption`. (Its air/berry `id === 0`
  sentinel is already inert — `UserOption` emits no `id`; the themes fall back
  to the `UUID: "all"` sentinel. See §1.5. This refactor neither fixes nor
  worsens that.)
- Field-level serializers (`JSONStringSlice`, `JSONStringMap`,
  `MCPToolPricingMap`, `LogMetadata`) are lossless round-trippers, not
  whitelist DTOs — **out of scope**, they stay.

---

## 3. Design

### 3.1 Shapes in `dto`, mappers on the model types

Because `model` already imports `dto` (§1.4), the pure response shapes live in
`dto` (leaf package, imports nothing from this repo) and the conversion
functions live in the `model` package, using the existing `model → dto` edge:

```go
// dto/responses.go — pure shapes, one per entity, copied verbatim from the
// current whitelist DTOs (Appendix B). Example:
type TokenResponse struct {
    UUID           string  `json:"uuid"`
    UserUUID       *string `json:"user_uuid"`
    Key            string  `json:"key"`
    // … exactly the fields of today's tokenDTO (model/token.go:71-87)
}

// model/token_view.go — explicit, boundary-invoked mapper.
// Moves the key-prefix normalization out of MarshalJSON unchanged.
func (t *Token) ToResponse() dto.TokenResponse { … }
func TokensToResponses(ts []*Token) []dto.TokenResponse { … }
```

Handlers change mechanically:

```go
// before                            // after
c.JSON(200, gin.H{"data": token})    c.JSON(200, gin.H{"data": token.ToResponse()})
c.JSON(200, gin.H{"data": tokens})   c.JSON(200, gin.H{"data": model.TokensToResponses(tokens)})
```

The hiding is thereby **explicit at every boundary call site** and absent
everywhere else. (Package placement of the mapper is a dependency-direction
necessity, not a layering statement; the property that matters is that the
mapping is an explicit call, never ambient.)

Design rules for the shapes:

- **Copied, not redesigned.** Each `XResponse` replicates today's whitelist
  struct field-for-field (Appendix B is the frozen source of truth). Any field
  addition/removal is a *separate* proposal.
- List mappers pre-allocate (`make([]dto.LogResponse, 0, len(logs))`) — log
  lists are the hot path (up to `MaxRecentItems` rows per page).
- `nil`-safety: mappers accept `nil` receivers and return the zero shape, so
  wrapper code (e.g. `channelListItem` with `item.Channel == nil`,
  [`channel_testing_model.go:97`](../../controller/channel_testing_model.go#L97))
  keeps its current behavior.

### 3.2 Secrets become unmarshalable (`json:"-"`), request DTOs absorb inbound

Today `User.Password/AccessToken/TotpSecret` have live JSON tags
([`model/user.go:39,49,50`](../../model/user.go#L39)) and are hidden outbound
*only* by the marshaler. Removing the marshaler without re-protecting them
would let any stray `json.Marshal(user)` (a future log statement, a future
cache) emit a bcrypt hash or a TOTP seed. Therefore, **before** the User
marshaler is removed:

1. Tag the three secret fields `json:"-"`. (`VerificationCode` is inbound-only
   and also moves to the request DTO; `TotpSecret` already has `omitempty` but
   that is not protection.)
2. Exactly **three** handlers bind inbound JSON into `model.User` and read
   these fields — each gets a small request DTO in `dto`:
   - `Register` ([`controller/user.go:234`](../../controller/user.go#L234)) →
     `dto.UserRegisterRequest` (username, password, display_name, email,
     verification_code — enumerate at implementation from the handler's actual
     reads, locked by the bind-equivalence test T7).
   - `CreateUser` ([`controller/user.go:1268`](../../controller/user.go#L1268))
     → `dto.UserCreateRequest`.
   - `UpdateSelf` ([`controller/user.go:1116`](../../controller/user.go#L1116))
     → `dto.UserSelfUpdateRequest` (pointer fields; the handler already
     re-reads the raw body to distinguish omitted-vs-empty — that logic stays).
3. All other password/secret flows already use dedicated request structs
   (`LoginRequest`, `UserAdminUpdatePayload`, TOTP `req` structs) — no change.

Beneficial side effect: the cache's explicit secret scrub
([`model/cache.go:117-119`](../../model/cache.go#L117)) becomes redundant
(kept as belt-and-suspenders or removed in P6 — either is safe once `json:"-"`
lands, because the tag makes the scrub's failure mode unrepresentable).

### 3.3 Per-entity migration mechanics (the ordering that keeps every commit green)

Each entity migrates in **one PR** with this internal order:

1. **Golden capture (before any code change):** a generator test marshals a
   fully-populated `model.X` through the *current* `MarshalJSON` and commits
   the bytes to `model/testdata/x_response.golden.json`. This freezes today's
   contract as data.
2. **Shape + mapper:** add `dto.XResponse` + `ToResponse()`; unit test asserts
   `json.Marshal(x.ToResponse())` ≡ golden bytes (T1). At this point the model
   marshaler still exists; the two coexist without interaction.
3. **Handler flip:** switch that entity's handler sites (Appendix A) to the
   mapper. Responses are byte-identical (same shape via a different path), so
   `uuid_contract_test.go` and all golden tests stay green.
4. **Marshaler removal — same PR:** delete the model-level `MarshalJSON`.
   *Everything that depended on it flips in this same commit:*
   - **Channel only:** rewrite
     [`buildChannelResponsePayload`](../../controller/channel.go#L198) to build
     from `channel.ToResponse()` (then splice `tooling` as today), and replace
     [`channelListItem.MarshalJSON`](../../controller/channel_testing_model.go#L86)'s
     byte-splicing with a plain struct: `struct { dto.ChannelResponse;
     TestModels []string `json:"test_models"` }` — the embedding-promotion
     hazard disappears because `dto.ChannelResponse` has no methods.
   - **User only:** §3.2 (secrets `json:"-"` + the three request DTOs) must be
     in this PR or an earlier one — never later.
   - The entity's rows in `uuid_contract_test.go` re-run unchanged and must
     stay green — they are the proof the flip lost nothing.
5. **Cache simplification (User/Token PRs or deferred to P6):** with the
   marshaler gone, `plainUser`/`plainToken` aliases become no-ops; remove them
   and re-run [`model/cache_user_id_test.go`](../../model/cache_user_id_test.go).

   **Mixed-version note — corrected against measurement (T11).** This manual
   originally asserted that "cache payloads only ever gain keys relative to the
   current fixed binary". That is **false**, as
   [`model/cache_mixed_version_test.go`](../../model/cache_mixed_version_test.go)
   now proves against fixtures generated by the real pre-refactor binary
   (git `6ebe28a5`):

   - `user_obj:<id>`: post-refactor **loses** three keys —
     `pre \ post = {"password", "access_token", "verification_code"}` — and
     gains none. The old `plainUser` alias emitted `password`/`access_token` as
     empty strings (the manual scrub blanked them but could not remove them),
     and emitted `verification_code` *unscrubbed*. All three are now `json:"-"`.
   - `token:<key>`: the delta is **empty in both directions** (17 keys each).
     The predicted "(for Token) gains `id`/`user_id`" never applied —
     `plainToken` already emitted both.

   The conclusion (**no cache flush needed**) nevertheless holds, for a
   different reason than the one originally given: the three dropped keys are
   exactly the fields that are now `json:"-"`, so a *reader* of either vintage
   ignores them on unmarshal, and none of them was ever load-bearing —
   `password`/`access_token` were already blanked to `""` before writing, and
   `verification_code` is `gorm:"-:all"`, so the `GetUserById` row feeding the
   cache can never carry a non-empty one (mechanically asserted by
   `TestMixedVersionUserCache_VerificationCodeWasNeverPersistable`).

   **Security note:** that `verification_code` gap is a second, independent
   instance of the §1.2 "default-dangerous / opt-into-safety" failure mode this
   manual exists to remove — the hand-written scrub enumerated three of the four
   secrets and missed one. `json:"-"` makes the omission unrepresentable rather
   than merely unlikely. The practical exposure was nil (the field is never
   populated on a DB-loaded row), but the defect was real and is now closed
   structurally.

### 3.4 Enforcement inversion — what replaces the type-level guarantee

The honest accounting: today, forgetting `plainX` breaks *internals* (fail
closed-ish, but silently — #353); after this change, forgetting `ToResponse()`
at a *new* handler would leak ids (fail open). That risk transfer is only
acceptable because three independent gates catch it:

| Gate | Layer | Catches |
| --- | --- | --- |
| E1 [`uuid_contract_test.go`](../../controller/uuid_contract_test.go) (exists) | runtime, per endpoint | any *existing* endpoint emitting `id`/`user_id`/secret keys — runs on every `go test ./controller/...` |
| E2 Golden byte-compat tests (new, §3.3.1) | unit, per entity | any drift between the frozen contract and the mapper output |
| E3 `noentityresponse` static analyzer (new) | compile-time types, whole repo | any `c.JSON`/`json.Marshal` argument whose type (or element/field type, transitively through `gin.H`/composite literals) is one of the five entities — **including brand-new endpoints E1 doesn't know about** |

E3 is a small `go/analysis` vet analyzer under `tools/analyzers/`, wired into
`make lint` and the lint workflow next to the existing ast-grep step. ast-grep
alone is insufficient here (it is syntactic; `gin.H{"data": users}` needs type
resolution to flag `users`). Allowlist: the `model` mappers themselves and
`model/cache.go`. The analyzer lands in P0 in *warn* mode and flips to *fail*
per entity as each migrates (a per-package allowlist shrinks to empty by P5).
An ast-grep rule with the obvious pattern (`c.JSON($_, gin.H{$$$, "data":
$VAR, $$$})` + manual triage) may be added as a cheap first-pass but is not a
gate.

New-endpoint policy (documented in `CLAUDE.md`/review checklist): handlers
return `dto.*Response` or hand-built `gin.H` of scalars — never a raw model
struct. E3 makes the policy mechanical.

### 3.5 Explicitly unchanged behaviors

- `ConsumeToken`'s `data: updatedToken` ([`token.go:357`](../../controller/token.go#L357))
  is an **API-client-facing contract** (TokenAuth callers) — byte-frozen by its
  golden test in the Token PR.
- `SetupLogin`'s response shape (consumed by password login, GitHub/Lark/OIDC/
  WeChat OAuth, passkey — 6 call sites, 1 serialization point) — byte-frozen;
  the dead `Id:` assignment in `cleanUser` is deleted *with* the flip so the
  code stops lying (`user.go:194-205` becomes `user.ToResponse()` on a reduced
  copy, or a dedicated `dto.LoginUserResponse` mirroring today's emitted keys).
- The relay/billing hot path never serializes these entities (verified §1.5) —
  zero relay risk.
- Redis cache **keys** and TTLs unchanged; payloads per §3.3.5.

---

## 4. Change list

Legend: 🆕 new file, ✏️ edit, 🗑️ delete-within-file. Anchors verified against
the working tree on 2026-07-14.

### P0 — Infrastructure (no behavior change)

| File | Change |
| --- | --- |
| 🆕 `model/testdata/*_response.golden.json` (×5) | golden capture from the **current** marshalers (generator test, §3.3.1) |
| 🆕 `tools/analyzers/noentityresponse/` | go/analysis analyzer + unit tests (warn mode) |
| ✏️ `Makefile`, `.github/workflows/lint.yml` | run the analyzer in `make lint` / CI |

### P1 — Pilot: Redemption (4 sites, admin-only, no secrets, no wrappers)

| File | Change |
| --- | --- |
| 🆕 `dto/responses.go` (+`RedemptionResponse`) | shape ≡ today's `redemptionDTO` ([`redemption.go:45-57`](../../model/redemption.go#L45)) |
| 🆕 `model/redemption_view.go` | `ToResponse()` + list mapper + golden test |
| ✏️ `controller/redemption.go` | R1 `:47`, R2 `:79`, R3 `:99`, R4 `:222` → mappers |
| 🗑️ `model/redemption.go:44-72` | remove `MarshalJSON` |

### P2 — Log (5 sites; list-heavy)

| File | Change |
| --- | --- |
| ✏️ `dto/responses.go` (+`LogResponse`) | shape ≡ `logJSON` ([`log_json.go:16-40`](../../model/log_json.go#L16)), incl. `omitempty` on `channel_name`/`metadata` |
| 🆕 `model/log_view.go` | mappers (pre-allocated list mapper) + golden test |
| ✏️ `controller/log.go` | L1 `:77`, L2 `:137`, L3 `:180`, L4 `:213`, L5 `:247` |
| 🗑️ `model/log_json.go` | remove file (marshaler only) |

### P3 — Token (9 sites; TokenAuth client contract; key-prefix logic)

| File | Change |
| --- | --- |
| ✏️ `dto/responses.go` (+`TokenResponse`) | shape ≡ `tokenDTO` ([`token.go:71-87`](../../model/token.go#L71)) |
| 🆕 `model/token_view.go` | `ToResponse()` carries the prefix normalization verbatim ([`token.go:62-69`](../../model/token.go#L62)) + golden test incl. prefix cases (port `model/token_json_test.go` assertions) |
| ✏️ `controller/token.go` | T1 `:80`, T2 `:112`, T3 `:132`, T4 `:214` (`cleanToken`), T5 `:357` (`ConsumeToken`), T6 `:986`, T7 `:1100`, T8 `:1134`, T9 `:1154` |
| 🗑️ `model/token.go:59-106` | remove `MarshalJSON` |
| ✏️ `model/cache.go:60-61` | drop the `plainToken` alias (now a no-op) — or defer to P6 |

### P4 — User (6 sites; secrets + request DTOs; login funnel)

| File | Change |
| --- | --- |
| ✏️ `dto/responses.go` (+`UserResponse`) | shape ≡ `userJSON` ([`user_json.go:9-30`](../../model/user_json.go#L9)) |
| 🆕 `dto/user_requests.go` | `UserRegisterRequest`, `UserCreateRequest`, `UserSelfUpdateRequest` (§3.2) |
| 🆕 `model/user_view.go` | mappers + golden test |
| ✏️ `model/user.go:39,48,49,50` | `Password`, `VerificationCode`, `AccessToken`, `TotpSecret` → `json:"-"` |
| ✏️ `controller/user.go` | U1 `:202` (`SetupLogin`), U2 `:329`, U3 `:350`, U4 `:373`, U5 `:778`, U6 `:1413` (`ManageUser`); rebind `:234` (`Register`), `:1268` (`CreateUser`), `:1116` (`UpdateSelf`) onto the request DTOs |
| 🗑️ `model/user_json.go` | remove file |
| ✏️ `model/cache.go:103-120` | shrink the plainUser comment/alias; scrub now redundant (keep or drop per §3.2) |
| ✏️ `model/cache_user_id_test.go` | assertions unchanged — must stay green (this is the #353 regression net) |

### P5 — Channel (4 sites; the two builders + embedding wrapper)

| File | Change |
| --- | --- |
| ✏️ `dto/responses.go` (+`ChannelResponse`) | shape ≡ `channelJSON` ([`channel_json.go:16-46`](../../model/channel_json.go#L16)) |
| 🆕 `model/channel_view.go` | mappers + golden test |
| ✏️ `controller/channel.go:198-218` | `buildChannelResponsePayload` builds from `ToResponse()` + `tooling` splice (C3 `:297`, C4 `:497` unchanged above it) |
| ✏️ `controller/channel_testing_model.go:74-126` | `channelListItem` → plain composite `{dto.ChannelResponse; TestModels []string}`; delete the byte-splicing `MarshalJSON` override (C1 `:255`, C2 `:277` via `buildChannelListResponse`) |
| 🗑️ `model/channel_json.go` | remove file |

### P6 — Cleanup & hardening

| File | Change |
| --- | --- |
| ✏️ `tools/analyzers/noentityresponse` | flip warn → fail repo-wide (allowlist now empty) |
| ✏️ `model/cache.go` | remove any remaining alias/scrub redundancy; comments updated |
| ✏️ `docs/arch/`, `CLAUDE.md` | document the boundary rule: "handlers return `dto.*Response`, never raw model structs" |
| 🗑️ golden generator's dependence on old marshalers | goldens become the frozen source (re-generation now goes through the mappers; any diff is a contract change requiring review) |

---

## 5. Phasing / rollout

| Phase | Content | Ships alone? | Revert |
| --- | --- | --- | --- |
| P0 | goldens + analyzer (warn) | Yes — zero behavior change | trivial |
| P1 | **Pilot: Redemption** — proves shape+mapper+flip+removal on the smallest, admin-only surface | Yes | single `git revert` restores the marshaler |
| P2 | Log | Yes | same |
| P3 | Token (includes the TokenAuth `ConsumeToken` client contract) | Yes | same |
| P4 | User (largest: secrets, request DTOs, login funnel ×6 providers) | Yes | same |
| P5 | Channel (builders + wrapper) | Yes | same |
| P6 | analyzer strict, cache cleanup, docs | Yes | trivial |

Pilot rationale: unlike the UUID proposal's pilot (token — chosen to prove
uuid *mechanics*), this refactor's risk is *coverage* (did we flip every
site?), so the pilot minimizes blast radius: Redemption has 4 admin-only
sites, no secrets, no wrappers, no TokenAuth clients. P3–P5 are ordered by
increasing coupling; any pause between phases leaves the tree fully
consistent (the two styles coexist per entity, never within one).

No deployment ordering constraints: no schema change, no cache-key change;
mixed old/new binaries share cache payloads safely (§3.3.5, T11).

---

## 6. Test matrix

Rows marked **(exists)** are already in the tree and act as regression nets;
the rest are new. DB guardrails from prior work apply throughout
(`SetMaxOpenConns(1)` for concurrent sqlite; `-race` on anything concurrent).

| # | Layer | Scenario | Assertion |
| --- | --- | --- | --- |
| T1 | Unit / contract | Per entity: fully-populated model → `ToResponse()` → `json.Marshal` vs the P0 golden bytes | byte-identical (`require.JSONEq` + exact key-set walk); run per PR P1–P5 |
| T2 | Unit / contract | Golden capture determinism: regenerate golden from the same fixture twice | identical bytes (no map-ordering flake — goldens compared via `JSONEq`) |
| T3 | Runtime gate **(exists)** | [`uuid_contract_test.go`](../../controller/uuid_contract_test.go): every endpoint, per entity, before & after its flip | still green at every phase; `id`/`user_id` absent; `uuid` keys present |
| T4 | Security | Post-P4: marshal a fully-populated `model.User` with **default** `json.Marshal` (no mapper) | output contains `id` (honest) and **never** `password`/`access_token`/`totp_secret`/`verification_code` (`json:"-"`) |
| T5 | Security | Post-P4: `dto.UserResponse` golden | no secret keys, no `id`/`inviter_id` (mirrors existing `TestUserMarshalJSON_StillHidesInternalIntId`, which is deleted only when its replacement lands in the same PR) |
| T6 | Cache **(exists)** | [`model/cache_user_id_test.go`](../../model/cache_user_id_test.go): miniredis round-trip preserves `Id`, scrubs secrets | green before and after P4's alias/scrub simplification |
| T7 | Inbound equivalence | P4: replay identical JSON bodies against `Register`/`CreateUser`/`UpdateSelf` pre- and post-request-DTO | resulting DB rows identical (password set & hashed, display_name/email/metadata semantics incl. omitted-vs-empty in `UpdateSelf`); unknown-field tolerance unchanged |
| T8 | Inbound regression | D3 legacy writers: `AddToken`/`UpdateToken` with `{"id":…}`/`{"user_id":…}` int bodies | ~~still parse~~ → **still *refused identically***: strict-in has rejected integer ids since `99c5ed01`, so the assertion is invariance of the refusal, not acceptance (`resource uuid is required`, same status + body pre/post). Tags untouched. Covered by the untouched [`TestTokenStrictInResponses`](../../controller/uuid_contract_test.go#L594) + the T17 differential. See the §1.3 correction. |
| T9 | Wrapper | P5: `channelListItem` replacement with 0, 1, n `test_models`, and `nil` channel | key-set identical to the old byte-splicer, incl. `"test_models":[]` (never omitted) and the nil-channel `{"test_models":[]}` shape ([`channel_testing_model.go:97`](../../controller/channel_testing_model.go#L97)) |
| T10 | Client contract | P3: `ConsumeToken` httptest — full response incl. `data` + `transaction` | byte-identical key-set to pre-flip capture |
| T11 | Mixed-version cache | Write cache entry with pre-change binary's payload shape (fixture), read with post-change code, and vice versa | both directions unmarshal correctly; `Id` preserved; no panic on missing/extra keys |
| T12 | Static gate | Analyzer unit tests: `c.JSON` with raw entity / slice / `gin.H`-wrapped / embedded; and negative cases (`dto.*Response`, scalars, mappers, cache aliases) | flags all positives, none of the negatives |
| T13 | Perf | Benchmark `LogsToResponses` on 10k rows vs old marshaler path | no regression >5% ns/op or allocs/op (pre-allocation, §3.1) |
| T14 | Regression | Full suite `go test -race ./...` at every phase | green |
| T15 | Frontend ×3 | Per migrated entity: list, create, edit, delete, search in modern/air/berry dev servers (`make dev`) | no visible change; uuid flows work; air/berry dashboard "all users" still selects after P4 (via the `UUID: "all"` sentinel — **not** via `id===0`, which is already inert; see §1.5) |
| T16 | Logging redaction | Post-P4: `zap.Any("user", user)` through the JSON encoder in a test logger | no secret keys in the encoded output (today this is protected by the marshaler; `json:"-"` must take over) |
| T17 | **Black-box differential harness** | httptest + seeded sqlite fixture (reuse the `uuid_contract_test.go` environment): drive **every** Appendix A endpoint plus its documented error cases (bad id, missing auth, malformed body, foreign owner) and record `(status, canonicalized JSON body)` into `controller/testdata/behavior/*.json` **on the commit before each phase**; re-run after the phase | zero diff, statuses and bodies both (timestamps/uuids normalized by the fixture, not by the differ); the capture set is regenerated at each phase start so drift can never hide inside a stale baseline |
| T18 | Error-path equivalence | For each flipped handler and each new request DTO (P4): invalid JSON, wrong types, oversized fields, unauthorized role, non-existent uuid, foreign-owner access | identical status code and error body pre/post (subset of T17, called out because request-DTO rebinding is where inbound error text could accidentally change) |
| T19 | User-journey E2E | Scripted journeys on the dev server: register → login (with and without TOTP) → create token → call `/v1/chat/completions` with it → view own logs → admin lists users/channels/tokens → manage user; OAuth callbacks exercised with mocked providers where CI allows, manually otherwise | every step succeeds with identical visible results pre/post phase; session cookie from a pre-upgrade login keeps working post-upgrade (no session invalidation) |
| T20 | Program-behavior (relay/billing) | High-concurrency `-race` relay stress (existing billing-race suites) + one full relay request against the dev server per phase | quota deltas, log rows, and cache contents identical; no serialization of the five entities occurs on the relay path (analyzer-verified) — this row exists to prove the *absence* of coupling, per §3.5 |

### Phase gates

| Gate | Requires green |
| --- | --- |
| G-P1 (pilot merges) | T1, T2 (redemption), T3, T12, T14, **T17+T18 scoped to redemption endpoints** |
| G-P2/P3 | entity's T1 + T3 + T14 + T17 + T18; P3 additionally T8, T10, T13, T20 |
| G-P4 | T1, T3, T4, T5, T6, T7, T11, T14, T15, T16, T17, T18, **T19 full journey set** |
| G-P5 | T1, T3, T9, T14, T15, T17, T18 |
| G-P6 (strict analyzer) | analyzer allowlist empty; T12 in fail mode; whole matrix green incl. a final full-surface T17 run |

---

## 7. Acceptance criteria

| # | Criterion |
| --- | --- |
| AC1 | `grep -rn "func (.*) MarshalJSON()" model/*.go` returns **only** the four lossless field serializers (`JSONStringSlice`, `JSONStringMap`, `MCPToolPricingMap`, `LogMetadata`) — the five entity whitelist marshalers are gone. |
| AC2 | Every endpoint in Appendix A emits a byte-identical key-set to its P0 golden (T1) and passes the existing contract gate (T3) — the external S2 contract is provably unchanged. |
| AC3 | `json.Marshal` of any of the five entities yields default, id-bearing JSON with **zero** secret keys for `User` (T4) — issue #353's bug class is unrepresentable. |
| AC4 | `model/cache.go` contains no `plainX` aliases and no secret-scrub that the type system doesn't already guarantee; [`model/cache_user_id_test.go`](../../model/cache_user_id_test.go) green (T6). |
| AC5 | The `noentityresponse` analyzer runs in CI in **fail mode** (`strict = true` by default); introducing `c.JSON(200, gin.H{"data": user})` in any package fails `make lint` (T12). ~~with an empty allowlist~~ **Corrected:** the allowlist is `{model/cache.go}`, not empty — AC5 as written contradicted §3.4, which *requires* that entry, because the internal Redis cache must marshal the raw entity (that honest, id-bearing payload is the #353 fix). An empty allowlist is therefore not a reachable end state. The analyzer additionally skips `_test.go` files, so tests may still marshal entities deliberately (the goldens and mixed-version fixtures depend on this). Verified: `make lint-entity-response` exits 0 repo-wide. |
| AC6 | ~~Legacy int-id inbound writers still work (T8) — D3 untouched.~~ **Corrected:** legacy int-id writers do **not** work and have not since `99c5ed01` (strict-in rejects them; see §1.3). The criterion that is actually meaningful, and that is met: **inbound behavior is byte-for-byte invariant** — `PUT /api/token/ {"id":<int>}` returns the same `resource uuid is required` refusal pre- and post-refactor (verified live on both binaries and by the untouched `TestTokenStrictInResponses`). All inbound tags are untouched (T8). |
| AC7 | All three frontends pass the T15 manual matrix; the air/berry dashboard sentinel works. |
| AC8 | Full `go test -race ./...` green; no new `golangci-lint`/`ast-grep` findings. |
| AC9 | The boundary rule ("handlers return `dto.*Response`/scalar `gin.H`, never raw model structs") is documented in the repo's contributor docs. |
| AC10 | **Behavioral invariance proven per phase:** each phase PR includes its T17 differential run (zero diff across every Appendix A endpoint, success *and* error cases) and its gate's journey/error rows (T18–T20 where applicable). All seven invariants I1–I7 (§2.1) hold at every merged commit — this refactor is observable only in the code, never in behavior. |

---

## 8. Risks & rollback

| Risk | Likelihood | Mitigation |
| --- | --- | --- |
| A missed handler site leaks `id` after its entity's marshaler is removed | Low — 28 sites triple-inventoried (Appendix A) | flip + removal are the **same PR**, so T3 (which exercises every endpoint) catches a miss before merge; E3 catches it statically |
| Golden files freeze a bug (goldens generated from a wrong fixture) | Low | goldens reviewed as JSON in the P0 PR; T2 determinism; fixtures use every field non-zero so omissions are visible |
| `ConsumeToken` / OAuth-login clients notice a shape change | Very low | T10/T15 byte-freeze; these paths change serializer, not shape |
| Secret leak in the window between marshaler removal and `json:"-"` | **Zero by construction** | §3.3 orders them into the same PR (P4); T4/T16 enforce |
| Future contributor returns a raw struct on a **new** endpoint | Medium over time | E3 analyzer (fail mode) + AC9 documented rule — this replaces the old ambient guarantee with a mechanical one |
| Mixed-version Redis payloads during rolling deploy | Very low | payload key-sets only grow; T11 covers both directions; no flush required |

**Rollback:** each phase is one revertible PR. Reverting an entity's PR
restores its `MarshalJSON` and handler sites atomically; goldens and the
contract gate are direction-agnostic, so a revert is re-verified by the same
tests. No data, schema, or cache migration accompanies any phase, so rollback
has no operational tail.

---

## 9. Alternatives considered

| Alternative | Verdict |
| --- | --- |
| **Status quo + `plainX` discipline** (keep type-level marshalers; require aliases internally) | Rejected. It is the design that produced #353: the safe path is opt-in and invisible in review. The token/user cache divergence proves discipline does not persist across contributors. |
| **`json:"-"` on `Id` fields** | Rejected — it would strip `Id` from the Redis object-cache payload and reintroduce issue #353 by construction (§1.3); tags cannot be direction-asymmetric (honest internally, hidden externally). *(The original rejection cited D3 legacy inbound int-id parsing; that premise proved obsolete — strict-in already rejects integer ids since `99c5ed01`. See the §1.3 correction.)* |
| **Keep marshalers, add a linter for missing `plainX`** | Rejected. Static detection of "this `json.Marshal` is internal, not API" requires intent, which is exactly what types can't express here; the DTO split makes intent structural instead. |
| **Response-filtering middleware** (strip denied keys from every outbound JSON) | Rejected. Runtime cost on every response; hides bugs instead of failing them; a denylist middleware can't know entity-specific keys (`user_id` legitimate in some payloads, forbidden in others). |
| **Code-generated DTOs/mappers** (e.g. go:generate from struct tags) | Deferred. Five hand-written mappers are small and reviewed once; codegen adds toolchain surface. Revisit if entity count grows. |

---

## 10. Verification results (2026-07-15)

### 10.1 Method — a real "before", not a frozen "now"

All P0–P6 code landed in a single working tree, so `git HEAD` (`6ebe28a5`) *is*
the pre-refactor state. Verification therefore used a **pre-refactor worktree**
(`git worktree add <dir> HEAD`) as a genuine baseline rather than capturing
current behavior and calling it frozen. Every differential below compares
**pre-refactor output against post-refactor output**; every harness was
additionally checked for **vacuity** (an injected change must make it fail),
because a differential that cannot fail proves nothing.

### 10.2 Results

| Row | Result |
| --- | --- |
| **T1/T2/AC2** | Green. **Provenance independently proven**: the five `model/testdata/*_response.golden.json` were replayed through the *legacy* marshalers in the pre-refactor worktree — all five match byte-for-byte. Since T1 proves *mapper == golden* in the main tree, this transitively proves **mapper == retired `MarshalJSON`**. Vacuity checked (mutating a fixture value fails the comparison). This closes the §8 "golden freezes a bug" risk. |
| **T3** | Green (`uuid_contract_test.go` untouched and passing). |
| **T4/T5/T6/T16** | Green. |
| **T10** | Implemented ([`controller/consume_token_contract_test.go`](../../controller/consume_token_contract_test.go), 6 subtests). Exact key-set frozen: top level `{data, message, success, transaction}`; `data` = 15 keys; `transaction` = 14 base keys + conditional `confirmed_at`/`canceled_at`/`elapsed_time_ms`. `id`/`user_id`/`token_id`/`log_id`/secrets asserted absent at any depth. **Passes unmodified on both trees.** Vacuity checked (injecting `Id` into `dto.TokenResponse` fails it). |
| **T11** | Implemented ([`model/cache_mixed_version_test.go`](../../model/cache_mixed_version_test.go), 6 tests) against fixtures generated by the **real pre-refactor binary**. Both directions green via miniredis + the real `CacheGetUserById`/`CacheGetTokenByKey`. **Found two doc errors — see §3.3.5 correction.** |
| **T13/I7** | **Measured, envelope holds with a wide margin — in the improving direction.** 10k fully-populated rows, both paths' JSON verified byte-identical first: sec/op **80.42m → 21.28m (−73.5%)**, allocs/op **80.00k → 50.00k (−37.5%)**, B/op −42.2% (benchstat n=10, p=0.000; direction reproduced at `GOMAXPROCS=1` and `16`). Cause: the retired `Log.MarshalJSON` ran a *nested* `json.Marshal` per row (8 allocs/row vs 5); the pre-allocated DTO slice costs exactly **1** allocation total. |
| **T14/AC8** | See §10.4. |
| **T15** | **Holds.** Verified across modern/air/berry: all key by `uuid`; zod/yup are *input*-only (no response schema anywhere); no theme *depends* on an integer FK. **Two §1.5 claims were factually wrong and are corrected there.** |
| **T17/T18** | Implemented ([`controller/behavior_differential_test.go`](../../controller/behavior_differential_test.go), **81 cases**, all 28 Appendix A sites + error paths). Baselines generated **on the pre-refactor tree**; the file compiles and runs on both sides. Capture determinism verified (two captures byte-identical). **78/81 zero-diff; 3 genuine behavior changes found — see §10.3.** |
| **T19** | **Green, live.** Two servers (refactored + pre-refactor) driven over real HTTP with real routing/middleware/session auth. Management journey: **54 steps** (45 returning data). Relay journey: **11 steps** including a real billed `/v1/chat/completions` against a mock upstream. Both recordings are **byte-identical (matching md5)** across the two binaries. Zero secret keys and zero integer-id key paths on either side. Differ vacuity-checked (injected status/shape changes are caught). |
| **T20** | **Green.** Relay → billing → log verified live: identical response, identical log row, identical `quota_delta=1` on both binaries. Statically, the `noentityresponse` analyzer passes repo-wide in strict mode, so no relay-path code serializes any of the five entities. |
| **AC6/T8** | **Criterion was wrong; behavior is invariant.** See the §1.3 correction — legacy int-id inbound has been rejected since `99c5ed01`, so the meaningful assertion is that the *refusal* is identical pre/post (verified live and by the untouched `TestTokenStrictInResponses`). |

### 10.3 Deviation: inbound strictness on unread fields is relaxed (I2)

The T17 differential caught the one real behavior change, in the three new
request DTOs:

| Case | Pre-refactor | Post-refactor |
| --- | --- | --- |
| `user/register_wrong_type_unbound_field` | `400 invalid parameter` | `200 success` + user created |
| `user/create_wrong_type_unbound_field` | `400 invalid parameter` | `200 success` + user created |
| `user/update_self_wrong_type_unbound_field` | `400 invalid parameter` | `200 success` + display_name changed |

**Mechanism (not a test artifact).** `Register`/`CreateUser`/`UpdateSelf` used
to decode into `model.User`, so a **type mismatch on any JSON-tagged
`model.User` field — including fields the handler never read** (`status`,
`quota` on `UserRegisterRequest`, …) — was a decode error that rejected the
whole body. The narrow DTOs omit those fields, so `encoding/json` now discards
them as unknown and the request proceeds. The old strictness was an *accident*
of decoding into the entity, not a designed check.

**Reachability: no first-party trigger.** All three themes were audited. The
condition "sends a `model.User` key absent from the DTO" *is* met (air+berry
echo the whole user object back to `PUT /api/user/self`; modern sends
`mcp_tool_blacklist` to `POST /api/user/`), but the condition "with a
mismatched **type**" is met by **no bundled client** — echoed values come from
the server's own encoding, so their types match by construction, and every
numeric input is either coerced or lives on a different endpoint. (A claim that
`web/air/src/pages/User/EditUser.js`'s `parseInt` quota coercion proved
reachability was investigated and **refuted**: that path posts to the admin
`PUT /api/user/`, which binds the pre-existing, untouched
`dto.UserAdminUpdatePayload`.) It remains reachable in principle for a
third-party caller that sends a wrong-typed value on a field these handlers
ignore.

**Resolution: accepted as a bounded, documented deviation from I2** — not
silently re-baselined. Restoring the old behavior exactly would require each
request DTO to mirror `model.User`'s **entire** pre-refactor tag set, including
the four secret fields whose tags were deliberately removed for G3 — i.e. it
would re-create the entity coupling this refactor exists to remove, and is not
expressible now that `Password`/`AccessToken`/`TotpSecret`/`VerificationCode`
are `json:"-"`. The relaxation loosens validation only on fields that are
ignored anyway; it exposes no secret, leaks no id, and changes no stored row.
The three cases are pinned to their **post-refactor** expectation in
`controller/testdata/behavior/deviations/`, with the pre-refactor bytes retained
alongside as the historical record, so the deviation stays asserted and any
*further* drift still fails.

### 10.4 Corrections this verification forced into the manual

The manual's own reasoning contained four factual errors. All are documentation
defects, not code defects — the implementation is sound — but each was
load-bearing for an argument:

1. **§1.3 / Alternatives / AC6 / T8** — "`json:"-"` on `Id` is ruled out by D3
   legacy inbound int-id parsing" rests on a premise that is false: strict-in
   has rejected integer ids since `99c5ed01`. The rejection stands on a stronger
   reason — `json:"-"` on `Id` would strip the id from the Redis cache payload
   and **reintroduce issue #353 by construction**.
2. **§3.3.5** — "cache payloads only ever gain keys" is backwards: the user
   payload **loses** `{password, access_token, verification_code}` and gains
   nothing. No flush is still needed, for a different (stronger) reason.
3. **§3.3.5 (Token)** — the predicted "(for Token) gains `id`/`user_id`" never
   applied; `plainToken` already emitted both.
4. **§1.5** — `GetDashboardUsers`' `UserOption` does **not** emit `id`, so the
   air/berry `id === 0` sentinel is already inert (it survives on the
   `UUID: "all"` fallback); and the air log-row key collapse is a real
   (pre-existing) defect, not merely cosmetic.

A fifth, security-relevant observation: the pre-refactor cache scrub enumerated
**three of four** secrets and missed `VerificationCode` — a second, independent
instance of the §1.2 "default-dangerous / opt-into-safety" failure mode, now
closed structurally by `json:"-"`. Practical exposure was nil (`gorm:"-:all"`
means the cached row never carries one), but the defect was real.

**Filed separately (pre-existing, out of scope):** the air Semi `<Table>` log
rowKey collapse (`web/air/src/components/LogsTable.js:308` sets every row's key
to the string `"undefined"`), and the inert air/berry dashboard `id === 0`
sentinel. Both are inherited from the 20260703 UUID proposal.

### 10.5 Acceptance criteria — final status

| # | Status |
| --- | --- |
| AC1 | **Met.** `grep` over `model/*.go` returns only the four lossless field serializers (`LogMetadata`, `JSONStringSlice`, `JSONStringMap`, `MCPToolPricingMap`). |
| AC2 | **Met, and provenance-proven** (§10.2 T1 row). |
| AC3 | **Met** (`TestUserDefaultMarshalIsHonestAndSecretFree`). |
| AC4 | **Met.** No `plainX` alias and no scrub remain in `model/cache.go` (the only surviving mention is the comment explaining why they are gone); `cache_user_id_test.go` green. |
| AC5 | **Met as corrected** — fail mode, allowlist `{model/cache.go}`; "empty allowlist" was never reachable (see the AC5 note). |
| AC6 | **Criterion corrected** — the refusal is invariant; legacy int-id acceptance has not existed since `99c5ed01` (§1.3). |
| AC7 | **Met** (T15, all three themes, code-level audit). |
| AC8 | **Met.** `go test -race ./...` green across 89 packages; `go vet` clean; `make lint-entity-response` exits 0; the `no-gin-context-as-spawn-arg` ast-grep guardrail passes. |
| AC9 | **Met.** The boundary rule is documented in [`docs/arch/boundary_response_dtos.md`](../arch/boundary_response_dtos.md), which is where the rule, its rationale, the four enforcement gates and the "adding a new endpoint" checklist live. |
| AC10 | **Met with one recorded deviation** (§10.3). I1, I4, I5, I6, I7 hold unconditionally and are proven by the differentials above; **I2/I3 hold except for the single accepted inbound-strictness relaxation**, which is pinned, documented, and unreachable from any bundled client. |

### 10.6 Reproducing the verification

```sh
# 1. Pre-refactor baseline tree (all refactor work is uncommitted, so HEAD == "before")
git worktree add /tmp/head-tree HEAD

# 2. Entity contract + endpoint differential + cache + perf
go test -race ./... -count=1
go test ./controller/ -run TestBehaviorDifferential -count=1     # 81 cases vs pre-refactor bytes
go test ./controller/ -run ConsumeToken -count=1                 # exact key-set freeze
go test -race ./model/ -run MixedVersion -count=1                # old<->new cache payloads

# 3. Boundary gate
make lint-entity-response                                        # strict; must exit 0
```

The live T19/T20 differential is committed as a reusable tool at
[`scripts/behavior-differential/`](../../scripts/behavior-differential/) (see its
README). It needs two running binaries, so it is a **manual** gate, not a CI one:

```sh
git worktree add /tmp/before <before-commit>
# start both builds on :3000 (candidate) and :3001 (baseline), separate empty DBs
cd scripts/behavior-differential
python3 mock_upstream.py 3002 &
python3 journey.py       http://127.0.0.1:3001 /tmp/before.json
python3 journey.py       http://127.0.0.1:3000 /tmp/after.json
python3 compare.py       /tmp/before.json /tmp/after.json      # exits non-zero on any diff
python3 relay_journey.py http://127.0.0.1:3001 http://127.0.0.1:3002 /tmp/relay_before.json
python3 relay_journey.py http://127.0.0.1:3000 http://127.0.0.1:3002 /tmp/relay_after.json
python3 compare.py       /tmp/relay_before.json /tmp/relay_after.json
python3 compare.py --self-test /tmp/after.json                 # the differ must be able to fail
```

For this refactor it produced byte-identical (md5-matching) recordings across the
two binaries: 54 management steps and 11 relay steps, `quota_delta=1` on both,
zero secret keys and zero integer-id key paths on either side.

Nothing about the tool is specific to this change — it compares any two builds —
so it is worth reaching for the next time a refactor claims behavior invariance.

## Appendix A — Verified handler-site inventory (the 28 flips)

Auth column from [`router/api.go`](../../router/api.go).

### User (6)

| # | Site | Handler | Shape | Auth |
| --- | --- | --- | --- | --- |
| U1 | [`controller/user.go:202`](../../controller/user.go#L202) | `SetupLogin` (`cleanUser`) — funnel for `Login`, WeChat/Lark/OIDC/GitHub OAuth, `PasskeyLoginFinish` | single | public→self |
| U2 | [`controller/user.go:329`](../../controller/user.go#L329) | `GetAllUsers` | list | admin |
| U3 | [`controller/user.go:350`](../../controller/user.go#L350) | `SearchUsers` | list | admin |
| U4 | [`controller/user.go:373`](../../controller/user.go#L373) | `GetUser` | single | admin |
| U5 | [`controller/user.go:778`](../../controller/user.go#L778) | `GetSelf` | single | self |
| U6 | [`controller/user.go:1413`](../../controller/user.go#L1413) | `ManageUser` (`clearUser`) | single | admin |

### Token (9)

| # | Site | Handler | Shape | Auth |
| --- | --- | --- | --- | --- |
| T1 | [`controller/token.go:80`](../../controller/token.go#L80) | `GetAllTokens` | list | self |
| T2 | [`controller/token.go:112`](../../controller/token.go#L112) | `SearchTokens` | list | self |
| T3 | [`controller/token.go:132`](../../controller/token.go#L132) | `GetToken` | single | self |
| T4 | [`controller/token.go:214`](../../controller/token.go#L214) | `AddToken` (`cleanToken`) | single | self |
| T5 | [`controller/token.go:357`](../../controller/token.go#L357) | `ConsumeToken` (`data: updatedToken`) | single | **TokenAuth (API clients)** |
| T6 | [`controller/token.go:986`](../../controller/token.go#L986) | `UpdateToken` (`cleanToken`) | single | self |
| T7 | [`controller/token.go:1100`](../../controller/token.go#L1100) | `AdminGetAllTokens` | list | admin |
| T8 | [`controller/token.go:1134`](../../controller/token.go#L1134) | `AdminSearchTokens` | list | admin |
| T9 | [`controller/token.go:1154`](../../controller/token.go#L1154) | `AdminGetToken` | single | admin |

### Channel (4, via shared builders)

| # | Site | Handler | Shape | Auth |
| --- | --- | --- | --- | --- |
| C1 | [`controller/channel.go:255`](../../controller/channel.go#L255) | `GetAllChannels` → `buildChannelListResponse` → `channelListItem` | list | admin |
| C2 | [`controller/channel.go:277`](../../controller/channel.go#L277) | `SearchChannels` → same | list | admin |
| C3 | [`controller/channel.go:297`](../../controller/channel.go#L297) | `GetChannel` → `buildChannelResponsePayload` | single | admin |
| C4 | [`controller/channel.go:497`](../../controller/channel.go#L497) | `UpdateChannel` → same | single | admin |

Shared mechanisms: [`buildChannelResponsePayload` `channel.go:198`](../../controller/channel.go#L198)
(`json.Marshal(channel)` at `:200`), [`buildChannelListResponse`
`channel_testing_model.go:126`](../../controller/channel_testing_model.go#L126),
[`channelListItem` + override `channel_testing_model.go:74,86`](../../controller/channel_testing_model.go#L74).
Inbound-only wrapper (no outbound change): `channelPayload`
[`channel.go:27`](../../controller/channel.go#L27).

### Redemption (4)

| # | Site | Handler | Shape | Auth |
| --- | --- | --- | --- | --- |
| R1 | [`controller/redemption.go:47`](../../controller/redemption.go#L47) | `GetAllRedemptions` | list | admin |
| R2 | [`controller/redemption.go:79`](../../controller/redemption.go#L79) | `SearchRedemptions` | list | admin |
| R3 | [`controller/redemption.go:99`](../../controller/redemption.go#L99) | `GetRedemption` | single | admin |
| R4 | [`controller/redemption.go:222`](../../controller/redemption.go#L222) | `UpdateRedemption` (`cleanRedemption`) | single | admin |

### Log (5)

| # | Site | Handler | Shape | Auth |
| --- | --- | --- | --- | --- |
| L1 | [`controller/log.go:77`](../../controller/log.go#L77) | `GetAllLogs` | list | admin |
| L2 | [`controller/log.go:137`](../../controller/log.go#L137) | `GetUserLogs` | list | self |
| L3 | [`controller/log.go:180`](../../controller/log.go#L180) | `GetTokenLogs` | list | TokenAuth |
| L4 | [`controller/log.go:213`](../../controller/log.go#L213) | `SearchAllLogs` | list | admin |
| L5 | [`controller/log.go:247`](../../controller/log.go#L247) | `SearchUserLogs` | list | self |

**Verified out of scope (already boundary-style, no change):**
`GetSelfByToken` (`user.go:701` hand-built `gin.H`), `GetDashboardUsers`
(`UserOption` DTO — carries the air/berry `id===0` sentinel), `DuplicateChannel`
(`gin.H{uuid,name}`), `GetTokenStatus`/`GetTokenBalance`, tracing handlers,
pricing/debug DTOs, `AddRedemption` (returns key strings). **No SSE/websocket/
export path serializes these entities.**

## Appendix B — Frozen response shapes (source of truth for the `dto` structs)

The five `dto.XResponse` structs replicate these existing whitelists
field-for-field; the golden files (P0) freeze their serialized form:

| Entity | Current shape | Anchor |
| --- | --- | --- |
| User | `userJSON` (20 fields, `uuid`…`updated_at`; no `id`, no `inviter_id`, no secrets) | [`model/user_json.go:9-30`](../../model/user_json.go#L9) |
| Token | `tokenDTO` (16 fields; `key` = prefix-normalized; no `id`, no `user_id`) | [`model/token.go:71-87`](../../model/token.go#L71) |
| Channel | `channelJSON` (28 fields; no `id`) | [`model/channel_json.go:16-46`](../../model/channel_json.go#L16) |
| Redemption | `redemptionDTO` (11 fields; no `id`, no `user_id`) | [`model/redemption.go:45-57`](../../model/redemption.go#L45) |
| Log | `logJSON` (22 fields; `channel_name`/`metadata` `omitempty`; no `id`, no int FKs) | [`model/log_json.go:16-40`](../../model/log_json.go#L16) |
