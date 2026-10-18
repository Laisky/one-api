# Codex Responses compatibility audit

Audit date: 2026-10-17. Gateway baseline: `b47a00e6785cea8146903facd0a0e0879d318644`.
This bounded matrix does not establish full Codex support. The urgent client tool-search patch is qualified separately against its final head.

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
That capture establishes only the exercised synthetic profile. The actual failed profile's execution, request sequence and tool catalog remain uncaptured.
Local qualification below applies only to the scoped synthetic contracts in this change.
No new real Codex/provider request was made for this follow-up.

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

**Source-derived** means source/test inspection, without a pass claim. **Locally qualified** means the scoped synthetic tests passed; actual-profile acceptance remains pending.
**Unqualified** means fidelity is not established. **Explicit unsupported** means rejection or a documented limitation applies.
Test paths are repository-relative; existing tests express intended contracts, not a claim that every capability passed in a captured Codex session.

| Capability | Existing focused coverage | State and boundary |
| --- | --- | --- |
| Native versus fallback routing | `relay/controller/response_support_test.go`: `TestSupportsNativeResponseAPIOpenAICompatible`, `TestSupportsNativeResponseAPIAzureGpt5`, `TestSupportsNativeResponseAPIDeepSeekV4`; `systematic_response_fallback_test.go` | **Source-derived.** Channel format, mapped model/provider contract and MCP/state diversion determine routing. A Responses URL alone does not guarantee native passthrough. |
| Standard/Lite client-search catalogs | `relay/adaptor/openai/responseapi_tool_search_test.go`; `relay/controller/response_client_tool_search_wire_test.go`; `response_client_tool_search_http_test.go` | **Locally qualified** synthetic root/input catalogs, client-only execution, hosted/unknown rejection, ambiguous keys and native/fallback paths. External provider/Codex acceptance remains pending. |
| Function/custom namespace identities | `response_namespace_http_test.go`: `TestResponseNamespaceHTTPNativePreservesWireAndBilling`, `TestResponseNamespaceHTTPFallbackRejectsBeforeState`; `relay/adaptor/openai/responseapi_namespace_test.go` | **Source-derived** native definitions/output calls. Namespace definitions on fallback are **explicit unsupported**. Namespace-bearing history without definitions remains **unqualified**. |
| Developer input/plaintext reasoning | `relay/adaptor/openai/responseapi_convert_request_test.go`: `TestConvertResponseAPIToChatCompletionRequestMapsDeveloperRole`, `TestConvertResponseAPIToChatCompletionRequestPreservesToolCallReasoning`; `relay/controller/response_io_reasoning_test.go` | **Source-derived.** Fallback maps developer to system and has turn/reasoning replay tests; encrypted/provider-bound reasoning is not generally portable. |
| Previous response/state/ownership | `response_state_native_test.go`; `response_state_behavior_test.go`; `response_state_portability_test.go` | **Source-derived.** Same-provider rewrite and different-provider diversion are tested. Owner-scoped gateway IDs apply; unknown/foreign handles fail closed. Unsupported hydrated state returns `state_not_portable`; some reasoning sidecars are intentionally dropped. |
| Compaction/context management | No compaction handler found; `router/relay.go` registers Responses create/get/delete/cancel | **Explicit unsupported / unqualified.** Compact-endpoint and compaction-item fidelity are not qualified. Add a local negative route/admission fixture. |
| SSE terminal/incomplete/errors | `relay/adaptor/openai/security_response_terminal_receipt_test.go`; `security_response_provisional_eof_test.go`; `relay/controller/responses_bridge_completion_ledger_test.go`; `security_response_ws_terminal_test.go` | **Source-derived.** Coverage distinguishes terminal completion from EOF/truncation and exercises incomplete/error/receipt settlement. Codex-version replay remains **unqualified**. |
| Cancellation/background | `security_response_stream_exit_test.go`; `relay/controller/security_response_background_test.go`; `response_state_governance_test.go`: `TestResponseStateCancelGatewayResolution`, `TestResponseStateCancelLegacyPassthrough` | **Source-derived.** Background execution is rejected; committed gateway responses cannot be cancelled and unknown IDs fail closed. No general background-job support claim. |
| Structured outputs/schema strictness | `relay/adaptor/openai/response_model_structured_test.go`; `response_model_structured_edge_cases_test.go` | **Source-derived** conversion coverage; fallback strict-schema fidelity **unqualified** because the converter clears strictness. |
| Images/audio/files/video | `responseapi_convert_request_test.go`: `TestConvertResponseAPIToChatCompletionRequestPreservesDeepSeekFileImages`; `relay/controller/security_response_image_admission_test.go` | **Source-derived** image cases. Audio conversion exists; broader file/video/unknown-part fidelity and explicit unsupported behavior remain **unqualified**. |
| Usage/cache/reasoning accounting | `relay/adaptor/openai/response_api_stream_test.go`: `TestResponseAPIStreamUsageCachedTokens`; `response_model_usage_websearch_test.go`: `TestResponseAPIUsageRoundTripPreservesKnownDetails`; `relay/controller/security_reasoning_ledger_test.go`; `responses_bridge_receipt_coverage_test.go` | **Source-derived.** Existing tests exercise receipts, cached/reasoning counters and ledger settlement; no actual-profile financial effect is inferred from source. |
| Root cache hints/client metadata | `relay/controller/response_billing_policy.go`: `nativeResponseRootAllowed`; `request_passthrough.go`: `allowedExtraBodyKeys` / `isAllowedExtraBodyKey` | **Source-derived limitation.** `prompt_cache_key` and `client_metadata` are absent from both allowlists and are stripped on the native wire path. No live failure cause or cache-performance effect is established. |

