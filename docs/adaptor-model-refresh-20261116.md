# Adaptor model refresh — 2026-11-16

## Purpose and result

Refresh the model information exposed by the gateway against current official
provider catalogs, model cards, API schemas, pricing tables and retirement
notices. The review covers all **51 directories** under `relay/adaptor`, including
shared protocol/helper directories. A reviewed provider does not necessarily
need a code change: several frontier catalogs were already updated on main.

The changes add published compatible model IDs, correct host-specific prices,
cache rates, context/output limits, supported controls and aliases, and annotate
announced retirements. Historical IDs remain configurable. Provider currency,
region, discount, tier and time-window distinctions are preserved, and operator
pricing overrides keep their existing precedence.

Model data remains directly maintained Go source. Each model has one complete
provider definition, and the existing duplicate-key checks cover catalog
assembly. No runtime catalog download or generated JSON dependency is added.

### Catalog organization

All 46 adaptors with catalogs have a stable `relay/adaptor/<provider>/models.go`
entry point. Small catalogs are consolidated there; larger catalogs assemble files
named for model families or capabilities. New entries are placed with their
siblings, including entries previously spread across monthly research batches.
The production layout no longer uses research dates as catalog boundaries.
Dates remain in provenance comments and these audit records.

For example, AliBailian's Qwen, DeepSeek and GLM definitions are grouped by their
families instead of being divided between the family files and
`models_november.go`. OpenRouter's GPT and Qwen catalogs use named subfamilies
instead of numbered continuation files. The 800-line handwritten-file limit
still applies. See the [maintenance guide](../relay/adaptor/MODELS.md) for the
entry points, update rules and verification procedure.

## Detailed source and decision records

| Record | Scope |
| --- | --- |
| [Frontier and cloud audit](research/20261116_frontier_model_audit.md) | OpenAI, Anthropic, Gemini, compatible Gemini, Vertex, Azure, Bedrock, AIProxy and xAI |
| [Chinese provider audit](research/20261116_china_model_audit.md) | Ali/Bailian, MiniMax, BigModel/Z.AI, Qianfan, Ark and the other Chinese provider adaptors |
| [Aggregator audit](research/20261116_aggregator_model_audit.md) | OpenRouter, DeepInfra, Novita and Together, including exact new IDs and changed prices |
| [Additional provider audit](research/20261116_other_model_audit.md) | Mistral, Cohere, Cloudflare, NVIDIA, TypeSafe, Replicate and other hosted/local adaptors |
| [Fireworks and SiliconFlow audit](research/20261116_fireworks_siliconflow_audit.md) | Standard serverless additions, MOSS-Image and lifecycle notices |

Each record links primary provider evidence and distinguishes a published model
from a supported gateway transport. Public access requirements alone do not
remove a compatible model from discovery. Unknown prices and limits remain
unknown; they are not converted into free tariffs or copied from a sibling host.

## Mutable alias and legacy ID changes

This section lists existing model IDs whose behavior or price changes in this
refresh. Newly added IDs are covered elsewhere. A client or channel mapping that
already uses one of these IDs gets the new behavior without changing its
request. To keep the previous model, map to a pinned ID where the provider
publishes one.

**`mistral-large-latest` (Mistral channel) now targets Mistral Large 4.**

| | Before (Large 3) | After (Large 4) |
| --- | --- | --- |
| USD per 1M input / cache read / output | 0.50 / 0.05 / 1.50 | 0.70 / 0.07 / 2.10 |
| Context | 262,144 | 1,000,000 |
| `reasoning_effort` | Not advertised; always omitted upstream | `low`, `medium`, `high`; provider default `high` |

- Supported effort values are trimmed and lowercased before forwarding.
  Unsupported values are dropped rather than sent to Mistral, which would reject
  them; the provider default then applies. Mistral removed `none` from Large 4 on
  October 29, so `none` is dropped for this model. `minimal`, `xhigh` and unknown
  values are also dropped. This applies to Chat `reasoning_effort` and to
  Responses `reasoning.effort`. Models that document `none`, such as
  `mistral-small-latest` and `mistral-medium-latest`, still forward it.
