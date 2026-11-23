# Adaptor model refresh — 2026-11-22

## Purpose and result

Refresh the gateway's model information against official provider catalogs,
model cards, API documentation, prices and lifecycle notices available on
November 22, 2026. This is an incremental review after the
[November 16 refresh](adaptor-model-refresh-20261116.md). The coverage ledger
below accounts for all **51 directories** under `relay/adaptor`, including
shared protocol and helper packages. A reviewed directory does not necessarily
need a code change, and review coverage does not certify live account access.

The changes add published models on supported request paths, update mutable
host prices and capabilities, annotate a confirmed retirement, and close the
OpenRouter automatic cache-write admission gap. Host tariffs, currencies,
regional differences, input thresholds and time windows remain separate.
Historical model IDs remain configurable, and channel price overrides keep
their existing precedence.

Model defaults remain directly maintained Go code in stable family files,
assembled through the existing `models.go` entry points and
`adaptor.JoinModelCatalogs`. This refresh does not introduce a runtime catalog
download, generated data dependency or dated registration layer. See the
[maintenance guide](../relay/adaptor/MODELS.md).

## Detailed evidence and decision records

| Record | Scope |
| --- | --- |
| [Frontier and cloud audit][Frontier] | OpenAI, Anthropic, Gemini, Gemini OpenAI compatible, Vertex AI, Azure, AWS Bedrock and AIProxy |
| [Chinese provider audit][China] | Ali/Bailian, Qianfan, Ark, DeepSeek, Moonshot, MiniMax, StepFun, BigModel/Z.AI and the remaining Chinese provider adaptors |
| [Aggregator audit][Aggregators] | OpenRouter, DeepInfra, Novita, Together AI, Fireworks and SiliconFlow |
| [Additional provider audit][Other] | xAI, Cerebras, Groq, Mistral, Cohere, Cloudflare, NVIDIA, Jina, TypeSafe, Replicate and the remaining hosted/local adaptors |

These records link the primary sources and explain each inclusion, exclusion
and unresolved contract. The aggregator audit also records retrieval times and
SHA-256 fingerprints of the official responses. Prices below are per million
tokens in **input / cache read / output** order, unless stated otherwise.
Unknown cache prices and output limits remain unspecified; they are not treated
as free service or copied from another host.

## New and newly qualified model information

### OpenAI GPT-6.2 and host isolation

Native OpenAI adds `gpt-6.2-astra` and `gpt-6.2-sol`. Both publish a
1,050,000-token context, maximum 922,000-token input and maximum 128,000-token
output. The separate input limit is recorded in the description because the
shared metadata schema has no dedicated field. Text and image input produce
text output. Existing native Chat/Claude-to-Responses conversion preserves
instructions and tools; GPT-6.2 tool calling requires Responses. [Frontier]

| Native model and prompt length | USD input / cache read / output | USD 30-minute cache write |
| --- | --- | ---: |
| `gpt-6.2-astra`, input ≤272,000 | 8 / 0.40 / 40 | 10 |
| `gpt-6.2-astra`, input >272,000 | 16 / 0.80 / 60 | 20 |
| `gpt-6.2-sol`, input ≤272,000 | 2 / 0.10 / 10 | 2.50 |
| `gpt-6.2-sol`, input >272,000 | 4 / 0.20 / 15 | 5 |

The expensive tier begins at **272,001 input tokens**, applies to the whole
request, and is not selected by output length. Both models advertise `low`,
`medium`, `high`, `xhigh` and `max`; the relay's existing fallback is `medium`.
The new definitions reuse native Standard HTTP billing policies, including
service-tier validation, cache-lifetime validation, raw receipt capture and
WebSocket exclusion. Their Standard prices do not qualify Batch, Fast,
Ultrafast or discounted background completion. [Frontier]

OpenRouter independently publishes `openai/gpt-6.2-astra`,
`openai/gpt-6.2-astra-pro`, `openai/gpt-6.2-sol` and
`openai/gpt-6.2-sol-pro`, with the same verified short/long token rates and
30-minute write prices shown above. These four host IDs are added on its
existing compatible route. The host's `-pro` aliases do not create native
OpenAI model IDs. Native `reasoning.mode="pro"` uses the ordinary native ID
and aggregate token tariff; this refresh does not add a typed Pro-mode field or
claim that every ingress conversion preserves that optional control.
[Frontier] [Aggregators]

