# Rotate a compromised / expiring channel key

Swap a channel's upstream API key with zero downtime and no billing loss.

**Scenario:** key leaked, or provider forces rotation, or you want to burn-down an old key.

## Prerequisites

- You have the new key ready (already issued by upstream).
- You can reach `ONEAPI_BASE_URL` with `ONEAPI_ADMIN_TOKEN` (role ≥ 10).
- You know the channel id (`scripts/oneapi channel list` or `GET /api/channel/search`).

## Checklist

- [ ] **1. Snapshot the current channel** — critical rollback material:
  ```bash
  CHANNEL_ID=42
  curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    "$ONEAPI_BASE_URL/api/channel/$CHANNEL_ID" \
    | tee /tmp/channel-$CHANNEL_ID-before.json \
    | jq '{id, name, type, status, key: "REDACTED", group, priority, weight}'
  ```

- [ ] **2. Verify the new key independently before touching one-api.** Run one manual request against upstream (OpenAI: `curl -H "Authorization: Bearer $NEW_KEY" https://api.openai.com/v1/models`). Do not proceed on a key you haven't smoke-tested outside one-api.

- [ ] **3. Update the channel key** (priority and status unchanged):
  ```bash
  jq -nc --argjson id "$CHANNEL_ID" --arg key "$NEW_KEY" '{id: $id, key: $key}' \
    | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -X PUT -d @- "$ONEAPI_BASE_URL/api/channel/" \
    | jq '{success, message}'
  ```
  `PUT` is atomic. The channel continues serving — the next request uses the new key.

- [ ] **4. Immediately test:**
  ```bash
  curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    "$ONEAPI_BASE_URL/api/channel/test/$CHANNEL_ID" \
    | jq '{success, message, time}'
  ```
  Red: `.success: false` → step 6 rollback.

- [ ] **5. Watch live traffic for 2-5 minutes:**
  ```bash
  curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    --data-urlencode "channel=$CHANNEL_ID" \
    --data-urlencode "start_timestamp=$(date -d '5 minutes ago' +%s)" \
    --data-urlencode "end_timestamp=$(date +%s)" \
    -G "$ONEAPI_BASE_URL/api/log/" \
    | jq '.data[] | select(.content != null and .content != "") | {created_at, model_name, content}'
  ```
  Any auth-related error in `.content` (e.g. `invalid_api_key`, `401`) → rollback.

- [ ] **6. Revoke the old key at the upstream provider.** This is outside one-api — do it in the provider's console (OpenAI dashboard, Azure portal, etc). Only now is the old key truly dead.

## Rollback

If step 4 or 5 fails:

```bash
# Restore from the snapshot (paste the old key from step 1's /tmp file — NOT committed anywhere)
OLD_KEY=<the old key you still have because you made a snapshot>
jq -nc --argjson id "$CHANNEL_ID" --arg key "$OLD_KEY" '{id: $id, key: $key}' \
  | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/channel/"
```
Verify with another `/test/:id`. Only then investigate the new-key issue.

## Round-robin key rotation

If the channel already uses comma-separated keys (`key1,key2,key3`), you can roll them one at a time by replacing one key in the list:

```bash
OLD_CSV=$(jq -r '.data.key' /tmp/channel-$CHANNEL_ID-before.json)  # "k1,k2,k3"
NEW_CSV=$(echo "$OLD_CSV" | tr , '\n' | sed "s|^k2\$|k2_new|" | paste -sd,)
jq -nc --argjson id "$CHANNEL_ID" --arg key "$NEW_CSV" '{id: $id, key: $key}' \
  | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/channel/"
```
This changes only one slot at a time, so 2/3 of traffic keeps using known-good keys while the new one is validated by real traffic.

## Pitfalls

- **Do not delete-and-recreate as a shortcut.** That loses the channel id, breaks historical log joins, and causes a routing gap during the interval.
- **Keys in the `key` field are stored in plaintext (encrypted-at-rest via DB, not app-level).** Assume a leak scenario means you must rotate at the provider, not just one-api.
- **If the channel serves multiple groups, rotating doesn't partition them** — the key change applies to everything the channel routes.
- **Never paste the old or new key into logs, chat, or commit messages.** Use shell history control (`HISTCONTROL=ignorespace` + leading space).
