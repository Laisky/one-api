# Auth reference

one-api has three auth mechanisms; this skill always uses **access token auth** for programmatic admin work.

## Mechanisms at a glance

| Use case                         | Mechanism                   | Header / cookie                          |
|----------------------------------|-----------------------------|------------------------------------------|
| Web UI interactions              | Session cookie (gin-session) | Browser-managed                          |
| **Admin/root API calls (this skill)** | **Access token**         | **`Authorization: <uuid>`**              |
| User-issued API keys (serving LLM traffic) | API token           | `Authorization: Bearer sk-...`           |

Access token auth is the only mechanism that works outside a browser and exposes the full admin surface.

## Role levels

Defined in [model/user.go](../../../../model/user.go):

| Role | Value | Capability                                                        |
|------|-------|-------------------------------------------------------------------|
| Common user | 1 | Self-service: own profile, own tokens, own logs          |
| Admin       | 10 | Everything under `middleware.AdminAuth()` — channels, users, logs, redemptions, MCP servers, groups, debug routes |
| Root        | 100 | Admin + `middleware.RootAuth()` — `/api/option/` (system-wide settings, ratios, feature toggles) |

Role promotion/demotion lives at `POST /api/user/manage` with `action=promote` / `action=demote`. Only root can promote common→admin.

## Minting an access token

Two ways:

### Via UI (recommended for humans)
1. Log in at `<BASE_URL>/panel` as an admin or root.
2. Open **Profile** → **Generate System Access Token**.
3. Copy the UUID shown. It replaces the previous token if one existed (old token is invalidated).

### Via API (if already authenticated in a session)
```bash
curl -fsS --cookie-jar /tmp/oneapi.cookies \
  -H "Content-Type: application/json" \
  -d '{"username":"<admin-user>","password":"<password>"}' \
  "$ONEAPI_BASE_URL/api/user/login" > /dev/null

curl -fsS --cookie /tmp/oneapi.cookies \
  "$ONEAPI_BASE_URL/api/user/token" \
  | jq -r '.data'
```
Handler: [controller/user.go:619](../../../../controller/user.go#L619) `GenerateAccessToken`. The returned UUID is stored in `users.access_token` (unique-indexed). **Generating again rotates the token** — do this intentionally if you suspect leakage.

## Header format

```
Authorization: 01234567-89ab-cdef-0123-456789abcdef
```

- No `Bearer ` prefix is required, but the server strips one if present (see [model/user.go:410](../../../../model/user.go#L410) `ValidateAccessToken`).
- Do **not** URL-encode or quote the value.
- Stay consistent: this skill's scripts and examples use the raw UUID form.

## Verifying a token

```bash
curl -sS -o /dev/null -w "%{http_code}\n" \
  -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/user/self"
```
Expected: `200`. `401` means the token is invalid, expired (manually rotated), or the user was disabled/banned.

Confirm role level:
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/user/self" \
  | jq '{id, username, role, status}'
```
- `role >= 10` → admin-capable
- `role == 100` → root-capable (required for `/api/option/*`)

## Rotating an access token

Regenerating via `GET /api/user/token` invalidates the old UUID immediately. Steps for a safe rotation:

1. Notify anyone sharing the token (if it's a shared ops credential — **don't share**; issue per-admin tokens).
2. Hit `GET /api/user/token` to mint the new one. The old becomes invalid on commit.
3. Update `ONEAPI_ADMIN_TOKEN` in any `.env` / secret manager.
4. Verify with the check from "Verifying a token" above.

## Auth failure matrix

| HTTP / envelope                                   | Cause                                        | Fix                                                        |
|---------------------------------------------------|----------------------------------------------|------------------------------------------------------------|
| `401` with `message: "not logged in and no access token provided"` | `Authorization` header missing / empty | Set the header; check your shell exported `ONEAPI_ADMIN_TOKEN` |
| `401` with `message: "access token is invalid"` | Token was rotated, user deleted, or token mistyped | Re-mint via UI or `GET /api/user/token`                      |
| `403` with `message: "User has been banned"`    | User row `status=2` (disabled)               | Re-enable via `POST /api/user/manage` `action=enable`         |
| `403` from `/api/option/*`                      | Token belongs to admin (role=10), not root   | Use a root token or have root run the call                  |
| `200` + `{success:false, message:"..."}`        | Request reached handler but validation failed | Read `.message`; see [errors.md](errors.md)                  |

## Security notes

- **Do not check `ONEAPI_ADMIN_TOKEN` into git.** Use a password manager or `.envrc` with `direnv allow`.
- **One token per admin human.** Sharing tokens destroys audit trail (`CreatedBy` fields trace the token owner's id).
- **Rotate after any suspected leak.** Regeneration takes seconds; the blast radius of a leaked admin token is the entire one-api instance including other users' configured upstream credentials.
- **Prefer least privilege.** If a job only needs to read logs, give the operator a common-user token and use `/api/log/self/*` — don't escalate to admin.
- **Never log the token.** `scripts/oneapi` masks it in error traces; custom scripts should do the same (`${ONEAPI_ADMIN_TOKEN:0:4}...` if you must display a prefix).
