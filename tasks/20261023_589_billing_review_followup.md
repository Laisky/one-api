# 2026-10-23 maintainer follow-up — PR #589 (issue #592)

## Decision and invariant

Keep explicitly configured, positively priced DeepSeek native Search/Fetch available.
Upstream has no authoritative internal paid-operation count; OneAPI may
**conservatively overcount**, but must not silently treat uncertain accepted
work as free. The current ten-call floor per admitted capability (per forwarded
create in WebSockets) is a customer-facing commercial estimate, not a proven
upstream maximum. Do not introduce blanket tool rejection, implicit new
pricing, or a second billing system.

An allowed but unsupported/ignored tool declaration may still incur the
configured conservative commercial charge after forwarding; absence of a
visible upstream tool receipt is not proof that no billable upstream work
occurred. Zero-price, local-function, and non-DeepSeek provider behavior is
intentionally preserved.

## Reproduced behaviors and fixes

1. **WebSocket denied model switch after paid tool funding.**
   - Reproduction: `TestDeepSeekToolReceiptWebSocketAdmission/model_switch_rejected_paid`
     RED at [run 39951003914](https://github.com/Laisky/one-api/actions/runs/39951003914).
     The old proxy ran paid-tool funding before model authorization, so an
     unforwarded `response.create` could retain a held charge.
   - Fix: first authorize ownership/model on the original create, then
     atomically reserve the valid positively priced tool before forwarding.
     Mark model denials as `ownership.denied` to refund a first rejected frame.
     Earlier forwarded work stays billed once. The integration test checks
     provider-forwarded payloads and persisted token, owner, and request-cost
     ledgers.

2. **Claude converted Search combined with an independent raw server tool.**
   - Reproduction: `TestClaudeConservativeIndependentToolAllowance`, RED at
     [run 39951597319](https://github.com/Laisky/one-api/actions/runs/39951597319):
     measured required quote 686 (16 tokens + 500 other tool + 170 Search),
     while `max(rawAllowance, conservativeAllowance)` funded only 516.
   - Fix: quote native non-Search/Fetch capability amounts independently and
     add the separate DeepSeek Search/Fetch conservative allowance. Search/Fetch
     present in both raw and converted requests are deduplicated. An unrelated
     provider retains the original native tool quote.
   - Controls: `TestClaudeConservativeRawAndConvertedToolQuoteCompatibility`
     tests DeepSeek Search/Fetch overlap and non-DeepSeek unchanged prices.

3. **Concurrent WebSocket client data writers.**
   - Reproduction: the accepted-paid-before-model-denial case exposed
     `WARNING: DATA RACE` under `-race` at
     [run 39952493816](https://github.com/Laisky/one-api/actions/runs/39952493816):
     locally emitted model-denial error event raced with upstream forwarding
     on the same Gorilla WebSocket connection.
   - Fix: session-owned mutex serializes both regular client data-frame writes;
     concurrent `WriteControl` remains supported.

## Acceptance evidence

- Focused Go 1.27.3 `-race -count=1` behavioral suite (WebSocket real
  proxy/controller + both Claude quote regressions) succeeded at
  [run 39953102731](https://github.com/Laisky/one-api/actions/runs/39953102731).
- The first broader six-package `go test -race -count=1` at
  [run 39953621198](https://github.com/Laisky/one-api/actions/runs/39953621198)
  failed because its host lacked `ffprobe` and an older model-switch test
  incorrectly expected non-nil usage from a proven pre-dispatch denial.
  Neither failure reproduced a DeepSeek tool billing defect.
- Diagnostic test [39973809958](https://github.com/Laisky/one-api/actions/runs/39973809958)
  captured the JSON `model_switch_denied` error event and matching 1008 Close,
  eliminating the suspected missing-event defect. The nil usage return is
  intentional: it lets the controller fully refund unforwarded work. The
  test was corrected and speculative WS close-handshake code reverted.
- After the correction, the model-switch test passed 40 race-enabled runs at
  [39974368661](https://github.com/Laisky/one-api/actions/runs/39974368661).
- The final temporary qualification run
  [39974858770](https://github.com/Laisky/one-api/actions/runs/39974858770)
  passed: **40 repeated model-switch cases under `-race`**, and all
  **six affected packages** under Go 1.27.3
  `go test -race -count=1 -timeout=15m` with `ffmpeg/ffprobe` installed.
  Package outputs were all `ok`: relay/controller, relay/tooling,
  relay/adaptor/openai, relay/adaptor/anthropic,
  relay/adaptor/common/deepseekcompat, and relay/adaptor/common/builtinname.
  No production DeepSeek endpoint or paid account was used.
- Existing tests cover paid and free tool controls, unrelated providers, exact
  JSON numbers, retry endpoint affinity, two-socket atomic funding, accepted
  work before denial, and persisted financial ledgers.
- Normal PR CI must pass on the final head **after** the temporary
  `.github/workflows/pr589-billing-qualification.yml` has been removed.
  The repository's standard workflows are not to be expanded permanently.

Repeatable focused check:

```sh
go test -race -count=1 -run '^(TestDeepSeekToolReceiptWebSocketAdmission|TestClaudeConservativeIndependentToolAllowance|TestClaudeConservativeRawAndConvertedToolQuoteCompatibility)$' ./relay/controller
```

## Compatibility and release safety

No schema or migration changes, default vendor tariff activation, API response
rewrites, or changes to unrelated-provider billing. Even unused declarations
with explicitly configured positive prices may be overcounted by policy.
WebSocket model/ownership rejection before forwarding now refunds rather than
charging unexecuted frames; a potentially dispatched write error is still
conservatively retained.

Residual risks are intentionally documented: DeepSeek ignores
`max_tool_calls`; invisible internal calls beyond the ten-call estimate cannot
be ruled out without upstream bill reconciliation or gateway-owned Search/Fetch.
A process crash between durable WebSocket debit and final ledger reconciliation
requires the existing billing audit and recovery process.

**Do not merge or deploy without explicit approval under issue #592.**
Merging `main` builds and deploys automatically. Rollback uses the prior
immutable image and does not undo historical customer charges.

## Final workflow cleanup

The temporary branch-only `.github/workflows/pr589-billing-qualification.yml`
was deleted in the same commit as this record. Standard minimal CI retains only
`ci.yml` and `lint.yml`. Final exact-head required CI and PR approval remain
independent gates; neither a green historical run nor Ready for Review authorizes
merging into `main`, which automatically publishes and deploys.
