#!/usr/bin/env bash
set -euo pipefail

# Runtime permission fix for bind-mounted /data.
# If /data is a bind mount with root-owned content, chown it (best effort) before dropping privileges.
USER_NAME=oneapi
USER_ID=${ONEAPI_UID:-10001}
GROUP_ID=${ONEAPI_GID:-10001}
APP_BIN=/one-api
DATA_DIR=/data
DEFAULT_LOG_DIR="$DATA_DIR/logs"

# Parse CLI arguments to discover custom log directory so we can prepare it
cli_args=("$@")
resolved_log_dir=""
i=0
while [ $i -lt ${#cli_args[@]} ]; do
  arg="${cli_args[$i]}"
  case "$arg" in
    --log-dir)
      next_index=$((i + 1))
      if [ $next_index -lt ${#cli_args[@]} ]; then
        resolved_log_dir="${cli_args[$next_index]}"
      fi
      ;;
    --log-dir=*)
      resolved_log_dir="${arg#--log-dir=}"
      ;;
  esac
  i=$((i + 1))
done

[ -n "$resolved_log_dir" ] || resolved_log_dir="$DEFAULT_LOG_DIR"

target_owner="$USER_ID:$GROUP_ID"

ensure_dir_owned() {
  local path="$1"
  local label="$2"

  if [ -d "$path" ]; then
    current_owner=$(stat -c %u:%g "$path" 2>/dev/null || echo "-1:-1")
    if [ "$current_owner" != "$target_owner" ]; then
      echo "Adjusting ownership of $label ($path) to $USER_NAME ($target_owner)" >&2 || true
      chown -R "$USER_ID:$GROUP_ID" "$path" || echo "Warning: could not chown $path" >&2
    fi
  else
    mkdir -p "$path" || true
    chown "$USER_ID:$GROUP_ID" "$path" || true
  fi
}

ensure_dir_owned "$DATA_DIR" "$DATA_DIR"
ensure_dir_owned "$resolved_log_dir" "log directory"

# Ensure SQLite directory exists and is owned by runtime user when SQL_DSN is unset
if [ -z "${SQL_DSN:-}" ]; then
  sqlite_path="${SQLITE_PATH:-one-api.db}"
  sqlite_dir=$(dirname "$sqlite_path")
  [ "$sqlite_dir" = "." ] && sqlite_dir="$DATA_DIR"
  ensure_dir_owned "$sqlite_dir" "sqlite directory"
fi

# Drop privileges using gosu
if [ "$(id -u)" = "0" ]; then
  exec gosu "$USER_NAME" "$APP_BIN" "$@"
else
  exec "$APP_BIN" "$@"
fi
