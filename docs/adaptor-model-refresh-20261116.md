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

Image 2.6 uses the established Images API reservation/fallback policy and the
five distinct text/image/cache/output token buckets. Exact model matching avoids
invented dated aliases. A positive operator fallback is still required for an
ambiguous or missing receipt. Actual token usage can refund excess reservation.
The fallback is operator policy, not an invented official per-image tariff.

Azure and legacy AIProxy no longer automatically inherit these three new native
IDs without their own provider contract. Existing mappings and historical
membership remain. Native lifecycle notices are updated without assigning
OpenAI shutdown dates to Azure deployments.

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
  remain distinct from model-creator maxima. Replicate Gemini 3.1 Pro now applies
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
