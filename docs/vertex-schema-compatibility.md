# Vertex Gemini schema compatibility

Research date: **2026-11-12 UTC**. Scope: the native Vertex Gemini **GenerateContent v1** adapter. This is not certification of all Google Cloud products, model families, regions, historical versions, Live API or Interactions API.

## Contract boundary

OpenAI function parameters are JSON Schema. Vertex offers two distinct wire contracts: typed `parameters` uses its OpenAPI-subset `Schema`, while `parametersJsonSchema` is a protobuf JSON Value containing a JSON Schema object. These fields are mutually exclusive. Function-result declarations similarly offer mutually exclusive `response` and `responseJsonSchema`. Parameterless functions can omit their schema. [REST reference][function-rest]

Issue #619 is consistent with a JSON Schema keyword reaching the typed protobuf Schema parser. Routing through the JSON-value field addresses that parser mismatch without deleting constraints. It does **not** prove that a selected model enforces every constraint. Request acceptance and generated-argument conformance are separate criteria.

Google's Go Gen AI SDK likewise exposes the JSON Schema fields as `any`, independently of its typed Schema. No SDK migration is needed to express the wire contract. [Go SDK types][go-types]

## Compatibility matrix

| Surface | Official contract | Adapter scope and qualification |
| --- | --- | --- |
| Function input | JSON-value `parametersJsonSchema` and typed `parameters` are alternatives in v1/v1beta1. | Use the JSON-value field for original caller schemas; never emit both fields. |
| Nested const, enum, composition and annotations | Arbitrary keys fit JSON-value transport; this is not an enforcement guarantee. | Preserve values/nesting without enum conversion or field stripping. Live model enforcement is unverified. |
| Typed Schema | Includes properties, items, anyOf, nullable, additionalProperties, ref and root defs; enum values are strings. const, oneOf and allOf are not declared fields. | Do not interchange this with JSON Schema or rename arbitrary property names. [Typed reference][schema-rest] |
| JSON references and nullable unions | JSON-value transport retains $ref, $defs, type arrays and composition syntax. | Offline fixtures can verify preservation; resolution and model semantics remain backend concerns. No references are fetched by this fix. |
| Structured output | `responseJsonSchema` replaces `responseSchema` and requires `responseMimeType`. | Restore the caller schema into the JSON-value field and omit the typed field. |
| Function result declaration | v1/v1beta1 expose `responseJsonSchema`. | The public OpenAI-compatible function model has no separate result-schema declaration. A new public extension is out of scope. |
| Plain chat, embeddings and non-Vertex Gemini | Separate request contracts. | Retain existing behavior and qualify with controls. |
| Tool selection | AUTO, ANY, NONE and VALIDATED are defined. | Existing converter handles none and named functions. String required currently falls through to default behavior; complete mode mapping needs follow-up. |
| Tool-result history | Function responses contain structured JSON; v1beta1 supports upstream call IDs. | Current native Part lacks functionResponse. Full result-history conversion is not claimed. |
| Thought signatures | Gemini 3 requires signatures for relevant function-call history. | Current native Part lacks thoughtSignature. Cross-format preservation needs follow-up. [Signature guide][thoughts] |
| Streamed calls | Complete args and opt-in incremental partialArgs are distinct protocols. | Existing complete-argument conversion remains. Incremental argument accumulation and stable upstream identity are out of scope. |
| Live, partner models, Interactions | Different protocol/model contracts. | Existing support is not requalified here. No universal GCP compatibility claim. |

Both official function proto versions independently confirm the JSON-value fields. v1beta1 additionally exposes call/result IDs and incremental argument fragments. Adjacent features are not necessarily identical across versions. [v1 definition][tool-v1], [v1beta1 definition][tool-beta]

For structured `responseJsonSchema`, the official v1 definition specifies a subset: references/definitions, type/format/title/description, string or number enum, items/prefixItems and array limits, numeric bounds, anyOf/oneOf, object properties/additionalProperties/required, and propertyOrdering. It interprets oneOf as anyOf. Cyclic references have limited expansion and must occur in non-required properties; referenced subschemas restrict siblings. The list does not establish const, allOf or not enforcement. [GenerationConfig source][content-v1]

The structured-output guide describes a narrower typed responseSchema subset, says unsupported fields can be ignored, and warns complexity can cause HTTP 400. Transport preservation must therefore be distinguished from full constraint enforcement. [Structured-output guide][structured]

## Existing behavior audit

At the reviewed base, relay/adaptor/vertexai/adaptor.go uses native v1 generateContent or streamGenerateContent endpoints. Vertex Gemini delegates content, tool choice and response handling to shared Gemini conversion. relay/adaptor/gemini/model.go models FunctionCall with name and args only; response conversion synthesizes OpenAI call IDs. These are static code findings, not live backend qualification.

The function-calling guide lists Gemini 3.8 Flash as supporting function calls. It distinguishes GenerateContent OpenAPI ref/defs from Interactions JSON Schema references, with bounded nesting and recursion. Optional argument streaming applies to Gemini 3+ in v1/v1beta1 except Gemini 3.1 Flash-Lite, using partialArgs/willContinue. [Function-calling guide][function-guide]

The Responses ingress has an existing cross-provider sanitizer before the Vertex adapter. Function tools always pass through responseFunctionToolToChat, which clears strict and removes $schema, additionalProperties at all levels, and root description/strict. For output text.format, omitted strict selects that sanitizer, while explicit strict true or false selects the existing lossless-clone path. The Vertex change preserves the schema it receives; it cannot recover constraints already removed by earlier conversion. Regression tests distinguish this boundary from adapter preservation. Claude Messages tool input_schema follows its native conversion path; general Claude output configuration is not requalified here.

Prioritized follow-ups: functionResponse history; thought signatures across Chat Completions, Responses and Claude Messages; complete tool-mode mapping; incremental argument streaming and stable identities. Qualify these independently before claiming comprehensive tool behavior.

## Evidence limits

No paid live API calls were made and no credentials were created. Offline mock transport tests establish only the emitted shapes, exclusivity, preservation, controls and response/stream fixtures they exercise. They cannot prove a real model accepts or enforces a keyword.

Future authorized live qualification should cover the reported model/region, the original nested const schema, plain-chat and enum controls, negative instances, generated argument conformance, and tool-result continuation.

Historical fileio task access was permission-denied in the parent session; no verified fileio closeout is claimed.

[function-rest]: https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest/Shared.Types/FunctionDeclaration
[schema-rest]: https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest/v1/Schema
[go-types]: https://github.com/googleapis/go-genai/blob/main/types.go
[tool-v1]: https://github.com/googleapis/googleapis/blob/master/google/cloud/aiplatform/v1/tool.proto
[tool-beta]: https://github.com/googleapis/googleapis/blob/master/google/cloud/aiplatform/v1beta1/tool.proto
[content-v1]: https://github.com/googleapis/googleapis/blob/master/google/cloud/aiplatform/v1/content.proto
[function-guide]: https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/tools/function-calling
[structured]: https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/capabilities/control-generated-output
[thoughts]: https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/thinking/thought-signatures
