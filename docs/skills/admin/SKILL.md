---
name: oneapi-admin
description: "Operates the one-api admin REST API to manage channels (upstream LLM providers), users, API tokens, groups, and billing ratios. TRIGGER when: the user asks to add/list/test/disable a channel in one-api; create/disable/promote/quota-adjust a user; investigate another user's tokens or quota usage; change group ratios, model ratios, or per-channel pricing; rotate a leaked channel key; inspect admin-only logs. Also include terms: one-api, oneapi, 渠道, 用户, 令牌, 分组, 倍率, 模型价格, quota, upstream, provider, Azure, OpenAI channel, balance refresh. SKIP: tasks that modify application source code, non-admin user-scoped token flows, frontend work, or any request that does not name one-api explicitly and has no admin API verb."
license: See repository LICENSE
compatibility: "Requires: bash, curl, jq. Network access to the target one-api instance. Env vars ONEAPI_BASE_URL and ONEAPI_ADMIN_TOKEN must be set before any call."
---

# one-api admin

Drive the one-api admin HTTP API from the command line. Every admin operation in this repo is reachable via `/api/...` — this skill routes you to the right endpoint, gives you copy-paste `curl` + `jq` blocks, and flags money-moving ops so they don't ship by accident.

## Before you start — one-time setup

1. **Install tooling** — `curl` and `jq` must be on PATH. If missing, install with `sudo apt-get install -y curl jq` (Debian/Ubuntu) or the platform equivalent.
2. **Mint an admin access token** via the UI:
   - Log in as an admin (role ≥ 10) or root (role = 100).
   - Go to Profile → "Generate System Access Token" (calls `GET /api/user/token`).
   - The returned UUID is your `ONEAPI_ADMIN_TOKEN`. It lives in `users.access_token` and stays valid until regenerated.
3. **Export env vars** in the shell you'll work from:
   ```bash
   export ONEAPI_BASE_URL="https://your-oneapi.example.com"   # no trailing slash
   export ONEAPI_ADMIN_TOKEN="<the-uuid-from-step-2>"
   ```
4. **Verify** you can reach the API and the token belongs to an admin:
   ```bash
   curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
     "$ONEAPI_BASE_URL/api/channel/?p=0&size=1" | jq '.success, .total'
   ```
   Expect `true` and an integer. A 401 means the token is wrong or the user was demoted.

All endpoints return the envelope `{success: bool, message: string, data: ..., total?: int64}` — always branch on `.success`, not HTTP status (the server sometimes returns 200 with `success: false`). See [references/errors.md](references/errors.md).

## Quick start — list enabled channels

```bash
curl -fsS -H "Authorization: $ONEAPI_ADMIN_TOKEN" \
  "$ONEAPI_BASE_URL/api/channel/?p=0&size=20" \
  | jq '.data[] | {id, name, type, status, group, priority}'
```

The bundled [scripts/oneapi](scripts/oneapi) wraps this: `scripts/oneapi channel list`.

## Which resource am I working with?

Read the matching reference file in full before composing calls — do **not** skim. Each contains endpoint tables, request/response schemas, pitfalls, and runnable examples.

| User intent                                            | Reference                                               |
|--------------------------------------------------------|---------------------------------------------------------|
| Channels: add / list / test / enable-disable / pricing | [references/channels.md](references/channels.md)        |
| Users: CRUD / role / quota / group / enable-disable    | [references/users.md](references/users.md)              |
| Tokens: admin read-only view of any user's tokens, plus user-scoped CRUD | [references/tokens.md](references/tokens.md) |
| Groups (billing group list) + model/completion/group ratios + per-channel pricing | [references/groups-and-ratios.md](references/groups-and-ratios.md) |
| Admin log queries for usage & billing investigation    | [references/logs.md](references/logs.md)                |
| Auth, header format, role levels, token rotation       | [references/auth.md](references/auth.md)                |
| Error envelope, HTTP vs app-level errors, retry rules  | [references/errors.md](references/errors.md)            |

## Common workflows

Step-by-step guides with validation gates. Follow the checkboxes in order — they exist because skipping a step has bitten someone before.

