# Tokens reference

Two distinct surfaces:

1. **User-scoped CRUD** at `/api/token/` — an admin can only manage **their own** tokens here (any user can, per [router/api.go:124](../../../../router/api.go#L124)).
2. **Admin read-only** at `/api/admin/tokens/` — admin sees any user's tokens, no writes. Added by this skill's patches at [router/api.go:134](../../../../router/api.go#L134).

There is no admin write path for another user's tokens by design. To cut off a user's tokens, disable the **user**: `POST /api/user/manage {action:"disable"}`.

## Token object

Schema: [model/token.go:27](../../../../model/token.go#L27).

| Field             | Type     | Notes                                                     |
|-------------------|----------|-----------------------------------------------------------|
| `id`              | int      |                                                           |
| `user_id`         | int      | Owning user                                               |
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
| GET    | `/api/admin/tokens/`            | List any user's tokens (optional `user_id` filter) |
| GET    | `/api/admin/tokens/search`      | Keyword search across all tokens                |
| GET    | `/api/admin/tokens/:id`         | Single token by id regardless of owner          |

### List tokens for a specific user (admin)

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/admin/tokens/?user_id=17&p=0&size=50" \
  | jq '{total, items: (.data | map({id, user_id, name, status, remain_quota, used_quota, expired_time}))}'
```
Omit `user_id` (or set to `0`) to list across all users.

### Search tokens by name (admin)

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/admin/tokens/search?keyword=prod" \
  | jq '.data[] | {id, user_id, name, status}'
```

### Fetch one token by id (admin)

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/admin/tokens/4823" \
  | jq '.data'
```

## User-scoped endpoints (operate on YOUR tokens)

Under `middleware.UserAuth()` — any authenticated user, including admin, using their own token.

| Method | Path                    | Purpose                            |
|--------|-------------------------|------------------------------------|
| GET    | `/api/token/`           | List my tokens (paginated)          |
| GET    | `/api/token/search`     | Keyword search my tokens            |
| GET    | `/api/token/:id`        | Fetch one of my tokens              |
| POST   | `/api/token/`           | Create a token for myself           |
| PUT    | `/api/token/`           | Update one of my tokens             |
| DELETE | `/api/token/:id`        | Delete one of my tokens             |

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
  | jq '{success, message, key: .data.key, id: .data.id}'
```

The returned `data.key` is the actual credential — capture it now, it's re-fetchable but the one-time generation flow in the UI doesn't show it after creation.

### Update a token (status-only flag)

`PUT /api/token/?status_only=1` updates just `status` without re-validating the other fields:
```bash
jq -nc '{id: 123, status: 2}' \
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
# 1. Resolve user to id
UID=$(curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/user/search?keyword=alice" \
  | jq -r '.data[0].id')

# 2. List their tokens
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/admin/tokens/?user_id=$UID&size=100" \
  | jq '.data[] | {id, name, status, remain_quota, used_quota, subnet, models, expired_time}'

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
3. **Delete the user** (`DELETE /api/user/:id`) — soft-delete, still reversible by DB restore, but not via API.

Admins cannot directly toggle another user's token status via the API. That's intentional: tokens are the user's credential.

## Pitfalls

- **The prefix displayed (`sk-...`) is configurable** via `TokenPrefix` option. The stored `key` field in the DB has no prefix. When parsing logs, strip the prefix before comparing.
- **`remain_quota` with `unlimited_quota=true` is meaningless** — the check short-circuits. Don't present `remain_quota` for unlimited tokens.
- **`status=3` (expired) and `status=4` (exhausted) are set by the server, not by admins.** Re-enabling (`status=1`) without fixing the underlying cause (extend `expired_time`, top up `remain_quota`) silently flips back on the next request.
- **IP subnet enforcement happens at request time.** A stale CIDR will reject legitimate calls with an opaque auth error — always check the user's current client IP before narrowing `subnet`.
- **`models` filter is AND-composed with channel-level models.** A token allowing `"gpt-5"` but no channel serves `gpt-5` → no route. Use `/api/channel/models` to confirm what's actually available.
