# Users reference

All under `AdminAuth` at `/api/user`. Source: [controller/user.go](../../../../controller/user.go), routes [router/api.go:75](../../../../router/api.go#L75).

## User object

Schema: [model/user.go](../../../../model/user.go).

| Field              | Type     | Notes                                                            |
|--------------------|----------|------------------------------------------------------------------|
| `id`               | int      | Auto                                                             |
| `username`         | string   | Unique, 3-30 chars                                               |
| `display_name`     | string   | ≤ 20 chars; auto-set to `username` on create if empty            |
| `email`            | string   | Optional, validated                                              |
| `role`             | int      | `1=common`, `10=admin`, `100=root` ([model/user.go:22](../../../../model/user.go#L22)) |
| `status`           | int      | `1=enabled`, `2=disabled`, `3=deleted`                           |
| `quota`            | int64    | Total quota granted (units — not USD)                            |
| `used_quota`       | int64    | Running total consumed                                           |
| `request_count`    | int      | Running total of requests                                        |
| `group`            | string   | Single billing group (e.g. `"default"`, `"vip"`)                 |
| `inviter_id`       | int      | Refer/affiliate tree                                             |
| `aff_code`         | string   | User's own affiliate code                                         |
| `github_id`/`wechat_id`/`lark_id`/`oidc_id` | string | OAuth bindings                            |
| `mcp_tool_blacklist` | []string | Per-user MCP tool deny-list                                   |
| `totp_secret`      | string   | Hidden in `GET`; non-empty means TOTP enabled                   |
| `created_at`/`updated_at` | int64 | Milliseconds                                                |

**Role escalation rule:** admins cannot create or manage users with `role >= own role`. Only root can promote common→admin.

## Endpoint index

| Method | Path                           | Purpose                                    |
|--------|--------------------------------|--------------------------------------------|
| GET    | `/api/user/`                   | Paginated list                             |
| GET    | `/api/user/search`             | Keyword search (username / email / display_name / id) |
| GET    | `/api/user/:id`                | Single user                                |
| POST   | `/api/user/`                   | Create                                     |
| PUT    | `/api/user/`                   | Update (id in body)                        |
| DELETE | `/api/user/:id`                | Hard-delete (soft-deletes: sets status=3)  |
| POST   | `/api/user/manage`             | Bulk actions by **username**: disable / enable / delete / promote / demote |
| POST   | `/api/user/totp/disable/:id`   | Force-disable target user's TOTP           |
| POST   | `/api/topup`                   | Add quota to a user (admin-top-up)         |

## List users

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/user/?p=0&size=50&sort=id&order=desc" \
  | jq '{total, items: (.data | map({id, username, role, status, quota, used_quota, group}))}'
```

Sort keys: `id`, `username`, `role`, `status`, `quota`, `used_quota`, `request_count`, `created_time`.

Soft-deleted users (`status=3`) are filtered out by the list query ([controller/user.go:82](../../../../controller/user.go#L82)).

## Search

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/user/search?keyword=alice" \
  | jq '.data[] | {id, username, email, role, status}'
```
Matches username, email, display_name, or numeric id.

## Create a user

```bash
jq -nc --arg user "alice" --arg pw "$INITIAL_PW" '{
  username: $user,
  password: $pw,
  display_name: "Alice Example",
  email: "alice@example.com",
  quota: 500000,
  group: "default"
}' | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X POST -d @- "$ONEAPI_BASE_URL/api/user/"
# Response: {success, message}  — the new user id is NOT returned
```

The response does not include the new id. Fetch it:
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/user/search?keyword=alice" \
  | jq '.data[] | select(.username == "alice") | .id'
```

**Validation rules:**
- `username`: 3-30 chars, unique.
- `password`: 8-20 chars (not returned in any response).
- `display_name`: ≤ 20 chars; empty → copies username.
- `role` in request body is capped — caller cannot create anyone with `role >= own role`.

## Update a user

PUT with `id` mandatory. Most fields are skipped when omitted (GORM `Updates` ignores nil/zero). **Exception: `display_name`** — sending `"display_name": ""` explicitly clears the stored display name. Omit the key to keep the current value.
```bash
# Change group
jq -nc '{id: 17, group: "vip"}' \
  | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/user/"

# Bump quota (absolute, NOT delta — you overwrite)
jq -nc '{id: 17, quota: 2000000}' \
  | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/user/"
```

**For additive quota grants, use `/api/topup` instead** — it writes an audit record:
```bash
jq -nc '{user_id: 17, quota: 1000000, remark: "ticket #4823"}' \
  | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X POST -d @- "$ONEAPI_BASE_URL/api/topup"
```

## Disable / enable / delete / promote / demote

`POST /api/user/manage` — **username-keyed**, not id:
```bash
# Disable
jq -nc --arg u alice '{username: $u, action: "disable"}' \
  | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X POST -d @- "$ONEAPI_BASE_URL/api/user/manage"

# Enable
jq -nc --arg u alice '{username: $u, action: "enable"}' \
  | curl -fsS ... -X POST -d @- "$ONEAPI_BASE_URL/api/user/manage"

# Promote to admin (ROOT ONLY)
jq -nc --arg u bob '{username: $u, action: "promote"}' \
  | curl -fsS ... -X POST -d @- "$ONEAPI_BASE_URL/api/user/manage"

# Demote from admin
jq -nc --arg u bob '{username: $u, action: "demote"}' \
  | curl -fsS ... -X POST -d @- "$ONEAPI_BASE_URL/api/user/manage"

# Soft-delete (sets status=3)
jq -nc --arg u alice '{username: $u, action: "delete"}' \
  | curl -fsS ... -X POST -d @- "$ONEAPI_BASE_URL/api/user/manage"
```

Rules enforced by [controller/user.go:1325](../../../../controller/user.go#L1325):
- You cannot manage anyone with role ≥ your role (except root managing root).
- You cannot disable or delete the root user.
- Only root can `promote`.

## Hard delete

`DELETE /api/user/:id` — equivalent to `manage action=delete`, but by id. Still soft-delete (`status=3`).
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  -X DELETE "$ONEAPI_BASE_URL/api/user/17"
```

## Disable a user's TOTP

If a user is locked out of MFA:
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  -X POST "$ONEAPI_BASE_URL/api/user/totp/disable/17"
```
Clears `totp_secret`. The user must re-enroll at next login.

## Quota mechanics

- `quota` is an int64 in internal units. `1 USD ≈ QuotaPerUnit` (typically 500000). Fetch: `curl .../api/option/ | jq '.data[] | select(.key=="QuotaPerUnit")'`.
- Charging is deducted from `quota`; `used_quota` is incremented. The server will refuse new requests when `quota <= 0` (if `quota != unlimited`).
- **Users** have `quota`, **tokens** under that user have `remain_quota`. Token charging draws from the token first, the user second.

Convert when talking to humans:
```bash
QPU=$(curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/option/" \
  | jq -r '.data[] | select(.key=="QuotaPerUnit") | .value')
echo "User 17 quota in USD: $(echo "scale=2; 2000000 / $QPU" | bc)"
```

## Pitfalls

- **`/api/user/manage` takes `username`, not id.** The adjacent endpoints all take id. Don't mix them up.
- **`quota` on PUT is absolute, not additive.** A value of `1000000` replaces whatever was there — use `/api/topup` to add.
- **Cannot promote common→admin as a non-root admin.** The server returns a specific error; escalate to a root user.
- **Soft-delete keeps the row.** Re-creating a user with the same username may collide with a soft-deleted row's unique index — contact DBA if this happens.
- **TOTP disable is audit-sensitive.** Always record the ticket/email thread that justified bypassing MFA.
- **`POST /api/user/` ignores most fields if they look suspicious.** Only `username`, `password`, `display_name`, `email` are taken from the request; `quota` and `group` are applied in a second UPDATE call. `role` from the body is validated against caller's role (see [controller/user.go:1278](../../../../controller/user.go#L1278)).
