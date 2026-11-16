# Frontier and cloud adaptor model audit — 2026-11-16

This audit compares the repository's existing catalog with current provider
documentation. It covers `openai`, `anthropic`, `gemini`,
`geminiOpenaiCompatible`, `vertexai`, `azure`, `aws`, `aiproxy`, and `xai`.
Prices below are USD per million tokens unless another unit is stated.
Account eligibility, deployment regions, and rate limits remain upstream checks;
access restrictions alone are not grounds to hide a compatible published model.
Historical IDs and operator overrides remain available.

## Results by adaptor

| Adaptor | Result | Implementation decision |
| --- | --- | --- |
| OpenAI | Updated | Add `gpt-6-cyber` and both GPT Image 2.6 IDs; update exact lifecycle notices. Integrate Cyber with existing Responses conversion and qualified Standard billing. |
| Anthropic | Audited; current | Main already includes Claude Fable/Mythos 5.2 and the 5.5 family. Retain existing native prices, effort handling, cache tiers, and legacy IDs. |
| Gemini | Audited; current | Main already includes the November 12 Hydrogen and November 10 Carbon refresh. Preserve promotion windows and separate protocol gates. |
| Gemini OpenAI compatible | Audited; current | Retain the existing compatible model subset and Gemini 4 reasoning conversion; new native-only protocols are not chat model aliases. |
| Vertex AI | Audited; current within implemented routes | Retain the November 12 Vertex-owned discovery list and explicit host pricing policy. Hosted closed OpenAI GPT-6 models require a separate Responses route. |
| Azure | Updated host isolation | Do not inherit the three new native OpenAI IDs into Azure discovery or default pricing without Foundry evidence. Existing catalog, explicit mappings, and fallback behavior remain. |
| AWS Bedrock | Audited; protocol gaps recorded | Existing Claude 5.2 entries are current. New Moonshot K3.1 Highspeed and closed GPT-6 offerings require family/transport work beyond model metadata. |
| AIProxy | Updated host isolation; evidence unavailable | Preserve historical OpenAI-derived defaults, but do not auto-add the three new native models to the legacy library API. |
| xAI | Audited; current | Existing Grok 4.7 rates and the November 2 Imagine alias migration already match the current documentation. |

## OpenAI

### New native models

| Model | Input | Output | Cached input | 30m cache write | Context / maximum output |
| --- | ---: | ---: | ---: | ---: | --- |
| `gpt-6-cyber`, input ≤272,000 tokens | 20 | 100 | 2 | 25 | 1,050,000 / 128,000 |
| `gpt-6-cyber`, input >272,000 tokens | 40 | 150 | 4 | Not published | Same model limits |
| `gpt-image-2.6-sunburst` | Text 5; image 8 | Image 30 | Text 1.25; image 2 | Not applicable | Do not infer context/output token limits |
| `gpt-image-2.6-flare` | Text 5; image 8 | Image 30 | Text 1.25; image 2 | Not applicable | Do not infer context/output token limits |

Cyber's card confirms text/image input, text output, reasoning, streaming,
function calling, structured output, and Responses tools. It lists Chat
Completions and Responses support but does not publish a discrete reasoning
effort ladder. The catalog therefore does not copy Astra's effort list or invent
a dated snapshot. The existing native Responses conversion now preserves Cyber
system instructions and tools, including requests mapped from an administrator
alias. Upstream Daybreak Red approval is described without adding a local
entitlement gate.

The generic cache guide describes multipliers, but the model-specific pricing
table explicitly leaves **long-context Cyber cache-write prices blank**. This
audit does not extrapolate a price. Short 30m writes use the existing legacy
`CacheWrite5mRatio` field; 12h writes are not mislabeled as Anthropic 1h writes.
An unexpected long-context write receipt is marked unpriced unless an explicit
applicable operator tier supplies its write rate. This preserves the reservation
through the existing unpriced settlement path. Published long input/output/cache
read prices remain usable when the receipt has no cache writes.

Only Standard processing is implemented for Cyber. Chat/Claude conversion and
direct native HTTP Responses pin `service_tier=default` and reject unpriced
tiers. The existing cache-lifetime guard rejects 12h controls. WebSocket requests
are rejected for this model because frame-level requests bypass those HTTP
checks. Unexpected non-Standard or 12h receipts remain unpriced. These checks do
not change unrelated models or other providers' request policies.