Azure documents GPT-6.2 deployment availability and version `2026-11-17`, but
the retrieved pricing page displays `$-` instead of usable numeric quotes.
Exact configured GPT-6.2 model names now select Azure's existing Responses
route. The bundled native prices and discovery entries are excluded from
automatic Azure and AIProxy inheritance. Operators can configure model mappings
and complete channel tariffs. This is a host-price qualification boundary,
not a claim that Azure lacks these models. AIProxy's library API documentation
was unavailable, so no new library contract is inferred. [Frontier]

### Gemini Neon and the Vertex Argon Cyber tariff

`gemini-4-neon` is added to native Gemini, Gemini OpenAI compatible and Vertex
GenerateContent discovery; OpenRouter adds `google/gemini-4-neon`. Each host's
tariff and limits are checked independently. The shared 1,048,576-token window
includes input, thinking and output. The verified Standard price is USD
**3 / 0.15 / 15**, but output qualification differs by transport.
[Frontier] [Aggregators]

| Host | Ordinary output ceiling in this relay | Qualification |
| --- | ---: | --- |
| Gemini Developer API, native and compatible | 65,536 | Published foreground-segment maximum; no continuation protocol is added. |
| Vertex AI | 32,768 | Enforced for streaming and non-streaming requests, including an omitted upstream limit. |
| OpenRouter | 65,536 | Advisory metadata from the FAQ and standard AI Studio endpoint; no new request cap is imposed. |

Vertex's pricing states that, from November 13, completions over 32K may be
charged for retrievable usage after a client/server timeout. The relay does
not retrieve those failed Gemini generations or parse their paid-error
receipts. The new Vertex Neon entry therefore stops at 32,768 output tokens,
while describing Google's 65,536 foreground limit separately. Existing
Hydrogen and other Gemini 4 paths are not newly qualified for that failure
contract by this refresh. Developer pricing was reviewed separately and does
not establish the same Cloud condition. [Frontier]

OpenRouter's aggregate catalog and standard Vertex endpoint instead advertise
943,718 output tokens. Its FAQ and standard AI Studio endpoint support 65,536
ordinary output tokens, and the model page requires continuation for long
decode. The catalog uses the conservative 65,536 advisory value and records
the evidence conflict. Its published cache-write quote is retained as a
conditional tariff: USD 3 input plus USD 1/12 storage per million written
tokens under the host's documented Gemini formula. Current endpoint responses
mark implicit caching unsupported, while the generic caching guide defers
support to Google. No cache-mode capability is claimed from the price alone.
[Aggregators]

Native and compatible Developer Neon support `low`, `medium`, `high` and
`xhigh` with `high` by default; OpenRouter publishes its own `medium` default.
The existing native converter maps effort to `thinkingLevel`, removes ignored
sampling controls and refuses integer thinking budgets. The compatible
converter preserves its documented `minimal` to `low` alias. These models
accept multimodal input and emit text; the addition does not implement Live,
generated audio/images or long decode continuation. [Frontier] [Aggregators]

The current Cloud price table also resolves the previous tariff gap for the
existing `gemini-4-argon-cyber-permissive` route. Its global Standard price is
USD **4 / 0.20 / 20**, with non-global **4.40 / 0.22 / 22** requiring the
existing channel override. The default preserves `low`/`medium`/`high`
reasoning, `high` default, Fairwind access and documented regions. It does not
claim computer use or ordinary Argon's promotional credits. The old
Argon-only manual-price requirement is removed; unrelated Claude 5.2 and Live
operator-price gates remain. Argon Cyber also enforces 32,768 output tokens
in both modes, narrowing its previous 65,536 streaming allowance because
failed longer output cannot yet be settled through this relay. [Frontier]

Neon's global Vertex rates are separate from non-global
**3.30 / 0.165 / 16.50**. Announced Vertex cache-write prices, including Neon
USD 3.75/M and Hydrogen USD 1.875/M, start on **January 12, 2027** and are not
charged early. TTL controls, receipt buckets and failed-generation accounting
need separate qualification before that future contract is supported.
[Frontier]

### Alibaba and other new hosted models

