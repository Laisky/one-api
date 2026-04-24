# Adjust model / group ratios safely

Changing ratios moves real money. This workflow is the difference between a careful rate change and an incident ticket.

**Scope:** `/api/option/` (root-only) changes to `GroupRatio`, `ModelRatio`, `CompletionRatio`, or swapping per-channel `model_configs`.

## Prerequisites

- **Root token** (role = 100). Admin tokens get 403 on `/api/option/*`. If you're admin-only, escalate and have root run step 3.
- A change ticket / RFC describing: the ratio before, after, reason, blast radius (affected users/requests/day), rollback plan.
- Explicit in-chat user approval for the exact numeric change.

## Checklist

- [ ] **1. Snapshot current state** — the rollback artifact:
  ```bash
  curl -fsS -H "Authorization: $ONEAPI_ROOT_TOKEN" \
    "$ONEAPI_BASE_URL/api/option/" \
    | jq '.data | map(select(.key|IN("GroupRatio","ModelRatio","CompletionRatio")))' \
    > /tmp/ratios-before.json
  cat /tmp/ratios-before.json
  ```
  Do not proceed without a readable file on disk.

- [ ] **2. Compute the exact new value** (outside the server, verify locally):
  ```bash
  # Example: lower `vip` group ratio from 0.5 to 0.4
  CUR=$(jq -r '.[] | select(.key=="GroupRatio") | .value' /tmp/ratios-before.json)
  echo "Current GroupRatio: $CUR"

  NEW=$(echo "$CUR" | jq -c '.vip = 0.4')
  echo "New GroupRatio: $NEW"

  # Sanity check: valid JSON object, contains expected keys, no spurious changes
  echo "$NEW" | jq -e 'type == "object"' >/dev/null && echo "structure OK"
  diff <(echo "$CUR" | jq -S .) <(echo "$NEW" | jq -S .)    # show the diff
  ```
  Eyeball the diff. If anything other than your intended key changed, stop.

- [ ] **3. Apply** — a single `PUT /api/option/`:
  ```bash
  jq -nc --arg v "$NEW" '{key: "GroupRatio", value: $v}' \
    | curl -fsS -H "Authorization: $ONEAPI_ROOT_TOKEN" \
        -H "Content-Type: application/json" \
        -X PUT -d @- "$ONEAPI_BASE_URL/api/option/" \
    | jq '{success, message}'
  ```

- [ ] **4. Verify effect within seconds:**
  ```bash
  curl -fsS -H "Authorization: $ONEAPI_ROOT_TOKEN" \
    "$ONEAPI_BASE_URL/api/option/" \
    | jq -r '.data[] | select(.key=="GroupRatio") | .value' \
    | jq .
  ```
  Must match `$NEW`.

- [ ] **5. Spot-check billing on the next few real requests.** Filter logs for a user in the affected group; verify `quota` per request matches `input_tokens * input_ratio * group_ratio + output_tokens * output_ratio * group_ratio` (approximately — rounding):
  ```bash
  sleep 30
  curl -fsS -H "Authorization: $ONEAPI_ROOT_TOKEN" \
    --data-urlencode "start_timestamp=$(date -d '1 minute ago' +%s)" \
    --data-urlencode "end_timestamp=$(date +%s)" \
    --data-urlencode "type=2" \
    --data-urlencode "size=20" \
    -G "$ONEAPI_BASE_URL/api/log/" \
    | jq '.data[] | {username, model: .model_name, in: .prompt_tokens, out: .completion_tokens, quota}'
  ```

- [ ] **6. Announce the change** — outside the API: team channel, customer notice, incident log, whichever applies. Include before/after value and effective timestamp.

## Rollback

Restore from snapshot:
```bash
ORIGINAL=$(jq -r '.[] | select(.key=="GroupRatio") | .value' /tmp/ratios-before.json)
jq -nc --arg v "$ORIGINAL" '{key: "GroupRatio", value: $v}' \
  | curl -fsS -H "Authorization: $ONEAPI_ROOT_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/option/"
```
Verify with step 4 again.

## Adjusting per-channel pricing instead (preferred for model prices)

Per-channel `model_configs` changes don't require root and don't globally reload ratios. Scope: single channel.

```bash
CHANNEL_ID=42
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/channel/pricing/$CHANNEL_ID" > /tmp/pricing-$CHANNEL_ID-before.json

# Mutate
jq '.["gpt-4o"].input_ratio = 2.0' /tmp/pricing-$CHANNEL_ID-before.json > /tmp/pricing-$CHANNEL_ID-new.json
diff <(jq -S . /tmp/pricing-$CHANNEL_ID-before.json) <(jq -S . /tmp/pricing-$CHANNEL_ID-new.json)

# Apply
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -X PUT -d @/tmp/pricing-$CHANNEL_ID-new.json \
  "$ONEAPI_BASE_URL/api/channel/pricing/$CHANNEL_ID"

# Verify
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/channel/pricing/$CHANNEL_ID" | jq .
```

This is the right knob for "upstream dropped their price, pass it through" — affects one channel, no cross-user surprise.

## Pitfalls

- **Ratio changes take effect on the next request.** No pre-announce window. Schedule deploys during low-traffic times.
- **`GroupRatio` fallback is 1.0×** when a group is missing — `del(.vip)` silently overcharges users in that group. Never delete a group that still has assignees.
- **Legacy `ModelRatio` / `CompletionRatio` only applies when a channel's `model_configs` lacks the model.** Changing it won't affect channels that override.
- **No compare-and-swap.** Two admins editing the same option JSON in parallel → last write wins. Announce first.
- **Never PUT an object** for ratio `value` — it must be a stringified JSON. Handler uses `decode(option.Value)` then calls the per-key parser ([controller/option.go:37](../../../../controller/option.go#L37)).
- **`QuotaPerUnit` is NOT a ratio knob.** Changing it re-denominates every existing balance. Off-limits for this workflow.