The real response renderers previously dropped envelope `service_tier` when
converting Responses usage, and native JSON error handling discarded counters
reported alongside a failed response. A request-local bounded observer now keeps
the original native Cyber receipt for all three client formats, in JSON and SSE.
It preserves service tier and cache lifetimes before presentation conversion,
so an HTTP 200 `response.failed` envelope with paid usage settles once instead
of being refunded. Ordinary provider errors without usage evidence keep their
existing refund behavior. Claude JSON still renders through the controller's
existing converted-body path; its corrected settlement receipt is carried
separately in request context. Capture-limit uncertainty retains the reservation
without counting a large JSON envelope as completion tokens. These receipt
changes apply only to the native OpenAI Cyber contract.

Image 2.6 uses the established Image API generation/edit path and all five token
billing buckets. No fixed render estimate is added to actual output-token usage.
As with Image 2.5, admission requires a positive operator reservation/fallback
tariff. Verified usage settles at actual token cost with a reservation refund;
missing or ambiguous usage retains the configured fallback policy. That policy
is not a published OpenAI per-image price.
The inherited Image 2.5/2.6 policy does not distinguish a calculated zero token
charge from a missing usable receipt: an explicit legacy model-ratio override
of zero with a positive image fallback still retains that fallback. A zero
group ratio is rejected by the existing positive-reservation admission rule.
This refresh retains those existing semantics; it does not add support for a
free Image 2.6 contract or claim that a zero override waives its fallback.
Only the two published base IDs are registered. The image generation guide also
documents optional `complexity`, SVG, and GIF capabilities; this model-information
refresh does not implement additional request/response formats.

Sources checked:

- [OpenAI changelog](https://developers.openai.com/api/docs/changelog)
- [GPT-6 Cyber model card](https://developers.openai.com/api/docs/models/gpt-6-cyber.md)
- [Daybreak setup and access contract](https://developers.openai.com/api/docs/guides/daybreak.md)
- [GPT Image 2.6 Sunburst](https://developers.openai.com/api/docs/models/gpt-image-2.6-sunburst)
- [GPT Image 2.6 Flare](https://developers.openai.com/api/docs/models/gpt-image-2.6-flare)
- [Pricing tables, including explicit short/long specialized rows](https://developers.openai.com/api/docs/pricing.md)
- [Prompt caching](https://developers.openai.com/api/docs/guides/prompt-caching)
- [Image generation](https://developers.openai.com/api/docs/guides/image-generation.md)

### Lifecycle metadata

The current [deprecations table](https://developers.openai.com/api/docs/deprecations.md)
supersedes earlier migration suggestions. The catalog keeps all existing IDs and
prices, updates replacement guidance for the December 1 image and December 11
GPT-5/o3 shutdowns, and annotates the newly announced February 10, 2027 GPT Image
2 shutdown. It also adds explicitly listed existing aliases/snapshots for the
January audio and February transcription shutdowns, the April GPT-5.1 snapshot,
and the already retired `gpt-4-1106-preview`.

No shutdown is extrapolated to GPT Image 2.5 or unrelated GPT-4o/4.1 aliases.
Unsupported historical IDs mentioned only in the source are not fabricated to
attach a notice. Speech migration guidance retains the warning that Realtime is
not a drop-in `/audio/speech` replacement. Native OpenAI lifecycle annotations do
not establish Azure retirement dates.

### Material OpenAI evidence gaps

The [October 30 changelog](https://developers.openai.com/api/docs/changelog) and
[Rosalind access documentation](https://help.openai.com/en/articles/20001193-gpt-rosalind-for-life-sciences-research)
confirm API availability of `gpt-6-rosalind`; the
[Responses schema](https://developers.openai.com/api/reference/resources/responses/methods/create)
also enumerates it and `gpt-rosalind-research`. The pricing table publishes
Rosalind 6 short/long tariffs and Research/Discovery tariffs. However, no
model-specific limits or numeric short/long threshold for Rosalind 6 could be
verified in the retrieved primary material, and the exact Discovery endpoint
contract was not confirmed. These models remain a recorded follow-up, with no
inherited Astra limits or guessed long-context threshold. Their access program
is not the reason for deferral.

`gpt-6-astra-law` appears in pricing, but the current
[Astra for Law help page](https://help.openai.com/en/articles/20001528-astra-for-law)
still describes API access as coming soon. A price row alone is insufficient to
claim a usable endpoint. The [announcement](https://openai.com/index/astra-for-law/)
also distinguishes current product access from forthcoming API access.

## Anthropic

The [model overview](https://platform.claude.com/docs/en/models/overview),
[pricing](https://platform.claude.com/docs/en/about-claude/pricing), and
[release notes](https://platform.claude.com/docs/en/release-notes/overview) confirm
the models already added by the November 10 audit. Fable/Mythos 5.2 remain 8/40
with cached input 0.2, 5m write 10, and 1h write 16. Opus 5.5 remains 4/20 and
Sonnet 5.5 remains 2/10. Haiku 5.5 retains its input-token tier at 100,000:
0.1/0.5 short and 0.5/2.5 long, with their separate cache rates.

No duplicate update is needed. Native effort support, thinking conversion, and
cloud-specific differences remain as documented in
`relay/adaptor/ANTHROPIC_PROVIDER_AUDIT_20261110.md`. The November 12 managed-agent
release does not add a new ordinary Messages model ID.

## Gemini and Gemini OpenAI compatible

The [Gemini changelog](https://aistudio.google.com/docs/changelog) confirms
Hydrogen's November 12 release, Carbon's November 10 release, and the November
11 Decision API. Main already contains the native and compatible Hydrogen
entries at 1.5/7.5, the documented low/medium/high/xhigh reasoning controls,
Carbon's 2/10 promotion through November 30, and independent cache/modality
prices. Preserve the existing request-time promotion windows and billing gates.

Decision 1 uses the separate Decisions API, while Gemini Omni video generation
is not a normal chat response. Neither is advertised as a new compatible chat
model. The embedding preview-to-stable redirect announced November 6 does not
justify deleting a historical ID or changing its vector dimensions. The
existing native image, Live, and embedding implementations retain their own
modality contracts. See `docs/gemini-model-refresh-20261112.md` for the preceding
model-by-model changes and provider-specific source links.

## Vertex AI

The [Google Cloud model catalog](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models)
and [model versions](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/model-versions)
confirm the existing November 12 catalog, including Carbon, Hydrogen, Argon,
documented Cyber variants, Fable 5.2, image, Live, and compatible MaaS families.
Preserve the separate Vertex suggestion list, explicit host-pricing requirements,
and legacy routes described in `docs/vertexai-model-audit-20261112.md`.

The newly documented hosted
[GPT-6.1 Sol](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/openai/gpt-6-1-sol)
and [GPT-6.1 Terra](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/openai/gpt-6-1-terra)
use a Responses surface. The current Vertex MaaS adapter is a Chat Completions
implementation. Adding the IDs to its map would claim unsupported routing and
could borrow the wrong native OpenAI tariff. These are an explicit transport
follow-up, not an entitlement exclusion.

## Azure AI Foundry

Microsoft's [models sold directly by Azure](https://learn.microsoft.com/en-us/azure/foundry/foundry-models/concepts/models-sold-directly-by-azure)
lists GPT-6.1 Sol/Terra and GPT Image 2.5 but did not publish GPT-6 Cyber or GPT
Image 2.6 in the retrieved current catalog. The three native additions therefore
do not enter Azure's automatic model list or default price map. Direct ratio
getters retain the ordinary unknown-model fallback; this fallback is not a
quoted Foundry price. Explicit administrator model/deployment mappings are not
blocked.

The [Foundry Claude documentation](https://learn.microsoft.com/en-us/azure/foundry/foundry-models/concepts/claude-models)
confirms the existing 5.2 entries. Keep Foundry's xhigh effort limit rather than
native Claude's max level, existing authentication behavior, and operator pricing
overrides. This audit does not redesign the inherited historical Azure catalog.
The native `chat-latest` card and Azure's `gpt-chat-latest` listing are different
IDs/contracts; neither is substituted for the other.

## AWS Bedrock

The [Bedrock document history](https://docs.aws.amazon.com/bedrock/latest/userguide/bedrock-ug-doc-history.html)
and [Fable 5.2 card](https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-fable-5-2.html)
confirm that the existing November Claude additions are current. Retain their
regional/global inference-profile mappings and host-specific pricing behavior.

The November 4
[Kimi K3.1 Pro Highspeed card](https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-moonshot-ai-kimi-k3-1-highspeed.html)
publishes `moonshotai.kimi-k3.1` and cross-region profiles, with global rates
4/20, cached input 0.2, and write 5; US/India rates are 4.4/22, 0.22, and 5.5.
Its profile and tier distinctions must survive a future implementation. The
current native AWS registry has no Moonshot adapter, so a name-only addition
would not be callable through it. Similarly, its OpenAI subadapter currently
handles GPT-OSS rather than the closed GPT-6 offering listed in the document
history. Those integrations require a separate wire, profile, and billing review.
Neither native Moonshot nor OpenAI prices are copied into Bedrock.

## AIProxy

This adaptor targets the legacy `/api/library/ask` knowledge-base endpoint and
references [docs.aiproxy.io/dev/library](https://docs.aiproxy.io/dev/library).
That primary URL could not be retrieved, and a targeted search did not return a
current authoritative model or tariff contract. Search results for
`aiproxy.com` describe a different Swift/proxy service and were not used to
qualify this implementation.

Historical OpenAI-derived defaults remain intact, but the new native Cyber and
Image 2.6 models no longer propagate automatically through a shared map. The
library API's actual request conversion does not establish their native model
or billing contract. Operator-configured IDs still use the existing route and
fallback policy. No current independent AIProxy price is asserted.

## xAI

The [model catalog](https://docs.x.ai/developers/models),
[release notes](https://docs.x.ai/developers/release-notes), and
[pricing](https://docs.x.ai/developers/pricing) retain the current Grok 4.7 text
contract and its long-input tier. The existing catalog already records the
[November 2 Imagine quality migration](https://docs.x.ai/developers/migration/imagine-image-quality-nov-2):
legacy quality/pro aliases now resolve to Image 2.0 low quality at 0.04/0.05/0.06
per generated 1K/1.5K/2K image, with 0.01 per input image. The effective-date
window preserves the earlier prices before November 2; no flat overwrite is
needed.

Grok 4.7 Fast is described as product-specific rather than a general public API
model. Voice Agent and transcription releases use separate audio protocols; the
current xAI adaptor does not gain those transports through a chat metadata entry.
No new transport or speculative alias is introduced.

## Validation

Focused tests cover discovery and non-invented IDs, the Cyber 272,000/272,001
input-only tier boundary, parsed 30m cache receipts, explicit long-write
overrides, unpriced receipts, Chat/Claude-to-Responses conversion, preserved
instructions/tools, HTTP and WebSocket tariff admission, exact lifecycle notices,
and Azure/AIProxy host isolation.

An additional regression reproduced lost envelope tiers and failed-response
usage in the actual adaptor handlers. It now passes across Chat Completions,
Responses, and Claude Messages, for completed/failed JSON and SSE. Controls
cover unrelated models/providers, an error without paid usage, and the bounded
capture limit. The full controller HTTP/SQLite test covers 18 combinations of
client format, streaming, Standard usage, unsupported actual tier, and paid
failure. It verifies the mapped upstream model and Responses path, Standard
admission, the delivered body, exact user/token/request/log debits, reservation
retention for unpriced work, and rejection of a duplicate retry.

The first regression run correctly failed on missing native IDs and lifecycle
guidance. A later run was temporarily blocked by a concurrently edited Baidu
duplicate ID during package initialization; no frontier tests executed in that
run. The focused run passes for OpenAI, Azure, AIProxy, quota, and controller:

```sh
go test ./relay/adaptor/openai ./relay/adaptor/azure ./relay/adaptor/aiproxy ./relay/quota ./relay/controller \
  -run 'Test(GPT6Cyber|GPTImage26Discovery|NovemberLifecycleGuidance|NativeNovemberModels|LibraryCatalogRequiresHostEvidence|OpenAIOctober|GPT61Terra)' -count=1
```

Full package tests passed for OpenAI, AIProxy, quota, and Azure. The initial Azure run
identified the old assertion that every native OpenAI entry must enter Azure;
the assertion now permits precisely the three documented exclusions while
retaining its duplicate-key and lost-entry protection.
The controller integration command is:

```sh
go test ./relay/controller -run TestGPT6CyberReceiptSettlementHTTP -count=1
go test -race ./relay/adaptor/openai -run 'TestGPT6Cyber' -count=1
```

Its initial fixture run correctly exposed that a localhost native Responses
fixture selected the Chat fallback and that streamed Chat delivery requires text
delta events. The corrected fixture uses the native host contract with a local
endpoint override and emits text deltas; the path and delivered-content
assertions remain in place. All 18 cases pass. No production routing or rendering
policy was changed to accommodate the fixture.
The focused Cyber race run also passes. It exercises the observer while the
stream reader can finish asynchronously. Full OpenAI, Azure, and AIProxy package
tests were rerun after the receipt changes and pass. The independent provider
isolation regression additionally verifies that custom compatible/Azure/Groq
tariffs do not inherit native Cyber restrictions, while native and unidentified
provider receipts retain the conservative guard.
Tests use Go 1.27.4, local synthetic requests/receipts, and no paid upstream calls.