| Provider and exact API ID | Currency and input / cache read / output | Context / maximum output |
| --- | --- | --- |
| Ali and AliBailian `kimi-k3.1-pro-prime` | CNY 26 / 1.30 / 136 | 1,048,576 / 1,048,576 |
| AliBailian `antdigital/Ling-3.1-flash` | CNY 0.50 / 0.10 / 1.50, current promotion | 1,048,576 / 262,144 |
| OpenRouter `thinkingmachines/inkling-2-mini` | USD 0.075 / 0.015 / 0.30 | 1,048,576 / 65,536 |
| OpenRouter `radixark/daisyrose-27b` | USD 0.15 / 0.0375 / 1.875 | 262,144 / 235,929 |
| OpenRouter `mistralai/pixtral-large-2502` | USD 2 / unspecified / 6 | 128,000 / 102,400 |
| DeepInfra `RadixArk/DaisyRose-27B` | USD 0.15 / 0.0375 / 1.875 | 262,144 / unspecified |
| DeepInfra `tencent/Hy4` | USD 0.834 / 0.04199999814 / 2.501 | 1,048,576 / 131,072 |
| Together AI `Qwen/Qwen4-Max` | USD 1.425 / 0.07125 / 4.275 | 1,000,000 / unspecified |
| Together AI `tencent/Hy4` | USD 0.834 / 0.042 / 2.501 | 1,048,576 / unspecified |

Alibaba's November 19 Prime model is a distinct hosted high-speed Kimi
contract. Its cache read remains CNY 1.30/M even though input/output prices
double the ordinary Pro prices. Ali's native path remains text-only; the
compatible path preserves supported image input without inferring video or
unsupported effort controls. [China]

Ling requires an operator-configured BaseURL:
`https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com`. The existing AliBailian
adaptor appends `/compatible-mode/v1/chat/completions`. Its 75% promotion
reduces CNY 2 / 0.40 / 6 list prices to the table's current rates; no expiry is
published, so none is invented. The model supports text, tools, JSON mode and
reasoning, without an inferred strict schema or effort vocabulary. It is not
added to the native Ali DashScope path. [China]

Host differences in the remaining additions are intentional. DeepInfra
DaisyRose's 25% discount is applied once and its FP8 serving is verified in its
own card; OpenRouter's output ceiling is not copied into DeepInfra. DeepInfra
Hy4 retains the exact captured cache multiplier, while Together publishes the
rounded USD 0.042/M quote. Pixtral has no published tools or cache-price claim
in its host row. Together Qwen4 Max and Hy4 support text/image input, function
calling and structured output on the documented Standard service.
[Aggregators]

## Existing defaults, schedules and lifecycle annotations

OpenRouter updates **32 existing input/cache/output quotes**, **28 published
context/output fields**, **ten parameter lists** and **two schedules**. The
[aggregator audit][Aggregators] contains every before/after value. These are
current routing-default observations; explicitly selected endpoints, negotiated
prices and future routing can differ. Existing reported-cost settlement and
channel overrides retain their roles. Important changed defaults include:

| Provider / model | Current input / cache read / output | Change or boundary |
| --- | --- | --- |
| OpenRouter `moonshotai/kimi-k3.1-pro` | USD 0.253 / 0.18 / 8.80 | Current hosted quote; not Moonshot's native CNY tariff. |
| OpenRouter `z-ai/glm-5.3-flash` and `~z-ai/glm-flash-latest` | USD 0.0399 / 0.03 / 0.0784 | Current alias and pinned quotes agree. |
| OpenRouter `deepseek/deepseek-v4.1-flash` | USD 0.0498 / 0.000996 / 0.1992 | Current hosted quote. |
| OpenRouter `deepseek/deepseek-v4-pro-0813` | USD 1.32 / 0.044 / 3.96 | Current flat quote. |
| OpenRouter `tencent/hy3` | USD 0.14 / 0.035 / 0.58 | Remove the former 16:00 UTC discount. |
| DeepInfra `zai-org/GLM-5.3-Flash` | USD 0.1005 / 0.0201 / 0.335 | Published 33% discount, no stated expiry. |
| Novita `tencent/hy4-flash` | USD 0.035 / 0.0015 / 0.1405 | Published decimal quote already includes the 50% discount. |
| Together `ricursive-ai/Ripple-65B-Preview` | USD 0.20 / 0.02 / 0.60 | Fill published cache price, tools, structured output and FP4 metadata. |

