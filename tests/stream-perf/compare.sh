#!/usr/bin/env bash
# Build two immutable revisions with one toolchain, then run the candidate's HTTP E2E harness against both.
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "Usage: bash tests/stream-perf/compare.sh BASELINE_REF NEW_OUTPUT_DIRECTORY [run.py options]" >&2
  exit 2
fi
root=$(git rev-parse --show-toplevel)
baseline=$(git rev-parse --verify --end-of-options "${1}^{commit}")
candidate=$(git rev-parse --verify HEAD)
output=$(realpath -m "$2")
shift 2
if [[ -e "$output" ]]; then
  echo "Refusing to overwrite existing experiment directory: $output" >&2
  exit 2
fi
mkdir -p "$output/build"
temporary=$(mktemp -d)
# cleanup removes only the worktrees owned by this invocation, including on failed builds.
cleanup() {
  git -C "$root" worktree remove --force "$temporary/baseline" >/dev/null 2>&1 || true
  git -C "$root" worktree remove --force "$temporary/candidate" >/dev/null 2>&1 || true
  rm -rf "$temporary"
}
trap cleanup EXIT

git -C "$root" worktree add --detach "$temporary/baseline" "$baseline"
git -C "$root" worktree add --detach "$temporary/candidate" "$candidate"
printf '%s\n' "$baseline" > "$output/build/baseline-commit.txt"
printf '%s\n' "$candidate" > "$output/build/candidate-commit.txt"
# A supplied cache is explicit offline input, not permission to download or repair assets.
# Resolve it in the caller's directory before any command enters a detached worktree.
token_cache="$output/build/token-cache"
if [[ -n "${TIKTOKEN_CACHE_DIR:-}" ]]; then
  token_cache=$(realpath -m "$TIKTOKEN_CACHE_DIR")
  python3 "$temporary/candidate/tests/stream-perf/cache_tokens.py" --cache "$token_cache" --check-only
fi
printf '%s\n' "$token_cache" > "$output/build/token-cache-path.txt"
# Pin the candidate toolchain; auto-selection must not silently build the baseline with another Go version.
toolchain=$(go env GOVERSION)
export GOTOOLCHAIN="$toolchain"
go version > "$output/build/go-version.txt"
for variant in baseline candidate; do
  (
    cd "$temporary/$variant"
    # API-only experiments use the same explicit embed fixture; no frontend traffic is measured.
    mkdir -p web/build
    printf '<!doctype html><title>API performance fixture</title>\n' > web/build/index.html
    go build -trimpath -o "$output/build/one-api-$variant" .
  )
done
(
  cd "$temporary/candidate"
  go build -trimpath -o "$output/build/stream-perf" ./tests/stream-perf
)
if [[ -z "${TIKTOKEN_CACHE_DIR:-}" ]]; then
  python3 "$temporary/candidate/tests/stream-perf/cache_tokens.py" --cache "$token_cache"
fi
python3 -m unittest discover -s "$temporary/candidate/tests/stream-perf" -p 'test_*.py'
sha256sum "$output/build/one-api-baseline" "$output/build/one-api-candidate" "$output/build/stream-perf" > "$output/build/sha256.txt"
# Managed identities are authoritative even when caller options repeat or abbreviate them.
# Put workload options first; argparse uses the last value for each managed option.
python3 "$temporary/candidate/tests/stream-perf/run.py" "$@" \
  --binary "$output/build/one-api-candidate" --baseline "$output/build/one-api-baseline" \
  --driver "$output/build/stream-perf" --token-cache "$token_cache" \
  --output "$output/results"
