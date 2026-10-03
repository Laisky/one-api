# Paired-process execution-trace diagnostic (registered locally before capture)

Parent: public PR427 HEAD `327808f1de2ded4bba951d23671e14346b891ef8`.
The previous gateway-only correlation run is complete and is not repeated. The new variable is an explicit bounded client-side trace acquired alongside the existing gateway trace. No production optimization, A/B speedup or default-build instrumentation is proposed.

Freeze one independently qualified diagnostic: 8,192 requests, c32, 1,024 x128-byte synthetic chunks, no upstream pacing; gateway/mock/client GOMAXPROCS 3/2/2. Thirty-second warm-up, sixty-second observation, concurrent five-second traces at +15s inside the window. Existing v2 stable gateway-only >50% machine CPU, progress drift, delivery and ten-second exact usage gates remain. Sampling is the existing deterministic 1/32 request cohort, at most2,048 events per selected request. Maximum128MiB per trace. Actual IPv4-loopback listeners and capture identities are verified. No concurrent build/test or other load experiment runs during capture.

Only the explicit diagnostic overlay starts the client's trace listener and adds numeric `request/frame/observe/CLOCK_MONOTONIC` markers. Existing gateway marker semantics and observation timestamps remain. Normal gateway and driver sources stay byte-identical. Successful capture requires two complete independently hashed traces within the qualified window; partial or failed observations are retained but never treated as success.

Analyze each process on its own trace/monotonic clocks. Match numeric frame IDs and raw client timestamps, compare within-process inter-frame intervals, reject >1ms trace/monotonic interval skew from state attribution, preserve excluded pairs. Never infer absolute cross-process one-way latency when time-namespace identity is unavailable. Attribute only complete same-goroutine Running/Runnable/Waiting/Syscall intervals and complete GC ranges. Match both traces by frame identity, not coincident absolute clock values. Client SSE parsing is not packet ingress and Flush return is not packet egress. A client Waiting interval cannot by itself distinguish network wait from other synchronization. Scheduler/GC overlap is descriptive, not causal proof or authorization to remove fairness.

Before capture: independent unit/negative controls for the opt-in listener, exact numeric markers, bound/closed-port/duplicate-capture failure, default inertness, role separation, clock-offset invariance, incomplete/changed-frame/goroutine identities and trace-boundary exclusions. Existing correlated wire/race and stable-window tests remain. No new gateway runtime change is accepted without a separate complete unprofiled A/B.

Current session exposes no GitHub write operation; this plan is saved locally with a timestamp before collection. It is not claimed as a posted PR comment or original GitHub commit. Delivery must include an applicable patch, raw records and durable handoff; do not recreate measurements on the next interruption merely to publish them.


## Later publication boundary

The preceding text is the original locally registered plan, not the current tool permission state. Tooling was subsequently published as `fcd6fb6b25a3263a398eff116a9eff1fef4a3614` above `a57e34809322ce382ebc87f5abbe353e8b88b5bd`. Its numeric measurements and capture source remain historical and unchanged; this integration did not repeat traffic or alter acceptance rules.
