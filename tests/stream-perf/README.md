# Streaming chat E2E performance

This harness measures a **real one-api process** over loopback HTTP. Only the upstream provider is simulated. Authentication, channel selection, parsing, quota tracking, streaming, logging and durable billing stay on the production path. It is not a handler microbenchmark or a capacity claim for all deployments.

## Run

For an immutable baseline and candidate build using the same toolchain:

```sh
bash tests/stream-perf/compare.sh BASELINE_COMMIT /tmp/new-stream-study \
  --repeats 5 --concurrency 1,8,32,64
python3 tests/stream-perf/report.py /tmp/new-stream-study/results/summary.json \
  --expected-repeats 5 --json-output /tmp/new-stream-study/comparison.json
```

The script builds **committed HEAD**, not uncommitted edits, in disposable worktrees. It uses identical API-only frontend embed fixtures, preserves binary hashes and source revisions, and never overwrites an existing output directory. The JSON reporter rejects incomplete studies, missing/duplicate A/B trials, changed binaries, workload mismatch, failed/dropped traffic and unequal billing. It prints median run observations plus paired changes; the JSON comparison also retains their observed min/max ranges.

For prebuilt binaries:

Requirements: Linux with `/proc`, the repository Go toolchain, Python 3.10+, and public tokenizer dictionaries cached before starting the experiment. No paid provider, production token, Docker or third-party Python package is needed.

```sh
go build -trimpath -o /tmp/one-api-candidate .
go build -trimpath -o /tmp/stream-perf ./tests/stream-perf
python3 tests/stream-perf/cache_tokens.py --cache /tmp/stream-token-cache
python3 -m unittest discover -s tests/stream-perf -p 'test_*.py'
python3 tests/stream-perf/run.py \
  --binary /tmp/one-api-candidate --baseline /tmp/one-api-baseline \
  --driver /tmp/stream-perf --token-cache /tmp/stream-token-cache \
  --output /tmp/stream-comparison --repeats 5 --concurrency 1,8,32,64
```

Build the baseline from its immutable commit in a separate checkout with the **same Go version and build flags**. Use the candidate harness for both binaries. Do not run other load tests or builds concurrently. Each cell uses a new gateway/database, warms it up, then alternates baseline/candidate order between repetitions. Default saturated traffic is 1,024 content chunks of 128 UTF-8 bytes per response; paced traffic is 32 chunks with a 2 ms delay per chunk. Set the request counts high enough for meaningful steady-state observations; the default counts are a bounded development experiment, not a soak test.

The existing `go test ./...` CI entrypoint discovers `TestStreamingGatewayE2E`: it builds the gateway and driver and runs correctness plus a small concurrent smoke matrix. No fixed RPS threshold gates noisy shared CI runners.

## Measurement contract

- TTFT is the first **content** delta, excluding HTTP headers, role-only events and heartbeats. Report client-observed TTFT, DONE and HTTP EOF p50/p95/p99, successful requests/s and content chunks/s. Chunks are not tokenizer tokens.
- Exact request-specific Unicode content, sequence, stop, final usage and one DONE must survive through EOF. Deliberately truncated, corrupted and malformed streams must fail for the expected reason. Invalid authentication and client cancellation are tested separately.
- Gateway and mock CPU deltas and sampled peak RSS are separate; driver CPU is reported separately. RSS sampling is every 20 ms, not an exact kernel high-water mark. Missing resource data is an error, not a zero. CPU/request includes waiting for completed requests to settle in the database.
- Durable root-account request counts must match successful traffic exactly and quota consumption must increase. Baseline/candidate quota totals can be compared in JSON. Warm-up traffic is excluded. Production billing is never disabled to improve scores.
- Every client result and failure is persisted as JSON. Aggregates in `summary.json` include binary/driver SHA-256, platform, CPU affinity, cgroup limits and configuration. Compare **run-level repetitions**, not thousands of dependent requests as independent experiments. Each completed trial is checkpointed atomically. `complete: false` identifies interrupted studies; a completed subset is not a completed matrix. Use a new output directory rather than overwriting or silently resuming mixed evidence.

## Capacity and failure visibility

`--rate N` uses fixed offered-rate scheduling, a bounded worker population and **no hidden request queue**. Requests that cannot be admitted are recorded as dropped. Scheduling delay is included in `scheduled_total_ms`; successful-only latency must always be read alongside failed/dropped counts. Use `--allow-overload` only for an intentional saturation sweep; it preserves failures instead of claiming success.

`--direct` calibrates the driver plus mock without routing measured traffic through one-api. This establishes whether the benchmark apparatus has enough headroom. It still starts and warms a local gateway for a consistent fixture. Do not subtract independent p95 values and call the result gateway latency.

The fixture pins GOMAXPROCS=2 for each process and uses a fresh SQLite database, one user/token and one type-50 OpenAI-compatible channel. Default WAL, database pools, quota, tracing and logging remain enabled. Only the still-enabled global API/relay rate-limit ceilings are raised to avoid measuring a deliberately low admission policy. Redis, TLS, WAN, external reverse proxies, PostgreSQL/MySQL, multiple tenants and cross-format conversion are not included in this first profile. Report host cgroup limits as well as GOMAXPROCS.

## Safety and interpretation

Targets and the mock listener must be literal loopback HTTP addresses. Proxy settings and ambient gateway configuration are not inherited. Random fixture credentials are passed through environment variables, never command lines or reports. The harness owns and reaps only its own processes; temporary databases and normal logs are deleted. Optional failure logs are redacted. Tokenizer downloads happen during setup and are SHA-256 verified; request traffic is fully local.

Accept an optimization only after correctness passes and repeated E2E measurements show a material improvement without unacceptable latency, CPU, RSS or billing regressions. Revert unsuccessful candidates. A stop decision is scoped to the tested candidates, workload and machine; it is not proof that no future optimization exists. Raw evidence and the final measured decision belong alongside the PR before marking it ready.

## Measured reference

See [the September 24, 2026 experiment report](results/20260924/REPORT.md), the adjacent run-level CSV evidence and JSON manifest. The long-stream result is not a promise of higher throughput for short, paced streams. Hardware quotas, a single shared account, SQLite billing and run-to-run variability all matter. An unsuccessful subsequent optimization is deliberately excluded rather than hidden.
