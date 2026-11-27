/usr/bin/zsh -s <<'DEPLOY'
source /home/laisky/.zshrc >/dev/null 2>&1 || exit $?
set -eu
cd /home/laisky/repo/VPS

# An inherited value must not outrank the private dotenv used by the edge.
op run --env-file=one-api/b1.env -- env -u EDGE_TOKEN /bin/bash -s <<'OTLP'
set +x
set -eu -o pipefail

compose=(docker compose --compatibility
  --env-file /opt/configs/observability/.env
  -f b1-docker-compose.yml -f observability/one-api.override.yml)
edge=b1.snake-carp.ts.net
port=14319
ip=100.122.41.16
ca=/opt/configs/observability/secrets/ca.crt
ca_target=/run/observability/ca.crt
url="https://$edge:$port"

preflight_failed() {
  echo 'one-api deployment blocked: OTLP authentication preflight failed' >&2
  exit 1
}

# Inspect the actual candidate service, not an independently parsed dotenv.
# Compose can print secrets in its rendered model; keep it in the pipe only.
token=$("${compose[@]}" config --format json oneapi 2>/dev/null |
  jq -er --arg url "$url" --arg host "$edge" --arg ip "$ip"     --arg ca "$ca" --arg target "$ca_target" '
    .services.oneapi |
    select(.environment.OTEL_EXPORTER_OTLP_ENDPOINT == $url and
           .environment.OTEL_EXPORTER_OTLP_INSECURE == "false" and
           .environment.OTEL_EXPORTER_OTLP_CERTIFICATE == $target and
           ([.extra_hosts[] | select(startswith($host + "="))] == [$host + "=" + $ip]) and
           any(.volumes[]; .type == "bind" and .source == $ca and
               .target == $target and .read_only == true) and
           (.environment | keys | all(
             (startswith("OTEL_EXPORTER_OTLP_") | not)
             or IN("OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_INSECURE",
                   "OTEL_EXPORTER_OTLP_CERTIFICATE", "OTEL_EXPORTER_OTLP_HEADERS")
           ))) |
    .environment.OTEL_EXPORTER_OTLP_HEADERS |
    capture("^Authorization=Bearer%20(?<token>[A-Za-z0-9._~-]+)$").token
  ' 2>/dev/null) || preflight_failed

# Match the app's CA, SNI and host mapping. No redirect or proxy, no body
# recording, and no credential in curl argv. With no Content-Type, the
# receiver responds after auth but before admitting or writing telemetry.
probe=(curl --disable --silent --noproxy '*' --max-time 5
  --cacert "$ca" --resolve "$edge:$port:$ip" --output /dev/null
  --write-out '%{http_code}' --request POST --header 'Content-Length: 0'
  "$url/v1/metrics")
status=$("${probe[@]}") || preflight_failed
[[ "$status" == 401 ]] || preflight_failed
status=$(printf 'Authorization: Bearer %s\n' "$token" |
  "${probe[@]}" --header @-) || preflight_failed
[[ "$status" == 415 ]] || preflight_failed

echo 'OTLP authentication preflight passed'
docker pull ppcelery/one-api:latest
# Keep the validated token even if the dotenv changes between config and up.
EDGE_TOKEN="$token" "${compose[@]}" up -d --no-deps --force-recreate --timeout 420 oneapi
OTLP
DEPLOY