| Workflow                                               | Guide                                                   |
|--------------------------------------------------------|---------------------------------------------------------|
| Onboard a new upstream provider end-to-end             | [workflows/onboard-channel.md](workflows/onboard-channel.md) |
| Rotate a leaked / compromised channel key safely       | [workflows/rotate-channel-key.md](workflows/rotate-channel-key.md) |
| Investigate a user's quota overrun or token abuse      | [workflows/investigate-user-quota.md](workflows/investigate-user-quota.md) |
| Adjust model/group ratios without breaking in-flight billing | [workflows/adjust-ratios-safely.md](workflows/adjust-ratios-safely.md) |

## Critical rules (read once, remember always)

1. **Auth header format is `Authorization: <uuid>`, NOT `Authorization: Bearer <uuid>`.** The server strips a `Bearer ` prefix if present ([model/user.go:410](../../../model/user.go#L410)), so Bearer-form also works — but stay consistent with the raw UUID form to match other examples in this skill.
2. **Role matters.** Admin routes (`role >= 10`) cover everything in this skill except `/api/option/` (model/group ratios, system toggles) which is **Root-only (`role == 100`)**. If a ratio PUT fails with 403, your token is admin-not-root — ask a root user to run it.
3. **Quota is a 64-bit integer in internal "quota units", NOT dollars.** 1 USD ≈ `QuotaPerUnit` units (default 500000). Always fetch `QuotaPerUnit` from `/api/option/` before converting, and present both unit and USD forms when talking to humans. A carelessly typed extra zero grants 10× intended credit.
4. **Channel `status` values:** `1=enabled`, `2=manually disabled`, `3=automatically disabled` (by health check / balance check). Setting `status=2` by PUT is the admin kill-switch. Do **not** overwrite `3` → `1` without understanding *why* it was auto-disabled (check `response_time`, `balance`, or run `GET /api/channel/test/:id`).
5. **Never delete a channel/user/token as a first step.** Disable first (`status=2` for channels; `POST /api/user/manage` with `action=disable`; or `PUT /api/token/?status_only=1`). Delete only after downstream usage has stopped — logs and billing rows reference `channel_id` / `user_id`.
6. **Ratio changes are system-wide and take effect immediately.** The server reloads option JSON on the next request. Validate your JSON with `jq -e` before PUT. Never hand-edit `ModelRatio` JSON in-place in the UI — copy-modify-paste so you have a rollback copy.
7. **Pagination is zero-indexed (`p=0` is the first page).** `size` is capped at `MaxItemsPerPage` (from config). Always read `total` from the response envelope and loop `while p*size < total`. See [scripts/lib.sh](scripts/lib.sh) `oneapi_paginate`.
8. **Use `jq` to parse responses, never `grep`/`sed`.** Channel/user fields can contain any character including commas and braces. Use `jq -r` for raw strings and `jq -e` for assertions (non-zero exit on null/false).
9. **Destructive endpoints require explicit confirmation.** Before calling `DELETE /api/channel/:id`, `DELETE /api/user/:id`, `DELETE /api/log/`, or any balance/quota grant, stop and show the resolved request to the user for approval. `scripts/oneapi` prints a confirmation prompt for delete/grant by default — do not pass `--yes` unless the user explicitly authorised the specific operation.
10. **Error wrapping (code you add to the repo):** if you write Go code to extend this skill's server-side endpoints, wrap errors with `github.com/Laisky/errors/v2` (`Wrap`/`Wrapf`/`WithStack`) — never return bare errors. Matches repo convention.

## Scripts (executable, don't read first)

Prefer these over hand-rolling `curl` — they handle pagination, auth, and response unwrapping, and emit structured stderr on failure.

- **[scripts/oneapi](scripts/oneapi)** — unified CLI. Subcommands: `channel`, `user`, `token`, `group`, `option`, `log`. Examples:
  ```bash
  scripts/oneapi channel list --enabled-only
  scripts/oneapi channel test 42               # test one channel
  scripts/oneapi user get --keyword alice
  scripts/oneapi user grant-quota 17 5000000   # add quota units (confirms first)
  scripts/oneapi option get GroupRatio | jq .  # current group ratios
  ```
  Run `scripts/oneapi --help` for the full command tree. If a subcommand you need is missing, use raw `curl` with the reference file — don't modify the script unless the user asks.

- **[scripts/lib.sh](scripts/lib.sh)** — sourced helpers for custom one-off scripts. Provides `oneapi_get`, `oneapi_post`, `oneapi_put`, `oneapi_delete`, `oneapi_paginate`, and `oneapi_confirm`.

## Common pitfalls

- **`key` field in create-channel is case-sensitive and may contain commas** to encode multi-key round-robin (`key1,key2,key3`). Encode the whole comma-string as a JSON string, no array.
- **`models` on channels and tokens is a comma-separated string**, not a JSON array. `"gpt-4o,gpt-4o-mini"`, no spaces after commas.
- **`model_configs` on channels is a JSON-encoded string** (double-escaped inside the outer JSON). Build it with `jq -c` and pass as `--arg` — do not hand-quote.
- **User creation does not return the new user id** — the response is `{success, message}`. To get the id, call `GET /api/user/search?keyword=<username>` right after.
- **`/api/user/manage` takes a `username`, not an id.** The adjacent endpoints all take ids. Read the [users reference](references/users.md) carefully.
- **Admin tokens cannot modify other users' API tokens.** Read-only admin token visibility is the supported flow ([references/tokens.md](references/tokens.md)). To cut off a user's token, disable the *user* via `/api/user/manage` `action=disable`.
- **Timestamps:** `created_at`/`updated_at` are **milliseconds**; log filter params `start_timestamp`/`end_timestamp` are **seconds**. Don't mix them.
- **Channel `test/:id` returns 200 with `success: false` when the upstream responds but returns a billing/auth error.** Always print `.message` and the `.time` field (roundtrip ms).
- **`DELETE /api/log/` wipes logs irreversibly and breaks historical billing audits.** Require explicit, recent user authorization before calling it; prefer a dated export first.
- **Empty values for sensitive option keys are no-ops.** `PUT /api/option/` ignores `value: ""` when the key has suffix `Token`, `Secret`, or `Password`; `GET /api/option/` already strips these. To rotate, send a non-empty value; to clear a non-sensitive option, an empty string does clear it.

## Known limitations

- **Admin write access to other users' tokens is intentionally absent.** The new `GET /api/admin/tokens/...` endpoints added alongside this skill are read-only. To stop a user's token, disable the owning user.
- **Groups are derived from the `GroupRatio` option JSON.** There is no `POST /api/group/` — adding a group means adding a key under `GroupRatio` via `PUT /api/option/` (root-only). See [references/groups-and-ratios.md](references/groups-and-ratios.md).
- **Global `ModelRatio` / `CompletionRatio` via `/api/option/` is the legacy format.** New pricing lives on each channel as `model_configs`. Treat option-level ratios as fallback; prefer per-channel pricing for new work.
- **No bulk endpoints for user/token.** To update N users, loop; respect the server's rate-limit middleware (default 60 req/min/ip — check `/api/option/`).

## If you need to extend the admin API

Only add server-side endpoints when a gap blocks a genuine admin workflow. When you do:
- Guard with `middleware.AdminAuth()` (role ≥ 10) or `middleware.RootAuth()` (role = 100) — pick the tighter one that still lets the job get done.
- Add the route in [router/api.go](../../../router/api.go) next to related resources.
- Place the handler in the matching `controller/*.go`. Wrap errors with `github.com/Laisky/errors/v2`.
- Keep new endpoints read-only unless the user explicitly asks for a mutation.
- Update [references/](references/) to document the new endpoint the same session you add it.

## Evaluation quick-prompts

Use these to spot-check the skill is working before real ops:

1. "List disabled channels and summarize why each was disabled (manual vs auto)."
2. "User `alice` says her quota is gone; show her current quota, recent logs, and token list."
3. "Add a new Azure OpenAI channel for gpt-4o, test it, and attach it to the `vip` group."
4. "Lower the `vip` group ratio from 0.5 to 0.4; show the before/after JSON." (root-only)
5. "Rotate the key on channel 42 with zero downtime."