- `?thinking=true` now injects `reasoning_effort: "high"` for this alias.
  Previously, no effort reached Mistral for this alias.
- The pinned Large 3 ID `mistral-large-2512` keeps 0.50 / 0.05 / 1.50. The
  expired Large 4 launch discount applies only to the pinned Large 4 IDs, not
  the alias.

**`voxtral-mini-tts-latest` (Mistral channel)** now targets Voxtral Mini TTS 3
preview. Its price changes from $16 to $13 per million input characters. The
March 2026 pinned IDs (`voxtral-mini-tts-2603`, `voxtral-tts-2603`) keep $16.

**Cohere North legacy spellings.** Cohere publishes `north-mini-code-1.0` and
`north-small-translate-1.0`. The earlier gateway spellings `north-mini-code-1-0`
and `north-small-translate-1-0` remain in the catalog so that existing channel
model mappings and direct requests keep working. On every chat entrypoint
(Chat, streaming, Responses and Claude Messages), the adaptor rewrites them to
the published dotted IDs before sending the request upstream. Billing and logs
keep the name configured by the administrator, and both spellings share the
same free public-API tariff.

**OpenRouter `~…-latest` routing aliases** have new routing-default quotes (USD
per 1M input / cache read / output; parenthesized values are cache writes or
the long-context tier):

| Alias | Before | After |
| --- | --- | --- |
| `~anthropic/claude-fable-latest` | 10 / 0.25 / 50 (cache write 5m 12.5, 1h 20) | 8 / 0.20 / 40 (cache write 5m 10, 1h 16) |
| `~deepseek/deepseek-pro-latest` | 0.24948 / 0.007938 / 0.74844 | 0.0999 / 0.0999 / 8.0 |
| `~deepseek/deepseek-flash-latest` | 0.0336 / 0.0336 / 1.188 | 0.036 / 0.01 / 0.5 |
| `~deepseek/deepseek-v4-flash-latest` | 0.0128 / 0.0128 / 0.768 | 0.0434 / 0.035 / 0.3 |
| `~google/gemini-pro-latest` | 2 / 0.2 / 12 (above 200K input: 4 / 0.4 / 18) | 2 / 0.1 / 10, long-context tier removed |
| `~google/gemini-flash-latest` | 0.75 / 0.075 / 3.75 | 1.5 / 0.075 / 7.5 |
| `~moonshotai/kimi-latest` | 0.72 / 0.3 / 24 | 0.1824 / 0.18 / 11.0055 |
| `~openai/gpt-terra-latest` | 2 / 0.2 / 12 (above 272K: 4 / 0.4 / 18) | 1 / 0.05 / 5 (above 272K: 2 / 0.1 / 7.5) |
| `~z-ai/glm-flash-latest` | 0.0232 / 0.0232 / 1.2 | 0.0399 / 0.03 / 0.0784 |
| `~z-ai/glm-latest` | 0.055 / 0.06 / 5 | 0.0699 / 0.0689 / 1.4 |

The `~deepseek/*-latest` and `~moonshotai/kimi-latest` aliases also advertise
`none` as a reasoning effort.

## Implementation highlights

### Native OpenAI and billing

Register `gpt-6-cyber`, `gpt-image-2.6-sunburst`, and `gpt-image-2.6-flare`.
Cyber uses its own Standard tariff and an input-only long-context boundary above
272,000 tokens. Preserve system instructions and tools through the existing
Chat/Claude-to-Responses conversion. Pin the implemented Standard service tier,
reject unsupported tier/cache-retention controls, and preserve actual receipt
information for settlement. A long-context cache-write tariff is not published;
those receipts remain unpriced unless an applicable explicit operator tier
supplies a rate. This prevents inheriting a short-context write rate silently.

OpenAI caches GPT-5.6+ prompts implicitly by default, so cache writes are the
normal case rather than an exception. Admission therefore funds implicit 30m
writes in short-context tiers. Above 272,000 input tokens it pins
`prompt_cache_options.mode="explicit"`, which disables the implicit write, and
refuses explicit client write controls with HTTP 400. This applies unless an
operator tier prices long-context writes. Without the pin, nearly every
long-context Cyber receipt settled its whole reservation as an estimate. See
"Native OpenAI Prompt-Cache Admission" in `docs/arch/billing.md`.

