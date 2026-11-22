# Frontier adaptor model audit — 2026-11-22

This audit checks official provider documentation against the November 16
catalogue and the request paths present in this repository. Prices are USD per
million tokens unless another unit is stated. Provider availability, billing
rates, and relay implementation limits are evaluated separately. Existing model
IDs and operator overrides are retained.

## Results

| Adaptor | Result |
| --- | --- |
| OpenAI | Add `gpt-6.2-astra` and `gpt-6.2-sol`, their exact Standard tariffs, input-only long-context tiers, reasoning metadata, and existing native billing policies. |
| Gemini | Add `gemini-4-neon` to native discovery and the existing Gemini 4 system-instruction/reasoning conversion. |
| Gemini OpenAI compatible | Add the same verified Developer API model and tariff, including the existing compatible effort normalization and search pricing. |
| Vertex AI | Add Neon and the now-published Argon Cyber global Standard tariff on existing GenerateContent routes. Qualify both for at most 32,768 output tokens because longer failed requests have a separate paid-usage contract. |
| Azure | Verify GPT-6.2 deployment support and wire explicitly configured GPT-6.2 deployments to Responses. Do not inherit unverified native prices into host discovery/defaults. |
| AIProxy | Retain the legacy library API catalogue; exclude the new native GPT-6.2 prices because its own public contract could not be retrieved. |
| Anthropic | Reviewed; existing Fable/Mythos 5.2 and Opus/Sonnet/Haiku 5.5 entries remain current. |
| AWS Bedrock | Reviewed; latest document-history changes do not add a model compatible with the currently implemented native subadaptors. |

## OpenAI GPT-6.2

