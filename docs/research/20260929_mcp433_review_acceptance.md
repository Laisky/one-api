# MCP PR 433 review acceptance — 2026-09-29

## Scope and disposition

Review target: PR 433 at `859d00c37d214ebc62236af523c4a7936f7c1f86`.

### CodeQL allocation arithmetic (#177)

The reported expression is `make(map[string]any, len(params)+1)` in
`doModernRPCWithOptions`. An integer-overflow exploit was **not reproduced**.
The production callers construct the outer parameter envelope themselves:
`server/discover` supplies no method parameters, `tools/list` supplies at most
one cursor, and `tools/call` supplies at most four fields (`name`, `arguments`,
`inputResponses`, `requestState`). User-controlled argument/metadata keys live
inside those fields; they do not make `len(params)` approach the integer limit.

This is a static arithmetic hazard with no established reachable runtime
overflow in the audited call paths, not a demonstrated remote vulnerability.
Remove the unnecessary capacity addition, including the same pattern in
forwarded metadata, tool-descriptor extensions, and tool-result extensions.
Do not suppress or dismiss the CodeQL query.

Behavioral equivalence controls exercise nil, empty, single-entry, and
4,096-entry maps. They verify exact HTTP round trips, preservation of large JSON
integers, authoritative-field precedence, capability filtering, and caller-map
immutability. The larger helper-level maps are robustness controls, not
remotely reachable outer-envelope shapes or integer-overflow reproductions.
These controls must pass both before and after the hardening.

### Legacy execution replay — behavioral defect

The modern call path marks ambiguous failures with `ToolExecutionUncertainError`,
but after safe protocol fallback the legacy `CallTool` returned an ordinary
error. The same omission affected already-negotiated and explicitly legacy
calls. `CallWithFallback` could then execute the operation on another eligible
server even though the first server had already performed it.

The fixture uses two actual HTTP peers and an atomic side-effect counter. The
first server accepts and counts `tools/call`, then returns HTTP 503, a truncated
JSON result, or a response with the wrong request ID. The second eligible
server would count and complete the operation again. Each of these three
faults is exercised through explicit legacy, pre-negotiated modern-client,
and modern-to-legacy fallback entry points.

Fix: preserve the original error chain but mark failed attempted legacy tool
RPCs as uncertain. Keep initialization and argument-validation failures outside
the marker so safe pre-execution candidate fallback remains possible. Preserve
successful legacy calls, `isError` results, and existing session-expiry recovery.
This is conservative replay prevention, **not** an exactly-once guarantee or a
claim that every failed attempt actually performed a side effect.

## Retained regression tests

- `relay/mcp/review_433_replay_test.go`: nine uncertain-receipt cases and five
  positive controls, including admission failures and session recovery.
- `relay/mcp/review_433_allocation_test.go`: HTTP metadata-copy and serializer
  extension equivalence controls.
- `controller/mcp_review_433_test.go`: gateway capability filtering and
  request-local metadata controls.

The tests run in the existing package/race CI; no production workflow changes
or opt-in test gates are needed.

## Commands

```sh
go test -race -count=1 -run '^TestMCP433Review' ./relay/mcp ./controller
go test -race -count=1 ./relay/mcp ./controller
go test -race -count=1 -run '(MCP|Mcp)' ./model
go vet ./...
go build .
```

The tests-only control must use the exact original PR head with only the three
new test files applied. A compile/setup failure is not a behavioral reproduction.
All nine receipt-failure cases must fail there, all four positive-control test
groups must pass there, and every retained case must pass on the repaired head.

Local execution was blocked by the installed Go 1.23.2 toolchain and unavailable
DNS access to the repository-required Go 1.27.1 download. Qualification therefore
uses the unchanged repository toolchain and real dependencies on GitHub Actions;
no Go downgrade, dependency stub, test skip, or relaxed assertion is used.

## Executed qualification

[Qualification run 36590226625](https://github.com/Laisky/one-api/actions/runs/36590226625)
passed using real **Go 1.27.1 on Linux/amd64**. Its archived JSONL events and exit
codes were independently downloaded and inspected before publishing the PR update.

| Check | Observed result |
| --- | --- |
| Tests-only control on unchanged `859d00c37d214ebc62236af523c4a7936f7c1f86` | Nine failing receipt-fault leaves, each with expected execution count 1 and actual count 2; 21 positive-control leaves pass. |
| Repaired `relay/mcp` and `controller` complete suites, with race detection | 515 top-level tests / 1,096 leaf cases pass; no failures, skips, or race reports. Parent and leaf counts are alternative views, not additive. |
| All new review tests | Five top-level tests / 30 leaf cases pass, including all nine formerly failing leaves. |
| Model MCP selection, with race detection | Ten top-level tests pass; the leaf view has 18 passes and two pre-existing environment-gated skips (`TestUpdateMCPServerMultiDB/mysql` and `/postgres`). This is not a full model-package run. |
| Repository-wide `go vet ./...` and application build | Both pass. |
| `gofmt`, staged diff check, and unchanged working-tree check | All pass. |

The qualified production commit is
`f0267160012513648bdde6864b4396865cb3ac6c`, with the original PR head as its sole
parent. Its verified tree is `fe7b603305668be6af32d384cb733ab1222fef72`.
Only the seven listed Go implementation/test paths changed in that qualification.
This acceptance document is a separate documentation-only child; standard PR CI
must validate the final head independently.

[Raw qualification artifact](https://github.com/Laisky/one-api/actions/runs/36590226625/artifacts/11044450243):

- Archive SHA-256: `f7c7760cdad9363e4dc6a449feea0c9eeac16065ab27072fb899c3e337e95436`.
- Exact repair patch SHA-256: `167061aec6fd35bb270166ad2f4058bdb9407998f3baa658268ccccc5e026860`.

The artifact contains full negative/positive/model events, stderr, exit codes,
vet/build output, toolchain identity, the empty negative-control production diff,
final patch, tree identity, and per-file SHA-256 manifest. All seven candidate
file hashes and the published commit's actual parent/tree were checked.
Temporary transfer/qualification workflows are isolated from the production
commit ancestry and are not changes to this PR's CI.

No live paid MCP operation, automatic merge, production deployment, or tenant
configuration change was performed. The CodeQL alert's final disposition and
repository-wide CI results belong to the final-head PR checks, not these
focused runtime test results.

## References

- [Reported CodeQL thread](https://github.com/Laisky/one-api/pull/433#discussion_r4135057053)
- [CodeQL allocation-size overflow rule](https://codeql.github.com/codeql-query-help/go/go-allocation-size-overflow/)
- [MCP Streamable HTTP compatibility](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http#backward-compatibility)
