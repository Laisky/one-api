# Issue #592 — served-candidate release acceptance (2026-10-27)

Tracking: [issue #592](https://github.com/Laisky/one-api/issues/592), [merged source PR #589](https://github.com/Laisky/one-api/pull/589), and [WebSocket continuation fix PR #598](https://github.com/Laisky/one-api/pull/598).

**Status: production acceptance not yet established.** This document is a source-only handoff of *verifiable historical receipts* and the outstanding attended checks. It is **not** authorization to deploy, alter billing prices, run paid production canaries, or close #592. An Actions job reporting success at one instant is not a proof of the service currently being served.

## Clear the historical dependency confusion

- [#589](https://github.com/Laisky/one-api/pull/589) was merged source-only on 2026-10-23 in commit `9ff8502dbf21ae43fe62dc57b9a2c333549a4925`. Its conservative ten-attempt DeepSeek Search/Fetch pricing floor and behavior tests are documented in [the operator contract](../docs/manual/deepseek_tool_billing_compatibility.md).
- [#595](https://github.com/Laisky/one-api/issues/595) was **cancelled** by the maintainer and closed *not planned* on 2026-10-24. It is no longer a release gate; cancellation does **not** imply that the separate OTLP collector/storage migration was verified.
- [#596](https://github.com/Laisky/one-api/issues/596) was **completed** by merged [PR #598](https://github.com/Laisky/one-api/pull/598) on 2026-10-24 (merge commit `9591c3287e402f80206b137ae81aed59e8ea8187`). The corrected eleven-scenario native Responses WS/SQLite regression and adjacent race tests passed in [run 40058596415](https://github.com/Laisky/one-api/actions/runs/40058596415), with final PR-head [CI 40058780575](https://github.com/Laisky/one-api/actions/runs/40058780575) successful. These are **source** test results.
- Commit ancestry comparison on 2026-10-27 confirmed both #589 and #598 merge commits are ancestors of the independently published source below (not merely similarly named branches).

## Evidence from actual GitHub release receipts

The latest *fully inspected* successful release in this handoff is the 2026-10-25 [build-and-deploy run 40160628420](https://github.com/Laisky/one-api/actions/runs/40160628420). Its AMD64 and ARM64 builds and deploy job all concluded successfully. The deployment job returned a `passed: true`, `phase: done`, `reason: Latest one-api is healthy` result with the source and image digest shown below. This is evidence of **the deployment job's result at that time**, not proof of current runtime state or pricing/canaries.

| Receipt | Recorded value |
| --- | --- |
| Source commit | `077c866f1c5cbdcef55896d0badbaaa626f6c7fa` |
| AMD64 published manifest-list digest and deploy input | `sha256:6a86236a11f53c659b18ac56cc256d3d15a59c19b664c6a83f60349312c8fb79` |
| ARM64 published manifest-list digest | `sha256:699cb2943d2e61eef770025c82f46f65b2ed8f08d302c2048a4b2ea08f4f4572` |
| Source-labelled tags (mutable; digests above are the immutable identity) | `ppcelery/one-api:077c866` (AMD64), `ppcelery/one-api:arm64-077c866` (ARM64) |
| Earlier successful deploy (candidate *only* for rollback investigation) | [run 40154286467](https://github.com/Laisky/one-api/actions/runs/40154286467), source `a424575f178a6a2b75ca19c6d63bf4fb299c2403`, AMD64 digest `sha256:54f501412dd05543cd48a70394a1170ca8837524a1817fc499bfaf9b97e9eaa4` |

**Freshness warning:** At the 2026-10-27 inspection, `main` had advanced to `c213bac4b3d973786ae7c07aad567d0a0475e172` and [build-and-deploy run 40505389612](https://github.com/Laisky/one-api/actions/runs/40505389612) was **in progress**. Recheck its *final* result and anything newer before using the October 25 digest. The mutable `:latest` and `:arm64-latest` tags are not immutable deployment evidence. Published multi-platform manifest digests and local image IDs are different kinds of identifiers; do not assume they are string-equal.

## Remaining evidence required to accept the *currently served* candidate

| Gate | Evidence to collect (with timestamp and operator) | Status in this record |
| --- | --- | --- |
| Release intent/authority | Explicit approval identifying *which* source SHA, image digests and deployment window are being accepted; previous approvals are not transferable | **Missing** |
| Artifact provenance | Current runtime source and selected immutable image/digest, architecture, current delivery receipt; reconcile with live container image identity and registry manifest | **Missing for current runtime** |
| Configuration | Current DeepSeek endpoint/allowlist, relevant channel/model mapping, per-Search/Fetch tariffs (including effective zero-price), ten-attempt floor disclosure, relevant request/usage accounting settings; disclose without keys | **Missing** |
| Rollback | Previously *running* immutable image/digest, its actual availability, compatible runtime/config/data behavior, and tested non-destructive procedure. A historical successful deploy is only a candidate, not recovery proof | **Missing** |
| Health and served content | Live service readiness, restart/OOM and error counters, public app/config/assets and correct source version; unchanged unrelated services | **Missing** |
| Synthetic paid-tool canary | Explicitly authorized priced Search and Fetch behavior, including recorded user/token/request-cost/consume/tool ledger and floor-versus-distinct-attempt settlement. Check separate per-tool tariffs; do not claim exact upstream provider calls from OneAPI's floor | **Not executed** |
| WS/security canaries | With explicit authorization, verify independent `response.create` funding, no paid charge for proven unforwarded model-switch denials, earlier accepted work retained, and inherited-context owner/token insufficiency denied before dispatch | **Not executed in production** |
| Controls/rollback decision | Zero-price and local function behavior, unrelated-provider control, no stale rollout artifacts, owner sign-off, monitoring/rollback criteria; preserve charges historically made even if rolling back | **Missing** |

### Safe collection boundaries

An authorized operator can use **read-only** container inspections, for example (replace the placeholder locally; do not paste full environment/credentials into issues or logs):

```sh
# Docker-based host; do not infer the running image solely from the mutable tag.
docker container inspect --format 'image_id={{.Image}} started={{.State.StartedAt}} status={{.State.Status}} oom={{.State.OOMKilled}} restarts={{.RestartCount}}' <container>
docker image inspect "$(docker container inspect --format '{{.Image}}' <container>)" --format 'repo_digests={{json .RepoDigests}}'
```

If `RepoDigests` are unavailable, use the deployment controller's digest-pinned desired-state evidence and verify actual runtime image IDs by an appropriate supported registry/runtime inspection; **do not** substitute a tag string or image ID for a manifest digest. Collect redacted outputs, comparison timestamp, intended image source and platform, and retain the prior digest before a rollout. Do not run unapproved mutation, paid upstream calls, or a destructive rollback test.

For **source-only** repeatable regression checks on an owned local fixture (Go 1.27.3; no paid production provider required):

```sh
go test -race -count=1 -run 'TestSecurityNativeResponseWebSocketKnownContinuationAdmission|TestDeepSeekToolReceiptWebSocketAdmission' ./relay/controller/
```

The existing model/Claude/MCP/Responses native-tool regressions and affected-package suites in [the #589 qualification record](20261021_592_deepseek_integration.md) and [the #589 follow-up](20261023_589_billing_review_followup.md) cover further source compatibility. Do not add expensive local/race/provider checks as permanent required CI gates.

## Closure rule

Close #592 **only** after an operator records immutable *current* release provenance, a recoverable rollback, exact effective configuration, approved live canary and ledger results, live service/assets health, unchanged unrelated services, and release sign-off. Mark each gate with a source link and UTC timestamp. Neither merged source PRs, cancelled #595, nor successful October 25 CI/deployment receipts alone satisfy that condition.

This source-only documentation PR does not deploy and does not change DeepSeek availability, billing, migration or CI behavior. Merging any ordinary `main` commit can run the existing automatic deployment workflow; the documentation itself supplies no release authorization.
