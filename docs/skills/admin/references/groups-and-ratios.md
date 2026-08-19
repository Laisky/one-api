# Groups and ratios reference

How one-api prices a request:

```
cost_in_quota_units = (prompt_tokens * input_ratio + completion_tokens * output_ratio)
                    * model_ratio          # base price per model
                    * group_ratio          # discount/surcharge per billing group
                    * completion_ratio     # multiplier for output tokens (legacy path)
```

Source of truth for the pricing code lives in [relay/billing/ratio/](../../../../relay/billing/ratio/).

Admin touches four knobs:

1. **Groups** — a list of billing-group names; each maps to a `group_ratio` multiplier.
2. **Model ratios** — base price per model (legacy global JSON, or per-channel `model_configs`).
3. **Completion ratios** — output-token multiplier (legacy).
4. **Per-channel pricing** (new preferred path) — all of the above in a single `model_configs` JSON on the channel row.

## Listing groups

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/group/" | jq '.data'
# => ["default", "vip", "svip"]
```
Source: [controller/group.go](../../../../controller/group.go), route [router/api.go:168](../../../../router/api.go#L168). Returns keys of the current `GroupRatio` map ([relay/billing/ratio/group.go:13](../../../../relay/billing/ratio/group.go#L13)). There is **no** create-group endpoint — a group is created by adding a key under `GroupRatio` via the Option API (see below).

## Reading the ratios

Groups, model ratios, completion ratios, and system config all live in the **Option** table, reachable via `/api/option/` (`RootAuth` — root user only).

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/option/" \
  | jq '.data | map(select(.key|IN("GroupRatio","ModelRatio","CompletionRatio","QuotaPerUnit")))'
```
Each entry is `{key: "<OptionKey>", value: "<string>"}`. For `ModelRatio`/`CompletionRatio`/`GroupRatio` the value is a **JSON-encoded string** — double-parse to inspect:
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/option/" \
  | jq -r '.data[] | select(.key=="GroupRatio") | .value' \
  | jq .
# => {"default":1, "vip":0.5, "svip":0.3}
```

## Changing a ratio (root only)

`PUT /api/option/` takes a single `{key, value}`:

```bash
# Read current, then write back with one modified entry.
CUR=$(curl -fsS -H "Authorization: $ONEAPI_ROOT_TOKEN" \
  "$ONEAPI_BASE_URL/api/option/" \
  | jq -r '.data[] | select(.key=="GroupRatio") | .value')

NEW=$(echo "$CUR" | jq -c '. + {"vip": 0.4}')   # lower vip from 0.5 to 0.4

jq -nc --arg v "$NEW" '{key: "GroupRatio", value: $v}' \
  | curl -fsS -H "Authorization: $ONEAPI_ROOT_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/option/"
```
**Validate before PUT.** `jq -e '. | type == "object"' <<< "$NEW"` must exit 0. A malformed JSON string is accepted but silently breaks ratio lookups on next reload.

Rollback:
```bash
jq -nc --arg v "$CUR" '{key: "GroupRatio", value: $v}' \
  | curl -fsS -H "Authorization: $ONEAPI_ROOT_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/option/"
```

Always keep `$CUR` in a shell var or paste it somewhere retrievable before mutating.

## Adding a new group

Add a key to `GroupRatio`:
```bash
CUR=$(curl -fsS -H "Authorization: $ONEAPI_ROOT_TOKEN" \
  "$ONEAPI_BASE_URL/api/option/" \
  | jq -r '.data[] | select(.key=="GroupRatio") | .value')

NEW=$(echo "$CUR" | jq -c '. + {"enterprise": 0.8}')

jq -nc --arg v "$NEW" '{key: "GroupRatio", value: $v}' \
  | curl -fsS -H "Authorization: $ONEAPI_ROOT_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/option/"
