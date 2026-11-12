# Vertex Gemini schema and tool compatibility

Research date: **2026-11-12 UTC**. Scope: the native Vertex Gemini **GenerateContent v1** adapter and its Chat Completions, Responses, and Claude Messages conversions. This is not certification of all Google Cloud products, model families, regions, historical versions, Live API or Interactions API.

## Contract boundary

OpenAI function parameters are JSON Schema. Vertex offers distinct wire contracts: typed `parameters` uses its OpenAPI-subset `Schema`, while `parametersJsonSchema` is a protobuf JSON Value containing a JSON Schema object. The alternatives are mutually exclusive. Function-result declarations similarly offer `response` or `responseJsonSchema`. Parameterless functions can omit their schema. [REST reference][function-rest]

Issue #619 is consistent with a JSON Schema keyword reaching the typed protobuf Schema parser. Using the JSON-value field addresses that parser mismatch without deleting constraints. It does **not** prove that a selected model enforces every constraint. Request acceptance and generated-argument conformance are separate criteria.

Google's Go Gen AI SDK exposes JSON Schema fields as `any`, independently of its typed Schema. No SDK migration is needed to express the wire contract. [Go SDK types][go-types]

## Compatibility matrix

| Surface | Official contract | Implemented scope and qualification |
| --- | --- | --- |
| Function input | JSON-value `parametersJsonSchema` and typed `parameters` are alternatives. | Native Vertex emits original caller schemas in the JSON-value field; never both fields. |
| Nested const, enum, composition and annotations | JSON-value transport can represent arbitrary keys; this is not an enforcement guarantee. | Preserve nesting/values without enum conversion or field stripping. Live model enforcement is unverified. |
| Typed Schema | Includes properties, items, anyOf, nullable, additionalProperties, ref and root defs; enum values are strings. const, oneOf and allOf are not declared fields. | Do not interchange with JSON Schema or rename arbitrary property names. [Typed reference][schema-rest] |
| JSON references and nullable unions | JSON-value transport retains $ref, $defs, type arrays and composition. | Preserve syntax; resolution and model semantics remain backend concerns. No external references are fetched. |
| Structured output | `responseJsonSchema` replaces `responseSchema` and requires `responseMimeType`. | Restore original schema and omit typed field. Native Vertex Claude response-format conversion also follows this path. |
| Responses schema ingress | Gateway conversion policy, separate from Google REST. | Native Vertex keeps function/output schemas, including omitted strict, through namespaces, discovery and tool_search paths. Other providers retain their existing sanitizers. |
| Responses result ingress | Gateway conversion policy, independent of schema fidelity. | Native Vertex rejects orphan or mismatched function outputs before dispatch; other providers retain their existing downgrade compatibility. |
| Function result declaration | v1/v1beta1 expose `responseJsonSchema`. | Public OpenAI-compatible function model has no separate result-schema declaration; no new result-schema extension. |
| Tool selection | AUTO, ANY, NONE and VALIDATED exist. | Vertex maps auto/default, none, required and named choices; required becomes ANY, named choice becomes ANY with an allowed name. Legacy function_call mapping is included. VALIDATED has no exact standard tool_choice equivalent. |
| Tool-result history | Native functionResponse requires a name and object response. | Correlate results to pending calls by source identity, retain restored native parallel call order, group results as user functionResponse Parts. Internal source indices survive positional restoration and never appear on the wire; reversed physical tool-call arrays retain their own arguments, signatures and results, including repeated function names. Scalar/array/null/text output uses an output wrapper; explicit Claude error status uses an error wrapper. |
| Call IDs | v1beta1 exposes upstream call/result IDs; v1 does not. | Downstream IDs correlate gateway history; no unsupported IDs are sent to native v1. |
| Thought signatures | Preserve opaque signatures on their original Parts. | Chat supports per-tool, per-message and per-content-item Google metadata. Responses/Claude carry the same keys through gateway extensions; those extensions are not official Responses/Anthropic APIs. Metadata must survive client retention and same-model replay. Signed/empty/text/mixed-order Chat and Claude replay and positional validity controls passed targeted tests. |
| Complete-call streams | Complete args differ from opt-in partialArgs. | Complete calls get distinct downstream indices across chunks/candidates; terminal finish mapping reflects tool calls. Nonempty or malformed partialArgs and true/malformed willContinue fail in a controlled way with any decoded authoritative usage receipt retained; omitted/null fields, empty partialArgs arrays and false willContinue defaults are accepted. Incremental argument streaming is not enabled. |
| Multimodal tool results | Google exposes function-response media Parts. | Native multimodal tool results are explicitly rejected rather than silently flattened. |
| Signed media / mixed content | Signature position and original Part matter. | Signed-media replay is not qualified. Internal Claude block order and a bounded gateway position extension preserve tested mixed text/tool turns. Late signatures after unpositioned no-tools scalar chunks reject incomplete replay in a controlled way. |
| Plain chat and non-Vertex Gemini | Different contracts. | Existing paths retained; final validation evidence is recorded separately. Vertex embedding protocol differences are documented in the separate model audit and are not qualified by these schema tests. |
| Live, partner models, Interactions | Different protocol/model contracts. | Existing support is not requalified here. No universal GCP compatibility claim. |

