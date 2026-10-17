# Codex Responses compatibility audit

Audit date: 2026-10-17. Initial gateway baseline: `b47a00e6785cea8146903facd0a0e0879d318644`.
The later client tool-search release is [PR #582](https://github.com/Laisky/one-api/pull/582), source `56291a366bd29cf0b93e2bbda3d1df8ca68a9129`, merged as `9569b56`.
The broader fallback-fidelity changes below are **unreleased**. Local qualification is recorded below; exact-head CI and release decisions belong to the Draft PR. This matrix does not establish full Codex support.

## Version provenance and evidence limits

Official release tags were resolved through GitHub's Git References API. Pins identify reproducible source, not the release that caused a failure.

| Official Codex tag | Immutable commit | Evidence in this audit |
| --- | --- | --- |
| [rust-v0.161.0](https://github.com/openai/codex/releases/tag/rust-v0.161.0) | `979011409de0a60b52f179721948e65531d26144` | Source pin; version-specific behavior unqualified |
| [rust-v0.162.0](https://github.com/openai/codex/releases/tag/rust-v0.162.0) | `c1382380de69521303b416720a52f42d51af6248` | Source pin; version-specific behavior unqualified |
| [rust-v0.162.1](https://github.com/openai/codex/releases/tag/rust-v0.162.1) | `092d3acd6bec3e3a14bdc7e7a2810ab628ab759d` | Source pin; version-specific behavior unqualified |
| [rust-v0.163.0](https://github.com/openai/codex/releases/tag/rust-v0.163.0) | `83086188ae0614bd882818ff1aa23f3194962012` | Request/catalog source inspected; bounded installed-client evidence below |

The user's last known working Codex build is **UNKNOWN**. No regression-introducing version is established.
The investigation verified an installed 0.163.0 client and a synthetic, no-MCP local capture.
That initial capture establishes only the exercised synthetic profile; it did not establish the actual failed-profile request sequence or tool catalog.
The separate PR #582 release passed one installed Codex 0.163.0 greeting using the existing profile at 16:29 UTC.
That bounded live result does not exercise namespace continuation, compaction, structured output, or every Standard/Lite catalog.
The broader fallback patch has no live acceptance result; its qualification is separate from the released greeting.

Retained evidence (sanitized receipts, not complete failed-profile execution captures):

- `/tmp/task20261017-1345-catalog-probe-u9gwvx7o/sanitized-comparison.json`: before/after each captured one request and exited 1.
- `/tmp/task20261017-1345-catalog-probe-u9gwvx7o/catalog-update-receipt.json`: catalog entries 2 to 3, adding `deepseek-flash`;
  old entries retained exactly, readback verified, namespace tools still present. A catalog entry change does not prove protocol compatibility.
- `/tmp/task20261017-1418-namespace-incident/fix/codex-acceptance.json`: gateway source `b47a00e6785cea8146903facd0a0e0879d318644`,
  exit 1, no final response, error code `tool_billing_not_supported`. This is a failed acceptance result, not a successful end-to-end qualification.


## Standard and Lite builders at the pinned 0.163.0 source

[client.rs:980-995](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/core/src/client.rs#L980)
selects the request builder using `model_info.use_responses_lite`:

- Standard serializes definitions into top-level `tools`.
- Lite prepends an `additional_tools` input item with role `developer` and a stable `at_` ID; top-level `tools` is omitted.
- [The Lite serializer](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/tools/src/tool_spec.rs#L95)
  groups function/custom tools into the `functions` namespace, merges an existing namespace of that name, and serializes other variants unchanged.
  Its root catalog can therefore contain `tool_search` itself, not just discovered functions.
- [The tool enum](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/tools/src/tool_spec.rs#L20)
  includes hosted `web_search`. Catalog placement must never bypass hosted-tool billing admission.
  Client search requires explicit `execution: "client"` validation; hosted/server or unknown execution is not implied safe.
- [Pinned upstream Lite fixtures](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/core/tests/suite/responses_lite.rs#L380)
  expect configured standalone `web.run` and `image_gen.imagegen` namespaces, excluding hosted `web_search`/`image_generation`.
  These fixtures describe tested configurations, not every runtime profile.
- [Client input handling](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/core/src/client.rs#L1007)
  distinguishes reasoning/compaction items and requests encrypted reasoning content. Source presence alone does not establish gateway support.

## Behavioral matrix

**Source-derived** means source/test inspection, without a pass claim. **Locally qualified** means the scoped synthetic tests passed; the separate released greeting remains limited to that exercised scenario.
**Unqualified** means fidelity is not established. **Explicit unsupported** means rejection or a documented limitation applies.
**Unreleased fixture** identifies new source and synthetic controls qualified by the final local receipt below; it does not establish live acceptance or release.
Test paths are repository-relative; existing tests express intended contracts, not a claim that every capability passed in a captured Codex session.

| Capability | Existing focused coverage | State and boundary |
| --- | --- | --- |
| Native versus fallback routing | `relay/controller/response_support_test.go`: `TestSupportsNativeResponseAPIOpenAICompatible`, `TestSupportsNativeResponseAPIAzureGpt5`, `TestSupportsNativeResponseAPIDeepSeekV4`; `systematic_response_fallback_test.go` | **Source-derived.** Channel format, mapped model/provider contract and MCP/state diversion determine routing. A Responses URL alone does not guarantee native passthrough. |
| Standard/Lite client-search catalogs | `relay/adaptor/openai/responseapi_tool_search_test.go`; `relay/controller/response_client_tool_search_wire_test.go`; `response_client_tool_search_http_test.go` | **Locally qualified** synthetic root/input catalogs, client-only execution, hosted/unknown rejection, ambiguous keys and native/fallback paths. PR #582 has the bounded greeting result above; full catalog/profile acceptance remains unqualified. |
| Function/custom namespace identities | `response_namespace_http_test.go`: `TestResponseNamespaceHTTPNativePreservesWireAndBilling`, `TestResponseNamespaceHTTPFallbackRejectsBeforeState`; `relay/adaptor/openai/responseapi_namespace_test.go` | **Source-derived** native definitions/output calls. Namespace definitions on fallback are **explicit unsupported**. New `TestClientCompatibilityCodexNamespaceHistory` and `TestClientCompatibilityCodexFallbackAdmission` reject namespace-bearing history without definitions on direct and hydrated fallback paths before pending state, quota or provider dispatch (**unreleased fixture**). Native `TestClientCompatibilityCodexNativeWire` preserves distinct call/item IDs and namespaces. |
| Developer input/plaintext reasoning | `relay/adaptor/openai/responseapi_convert_request_test.go`: `TestConvertResponseAPIToChatCompletionRequestMapsDeveloperRole`, `TestConvertResponseAPIToChatCompletionRequestPreservesToolCallReasoning`; `relay/controller/response_io_reasoning_test.go` | **Source-derived.** Fallback maps developer to system and has turn/reasoning replay tests; encrypted/provider-bound reasoning is not generally portable. |
| Previous response/state/ownership | `response_state_native_test.go`; `response_state_behavior_test.go`; `response_state_portability_test.go` | **Source-derived.** Same-provider rewrite and different-provider diversion are tested. Owner-scoped gateway IDs apply; unknown/foreign handles fail closed. Unsupported hydrated state returns `state_not_portable`; some reasoning sidecars are intentionally dropped. |
| Compaction/context management | `TestClientCompatibilityCodexCompaction`, `TestClientCompatibilityCodexFallbackAdmission`, `TestClientCompatibilityCodexNativeWire` | **Unreleased fixture.** Opaque compaction input fails explicitly on fallback; native input remains provider-owned. No general `/responses/compact` endpoint or Codex compaction-session acceptance claim. |
| SSE terminal/incomplete/errors | `relay/adaptor/openai/security_response_terminal_receipt_test.go`; `security_response_provisional_eof_test.go`; `relay/controller/responses_bridge_completion_ledger_test.go`; `security_response_ws_terminal_test.go` | **Source-derived.** Coverage distinguishes terminal completion from EOF/truncation and exercises incomplete/error/receipt settlement. Codex-version replay remains **unqualified**. |
| Cancellation/background | `security_response_stream_exit_test.go`; `relay/controller/security_response_background_test.go`; `response_state_governance_test.go`: `TestResponseStateCancelGatewayResolution`, `TestResponseStateCancelLegacyPassthrough` | **Source-derived.** Background execution is rejected; committed gateway responses cannot be cancelled and unknown IDs fail closed. No general background-job support claim. |
| Structured outputs/schema strictness | `TestClientCompatibilityCodexJSONSchemaStrict`; `responseapi_convert_legacy_contract_test.go`: `TestConvertResponseAPIToChatCompletionRequestSanitizesFunctionParameters` | **Unreleased fixture.** Explicit strict true/false preserves the complete text schema and flag in conversion, using detached maps/arrays/pointers. Absent strict retains the legacy sanitizer; tool-parameter sanitation is unchanged. Downstream channel normalization still applies. |
| Images/audio/files/video | `responseapi_convert_request_test.go`: `TestConvertResponseAPIToChatCompletionRequestPreservesDeepSeekFileImages`; `relay/controller/security_response_image_admission_test.go` | **Source-derived** image cases. New `TestClientCompatibilityCodexMixedContent` and `TestClientCompatibilityCodexKnownContent` cover readable text aliases/refusals and explicit rejection of `input_file` ID/inline data/URLs or unknown parts before dispatch (**unreleased fixture**). This is not general video or provider file-handle acceptance. |
| Usage/cache/reasoning accounting | `relay/adaptor/openai/response_api_stream_test.go`: `TestResponseAPIStreamUsageCachedTokens`; `response_model_usage_websearch_test.go`: `TestResponseAPIUsageRoundTripPreservesKnownDetails`; `relay/controller/security_reasoning_ledger_test.go`; `responses_bridge_receipt_coverage_test.go` | **Source-derived.** Existing tests exercise receipts, cached/reasoning counters and ledger settlement; no actual-profile financial effect is inferred from source. |
| Accepted MCP failure settlement | `relay/controller/client_compatibility_generic_mcp_error_test.go`: `TestClientCompatibilityGenericMCPMergeErrorSettlesAcceptedHolds`, `TestClientCompatibilityGenericMCPNormalLoop`; `client_compatibility_mcp_merge_safety_test.go`: `TestClientCompatibilityMCPMergePreservesUncertainty`, `TestClientCompatibilityMCPMergeRejectsOverflow` | **Unreleased fixture / intended contract.** A later accepted body/receipt/merge failure must retain established holds, reconcile one estimated ledger, reject unsafe sums and prevent retry of accepted work. A normal measured two-round control remains exact. Normal and overflow controls pass on actual Chat, Responses fallback and converted Messages routes; each requires exactly two provider requests and one MCP call. Converted Messages ownership is detected before ordinary dispatch. |
| Root cache hints/client metadata | `relay/controller/response_billing_policy.go`: `nativeResponseRootAllowed`; `request_passthrough.go`: `allowedExtraBodyKeys` / `isAllowedExtraBodyKey` | **Source-derived limitation.** `prompt_cache_key` and `client_metadata` are absent from both allowlists and are stripped on the native wire path. No live failure cause or cache-performance effect is established. |

## Resolved source gaps and remaining limits

The unreleased fallback repair addresses namespace history without `tools`, silent mixed-content loss, and explicit text-schema strictness.
The same fidelity validator runs at converter entry and before/after owner-scoped hydration. A successful pending-state snapshot still uses
only the original incremental input; rejected input does not stage a commit. `input_file` fallback is explicitly unsupported; native Responses forwarding is unchanged. The shared internal file fields do not establish the nested Chat file wire contract.
Unknown message content returns `response_fallback_not_supported`/400; unsupported canonical state (including opaque compaction) retains `state_not_portable`/409. Namespace history retains its explicit 400 rejection. Nothing is dropped or converted to JSON text to bypass admission. Known refusal content remains readable.

Remaining limits:

1. **Downstream schema normalization.** `NormalizeStructuredJSONSchema` still removes numeric-bound keywords and applies Azure defaults.
   Conversion preservation does not guarantee arbitrary schema fidelity through every provider. Absent strict and function-tool parameters
   retain legacy sanitization; this repair does not broaden their contract.
2. **Reasoning portability.** `state.FallbackLowering` intentionally drops provider-bound reasoning/thinking sidecars, including encrypted-only
   reasoning with no summary. Direct conversion preserves replayable summaries only on their assistant turn. This is a documented lossy
   fallback policy, not portable encrypted-state support; native passthrough is a distinct control.
3. **Compaction and media.** Native opaque-input forwarding and fallback rejection do not qualify a compaction endpoint/session, video,
   or external provider file handles. No live compaction or multimodal acceptance was performed.
4. **Root cache hints/client metadata.** The audited native allowlists omit `prompt_cache_key` and `client_metadata`; the current follow-up
   does not establish their preservation or cache-performance impact. Do not infer a failure cause from this source limitation.
5. **File wire compatibility.** [Official Chat file inputs](https://developers.openai.com/api/docs/guides/file-inputs) use a nested `file` object, support PDFs only, and do not support Responses-only file URL/detail semantics. This bounded repair rejects new `input_file` fallback rather than changing global content serialization. Existing legacy `input_image` file handling is outside this repair.
6. **Version/profile provenance.** The last working client version and regression-introducing version remain unknown. One released greeting
   is not proof of all installed-profile tool flows, retry/resume behavior, or every Standard/Lite configuration.

Compaction cache accounting on the native Claude/MCP path is a separate implemented source contract: cumulative
root message counters replace the non-compaction component while compaction/cache TTL buckets remain counted once.
`TestClientCompatibilityIterationRootCacheDeltas`, `TestClientCompatibilityIterationRootCacheHTTPBilling` and
`TestClientCompatibilityClaudeMCPIterationUsage` provide synthetic coverage. That does not add Codex compaction
fallback support or establish live cache effectiveness. Native accepted-work errors and the internal typed no-work
refund proof have distinct settlement rules; see the [Claude Code matrix](claude_code_compatibility.md) for exact fixtures.
An HTTP error status alone does not prove that no paid work occurred. See final local qualification below.

## Bounded local reproduction

Run from the repository root with its Go toolchain. These select local tests; they do not run Codex or paid provider tests.
On shared dev, use the existing nonblocking validation lease; a busy lease means stop and coordinate.

```sh
flock -n /tmp/laisky-heavy-validation-20261008.lock go test ./relay/adaptor/openai -v -count=1 \
  -run '^(TestClientCompatibilityCodex.*|TestClientToolSearch.*|TestResponseAPINamespace.*|TestSecurityResponseToolID.*|TestResponseAPIStreamUsageCachedTokens|TestResponseAPIUsageRoundTripPreservesKnownDetails|TestConvertResponseAPIToChatCompletionRequestMapsDeveloperRole)$'

flock -n /tmp/laisky-heavy-validation-20261008.lock go test ./relay/controller -v -count=1 \
  -run '^(TestClientCompatibilityCodex.*|TestResponseClientToolSearch.*|TestResponseNamespace.*|TestSupportsNativeResponseAPI.*|TestResolveNativePreviousResponse.*|TestSecurityReasoningHTTPSettlement|TestResponseStateCancelGatewayResolution)$'
```

Before recording a pass, retain the exact gateway commit/tree or dirty-diff hash, toolchain version, command, exit status and sanitized output path.
A filtered command succeeding when a newly named test is absent is not evidence: confirm expected test names are present in its output.

Future fixtures should record: official tag/commit; builder source/blob; Standard/Lite switch; synthetic versus captured origin;
sanitized model/channel-format/provider-contract metadata; request and SSE sequence; expected wire/admission/terminal/accounting result.
Use local mock providers and fake credentials. Include positive controls and hosted/unknown-tool negatives at every catalog location.
Add cases for these gaps before broadening implementation scope. Capture the actual failed profile only through an authorized privacy-preserving route.

## Historical client-search qualification and current follow-up

On Go 1.27.2/linux-amd64, this change passed 76 client-search/namespace controller race test/subtest results,
1,713 full OpenAI adapter results and 10,715 relevant Responses/MCP/reasoning controller results.
The two adapter skips are existing opt-in live DeepSeek tests; no paid-provider test was enabled.
Quick checks and `go vet ./...` passed. Exact logs and source-file hashes are retained under
`/tmp/task20261017-1536/validation-receipt.json`; the corrected entity guard has its separate receipt.
These are source qualification results, not successful acceptance of the actual failed profile.
The separate release at b47a00e still failed that acceptance on client tool_search.

The broader repair retains valid baseline RED receipts and corrected fixture provenance under
`/tmp/task20261017-08/`. Initial integration logs are historical evidence; the final local receipt below
qualifies the integrated source. See also [Claude Code compatibility](claude_code_compatibility.md).


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
