# Autopsy: Duplicate DB Operation Handling

- Status: Fixed
- Date: 2026-07-12
- Area: controller error handling / uniqueness constraints / behavior tests
- Audience: junior backend engineers
- Related files: `controller/user.go`, `controller/mcp_server.go`, `controller/duplicate_db_error.go`, `controller/auth/*`, `controller/*_test.go`

## 1. Summary

A user-triggerable duplicate database operation could return raw database unique
constraint details to API clients. The first visible case was duplicate username
registration, but a systematic audit found the same bug class in other public or
admin-facing flows:

- Admin-created users with an existing username.
- Admin username updates that rename a user to an existing username.
- Self-service username updates that collide with another user.
- OAuth auto-provisioning when a provider-derived username collides.
- MCP server create/update when the requested server name already exists.

The database behaved correctly: it rejected duplicate rows. The bug was in the
application boundary: expected business conflicts were treated as generic
database failures and passed through `helper.RespondError`, which serializes
`err.Error()` into the response envelope.

## 2. Impact

The impact was mostly correctness and operability, not data corruption.

- Users saw implementation details such as `UNIQUE constraint failed` or driver
  duplicate-key text instead of a stable product message.
- Expected duplicate requests could be logged like server errors, adding noise.
- API behavior was inconsistent: registration had a targeted duplicate-username
  response, while sibling create/update paths leaked raw DB errors.
- Tests did not cover duplicate behavior for admin, self-update, OAuth, or MCP
  server flows, so regressions were easy to miss.

## 3. How To Reproduce

The simplest reproduction before the fix:

1. Create a user named `alice`.
2. Send a second user-create or username-update request that also uses `alice`.
3. Observe that the database rejects the operation.
4. Observe that the API response contains database-driver text instead of
   `Username already exists`.

The equivalent MCP reproduction:

1. Create an MCP server named `prod-tools`.
2. Create another MCP server with the same name, or update another server to
   that name.
3. Observe a raw unique-constraint failure instead of
   `MCP server name already exists`.

Good reproduction tests assert both sides:

- The response is the stable public message.
- The response body does not contain driver text such as `unique constraint` or
  `duplicate key`.

## 4. Root Cause

### 4.1 Immediate Cause

Several controller paths called model create/update functions and returned any
error directly:

```go
if err := cleanUser.Insert(ctx, 0); err != nil {
    helper.RespondError(c, err)
    return
}
```

That is fine for truly unexpected failures, but a duplicate username or duplicate
MCP server name is an expected business conflict. It must be mapped at the API
boundary.

### 4.2 Why Registration Looked Fixed But The Class Was Not

The initial registration endpoint had targeted duplicate handling, but it was a
local fix. The same uniqueness invariant existed in multiple sibling flows:

- `users.username` is unique.
- `mcp_servers.name` is unique.

Only one path had the explicit public response. Other paths shared the same
database invariant but not the same error policy.

That is the classic smell: a bug is fixed at the symptom site, not at the bug
class boundary.

### 4.3 Why Pre-checks Alone Are Not Enough

It is useful to check before writing:

```go
if model.IsUsernameAlreadyTaken(username) {
    respondUsernameAlreadyExists(c)
    return
}
```

But this is not sufficient. Two requests can pass the pre-check concurrently,
then one wins the insert/update and the other hits the unique index. The database
constraint remains the source of truth.

Correct handling needs both:

- Pre-check for ordinary duplicate requests and cleaner control flow.
- Late unique-error mapping for races.

## 5. Fix

The fix has three parts.

### 5.1 Shared Duplicate Error Classification

`controller/duplicate_db_error.go` now contains a controller-level classifier
for wrapped database unique-constraint errors. It recognizes common duplicate
signals and checks that the error references the expected field or index.

Public username helpers were added so packages like `controller/auth` can reuse
the same policy instead of copying database-driver string checks.

### 5.2 Public Duplicate Responses At Every User-facing Path

The following flows now return stable public messages:

- Username duplicate: `Username already exists`.
- MCP server name duplicate: `MCP server name already exists`.

For MCP server create/update, expected duplicate-name failures are handled before
the generic error log, so routine conflicts no longer pollute error logs.

### 5.3 Behavior Tests