OpenRouter `deepseek/deepseek-v4.1-flash-1017` stores the conservative peak
base of USD **0.30 / 0.006 / 1.20**. Peak windows are **01:00–04:00 and
06:00–10:00 UTC on weekdays**. Other weekday hours and the full Saturday/Sunday
use USD **0.15 / 0.003 / 0.60**. Explicit weekday and weekend windows implement
the host's schedule. The separate mutable alias
`~deepseek/deepseek-flash-latest` keeps its own flat
**0.036 / 0.01 / 0.50** quote; an alias is not forced to match a pinned ID.
Hy4 Preview and MiniMax M3.1 Flash retain their independently sourced schedules.
[Aggregators]

The ten OpenRouter parameter corrections remove stale claims such as logprobs,
JSON mode or parallel tools where the current host no longer advertises the
corresponding control, and add newly advertised controls where present. These
are synchronized metadata and derived features, not a new request filter or a
claim about every underlying native model. [Aggregators]

DeepSeek's native Search/Fetch note now records that the model-triggered tool
fee is waived from **November 18, 2026 at 04:00 UTC**, while returned context
remains token-billed. Standalone Search/Fetch remains USD 0.75 per 1,000
successful requests. This is a pricing-note correction; no new tool endpoint
or server-tool accounting policy is introduced. [China]

xAI's official release notes confirm `grok-build-0.1` reached end of life on
**November 19, 2026**. The legacy `grok-code-fast-1`, `grok-code-fast` and
`grok-code-fast-1-0825` descriptions now identify the retirement of their
documented target. Their IDs, prices and existing forwarding are preserved;
the change does not invent a replacement route. `grok-build-latest` retains
its existing Grok 4.5 target. Earlier Cloudflare retirement annotations and
other historical provider IDs remain intact. [Other]

## Automatic cache-write admission through OpenRouter

OpenRouter's own guide documents automatic GPT-5.6-and-later cache writes at
1.25 times ordinary input pricing with a 30-minute lifetime. No request opt-in
is necessary. Merely settling a later provider cost receipt does not fund this
charge before generation. The controller now includes the host's published
automatic-write tariff in admission for qualified OpenRouter GPT-5.6/GPT-6
models and verified current family aliases. Its predicate reads the host's own
catalog and requires a positive published write rate; it does not import
native OpenAI prices or classify unknown/other-host IDs. [Aggregators]

Admission validates the exact prepared Chat or Responses payload and honors an
explicit opt-out when that control is present in the final upstream body.
Existing Responses normalization strips root `prompt_cache_options`; this
refresh does not add a new forwarding contract, and those normalized requests
must still fund automatic writes. Unsupported 12-hour controls remaining in
the provider body are refused at the root or content-block level. Anthropic
cache controls translated into this OpenRouter contract fund the applicable
30-minute write bucket. Flat channel overrides now receive the same complete
admission check without restoring the provider tiers that the operator replaced.
The scoped path covers Chat, Responses, Messages, MCP rounds and WebSocket
per-create funding. Existing explicit channel prices and provider scope are
preserved. Regression coverage includes prepared payloads, implicit writes,
opt-outs, unsupported TTLs and host isolation. Native GPT-6.2 also has HTTP/SQLite
settlement coverage through the three supported client formats.

### Review follow-ups

- **Native OpenAI flat overrides:** the same complete-admission predicate now
  also qualifies native OpenAI GPT-5.6+ models whose implicit 30-minute writes
  are metered (`openai.ImplicitPromptCacheWrites`). A flat `model_configs`
  override previously skipped cache-write funding, so a 100,000-token
  `gpt-6.2-astra` request was held at 402,020 quota and could settle at
  500,020. Flat overrides stay flat, explicit opt-outs and breakpoints keep
  their meaning, and the existing GPT-6 Cyber long-context pinning applies
  unchanged.
- **Tool definitions in prompt estimates:** Chat estimates now count tool,
  function and `response_format` schemas plus replayed assistant tool-call
  fields, and Responses estimates count tools (once, without the nested
  compatibility copy) and text formats. The change applies to initial
  admission, every MCP round quote and WebSocket frames. The Claude-to-Responses
  path no longer counts its tools twice. Settlement from provider receipts is
  unchanged; usage-less fallbacks now include the tool input.
