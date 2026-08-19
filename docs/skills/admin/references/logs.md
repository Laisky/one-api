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

## Query parameters

All list/search endpoints accept:

| Param             | Type   | Notes                                                   |
|-------------------|--------|---------------------------------------------------------|
| `p`               | int    | 0-indexed page                                          |
| `size`            | int    | Capped at `MaxItemsPerPage`                             |
| `type`            | int    | Log type filter. `1=top-up`, `2=consume`, `3=manage`, `4=system`. Omit for all |
| `start_timestamp` | int64  | Unix **seconds** (inclusive)                            |
| `end_timestamp`   | int64  | Unix **seconds** (exclusive)                            |
| `username`        | string | Admin-only filter (self-routes ignore)                  |
| `token_name`      | string | Filter by token label                                   |
| `model_name`      | string | e.g. `gpt-4o`                                            |
| `channel`         | int    | Channel id                                              |
| `sort` / `sort_by`     | string | Column name                                        |
| `order` / `sort_order` | string | `asc` / `desc`                                      |

**Time range is capped at 30 days when sort requires it** ([controller/log.go](../../../../controller/log.go) — look for `thirty days` / `30 day` guards).

## List logs

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  --data-urlencode "username=alice" \
  --data-urlencode "start_timestamp=$(date -d '7 days ago' +%s)" \
  --data-urlencode "end_timestamp=$(date +%s)" \
  --data-urlencode "type=2" \
  --data-urlencode "size=100" \
  -G "$ONEAPI_BASE_URL/api/log/" \
  | jq '{total, items: (.data | map({created_at, username, token_name, model_name, prompt_tokens, completion_tokens, quota, channel_id}))}'
```

Always pass timestamps via `--data-urlencode` — some shells mangle the Unix-seconds integer into scientific notation.

## Log object — fields you'll use

(Same as web UI table rows.)

| Field               | Notes                                                     |
|---------------------|-----------------------------------------------------------|
| `created_at`        | Millisecond timestamp                                      |
| `type`              | 1=top-up, 2=consume, 3=manage, 4=system                    |
| `username`          | Who made the request                                       |
| `token_name`        | Token label (useful for drilling into a specific key)      |
| `model_name`        | Model as seen from consumer                                |
| `channel_id`        | Which upstream served it                                   |
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

# By log id (when you only have the log row id)
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/trace/log/$LOG_ID" | jq .
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
- **`channel_id` shows which upstream served the request**, but if a request retried, only the final channel is recorded. For retry telemetry, check the trace.
- **`content` field carries freeform upstream error text.** Parse defensively — do not regex it into JSON.
