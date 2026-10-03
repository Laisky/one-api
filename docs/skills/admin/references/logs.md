# Logs reference

Usage and billing audit trail. Admin view at `/api/log/`; user self-view at `/api/log/self`. Source: [controller/log.go](../../../../controller/log.go), routes [router/api.go:152](../../../../router/api.go#L152).

## Endpoint index

| Method | Path                    | Role   | Purpose                                 |
|--------|-------------------------|--------|-----------------------------------------|
| GET    | `/api/log/`             | Admin  | List all logs                           |
| GET    | `/api/log/search`       | Admin  | Search all logs                         |
| GET    | `/api/log/stat`         | Admin  | Aggregate stats                         |
| GET    | `/api/log/self`         | User   | Self-logs only                          |
| GET    | `/api/log/self/search`  | User   | Search self-logs                        |
| GET    | `/api/log/self/stat`    | User   | Self-stats                              |
| DELETE | `/api/log/`             | Admin  | **Destructive.** Delete logs by filter |
| GET    | `/api/log/cursor`       | Admin  | List all logs, keyset pagination (opt-in) |
| GET    | `/api/log/self/cursor`  | User   | Self-logs, keyset pagination (opt-in)     |

## Query parameters

All offset list/search endpoints accept:

| Param             | Type   | Notes                                                   |
|-------------------|--------|---------------------------------------------------------|
| `p`               | int    | 0-indexed page                                          |
| `size`            | int    | Capped at `MaxItemsPerPage`                             |
| `type`            | int    | Log type filter. `1=top-up`, `2=consume`, `3=manage`, `4=system`. Omit for all |
| `start_timestamp` | int64  | Unix **seconds** (inclusive)                            |
| `end_timestamp`   | int64  | Unix **seconds** (inclusive — `created_at <= end_timestamp`) |
| `username`        | string | Admin-only filter (self-routes ignore)                  |
| `token_name`      | string | Filter by token label                                   |
| `model_name`      | string | e.g. `gpt-4o`                                            |
| `channel`         | string | Channel `uuid` (resolved via `resolveOptionalChannelRef`; empty = no filter) |
| `sort` / `sort_by`     | string | One of `created_at` (alias `created_time`), `prompt_tokens`, `completion_tokens`, `quota`, `elapsed_time` ([model/log.go](../../../../model/log.go) `logSortFields`); `id` accepted but opaque |
| `order` / `sort_order` | string | `asc` / `desc`                                      |

**Time range is capped at 30 days when sort requires it** ([controller/log.go](../../../../controller/log.go) — look for `thirty days` / `30 day` guards).


## Keyset pagination (opt-in)

The two `/cursor` routes are **additive siblings** of the offset routes, not a mode of them.
The offset routes' pagination, filters, sorts, default `id DESC` order and exact `total` are
unchanged; use them for anything that needs a stable page address or a snapshot-shaped walk.

They are **disabled by default** (`LOG_CURSOR_ENABLED=false`). The keyset order
(`created_at DESC, id DESC`) needs an access path the shipped schema does not have — on
MySQL 8.4 the first page is a full table scan without it. Enable only on a database with the
supporting indexes; see
[the plan evidence](../../../benchmarks/20260906_w24-cursor-plans.md). When disabled, the
routes answer `{"success": false, "code": "capability_disabled"}` and clients fall back.

Parameters differ from the offset routes:

| Param    | Type   | Notes                                                                  |
|----------|--------|------------------------------------------------------------------------|
| `v`      | int    | Capability version; must be `1` if supplied                            |
| `cursor` | string | Opaque page token from the previous response; omit for the first page  |
| `size`   | int    | Capped at `MaxItemsPerPage`                                            |
| `count`  | string | `exact` requests an exact count under its own budget; omit for the bounded probe |
| `p`      | —      | **Rejected.** A keyset page has no page number                         |
| `sort`   | string | Only `created_at` is supported                                         |
| `order`  | string | Only `desc` is supported                                               |

Filters (`type`, `start_timestamp`, `end_timestamp`, `username`, `token_name`, `model_name`,
`channel`) behave exactly as on the offset routes and select exactly the same rows.

Response:

```json
{
  "success": true,
  "version": 1,
  "data": [ /* log rows, newest first */ ],
  "has_more": true,
  "next_cursor": "lc1.…",
  "count": { "value": 10000, "quality": "lower_bound", "as_of": 1767225540, "cached": false },
  "bytes_capped": true
}
```

- `count.quality` is `exact`, `lower_bound` or `unavailable`. A `lower_bound` means the
  bounded probe stopped at its limit — it is **not** a total. An `unavailable` count carries
  a null `value`; never render it as zero.
- `count.as_of` is when the count ran, and `count.cached` marks a reused one. A cached exact
  count is exact as of `as_of`, not as of now.
- `bytes_capped` means the page stopped early to stay under `LOG_CURSOR_MAX_RESPONSE_BYTES`.
  `oversized_record` means one record alone exceeded the budget and is returned on its own.
  **No field is ever truncated.**

A cursor is bound to the caller's scope, the endpoint, the normalized filters and an expiry.
Presenting it under a different user, on the other route, or with changed filters returns
`{"success": false, "restart_required": true, "code": "cursor_expired" | "cursor_invalid" |
"cursor_query_changed"}` — restart from the first page. A cursor is never an authorization
grant; scope is re-derived from the authenticated principal on every request.

**Live traversal, not a snapshot.** Rows inserted ahead of the anchor do not appear and do
not shift later pages; deletes may shorten them; late provisional finalization can change
membership. For snapshot-complete audit work, use the export path, which walks the offset
routes.

## List logs

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  --data-urlencode "username=alice" \
  --data-urlencode "start_timestamp=$(date -d '7 days ago' +%s)" \
  --data-urlencode "end_timestamp=$(date +%s)" \
  --data-urlencode "type=2" \
  --data-urlencode "size=100" \
  -G "$ONEAPI_BASE_URL/api/log/" \
  | jq '{total, items: (.data | map({created_at, username, token_name, model_name, prompt_tokens, completion_tokens, quota, channel_uuid, channel_name}))}'
```

Always pass timestamps via `--data-urlencode` — some shells mangle the Unix-seconds integer into scientific notation.

## Log object — fields you'll use

(Same as web UI table rows.)

| Field               | Notes                                                     |
|---------------------|-----------------------------------------------------------|
| `uuid`              | Log row identifier (string); feed it to `/api/trace/log/:log_id` |
| `user_uuid`         | Requesting user's `uuid` (nullable)                        |
| `created_at`        | Unix **seconds** (`helper.GetTimestamp()`), not milliseconds |
| `type`              | 1=top-up, 2=consume, 3=manage, 4=system                    |
| `username`          | Who made the request                                       |
| `token_name`        | Token label (useful for drilling into a specific key)      |
| `token_uuid`        | Token's `uuid` (nullable) — exact join key to `/api/admin/tokens/:uuid` |
| `model_name`        | Model as seen from consumer                                |
| `channel_uuid`      | Which upstream served it (nullable); pass it back as `?channel=` |
| `channel_name`      | Channel label at write time (omitted when empty)           |
| `prompt_tokens`     | Input token count                                          |
| `completion_tokens` | Output token count                                         |
| `quota`             | Units charged (convert with `QuotaPerUnit`)                |
| `content`           | Free-text detail (errors, admin actions)                   |
| `trace_id`          | Join key with the tracing table                            |
| `request_id`        | Upstream request id if echoed                              |

## Top spenders (ad-hoc)

```bash
# Last 24h, group by user, top 10
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  --data-urlencode "start_timestamp=$(date -d '24 hours ago' +%s)" \
  --data-urlencode "end_timestamp=$(date +%s)" \
  --data-urlencode "type=2" \
  --data-urlencode "size=1000" \
  -G "$ONEAPI_BASE_URL/api/log/" \
  | jq '[.data[] | {username, quota}] | group_by(.username) | map({username: .[0].username, total: (map(.quota) | add)}) | sort_by(-.total) | .[0:10]'
```

For anything beyond a few hundred rows, paginate (see [scripts/lib.sh](../scripts/lib.sh) `oneapi_paginate`).

## Per-token usage

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  --data-urlencode "token_name=prod-api-key" \
  --data-urlencode "start_timestamp=$(date -d '30 days ago' +%s)" \
  --data-urlencode "end_timestamp=$(date +%s)" \
  --data-urlencode "size=200" \
  -G "$ONEAPI_BASE_URL/api/log/" \
  | jq '.data | group_by(.model_name) | map({model: .[0].model_name, calls: length, tokens_in: (map(.prompt_tokens)|add), tokens_out: (map(.completion_tokens)|add), quota: (map(.quota)|add)})'
```

## Aggregate stats

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/log/stat" | jq .
```
Shape varies by version — inspect before depending on specific keys.

## Trace lookup

Every request produces a trace record with per-stage timestamps.

```bash
# By trace_id (from a log row)
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/trace/$TRACE_ID" | jq .

# By log uuid (the log row's `uuid` field)
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/trace/log/$LOG_UUID" | jq .
```
Admin and user both allowed — users see their own traces only.

## Deleting logs

Destructive and irreversible. Breaks historical billing audit.

```bash
# DO NOT RUN without explicit authorization.
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  --data-urlencode "target_timestamp=$(date -d '1 year ago' +%s)" \
  -G -X DELETE "$ONEAPI_BASE_URL/api/log/"
```
Before running: export a copy of the rows you're about to delete, and confirm with the user in-chat.

## Pitfalls

- **`start_timestamp`/`end_timestamp` are seconds; `created_at` is milliseconds.** Don't cross the streams.
- **Logs include both successful and failed requests.** Filter by `type=2` (consume) for billing-relevant rows; `type=4` (system) for server internals.
- **Pagination past ~10000 rows is slow** — the `count(*)` becomes expensive on large deployments. Always pass a tight time window.
- **`username` filter is a substring match**, not an exact match on newer versions — double-check results for collisions like `alice` matching `alice-bot`.
- **No integer ids on log rows.** `id`, `user_id`, `channel_id`, `token_id` are gone; use `uuid`, `user_uuid`, `channel_uuid`, `token_uuid`. The `?channel=` filter takes a channel `uuid`.
- **`channel_uuid` shows which upstream served the request**, but if a request retried, only the final channel is recorded. For retry telemetry, check the trace.
- **`content` field carries freeform upstream error text.** Parse defensively — do not regex it into JSON.
