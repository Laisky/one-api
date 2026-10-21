# PR #589 current-main integration and regression qualification

Date: 2026-10-21 UTC. Original task: **20261016-2010**. Implementation lane: **20261019-0253-deepseek-tools**.

Tracking: [issue #592](https://github.com/Laisky/one-api/issues/592), [PR #589](https://github.com/Laisky/one-api/pull/589). The requested PR #582 is already merged; #592 identifies #589 as the remaining integration and release handoff. This record extends the delivered containment rather than recreating #582 or the HTTP implementation.

## Initial source identity

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
| Initial qualified source tree, before documentation-only changes | `98cdb63512ecee288328a52153fa7ac4f995f050` |
| First published follow-up head | `2c9a73d3e53ddaafeda3f2483c4b91db033117d2` |

Current main adds #586 cancellation logging and #590 Jev catalog/alias updates. They integrate without source conflicts. The live `main` branch endpoint was checked directly; the old PR base metadata was its common ancestor, not the current target tip.

The local index tree exactly matched the remotely created source tree above. The first publication added documentation only. Its required security gate subsequently identified the toolchain issue documented below. The PR and issue maintain the final published head, subsequent exact-head CI/review links, and any separately authorized release receipts; this local qualification record does not substitute for those later checks.

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

## Initial verification with Go 1.27.2

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

## Required security-gate follow-up with Go 1.27.3

The first published head's [ordinary CI run](https://github.com/Laisky/one-api/actions/runs/39615167572) correctly failed its required vulnerability gate. Its [scanner job](https://github.com/Laisky/one-api/actions/runs/39615167572/job/119274144754) reported **GO-2026-6809 / CVE-2026-111943**, with `PasskeyRegisterFinish` reaching `webauthn.FinishRegistration` and `crypto/x509.Certificate.Verify`. The pinned Go 1.27.2 standard library was affected; Go 1.27.3 contains the fix. All other mandatory jobs passed, and [four-language CodeQL](https://github.com/Laisky/one-api/actions/runs/39615167577) completed successfully on that first head. Those results do not turn the failed aggregate gate into a pass.

The [official Go advisory](https://github.com/golang/vulndb/blob/master/data/osv/GO-2026-6809.json), [release history](https://go.dev/doc/devel/release#go1.27.3), and [Go 1.27 backport](https://github.com/golang/go/commit/90617fb7ceb59cf08bee7826bb51b8c8fb8c11de) confirm the standard-library distrust change. This is a concrete newly observed required-gate finding, not a waiver or a retry of the handoff's unrelated previously restricted assessment.

### Behavioral reproduction and compatible patch selection

`controller/toolchain_x509_test.go` verifies a genuine signed public intermediate against its issuing root in an explicit certificate pool. Both certificate fingerprints and the intermediate SPKI hash are pinned; signatures are checked independently and the verification time is fixed inside both validity periods. Thus expiry, an invalid signature, missing system roots, or network failure cannot satisfy the distrust assertion by accident.

The exact same test source demonstrates:

- **RED on Go 1.27.2:** the real distrusted chain is accepted and the rejection assertion fails. The owned valid-chain and unknown-root controls pass.
- **GREEN on Go 1.27.3:** the distrusted chain returns `x509.NoValidChains`; the valid owned chain and unknown-root controls still pass. One top-level test and two subtests pass with race enabled, with no skips.

The fixture [provenance record](../controller/testdata/toolchain_x509/README.md) links the issuer's public certificates and their fingerprints. The test uses no compromised private key, mock SPKI mutation, system trust mutation, live service, or runtime network access.

The shared fix selects Go **1.27.3** in all five existing CI Go jobs, the Docker builder, and the module's preferred `toolchain`. It retains the `go 1.27.2` compatibility floor, all application dependency requirements, `go.sum`, CGO, Bookworm, and target architecture arguments. The existing workflow contract now asserts alignment across CI, developer selection, and Docker, so changing only the scanner cannot leave the release compiler behind. `GOTOOLCHAIN=local` remains an explicit override and must use a patched compiler for qualified builds.

The security follow-up is commit **`df12fdf25cbda7b0be4341d5cd66f0ab38d8f3d7`**, tree **`9b9b3f83f94880db5021b5081b7d60e7503cfd3b`**. The local index tree exactly matches this remote source tree. Its Go 1.27.3 linux/amd64 archive was verified against the official download manifest: SHA256 `65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40`. The official Docker image catalog lists `1.27.3-bookworm` for both amd64 and arm64v8; a release image build/publication remains part of the separately approved delivery.

### Final local qualification on the patched compiler

All runs used Go 1.27.3 explicitly, `GOTOOLCHAIN=local`, and fresh `-count=1` execution. The new compiler justified repeating the affected and current-main controls; the original Go 1.27.2 receipts above retain their actual provenance.

| Scope | Result |
| --- | --- |
| Five affected packages plus middleware and Typesafe, race enabled | **21,329 passing events**, seven packages, zero failures; the same ten opt-in skips |
| Event accounting | **1,706 top-level + 19,623 subtest** pass events; the 22 WebSocket child cases also pass and remain separately visible in child output |
| Existing passkey response control plus new X.509 trust test, race enabled | **Four passing events**, zero failures/skips |
| `govulncheck ./...`, scanner v1.9.0 | Exit 0; **zero reachable findings**, zero imported-package findings; one informational required-module finding is not called by this code |
| `go vet ./...` and entity-response guard | Both pass, no findings |
| `make test-quick` | Pass; all **17** discovered essential tests executed |
| Python workflow contracts | **19 passed**, one existing opt-in skip for disposable Docker metadata builds |
| Formatting, source review, and module checks | Clean; source/fixture review found no blocker; application dependency requirements and `go.sum` unchanged |

The existing passkey control covers list/delete/UUID response behavior and rejection of legacy numeric IDs. No existing controller registration-ceremony test was found, so this is not a claim of complete `PasskeyRegisterFinish` ceremony coverage. The signed-chain regression directly exercises the vulnerable standard-library boundary identified by the scanner.

Repeat the final application run with the seven packages listed below, then the narrow authentication selection and existing gates:

```sh
go test -race -p 2 -json -count=1 -timeout 20m \
  ./relay/controller ./relay/adaptor/openai ./relay/adaptor/anthropic \
  ./relay/adaptor/common/deepseekcompat ./relay/tooling \
  ./middleware ./relay/adaptor/typesafe
go test -race -p 2 -json -count=1 -timeout 10m \
  -run '^(TestToolchainX509CertificateTrust|TestPasskeyStrictOutResponses)$' ./controller
govulncheck ./...
python3 .github/scripts/test_workflows.py
```

| Patched-compiler evidence | SHA256 |
| --- | --- |
| Seven-package race JSON | `6ff547ea2b908e507bb8547c68df07d7c87dcb85f53cdec4f3bd2fe2fbc4f3aa` |
| X.509 RED JSON on Go 1.27.2 | `eaa8a1b87d981835c213a39d217db19c70f9f822fe693dc1c4ac30a7b013593b` |
| X.509 GREEN JSON on Go 1.27.3 | `a53d14d680bc6f6cea4d599d34dfee8044763dd94c2926302a10706040273787` |
| Passkey/X.509 controls JSON | `302ee60fb609d8d4ba70acd043ca0bcf44d3a4026fb54628627cb690c8784d3a` |
| Vulnerability scan log | `1317ef0cabdfc5bea69eafe177f0bfdbf91fee052c95cbcdcbb88e65b04d3332` |
| Vet log | `67f981a1dc23742fe65e2f656d341fc9ec889c4d69cdaf6798f2e44b2257ab30` |
| Entity-response guard log | `acda4a5393e4f704b43d7e433ee94c28f5755aa8ab6836827a652cc3b64fc7a3` |
| Quick-gate log | `78598410d42c6400630b6aed9328a647b668df33499785dda52e4a57e4406d55` |
| Workflow-contract log | `f0ee8f8608071836361d87cf99c8b0daa52eedd54d5718c235b1b2fc4faf1549` |

The compiler changes intentionally activate all three frontend dependency audits under the existing path filters. They must pass in the next exact-head CI run. Full frontend builds/browser qualification, full database/race shards, and disposable Docker metadata builds remain unqualified unless separately run. No required job, advisory, trust check, or failure condition was bypassed.

## Compatibility and review

The deliberate behavior change remains rejection of positive-priced DeepSeek native tools for which OneAPI lacks an authoritative paid-operation receipt. No tariffs, currencies, effective dates, channel configuration, schema migrations, production balances, or historical ledgers were changed. Effective-zero/default/local/client behavior and unrelated providers retain their established controls.

The implementation reuses pricing, discovery, ownership, and settlement boundaries. It does not introduce a parallel billing system, enable direct Search/Fetch settlement, or turn the upstream grace date into automatic pricing activation. The existing WebSocket resolver still uses `BaseURL`; full-endpoint override support remains separate routing work.

Independent review of the six changed/new Go files found no further implementation blocker. Its two documentation clarifications were applied: the native WS channel is specifically an OpenAI channel, and the byte-preservation guarantee is scoped to admission/model guarding rather than the whole ownership pipeline. [Fresh CodeRabbit review](https://github.com/Laisky/one-api/pull/589#issuecomment-6274463849) covered all eight follow-up files from `e2740d6` through `2c9a73d3` and reported no actionable comments. The later toolchain update requires separate refreshed checks and review coverage.

See [DeepSeek server-tool billing compatibility](../docs/manual/deepseek_tool_billing_compatibility.md) for the user-visible contract and official provider references.

## Release boundary and rollback

The old queue behind #586 ARM64 publication is no longer current. [The #586 delivery run](https://github.com/Laisky/one-api/actions/runs/39348575942) completed successfully. The latest main [delivery run 39525224970](https://github.com/Laisky/one-api/actions/runs/39525224970) also completed its amd64 publication, deployment, and ARM64 publication jobs for `a53b80265330c8960c87adff3f3d2f204a95d10a`.

Recorded latest-main artifacts:

- amd64: `sha256:bb4c808bda504aa0de2734d06146776e0afdf282341f3dcc80693e19e05e5d4a`.
- ARM64: `sha256:742710131206e2c3ef713e17794758dc4b1b69ee9d37466fb419149dba489215`.
- Deployment workflow receipt: 2026-10-21T01:12:15Z, passed, source `a53b802`, phase `done`.

These are observed workflow receipts, not a new live-host verification or registry-configuration qualification. Before any new release, recheck the running source/artifact and retain the applicable immutable rollback image. Do not repeat an already-completed release based on the obsolete queue state.

This work updates the existing PR branch only. Merging to `main` invokes the repository's automatic image publication and production deployment, so the explicit merge/release approval boundary in #589/#592 remains in effect. A separately authorized release must verify immutable source/image identity, architecture publication, service health, public application/config/assets, and unchanged unrelated services before closing the handoff.

Source rollback requires no migration or price/configuration rollback and does not reverse historical charges. Reverting the new follow-up restores both the WebSocket admission gap and model-rewrite precision loss. Reverting the compiler to Go 1.27.2 also restores the known certificate-trust issue; retain that tradeoff in any rollback decision and keep a patched compiler when rebuilding older application source where possible.