- **Responses system instructions:** every route that converts Chat or Claude
  ingress to Responses (Azure, native OpenAI and OpenAI-compatible Responses
  channels) now sends the leading system message as `instructions` for
  reasoning models. Only `o1-preview` and `o1-mini`, which never accepted
  system or developer roles, keep the historical removal, and Chat-routed
  requests are unchanged. An empty leading system message no longer consumes
  the following user turn.

## Deferred and unknown contracts

The source reports explicitly retain these boundaries:

- **Different upstream protocols:** Cloud-hosted GPT-6.2 is a private-preview
  Responses-only offering, while the current Vertex MaaS subadaptor uses Chat
  Completions. Existing Bedrock Kimi/closed-OpenAI transport gaps, OpenAI Live
  1.1, the new image-only moderation classifier, Genie 4 and new media APIs
  require separate protocol qualification. A name or free price alone does
  not establish a compatible request/response path. [Frontier]
- **Incomplete host billing:** Novita `qwen/qwen3-vl-plus` has published tiers
  ending below 256,000 input tokens despite a 262,000-token context and
  conflicting generic prices. It is deferred until the last interval is
  explained. Together's existing per-image-area extrapolation remains an
  unqualified historical assumption for image estimates; no new such image
  model is introduced. [Aggregators]
- **Separate legacy services:** Tencent's TC3 service, Xunfei's legacy WebSocket
  and HTTP services, 360's legacy endpoint and Yi's older platform are not
  silently replaced by current supplier marketplaces or MaaS routes. An
  unavailable page does not revalidate an old price or retirement date.
  Historical configuration is preserved with those limits documented. [China]
- **Account or product pricing:** Azure/AIProxy, Copilot, Coze, DeepL regional
  plans, local Ollama tags and private/dedicated offerings do not acquire a
  guessed public token tariff. Cohere Rerank 5's token meter and newer
  specialized NVIDIA/Jina services need their own contracts. [Frontier] [Other]
- **Future releases and dates:** Announced January 2027 Vertex write prices
  are not active in November. A StepFun release dated November 23 is outside
  this audit cutoff. Existing Baidu promotional windows retain their precise
  inclusive final day through the next day's exclusive boundary; an expired
  old label without a replacement quote does not justify an invented new
  rate. [Frontier] [China]

The review preserves prior audits and provenance rather than rewriting old
claims as fresh observations. Public catalog absence does not delete an
operator's configured model, and an unknown price is distinct from an explicitly
free tariff. No authenticated generation or regional entitlement certification
is claimed by the research itself.

## Complete directory coverage ledger

The following ledger contains one row for every actual adaptor directory.
“Reviewed” means that the corresponding audit records a disposition; it does
not mean every historical row has a newly verified live tariff.