Both v1/v1beta1 proto definitions confirm the JSON-value schema alternatives. The v1beta1 definition additionally exposes IDs. Both define partial-argument fields and explicit argument-streaming configuration; wire capability does not mean this gateway implements that optional protocol. Function responses accept an object whose output/error keys have defined meanings. [v1 definition][tool-v1], [v1beta1 definition][tool-beta]

For structured `responseJsonSchema`, v1 specifies a subset: references/definitions, type/format/title/description, string or number enum, items/prefixItems and array limits, numeric bounds, anyOf/oneOf, object properties/additionalProperties/required, and propertyOrdering. It interprets oneOf as anyOf. Cyclic references have limited expansion and must occur in non-required properties; referenced subschemas restrict siblings. This does not establish const, allOf or not enforcement. [GenerationConfig source][content-v1]

The structured-output guide describes a narrower typed responseSchema subset, says unsupported fields can be ignored, and warns complexity can cause HTTP 400. Transport preservation must therefore be distinguished from constraint enforcement. [Structured-output guide][structured]

## Signatures, history and streams

Official OpenAI-compatibility examples place extra_content on string-content messages, individual content-array items, and individual tool calls. The overview defines google.thought_signature separately from google.thought. Chat uses these documented carriers. Gateway extensions using the same Google metadata keys on Responses or Claude records are conversion mechanisms, not claims that Google or Anthropic specifies those formats. [Carrier examples][openai-examples], [Carrier fields][openai-overview]

The signature guide requires relevant function-call history signatures and preserves Part boundaries. Non-function final text signatures, including empty streaming text Parts, are recommended for continuity; omission does not have the same documented HTTP 400 rule. Parallel calls precede their ordered results. Never fabricate signatures, combine opaque signatures, or substitute Anthropic thinking signatures. [Signature guide][thoughts]

The native adapter retains v1 generateContent and streamGenerateContent endpoints, whose bindings are in the official service definition. [Endpoint definition][prediction-v1] The function-calling guide lists Gemini 3.8 Flash and describes optional partialArgs/willContinue streaming for Gemini 3+ except Gemini 3.1 Flash-Lite. That optional protocol remains unsupported here. [Function-calling guide][function-guide]

The internal Message.NativePartOrder carrier preserves ordered Claude blocks during conversion. The projected Chat carriers extra_content.oneapi.gemini_part_index and gemini_text_present are **gateway extensions**, not official Google fields. The optional presence marker distinguishes an explicit empty text oneof from a metadata-only Part. The index carries bounded positions across text/tool kinds; native replay requires complete, unique, contiguous positions from zero before reconstructing ordered Parts and removes the extension before upstream transmission. Clients must retain the metadata and replay it to the same model. Targeted signed/empty/text/mixed-order native Chat and Claude tests and positional validity controls passed. Actual-controller native-to-Responses-to-native JSON/SSE tests preserved the tested four-Part mixed turn.

