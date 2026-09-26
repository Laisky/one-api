# Pinned tokenizer implementation

This package copies `github.com/pkoukk/tiktoken-go` v0.1.8, MIT licensed.
`encoding.go`, `load.go`, `tiktoken.go`, and `LICENSE` are unchanged.
`bpe.go` preserves the upstream merge algorithm; the post-integration
`checkedMergePartCount` guard rejects an unrepresentable sentinel-inclusive
allocation dimension before `int` addition, with boundary tests requiring no
huge allocations. It adds no smaller request-size limit or memory bound and
must not be described as making arbitrarily large BPE inputs safe.
`core_bpe.go` adds exact-pattern ordinary dispatch and the finite-timeout guard.
Special-token encoding, decoding, dictionary ranks and the byte-pair merge
sequence are unchanged. `canonical_ordinary.go` adds Unicode-aware canonical
pre-tokenization with synchronous cooperative scheduling after at least 4 KiB
between complete pieces. It neither bounds a large single-piece merge nor
promises a scheduling deadline.

The original dependency remains in go.mod as an independent test oracle. No
approximate count, text cache, deferred accounting, semaphore, response buffering
or altered SSE flush policy is introduced. Unsupported patterns and finite
regexp2 match-timeout settings retain the original core path. Selection checks
the captured instance timeout against the pinned unlimited sentinel, not a
mutable default. Invalid UTF-8 uses the original per-byte replacement.

Production integration `b6efcb89ddee034d33148d66d8c3d6c755056cf7` exactly matched
final local candidate `eee3e056801ae7ed45abc329be86a9d261765df6` in the audited
4 KiB study. The later arithmetic guard has its own correctness validation and
binary identity in `tests/stream-perf/results/20260925-byte-budget/INTEGRATION.md`.
Do not relabel the original measured binary as this later guarded build.
The encoder-only candidate was rejected. Do not confuse this accepted bundle
with rejected line-quantum, traversal, stack-scratch or split-only experiments.

Maintenance: preserve the upstream license; compare unchanged files and all
custom-path semantics when updating the original module; rerun differential
full-token tests against the independent dependency and full same-host E2E gates.
Unmodified upstream files retain upstream formatting and comments so exact
source verification remains possible. Do not silently remove the original oracle.

References:
- https://github.com/pkoukk/tiktoken-go/tree/v0.1.8
- https://pkg.go.dev/regexp#Compile
- https://pkg.go.dev/runtime#Gosched
- https://codeql.github.com/codeql-query-help/go/go-allocation-size-overflow/
