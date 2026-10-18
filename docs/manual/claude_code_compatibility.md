# Claude Code compatibility matrix

Audit date: 2026-10-17. This is a bounded source/fixture audit, not a full Claude Code support declaration.
The broader compatibility patch is **unreleased**. Local source qualification is recorded below; exact-head CI and release decisions belong to its Draft PR.

## Version and acceptance provenance

| Client | Immutable source pin | Evidence boundary |
| --- | --- | --- |
| Installed Claude Code 2.1.302 | [official v2.1.302](https://github.com/anthropics/claude-code/releases/tag/v2.1.302), commit `c017428c8c27e604840435048d5bce6a9a22c040` | Version/source pin; synthetic gateway contracts below. No live Claude Code/provider acceptance was run. |
| Last known working Claude Code version | **UNKNOWN** | No regression-introducing version is established. |

The working-tree base is OneAPI `56291a366bd29cf0b93e2bbda3d1df8ca68a9129` (released separately through PR #582,
merged as `9569b56`). That release passed one Codex 0.163.0 greeting at 16:29 UTC, which supplies no Claude acceptance evidence.
The initial integration receipt `/tmp/task20261017-08/integration-first.jsonl` is not a final qualification receipt.
The matrix describes current source and fixture contracts, including cumulative cache deltas, native MCP usage and precision.
The final local checks below cover the integrated source; live client/provider acceptance remains separate.
All fixture payloads and tools are synthetic; no external paid provider or user data is required.

## Protocol contract and fixture matrix

The official [gateway guide](https://code.claude.com/docs/en/llm-gateway-protocol) explains why protocol headers,
paired beta/body fields, cache markers, retry metadata and recognizable error envelopes matter.
Its Anthropic-format guidance does not imply identical behavior for Bedrock, Vertex or other translated routes.
The table records scoped synthetic behavior and actual test names; the final local receipt below identifies the qualified source and commands.
Test paths are relative to the repository root.

| Boundary | Synthetic fixture and path | Intended behavior / limit |
| --- | --- | --- |
| Native request headers/body | `relay/controller/client_compatibility_claude_headers_test.go`: `TestClientCompatibilityClaudeProtocolHeadersAndNativeBody`; `relay/adaptor/anthropic/client_compatibility_native_headers_test.go`: `TestClientCompatibilityClaudeHeaderBoundary` | Preserve protocol headers and native system/tool/cache/root fields on JSON and SSE. Channel credentials remain authoritative; caller credentials cannot replace them. Translated provider dialects are not qualified by these native fixtures. |
| Native response headers | `TestClientCompatibilityClaudeNativeResponseHeaders` in the controller headers file | Preserve selected correlation/retry/rate-limit headers; do not forward cookies, authorization or arbitrary private provider headers. |
| Final native error and retries | `controller/client_compatibility_claude_error_test.go`: `TestClientCompatibilityClaudeFinalNativeError`, `TestClientCompatibilityClaudeRetryDiscardsStaleNativeError`; adapter `TestClientCompatibilityNativeErrorCaptureBoundary`, `TestClientCompatibilityNativeErrorIdentity` | Preserve a selected final native error envelope/status and safe headers. Capture must not commit an earlier attempt, prevent eligible retries, or contaminate later success/local/converted errors. Gateway-generated errors retain their own contract. |
| Opaque content roundtrip | `relay/adaptor/anthropic/client_compatibility_content_test.go`: `TestClientCompatibilityClaudeContentOpaqueRoundTrip`, `TestClientCompatibilityClaudeContentTypedMutationWins`, `TestClientCompatibilityClaudeContentLegacyConstructed` | Retain opaque provider fields while respecting explicit typed mutations and legacy constructed blocks. Native wire preservation is distinct from cross-provider portability. |
| Native MCP continuation | `relay/controller/client_compatibility_claude_loop_test.go`: `TestClientCompatibilityClaudeOpaqueContinuation`, `TestClientCompatibilityClaudeOriginalControls`, `TestClientCompatibilityClaudeAssistantOpaqueReconstruction` | Replay opaque assistant blocks and original root/tool/cache controls across a real local follow-up round. Signed/opaque blocks are not flattened into plaintext. |
| Tool execution ownership | `TestClientCompatibilityClaudeMixedOwnership`, `TestClientCompatibilityClaudeAllMCPOwnership` in the loop file | Yield the complete parallel turn when any call belongs to the client; execute no MCP prefix. Execute an all-gateway-MCP turn once per call in original order. These controls do not authorize arbitrary hosted tool billing. |
| MCP argument fidelity | `TestClientCompatibilityClaudeMCPExecutionPrecision`, `TestClientCompatibilityClaudeMCPMissingInputPreservesObject` | Exercise actual local MCP argument bytes, large numeric tokens, typed-mutation precedence and empty-object input. Source preserves numeric tokens through execution and replay; See the final local qualification below. |
| MCP cache accounting | `relay/controller/client_compatibility_mcp_cache_test.go`: `TestClientCompatibilityMCPRoundCacheBilling`, `TestClientCompatibilityMCPRoundCacheIsolation` | Settle input/output, cache reads and 5-minute/1-hour writes from each round exactly once; retain earlier snapshots and tool fees. This is a synthetic ledger contract, not observed provider billing. |
| Usage uncertainty/overflow | `relay/controller/client_compatibility_mcp_merge_safety_test.go`: `TestClientCompatibilityMCPMergePreservesUncertainty`, `TestClientCompatibilityMCPLaterEstimateRetainsReservation`, `TestClientCompatibilityMCPMergeRejectsOverflow` | Preserve uncertainty/reservations and reject unsafe sums; no false verified receipt from an estimate or overflowing aggregate. See the final local qualification below. |
| Compaction receipts | `relay/adaptor/anthropic/client_compatibility_iterations_test.go`: `TestClientCompatibilityIterationReceipts`, `TestClientCompatibilityIterationCumulativeReplacement`, `TestClientCompatibilityInvalidIterationIsAtomic`, `TestClientCompatibilityIterationPartialFields`, `TestClientCompatibilityIterationCrossEntryOverflow` | Treat iterations as canonical usage when supplied, replace repeated cumulative snapshots, retain omitted verified fields and reject malformed/overflowing updates atomically. Root totals must not be added again. |
| Root-only deltas/native MCP usage | `client_compatibility_iteration_delta_test.go`: `TestClientCompatibilityIterationRootOnlyDeltas`, `TestClientCompatibilityIterationPartialSnapshotThenRootDelta`; controller `TestClientCompatibilityClaudeMCPIterationUsage` | The shared receipt logic replaces cumulative non-compaction counters while retaining compaction work, and the native MCP parser uses the same canonical accounting. See the final local qualification below. |
| Cumulative compaction cache deltas | `relay/adaptor/anthropic/client_compatibility_iteration_cache_delta_test.go`: `TestClientCompatibilityIterationRootCacheDeltas`; `relay/controller/client_compatibility_iteration_cache_billing_test.go`: `TestClientCompatibilityIterationRootCacheHTTPBilling`, `TestClientCompatibilityIterationAlternatingRootIndexedCache` | Update cumulative message cache reads/writes without losing separately accumulated compaction caches. Retain verified TTL splits; reject contradictory aggregate updates atomically. Synthetic SSE settlement checks each bucket once. |
| Native accepted-work failure settlement | `relay/controller/client_compatibility_claude_paid_error_test.go`: `TestClientCompatibilityClaudePaidErrorsRetainLedger`, `TestClientCompatibilityClaudePaidErrorMeasuredControl` | Intended contract: accepted body/receipt/merge failures retain established initial and incremental holds as one estimated ledger, do not synthesize overflowed usage, do not render success or retry accepted work, and reconcile repeat failure handling idempotently. Valid measured receipts retain exact settlement. See the final local qualification below. |
| Internal no-work proof | `relay/adaptor/anthropic/client_compatibility_admission_message_test.go`: `TestClientCompatibilityClaudeMCPAdmissionProof`; controller `TestClientCompatibilityClaudeMCPNoWorkRefundControl` | Only the internal typed proof of a complete native admission rejection with no receipt and a clean body close permits the no-work refund path. An HTTP status or apparent error envelope alone is insufficient; accepted/uncertain work keeps its holds. No client-supplied field grants this proof. |
| Generic MCP accepted-work failure | `relay/controller/client_compatibility_generic_mcp_error_test.go`: `TestClientCompatibilityGenericMCPMergeErrorSettlesAcceptedHolds`, `TestClientCompatibilityGenericMCPNormalLoop`, `TestClientCompatibilityGenericMCPResponsesError`, `TestClientCompatibilityGenericMCPResponsesNormal`, `TestClientCompatibilityGenericMCPClaudeError`, `TestClientCompatibilityGenericMCPClaudeNormal` | Intended contract on the actual Chat route: an accepted round followed by an unrepresentable usage sum fails explicitly, retains all established holds in one reconciled ledger and disables retry. The normal owned-alias two-round control settles measured usage. Chat, Responses fallback and converted Messages each pass strict two-provider/one-MCP normal and overflow controls. Converted Messages detects ownership before ordinary dispatch and uses Chat parsing for internal rounds. |
| Compaction HTTP/SSE settlement | `relay/controller/client_compatibility_compaction_billing_test.go`: `TestClientCompatibilityCompactionHTTPBilling` | Local native JSON/SSE returns a complete signed block, ping and repeated receipt controls; verifies user/token/request/log settlement and ordinary-message control. No live compaction session has been accepted. |

## Compaction semantics and limits

[Threshold compaction](https://platform.claude.com/docs/en/build-with-claude/compaction-threshold) uses
`context_management` and its threshold beta; [on-demand compaction](https://platform.claude.com/docs/en/build-with-claude/compaction-on-demand)
uses a separate `compaction` request and signed block. They are different protocols, not interchangeable flags.
For both accounting fixtures, sum canonical `usage.iterations` when present; do not add the root message counters again.
The on-demand SSE fixture includes a complete block without text deltas and preserves ping events.

Cumulative root input/output and cache counters describe the non-compaction component. Source receipt updates replace
that component and retain compaction input/output, cache reads, and 5-minute/1-hour writes separately; they do not
sum repeated root snapshots or add root totals on top of complete iterations. Partial indexed snapshots retain
previously verified fields. A later contradictory cache aggregate or invalid/overflowing counter update is rejected
without partially mutating the last verified snapshot. The root-cache adapter and HTTP-billing fixtures above cover
this distinction; they do not establish live provider cache effectiveness.

Accepted upstream work must not become a zero-cost request merely because body parsing, receipt validation or
cross-round aggregation fails. The intended error settlement uses the already established allowance as an estimated
floor, reconciles one user/token/request/log result, and prevents retry from replaying accepted work. This differs
from a positively proven no-work admission rejection, which refunds its holds. Both measured-success and no-work
controls accompany the failure fixtures; generic failure controls now pass on all three protocol entry points.

Synthetic passthrough/accounting coverage does not prove that a selected provider/model accepts either beta,
that a client correctly adopts the returned signed block, or that arbitrary cross-provider replay is safe.
The native Messages/MCP path is distinct from Responses-to-Chat/Claude fallback: that fallback intentionally
loses reasoning/thinking sidecars under its documented portability table. See [Codex Responses compatibility](codex_responses_compatibility.md).
Endpoint coverage, translated-provider dialects, exact token-counting behavior, real client retries and long-running
compaction/tool sessions remain separately unqualified.

## Reproduction and release evidence

The focused local selector is `TestClientCompatibility`; it runs adapter/controller contracts against loopback
providers, fake MCP tools and disposable ledgers. Select the existing test files explicitly in receipts and confirm
actual test/subtest names: an empty filtered run is not evidence. On shared dev, acquire the existing nonblocking
`/tmp/laisky-heavy-validation-20261008.lock` lease and stop if busy; coordinate with the validation owner.
No opt-in paid/live tests are implied by this matrix.

Before recording final qualification, retain exact source/tree hashes, Go version, commands, exits and sanitized
logs for the final integrated tree, plus required unit/race/quick/vet/entity/CI gates. Record live acceptance
separately with the client version, existing profile, selected route/model and bounded exercised scenario.
This document records neither a new live acceptance nor permission to merge or deploy.


## Final local source qualification

On 2026-10-17, Go 1.27.2/linux-amd64 passed the complete `TestClientCompatibility` selection
across the Anthropic/OpenAI adapters and both controller packages with `-race -count=2`: 312 test/subtest
PASS events, zero failures or skips. The complete four-package `-race -count=1` run passed 22,080 events
with ten existing skips: two opt-in live DeepSeek cases and eight MySQL/PostgreSQL transport-audit cases
requiring configured disposable backends. These are hierarchical event counts, not independent scenarios.

`make test-quick`, `go vet ./...`, `make lint-entity-response`, all seven reviewed browser-input contracts
and manifest completeness passed. The unchanged production guard passed against a Docker-shaped copy
of actual source without Git/GitHub metadata, then rejected a newly inserted unlisted source canary.
A fresh Docker image build was not attempted on the capacity-constrained dev filesystem. No image tag
was published for this patch. Full database/frontend qualification and security/ordinary CI must be read
from the exact Draft PR head; this local receipt does not substitute for them.

Receipts are `/tmp/task20261017-08/final-qualification-receipt.json`, `final-source-hashes.json` and
`docker-context-receipt.json`. All 50 touched Go files retained identical bytes throughout the run;
this documentation-only update records those results. Every touched Go file is at most 705 lines.
All fixtures use local providers, synthetic tools and disposable ledgers. There was no additional live
Codex or Claude Code acceptance, production billing rewrite, merge or deployment for this broader patch.