New behavior tests cover the user-triggerable paths:

- `TestCreateUserDuplicateUsernameReturnsPublicFailure`
- `TestUpdateUserDuplicateUsernameReturnsPublicFailure`
- `TestUpdateSelfDuplicateUsernameReturnsPublicFailure`
- `TestCreateMCPServerDuplicateNameReturnsPublicFailure`
- `TestUpdateMCPServerDuplicateNameReturnsPublicFailure`

These tests reproduce the behavior through HTTP handlers and the real SQLite
unique indexes. That matters: this bug lives at the integration boundary between
controller, model, and database.

## 6. Systematic Audit Findings

The audit started from unique constraints, then traced every create/update path
that could hit them. The high-risk, user-triggerable issues were fixed in this
change:

| Unique value | User-controlled? | Fixed behavior |
| --- | --- | --- |
| `users.username` | Yes | Public duplicate response in register, admin create/update, self update, and OAuth auto-provisioning. |
| `mcp_servers.name` | Yes | Public duplicate response in create/update, with pre-check plus late race mapping. |

Other unique indexes exist but are lower-risk follow-up work because values are
usually generated or internal:

| Unique value | Notes |
| --- | --- |
| `users.access_token` | Generated token; collision is unlikely, but retry policy would be cleaner. |
| `users.aff_code` | Short generated invite code; collision risk is higher than UUID-like values and deserves retry hardening. |
| `tokens.key` | Generated key; collision should be retried rather than leaked. |
| `redemptions.key` | Generated batch key; retry/transaction behavior should be reviewed. |
| `passkey_credentials.credential_id` | WebAuthn-controlled value; low-frequency conflict path should be mapped. |
| `traces.trace_id` | Intended duplicate no-op, but dialect-specific duplicate matching should be verified. |
| `token_transactions(transaction_id, token_id)` | Some paths accept request-linked IDs; should be reviewed for idempotency semantics. |

The important lesson is not that every unique index has the same severity. The
important lesson is to inventory the whole class first, then prioritize by
whether normal users can trigger it and whether the value is generated or
client-controlled.

## 7. Lessons Learned

### 7.1 A Unique Index Is A Guardrail, Not A UX Policy

The database must enforce uniqueness. The API must still translate expected
constraint failures into stable product behavior.

Do not expose database-driver text to clients. It is noisy for users, brittle for
frontend logic, and often reveals schema details.

### 7.2 Fix The Bug Class, Not Only The First Endpoint

When one endpoint fails on `users.username`, search for every path that writes
`users.username`. In this incident, registration was only one of several write
surfaces.

A good audit question:

> What other controller can write the same unique column?

### 7.3 Pre-check And Race Handling Are A Pair

Pre-checks improve normal behavior, but they do not close races. The write path
must still catch the unique-index failure and map it to the same public response.

If a test only covers the pre-check branch, it misses the harder bug.

### 7.4 Expected Conflicts Should Not Be Logged As Server Errors

Duplicate username and duplicate MCP server name are expected conflicts. They
should return a public failure response, not emit generic `ERROR` logs.

Unexpected database failures still belong in the generic error path.

### 7.5 Behavior Tests Beat String-only Unit Tests Here

The regression tests go through HTTP handlers and a real database schema. That
proves:

- The route returns the expected public envelope.
- The database still prevents duplicates.
- Raw driver messages do not leak.
- Existing rows are not accidentally overwritten.

Mocking the model call would not prove the unique index behavior.

## 8. Review Checklist For Similar Bugs

Use this checklist when touching a create/update endpoint:

1. Does this write any unique column or composite unique index?
2. Is the unique value user-controlled, provider-controlled, or generated?
3. Is there a normal user flow that can trigger a duplicate?
4. Does the endpoint return a stable product message for the duplicate?
5. Does it also handle the late race where the pre-check passed but the DB write failed?
6. Does the response avoid raw SQL, table names, index names, and driver text?
7. Are expected conflicts kept out of error logs?
8. Is there a behavior test that fails before the fix and passes after it?

## 9. Verification

The fix was validated with:

```bash
go test ./controller ./controller/auth ./common ./common/helper ./middleware ./model -count=1
go vet ./...
go test -race ./...
make build-frontend-modern
```

All checks passed.
