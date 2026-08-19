#!/usr/bin/env bash
# one-api admin helper library. Source this in custom scripts.
#
#   source "$(dirname "$0")/lib.sh"
#
# Requires: curl, jq. Reads ONEAPI_BASE_URL and ONEAPI_ADMIN_TOKEN from env.

set -euo pipefail

oneapi_require_env() {
  local missing=()
  [[ -n "${ONEAPI_BASE_URL:-}" ]] || missing+=(ONEAPI_BASE_URL)
  [[ -n "${ONEAPI_ADMIN_TOKEN:-}" ]] || missing+=(ONEAPI_ADMIN_TOKEN)
  if (( ${#missing[@]} > 0 )); then
    printf 'error: missing env vars: %s\n' "${missing[*]}" >&2
    printf 'see docs/skills/admin/references/auth.md for setup\n' >&2
    return 1
  fi
  command -v jq >/dev/null || { echo "error: jq not found on PATH" >&2; return 1; }
  command -v curl >/dev/null || { echo "error: curl not found on PATH" >&2; return 1; }
}

# Mask the token for error traces. Never echo the raw token.
_oneapi_mask() {
  local t="${ONEAPI_ADMIN_TOKEN:-}"
  if (( ${#t} > 8 )); then printf '%s...' "${t:0:4}"; else printf '****'; fi
}

# Core request helper. Handles auth header, HTTP error, and envelope unwrap.
# Usage: oneapi_request GET /api/channel/ [curl extra args...]
# Outputs: `.data` on success; non-zero exit on any error (HTTP or app-level).
oneapi_request() {
  oneapi_require_env
  local method="$1" path="$2"; shift 2
  local url="${ONEAPI_BASE_URL%/}${path}"

  local resp http
  http=$(curl -sS -o /tmp/.oneapi-resp.$$ -w '%{http_code}' \
    -H "Authorization: ${ONEAPI_ADMIN_TOKEN}" \
    -X "$method" "$@" "$url") || {
      echo "error: curl network failure for $method $path" >&2
      rm -f /tmp/.oneapi-resp.$$
      return 1
    }
  resp=$(cat /tmp/.oneapi-resp.$$)
  rm -f /tmp/.oneapi-resp.$$

  if [[ "$http" -ge 400 ]]; then
    printf 'error: HTTP %s for %s %s (token=%s)\nresponse: %s\n' \
      "$http" "$method" "$path" "$(_oneapi_mask)" "$resp" >&2
    return 1
  fi

  if ! jq -e '.success' <<<"$resp" >/dev/null 2>&1; then
    printf 'error: app-level failure for %s %s\nmessage: %s\n' \
      "$method" "$path" "$(jq -r '.message // "unknown"' <<<"$resp")" >&2
    return 2
  fi

  echo "$resp"
}

oneapi_get() {
  local path="$1"; shift
  oneapi_request GET "$path" "$@"
}

oneapi_post() {
  local path="$1" body="$2"; shift 2
  oneapi_request POST "$path" -H 'Content-Type: application/json' -d "$body" "$@"
}

oneapi_put() {
  local path="$1" body="$2"; shift 2
  oneapi_request PUT "$path" -H 'Content-Type: application/json' -d "$body" "$@"
}

oneapi_delete() {
  local path="$1"; shift
  oneapi_request DELETE "$path" "$@"
}

# Paginate through a list endpoint, printing each .data item as a JSON line.
# Usage: oneapi_paginate /api/channel/ [extra query string without leading ?]
# Honors ONEAPI_PAGE_SIZE (default 100).
oneapi_paginate() {
  local path="$1" extra="${2:-}"
  local size="${ONEAPI_PAGE_SIZE:-100}"
  local page=0 total=-1 fetched=0 sep='?'
  [[ "$path" == *'?'* ]] && sep='&'

  while : ; do
    local url="${path}${sep}p=${page}&size=${size}"
    [[ -n "$extra" ]] && url="${url}&${extra}"
    local resp
    resp=$(oneapi_request GET "$url") || return $?
    total=$(jq -r '.total // 0' <<<"$resp")
    local count
    count=$(jq -r '.data | length' <<<"$resp")
    (( count == 0 )) && break
    jq -c '.data[]' <<<"$resp"
    fetched=$(( fetched + count ))
    (( fetched >= total )) && break
    page=$(( page + 1 ))
    oneapi_throttle
  done
}

# 100ms sleep between requests when looping. Override with ONEAPI_THROTTLE_MS.
oneapi_throttle() {
  local ms="${ONEAPI_THROTTLE_MS:-100}"
  # busybox `sleep` supports fractional seconds on most modern systems
  sleep "$(awk -v m="$ms" 'BEGIN{printf "%.3f", m/1000}')"
}

# Ask the user for y/N confirmation. Auto-yes if ONEAPI_ASSUME_YES=1.
# Usage: oneapi_confirm "delete channel 42?"
oneapi_confirm() {
  local prompt="$1"
  if [[ "${ONEAPI_ASSUME_YES:-0}" == "1" ]]; then
    return 0
  fi
  if [[ ! -t 0 ]]; then
    echo "error: confirmation required but stdin is not a tty — pass --yes or set ONEAPI_ASSUME_YES=1" >&2
    return 1
  fi
  local reply
  read -r -p "$prompt [y/N] " reply
  [[ "$reply" =~ ^[yY]([eE][sS])?$ ]]
}

# Convert quota units to USD using QuotaPerUnit from /api/option/.
# Usage: echo 1500000 | oneapi_to_usd
oneapi_to_usd() {
  local qpu
  qpu=$(oneapi_get /api/option/ | jq -r '.data[] | select(.key=="QuotaPerUnit") | .value')
  awk -v q="$qpu" '{printf "%.4f\n", $1 / q}'
}

oneapi_role_of_self() {
  oneapi_get /api/user/self | jq -r '.data.role'
}