```

Then a channel or user assigned `group: "enterprise"` will be billed at 0.8× the base. Without this step, they're silently billed at 1.0× (fallback when group is unknown — see [relay/billing/ratio/group.go:37](../../../../relay/billing/ratio/group.go#L37)).

## Removing a group

```bash
CUR=$(curl -fsS -H "Authorization: $ONEAPI_ROOT_TOKEN" "$ONEAPI_BASE_URL/api/option/" \
  | jq -r '.data[] | select(.key=="GroupRatio") | .value')

# Check no user/channel still references it before deleting
# (queries require direct DB access — grep for the group in users and channels via the search endpoints)

NEW=$(echo "$CUR" | jq -c 'del(.enterprise)')
jq -nc --arg v "$NEW" '{key: "GroupRatio", value: $v}' \
  | curl -fsS -H "Authorization: $ONEAPI_ROOT_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/option/"
```
Any user or channel still in the removed group will silently fall back to 1.0× and spam error logs.

## Model ratios (legacy global)

`ModelRatio` JSON format: `{"<model-name>": <ratio>, ...}`. Example subset:
```json
{"gpt-4o": 2.5, "gpt-4o-mini": 0.15, "claude-3-5-sonnet-20241022": 3.0}
```
`CompletionRatio`: same shape, output-token multiplier.

These are applied when a channel **does not** have `model_configs` or the requested model is missing from it. Prefer per-channel pricing for new channels.

## Per-channel pricing (new preferred)

Each channel can carry its own pricing via the `model_configs` JSON string field, or via the dedicated pricing endpoints at `/api/channel/pricing/:id`.

**Shape:**
```json
{
  "gpt-4o":      {"input_ratio": 2.5, "output_ratio": 10.0},
  "gpt-4o-mini": {"input_ratio": 0.15, "output_ratio": 0.6}
}
```

Apply via dedicated endpoint (admin, not root-only):
```bash
jq -nc '{
  "gpt-4o":      {"input_ratio": 2.5, "output_ratio": 10.0},
  "gpt-4o-mini": {"input_ratio": 0.15, "output_ratio": 0.6}
}' | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/channel/pricing/42"
```

Fetch a channel's pricing:
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/channel/pricing/42" | jq .
```

Fetch system defaults (useful to seed a new channel):
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/channel/default-pricing" | jq .
```

## Migration status

If you inherited a deployment that still uses legacy `model_ratio`/`completion_ratio` fields on channels:
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/debug/channel/42/migration-status" \
  | jq .
# migration_status: "unknown" | "empty" | "needs_migration" | "migrated" | "migrated_with_legacy"
```

Bulk migrate:
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  -X POST "$ONEAPI_BASE_URL/api/debug/channels/remigrate"
```
Both are admin routes under `/api/debug/` ([router/api.go:114](../../../../router/api.go#L114)). Run in a maintenance window — remigrate touches every channel.

## QuotaPerUnit

Converts internal units ↔ USD. Default 500000 (→ $1 = 500000 units). Also in `Option`:
```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/option/" \
  | jq -r '.data[] | select(.key=="QuotaPerUnit") | .value'
```
Changing `QuotaPerUnit` re-denominates **every existing balance and token quota** — a very rare, very consequential change. Never touch without explicit, written, cross-team authorization.

## Pitfalls

- **`/api/option/` is root-only.** Admin calls get 403. Ratio changes need a root token.
- **Ratio option values are JSON-encoded strings.** When you `PUT`, the `value` field must be the stringified JSON, not an object. `jq -c | --arg v "$NEW"` is the safest pattern.
- **No atomic compare-and-swap.** Between read-modify-write, another admin's change can be lost. Announce changes in the team channel first.
- **Group ratio lookup falls back to 1.0 silently** when the group name is missing — no error, no log. The only signal is unexpected billing.
- **`model_configs` JSON is the source of truth for new deployments**, but the pricing engine still reads legacy `ModelRatio` as fallback for models not present in `model_configs`. Keep both consistent during migrations.
- **Updating `GroupRatio` does NOT retroactively adjust already-charged logs.** Past bills stay at the old ratio — correct.
- **Pricing changes take effect on the next request** (<1s latency from reload). There is no "schedule for midnight" feature — announce downtime windows to users.
