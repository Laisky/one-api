# Channels reference

Channels are the upstream LLM providers (OpenAI, Azure, Anthropic, ...). All endpoints are `AdminAuth`-guarded under `/api/channel`. Source: [controller/channel.go](../../../../controller/channel.go), routes [router/api.go:94](../../../../router/api.go#L94).

## Channel object

The full schema lives in [model/channel.go](../../../../model/channel.go). Fields you'll use most:

| Field            | Type     | Notes                                                                 |
|------------------|----------|-----------------------------------------------------------------------|
| `uuid`           | string   | Server-assigned external identifier. Use it in every `:uuid` path and `PUT` body; no integer `id` is exposed |
| `type`           | int      | Provider enum — see table below                                       |
| `name`           | string   | Human label, not unique                                               |
| `key`            | string   | Upstream API key. Comma-separated for round-robin: `"k1,k2,k3"`       |
| `base_url`       | *string  | Override upstream URL. Optional for OpenAI (uses default)             |
| `models`         | string   | Comma-separated model ids: `"gpt-4o,gpt-4o-mini"`                      |
| `model_mapping`  | *string  | JSON string mapping consumer→upstream: `{"gpt-4":"gpt-4-turbo"}`. Send `null` or `""` to clear; omit to keep current value. |
| `model_configs`  | *string  | JSON string with per-model pricing (new format). Preferred over `model_ratio`/`completion_ratio`. Send `null` or `""` to clear; omit to keep current value. |
| `group`          | string   | Billing groups (comma-separated): `"default,vip"`                      |
| `priority`       | *int64   | Higher wins when the same model is served by multiple channels        |
| `weight`         | *uint    | Tie-break among same-priority channels (weighted random)              |
| `status`         | int      | `1=enabled`, `2=manually disabled`, `3=auto disabled` — see [model/channel.go:26](../../../../model/channel.go#L26) |
| `config`         | string   | Provider-specific JSON config (e.g. Azure `api_version`, `plugin`)    |
| `system_prompt`  | *string  | Injected at top of every request — leave null unless you know why. Send `null` or `""` to clear; omit to keep current value. |
| `ratelimit`      | *int     | Per-channel req/min cap; null = unlimited                             |
| `testing_model`  | *string  | Model used by `GET /test/:id`. Null → cheapest supported              |
| `balance`        | float64  | USD, refreshed by `/update_balance/:id`                                |

### Channel types (subset)

Defined in [relay/channeltype/define.go](../../../../relay/channeltype/define.go). Most common:

| Int | Provider      | Int | Provider        |
|-----|---------------|-----|-----------------|
| 1   | OpenAI        | 28  | Gemini          |
| 3   | Azure         | 33  | AwsClaude       |
| 14  | Anthropic     | 36  | DeepSeek        |
| 15  | Baidu         | 45  | Doubao          |
| 17  | Ali (Tongyi)  | 49  | XAI             |
| 19  | AI360         | 52  | AliBailian      |
| 20  | OpenRouter    | 54  | OpenAICompatible |
| 25  | Moonshot      | 58  | Fireworks       |

For the complete list, read [relay/channeltype/define.go](../../../../relay/channeltype/define.go) and count iota from 1.

## Endpoint index

| Method | Path                          | Purpose                                  |
|--------|-------------------------------|------------------------------------------|
| GET    | `/api/channel/`               | Paginated list                           |
| GET    | `/api/channel/search`         | Keyword search                           |
| GET    | `/api/channel/:uuid`          | Single channel                           |
| POST   | `/api/channel/`               | Create (returns `{success, message}` only — no `data`) |
| PUT    | `/api/channel/`               | Update (`uuid` in body)                  |
| DELETE | `/api/channel/:uuid`          | Delete                                   |
| DELETE | `/api/channel/disabled`       | Delete all status-2 and status-3         |
| GET    | `/api/channel/models`         | All model ids supported across channels  |
| GET    | `/api/channel/metadata`       | Channel-building metadata (types, defaults) |
| GET    | `/api/channel/test`           | Test multiple channels (async)           |
| GET    | `/api/channel/test/:uuid`     | Test one channel synchronously           |
| GET    | `/api/channel/update_balance` | Refresh balance for all (async)          |
| GET    | `/api/channel/update_balance/:uuid` | Refresh balance for one               |
| GET    | `/api/channel/pricing/:uuid`  | Fetch per-channel pricing config         |
| PUT    | `/api/channel/pricing/:uuid`  | Replace per-channel pricing config       |

Every `:uuid` segment is the channel's `uuid` string; the server resolves it via `resolveChannelRef` ([controller/id_refs.go](../../../../controller/id_refs.go)) and returns `channel not found` for anything else.
| GET    | `/api/channel/default-pricing`| System default pricing                   |

## List channels

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/channel/?p=0&size=50&sort=priority&sort_order=desc" \
  | jq '{total, items: (.data | map({uuid, name, type, status, group, priority}))}'
```
Query params: `p` (page, 0-indexed), `size`, `sort`, `sort_order` (`asc|desc`). Sort allowlist ([model/channel.go](../../../../model/channel.go) `channelSortFields`): `name|type|status|response_time|test_time|priority|weight|used_quota|created_at|updated_at`. `id` is accepted but sorts on a value you cannot see — use `created_at` for insertion order. `balance` is **not** sortable server-side; sort client-side with `jq 'sort_by(.balance)'`.

## Search

```bash
# Matches name / model membership; a keyword that parses as a UUID matches `uuid` exactly
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/channel/search?keyword=azure" \
  | jq '.data[] | {uuid, name, type}'
```

## Create a channel

Minimal OpenAI example:
```bash
jq -nc --arg key "$UPSTREAM_KEY" '{
  type: 1,
  name: "openai-prod",
  key: $key,
  models: "gpt-4o,gpt-4o-mini,gpt-3.5-turbo",
  group: "default",
  priority: 10,
  status: 1
}' | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X POST -d @- \
      "$ONEAPI_BASE_URL/api/channel/" \
  | jq '{success, message}'

# The create response carries no `data`; look the new channel up by name to get its uuid
CHANNEL_UUID=$(curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/channel/search?keyword=openai-prod" \
  | jq -r '.data[] | select(.name == "openai-prod") | .uuid')
```

Azure example (needs `config` with `api_version`, and `base_url` to your Azure endpoint):
```bash
jq -nc --arg key "$AZURE_KEY" --arg url "$AZURE_ENDPOINT" '{
  type: 3,
  name: "azure-eastus",
  key: $key,
  base_url: $url,
  models: "gpt-4o",
  config: ({api_version: "2024-10-21"} | tostring),
  group: "default",
  priority: 5,
  status: 1
}' | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X POST -d @- "$ONEAPI_BASE_URL/api/channel/"
```

**Encoding pitfalls:**
- `models`: comma-separated string, no spaces.
- `key`: for round-robin use comma-separated. For JSON-key upstreams (e.g. Vertex AI service account), the whole JSON goes in `key` as a single string.
- `config`: JSON string, not an object. Build with `(… | tostring)` in `jq`.
- `model_mapping`: JSON string. `{"gpt-4":"gpt-4-0613"}` tells the router "when the user asks for gpt-4, call upstream's gpt-4-0613".
- `model_configs`: per-channel pricing JSON. See [groups-and-ratios.md](groups-and-ratios.md).

## Update a channel

PUT with `uuid` in the body (missing → `resource uuid is required`; [controller/channel.go](../../../../controller/channel.go) `UpdateChannel`). Include only fields you want to change, plus fields the server requires:
```bash
# Change priority
jq -nc --arg uuid "$CHANNEL_UUID" '{uuid: $uuid, priority: 20}' \
  | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/channel/"
```

To re-enable an auto-disabled channel:
```bash
jq -nc --arg uuid "$CHANNEL_UUID" '{uuid: $uuid, status: 1}' \
  | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/channel/"
```
**Do this only after fixing the root cause.** A test call (`GET /api/channel/test/$CHANNEL_UUID`) before re-enabling is mandatory.

## Disable / delete

**Disable (preferred):**
```bash
jq -nc --arg uuid "$CHANNEL_UUID" '{uuid: $uuid, status: 2}' \
  | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/channel/"
```

**Delete (destructive, irreversible):**
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  -X DELETE "$ONEAPI_BASE_URL/api/channel/$CHANNEL_UUID" \
  | jq '{success, message}'
```
Historical logs keep their `channel_uuid`, but the channel row is gone. Prefer keep-disabled for at least one billing cycle.

**Bulk-delete all disabled:**
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  -X DELETE "$ONEAPI_BASE_URL/api/channel/disabled"
```
Removes every channel with `status` in `{2, 3}`. Require explicit user confirmation.

## Test a channel

Single, synchronous:
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/channel/test/$CHANNEL_UUID?model=gpt-4o-mini" \
  | jq '{success, message, time, modelName}'
```
Response:
- `success: true` → round-trip succeeded. `time` is seconds.
- `success: false` → `.message` has the upstream error (auth, rate limit, model unavailable). The channel is **not** auto-disabled from a single failed manual test.

All at once, async (fire-and-forget):
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/channel/test?scope=enabled"
```
`scope`: `all` | `enabled` | `<group-name>`. Results appear in channel rows (`test_time`, `response_time`) — poll with list.

## Balance refresh

```bash
# Single
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/channel/update_balance/$CHANNEL_UUID" \
  | jq '{success, balance, message}'

# All (async)
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/channel/update_balance"
```
Not all providers expose balance APIs. `success=false` with `"not supported"` is normal for those.

## Pricing

Per-channel pricing lives in two places:
- `model_configs` field on the channel row (preferred, new format).
- `/api/channel/pricing/:uuid` endpoints (same data, dedicated endpoints).

See [groups-and-ratios.md](groups-and-ratios.md) for the ratio model and JSON shape.

Fetch:
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/channel/pricing/$CHANNEL_UUID" | jq .
```

Replace:
```bash
jq -nc '{
  "gpt-4o":      {"input_ratio": 2.5, "output_ratio": 10.0},
  "gpt-4o-mini": {"input_ratio": 0.15, "output_ratio": 0.6}
}' | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/channel/pricing/$CHANNEL_UUID"
```

## Pitfalls

- **No integer ids.** Responses expose `uuid` only and every path/body identifier must be that string. `{id: 42, ...}` fails with `resource uuid is required`; build bodies with `jq --arg uuid` (string), never `--argjson`.
- **`POST /api/channel/` returns no `data`.** Capture the new `uuid` with a follow-up `/api/channel/search?keyword=<name>` filtered by exact `.name`.
- **`weight` is a `*uint` pointer.** Null means "use default weight". To force equal weight, omit the field rather than sending `0`.
- **`priority` default is 0.** If you create a channel without setting priority, it's the lowest. Set `priority: 10` for "normal" and higher for premium routes.
- **Changing `models` does not resync abilities automatically — most server versions trigger an ability-table rebuild on channel update. If a newly added model isn't routable, call `POST /api/debug/channel/:uuid/fix`.**
- **`group` must match a group listed in `/api/group/`.** Adding a channel with `group: "vip"` when `vip` doesn't exist in `GroupRatio` silently routes no one to it. Create the group first (see [groups-and-ratios.md](groups-and-ratios.md)).
- **Azure channels need `config.api_version`.** Missing → 404 from upstream on first request.
- **Deprecated fields `model_ratio` and `completion_ratio` on the channel row are for backward compat.** Prefer `model_configs`.
- **Testing model defaults to the cheapest supported model on the channel.** To avoid per-test cost surprises, set `testing_model` explicitly (e.g. `"gpt-4o-mini"` for OpenAI, `"gemini-2.0-flash-lite"` for Gemini).
- **Clearing nullable text fields (`model_mapping`, `model_configs`, `system_prompt`, `inference_profile_arn_map`) requires sending the key with `null` or `""`.** Omitting the key keeps the previous value (the controller records which keys were present in the raw body and forces a per-column update for those).
- **Clearing `hidden_models` requires sending the key explicitly** (same reason).
