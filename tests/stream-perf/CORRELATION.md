# Per-event SSE correlation (diagnostic only)

Use this only after a qualified CPU/trace diagnostic leaves an event-level question. It does not tune production, replace the unprofiled A/B gates, or make an instrumented build a baseline. Current results are in [the event-correlation report](results/20260926-event-correlation/REPORT.md).

## Explicit build overlay

`correlation_build.py` checks exact Git blob identities for the renderer and existing driver/protocol, then writes copies and a Go overlay into an exclusive directory. The original files are never changed. Unsupported source or nonunique anchors fail before a misleading capture. Normal gateway/driver builds do not import the observation package; no production feature flag is added.

The overlay observes one of every 32 synthetic request indices (0..8191), at most 1,024 content chunks / 2,048 total data events per request. Gateway markers contain only numeric request/frame IDs, one of three fixed stages and CLOCK_MONOTONIC nanoseconds. They record StringData entry, entry to the original Flush call and its return. Client observations occur after a complete SSE data event is read and before JSON validation. Role, finish, usage and DONE events advance the same ordinal; comments do not. Counters advance outside trace capture too. Unsupported headers are not logged. Invalid clocks and frame overflow fail explicitly.

Example from the repository root, with Go 1.27 and a previously validated tokenizer cache:

```sh
work=$(mktemp -d)
python3 tests/stream-perf/correlation_build.py --repository "$PWD" --output "$work/overlay"
go build -trimpath -buildvcs=false -overlay "$work/overlay/overlay.json" -o "$work/observed-gateway" .
go build -trimpath -buildvcs=false -overlay "$work/overlay/overlay.json" -o "$work/observed-driver" ./tests/stream-perf
go build -o "$work/trace-tool" cmd/trace
ONEAPI_TEST_CORRELATION_OVERLAY=1 go test -race -overlay "$work/overlay/overlay.json" ./tests/stream-perf/observation
python3 tests/stream-perf/profile_run.py \
  --binary "$work/observed-gateway" --driver "$work/observed-driver" \
  --token-cache "$TIKTOKEN_CACHE_DIR" --output "$work/capture" \
  --mode trace --gateway-procs 3 --auxiliary-procs 2 --concurrency 32 \
  --requests 8192 --chunks 1024 --chunk-bytes 128 \
  --warmup 30 --seconds 60 --trace-seconds 5 --trace-offset 15 --deadline 600
python3 tests/stream-perf/correlation_decode.py \
  --trace-tool "$work/trace-tool" --trace "$work/capture/runtime.trace" --output "$work/capture/events.jsonl"
python3 tests/stream-perf/correlation_analyze.py "$work/capture" --output "$work/analysis.json"
```

Do not use a captured or observed binary for acceptance throughput comparisons. The diagnostic adds clock reads, marker formatting and client timestamp storage; observer overhead has not been calibrated away. The ordinary A/B driver must remain the unmodified build. Do not overlap builds, tests or separate profiling workloads with the measured window.

## Clock and identity contract

The profiler records boot identity and per-process time namespaces. A mismatched, malformed or partly missing namespace fails; a vanished process is not treated as an old kernel. If all time-namespace APIs are absent, the output explicitly sets `absolute_cross_process_verified=false`. In that case the analyzer omits all absolute render-to-client and post-flush one-way latency. It compares only per-process intervals and their difference; a constant epoch offset cancels. This does not authorize comparing different hosts or clocks with different rates. The original capture's all-missing case is not silently upgraded to a shared epoch.

A frame is identified by request and event ordinal, not by text. All data events, complete phase triplets and adjacent content events are validated; missing trace-boundary observations are excluded, never imputed. The parser recognizes exact Go trace headers, rejects malformed/overflow markers, drops unrelated logs/stacks and limits decoded records to 128 MiB with a deadline. It records tool, trace and decoded-output hashes. A new decoder syntax must be explicitly reviewed, not guessed.

## Interpretation

