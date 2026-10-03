# Post-integration allocation arithmetic review

## Scope and behavior

After publishing the exactly measured bundle as `b6efcb89ddee034d33148d66d8c3d6c755056cf7` (full CI 36169997020 passed), CodeQL review comment 4107214497 identified unchecked `len(piece)+1` in the copied upstream BPE implementation. The arithmetic overflows only at the machine-int maximum; no realistic HTTP exploit was reproduced and no claim of reachable memory corruption is made.

`checkedMergePartCount` validates the sentinel-inclusive table length before addition. Invalid dimensions still panic, now before arithmetic/allocation; representable lengths, merge choices, token IDs, text handling and flush behavior are unchanged. The helper introduces no smaller input policy or general memory/CPU bound. Resource exhaustion for enormous inputs is a separate issue. The upstream maintenance note explicitly records this small divergence rather than continuing to call `bpe.go` byte-identical.

## Fresh validation

- Internal tokenizer tests, including both 16,256-input differential encodings, special/custom/finite-timeout cases, concurrent encoders and the two new bounds/allocation tests, passed under race detection.
- Renderer/SSE and focused OpenAI token/encoder/count/stream tests passed under race detection; focused vet and a fresh gateway build passed.
- Boundary tests pass actual machine-int values through the size helper without fabricating giant slices or allocating huge buffers. Removing the guard makes the expected negative control fail. The ordinary helper call performs zero measured allocations in its focused regression test; this is not an E2E speedup claim.
- Both pre-guard and guarded gateways independently passed normal/CRLF/fragmented streams, corrupt/truncated/malformed rejection, invalid authentication and eight-client cancellation cleanup.
- A separate eight-trial correctness comparison (one pair in each short/long, c8/c64 cell) completed **640 requests**, zero failures/drops, matching paired durable usage. The requested 32 requests become 128 in c64 cells because the runner enforces at least two waves; the reported total uses actual counters, not command-line counts.

This is an integration/safety check, **not a replacement five-pair performance study**. Do not pool it with the 120 prior trials, describe it as a stable 50%-load profile, or use its small timing sample to claim equivalence. The accepted performance table in REPORT.md remains attached to its original measured binary. Subsequent profiling should start from the guarded public revision and use PROFILING.md.

Build: recovered Go 1.27.1, `-p=2 -mod=vendor -trimpath -buildvcs=false`, same embed/dependency inputs. Guarded binary SHA-256: `13a1ec75315310d1ada3f9b343d63ccb3770ffcc604d4790e92a0e0344a51f3b`. Original measured/pre-guard binary: `6550a116e0d978f4ce7d5f323bc69ab83321adfe484fab15f154f404f993304b`. Driver: `7f52e6fef16f2ad4fa9ecb27d937663e18325813f60ce26d7e29b56b117fc61e`. Local integration logs and raw smoke records are delivered separately; the 640-request raw files are not committed here. Current final-head CI and CodeQL must be read from the PR checks, not inferred from pre-guard CI.
