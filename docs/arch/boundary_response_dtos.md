# Boundary Response DTOs

This document describes the serialization boundary rule for the management API.
It is the shipped form of
[`docs/proposals/20260714_boundary-response-dtos.md`](../proposals/20260714_boundary-response-dtos.md).

## The rule

Management-API handlers **must not serialize raw model entities**. For the five
entities `model.User`, `model.Token`, `model.Channel`, `model.Redemption`, and
`model.Log`, a handler returns either:

- a `dto.*Response`, built via the entity's boundary mapper
  (`entity.ToResponse()` for a single row, `model.XsToResponses(rows)` for a
  slice), or
- a hand-built `gin.H` of scalar values.

Never `c.JSON(200, gin.H{"data": user})` with a raw `*model.User` (or slice, or
`gin.H`-nested entity). The same applies to `json.Marshal` of these entities
anywhere except the allowlisted internal cache.

## Why

The external "strict-out" contract — external UUID identifiers only, no internal
integer ids, no secrets — used to be enforced by a value-receiver `MarshalJSON`
on each entity. That method participates in **every** `json.Marshal` of the
type: HTTP responses, Redis caches, log encoders, and any future queue/blob
serialization. It could not tell an external response from an internal cache
write, so the *external* contract silently governed *internal* serialization
too. That ambiguity produced production incident
[#353](https://github.com/Laisky/one-api/issues/353): the user object cache
serialized through the API marshaler, dropped the internal integer `Id`, and a
later cache hit reconstructed `Id == 0`, surfacing as a `500 "user id is empty"`.

Moving the contract to explicit boundary DTOs makes internal serialization
**honest and safe by default**:

- `json.Marshal(user)` now emits the integer `id` (what the cache needs), so the
  `plainUser`/`plainToken` alias workarounds are gone.
- Secret fields on `model.User` (`Password`, `AccessToken`, `TotpSecret`,
  `VerificationCode`) are `json:"-"`, so no serialization path — response, cache,
  log, or future — can emit them, even by accident.
- The three handlers that used to bind inbound JSON into `model.User` and read
  its secrets (`Register`, `CreateUser`, `UpdateSelf`) bind into request DTOs
  (`dto.UserRegisterRequest` / `UserCreateRequest` / `UserSelfUpdateRequest`),
  which keep `json:"password"` etc. so every field these handlers actually read
  parses exactly as before.

  Two precise notes, because both are easy to get wrong:

  - **Inbound int-id refs**: `Token.Id`/`UserId` keep their
    `json:"id"`/`json:"user_id"` tags, but those values are *decoded and
    discarded* — `preferUUIDRef` ([`controller/id_refs.go`](../../controller/id_refs.go))
    returns `resource uuid is required` without a UUID, and
    [`idresolve.Resolve`](../../common/idresolve/idresolve.go) rejects digit-only
    refs. Legacy integer-id writers have **not** worked since commit `99c5ed01`
    (the UUID proposal), and this refactor did not change that in either
    direction.
  - **One accepted inbound deviation**: the old handlers decoded into
    `model.User`, so a *wrong-typed* value on any of its JSON fields — even one
    the handler never read — rejected the whole body with `400 invalid
    parameter`. The narrow DTOs discard such fields as unknown, so those requests
    now succeed. This is deliberate and documented (§10.3 of the proposal); it is
    pinned by `controller/testdata/behavior/deviations/`. No bundled frontend can
    trigger it.

## Where things live

- Response shapes: [`dto/responses.go`](../../dto/responses.go) (leaf package,
  imports nothing from this repo).
- Request DTOs: [`dto/user_requests.go`](../../dto/user_requests.go).
- Mappers: `model/<entity>_view.go` (`ToResponse()` + list mapper). `model`
  imports `dto`, so the mappers live here.
- Frozen contract goldens: `model/testdata/*_response.golden.json`, checked by
  `TestResponseGoldens` in
  [`model/responses_golden_test.go`](../../model/responses_golden_test.go).

## Enforcement

Four independent gates keep a new handler from leaking an id or secret:

1. **Runtime, per endpoint** —
   [`controller/uuid_contract_test.go`](../../controller/uuid_contract_test.go)
   drives every endpoint and asserts `uuid` keys are present and `id`/`user_id`
   keys are absent.
2. **Unit, per entity** — the golden byte-compat tests (`TestResponseGoldens`)
   fail on any drift between the frozen contract and the mapper output.
3. **Compile-time, whole repo** — the `noentityresponse` go/analysis analyzer
   ([`tools/analyzers/noentityresponse`](../../tools/analyzers/noentityresponse))
   flags any `c.JSON`/`json.Marshal` argument whose type (transitively through
   `gin.H` / slices) is one of the five entities, including brand-new endpoints
   the runtime gate does not yet know about. It runs in `make lint`
   (`lint-entity-response`) and CI. Allowlist: `model/cache.go` (the one
   intentional internal whole-entity round-trip). `_test.go` files are skipped,
   so tests may marshal entities deliberately — the goldens and the
   mixed-version cache fixtures rely on that.
4. **Black-box differential, whole surface** —
   [`controller/behavior_differential_test.go`](../../controller/behavior_differential_test.go)
   replays 81 recorded `(status, canonical body)` captures — every Appendix A
   endpoint plus its error cases — against baselines taken from the
   **pre-refactor** binary. It is what proved this refactor observable-behavior
   neutral, and it now catches any future drift on those endpoints. Accepted
   differences live in `behaviorDiffKnownDeviations` + `testdata/behavior/deviations/`,
   never by re-baselining (regenerating baselines on the current tree would make
   the harness tautological — see the file header).

## Adding a new management endpoint

Return a `dto.*Response` mapper or a scalar `gin.H`. If you genuinely need a new
response field, add it to the DTO and re-generate the golden
(`go test ./model -run TestResponseGoldens -update-golden`) as a reviewed,
deliberate contract change — never by returning the raw entity.