| Directory | Disposition in this refresh | Evidence |
| --- | --- | --- |
| `ai360` | Reviewed; keep legacy `ai.360.cn` configuration separate from the newer supplier router and its prices. | [China] |
| `aiproxy` | Exclude new native GPT-6.2 defaults from inheritance; legacy library API evidence remains unavailable. | [Frontier] |
| `ali` | Add hosted `kimi-k3.1-pro-prime`; retain native DashScope capability boundaries. | [China] |
| `alibailian` | Add Kimi Prime and Ling 3.1 Flash with the documented workspace BaseURL and current promotion. | [China] |
| `anthropic` | Reviewed; existing Fable/Mythos 5.2 and Opus/Sonnet/Haiku 5.5 entries match current Messages documentation. | [Frontier] |
| `aws` | Reviewed; current native subadaptors already cover compatible releases; private/closed-model transport gaps remain. | [Frontier] |
| `azure` | Route exact configured GPT-6.2 names to Responses; exclude unverified native prices from host defaults. | [Frontier] |
| `baichuan` | Reviewed; existing M3/M3 Plus prices remain current; separate embedding/per-call meters retained. | [China] |
| `baidu` | Reviewed; preserve legacy OAuth V1 IDs and migration boundary, without copying Qianfan V2 models into V1. | [China] |
| `baiduv2` | Reviewed; current ERNIE/LightDT/DeepSeek rows and dated promotional boundaries are already represented. | [China] |
| `cerebras` | Reviewed; public API prices/limits remain current and distinct from larger paid/dedicated contexts. | [Other] |
| `cloudflare` | Reviewed; compatible current releases and earlier November 20 retirement annotations are already present. | [Other] |
| `cohere` | Reviewed; Command/North/Embed defaults remain current; new Rerank token metering remains separate. | [Other] |
| `common` | Shared adaptor helpers; no independent upstream model catalog or tariff to refresh. | Local implementation |
| `copilot` | Reviewed; product model display names do not establish inference IDs or public token prices. | [Other] |
| `coze` | Reviewed; configured-bot legacy route retained; unavailable pricing does not establish a token tariff. | [Other] |
| `deepinfra` | Add DaisyRose and Hy4; update GLM 5.3 Flash's discount and preserve the existing legacy safety floor. | [Aggregators] |
| `deepl` | Reviewed; character billing and regional plan differences retained; historical default not newly price-certified. | [Other] |
| `deepseek` | Correct Search/Fetch fee-waiver provenance; existing token prices and tool transport remain unchanged. | [China] |
| `doubao` | Reviewed; existing Ark models/prices remain current; completion-time pricing qualification is still separate. | [China] |
| `fireworks` | Reviewed; current compatible Standard catalog is already represented; other serving tiers stay separate. | [Aggregators] |
| `gemini` | Add Developer Neon with independent rates, 65,536 foreground output and native thinking conversion. | [Frontier] |
| `geminiOpenaiCompatible` | Add Developer Neon with the compatible effort normalization and existing search accounting. | [Frontier] |
| `groq` | Reviewed; production/preview catalogs remain current; enterprise quotes remain unknown. | [Other] |
| `internal` | Shared implementation helpers; no independent upstream catalog or commercial contract. | Local implementation |
| `jina` | Reviewed; existing embedding/rerank/OCR schemas remain current; async and other specialized APIs stay separate. | [Other] |
| `lingyiwanwu` | Reviewed with retrieval limitation; preserve historical IDs and prior closure notes without freshly certifying them. | [China] |
| `minimax` | Reviewed; M3/M3.1 prices and long-context tiers are current; new media and legacy native limitations remain separate. | [China] |
| `mistral` | Reviewed; existing Large 4, hosted model and Voxtral 3 definitions remain current. | [Other] |
| `moonshot` | Reviewed; native K3.1 Pro/Highspeed token and cache-write prices remain distinct from reseller tariffs. | [China] |
| `muapi` | Reviewed; dynamic video quotes and receipts remain the implemented contract; text uses a separate API. | [Other] |
| `novita` | Update Hy4 Flash's discounted quote; defer Qwen3 VL Plus's incomplete final input interval. | [Aggregators] |
| `nvidia` | Reviewed; existing compatible NIM chat models retained; new PII/protein APIs do not qualify as chat models. | [Other] |
| `ollama` | Reviewed; installed tags, quantization and local availability remain operator-specific. | [Other] |
| `openai` | Add Astra/Sol 6.2 with Standard prices, input-only tiers, cache writes, reasoning and existing Responses conversion. | [Frontier] |
| `openai_compatible` | Reviewed shared conversion package; no independent provider catalog or tariff. | [Other] |
| `openrouter` | Add eight IDs; update prices, limits, controls and schedules; fund published automatic OpenAI cache writes. | [Aggregators] |
| `palm` | Reviewed; historical PaLM IDs retained after old documentation became unavailable; Gemini uses separate adaptors. | [Other] |
| `proxy` | Reviewed local forwarding route; upstream catalog and pricing remain operator-provided. | [Other] |
| `replicate` | Reviewed; current qualified Gemini 3.1 Pro tiers retained; inaccessible/unsupported model contracts remain unresolved. | [Other] |
| `siliconflow` | Reviewed; latest published November 16 notes and earlier compatible releases are already represented. | [Aggregators] |
| `stepfun` | Reviewed; Step 5/3.7 token prices remain current; future audio release and separate meters excluded. | [China] |
| `tencent` | Reviewed; legacy TC3 and current TokenHub are distinct platforms, with no implicit route migration. | [China] |
| `togetherai` | Add Qwen4 Max and Hy4; complete Ripple cache/capability metadata; retain separate image-estimate limitation. | [Aggregators] |
| `typesafe` | Reviewed; Jev aliases, pinned versions and published token prices remain current. | [Other] |
| `vertexai` | Add Neon and verified Argon Cyber tariff; enforce 32,768 output for both; preserve other operator-price gates. | [Frontier] |
| `xai` | Annotate the confirmed November 19 retirement behind legacy Grok Code Fast IDs; retain IDs and existing routes. | [Other] |
| `xunfei` | Reviewed; legacy WebSocket models and free Lite remain distinct from newer MaaS offerings. | [China] |
| `xunfeiv2` | Reviewed; legacy HTTP IDs remain distinct from MaaS X2.6 and its separately announced retirements. | [China] |
| `zai` | Reviewed; existing GLM 5.3/Flash/Legal USD tariffs remain independently sourced from Z.AI. | [China] |
| `zhipu` | Reviewed; existing GLM 5.3/Flash/Legal CNY tariffs remain independently sourced from BigModel. | [China] |

