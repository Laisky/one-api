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
python3 "$temporary/candidate/tests/stream-perf/cache_tokens.py" --cache "$output/build/token-cache"
python3 -m unittest discover -s "$temporary/candidate/tests/stream-perf" -p 'test_*.py'
sha256sum "$output/build/one-api-baseline" "$output/build/one-api-candidate" "$output/build/stream-perf" > "$output/build/sha256.txt"
python3 "$temporary/candidate/tests/stream-perf/run.py" \
  --binary "$output/build/one-api-candidate" --baseline "$output/build/one-api-baseline" \
  --driver "$output/build/stream-perf" --token-cache "$output/build/token-cache" \
  --output "$output/results" "$@"
