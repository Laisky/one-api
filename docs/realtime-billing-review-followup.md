# Realtime billing review follow-up

This note records the review follow-up for PR #402. The original reviewed revision is `6138a94f9749312a9c31c7c2ebdcbea758b04235`. The existing API transport limitations in [the billing audit](realtime-billing-audit.md) still apply; this work does not add provider invoice reconciliation, direct WebRTC/SIP metering, or durable crash recovery.

## Review decisions

| Finding | Decision and change | Regression evidence |
| --- | --- | --- |
| Unbounded receipts and identity maps | Confirmed. Bound receipts and input items to 4,096, pending responses to 1,024, and retained identity/model strings to 256 bytes. Stop the connection at capacity rather than evicting deduplication state and silently continuing unmetered. Keep the last accepted receipt billable. | Receipt/item/pending floods and a loopback WebSocket test exercise the actual collector and transport. |
| Full ledger persisted into a TEXT metadata column | Confirmed. Persist at most eight receipt samples, counts and an ordered-receipt SHA-256 digest. Bound diagnostic strings and enforce a 24 KiB serialized receipt-metadata budget. This is bounded audit evidence, not a complete replay journal. | Controller metadata tests exercise serialization and size limits without changing the database schema. |
| Missing transcription content index defaults to zero | Confirmed. Decode presence explicitly. Reject omitted/null, negative and fractional indexes; accept an explicit zero. Invalid events cannot occupy the valid item/index deduplication key. | `TestReviewTranscriptionIndexPresence` includes rejection and valid-receipt controls. CI also runs that same regression against the original reviewed revision and requires an assertion failure, not a build/setup failure. |
| Cached modality rejected solely because a model lacks a family supplement | Confirmed. Use resolved modality pricing, including channel image pricing. Preserve independently priceable portions and distinguish missing metadata from intentional free pricing. | The real quota resolver is tested with `private-realtime-image`, cached image usage, and mixed input/output. |
| Mixed scalar cache count without modality split discards the entire receipt | Confirmed, but the proposed blanket full-rate billing is not accepted. Preserve input/output and the known cache count. Calculate a conservative lower bound over feasible cache allocations and explicitly flag reconciliation. Do not present an inferred split as provider evidence. Fully cached mixed input and single-modality scalar caches remain exactly inferable. | Collector, quota and settlement tests verify that output charges survive, known cached tokens are not silently billed as uncached, and calculation does not mutate usage. |
| Raw test assertions instead of testify/require | Valid repository-convention issue, not a P1 financial defect. Convert assertions without weakening their behavioral expectations. | The core package and fuzz invariants use `github.com/stretchr/testify/require`. |
| Dynamic errors fail err113 | Valid lint and error-classification issue. Wrap static sentinels with `github.com/Laisky/errors/v2` and retain bounded diagnostic text. | Sentinel classification tests, package vet, and a focused err113 CI step. The focused linter is not a substitute for the repository's other lint checks. |
| Checkout leaves credentials available to PR tests | Confirmed through a failing post-checkout configuration assertion. Set `persist-credentials: false` and keep the assertion before executing PR code. The check prints no credential names or values. | The test-only workflow fails before setup/tests when credentials remain; the corrected checkout must satisfy the same assertion. |

## Additional defects found during acceptance

The expanded settlement initially changed the legacy no-ledger path and caused existing controller tests to execute database writes without a database fixture. `cac6aa058df0b451c3136ea111097015241071ee` restores the legacy path's previous behavior. Explicit OpenAI ledgers still use persisted measured or labeled-estimate settlement. This is not a blanket change to every provider's no-usage policy.

A second behavior test found an ordering regression: a malformed transcription replay arriving after a valid final receipt recreated pending work and restored the reservation. The test-only revision `512c18b4d9b53a78728a90834e1a1a49030dfc57` reproduces the failure. `b1e0ed0c5e4b2731b45611c8b113c9922af6a1e9` fixes it: binding already tracks pending work, so rejecting an unidentifiable replay must not reopen an item that has completed.

`TestReviewMalformedTranscriptionReplayDoesNotRestoreReservation` covers missing, null and negative indexes in both arrival orders. It verifies that the legitimate half-second Whisper receipt costs 25 quota units, does not restore a 999-unit reservation, remains deduplicated, and retains the malformed-event diagnostic.

A genuine missing/unpriceable receipt remains different from a cache-pricing uncertainty or an idle session. Only the former can retain an explicitly labeled reservation estimate. Authoritative idle and zero-usage sessions do not acquire a minimum fee.

## Execution evidence and limits

- [Test-only baseline run 34409465742](https://github.com/Laisky/one-api/actions/runs/34409465742), revision `5fa0a9b65b326f4793aa4eef0ac57f1edb16c221`: receipt/item retention assertions fail; the private-model cached-image receipt is discarded; the mixed-cache receipt produces a zero charge. These are runtime assertion failures, not compiler errors.
- [Reverse-order replay reproduction](https://github.com/Laisky/one-api/actions/runs/34412988175), revision `512c18b4d9b53a78728a90834e1a1a49030dfc57`: the controller regression fails for malformed events after a valid completion.
- [Post-fix regression run 34413412899](https://github.com/Laisky/one-api/actions/runs/34413412899), revision `b1e0ed0c5e4b2731b45611c8b113c9922af6a1e9`: the reviewer behavior regressions pass; full race-enabled package suites and vet pass for `relay/realtime`, `relay/quota`, and `relay/adaptor/openai`.
- [Credential assertion reproduction](https://github.com/Laisky/one-api/actions/runs/34413542004): the checkout assertion fails before PR code is executed. The subsequent workflow change disables credential persistence.

The focused workflow runs `go test -race -count=1` and `go vet` for each of the four affected packages, including `controller`. The current PR checks are authoritative for the latest head; a passing targeted test is not evidence that the full controller suite, all repository packages, every configured linter, or the frontend build passed. Read those separate checks before merging. No paid upstream API session, live database billing experiment, or provider-invoice comparison was performed in this follow-up.
