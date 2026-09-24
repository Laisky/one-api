# Adaptor model-catalog review — 2026-09-24

## Delivery and acceptance

PR: [#425](https://github.com/Laisky/one-api/pull/425). Branch: `feat/adaptor-catalog-audit-20260924`. Baseline: `4b455cd03126c3358bde5fda19262d97b31e3300`.

All **47 catalog surfaces** now have a disposition below: 34 registered adaptors plus 13 provider catalogs selected through the generic OpenAI channel. Shared implementation directories `common`, `internal`, and `openai_compatible` are not additional providers. `TestCatalogReviewInventory` checks that this table covers every catalog directory exactly once. The compiled-catalog test also examines all 34 API registrations and 27 generic channel configurations after initialization.

**This delivery is a catalog review and a verified update, not a claim that every upstream native API or every historic price is implemented or newly certified.** Source-blocked tariffs, deployment-specific catalogs, and unsupported billing/protocols are explicitly retained or excluded below. No model permissions were inferred from this account. No paid inference, production deployment, automatic model-ID rewriting, or PR merge was performed. Groq [#424](https://github.com/Laisky/one-api/pull/424) and MuAPI [#423](https://github.com/Laisky/one-api/pull/423) remain independent.

The bulk update is published in `ddfeecca19ac993a196d4d5765afd7254b4cb8c9`. Its validating [runner](https://github.com/Laisky/one-api/actions/runs/36001316470) passed all Python converter tests, regenerated the catalogs, ran `go test -race -count=1 ./relay/adaptor/...` and `go vet ./relay/...`, and passed whitespace checks **before** committing the result. Fireworks, Cloudflare, and Doubao follow-up tests also passed locally under Go 1.27.1. The PR checks are authoritative for the final head, including inventory and cleanup changes; an earlier run is not substituted for final-head CI.

All temporary evidence/publication workflows and encoded patch fragments are removed in the final diff. There is no runtime catalog fetch, added CI permission, or deployment workflow in the delivered change.

## Reproducible snapshots

| Table | Reconciled rows | Scope |
| --- | ---: | --- |
| OpenRouter | 351 | Published synchronous prices and supported metadata; separate cache reads/writes and context tiers. |
| DeepInfra | 123 | Published serverless tariffs, with native units and separate discount applied once. |
| Novita | 108 | Exact provider IDs, pricing, context, and representable metadata. |
| Together | 20 | Serverless table; affirmative capabilities preserve existing unmentioned features. |
| Baichuan | 12 | Native CNY prices and documented time windows. |
| Alibaba | 143 | Beijing/domestic CNY, per-model tiers and cache/daypart rules. |
| AliBailian | 143 | Same verified domestic source, applied to its separately selected channel. |
| SiliconFlow China | 36 | CNY catalog, independently priced caching and clock windows. |
| Qianfan V2 | 13 | Native IDs, input tiers, dayparts and dated holiday discounts. |
| **Total** | **949** | Refreshed rows, **not 949 new models**. |

Each table has a deployed JSON snapshot, a generated Go embedding wrapper, and an inclusion/exclusion report in `docs/research/model_catalog_20260924/`. Exact source URLs, byte-level SHA-256 hashes, and parsers are in `scripts/model_catalog/`. See its [regeneration instructions](../../scripts/model_catalog/README.md).

The loader rejects malformed schema, non-finite numbers, unsupported fields, and incomplete new prices. It clones nested metadata before overlaying explicitly supplied fields. Absent metadata is not interpreted as false or free. Old IDs and unspecified compatibility defaults are retained; exclusions do not become a request denylist. A catalog context field does not set an account-specific `MaxTokens` cap.

## Verified behavior and billing corrections

**Cerebras, Z.AI and Azure:** retain the earlier current-model additions, provider-specific reasoning vocabularies and independent USD prices. Cerebras Qwen's public machine-readable limit differs from the prose paid-tier context; the discrepancy is retained in its description rather than converted into a request restriction. Azure adds current Claude discovery without pretending every regional contract has first-party rates.

**AWS:** add Opus 5.5's canonical pricing, native ID registry and published geographic/profile forms together. Regional routing is tested rather than assuming a metadata entry is routable. Bedrock-specific capability differences are not copied blindly from Anthropic.

**DeepSeek:** remove the superseded September 14 Pro-to-Flash pricing transition. The revised official notice preserves V4 Pro and its billing. Flash keeps its own actual September pricing schedule. Regression tests cover both sides of the formerly scheduled cutover.

**Domestic prices:** Alibaba's strict 128K boundary starts at 128,001 tokens, SiliconFlow's inclusive boundary at 128,000, and Qianfan's strict 32K boundary at 32,001. Real resolver tests cover the adjacent counts. Qianfan's holiday window is September 24 inclusive through October 8 exclusive in Asia/Shanghai; tests cover all input/cache/output prices and 08:00/22:00 clock changes. The tariffs are not copied between hosts merely because the model weights are related.

**DeepInfra:** its source represents native prices and a separate discount. Both are normalized once; cached-input multipliers are not mistaken for absolute USD prices. Fixture tests prevent silent double-discounting.

**BigModel:** add GLM-5.3-FlashX at its own CNY 2 input / 0.57 cache / 7 output per million, separate from Z.AI. Correct TTS character units, ASR input/output prices, search-call currency and voice-clone pricing. CNY 6 per clone converts through the canonical conversion rather than a rounded integer quota factor.

**Mistral:** preserve/promote supported reasoning efforts, map native request controls, and convert native thinking/text content arrays for JSON and SSE. Tests cover precedence, request immutability, tool payloads, multiline events, usage/errors, malformed chunks and body closure. Bounded lazy conversion introduces no goroutine. Add hosted GLM/Leanstral entries and 15 independently published cache prices.

**DeepL:** bill Unicode code points rather than UTF-8 bytes. JSON and streaming tests cover Chinese, emoji, accented and combining characters, and empty input.

**Fireworks and Cloudflare:** add Fireworks Ember-1 and DeepSeek V4.1 Flash plus Cloudflare GLM-5.3 and GLM-5.3-Flash, with each host's own rates. Preserve exact model IDs, image/tool payloads and independently published context fields. No unpublished output ceiling, precision or reasoning default is invented.

**Doubao:** reproduce and fix a raw-token unit error in the existing Seed 1.6 compatibility tariff. Higher bands previously activated at 32 and 128 tokens; the documented closed upper bounds require 32,001 and 128,001. Rates are preserved. This does not certify a new September tariff or invent prices for the latest endpoint-specific releases.

## Surface-by-surface disposition

“Retain” means an intentional compatibility or deployment-specific decision, not a claim that every historical price is current. “Evidence-limited” identifies a real limitation encountered after checking the indicated source; no guessed substitute quote is installed.

| Surface | Disposition | Checked source and implementation decision |
| --- | --- | --- |
| `ai360` | Evidence-limited; retain | [Official platform](https://ai.360.com/platform/docs) did not expose a usable current price table. Preserve the existing generic-channel entries, not an invented free or current tariff. |
| `aiproxy` | Delegated; retain | Library requests and pricing delegate to the existing OpenAI catalog. Tenant library IDs are configuration, not globally published models; no independent reseller tariff is inferred. |
| `ali` | Updated snapshot | [Domestic model prices](https://help.aliyun.com/zh/model-studio/model-pricing): 143 representable rows, including source-specific cache and time windows. Thinking-mode price differences that cannot be represented safely are recorded, not flattened. |
| `alibailian` | Updated snapshot | Independently selected generic channel receives the same 143 domestic rows; no international USD rate is substituted. |
| `anthropic` | Current release entries retained | [Model overview](https://platform.claude.com/docs/en/models/overview): Opus 5.5, Fable/Mythos 5.1 and permanent Sonnet 5 rates already exist. First-party retirement is not applied indiscriminately to cloud compatibility IDs. |
| `aws` | Updated | [Opus 5.5 card](https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-opus-5-5.html) and [pricing](https://aws.amazon.com/bedrock/pricing/): coordinated native IDs, regional/profile handling and tariffs. Provisioned and deployment contracts remain operator-specific. |
| `azure` | Updated discovery | [Foundry Claude](https://learn.microsoft.com/azure/foundry/foundry-models/concepts/claude-models): four additional IDs. Existing native Messages and family pricing dispatch are preserved, including custom deployment-name mapping. |
| `baichuan` | Updated snapshot | [Native price table](https://platform.baichuan-ai.com/prices): 12 exact IDs with CNY/unit and clock handling. Search fees are not silently folded into a token price. |
| `baidu` | Retain legacy transport | OAuth/native legacy model routing differs from Qianfan V2. Do not copy V2 IDs into the older request protocol or change authentication as a catalog update. |
| `baiduv2` | Updated snapshot | [Qianfan pricing](https://cloud.baidu.com/doc/qianfan/s/wmh4sv6ya): 13 safe rows, exact case-sensitive IDs, holiday and daypart boundaries. Malformed/ambiguous source rows are excluded explicitly. |
| `cerebras` | Updated | [Public models API](https://api.cerebras.ai/public/v1/models), [overview](https://inference-docs.cerebras.ai/models/overview): Qwen 3.8, GPT-OSS limits and capability corrections. Dedicated Gemma/GLM historical defaults remain labeled as non-quotes. |
| `cloudflare` | Updated | [Workers AI catalog](https://developers.cloudflare.com/workers-ai/models/), [GLM 5.3](https://developers.cloudflare.com/workers-ai/models/glm-5.3/), [Flash](https://developers.cloudflare.com/workers-ai/models/glm-5.3-flash/): add the two GLM entries to existing Qwen/DeepSeek updates. Non-token native tariffs are not converted blindly. |
| `cohere` | Updated North entries | [North Mini Code](https://docs.cohere.com/docs/north-mini-code-1.0) and [Translate](https://docs.cohere.com/docs/north-small-translate-1.0): explicitly free rate-limited API access. Basic text conversion is tested; native tool/reasoning controls require a separate protocol implementation. Legacy estimates are not newly certified. |
| `copilot` | Deployment/plan-specific; retain | [Supported models](https://docs.github.com/en/copilot/reference/ai-models/supported-models): plan availability and premium-request multipliers are not public USD/token tariffs. Preserve dynamic/configured discovery rather than inventing token prices. |
| `coze` | Bot-specific; retain | [Coze pricing](https://www.coze.com/premium): bot/deployment IDs and credit contracts are not a global LLM model-price list. Retain administrator configuration. |
| `deepinfra` | Updated snapshot | [Model API](https://api.deepinfra.com/models/list): 123 safe entries after native-unit/discount conversion; 260 excluded rows have reasons, including incompatible native billing shapes. |
| `deepl` | Updated metering | [Language API](https://developers.deepl.com/docs/getting-started/supported-languages): aliases are target languages, not LLM IDs. Preserve the existing rate while correcting source-character counting. |
| `deepseek` | Updated transition | [Pricing](https://api-docs.deepseek.com/quick_start/pricing) and [revised announcements](https://api-docs.deepseek.com/updates/): cancel the obsolete Pro transition without overwriting independent Flash windows. |
| `doubao` | Unit bug fixed; new tariffs evidence-limited | [Models](https://www.volcengine.com/docs/82379/1330310) identify newer endpoint-specific releases; [pricing](https://www.volcengine.com/docs/82379/1544106) redirects to a JS-only surface. Fix existing tier units, but do not copy another host's prices into unquoted September IDs. |
| `fireworks` | Updated | [Live serverless catalog](https://fireworks.ai/models?modelTypes=Serverless), [Ember-1](https://fireworks.ai/models/fireworks/ember-1), [V4.1 Flash](https://fireworks.ai/models/deepseek-ai/deepseek-v4p1-flash): two additions at Fireworks rates. Dedicated GPU quotes are not per-token serverless prices. |
| `gemini` | Shared catalog reviewed; retain | [Current models](https://ai.google.dev/gemini-api/docs/models): current text/Omni entries already present. New 3.8 TTS protocol exclusions are recorded below; page slugs are not treated as API IDs. |
| `geminiOpenaiCompatible` | Shared catalog reviewed; retain | Supplies Gemini metadata and prices to multiple routes. Do not infer native audio/Live transport support from an OpenAI-compatible chat route. |
| `groq` | Separate completed patch | [Supported models](https://console.groq.com/docs/models): updates are in #424. This PR does not duplicate or merge that independent change. |
| `jina` | Current hosted entries retained | [Native OpenAPI](https://api.jina.ai/openapi.json) checked against the catalog. SDK/on-prem model names do not automatically imply hosted endpoints or a public hosted tariff. |
| `lingyiwanwu` | Evidence-limited; retain | [Official platform](https://platform.lingyiwanwu.com/docs) did not yield a usable current hosted price matrix. Keep historical defaults without labeling them newly verified quotes. |
| `minimax` | Standard tariff retained | [Model list](https://platform.minimaxi.com/docs/guides/models-intro) and [pricing](https://platform.minimaxi.com/docs/guides/pricing-paygo): existing M3/M2.7/M2.5 standard rates reviewed. Priority pricing is not substituted for ordinary requests. |
| `mistral` | Updated catalog and protocol | [Pricing](https://docs.mistral.ai/inference/pricing), [reasoning](https://docs.mistral.ai/studio/conversations/reasoning): cache prices, hosted GLM/Leanstral and actual JSON/SSE thinking compatibility. OCR and other unsupported native routes are not advertised as chat. |
| `moonshot` | Current IDs retained; legacy noted | [Kimi model lifecycle](https://platform.kimi.com/docs/models): four current K3/K2.7/K2.6 IDs already present. K2.5/V1 sunset on August 31; retain compatibility rates, not live-availability guarantees. Cache-write TTL prices were not readable, so no guessed multipliers are added. |
| `novita` | Updated snapshot | [Models API](https://api.novita.ai/v3/openai/models): 108 included, 12 excluded with reasons. Account/deployment discovery remains an administrator concern. |
| `nvidia` | Deployment-specific; retain | [NIM support matrix](https://docs.nvidia.com/nim/large-language-models/latest/supported-architectures.html) is broader than hosted trial availability. Preserve curated entries; a hosted trial is not evidence of free private NIM production. |
| `ollama` | Installation-specific; retain | [Installed tags API](https://docs.ollama.com/api/tags) is the actual local inventory. Global library names are not necessarily installed; symbolic gateway prices are not upstream hosted-service quotes. |
| `openai` | Current ordinary catalog retained | [Official model index](https://developers.openai.com/api/docs/models): existing September overlay covers current ordinary models. `gpt-live-1` needs a separately metered session protocol; it is not a plain token-priced model addition. |
| `openrouter` | Updated snapshot | [Public models API](https://openrouter.ai/api/v1/models): 351 included, 108 excluded with explicit billing/availability reasons; first-party prices are not substituted for host tariffs. |
| `palm` | Retired compatibility retained | [Official retirement](https://ai.google.dev/palm_docs/deprecation): decommissioned PaLM alias is already labeled legacy. Do not silently redirect it to a differently priced Gemini model. |
| `proxy` | Administrator-specific; retain | Generic upstream proxy has no independent public model tariff. Empty/configured discovery is intentional rather than evidence that new provider IDs should be hardcoded. |
| `replicate` | Deployment/version-specific; retain | [Official collection](https://replicate.com/collections/language-models) and [pricing](https://replicate.com/pricing): model versions and GPU-time/image/video billing cannot be reconciled as a single text-token table. Preserve existing contracts; do not invent conversion rates. |
| `siliconflow` | Updated China snapshot | [China pricing](https://siliconflow.cn/pricing): 36 included, 17 excluded. China CNY and international USD prices remain distinct; exact cache/daypart/tier boundaries are tested. |
| `stepfun` | Updated | [Official prices](https://platform.stepfun.com/docs/zh/guides/pricing/details.md): add Step-5 Preview with quoted CNY rates and published metadata, without inventing a reasoning default or unreleased weight identity. |
| `tencent` | Legacy platform retained | [TC3-platform prices](https://cloud.tencent.com/document/product/1729/97731) and [migration](https://cloud.tencent.com/document/product/1729/131925): TokenHub is a different product/authentication surface. Preserve existing contracts, not an automatic migration or assumed tariff parity. |
| `togetherai` | Updated snapshot | [Serverless table](https://docs.together.ai/docs/serverless-models): 20 entries; missing capability columns do not erase known features. Dedicated-instance rates remain separate. |
| `typesafe` | Current three IDs retained | [Models](https://docs.typesafe.ai/models): `jev-1.13.0`, `jev-latest`, `jev-preview`, $0.042/M input and free output remain aligned. Native evaluation is not chat. |
| `vertexai` | Cloud-specific catalog retained | [Vertex models](https://cloud.google.com/vertex-ai/generative-ai/docs/learn/models): current Claude/Gemini registrations reviewed. Seven Live IDs intentionally require explicit realtime pricing; do not replace that safeguard with Developer API tariffs. |
| `xai` | Recent overlay retained | [Official models](https://docs.x.ai/developers/models): existing September 21 update includes Grok 4.7, aliases and dated image tariffs. Documentation slugs are not additional API model IDs. |
| `xunfei` | Native Spark route retained | [Spark API](https://www.xfyun.cn/doc/spark/Web.html): WebSocket application configuration differs from the HTTP generations. No fabricated cross-protocol ID or unquoted price is installed. |
| `xunfeiv2` | Endpoint-specific; retain | [Spark HTTP](https://www.xfyun.cn/doc/spark/X1http.html): `spark-x` is reused across versioned endpoint paths. Newer X2/X2.5 marketing names do not justify invented model slugs or automatic endpoint/price changes. |
| `zai` | Updated FlashX | [Z.AI USD prices](https://docs.z.ai/guides/overview/pricing): independent $0.37/$0.075 cache/$1.25 price. Flash's expired discount and BigModel CNY artifacts are not inherited. |
| `zhipu` | Updated native tariffs | [BigModel pricing](https://docs.bigmodel.cn/cn/guide/start/pricing): FlashX plus corrected character/audio/search/clone units. Existing Z.AI USD derivation remains independently tested. |

## Explicit native-protocol exclusions

These are known differences, not forgotten model-list rows. Adding their names to a chat price map would falsely promise usable transport or billing.

- OpenAI [`gpt-live-1`](https://developers.openai.com/api/docs/models/gpt-live-1) uses `/v1/live/sessions` and session-minute billing, with separately charged backing-model/tool calls. This gateway does not implement that duration/delegation ledger. It is not added as an ordinary Chat Completions or Realtime token SKU.
- Gemini [`gemini-3.8-flash-tts`](https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash-tts) and [`gemini-3.8-flash-lite-tts`](https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash-lite-tts) require the new speech metadata/annotation and audio-output contract. The current native audio route does not implement that contract; older TTS preview metadata is not evidence otherwise. Their dated promotional tariffs must be integrated with the native route, not transplanted into plain chat billing.
- Cohere North native tool/reasoning controls, Mistral OCR and other provider-native non-chat APIs are not certified by basic chat payload tests. The inclusion reports likewise exclude unrepresentable per-call, image, duration, request-dependent and batch-only rows.

These protocol expansions and source-blocked contract prices are outside the verified catalog changes in this PR. The review records them precisely so a future implementation has an explicit boundary rather than an apparently supported but broken entry.

## Evidence and operational limits

Checksum-pinned public source bytes are retained in [artifact 10808950479](https://github.com/Laisky/one-api/actions/runs/36001316470/artifacts/10808950479), expiring **2026-12-23**. Archive SHA-256: `82b8b9bce5d5b0094a2854711f0458fd01b2a3411bfadd084514bd5268d5f29a`. The deployed snapshots, inclusion reports, source lock and generator remain in Git after the raw artifact expires. Copy the public archive to the project's normal long-term evidence store before that date when future byte-identical regeneration is required.

No raw provider response or model list is fetched at application startup. No account credentials or private channel settings are included in the source archive or snapshots. Unknown production pricing is not newly encoded as zero. Existing estimates and compatibility placeholders remain explicit limitations rather than being silently turned into current quotes.