## Prioritized unresolved cases

1. **Namespace-bearing fallback history without definitions.** `prepareResponseFallbackInput` rejects namespace definitions in `request.Tools`,
   but state lowering carries `function_call` items by type and `responseapi_convert_request.go` lowers name/IDs without namespace.
   Test direct input and owner-scoped native-to-fallback continuation with two namespaces, the same leaf name and no `tools` array.
   Require explicit rejection before reservation/upstream; do not flatten names or merge identities.
2. **Unsupported content loss.** `responseContentItemToMessage` handles known text/image/audio forms, but unrecognized nontext parts can disappear.
   Add file/video/unknown-part fixtures with a mixed valid-text control. Assert supported preservation or an explicit unsupported result.
3. **Strict schema downgrade.** `ConvertResponseAPIToChatCompletionRequest` sets `ResponseFormat.JsonSchema.Strict = nil`.
   Add strict-true/strict-false fixtures and document or reject the downgrade; do not advertise strict fidelity.
4. **Compaction.** Add local compact-route and compaction-item fixtures, asserting the actual unsupported status/code/body without provider contact.
   Do not silently translate encrypted compaction state into messages.
5. **Version/profile provenance.** Keep per-version Standard/Lite fixtures separate from actual failed-profile evidence.
   Current installed version alone establishes neither the last working version nor release causality.
6. **Root cache hints/client metadata.** Add positive metadata controls and negative/observational fixtures for `prompt_cache_key` and `client_metadata`,
   recording the native wire stripping behavior. Review any future admission independently; this audit does not implement or attribute the failure to these fields.

## Bounded local reproduction

Run from the repository root with its Go toolchain. These select local tests; they do not run Codex or paid provider tests.
On shared dev, use the existing nonblocking validation lease; a busy lease means stop and coordinate.

```sh
flock -n /tmp/laisky-heavy-validation-20261008.lock go test ./relay/adaptor/openai -v -count=1 \
  -run '^(TestClientToolSearch.*|TestResponseAPINamespace.*|TestSecurityResponseToolID.*|TestResponseAPIStreamUsageCachedTokens|TestResponseAPIUsageRoundTripPreservesKnownDetails|TestConvertResponseAPIToChatCompletionRequestMapsDeveloperRole)$'

flock -n /tmp/laisky-heavy-validation-20261008.lock go test ./relay/controller -v -count=1 \
  -run '^(TestResponseClientToolSearch.*|TestResponseNamespace.*|TestSupportsNativeResponseAPI.*|TestResolveNativePreviousResponse.*|TestSecurityReasoningHTTPSettlement|TestResponseStateCancelGatewayResolution)$'
```

Before recording a pass, retain the exact gateway commit/tree or dirty-diff hash, toolchain version, command, exit status and sanitized output path.
A filtered command succeeding when a newly named test is absent is not evidence: confirm expected test names are present in its output.

Future fixtures should record: official tag/commit; builder source/blob; Standard/Lite switch; synthetic versus captured origin;
sanitized model/channel-format/provider-contract metadata; request and SSE sequence; expected wire/admission/terminal/accounting result.
Use local mock providers and fake credentials. Include positive controls and hosted/unknown-tool negatives at every catalog location.
Add cases for these gaps before broadening implementation scope. Capture the actual failed profile only through an authorized privacy-preserving route.

## Scoped local qualification

On Go 1.27.2/linux-amd64, this change passed 76 client-search/namespace controller race test/subtest results,
1,713 full OpenAI adapter results and 10,715 relevant Responses/MCP/reasoning controller results.
The two adapter skips are existing opt-in live DeepSeek tests; no paid-provider test was enabled.
Quick checks and `go vet ./...` passed. Exact logs and source-file hashes are retained under
`/tmp/task20261017-1536/validation-receipt.json`; the corrected entity guard has its separate receipt.
These are source qualification results, not successful acceptance of the actual failed profile.
The separate release at b47a00e still failed that acceptance on client tool_search.
