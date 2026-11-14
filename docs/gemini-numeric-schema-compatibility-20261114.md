# Numeric JSON Schema bounds compatibility — November 14, 2026

This records the contract evidence and bounded compatibility policy for PR #622.
The issue is gateway loss of supported schema constraints, not a claim that every
backend implements the complete JSON Schema specification.

## Destination policy

Policy is selected from the effective mapped upstream model and channel, rather
than the ingress protocol or a model-looking caller alias.

| Destination/profile used by shared normalization | Inclusive minimum/maximum | Exclusive minimum/maximum | Qualification |
| --- | --- | --- | --- |
| OpenAI: registered model with the structured_outputs feature | Preserve | Preserve | Current standard-model documentation supports all four. |
| OpenAI: fine-tuned ID or unknown/unregistered profile | Conservative legacy adaptation | Conservative legacy adaptation | Fine-tuned models have documented additional restrictions; unknown capabilities are not inferred from a familiar name. |
| Gemini-compatible/native Gemini/Vertex: qualified model in the shared Gemini registry with structured_outputs | Preserve | Conservative legacy adaptation | Google documents inclusive bounds. The inspected subset does not establish exclusive-bound support. |
| Azure OpenAI | Conservative legacy adaptation | Conservative legacy adaptation | Azure's documented schema subset still excludes numeric minimum/maximum. OpenAI's current support does not establish Azure deployment support. |
| Other/unverified destinations | Conservative legacy adaptation | Conservative legacy adaptation | No broader support claim is made. |

“Conservative legacy adaptation” means the existing numeric-keyword removal
policy remains in that path. It is not evidence that every unknown endpoint
rejects those keywords. Provider-only or custom profiles outside the qualified
registry require explicit qualification before receiving a stronger capability
claim.

The table describes the shared normalizer. Existing native-format provenance
can preserve a caller's complete schema through its already-qualified conversion
path; preserving a keyword on the wire does not establish backend enforcement.

### Google evidence

The [Gemini structured-output subset](https://aistudio.google.com/docs/structured-output#json-schema-support)
explicitly supports inclusive numeric bounds. Its current examples use
Interactions; the [compatible structured-output guide](https://aistudio.google.com/docs/openai#structured-output)
links to that subset. The
[GenerateContent Schema reference](https://aistudio.google.com/api-reference/generate-content#Resource:Schema)
independently exposes minimum and maximum, and FunctionDeclaration.parameters
uses that schema. Vertex's
[typed Schema reference](https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest/v1/Schema)
also exposes inclusive numeric bounds. These sources do not document exclusive
bounds in the inspected typed subset.

This establishes inclusive-bound preservation, not every Google model, partner
family, region, JSON-schema carrier or grammar mode. The separate native Vertex
component qualification verifies conversion fidelity; it is not live model
entitlement or constrained-decoding qualification.

### OpenAI, Azure and Anthropic differences

The current [OpenAI supported-schema guide](https://developers.openai.com/api/docs/guides/structured-outputs#supported-schemas)
documents both inclusive and exclusive numeric bounds for standard models, with
additional restrictions for fine-tuned models. Historical blanket OpenAI stripping
is therefore not a valid compatibility assumption. Registered standard profiles
are distinguished from fine-tuned and unverified destinations.

[Azure's structured-output subset](https://learn.microsoft.com/en-us/azure/foundry/openai/how-to/structured-outputs#unsupported-type-specific-keywords)
lists minimum and maximum as unsupported numeric keywords. Its root
additionalProperties behavior is retained independently of the new numeric
capability policy.

[Anthropic's native structured-output limitations](https://platform.claude.com/docs/en/build-with-claude/structured-outputs#json-schema-limitations)
exclude numerical constraints. Its SDK can place unsupported constraints in
descriptions and validate responses locally; that is not equivalent to native
grammar enforcement. A Claude tool schema received by one-api is an ingress
schema: its handling must follow the actual destination, rather than treating
all Claude-shaped requests as Anthropic upstream traffic.

## Schema semantics and fidelity

Schema traversal must distinguish schema nodes from dictionaries of property
names and literal values. A property named minimum, a required entry with that
name, or the same text in enum/default/const data must not disappear merely
because a numeric keyword has the same spelling. Normalization owns changed
branches rather than modifying caller-owned schema trees.

Modern [JSON Schema numeric ranges](https://json-schema.org/understanding-json-schema/reference/numeric#range)
use numeric exclusive thresholds. Draft 4 and
[OpenAPI 3.0](https://swagger.io/docs/specification/v3_0/data-models/data-types/#minimum-and-maximum)
instead use boolean exclusivity attached to inclusive bounds. These forms are
not interchangeable. This change does not invent floating-point epsilon,
coerce boolean true to numeric one, or claim undocumented Google exclusivity
support.

Schema decoding must retain exact JSON number tokens rather than rounding
through float64. Tests use integer bounds above 2^53 and long decimal exclusive
bounds. Gateway token fidelity does not establish arbitrary-precision storage or
constraint enforcement inside an upstream provider.

## Qualification evidence

The permanent controller test
`openai_numeric_bounds_transport_test.go` exercises both actual Chat
response_format to native Responses conversion and native Responses text.format.
It uses a mapped gpt-6.1-sol destination, valid completed Responses receipts,
schema-conforming mock output, raw source ownership and exact quota settlement.
A strict test-only transport sends the logical OpenAI endpoint exclusively to a
local TLS fixture; it never contacts the live provider.

The authoritative pre-policy failure receipt is
`bounds-openai-transport-red-corrected.log`. Both paths lost all four bounds and
a property named minimum while transport, mapping, source and billing controls
passed. The first native-Responses fixture attempt used an unqualified localhost
provider identity and selected fallback; that route failure is explicitly
excluded from production-defect evidence.

Gemini ingress/MCP and native Vertex component evidence is retained separately.
Provider policy, source ownership, promotion, explicit-format precedence and
unrelated/fine-tuned/unknown controls remain necessary acceptance checks.

No paid/live API, new credentials, production or security changes were used for
qualification. Full database/race and frontend/browser matrices are separate.
FileIO history capability remains unavailable; no FileIO search, save or verified
retrieval is claimed. Earlier JSON-reconstruction provenance and unexplained
DeepSeek qualification caveats remain in the PR history.

Stored MCP expansion and deferred tool injection had two additional schema-only
float64 decode sites. Actual two-round wire and deferred-carrier tests reproduced
rounding of odd large integer bounds before correction. Those sites now use the
existing exact MCP JSON decoder; argument decoding remains unchanged. This does
not add numeric-bound enforcement to MCP's best-effort argument validator.