For adjacent content frames, `client_gap = gateway_flush_gap + downstream_lag_change`. The difference is an interval identity, not a decomposition into network delay. Clients may parse a frame before its sender's Flush call returns. Negative values are retained rather than clipped. Flush return is not packet arrival; client parsing is not network ingress.

Scheduler/GC overlaps are computed only on the gateway trace clock between the previous flush-end marker and current render-entry marker. A trace-versus-monotonic interval discrepancy above 1 ms excludes state attribution but does not delete the client gap. Keep first/final partial runtime ranges excluded. Running/Runnable/Waiting/Syscall are mutually exclusive states; GC and mark-assist ranges may overlap these states and must not be added to them as extra time or labeled CPU cost.

Report the deterministic cohort, trace overlap, all exclusions, sample size and exceptions. A five-second trace covering a few requests is not an unbiased distribution of all users or evidence that one change will improve tails. Form a separately registered runtime hypothesis only after preserving the complete result; retain the original behavior, accounting and no-regression gates.

References: https://pkg.go.dev/net/http#Flusher ; https://pkg.go.dev/runtime/trace#Log ; https://man7.org/linux/man-pages/man7/time_namespaces.7.html


## Optional paired client trace and bounded selection

The published client-scheduling tooling adds an explicit `--client-trace` switch to the existing trace runner. Use it only with the matching diagnostic overlay; ordinary binaries do not start this listener or emit client trace markers. The driver private listener is IPv4 loopback and accepts only bounded1–10s trace captures. Default runs discard ambient `STREAM_PERF_CLIENT_TRACE_LISTEN`; both actual capture roles, hashes and complete in-window intervals must qualify.

Build as above, then add `--client-trace` to the existing profile_run command. Capture gateway and driver concurrently in the same declared subwindow. Do not start another runner or compare absolute epochs. Afterwards:

```sh
python3 tests/stream-perf/correlation_decode.py --selected-only \
  --trace-tool "$work/trace-tool" --trace "$work/capture/runtime.trace" --output "$work/capture/events.jsonl"
python3 tests/stream-perf/correlation_decode.py --selected-only \
  --trace-tool "$work/trace-tool" --trace "$work/capture/client.trace" --output "$work/capture/client-events.jsonl"
python3 tests/stream-perf/correlation_client.py "$work/capture" --output "$work/client-analysis.json"
```

Selection makes two independently bounded offline passes, first discovering marker goroutines, then retaining complete captured histories for that cohort. It preserves pre-marker states/global pauses, verifies marker identity across passes, and does not raise the128MiB output limit. Retain failed whole-trace partial output; use a new output name/directory rather than overwriting. `correlation_selection.py` audits both pass metadata and records. The helper does not prove that unsupported Go trace syntax was understood; parser changes still require explicit tests and tool pinning.

Client runtime intervals and gateway intervals are attributed only on their own trace clocks. A `Waiting`/network reason is not a claim of network causality. The complete [client scheduling report](results/20260926-client-scheduling/REPORT.md) preserves the small cohort, skew/boundary exclusions and diagnostic overhead limits. The tooling is published in PR427; this is diagnostic support, not a new runtime speedup.


### Published integration and concurrent controls

The paired tooling is published as `fcd6fb6b25a3263a398eff116a9eff1fef4a3614`, above `a57e34809322ce382ebc87f5abbe353e8b88b5bd`. Both independent client CPU slots (`7849709`) and exact observation origins/cohort replay (`a57e348`) are preserved. `--client-procs` and `--client-trace` compose without altering default2/2/2 unprofiled A/B. New captures record `window_started_elapsed`; legacy replays require every saved field to match an explicitly named origin convention, never a favorable replacement window.

The existing `correlation_cohort.py` decoder and this paired selected-pass decoder coexist. Their metadata formats differ; preserve original decoder/selection identities when auditing an old capture, and do not substitute one output or historical acceptance decision for another. `correlation_audit.py` retains the current strict `window_replay` behavior.

The merged source passes93 targeted Python tests and explicit probe/client endpoint race tests. Historical capture source and measurements predate this integration and are not relabeled as new-head results. Recheck the exact current CI; no completed workload needs repeating just to publish or restore context.