Image 2.6 uses the established Images API reservation/fallback policy and the
five distinct text/image/cache/output token buckets. Exact model matching avoids
invented dated aliases. A positive operator fallback is still required for a
missing or unusable receipt. Actual token usage can refund excess reservation.
The fallback is operator policy, not an invented official per-image tariff.
A receipt with usable totals but an incomplete modality or cache split (for
example `input_tokens` without `input_tokens_details`) settles the most
expensive assignment consistent with its totals: unclassified input at the
higher of the text/image input rates, and an unsplit cache read credited where
the cache discount is smallest. The charge is logged as
`billing_source=provider_tokens_upper_bound` with `billing_estimated` metadata;
the configured fallback is also flagged as an estimate. Previously unclassified
input was billed at the text rate and an unsplit cache read fell back to the
operator price, both of which could under-bill.
On OpenAI and Azure channels the Images API also accepts the 2.6 request
controls: `complexity` (`low`, `medium` or `high`, valid only for the two 2.6
IDs and rejected for older models), `safety_identifier` (at most 128
characters, forwarded alongside the deprecated `user`), and `output_format`
limited to `png`, `jpeg` and `webp`. Azure documents neither `complexity` nor
`safety_identifier` for image requests, so Azure generation bodies keep
omitting both, exactly as before these fields were decoded. SVG and GIF output,
whose billing differs from the qualified image-token tariff, are rejected
before quota reservation.

