# Registered event-correlation diagnostic

Initial design: PR427 comment5841978479, before capture. Explicit diagnostic build overlays only, unchanged normal binaries, numeric1/32 request/frame markers, at most8192 requests/1024 chunks/2048 frames. c32,30s warm-up,60s stable window,5s trace at+15s,3/2/2 process slots. Exact behavior/qualification, same-host clocks and interval-skew checks. No A/B speedup claim.

Amendment5842057736 was made after the initial first-snapshot missing-time-namespace failure and BEFORE the captured workload. All-absent time-namespace APIs permit only per-process interval comparisons invariant to constant epoch offsets. Absolute one-way latency is forbidden; partial/different/malformed namespace data still fails. No CPU/progress/throughput/usage criterion changed.

Analysis uses complete phase triplets and adjacent content frames. Exclude state attribution when trace/monotonic interval skew exceeds1ms, preserve client observations, and report incomplete boundaries. Intersections stay on the gateway trace clock; GC/state intervals are overlapping descriptions, not additive causal components. Previous completed studies are not repeated.