Unsigned scalar Chat responses without tools retain their existing plain shape. Unsigned Developer API responses, including tool calls, retain their existing wire shape; proactive tool-capable positioning is scoped to native Vertex. Signed image replay still lacks qualification for native fileData/inlineData representation changes. For tool-capable actual streams, the converted request enables cumulative per-candidate positions from the first frame. Without tool context, unsigned scalar frames emitted before later signed text cannot recover the earlier Part boundaries: incomplete positional replay rejects in a controlled way rather than guessing.

Remaining limits include signed-media replay, recovery of boundaries after unpositioned no-tools scalar chunks, richer native multimodal function results, optional incremental argument streaming, and model-specific schema enforcement. Tested mixed-order conversions do not certify every possible media/interleaving combination.

Stored opaque Google signatures automatically replay on native Google only when the existing record binding matches the selected channel, API family and actual model. Ancestor response records are checked before flattening. Unknown origin, including a standalone item reference without model provenance, returns state_not_portable rather than forwarding opaque state. Unsigned canonical history and gateway ordering metadata remain portable without a binding. This is a conversion-compatibility check using existing records; storage schema, ownership and authentication are unchanged. Explicit stateless caller history remains caller-supplied and backend validation stays authoritative.

The final ordering regression first failed when valid caller positions reversed two tool calls: arguments followed physical array order while signatures followed restored native order. An ordinary-order control passed. The fixed path binds each native Part to its original call before restoring order, with repeated-name and large-integer precision controls, ordered result checks, source immutability and no upstream v1 ID leakage.

The [dated Vertex model audit](vertexai-model-audit-20261112.md) distinguishes the refreshed advertised catalog from retained legacy routing, protocol support and entitlement.

## Evidence limits

No paid live API calls were made and no credentials were created. Offline mock transport and regression tests establish only the shapes, exclusivity, preservation, controls and response/stream fixtures they exercise. They cannot prove a real model accepts or enforces a keyword. Root qualification exercised actual-controller stateless two-turn JSON/SSE continuation, and adapter qualification exercised actual-controller stateful previous_response_id JSON/SSE continuation. Those passed without state/security implementation changes. Targeted signature, empty-Part, mixed-order and position-validity checks also passed. These fixtures remain narrower than live model qualification. Consult the final PR evidence for broader suites, races, quick checks and vet; this note does not infer their results from targeted passes.

Future authorized live qualification should cover the reported model/region, original nested const schema, plain-chat and enum controls, negative instances, generated argument conformance, and tool-result continuation.

Historical fileio task access was permission-denied in the parent session; no verified fileio closeout is claimed.

[function-rest]: https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest/Shared.Types/FunctionDeclaration
[schema-rest]: https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest/v1/Schema
[go-types]: https://github.com/googleapis/go-genai/blob/main/types.go
[tool-v1]: https://github.com/googleapis/googleapis/blob/master/google/cloud/aiplatform/v1/tool.proto
[tool-beta]: https://github.com/googleapis/googleapis/blob/master/google/cloud/aiplatform/v1beta1/tool.proto
[content-v1]: https://github.com/googleapis/googleapis/blob/master/google/cloud/aiplatform/v1/content.proto
[prediction-v1]: https://github.com/googleapis/googleapis/blob/master/google/cloud/aiplatform/v1/prediction_service.proto
[function-guide]: https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/tools/function-calling
[structured]: https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/capabilities/control-generated-output
[thoughts]: https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/thinking/thought-signatures
[openai-examples]: https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/migrate/openai/examples
[openai-overview]: https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/migrate/openai/overview
