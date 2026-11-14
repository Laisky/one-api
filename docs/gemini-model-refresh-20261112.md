# Gemini model refresh — November 12, 2026

This refresh adds the published Gemini 4 Carbon and Hydrogen IDs to the shared
catalogue used by the Gemini Developer API native and OpenAI-compatible adaptors.
It also fixes reasoning controls that those adaptors previously discarded and
updates lifecycle descriptions from Google's November notices.

## Verified model contracts

| Model | Status | Shared context | Discovery output limit | Reasoning levels | Default |
| --- | --- | ---: | ---: | --- | --- |
| `gemini-4-carbon` | GA November 10 | 1,048,576 | 32,768 | low, medium, high | medium |
| `gemini-4-hydrogen` | GA November 12 | 1,048,576 | 65,536 | low, medium, high, xhigh | high |
| `gemini-4-argon` | Existing stable ID | 1,048,576 | 32,768, unchanged | low, medium, high | medium |

These models accept text, images, audio, video, and PDFs and produce text.
Discovery includes function tools, JSON/structured output, search, and reasoning;
it does not register image generation, audio generation, or Live transports.
Only implicit caching is supported. Numeric thinking budgets are not supported.

The generic Developer API cards describe 65,536-token synchronous output, while
the GenerateContent model comparison labels Carbon/Argon output as **64K stream**.
Google Cloud's model-specific GenerateContent documentation distinguishes a
32,768-token non-streaming limit from 65,536 streaming. The shared catalogue has
one output-limit field, so Carbon follows the existing conservative Argon value;
Hydrogen has a 65,536 foreground limit for both transports. Explicit streaming
requests are not clamped to the discovery value. A 1M-token output requires
continuation/long decode; this refresh does not implement that protocol.

Sources, checked November 12:

- [Gemini release notes](https://aistudio.google.com/docs/changelog)
- [GenerateContent model comparison](https://aistudio.google.com/docs/latest-model?generate-content=true)
- [Carbon model card](https://aistudio.google.com/docs/models/gemini-4-carbon)
- [Cloud Carbon limits](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-carbon)
- [Cloud Hydrogen limits](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-hydrogen)

## Standard Developer API pricing

Prices are USD per million tokens. Output includes thinking tokens.

| Model | Input through November 30 | Output through November 30 | Cached input through November 30 | From December 1 |
| --- | ---: | ---: | ---: | --- |
| Carbon | 2.00 | 10.00 | 0.10 | 4.00 / 20.00 / 0.20 |
| Argon, existing | 2.00 | 10.00 | 0.10 | 4.00 / 20.00 / 0.20 |
| Hydrogen | 1.50 | 7.50 | 0.075 | Unchanged |

Carbon uses the existing date-window resolver, switching at December 1, 2026,
00:00 UTC. Hydrogen has no invented expiry or inherited Flash/Argon promotion.
Neither new model inherits the older Pro family's long-context surcharge.
The search tariff remains $14 per 1,000 queries for Gemini 3 and later.
Batch, Flex, Priority, and account-specific adjustments are separate from these
standard defaults.

Sources: [Developer API pricing](https://aistudio.google.com/docs/pricing) and
[Cloud pricing](https://cloud.google.com/model-garden/pricing).

## Request behavior

Native conversion selects the mapped upstream model before reading its reasoning
contract. It forwards `reasoning_effort` or `reasoning.effort` as native
`thinkingConfig.thinkingLevel`, applies the model default when omitted, and
rejects conflicting or unsupported controls. Hydrogen accepts `xhigh`;
Carbon/Argon do not. Ignored temperature/top-p/top-k controls are omitted from
native payloads without modifying caller-owned requests. System instructions
remain native system instructions.

The Gemini OpenAI-compatible channel previously applied OpenAI-only reasoning
detection, which removed Gemini's requested effort. Its narrowly scoped Gemini 4
path preserves and validates effort without applying OpenAI's historical
system-message filtering. Claude `output_config.effort` and lowered Responses
reasoning use the same published model contract. Unsupported provider-specific
thinking controls fail clearly. Other providers and unpublished model IDs keep
their existing transformations.

Google's [OpenAI compatibility mapping](https://aistudio.google.com/docs/openai#thinking)
accepts OpenAI `minimal` effort as Gemini 4 `low`. The compatible adaptor therefore
normalizes that alias to `low`; it does not send an unsupported native
`thinkingLevel: minimal`. Native conversion retains the model's strict level
validation. Omitted/default compatible effort remains omitted, allowing the
upstream to apply its documented model default.

## Lifecycle and compatibility

| Existing ID | Verified Developer API change | Local behavior |
| --- | --- | --- |
| `gemini-2.5-pro` | Deprecated; upstream redirects to `gemini-3.8-flash` on December 1 | Retain ID and historical tariff; update migration advice |
| `gemini-2.5-flash` | Deprecated; upstream redirects to `gemini-3.5-flash-lite` on December 1 | Retain ID and historical tariff; update migration advice |
| `gemini-embedding-2-preview` | Redirects to stable `gemini-embedding-2` since November 6 | Retain compatible schema, embedding space, and price |
| `gemini-omni-1.1-flash` | Redirects to `gemini-omni-1.2-flash` on December 3 | Record migration; retain historical registration |

Use the replacement IDs explicitly before changing upstream contracts or tariffs.
Lifecycle descriptions do not silently rewrite configured models or adopt a new
tariff for existing channels. Developer API redirects are not assumed to apply
to Vertex. Stable Embedding 2 was already registered with an 8,192-token context;
this change does not add duplicate entries or require reindexing existing vectors.
No unverified `*-latest` aliases are introduced.

Sources: [deprecation tables](https://aistudio.google.com/docs/deprecations),
[release notes](https://aistudio.google.com/docs/changelog), and
[Embedding 2 model card](https://aistudio.google.com/docs/models/gemini-embedding-2).

## Related surfaces and boundaries

The existing Vertex registry inherits the two shared IDs and routes them through
its native Gemini adaptor at the global endpoint. Carbon's Vertex defaults use
the gross Standard tariff of $4 input / $20 output / $0.20 cached per million
tokens, with no Developer API promotional window. Cloud introductory credits
are account-level adjustments; regional/account rates can differ. This override
does not mutate the Developer catalogue or change existing Argon pricing policy.
The larger independent Vertex catalogue work is tracked in
[PR #621](https://github.com/Laisky/one-api/pull/621). Regressions check Carbon's
provider-specific tariff, discovery, and dispatch without assuming every provider
has the same price.

`gemini-decision-1` requires the Decisions API, and `gemini-omni-1.2-flash` requires
video generation/editing support. Neither is advertised as a new chat model.
Published Cyber variants are Vertex-specific and belong to that separate
catalogue integration. This refresh does not add an entitlement gate, invent
thought signatures, or implement long decode, Decisions, voice management, or
new video protocols.

## Regression coverage

The new tests exercise catalogue discovery through native Gemini, Gemini
OpenAI-compatible, and Vertex consumers; explicit pricing and the UTC date
boundary; model-config isolation; system placement; mapped aliases; valid and
invalid efforts; omitted sampling fields; Claude/Responses lowering; and
unchanged unrelated-provider behavior. Missing catalogue entries and dropped
reasoning were reproduced before their fixes. Existing lifecycle assertions are
updated to Google's superseding November notice while preserving tariff checks.

Tests are offline. They verify conversion, registration, and billing behavior;
they do not establish live provider availability for a particular account.

## Review repair and main integration — November 14, 2026

The Gemini 4 OpenAI-compatible conversion previously read only
`output_config.effort` and removed the complete native control object. An offline
regression through the actual Responses fallback and final Chat serialization
reproduced a successful payload with its schema missing. The independently
translated `text.format` path was a false-positive control.

| Caller control | Gemini 4 compatible behavior |
| --- | --- |
| `output_config.format` with `type: json_schema` and object schema | Lower to `response_format.json_schema`; generate the name `response` if absent and default strict to true. Preserve explicit strict, all schema keywords and exact JSON numeric values. |
| Equivalent explicit output format carriers | Accept identical serialized constraints; reject competing formats. Representations differing in metadata/defaults can be conservatively rejected. |
| Unknown native output controls, malformed or unrepresentable formats | Return a validation error before clearing controls. |
| `reasoning.summary` | Reject nonblank values; the documented Google include-thoughts extension has no equivalent concise/detailed selection contract. Nil/blank remains accepted. |
| `temperature`, `top_p`, `top_k` | Omit consistently for the registered Gemini 4 compatible contract, matching native fixed-sampling behavior. Live rejection of top_k was not tested. |
| Legacy models and unrelated provider channels | Retain their existing control behavior. |

The lowered native schema bypasses the legacy cross-provider schema sanitizer:
removing a numeric bound would silently weaken the constraint. Existing
`text.format` and unrelated-provider normalization contracts remain unchanged.
Decoding owns a new schema tree and uses JSON numbers rather than float64.
The native Claude ingress and Responses fallback share this scoped conversion;
the caller's native raw configuration remains unchanged.

Official references checked November 14, 2026:

- [Google OpenAI compatibility: structured output and thinking](https://aistudio.google.com/docs/openai)
- [Claude structured outputs: native output_config.format shape](https://platform.claude.com/docs/en/build-with-claude/structured-outputs)

Latest main `148ea1dc431c8c846cfa8f53ef20c9c24a59958c` was merged rather than
rebased. The system-instruction catalog conflict retains both the shared
Carbon/Hydrogen constants and main's Vertex/Cyber entries. Catalog integration
tests distinguish IDs available on both APIs from Vertex-only variants and
continue to verify independent metadata ownership and provider pricing.

Qualification is offline. Schema preservation in the upstream payload does not
prove every schema keyword is accepted or enforced by every Google deployment.
No live account entitlement, regional availability, paid API call or provider
billing qualification was performed. Full database/race and frontend/browser
qualification are separate from the focused backend checks.

## Follow-up: repeated MCP conversion and native enum output — November 14

The review of `edad62b0` identified two independently reproduced wire defects:

- **Repeated compatible conversion (P2):** the first conversion moved a native
  schema into `response_format` and cleared `output_config`. A later conversion
  recomputed its policy from the now-empty source field and removed bounds.
  The actual MCP loop reproduced this on its second local provider request:
  round one retained minimum/maximum, round two lost them.
- **Native enum casing (P1):** the native Argon/Carbon/Hydrogen converter emitted
  lowercase thinking levels. The current
  [GenerateContent ThinkingConfig reference](https://aistudio.google.com/api-reference/generate-content)
  specifies uppercase `LOW`, `MEDIUM`, `HIGH`, and `XHIGH`; the
  [official Go SDK enum](https://github.com/googleapis/go-genai/blob/main/types.go)
  agrees. Corrected serialized-payload tests on exact `edad62b0` reproduced the
  mismatch. The first test attempt used an incorrect fixture decoding key and
  is explicitly excluded from reproduction evidence.

Lossless-schema provenance now lives on the actual in-memory JSONSchema object,
scoped to its destination channel. It follows typed request reuse and shallow
copies, is absent from JSON, and cannot be supplied by caller JSON. Replacing
the schema drops the provenance. Normalization still checks the exact Gemini 4
compatible channel/model guard; ordinary formats and other providers keep
their existing behavior. This adapts supported native schemas through repeated
MCP rounds without rejecting them.

The provenance is intentionally not a public wire extension or persistence
format. JSON serialization/reconstruction does not carry it; current typed MCP
paths do not cross that boundary. Any future internal reconstruction must carry
trusted provenance explicitly rather than infer it from caller fields.

Native generation now uppercases only the emitted Gemini 4 enum. Input parsing,
mapped-model selection, defaults and validation stay unchanged. The compatible
transport still emits lowercase `reasoning_effort`. Tests cover all three
registered native models, aliases, omitted/default/empty/lowercase/uppercase/
spaced/nested inputs, and unrelated native/compatible controls. Live provider
acceptance or rejection was not exercised.

The prior unexplained DeepSeek concurrent-funding qualification failure remains
recorded in the PR description and local evidence; this follow-up does not relabel it as a proven flake or weaken
its assertions. FileIO remains unavailable: no history search/save or verified
retrieval is claimed for `one-api`, `622`, `Gemini4`, `output_config`,
`MCP repeated conversion`, `schema sanitizer`, or `thinkingLevel`.
