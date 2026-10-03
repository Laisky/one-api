# Sustained-load, profile-led optimization protocol

This is the execution protocol for the next optimization cycle, following the owner's September 25 request. It does not retroactively relabel the short historical A/B trials as sustained-load profiling. Keep PR427 as the only remote delivery branch. A profile is a hypothesis-generating diagnostic; adoption requires unchanged behavior and separate unprofiled E2E evidence.

## 1. Freeze the workload and machine allowance

Record CPU model, affinity/cpuset, effective cgroup CPU quota (including tighter ancestor limits), memory limit, Go version, GOMAXPROCS for **each** process, GOGC/GOMEMLIMIT, source/build hashes, database/settings, request corpus seed, chunk size/count/pacing, arrival rate or concurrency and admission bound. Keep gateway, mock and generator CPU/RSS separate. Shared host load is not gateway utilization. A four-core quota with GOMAXPROCS=2 must not be described as four available Go execution slots.

Use a fixed closed-loop workload for CPU hotspot investigation and a bounded fixed-arrival workload for capacity/queueing investigation. Calibrate beforehand: warm caches, encoders, connections and the database, then adjust the workload until the **gateway** sustains at least 50% of the effective machine CPU allowance. Start with a 50-80% target. Do not adapt the load during the measured window or add requests merely to hit a favorable RPS. If GOMAXPROCS caps the gateway below that target, explicitly select and record a different setting before the study, or report the target as unmet. Do not substitute mock/client CPU for missing gateway load.

At one-second intervals, record cumulative process CPU, RSS, completions/errors/drops and available in-flight/queue metrics. Compute:

```
gateway_cores = delta_gateway_CPU_seconds / delta_wall_seconds
machine_utilization = gateway_cores / effective_machine_cores
gateway_slot_utilization = gateway_cores / min(effective_machine_cores, GOMAXPROCS)
```

These are different denominators. Record both, plus cgroup throttled time. Do not use host logical CPU count alone. Memory is a measured outcome, not a target to fill above 50%: intentionally filling memory could create a different GC/paging experiment.

Predeclare the stable-window gate. Initial protocol: at least 30 seconds of warm-up, followed by a 60-second observation window; at least 80% of the one-second intervals at or above 50% machine utilization, and no unexplained errors, drops or monotonic queue growth in the steady non-overload profile. Compare first/last halves of the window for drift. A failed stability gate is a diagnostic limitation, not a successful optimization or a reason to discard the run. Longer streams or larger request counts may be needed to keep the workload alive through the full window. Use explicit bounded run deadlines and clean up all children.

A saturation stress profile may intentionally build queues or shed load, but it must be labeled separately and retain failed/dropped offered requests. Do not compare only its successful requests and call that production capacity.

## 2. Capture CPU and memory in separate diagnostic runs

The application already exposes an optional separate pprof server through `ENABLE_PPROF=true` and `PPROF_LISTEN=127.0.0.1:6060`. The ordinary comparison fixture deliberately builds a restricted environment and does **not** forward these shell variables. Enable profiling explicitly in a diagnostic launcher; never silently enable it in normal `compare.sh` acceptance runs. Bind pprof only to loopback and use an isolated test environment with synthetic credentials/data. The gateway API itself currently binds `:PORT`; loopback client URLs are not proof that it is inaccessible from other interfaces. Isolate the network/firewall/container rather than exposing the fixture or profiles publicly.

After the stable warm-up, collect one diagnostic type at a time. Example commands for an already running, isolated fixture (replace only the exact local port and binary/output paths):

```sh
# CPU run: keep the fixed workload active throughout this request.
curl --fail --silent --show-error --noproxy '*' \
  'http://127.0.0.1:6060/debug/pprof/profile?seconds=60' -o cpu.pprof
go tool pprof -top ./one-api cpu.pprof
go tool pprof -top -cum ./one-api cpu.pprof

# Separate memory run: take equivalent-phase snapshots before/after its fixed window.
# Forced GC changes execution; keep it outside CPU and latency acceptance measurements.
curl --fail --silent --show-error --noproxy '*' \
  'http://127.0.0.1:6060/debug/pprof/heap?gc=1' -o heap-before.pprof
# Keep the declared workload running for the declared observation window, then:
curl --fail --silent --show-error --noproxy '*' \
  'http://127.0.0.1:6060/debug/pprof/heap?gc=1' -o heap-after.pprof
go tool pprof -top -sample_index=inuse_space ./one-api heap-after.pprof
go tool pprof -top -sample_index=alloc_space -base heap-before.pprof ./one-api heap-after.pprof
go tool pprof -top -sample_index=alloc_objects -base heap-before.pprof ./one-api heap-after.pprof
```

CPU flat time identifies directly expensive functions; cumulative time identifies expensive call paths and is not additive across ancestors. `alloc_space`/`alloc_objects` diagnose allocation churn, not live-memory savings or leaks. `inuse_space` after comparable GCs concerns retained heap; RSS includes additional runtime/OS state. Save both heap snapshots, sampling settings, GC count/pause/CPU where available, process RSS and workload counters. Negative profile deltas or sample noise need explanation, not clamping to zero. A mutex/block profile or runtime trace is a separate, explicitly enabled diagnostic for scheduling/lock questions; do not infer contention from CPU percentages alone.

References: [Go diagnostics](https://go.dev/doc/diagnostics), [net/http/pprof](https://pkg.go.dev/net/http/pprof), [pprof interpretation](https://github.com/google/pprof/blob/main/doc/README.md). Profiling has overhead; do not run competing profilers concurrently or use these runs as the unprofiled latency result.

## 3. Change one measured mechanism

For each iteration, record the dominant cost, its call path, CPU or allocation share, a precise hypothesis and the smallest candidate addressing it. Choose based on actual cost and behavior risk, not a theoretical micro-optimization. A material implementation refactor is acceptable when the evidence supports it. A microbenchmark can screen candidates but cannot certify the gateway improvement.

Before implementation, add/retain behavioral or differential tests against an independent oracle. Tokenizer changes must preserve full token ID sequences, Unicode/invalid UTF-8, special/custom patterns, timeout behavior and concurrency. Streaming changes must preserve byte framing, per-event flush, first content, cancellation, downstream errors and bounded resource cleanup. Keep exact usage/quota checks and production logging/tracing. No synthetic-only content caches, approximate counts or deferred accounting. Deliberately break the selected invariant to prove that a negative control detects it.

Do not combine a pending line-count scheduler with the accepted byte budget or restore earlier rejected candidates without a distinct predeclared hypothesis. A copied dependency is a maintenance cost: pin its upstream source/license and independent oracle.

## 4. Prove, publish, then repeat

Build control and candidate from immutable committed sources with identical toolchain, flags and configuration. Stop profilers. First run independent correctness qualification, then the registered five alternating pairs per short/long and concurrency-8/64 cell. Add a predeclared sustained validation at matched offered load when the mechanism concerns steady-state GC, memory retention or queueing. Never silently replace the historical short matrix with that different experiment.

Apply the RUNBOOK's full material-benefit, repeatability, latency, continuity and RSS gates. Preserve every adverse cell and raw request/drop/accounting record, plus source/binary/profile hashes. A reduced allocation count is not sufficient if memory or latency regresses. Publish the exact accepted production code on this PR, refresh final-head CI, and update RUNBOOK with the next dominant cost. Reprofile the new baseline: after removing one bottleneck, the next one may change.

Stop a candidate when its gates fail, retaining an unapplied patch and evidence. Stop an iteration only with its decision and recovery state recorded; do not claim a global performance optimum. Avoid spending the next session repeating already-complete evidence merely because context was lost.
