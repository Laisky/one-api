# PR #589 current-main integration and regression qualification

Date: 2026-10-21 UTC. Original task: **20261016-2010**. Implementation lane: **20261019-0253-deepseek-tools**.

Tracking: [issue #592](https://github.com/Laisky/one-api/issues/592), [PR #589](https://github.com/Laisky/one-api/pull/589). The requested PR #582 is already merged; #592 identifies #589 as the remaining integration and release handoff. This record extends the delivered containment rather than recreating #582 or the HTTP implementation.

## Source identity

| Checkpoint | Identity |
| --- | --- |
| Previously qualified PR head | `e2740d6bdafe426b472c5f2c6038f60f7a54fcac` |
| Current main integrated here | `a53b80265330c8960c87adff3f3d2f204a95d10a` |
| Original PR common ancestor | `f4f26683404611c2313594e98e70ff5b36647dfd` |
| GitHub synthetic integrated baseline | `b6d1227286905d863914da6385c7c02a4c0b37b9` |
| Baseline integrated tree | `2e6cd6c8ec64b15f8169671ed4acba218f7ae97f` |
| Integration commit, PR head first parent | `e4792dffc505055bd31a401ef152d8def0d54e75` |
| WebSocket admission and behavioral regressions | `37f7c977832d8fd1cb65712f09900ddd70469590` |
| Model precision fix and regressions | `a313fd6919351ff83a74cd3798234992ccb7f912` |
| Qualified source tree, before documentation-only changes | `98cdb63512ecee288328a52153fa7ac4f995f050` |

Current main adds #586 cancellation logging and #590 Jev catalog/alias updates. They integrate without source conflicts. The live `main` branch endpoint was checked directly; the old PR base metadata was its common ancestor, not the current target tip.

The local index tree exactly matches the remotely created source tree above. The final publication adds documentation only. The PR and issue maintain the final published head, subsequent exact-head CI/review links, and any separately authorized release receipts; this local qualification record does not substitute for those later checks.

## Confirmed defects and shared fixes

### 1. Existing HTTP containment remains necessary

The original PR regression files were replayed against current main in an isolated worktree. The controller, OpenAI, and Anthropic receipt selections produced **41 failing subcases across 15 failing top-level tests**, while **16 control subcases passed**. This confirms that merging the current-main changes did not independently resolve the unsupported receipt/admission behavior.

On the integrated fixed source, the focused receipt selection passed **110 top-level/subtest events across five packages**, with no failures or skips. The full affected-package race run below also includes these regressions.

The existing shared `BuiltinToolQuotes` and DeepSeek receipt-contract boundary remain authoritative. Positive effective tool prices reject before HTTP reservations or dispatch; generic response items and counters do not establish paid DeepSeek tool invocations. Planned HTTP endpoint overrides, retry identity, finite-price overflow, JSON/SSE preservation, and token/cache settlement retain their original regression coverage.

### 2. Native Responses WebSocket admission bypass

The actual controller can accept an OpenAI channel whose `BaseURL` resolves to DeepSeek. Its native WebSocket path previously bypassed the HTTP tool policy and forwarded paid native tool declarations.

A real controller/socket fixture reproduced **15 rejected-frame failures** on the integrated baseline while **seven compatibility controls passed**. The test observes physical upstream frames and final user/token/request-cost records. It does not merely call a classifier.

The fix adds a transport admission callback that receives original text/binary bytes and the resolved URL actually used to dial upstream. The controller snapshots model/channel policy without retaining `gin.Context` across goroutines, then reuses the existing pricing and client discovery validators. Validation precedes ownership or model rewrites that might otherwise collapse ambiguous keys. A rejected frame receives close code **1008**, reason **`tool_billing_not_supported`**, and never reaches the upstream.

The complete **22-case suite passes with the race detector**:

- Positive quota, binary frame, finite USD overflow, inherited price, and paid Fetch.
- Duplicate/case-folded tool types, root tools, and root input keys.
- Hosted tools introduced through discovery output or developer `additional_tools`.
- A rejected turn after previously accepted work.
- Both directions of an unused HTTP endpoint override, classified by the actual WebSocket target.
- Effective zero, local functions, client tool search, local discovery/additional tools, and unrelated OpenAI controls.

The existing WebSocket handshake still opens a connection and reserves a provisional hold before receiving a frame. First-create denial restores final user/token balances. Later denial retains the prior response's **15-quota charge** and request-cost record. This is not a claim of zero WebSocket handshake activity or zero transient reservation.

The test launches a child test process to isolate `http.ProxyFromEnvironment` caching. Its owned CONNECT proxy only permits the fixture hostnames and always tunnels to an owned loopback server. No live provider is contacted. The parent's JSON test stream reports one top-level test; its logged child output contains all 22 passing subcases. These child cases are not silently added to the parent event totals below.

### 3. Numeric precision during model pinning

The existing `map[string]any` model rewrite rounded large integers and precise decimals when injecting a missing/empty model or replacing a client alias. Three behavioral numeric-preservation cases failed on the old implementation; eleven leaf controls passed.

The model guard now keeps unrelated fields as `json.RawMessage` and changes only the bound model value. The focused race selection passes **41 test/subtest events**, including existing model guards. The new regression file checks exact raw values in provider extensions, function schemas, and client discovery arguments, plus absent/empty/alias/already-bound and legacy non-string/malformed cases.

Admission itself does not rewrite frames. The model guard leaves an already-bound frame unchanged; the pre-existing ownership guard can still normalize authorized selectors and response IDs. The change preserves exact numeric values, without promising identical whitespace or field order when a model replacement is necessary.

## False positives excluded

- Initial sandbox runs could not bind localhost sockets. Those environment failures are excluded from behavioral RED evidence. The same owned loopback fixtures ran successfully with the normal approved execution path.
- An initial attempt to replay the tooling tests against main failed to compile because main did not contain the new error sentinel. That build failure is excluded; the reported main RED evidence uses the three packages that compile unchanged.
- A proposed cancellation-before-denial settlement problem is unreachable: session ownership rejects all non-`response.create` events before forwarding. No speculative settlement counter redesign was made.
- An unused HTTP full-endpoint override is not the WebSocket dispatch target. The fix follows the actual WebSocket URL and preserves existing routing rather than applying the HTTP target-selection assumptions to this transport.

## Fresh verification

Execution used Go **1.27.2 linux/amd64**, `GOTOOLCHAIN=local`, `GOMAXPROCS=4`, and test/build parallelism `-p 2`. The toolchain archive was verified against the official Go download manifest. Tests used `-count=1`; package cache hits were not treated as fresh execution.

| Scope | Result |
| --- | --- |
| Five affected packages, race enabled | **20,846 passing test/subtest events**, five passing packages, zero failures |
| New WebSocket child behavior suite | **22 passing cases**, race enabled; child output retained |
| Current-main middleware and Typesafe controls, race enabled | **483 passing test/subtest events**, two passing packages, zero failures/skips |
| `go vet ./...` | Pass, no diagnostics |
| `make lint-entity-response` | Pass, no findings |
| `make test-quick` | Pass, all **17** discovered essential top-level tests executed |
| Formatting and `git diff --check` | Clean; all six new/modified Go files independently checked |

The current-main control run covered cancellation provenance, actual TCP disconnects, operational-error preservation, Redis fallback behavior, and distinct Jev stable/preview catalog aliases. It ran before the new WebSocket patch; those control packages and the main changes remained unchanged throughout final qualification.

The full affected run reported **ten existing opt-in skips**, not passes:

| Test | Unqualified prerequisite |
| --- | --- |
| `TestDeepSeekLiveResponseFallbackAccepted` | Live-provider opt-in |
| `TestDeepSeekLiveContractRejectsUnrepairedHistory` | Live-provider opt-in |
| `TestBillingTransportAuditIntegration/mysql` and `/postgres` | External database opt-ins |
| `TestBillingTransportAuditOpaqueDialErrorRetainsHold/mysql` and `/postgres` | External database opt-ins |
| `TestBillingTransportAuditRedirectThenDialFailureRetainsHold/mysql` and `/postgres` | External database opt-ins |
| `TestBillingTransportAuditCancelledBeforeDoRefunds/mysql` and `/postgres` | External database opt-ins |

The race-event total is **1,568 top-level plus 19,278 subtest pass events**; nested parent events are not independent scenarios. The separate WebSocket child output contains its 22 leaf cases. No full `go test -race ./...`, live-provider, external database, or frontend qualification is claimed. The historical 20,828-event record at `e2740d6` remains historical and is not substituted for this run.

### Repeatable commands

Run from the repository root with the supported Go toolchain and owned test resources:

```sh
go test -race -p 2 -json -count=1 -timeout 20m \
  ./relay/controller ./relay/adaptor/openai ./relay/adaptor/anthropic \
  ./relay/adaptor/common/deepseekcompat ./relay/tooling

go test -race -p 2 -json -count=1 -timeout 10m \
  -run '^TestDeepSeekToolReceiptWebSocketAdmission$' ./relay/controller

go test -race -p 2 -json -count=1 -timeout 10m \
  -run '^TestEnforceResponseCreateModel' ./relay/adaptor/openai

go test -race -p 2 -json -count=1 -timeout 10m \
  ./middleware ./relay/adaptor/typesafe

go vet ./...
make lint-entity-response
make test-quick
git diff --check
```

For RED reproduction, apply only the named test files to the recorded baseline in a separate worktree. The WebSocket test needs the integrated baseline; the precision tests apply to its original model guard. For the original HTTP RED run, use the PR's receipt tests in controller/OpenAI/Anthropic against main `a53b802`; do not include the new tooling sentinel test in that main replay.

### Evidence checksums

Raw test output is transient execution evidence, not a committed generated test directory. The following SHA256 values identify the complete local logs used for this record; the behavioral regressions and commands are committed for reproduction.

| Evidence | SHA256 |
| --- | --- |
| Current-main HTTP RED JSON | `9cece3bbafe6e9b3164bb50c7414e287917454cc00db8fc1164670288534031b` |
| Final 22-case WebSocket RED log | `a1d6c1d0c1336fa436e3b800c5561fdfa7004614862269137c8867252edf577c` |
| Focused WebSocket GREEN JSON | `3976b078aabb1f99b670582655544db0ca26dd6ffaafe06d8ae58d25290f5a2e` |
| Model precision RED JSON | `096e70ce80299187a299fbdf26a75850fe0c78974b03d25fba827c4cba08ce32` |
| Model guard GREEN JSON | `cffbcbffc4c9bf3d95b73cdd1214777d72caf875e0fc1abd18fb8e645f1f8b25` |
| Full affected-package race JSON | `132467e54a493a8592eef029958c7dbd0d27e03a29da20b1934b5a30b1e177d6` |
| Current-main controls JSON | `034c4b8a53fd29fd0abaaaa83c43b728f5268147a311e1f56c2e0029a2b56d9b` |
| Static-gate receipt with six source hashes | `cc74bc7c25329ba27b6d2ea9ed2a28d8304a4fe9a1fc8a6751e92c3c8cb5d3cb` |

## Compatibility and review

The deliberate behavior change remains rejection of positive-priced DeepSeek native tools for which OneAPI lacks an authoritative paid-operation receipt. No tariffs, currencies, effective dates, channel configuration, schema migrations, production balances, or historical ledgers were changed. Effective-zero/default/local/client behavior and unrelated providers retain their established controls.

The implementation reuses pricing, discovery, ownership, and settlement boundaries. It does not introduce a parallel billing system, enable direct Search/Fetch settlement, or turn the upstream grace date into automatic pricing activation. The existing WebSocket resolver still uses `BaseURL`; full-endpoint override support remains separate routing work.

Independent review of the six changed/new Go files found no further implementation blocker. Its two documentation clarifications were applied: the native WS channel is specifically an OpenAI channel, and the byte-preservation guarantee is scoped to admission/model guarding rather than the whole ownership pipeline. Old automatic review results cover the old candidate only; fresh automatic review coverage must be inspected after publication.

See [DeepSeek server-tool billing compatibility](../docs/manual/deepseek_tool_billing_compatibility.md) for the user-visible contract and official provider references.

## Release boundary and rollback

The old queue behind #586 ARM64 publication is no longer current. [The #586 delivery run](https://github.com/Laisky/one-api/actions/runs/39348575942) completed successfully. The latest main [delivery run 39525224970](https://github.com/Laisky/one-api/actions/runs/39525224970) also completed its amd64 publication, deployment, and ARM64 publication jobs for `a53b80265330c8960c87adff3f3d2f204a95d10a`.

Recorded latest-main artifacts:

- amd64: `sha256:bb4c808bda504aa0de2734d06146776e0afdf282341f3dcc80693e19e05e5d4a`.
- ARM64: `sha256:742710131206e2c3ef713e17794758dc4b1b69ee9d37466fb419149dba489215`.
- Deployment workflow receipt: 2026-10-21T01:12:15Z, passed, source `a53b802`, phase `done`.

These are observed workflow receipts, not a new live-host verification or registry-configuration qualification. Before any new release, recheck the running source/artifact and retain the applicable immutable rollback image. Do not repeat an already-completed release based on the obsolete queue state.

This work updates the existing PR branch only. Merging to `main` invokes the repository's automatic image publication and production deployment, so the explicit merge/release approval boundary in #589/#592 remains in effect. A separately authorized release must verify immutable source/image identity, architecture publication, service health, public application/config/assets, and unchanged unrelated services before closing the handoff.

Source rollback requires no migration or price/configuration rollback and does not reverse historical charges. Reverting the new follow-up restores both the WebSocket admission gap and model-rewrite precision loss; retain that tradeoff in any rollback decision.