A native OpenAI or Azure OpenAI non-2xx response whose body nevertheless
carries an explicit usage receipt (a root `usage` object or a failed Responses
envelope) is settled exactly like a 2xx response with the same receipt and is
never refunded or replayed on another channel. OpenAI's documented error schema
carries no usage, so this closes a possible rather than an observed
under-billing window; error bodies without a receipt keep the existing
retained-hold and retry path. See
[billing architecture](arch/billing.md#paid-upstream-error-receipts).

Azure and legacy AIProxy no longer automatically inherit these three new native
IDs without their own provider contract. Existing mappings and historical
membership remain. Native lifecycle notices are updated without assigning
OpenAI shutdown dates to Azure deployments. The exclusion is enforced where
prices are resolved, not only in discovery: both adaptors declare the IDs
through `adaptor.OperatorTariffPolicy`, so no pricing layer substitutes native
OpenAI's global tariff, and dispatch is refused before any upstream contact
unless the channel's `model_configs` supplies a complete operator tariff (see
[billing architecture](arch/billing.md#operator-only-tariffs)).

### Provider-owned catalogs

- Alibaba and Bailian gain independent Qwen4 Max, MiniMax M3.1 Flash, Kimi K3.1,
  GLM 5.3 Prime, MiMo Ultraspeed and Flash 1017 entries. Their hosted MiniMax M3
  long-context prices are corrected.
- Native MiniMax Flash and BigModel/Z.AI Legal entries use their own limits,
  controls and CNY/USD tariffs. Qianfan gains lowercase current IDs and correctly
  ends its documented promotion at the exclusive December 1 local boundary.
- OpenRouter adds 43 compatible IDs and refreshes current routing quotes, Google
  explicit-cache/audio pricing, long-context thresholds and UTC schedules.
  DeepInfra applies public discounts once; Novita preserves exact decimal and
  Highspeed cache prices; Together keeps its Standard serverless contract.
- Mistral Large 4 uses current GA pricing and the documented latest alias. Its
  expired launch window remains on the applicable pinned IDs. Voxtral 3 uses
  the existing duration/character audio billing paths.
- Cohere Embed 5 gains its documented dimensions and tariffs while preserving
  model-specific older validation. NVIDIA hosted limits and effort vocabularies
  remain distinct from model-creator maxima. NVIDIA forwards a reasoning effort
  only when the model's hosted vocabulary lists it (trimmed and lowercased);
  any other value, including one arriving as a Responses `reasoning.effort`, is
  dropped so the provider applies its default. NVIDIA, xAI (Chat and native
  Responses), Mistral, Moonshot and Vertex GPT-OSS now share one helper,
  `adaptor.SupportedReasoningEffort`. Replicate Gemini 3.1 Pro now applies
  its already documented expensive bracket above 200,000 input tokens.
- Cloudflare and Fireworks gain hosted additions with their own cache prices and
  actual capabilities. SiliconFlow MOSS-Image uses CNY 0.30/image under the
  existing one-image JSON protocol. Announced retirements retain legacy IDs.

## Complete directory disposition

The evidence links point to the detailed records above; each contains the
relevant official model/API/pricing sources.

| Directory | Disposition | Evidence / reason |
| --- | --- | --- |
| `ai360` | Evidence comments corrected | [China](research/20261116_china_model_audit.md): current aggregate endpoint and supplier prices do not establish the legacy route's tariff. |
| `aiproxy` | New-model inheritance isolated | [Frontier](research/20261116_frontier_model_audit.md): legacy library API has no verified contract for the three new native OpenAI IDs. |
| `ali` | Updated | [China](research/20261116_china_model_audit.md): eight Beijing-priced additions, hosted M3 tiers, native text transport limits. |
| `alibailian` | Updated | [China](research/20261116_china_model_audit.md): independently owned compatible additions and media capabilities. |
| `anthropic` | Current | [Frontier](research/20261116_frontier_model_audit.md): Fable/Mythos 5.2 and existing tariffs already match main. |
| `aws` | Current implemented families; gaps recorded | [Frontier](research/20261116_frontier_model_audit.md): new Moonshot/closed OpenAI offerings require native transport/profile work. |
| `azure` | New-model inheritance isolated | [Frontier](research/20261116_frontier_model_audit.md): Foundry evidence is separate from native OpenAI. |
| `baichuan` | Current | [China](research/20261116_china_model_audit.md): published chat, embedding and per-call distinctions retained. |
| `baidu` | Historical contract retained | [China](research/20261116_china_model_audit.md): legacy OAuth/V1 path is distinct from Qianfan V2. |
| `baiduv2` | Updated | [China](research/20261116_china_model_audit.md): ERNIE/DeepSeek IDs, decimal-K boundary and promotion expiry. |
| `cerebras` | Current | [Additional](research/20261116_other_model_audit.md): public API matches existing hosted defaults. |
| `cloudflare` | Updated | [Additional](research/20261116_other_model_audit.md): two chat models, EmbeddingGemma 2 and November 20 notices. |
| `cohere` | Updated; Rerank 5 deferred | [Additional](research/20261116_other_model_audit.md): Embed 5 dimensions and canonical North IDs; new rerank token meter needs implementation. |
| `common` | Shared helper | No provider catalog; affected conversions are exercised by the provider suites. |
| `copilot` | Configured contract retained | [Additional](research/20261116_other_model_audit.md): product labels/credits do not establish public model IDs and token tariffs. |
| `coze` | Configured contract retained | [Additional](research/20261116_other_model_audit.md): configured bot route; no inferred token price from subscriptions. |
| `deepinfra` | Updated | [Aggregators](research/20261116_aggregator_model_audit.md): five additions, discount/cache-unit corrections and capability tags. |
| `deepl` | Evidence comments clarified | [Additional](research/20261116_other_model_audit.md): source-character meter and historical regional default remain explicit. |
| `deepseek` | Current; tooling note clarified | [China](research/20261116_china_model_audit.md): native aliases/prices match; new server tools are separate. |
| `doubao` | Updated; clock gap preserved | [China](research/20261116_china_model_audit.md): Ark Kimi addition; variable Flash tariffs use completion time. |
| `fireworks` | Updated | [Fireworks/SiliconFlow](research/20261116_fireworks_siliconflow_audit.md): five Standard IDs and duplicate-rejecting assembly. |
| `gemini` | Current | [Frontier](research/20261116_frontier_model_audit.md): November 12 Hydrogen and November 10 Carbon already present. |
| `geminiOpenaiCompatible` | Current | [Frontier](research/20261116_frontier_model_audit.md): compatible subset and reasoning conversion preserved. |
| `groq` | Current | [Additional](research/20261116_other_model_audit.md): production/preview model table matches existing defaults. |
| `internal` | Shared helper | No independent upstream model/pricing contract. |
| `jina` | Current implemented products | [Additional](research/20261116_other_model_audit.md): native embeddings/rerank/OCR retain their distinct units. |
| `lingyiwanwu` | Lifecycle updated | [China](research/20261116_china_model_audit.md): public API shutdown documented; historical Yi IDs retained. |
| `minimax` | Updated | [China](research/20261116_china_model_audit.md): native M3.1 Flash and published M3 controls/limits; existing catalog-only adaptor limitation stated. |
| `mistral` | Updated | [Additional](research/20261116_other_model_audit.md): Large 4, hosted models, Voxtral 3 and native unit prices. |
| `moonshot` | Current | [China](research/20261116_china_model_audit.md): K3.1 Pro/Highspeed already independently priced. |
| `muapi` | Dynamic contract retained | [Additional](research/20261116_other_model_audit.md): quote/receipt-driven native video models remain dynamic. |
| `novita` | Updated | [Aggregators](research/20261116_aggregator_model_audit.md): ten additions and exact cache/write decimal prices. |
| `nvidia` | Updated | [Additional](research/20261116_other_model_audit.md): nine trial endpoints with hosted schemas and conditional effort forwarding. |
| `ollama` | Deployment distinction clarified | [Additional](research/20261116_other_model_audit.md): local installed tags determine availability; symbolic historical defaults retained. |
| `openai` | Updated | [Frontier](research/20261116_frontier_model_audit.md): Cyber, Image 2.6, lifecycle and qualified billing contracts. |
| `openai_compatible` | Shared protocol reviewed | [Additional](research/20261116_other_model_audit.md): no separate provider catalog; historical forwarding remains. |
| `openrouter` | Updated | [Aggregators](research/20261116_aggregator_model_audit.md): public host quotes, compatible IDs, cache/audio and schedules. |
| `palm` | Historical contract retained | [Additional](research/20261116_other_model_audit.md): no new verified current PaLM tariff or endpoint. |
| `proxy` | Configured contract retained | [Additional](research/20261116_other_model_audit.md): operator-selected upstream models and pricing. |
| `replicate` | Pricing/limits corrected | [Additional](research/20261116_other_model_audit.md): Gemini tier encoded; unconfirmed GPT-5.6 limits removed. |
| `siliconflow` | Updated | [Fireworks/SiliconFlow](research/20261116_fireworks_siliconflow_audit.md): MOSS-Image and six matching retirements. |
| `stepfun` | Current implemented products | [China](research/20261116_china_model_audit.md): existing cache policy retained; future audio contracts remain separate. |
| `tencent` | Legacy contract retained | [China](research/20261116_china_model_audit.md): TokenHub migration requires its own authentication/transport. |
| `togetherai` | Updated | [Aggregators](research/20261116_aggregator_model_audit.md): nine additions, current Kimi price and hosted metadata. |
| `typesafe` | Alias metadata updated | [Additional](research/20261116_other_model_audit.md): latest/preview now select Jev 1.14. |
| `vertexai` | Current implemented routes | [Frontier](research/20261116_frontier_model_audit.md): independent Vertex contracts retained; new Responses-only partners deferred. |
| `xai` | Current | [Frontier](research/20261116_frontier_model_audit.md): Grok tariff and November 2 image migration already represented. |
| `xunfei` | Legacy transport retained | [China](research/20261116_china_model_audit.md): WebSocket products are distinct from MaaS. |
| `xunfeiv2` | Legacy transport retained | [China](research/20261116_china_model_audit.md): new MaaS X2.6 endpoint is not the legacy Spark route. |
| `zai` | Updated | [China](research/20261116_china_model_audit.md): Legal model with independently published USD pricing. |
| `zhipu` | Updated | [China](research/20261116_china_model_audit.md): Legal model with native CNY pricing. |

## Limits that remain explicit

This refresh does not certify private deployments, negotiated prices, account
eligibility or live inference. Intraday aggregator quotes can change; the
aggregator audit identifies the captured evidence and follow-up corrections.
Optional server tools, media, storage, service tiers and dedicated endpoints may
use different billing units and are not folded into a cheap text-token rate.

Material follow-ups include Rosalind's missing model-specific long-context
boundary, Astra for Law's forthcoming API, Bedrock/Vertex native transport gaps,
Cohere Rerank 5's token meter, Ark completion-time settlement, DeepInfra explicit
retention normalization, and Replicate Carbon's native reasoning receipts.
The inherited GPT Image settlement path also treats a zero computed token
charge as missing usage: a zero legacy token-price override with a positive
fallback keeps that fallback. Zero group pricing is rejected by admission.
Separating valid free receipts from unusable receipts is a billing follow-up;
this refresh retains the existing image policy.
These decisions are based on a missing price or protocol contract, not solely on
an access program. Source-specific details and legacy limitations are recorded
in the linked audits.

## Validation

### Model and billing refresh

Validation completed successfully with Go 1.27.4:

```sh
make test-quick
go test -p 2 ./relay/adaptor/... ./relay/pricing ./relay/quota ./relay/model ./relay -count=1
go test -p 2 ./relay/controller -run '^Test(GPT6Cyber|GPTImage25|GPTImage26|ProtocolAuditAudioHTTP)' -count=1
go test -race ./relay/adaptor/openai -run 'TestGPT6Cyber' -count=1
go vet -p 2 ./...
git diff --check
```

The existing quick gate passed all 28 selected tests and formatting for 3,083
tracked Go files. The broad adaptor/shared-pricing run passed every test package;
helper packages without tests compiled successfully. The focused controller run
includes the 18-case Cyber HTTP/SQLite matrix, existing and new image settlement
regressions, and the audio HTTP/ledger tests. The focused Cyber race run passed.
All changed manually maintained Go files are below the 800-line limit, and the
51-directory audit ledger and its relative document links were checked.

Independent review compared changed aggregate prices/limits, all published
OpenRouter tiers/schedules and Together chat rows with the public snapshots. It
also checked native/host isolation, Cohere dimensions, NVIDIA effort forwarding,
Replicate tier boundaries, Mistral aliases, and image reservation behavior. The
Cyber host-isolation regression failed before its fix and passed afterward;
the image 2.6 billing regression likewise failed before registering the exact
new model policy. Prior price fixtures were refreshed from official evidence.

These are local tests with synthetic upstream HTTP responses and isolated
SQLite fixtures. No paid provider generation was performed. Full multi-database,
repository-wide race and frontend qualification were not run; the existing lean
CI gate remains unchanged. The detailed audits identify inherited limitations
and the model/protocol contracts requiring further work.

### Catalog consolidation

The subsequent layout cleanup was compared against commit
`823861c9572bce6314879e46a9534ad36e3448e2`, before moving any definitions.
`TestCompiledAdaptorCatalogSnapshot` captured all 62 registered API/channel
configurations before and after the cleanup. All 6,234 pricing records and
6,115 discoverable model IDs match, including descriptions, capabilities,
nested tariffs, thresholds and schedules. Only the top-level discovery lists
were sorted for comparison because `GetModelListFromPricing` already iterates
unordered Go maps; configuration slices and all numeric values were compared
exactly. The canonical snapshots have the same SHA-256:
`6ef23c0b221d6fd0d6318a6c1db99cd572b787293ecc292ae498bf6bcd2a1d72`.

Checks repeated successfully on the consolidated tree:

```sh
make test-quick
go test -p 2 ./relay/adaptor/... ./relay/pricing ./relay/quota ./relay/model ./relay -count=1
go vet -p 2 ./...
git diff --check
```

The quick gate passed all 28 selected tests and formatting for 3,081 tracked Go
files. The broad run passed 72 test packages and compiled 12 packages without
tests. All 260 changed Go files are within the 800-line limit; the largest has
686 lines. No production catalog filename retains a research date or month.
Independent review checked family registration, ownership, initialization
order, aliases, lifecycle annotations and active documentation references.
All 1,002 model expressions in the four aggregator catalogs are unchanged,
and an isolated xAI comparison also confirmed identical tooling defaults.
