# Errors and response envelope

## Envelope

Every handler returns:

```json
{
  "success": true,
  "message": "",
  "data": <endpoint-specific>,
  "total": 0
}
```
`total` only appears on paginated list endpoints. `data` is absent on pure-ack responses (create / delete / manage).

## Truth of success is `.success`, not HTTP status

The server frequently returns **HTTP 200 with `"success": false`** for validation failures and permission denials. Only a few errors surface as non-200:

| HTTP | Typical cause                                          |
|------|--------------------------------------------------------|
| 200  | Everything else — check `.success` in the body (including an unknown resource UUID: `resource reference not found`) |
| 400  | Missing or malformed resource UUID reference (`resource uuid is required`, `invalid resource reference`; see [Resource references](#resource-references-every-resource)), a few explicit validation errors (e.g. `Invalid channel status`), and malformed JSON on `PUT /api/option/`. Most other handlers answer a malformed JSON body with HTTP 200 `invalid parameter`. |
| 401  | Auth header missing / invalid access token             |
| 403  | User disabled or banned, root-only endpoint hit by admin, or a session-cookie mutation without a trusted `Origin` (see [auth.md](auth.md)) |
| 405  | Legacy `GET` on a `POST`-only action route (`Allow: POST`) |
| 5xx  | Database / panic — capture `.message` and report       |

The golden rule: **always parse the body and branch on `.success`** before declaring success. HTTP 200 alone proves nothing. Non-2xx management responses usually carry the same `{"success": false, "message": ...}` body, so read `.message` from them too.

## Pattern: robust curl + jq

```bash
resp=$(curl -sS --fail-with-body -H "Authorization: $ONEAPI_ADMIN_TOKEN" "$URL") \
  || { echo "HTTP error (network / 4xx / 5xx): ${resp:-no body}" >&2; exit 1; }

if ! jq -e '.success' <<<"$resp" >/dev/null; then
  echo "App error: $(jq -r '.message // "unknown"' <<<"$resp")" >&2
  exit 2
fi

echo "$resp" | jq '.data'
```

`curl --fail-with-body` (curl >= 7.76) kicks out on non-2xx but keeps the body, so the `.message` of a `400` reference error stays visible; plain `curl -f` discards it. `oneapi_request` in [scripts/lib.sh](../scripts/lib.sh) gets the same effect by capturing the status separately and printing the body for any HTTP >= 400. `jq -e` exits non-zero on `false`/`null`.

## Common `.message` strings and what to do

### Resource references (every resource)

Users, tokens, channels, redemptions, MCP servers, logs, and passkeys are addressed by UUID (path `:uuid`, body `uuid`/`user_uuid`, or filters such as `channel`, `user_id`, `server_id`). Integer ids are rejected. Source: [controller/id_refs.go](../../../../controller/id_refs.go) `respondResourceRefError`.

| Message                                                            | HTTP | Cause                                              |
|--------------------------------------------------------------------|------|----------------------------------------------------|
| `resource uuid is required`                                         | 400  | `PUT` update or `POST /api/topup` body lacks `uuid`/`user_uuid` (e.g. sending only the integer `id`/`user_id`) — `preferUUIDRef` |
| `invalid resource reference`                                        | 400  | The reference is not a UUID (e.g. a legacy integer id) |
| `resource reference not found`                                      | 200  | Well-formed UUID with no matching row              |
| `Invalid user ID` / `invalid credential id` / `Invalid user_id parameter` | 400 or 200 | Same classification on the admin TOTP, passkey, and `GET /api/user/dashboard` (`user_id`) endpoints, with an endpoint-specific message that a database failure also answers |
| `invalid token id: invalid resource reference`                     | 400  | Malformed `:id` on `GET /api/admin/tokens/:id`; an unknown UUID there answers `resource reference not found` |

A 400 here is a caller bug: fix the reference, do not retry. A database failure during resolution is a server error: HTTP 200 `success:false`, logged at ERROR on the server.

### Auth

| Message                                                            | Cause                                              | Fix |
|--------------------------------------------------------------------|----------------------------------------------------|-----|
| `not logged in and no access token provided`                        | `Authorization` header missing                     | Export `ONEAPI_ADMIN_TOKEN` and include header    |
| `access token is invalid`                                           | Token rotated, mistyped, or user deleted          | Re-mint via UI or `POST /api/user/token`          |
| `User has been banned`                                              | Caller's user row `status=2`                       | Re-enable via another admin / unbanning           |
| `No permission to perform this operation`                           | Insufficient role                                  | Use root token for `/api/option/*`                |

### Users

| Message                                                            | Cause                                              |
|--------------------------------------------------------------------|----------------------------------------------------|
| `Unable to create users with permissions greater than or equal to your own` | Request body sets `role >= caller.role`        |
| `Unable to disable super administrator user`                       | `manage action=disable` targeted root              |
| `Ordinary administrator users cannot promote other users to administrators` | Admin called `promote`; requires root         |
| `User does not exist`                                              | `username` in `/api/user/manage` had no match      |
| `Display name cannot be empty if provided`                         | Whitespace-only display_name in create             |

### Channels

| Message                                                            | Cause                                              |
|--------------------------------------------------------------------|----------------------------------------------------|
| `invalid parameters` / `invalid input`                              | JSON body failed struct validation                 |
| `resource uuid is required` (HTTP 400)                              | `PUT` body lacks `uuid` (sending integer `id` instead) — see [Resource references](#resource-references-every-resource) |
| `invalid resource reference` (HTTP 400) / `resource reference not found` (HTTP 200) | `:uuid` is not a UUID / doesn't exist; this includes the `/api/debug/channel/:uuid/*` routes |
| upstream error strings (`auth`, `rate limit`, `model not found`)    | `POST /test/:id` — issue is on upstream side       |

### Tokens

| Message                                                            | Cause                                              |
|--------------------------------------------------------------------|----------------------------------------------------|
| `Token name is too long`                                            | `name` > 30 chars                                  |
| `invalid network segment`                                           | Malformed CIDR in `subnet`                         |
| `token … has expired at timestamp …`                                | Token's `expired_time` in past                     |
| `API Key … quota has been exhausted`                                | Token hit `remain_quota <= 0`                      |

### Options

| Message                                                            | Cause                                              |
|--------------------------------------------------------------------|----------------------------------------------------|
| `invalid theme`                                                     | PUT `Theme` with unknown value                     |
| `Unable to enable … please fill in … first`                         | Enabling a feature toggle whose prerequisites aren't set |

## Idempotency and retries

- **Create endpoints are NOT idempotent.** Re-posting creates a duplicate (unique constraints may catch some — e.g. `users.username`). For network-retry safety, fetch-before-create.
- **Update endpoints (PUT) overwrite fields you set to non-null.** Always fetch-merge-put for partial updates unless the reference explicitly lists the field as set-only-if-present.
- **Delete endpoints return success if the target already gone.** Check `.message` — empty is fine.
- **Async endpoints** (`test`, `update_balance` without `/:id`) return immediately with `success: true` and do the work in the background. Poll with list/`get` to observe completion.

## Rate limiting

Default rate limits are configurable via `Option`:
- `GlobalApiRateLimitNum` — per-user-id, per-minute
- `GlobalApiRateLimitDuration` — window length

On 429 from the server, back off (one-api does not send `Retry-After`). For bulk admin loops, use `scripts/lib.sh oneapi_throttle` which sleeps 100ms between requests.

## Reporting incidents

When something breaks in a way this document doesn't cover:
1. Capture the exact `curl` (redact `Authorization`).
2. Capture the full response body (not just `.message`).
3. Capture `trace_id` from the response headers or the relevant log row if available.
4. Paste into the incident channel and include the commit SHA of the running server (`GET /api/status` if exposed, else ask ops).
