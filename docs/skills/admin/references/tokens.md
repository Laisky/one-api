# Tokens reference

Two distinct surfaces:

1. **User-scoped CRUD** at `/api/token/` — an admin can only manage **their own** tokens here (any user can, per [router/api.go:124](../../../../router/api.go#L124)).
2. **Admin read-only** at `/api/admin/tokens/` — admin sees any user's tokens, no writes. Added by this skill's patches at [router/api.go:134](../../../../router/api.go#L134).

There is no admin write path for another user's tokens by design. To cut off a user's tokens, disable the **user**: `POST /api/user/manage {action:"disable"}`.

## Token object

Schema: [model/token.go:27](../../../../model/token.go#L27).

| Field             | Type     | Notes                                                     |
|-------------------|----------|-----------------------------------------------------------|
| `uuid`            | string   | External identifier; use it in `/api/token/:uuid`, `/api/admin/tokens/:uuid` and `PUT` bodies. No integer `id` is exposed |
| `user_uuid`       | *string  | Owning user's `uuid`                                       |
| `key`             | string   | The actual bearer credential. Stored without prefix; returned with configured prefix (default `sk-`) |
| `status`          | int      | `1=enabled`, `2=disabled`, `3=expired`, `4=exhausted`     |
| `name`            | string   | ≤ 30 chars, user-chosen label                             |
| `expired_time`    | int64    | Unix seconds; `-1` = never                                 |
| `remain_quota`    | int64    | Remaining units (int64)                                   |
| `unlimited_quota` | bool     | Bypasses `remain_quota`                                   |
| `used_quota`      | int64    | Running total                                             |
| `models`          | *string  | Comma-separated allow-list; null = all                    |
| `subnet`          | *string  | CIDR allow-list (e.g. `"10.0.0.0/8,192.168.1.0/24"`)       |
| `created_at`/`updated_at` | int64 | Milliseconds                                          |

## Admin endpoints (read-only, added by this skill)

| Method | Path                            | Purpose                                        |
|--------|---------------------------------|------------------------------------------------|
| GET    | `/api/admin/tokens/`            | List any user's tokens (optional `user_id=<user-uuid>` filter) |
| GET    | `/api/admin/tokens/search`      | Keyword search across all tokens (name, or a pasted token/user uuid) |
| GET    | `/api/admin/tokens/:uuid`       | Single token by uuid regardless of owner        |

### List tokens for a specific user (admin)

`user_id` is the **owning user's `uuid`** (the param name is historical). Resolved by `resolveOptionalUserRef` ([controller/token.go](../../../../controller/token.go) `AdminGetAllTokens`).
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  --data-urlencode "user_id=$USER_UUID" \
  --data-urlencode "p=0" --data-urlencode "size=50" \
  -G "$ONEAPI_BASE_URL/api/admin/tokens/" \
  | jq '{total, items: (.data | map({uuid, user_uuid, name, status, remain_quota, used_quota, expired_time}))}'
```
**Omit `user_id` entirely** to list across all users. Do not send `user_id=0` — it is not a uuid and is rejected. Sort keys ([model/token.go](../../../../model/token.go) `tokenSortFields`): `uuid`, `name`, `status`, `expired_time`, `remain_quota`, `used_quota`, `created_at`, `updated_at` (`id` accepted but opaque — prefer `created_at`).

### Search tokens by name (admin)

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/admin/tokens/search?keyword=prod" \
  | jq '.data[] | {uuid, user_uuid, name, status}'
```
A keyword that parses as a UUID matches `uuid` **or** `user_uuid` exactly ([model/token.go](../../../../model/token.go) `applyUUIDKeyword`), so pasting a user's uuid returns that user's tokens.

### Fetch one token by uuid (admin)

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/admin/tokens/$TOKEN_UUID" \
  | jq '.data'
```

## User-scoped endpoints (operate on YOUR tokens)

Under `middleware.UserAuth()` — any authenticated user, including admin, using their own token.

| Method | Path                    | Purpose                            |
|--------|-------------------------|------------------------------------|
| GET    | `/api/token/`           | List my tokens (paginated)          |
| GET    | `/api/token/search`     | Keyword search my tokens            |
| GET    | `/api/token/:uuid`      | Fetch one of my tokens              |
| POST   | `/api/token/`           | Create a token for myself           |
| PUT    | `/api/token/`           | Update one of my tokens (`uuid` in body) |
| DELETE | `/api/token/:uuid`      | Delete one of my tokens             |

### Create a token (your own)

```bash
jq -nc '{
  name: "prod-api-key",
  expired_time: -1,
  remain_quota: 100000,
  unlimited_quota: false,
  models: "gpt-4o,gpt-4o-mini",
  subnet: "10.0.0.0/8"
}' | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X POST -d @- "$ONEAPI_BASE_URL/api/token/" \
  | jq '{success, message, key: .data.key, uuid: .data.uuid}'
```

`data` is the created token in response shape — capture `data.uuid` for later `PUT`/`DELETE` calls. The returned `data.key` is the actual credential — capture it now, it's re-fetchable but the one-time generation flow in the UI doesn't show it after creation.

### Update a token (status-only flag)

`PUT /api/token/?status_only=1` updates just `status` without re-validating the other fields. The body must carry the token's `uuid` ([controller/token.go](../../../../controller/token.go) `UpdateToken` → `preferUUIDRef`):
```bash
jq -nc --arg uuid "$TOKEN_UUID" '{uuid: $uuid, status: 2}' \
  | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/token/?status_only=1"
```

## Token-authenticated endpoints (not for admin ops)

These take a `Bearer sk-...` (the token's `key`), not your admin access token — different auth mechanism.

| Method | Path                        | Purpose                                         |
|--------|-----------------------------|-------------------------------------------------|
| GET    | `/api/token/balance`        | Remaining quota for the token making the call   |
| GET    | `/api/token/transactions`   | Token's transaction log                         |
| GET    | `/api/token/logs`           | Usage logs for this token                        |
| POST   | `/api/token/consume`        | External billing: pre-consume / post-consume / cancel a transaction |

Use from external billing integrations, not admin flows.

## Investigating another user's tokens

Now that admin read endpoints exist, the standard flow for "user X's tokens are misbehaving" is:

```bash
# 1. Resolve user to uuid (select by exact username — search is a substring match)
USER_UUID=$(curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/user/search?keyword=alice" \
  | jq -r '.data[] | select(.username == "alice") | .uuid')

# 2. List their tokens (user_id = the user's uuid)
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  --data-urlencode "user_id=$USER_UUID" --data-urlencode "size=100" \
  -G "$ONEAPI_BASE_URL/api/admin/tokens/" \
  | jq '.data[] | {uuid, name, status, remain_quota, used_quota, subnet, models, expired_time}'

# 3. Cross-reference with logs (filter by token_name found above)
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  --data-urlencode "token_name=<name-from-step-2>" \
  --data-urlencode "start_timestamp=$(date -d '24 hours ago' +%s)" \
  --data-urlencode "end_timestamp=$(date +%s)" \
  -G "$ONEAPI_BASE_URL/api/log/" \
  | jq '.data[] | {created_at, model_name, prompt_tokens, completion_tokens, quota}'
```

## Stopping a user's access

Options in order of reversibility:

1. **Disable the user** (`POST /api/user/manage {action:"disable"}`) — all their tokens stop working. Reversible.
2. **Have the user disable their own token** — route the admin request through support.
3. **Delete the user** (`DELETE /api/user/:uuid`) — soft-delete, still reversible by DB restore, but not via API.

Admins cannot directly toggle another user's token status via the API. That's intentional: tokens are the user's credential.

## Pitfalls

- **No integer ids.** Token rows expose `uuid` and `user_uuid` only. `{id: 123, status: 2}` fails with `resource uuid is required`; `?user_id=0` is rejected — omit the filter instead.
- **The prefix displayed (`sk-...`) is configurable** via `TokenPrefix` option. The stored `key` field in the DB has no prefix. When parsing logs, strip the prefix before comparing.
- **`remain_quota` with `unlimited_quota=true` is meaningless** — the check short-circuits. Don't present `remain_quota` for unlimited tokens.
- **`status=3` (expired) and `status=4` (exhausted) are set by the server, not by admins.** Re-enabling (`status=1`) without fixing the underlying cause (extend `expired_time`, top up `remain_quota`) silently flips back on the next request.
- **IP subnet enforcement happens at request time.** A stale CIDR will reject legitimate calls with an opaque auth error — always check the user's current client IP before narrowing `subnet`.
- **`models` filter is AND-composed with channel-level models.** A token allowing `"gpt-5"` but no channel serves `gpt-5` → no route. Use `/api/channel/models` to confirm what's actually available.