The [November 17 changelog](https://developers.openai.com/api/docs/changelog)
publishes the two new IDs. Their model cards list text/image input, text output,
a 1,050,000-token context window, a maximum 922,000-token input, and maximum
128,000-token output. Maximum input is recorded in the description because the
shared schema has no separate input-limit field.

| Native model and input length | Input | Output, including reasoning | Cache read | 30m cache write |
| --- | ---: | ---: | ---: | ---: |
| `gpt-6.2-astra`, input ≤272,000 | 8 | 40 | 0.40 | 10 |
| `gpt-6.2-astra`, input >272,000 | 16 | 60 | 0.80 | 20 |
| `gpt-6.2-sol`, input ≤272,000 | 2 | 10 | 0.10 | 2.50 |
| `gpt-6.2-sol`, input >272,000 | 4 | 15 | 0.20 | 5 |

Sources: [Astra model card](https://developers.openai.com/api/docs/models/gpt-6.2-astra.md),
[Sol model card](https://developers.openai.com/api/docs/models/gpt-6.2-sol.md), and
[native pricing table](https://developers.openai.com/api/docs/pricing.md).
The long-context tariff applies to the **whole request**. It begins at 272,001
input tokens; output tokens do not select that tier. The legacy
`CacheWrite5mRatio` field carries OpenAI's 30-minute write price, consistent with
the existing OpenAI cache accounting. The published 12-hour prices are not put
in `CacheWrite1hRatio`, whose meaning is Anthropic's one-hour write bucket.

### Request and billing qualification

The [deployment checklist](https://developers.openai.com/api/docs/guides/deployment-checklist)
explicitly lists `low`, `medium`, `high`, `xhigh`, and `max` for both models.
Neither supports `none`; `minimal` is not in the supported ladder. The relay
uses its existing `medium` fallback for omitted or unsupported reasoning
efforts. This is the gateway's configured default, consistent with OpenAI's
recommended starting point, rather than a claim about every upstream default.
The existing catalog-driven serializer removes incompatible sampling/logprob
parameters while retaining tools and reasoning-encrypted-content includes.

Both upstream models support Chat Completions for text, but **tool calling
requires Responses**. The existing native Chat/Claude-to-Responses conversion
is used and tested, including system instructions and tool preservation. This
does not claim that plain Chat Completions is unavailable upstream.

The new tariffs are qualified for native Standard HTTP requests. They reuse the
existing catalog-driven `service_tier=default` pin, other-tier rejection, HTTP
cache-lifetime validation, WebSocket exclusion, and raw usage-receipt capture.
Batch, Fast, Ultrafast, and discounted background-completion tariffs are not
represented by these Standard entries. No new endpoint or background execution
implementation is included. Existing service-tier and cache guards retain
unpriced receipts rather than treating an unsupported charge as free.

### Pro mode, aliases, and migration details

The native model cards' model-ID sections publish only `gpt-6.2-astra` and
`gpt-6.2-sol`. They do not publish `-pro`, `-latest`, dated `-2026-11-17`, or
`-ultrafast` model IDs. None are manufactured here. The
[reasoning guide](https://developers.openai.com/api/docs/guides/reasoning.md)
documents `reasoning.mode="pro"` on the same native model ID, billed for its
aggregate token usage at that model's rates. A host's separately named Pro
offering does not establish a native OpenAI ID or a separate native tariff.
This catalogue change does not add a typed Pro-mode request field or claim that
all ingress conversions preserve that optional control.

The [GPT-6.2 migration guide](https://developers.openai.com/api/docs/guides/latest-model.md)
also changes prefix-edit reasoning preservation and the placement of top-level
`instructions`. Existing conversion preserves those instructions semantically;
cache-prefix optimization and new stateful migration behavior are outside this
model-information change.

## Gemini 4 Neon: Developer API

The [Gemini changelog](https://aistudio.google.com/docs/changelog) records Neon's
November 18 release. The [model card](https://aistudio.google.com/docs/models/gemini-4-neon)
and [migration guide](https://aistudio.google.com/docs/whats-new-gemini-4-neon)
identify stable `gemini-4-neon` and the existing GenerateContent surface. It has
text/image/audio/video/file input and text output, a shared 1,048,576-token
input/thinking/output window, and 65,536 output tokens per foreground segment.
The advertised maximum is therefore 65,536, not the million-token continuation
limit. Live, image generation, audio generation, explicit caches, and long
decode continuation are not added by this entry.

The [Developer pricing table](https://aistudio.google.com/docs/pricing#gemini-4-neon)
publishes Standard input **3**, output/thinking **15**, and implicit-cache reads
**0.15**. There is no published introductory expiration or long-context tier for
this model. It uses its own full definition rather than copying Carbon's
promotional windows. Existing Google Search accounting receives its documented
14 USD per 1,000 queries default; account-specific free allowances are not
converted into a free per-request tariff.

Reasoning is `low`, `medium`, `high`, or `xhigh`, with `high` by default. The
existing native converter maps these to `thinkingLevel`, removes ignored
sampling controls, and rejects integer thinking budgets. The existing
OpenAI-compatible converter retains the
[compatibility guide's](https://aistudio.google.com/docs/openai#thinking)
documented `minimal` to `low` alias and Neon-specific `xhigh` support;
native `MINIMAL` remains invalid. Discovery and actual conversion are covered
for mapped upstream names, rather than only testing a standalone config map.

## Vertex AI: independent tariff and transport limits

The [Cloud Neon card](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-neon)
and [model lifecycle table](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/model-versions)
confirm the same model ID, release date, supported thinking levels, and
GenerateContent transport. The current catalogue records global Standard
input/output/cache-read prices **3 / 15 / 0.15**, verified independently in
[Cloud pricing](https://cloud.google.com/model-garden/pricing). The non-global
prices are **3.30 / 16.50 / 0.165** and require a channel override under the
existing host-pricing convention. Enterprise eligibility and regional
availability are described without adding a local entitlement check.

Cloud pricing has a material qualification: from November 13, completions over
32K can incur charges for retrievable usage even if they fail on a client/server
timeout. This relay does not retrieve those failed Gemini generations or parse
a provider-specific paid-error receipt. The new Vertex Neon entry therefore
advertises and enforces **32,768 output tokens**, for streaming and non-streaming
requests; a zero upstream limit is pinned to this boundary. Google's documented
65,536-token foreground segment is stated separately. Developer Neon retains
65,536 because its independently reviewed pricing does not state that Cloud
paid-failure contract. The existing Hydrogen and other Gemini 4 paths are not
newly qualified for paid-failure retrieval by this change.

The Cloud table announces Neon cache writes at **3.75** per million tokens
(non-global **4.125**), effective **January 12, 2027**. Hydrogen's corresponding
future global rate is **1.875**. The current November tariffs do not charge these
future writes early. TTL selection, receipt buckets, and failed-generation
accounting need a separate implementation before those future controls can be
advertised. A zero field in the current catalogue is not a promise of free
cache writes after that date.

Carbon/Argon's displayed introductory discount is still delivered through
account credits. It does not replace their existing gross Cloud invoice tariff
with the Developer API's time-window discount.

### Argon Cyber tariff gap resolved

The same current Cloud price table now gives `gemini-4-argon-cyber-permissive`
global Standard input/output/cache-read rates **4 / 20 / 0.20** and non-global
rates **4.40 / 22 / 0.22**. The [Argon model card](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-argon)
confirms its separate ID, `low`/`medium`/`high` levels with `high` by default,
Fairwind access, global/us/eu availability, and absence of computer use.

The route already exists, so this audit adds a complete Vertex-owned tariff
and removes the old Argon-only requirement for manually supplied token/cache
prices. The existing Claude 5.2 and Live operator-price gates remain. Channel
prices still take precedence over this default. Like the new Neon entry, Argon
Cyber is qualified for at most 32,768 foreground output tokens in both modes;
the previously permitted 65,536 streaming output is narrowed because its failed
usage is not yet billable through this relay. Cyber does not receive the
ordinary Argon promotional credits. This resolves the price-evidence gap
recorded in the previous audit without borrowing a Developer API tariff.

Cloud-hosted [GPT-6.2 Astra](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/openai/gpt-6-2-astra)
and [GPT-6.2 Sol](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/openai/gpt-6-2-sol)
are documented as private-preview **Responses-only** offerings; their cards
explicitly exclude Chat Completions. The current Vertex MaaS subadaptor uses
Chat Completions. They remain a separate transport/pricing follow-up, and their
native OpenAI tariffs are not copied into Vertex.

## Azure and AIProxy host isolation

Microsoft's [directly sold model catalogue](https://learn.microsoft.com/en-us/azure/foundry/foundry-models/concepts/models-sold-directly-by-azure)
and [Responses guide](https://learn.microsoft.com/en-us/azure/foundry/openai/how-to/responses)
verify GPT-6.2 support, with version `2026-11-17`. Exact known model names now
select Azure's `/openai/v1/responses?api-version=v1` route; arbitrary deployment
names retain existing routing semantics.

The retrieved [Azure price page](https://azure.microsoft.com/en-us/pricing/details/azure-openai/)
contains GPT-6.2 short/long and cache rows, but displays `$-` instead of usable
numeric quotes. Availability alone cannot qualify an Azure tariff. Both new
native IDs are therefore excluded from automatically inherited Azure and
AIProxy discovery/pricing. Administrators can still configure their own model
mapping and complete channel tariff. The native-only catalogue flag means a
**bundled price verified only for the native host**, not a claim that Azure does
not offer these models.

The AIProxy adaptor still targets `/api/library/ask`.
[Its referenced documentation](https://docs.aiproxy.io/dev/library) was again
unavailable to retrieval. No independently verified library API support or
tariff was found for GPT-6.2. Existing inherited IDs are preserved.

## Providers reviewed without catalogue changes

Anthropic's [overview](https://platform.claude.com/docs/en/models/overview),
[pricing](https://platform.claude.com/docs/en/about-claude/pricing), and
[release notes](https://platform.claude.com/docs/en/release-notes/overview) still
match existing Fable/Mythos 5.2 (**8/40**, cache read **0.20**, 5m write **10**, 1h
write **16**), Opus 5.5 (**4/20**), Sonnet 5.5 (**2/10**), and Haiku 5.5
(**0.10/0.50** through 100K input; **0.50/2.50** above it). Their existing cache,
reasoning, and cloud differences are retained. The latest linked-billing and
watermarking releases do not add a new ordinary Messages model ID.

The [Bedrock document history](https://docs.aws.amazon.com/bedrock/latest/userguide/bedrock-ug-doc-history.html)
adds watermarking guidance on November 19 and private-offer distribution on
November 16. Existing Fable/Mythos 5.2 native entries already cover the November
9 model release. The previously recorded Kimi K3.1 and closed OpenAI transport
gaps remain: their supported upstream offerings require subadaptor/protocol
work, not a name-only catalogue addition. No native creator price is substituted
for a Bedrock price.

## Other reviewed releases and follow-ups

The [OpenAI deprecation table](https://developers.openai.com/api/docs/deprecations.md)
does not require changing the exact lifecycle annotations from the preceding
audit. Existing historical IDs are not deleted or assigned new retirement dates.

The [OpenAI changelog](https://developers.openai.com/api/docs/changelog) introduces
GPT Live 1.1 through `/v1/live/sessions`, distinct from this relay's existing
Realtime integration. It is not listed as a chat/Realtime alias. The new
[CSAM image classifier](https://developers.openai.com/api/docs/guides/csam-classifier.md)
uses an image-only Moderations contract. Although this repository has a
Moderations route, its generic request conversion has not been qualified for
that classifier's specific input and output schema; no new catalogue entry is
claimed solely from the model name. This is a transport-validation follow-up,
not an assertion that the upstream API or its documented free tariff is absent.

The November 19 Genie 4 release in the
[Gemini changelog](https://aistudio.google.com/docs/changelog) uses a separate
interactive world-generation protocol. It is not an ordinary GenerateContent
model. Existing Decisions, Live, and multimodal generation integrations keep
their own protocol boundaries.

## Regression coverage

Focused regressions cover the GPT-6.2 272,000/272,001 input-only price boundary,
all four token-price buckets, native Responses serialization and tool routing,
reasoning efforts, Standard billing policies, and exact Azure/AIProxy exclusions.
Neon coverage checks independent host pricing, shared discovery, streaming and
non-streaming native URLs, mapped effort handling, sampling removal, and the
Vertex 32K limit including omitted and alternate token controls.
Argon coverage checks the real pricing resolver's provider default and channel
override precedence, plus preservation of the unrelated operator-price gates.

The focused command is:

```sh
go test ./relay/adaptor/openai ./relay/adaptor/azure ./relay/adaptor/aiproxy \
  ./relay/adaptor/gemini ./relay/adaptor/geminiOpenaiCompatible ./relay/adaptor/vertexai \
  -run 'Test(GPT62|AzureGPT6|ModelBillingPolic|NativeNovemberModels|LibraryCatalogRequiresHostEvidence|Gemini4|GeminiNovember12|GeminiNeon|VertexGoogle|VertexNewTariff|VertexArgon)' -count=1
```

These tests use synthetic local requests and tariff resolution; they make no
paid upstream inference calls. The PR's consolidated validation record reports
the actual commands and outcomes for the complete refresh.

The final scoped Vertex check passed with Go 1.27.4:

```sh
go test ./relay/adaptor/vertexai -run 'TestVertex(ArgonCyber|NewTariff|Google)' -count=1
```
