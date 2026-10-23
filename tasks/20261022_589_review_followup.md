# PR #589 review fixes — 2026-10-22

Continue issue #592 and existing PR #589; this task does not authorize merge or deployment.

## Verified RED on original head `34fbc332`

- Receipt aliases: three DeepSeek scenarios charged 33/33/39 rather than 16 token-only quota units. Canonical controls and unrelated Anthropic were correct. Physical user/token/request-cost/log records agreed.
- MCP endpoint selection: OpenAI Messages -> DeepSeek Chat incorrectly dispatched a paid native Search; DeepSeek Messages -> OpenAI Chat incorrectly rejected a request. Both same-provider controls passed.
- The owned TLS/SQLite review reproductions were preserved in the previous review evidence bundle. No live paid provider or production balances were involved.

## Existing PR branch changes

- `33005162`: share capability-name normalization between Anthropic receipt producers and tooling billing consumers.
- `cc49ad8c`: plan the real MCP execution mode/endpoint before tool admission and reservation; validate the final Chat request.
- `4eece993`: retain controller/loopback/persisted-ledger regression tests and source formatting.
- This documentation update clarifies RED evidence, selected-route semantics, and diagnostics.

## Acceptance — do not conflate historical results with this head

```sh
GOTOOLCHAIN=local go test -race -count=1 -p 2 -timeout 15m -run '^(TestDeepSeekReceiptAliasLedger|TestDeepSeekClaudeMCPEndpointAdmission)$' ./relay/controller
GOTOOLCHAIN=local go test -race -count=1 -p 2 -timeout 20m ./relay/controller ./relay/adaptor/anthropic ./relay/adaptor/openai ./relay/adaptor/common/deepseekcompat ./relay/adaptor/common/builtinname ./relay/tooling
GOTOOLCHAIN=local go vet ./...
make lint-entity-response
make test-quick
python3 .github/scripts/format_check.py
```

Use a patched Go 1.27.3 toolchain and owned test listeners. The ordinary PR CI executes a deliberately minimal unit gate and static/security checks; full affected-package race and database tests require separate qualification. Record exact-head results before marking ready.

## Compatibility and release

No schema, tariffs, currencies, auto-activation, or existing production records change. In the **historical receipt-only revision**, positive-priced DeepSeek Search/Fetch was intentionally rejected. That admission decision has been superseded by the conservative billing continuation below; unrelated providers and local tools retain their existing behavior. Source rollback restores the known admission bugs. PR remains Draft pending full fixed-head acceptance. Issue #592 retains explicit merge/release approval because main automatically deploys.

## 2026-10-23 conservative billing continuation

The product goal changed from rejecting unsupported DeepSeek paid-tool receipts to retaining native tool availability with an explicit conservative estimated charge. The earlier RED counts remain regression evidence for *accidental* paid-tool leakage, not the desired price for an admitted hosted tool. The current implementation reserves a ten-attempt floor per positively priced tool in HTTP/Claude/Chat and settles funded WebSocket creates after the proxy drains, reusing the existing ledger. Unrelated providers and unadmitted tools remain excluded from DeepSeek-specific floors.

The temporary branch-only workflow `.github/workflows/pr589-conservative-acceptance.yml` runs Go 1.27.3 `-race` for relevant controller/adapter/tooling suites. All green claims must cite the exact commit/job result. Do not merge or deploy without final acceptance and issue #592 approval. The estimated count is a commercial floor, not a proof of no undercharging when upstream private internal calls are unbounded; see `docs/manual/deepseek_tool_billing_compatibility.md`.

## Fixed-code qualification and cleanup — October 23, 2026

**Code revision:** `7d40764b82c3c79d26a346895c20848523507db7`.
The temporary branch-scoped [qualification workflow](https://github.com/Laisky/one-api/actions/runs/39900542510) completed **successfully** under Go 1.27.3, with `-race -count=1 -p 2` and no test-name filter over all tests in these nine affected packages:

```sh
go test -race -count=1 -p 2 -timeout 25m \
  ./relay/controller ./relay/tooling ./relay/adaptor/openai \
  ./relay/adaptor/anthropic ./relay/adaptor/deepseek \
  ./relay/adaptor/common/deepseekcompat ./relay/adaptor/common/builtinname \
  ./relay/quota ./relay/billing
```

The same revision also passed the isolated `TestSecurityDeferredToolNativeToConvertedRetry` `-race` test using its original 100,000-quota funded fixture. Earlier failure was a genuine double reservation introduced when deferred MCP schemas were moved before the first Claude quote; the fix preserves the old client-facing quote and funds expanded schemas in their own internal rounds. The full run installed `ffprobe`/`ffmpeg` because unrelated audio Controller tests require that prerequisite; this is an environment dependency, not an API schema change.

**Standard CI on `7d40764b`:** formatting/essential tests, Go static guardrails, vulnerability scan, and four-language CodeQL succeeded. Its aggregate required check remained red **only** because the repository's policy allows exactly `ci.yml` and `lint.yml`, while the temporary qualification workflow was present. This cleanup removes the temporary workflow and restores the original formatter script. The new exact-head required CI must be verified green independently before Ready.

The WebSocket conservative allowance is now **atomically reserved per priced `response.create`**, not merely checked against a session quota snapshot. Unit and full-controller regression include competing WebSockets, held-but-rejected frames, accepted paid work before subsequent denial, and real user/token/request-cost settlement. Upstream undocumented internal tool activity beyond the operator-facing ten-call floor cannot be proven absent; the charge is a disclosed estimate, not an exact upstream receipt.

**Release permission remains separate:** do not merge or deploy PR #589 without explicit issue #592 approval. No production call or balance was changed by these tests.
