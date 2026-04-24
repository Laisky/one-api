# Investigate a user's quota overrun / token abuse

User reports: "my quota is gone already" or "why is my bill so high?" Walk the evidence from user → tokens → logs → channels.

## Prerequisites

- Admin token (`role >= 10`).
- The username or email of the subject.
- A time window in mind (usually "last 7d" or "since last top-up").

## Walkthrough

- [ ] **1. Find the user**:
  ```bash
  curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    "$ONEAPI_BASE_URL/api/user/search?keyword=alice" \
    | jq '.data[] | {id, username, email, role, status, group, quota, used_quota}'
  UID=17
  ```

- [ ] **2. Check quota state and convert to dollars:**
  ```bash
  QPU=$(curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    "$ONEAPI_BASE_URL/api/option/" \
    | jq -r '.data[] | select(.key=="QuotaPerUnit") | .value')

  curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    "$ONEAPI_BASE_URL/api/user/$UID" \
    | jq --argjson qpu "$QPU" '{
        id, username, group, status,
        quota_units: .quota, quota_usd: (.quota/$qpu),
        used_units: .used_quota, used_usd: (.used_quota/$qpu),
        request_count
      }'
  ```

- [ ] **3. List their tokens** (admin read endpoint):
  ```bash
  curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    "$ONEAPI_BASE_URL/api/admin/tokens/?user_id=$UID&size=100" \
    | jq '.data[] | {id, name, status, unlimited_quota, remain_quota, used_quota, subnet, models, expired_time}'
  ```
  Look for: high `used_quota` on one token (the leak), `unlimited_quota: true` (misissued), suspicious `subnet` (too permissive / empty).

- [ ] **4. Narrow the time window to suspected abuse:**
  ```bash
  START=$(date -d '7 days ago' +%s)
  END=$(date +%s)
  ```

- [ ] **5. Per-day usage shape** (spot bursts):
  ```bash
  curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    --data-urlencode "username=alice" \
    --data-urlencode "start_timestamp=$START" \
    --data-urlencode "end_timestamp=$END" \
    --data-urlencode "type=2" \
    --data-urlencode "size=1000" \
    -G "$ONEAPI_BASE_URL/api/log/" \
    | jq --argjson qpu "$QPU" '
        [.data[] | {day: (.created_at/1000/86400 | floor * 86400), quota, tokens: (.prompt_tokens + .completion_tokens)}]
        | group_by(.day)
        | map({day: (.[0].day | strftime("%Y-%m-%d")), requests: length, tokens: (map(.tokens)|add), usd: ((map(.quota)|add)/$qpu)})
      '
  ```
  Look for a day with 10-100× the baseline — that's the incident.

- [ ] **6. Break down by model and token for that day:**
  ```bash
  DAY_START=$(date -d 'yesterday 00:00' +%s)
  DAY_END=$(date -d 'today 00:00' +%s)

  curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    --data-urlencode "username=alice" \
    --data-urlencode "start_timestamp=$DAY_START" \
    --data-urlencode "end_timestamp=$DAY_END" \
    --data-urlencode "type=2" \
    --data-urlencode "size=1000" \
    -G "$ONEAPI_BASE_URL/api/log/" \
    | jq --argjson qpu "$QPU" '
        [.data[] | {token: .token_name, model: .model_name, quota, channel: .channel_id, tokens: (.prompt_tokens + .completion_tokens)}]
        | group_by([.token, .model])
        | map({token: .[0].token, model: .[0].model, calls: length, tokens: (map(.tokens)|add), usd: ((map(.quota)|add)/$qpu)})
        | sort_by(-.usd)
      '
  ```

- [ ] **7. Inspect specific anomalous requests** — look for abnormally large prompts (context stuffing) or suspicious model choices:
  ```bash
  curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    --data-urlencode "username=alice" \
    --data-urlencode "start_timestamp=$DAY_START" \
    --data-urlencode "end_timestamp=$DAY_END" \
    --data-urlencode "size=50" \
    --data-urlencode "sort=quota" \
    --data-urlencode "order=desc" \
    -G "$ONEAPI_BASE_URL/api/log/" \
    | jq '.data[] | {created_at, token_name, model_name, prompt_tokens, completion_tokens, quota, trace_id, request_id}'
  ```

- [ ] **8. Trace a specific suspicious call** for timing / upstream attribution:
  ```bash
  curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
    "$ONEAPI_BASE_URL/api/trace/log/<LOG_ID>" | jq .
  ```

## Take action

Pick the smallest hammer:

- **Ask user to disable the offending token** via support (preferred — user-owned credential).
- **Disable the user entirely** if abuse is ongoing and you can't reach them:
  ```bash
  jq -nc --arg u alice '{username: $u, action: "disable"}' \
    | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -X POST -d @- "$ONEAPI_BASE_URL/api/user/manage"
  ```
- **Tighten subnet or models list** on the token — admins can't, but can tell the user what to restrict.
- **Adjust the user's group** to a ratio-capped group (e.g. `"throttled"`) — prevents repeat:
  ```bash
  jq -nc --argjson id "$UID" '{id: $id, group: "throttled"}' \
    | curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        -X PUT -d @- "$ONEAPI_BASE_URL/api/user/"
  ```
  (Requires `"throttled"` group to exist with an inflated ratio — see [groups-and-ratios.md](../references/groups-and-ratios.md).)
- **Compensate legitimate users** affected by the abuse via `/api/topup` with a written remark.

## Document the outcome

Write down: user id, token id (if identified), incident time window, estimated loss in USD, action taken, follow-up (e.g. key rotation). This lives in your team's incident log — not an API call.

## Pitfalls

- **Username substring match** in log filter means `alice` can match `alice-bot`. Cross-check with user id in the log output.
- **Streaming calls report tokens after completion** — a still-running megaburst won't show up until it finishes.
- **`content` field can be large** on failed responses. `jq` may be slow for thousands of rows — filter first with time window and specific token name.
- **Group change doesn't affect in-flight requests.** Apply before disabling, so the user sees consistent behavior.
