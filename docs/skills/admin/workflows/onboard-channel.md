# Onboard a new upstream provider

Add a fresh upstream (OpenAI / Azure / Anthropic / Gemini / ...) safely, from zero to production traffic.

**Follow the checkboxes in order.** Do not skip to `status: 1` before the test passes.

## Prerequisites

- You have the upstream API key (or Azure endpoint + key + api_version, or Vertex service account JSON, etc.).
- You know the billing group: existing group from `GET /api/group/`, or you'll create one first ([groups-and-ratios.md](../references/groups-and-ratios.md)).
- You know which models this channel will serve and their current prices (have them listed).
- You have admin access (`ONEAPI_ADMIN_TOKEN` exported, role ≥ 10).

## Checklist

- [ ] **1. Inspect current state** for the target group. Know what's already serving the same models so you can compare priority:
  ```bash
  curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    "$ONEAPI_BASE_URL/api/channel/search?keyword=<model-or-provider>" \
    | jq '.data[] | {id, name, type, group, priority, status}'
  ```

- [ ] **2. Create the channel with `status: 2` (disabled).** This makes it addressable without taking traffic.
  ```bash
  jq -nc --arg key "$UPSTREAM_KEY" '{
    type: 1,                         # see references/channels.md for type ids
    name: "openai-prod-backup",
    key: $key,
    models: "gpt-4o,gpt-4o-mini",
    group: "default",
    priority: 5,                     # lower than current primary — initial fallback
    status: 2                        # START DISABLED
  }' | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -X POST -d @- "$ONEAPI_BASE_URL/api/channel/" \
    | tee /tmp/onboard-channel-resp.json \
    | jq '{success, message, id: .data.id}'
  CHANNEL_ID=$(jq -r '.data.id' /tmp/onboard-channel-resp.json)
  echo "New channel id: $CHANNEL_ID"
  ```
  If `success` is false, stop — fix the message and retry. Nothing has hit traffic yet.

- [ ] **3. Test the disabled channel.** `/test/:id` works regardless of status:
  ```bash
  curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    "$ONEAPI_BASE_URL/api/channel/test/$CHANNEL_ID" \
    | jq '{success, message, time, modelName}'
  ```
  - `success: true`, `time < 10` → proceed.
  - `success: false` → read `.message`, fix upstream config (key, base_url, api_version), retest. Do not enable until green.

- [ ] **4. Pull balance** (if provider supports it):
  ```bash
  curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    "$ONEAPI_BASE_URL/api/channel/update_balance/$CHANNEL_ID" \
    | jq '{success, balance, message}'
  ```
  `success: false, message: not supported` is fine for providers without a balance API (Anthropic, Gemini, etc).

- [ ] **5. Set pricing** (per-channel, preferred):
  ```bash
  jq -nc '{
    "gpt-4o":      {"input_ratio": 2.5, "output_ratio": 10.0},
    "gpt-4o-mini": {"input_ratio": 0.15, "output_ratio": 0.6}
  }' | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -X PUT -d @- "$ONEAPI_BASE_URL/api/channel/pricing/$CHANNEL_ID"
  ```
  Confirm with `curl ... /api/channel/pricing/$CHANNEL_ID | jq .`.

- [ ] **6. Enable with low priority.** Route a small fraction of traffic:
  ```bash
  jq -nc --argjson id "$CHANNEL_ID" '{id: $id, status: 1, priority: 1}' \
    | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -X PUT -d @- "$ONEAPI_BASE_URL/api/channel/"
  ```

- [ ] **7. Watch logs for 5-15 minutes.** Look for real traffic and no errors:
  ```bash
  curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    --data-urlencode "channel=$CHANNEL_ID" \
    --data-urlencode "start_timestamp=$(date -d '15 minutes ago' +%s)" \
    --data-urlencode "end_timestamp=$(date +%s)" \
    -G "$ONEAPI_BASE_URL/api/log/" \
    | jq '.data[] | {created_at, model_name, quota, prompt_tokens, completion_tokens, content}'
  ```
  Red flags: repeated 4xx/5xx upstream errors in `content`, zero traffic (route not firing → check `group` and `models` match).

- [ ] **8. Promote.** Raise priority to production level (match or beat existing primary):
  ```bash
  jq -nc --argjson id "$CHANNEL_ID" '{id: $id, priority: 10}' \
    | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -X PUT -d @- "$ONEAPI_BASE_URL/api/channel/"
  ```

- [ ] **9. Document in runbook.** Record channel id, name, owner, expected traffic share, rotation schedule. This is outside the API — it goes in your team's ops doc.

## Rollback

Any step fails or traffic degrades: disable immediately.
```bash
jq -nc --argjson id "$CHANNEL_ID" '{id: $id, status: 2}' \
  | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -X PUT -d @- "$ONEAPI_BASE_URL/api/channel/"
```
Delete only if the channel was misconfigured from the start and you've verified no historical logs reference it yet.