## Consolidated validation

Local validation passed with **Go 1.27.4**:

```sh
make test-quick
go test ./relay/adaptor/... ./relay/pricing ./relay -count=1 -timeout 5m
go test ./relay/quota -count=1 -timeout 5m
go test ./controller -run 'Test(ListAllModels|StaticModels|SharedGLM|DedupeStaticModels|ListModelsSharedGLM)' -count=1 -timeout 5m
go test ./test -run '^Test(EffectiveCatalogSnapshot|EveryAdvertisedModelIsPricedByItsChannel|CompatibleChannelPricingMatchesAdvertisedCatalog|NoPerTokenPriceLooksLikeAUnitError|SameVendorTablesAgreeOnSharedModels|VideoModelsCarryPerCallOrPerSecondPricing|CatalogFloatLiterals)' -count=1 -timeout 5m
go test ./relay/controller -run '^Test(OpenRouter|NativeOpenAI|NativeOnlyModelOnAzure|NativePromptCache|NativeRequestControl|TierAdmissionCache|Systematic|ResolvePricingAdaptor|OutputBillingContextFromRequest|EvaluateChannelPricingReadiness|ResolveChannelPricingAdaptor|ChannelPricingReadinessMatchesDispatch|SecurityResponseTierPricingControls|SecurityMCPTierCumulativeQuote|ResponseWSTierFunding)' -count=1 -timeout 5m
go vet ./...
git diff --check
```

The final quick gate passed all **28 selected tests** and formatting for
**3,377 tracked Go files**. The broad adaptor/shared-pricing run passed
**71 test packages** and compiled helper packages without tests. The full quota
package and the focused controller, discovery, cross-provider wiring and snapshot
checks also passed. The final controller selection covers both tiered and flat
OpenRouter cache funding, prepared Responses normalization, durable Messages
owner/token holds, MCP cumulative quotes, WebSocket per-create funding, and
native GPT-6.2 receipt settlement through Chat, Responses and Messages.

The OpenRouter automatic-write and translated Responses-marker regressions
failed before their fixes and passed afterward. Independent review checked
captured aggregator prices, arithmetic, schedules, capabilities, host isolation,
Vertex paid-failure limits, and the flat-override admission call sites. The
new GPT-6.2 boundary test also caught and corrected integer division in the
OpenRouter long-context output multipliers. Four obsolete numeric-literal
exceptions were removed after their source expressions became exact quotients;
the existing guard remains strict.

The effective catalogue snapshot was regenerated with
`ONEAPI_UPDATE_CATALOG_SNAPSHOT=1 go test ./test -run '^TestEffectiveCatalogSnapshot$' -count=1 -timeout 5m`,
reviewed, and then verified without the update flag. It adds **20 model rows**,
removes **none**, and changes 68 existing rows including provider-count summaries,
prices, limits and Argon Cyber's transition from operator-only to provider
pricing. The existing fixed snapshot clock remains unchanged; this is a
regression fixture for the current definitions, not a historical price claim.
All changed handwritten Go files remain within 800 lines. The coverage ledger
matches all 51 directories and all 11 relative documentation links resolve.

No authenticated provider inference was performed. Full repository-wide race,
multi-database integration and frontend qualification were not run. The
[existing testing policy](ci-testing.md) and automatic CI workflows remain
unchanged; required remote security, dependency and structural guardrails are
reported separately by the PR checks. Local results above do not claim those
remote checks have already completed.

[Frontier]: research/20261122_frontier_model_audit.md
[China]: research/20261122_china_model_audit.md
[Aggregators]: research/20261122_aggregator_model_audit.md
[Other]: research/20261122_other_model_audit.md
