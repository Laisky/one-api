# Adaptor model catalogue review — 2026-10-12

## Scope and interpretation

This ledger replaces the initial three-provider checkpoint. Every directory below
has an explicit disposition. **Directory coverage is not a claim that every
upstream product, historical price, account entitlement or live endpoint has been
implemented or certified.** Changes cover source-backed defaults representable by
the current gateway, plus narrowly required request-parameter fixes. Unpublished
prices, incompatible billing units and new native protocols remain explicit
exclusions rather than guessed/free defaults.

Each changed provider was checkpointed separately. Production Go definitions,
not a runtime imported catalogue, remain authoritative. The shared OpenAI bridge
selects several provider catalogues by channel type; native adaptor stubs are not
mistaken for the production compatible route. Administrator overrides, existing
credentials, database schema and deployment configuration are unchanged.

Disposition meanings: `updated` = this PR changes provider code; `shared` = uses
a catalogue updated elsewhere in this PR; `retained` = scoped comparison supports
keeping the relevant existing defaults; `configured` = account/installation/quote
specific; `limited` = public evidence does not justify a new default; `helper` =
not an independently priced provider. A retained row is not a blanket audit of all
old metadata or prices. Primary evidence and detailed exclusions are linked below.

## Complete directory inventory

| Surface | Disposition | Evidence and implementation decision |
| --- | --- | --- |
| `ai360` | limited | [Official platform](https://ai.360.com/platform/docs) did not yield a usable current rate table. Existing generic-channel records remain historical, not newly certified current/free tariffs. |
| `aiproxy` | configured | `relay/adaptor/aiproxy/adaptor.go` delegates pricing to OpenAI and uses configured library identifiers. No independent reseller quote or global tenant-library model list is inferred. |
| `ali` | updated | [Domestic pricing](https://help.aliyun.com/zh/model-studio/model-pricing); [audit](ali-model-audit-20261012.md). Correct OCR/MT native CNY prices and capabilities; preserve the native-versus-compatible routing boundary. |
| `alibailian` | updated | [Domestic pricing](https://help.aliyun.com/zh/model-studio/model-pricing); [audit](alibailian-model-audit-20261012.md). Add verified OCR IDs and correct OCR/MT metadata on the compatible image route, independently of native Ali. |
| `anthropic` | updated | [Effort](https://platform.claude.com/docs/en/build-with-claude/effort); [audit](anthropic-model-audit-20261012.md). Opus 5.5 effort levels/default and wire controls; current Haiku 5.5 was already present. No invented synchronous Batch-beta output limit. |
| `aws` | updated | [Bedrock pricing](https://aws.amazon.com/bedrock/pricing/); [audit](aws-model-audit-20261012.md). Sonnet 5.5 cache-read reduction and Opus effort metadata. Preserve native IDs and regional/inference-profile routing. |
| `azure` | updated | [Foundry Claude models](https://learn.microsoft.com/azure/foundry/foundry-models/concepts/claude-models); [audit](azure-model-audit-20261012.md). Haiku 5.5 native discovery with deployment routing and 100K pricing-boundary coverage. |
| `baichuan` | updated | [Medical API](https://platform.baichuan-ai.com/docs/medical), [prices](https://platform.baichuan-ai.com/prices); [audit](baichuan-model-audit-20261012.md). Four M-series output ceilings and sampling controls corrected; native CNY prices unchanged. Medical files/search billing are not implied. |
| `baidu` | updated | [v1 retirement](https://cloud.baidu.com/doc/qianfan/s/rmse4vxp8); [audit](baidu-model-audit-20261012.md). Mark retired v1 endpoint across 16 historical configurations without declaring every underlying v2 model retired. No automatic OAuth/protocol migration. |
| `baiduv2` | updated | [Model parameters](https://cloud.baidu.com/doc/qianfan/s/rmh4stp0j), [prices](https://cloud.baidu.com/doc/qianfan/s/wmh4sv6ya); [audit](baiduv2-model-audit-20261012.md). Seven exact lowercase chat IDs, independent host tariffs and Qwen prompt tiers; actual compatible request-budget preservation tested. |
| `cerebras` | retained | [Public models API](https://api.cerebras.ai/public/v1/models) still matches the two shared models' IDs, base prices and limits. Dedicated/legacy entries are not current enterprise quotes; mixed host precision is not flattened to an invented label. |
| `cloudflare` | updated | [Workers AI models](https://developers.cloudflare.com/workers-ai/models/); [audit](cloudflare-model-audit-20261012.md). Correct model-specific reasoning controls and limits, not a blanket provider-wide vocabulary. |
| `cohere` | updated | [Command A+](https://docs.cohere.com/docs/command-a-plus) and linked model cards; [audit](cohere-model-audit-20261012.md). Four explicitly free rate-limited API prices replace inherited paid estimates. Paid deployment contracts and newer native endpoints remain separate. |
| `common` | helper | Shared adaptor plumbing, not an independent upstream catalogue or tariff. Covered through consumer package and integration tests. |
| `copilot` | configured | [Supported models](https://docs.github.com/en/copilot/reference/ai-models/supported-models). Plan availability/premium-request multipliers are not USD-token tariffs. Preserve configured/dynamic discovery. |
| `coze` | configured | [Official pricing](https://www.coze.com/premium) is not a readable global bot-ID/token-price table. Bot/deployment contracts and administrator configuration remain authoritative. |
| `deepinfra` | updated | [Public model API](https://api.deepinfra.com/models/list); [audit](deepinfra-model-audit-20261012.md). Refresh eligible public chat IDs, host rates and independently owned metadata; exclude incompatible billing shapes. |
| `deepl` | configured | [Supported languages](https://developers.deepl.com/docs/getting-started/supported-languages). Catalogue aliases are translation targets, not new LLM IDs. Existing source-character metering is retained; subscription/region-specific contracts are not converted to guessed token prices. |
| `deepseek` | updated | [Native pricing](https://api-docs.deepseek.com/quick_start/pricing/); [audit](deepseek-model-audit-20261012.md). Exclude published national holidays from peak pricing while preserving the separate Flash transition and canceled Pro migration. |
| `doubao` | updated | [Ark IDs](https://www.volcengine.com/docs/82379/1330310), [Standard prices](https://www.volcengine.com/docs/82379/1544106); [audit](doubao-model-audit-20261012.md). Ten exact Standard non-audio chat IDs. Completion-time tariffs and audio-cache attribution are not misrepresented by start-time/text pricing. |
| `fireworks` | updated | [Serverless models](https://fireworks.ai/models?modelTypes=Serverless); [audit](fireworks-model-audit-20261012.md). Correct the host's V4.1 Flash Standard tariff without substituting native DeepSeek or dedicated-GPU prices. |
| `gemini` | shared | [Developer API changelog](https://ai.google.dev/gemini-api/docs/changelog); [audit](gemini-model-audit-20261012.md). Receives shared October lifecycle descriptions; native/shared initialization is tested. New named image-family protocol integration is excluded below. |
| `geminiOpenaiCompatible` | updated | [Developer API changelog](https://ai.google.dev/gemini-api/docs/changelog); [audit](gemini-model-audit-20261012.md). Correct three deprecation/redirect descriptions. Legacy 3.5 pricing is explicitly historical, not a certified redirected tariff. |
| `groq` | retained | [Groq models](https://console.groq.com/docs/models). The separately qualified October 12 update is already merged as #559 in this PR's base. Preserve its verified audio/model metadata rather than duplicating it. |
| `internal` | helper | Internal catalogue and transport utilities; not a provider tariff. Validate through their packages and consumers. |
| `jina` | retained | [Live model data](https://api.jina.ai/v1/models), [native schemas](https://api.jina.ai/openapi.json). Existing admitted hosted IDs and token prices are retained; vector width is not output-token length. Broader research/media entries do not automatically imply gateway endpoint support. |
| `lingyiwanwu` | limited | [Official platform](https://platform.lingyiwanwu.com/docs) did not expose usable current hosted prices in this review. Retain historical defaults without describing them as new confirmed tariffs. |
| `minimax` | updated | [Native prices](https://platform.minimax.cn/docs/guides/pricing-paygo); [audit](minimax-model-audit-20261012.md). Model-specific controls and PAYG lifecycle corrections; preserve Standard prices and distinguish subscription-only previews from PAYG. |
| `mistral` | updated | [Large 4](https://docs.mistral.ai/models/mistral-large-4-0), [prices](https://docs.mistral.ai/inference/pricing); [audit](mistral-large4-audit-20261012.md). Large 4 entries, bounded launch promotion, GLM 5.3 effort preservation and lifecycle updates. No guessed latest-alias rewrite. |
| `moonshot` | updated | [Kimi catalogue](https://platform.kimi.com/docs/models); [audit](moonshot-model-audit-20261012.md). Retired-model descriptions and verified K3 controls; preserve independent domestic currency and cache semantics. |
| `muapi` | configured | [Native API](https://muapi.ai/de/docs/api-reference). Request-normalized asynchronous video quotes, not a universal text-token list. Existing illustrative discovery is not an admission allowlist; no static price is fabricated. |
| `novita` | updated | [Live models API](https://api.novita.ai/v3/openai/models); [audit](novita-model-audit-20261012.md). Nine verified public chat additions with host rates and ownership tests; deployment/unsupported media rows remain excluded. |
| `nvidia` | updated | [GLM 5.3 card](https://build.nvidia.com/z-ai/glm-5-3/modelcard), [Flash card](https://build.nvidia.com/z-ai/glm-5-3-flash/modelcard); [audit](nvidia-model-audit-20261012.md). Two dotted native IDs, host precision and explicit free-trial prices; preserve effort only for the verified models. |
| `ollama` | updated | [Thinking](https://docs.ollama.com/capabilities/thinking), [Nomic tag](https://ollama.com/library/nomic-embed-text:latest); [audit](ollama-model-audit-20261012.md). Correct embedding precision/output and remove inferred effort ladders. Exact tag num_ctx=8192 overrides an ambiguous summary label; local symbolic prices stay unchanged. |
| `openai` | retained | [Model index](https://developers.openai.com/api/docs/models), [Astra](https://developers.openai.com/api/docs/models/gpt-6-astra), [Sol 6.1](https://developers.openai.com/api/docs/models/gpt-6.1-sol). Current ordinary IDs, Standard/cache/long-context rates and effort metadata already exist. GPT-Live's distinct session/metering protocol is not a plain catalogue addition. |
| `openai_compatible` | helper | Common compatible conversion support. Its consumers supply provider-specific model identifiers and tariffs; no independent price source exists. |
| `openrouter` | updated | [Public models API](https://openrouter.ai/api/v1/models); [audit](openrouter-model-audit-20261012.md). Refresh 75 verified chat definitions with host-specific prices and metadata, preserving documented exclusions and native pricing units. |
| `palm` | retained | [Official deprecation](https://ai.google.dev/palm_docs/deprecation). Already labeled retired compatibility record; do not silently remap to a differently priced Gemini model. |
| `proxy` | configured | `relay/adaptor/proxy/adaptor.go` intentionally has configured/empty discovery and no independent upstream tariff. Do not fill it with unrelated provider defaults. |
| `replicate` | configured | [Model collection](https://replicate.com/collections/language-models), [pricing](https://replicate.com/pricing). Version/deployment and hardware-time/media billing require their own contracts. No universal per-token conversion is invented. |
| `siliconflow` | updated | [China pricing](https://siliconflow.cn/pricing); [audit](siliconflow-model-audit-20261012.md). Add verified GLM 5.3 Flash with independent domestic tariff; retain China/international and tier boundaries. |
| `stepfun` | updated | [Official prices](https://platform.stepfun.com/docs/zh/guides/pricing/details.md); [audit](stepfun-model-audit-20261012.md). Document account-dependent Step 5 cache pricing without claiming an unpublished universal cache tariff. |
| `tencent` | updated | [Legacy retirement](https://cloud.tencent.com/announce/detail/2301), [Hy3 migration](https://cloud.tencent.com/announce/detail/2391); [audit](tencent-model-audit-20261012.md). Correct scoped retirement labels. TokenHub PAYG, Token Plan subscriptions and legacy TC3 transport remain distinct. |
| `togetherai` | updated | [Serverless models](https://docs.together.ai/docs/serverless-models); [audit](togetherai-model-audit-20261012.md). Host serverless prices and capability corrections; dedicated-instance quotes are not token defaults. |
| `typesafe` | retained | [Native models](https://docs.typesafe.ai/models) successfully rechecked: jev-1.13.0 and both aliases, USD 0.042/M input/free output, 64K combined/32K longest-question limits remain aligned. This is a real native evaluation provider, not a helper. Published account rate limits are dynamic, not catalogue admission rules. |
| `vertexai` | updated | [Google Cloud prices](https://cloud.google.com/gemini-enterprise-agent-platform/generative-ai/pricing); [audit](vertexai-model-audit-20261012.md). Independent Haiku 5.5 global Standard tiers and Sonnet cache price, native publisher routes and cloud-specific exclusions. |
| `xai` | retained | [Official models](https://docs.x.ai/developers/models). Existing September 21 catalogue assembly already contains Grok 4.7, documented aliases and dated image-pricing changes. No missing-model claim is made from constants.go alone. |
| `xunfei` | updated | [Native Spark](https://www.xfyun.cn/doc/spark/Web.html); [audit](xunfei-model-audit-20261012.md). Explicit free Lite tariff and model-specific WebSocket capabilities. Existing paid models and unknown-model fallback remain nonzero. |
| `xunfeiv2` | updated | [Spark HTTP](https://www.xfyun.cn/doc/spark/X1http.html); [audit](xunfeiv2-model-audit-20261012.md). Explicit free Lite and HTTP capability corrections. Endpoint-version names are not invented model IDs. |
| `zai` | retained | [Z.AI native USD prices](https://docs.z.ai/guides/overview/pricing). Existing current FlashX and GLM entries retain independent international tariffs. Do not inherit BigModel CNY or expired discounts. |
| `zhipu` | retained | [BigModel native prices](https://docs.bigmodel.cn/cn/guide/start/pricing). The recent separately completed model/unit update is already in the base; preserve domestic character/audio/search units and independent Z.AI derivation tests. |

## Important exclusions and deployment decisions

- **Mistral Large 4 is implemented**, superseding the earlier pending note. Its
  October 6–20 exclusive UTC promotional window follows the repository's
  date-only convention; the exact upstream expiry timezone is not established.
  Standard rates remain the base, so the discount cannot become permanent.
  Operators must reconcile that boundary with their contract/invoice.
- **Gemini Nano Banana 2.1 is not implemented by this PR.** Its named-family
  image routing, 4K output pricing, search fees and backend boundaries require
  a separate native integration. The redirected legacy Gemini 3.5 tariff also
  remains explicitly historical; select the current replacement ID and verify
  backend-specific billing rather than assuming old prices survive a redirect.
- **Cohere Embed 5 / Parse 5 / additional audio endpoints and GPT-Live** require
  separate transport/metering work. Instance-hour, page, character, session,
  request and generated-image prices must not become guessed token prices.
- **Ark DeepSeek V4.1** selects prices by completion time, not the common
  request-start window. Seed 2.1 Lite audio-cache attribution is not advertised
  as a working text-price fallback. TokenHub's native receipt-time schedule and
  native DeepSeek's holiday windows are different policies.
- **Legacy Tencent and Baidu v1** records are historical. New TokenHub or v2
  product availability does not justify silently changing their authentication,
  endpoints, retirement semantics or numerical historical tariffs.
- Advisory K/M token limits preserve each reviewed provider's documented or
  explicitly noted normalization. They are not new account limits (`MaxTokens`).
  Unknown cached rates are not described as verified zero-cost cache service.

## Qualification and reproducibility

Provider-specific public behavior tests are retained as regressions. Later
provider commits require named pre-fix assertion failures followed by complete
affected-package race suites before the isolated authoring runner publishes each
commit. Early Mistral/Cohere/Gemini checkpoints had package-suite evidence but
**did not execute pre-fix negative controls**; later results do not rewrite that
history. Counts in provider receipts include subtests unless explicitly separated.

The root paid-model smoke test now samples Spark-Pro instead of the verified
free Spark-Lite. A separate integration regression checks explicit zero-price
models against paid and unknown-model fallbacks; the positive-price invariant is
not removed. The October inventory test requires every adaptor directory exactly
once, while the historical September inventory remains unchanged.

The final qualification receipt records exact commands, source commit, tree,
pass/fail events and compiled catalogues. A full adaptor/package race suite is
not the full repository's database/integration qualification. No paid provider
calls, live account entitlement checks, production deployment or frontend build
are claimed. Required automatic CI must be checked against the actual PR head.
The product diff must contain no QA scripts or workflow changes.

## Release and rollback

Review changed default tariffs before rollout; channel-specific overrides remain
administrator-owned. Free NVIDIA and Cohere values apply only to their explicit
rate-limited public/trial offers, not separately paid enterprise deployments.
Review dated promotions and retained historical IDs against actual contracts.
No database migration or credential rotation is required. Revert the relevant
provider commit to undo its default/metadata change; revert the integration
checkpoint separately for audit-only tests/documents. This PR is not deployed or
merged as part of the research task.

## Final executed integration receipt

Provider source parent: `5632f14b3c5629969923e317ee250e0cd312e524`. Qualification run: https://github.com/Laisky/one-api/actions/runs/38315373143

The final integration changes only this ledger and two root regression tests.
The following commands passed on the same staged production source:

```sh
go test -p 2 -json -race -count=1 -timeout=8m ./relay/adaptor/... ./relay/pricing ./relay/format ./relay
go vet ./...
make test-quick
python3 .github/scripts/test_workflows.py
```

Passed top-level tests: 1613; total passing
named events including subtests: 5722; passing packages:
68. Full JSON results retain every skip and failure
field. Live-provider tests may skip without credentials; no live qualification is
inferred. The stale Spark-Lite positive-price assertion failed on the unchanged
parent before the integration correction. There are no product workflow changes.

### Checkpoint history

```text
3966bd75 fix(mistral): refresh hosted model lifecycle metadata
594c2800 fix(cohere): use verified free API tariffs for Command research models
ef0e340e test(adaptors): run affected model catalogue packages in PR checks
809a63ef test(adaptors): retain scoped catalogue audit inputs temporarily
bd76940f fix(gemini): document October model deprecations and upstream redirects
bfc5d028 test(adaptors): qualify shared and native Gemini catalogue initialization
6a28cb70 docs(adaptors): record package-test evidence and restore CI policy
bf9f50b5 feat(mistral): add Large 4 and preserve hosted GLM 5.3 reasoning
52381e23 fix(minimax): refresh effective M-series controls and retirement metadata
80079276 fix(moonshot): mark retired models and expose verified K3 controls
aa02321b test(adaptors): fix Kimi output-limit pointer and format catalogue changes
0b70a6ed feat(siliconflow): add verified GLM 5.3 Flash token pricing
121d83ec fix(stepfun): document account-dependent Step 5 cache pricing
faa204ec fix(openrouter): refresh 75 verified chat model definitions
d7c7d09a feat(novita): add nine verified public chat model defaults
25e8632f fix(deepinfra): refresh public chat models and host token prices
f9c0b51a fix(togetherai): refresh serverless prices and capability metadata
0c3f2fba fix(deepseek): exclude published national holidays from peak pricing
da838ee2 fix(ali): correct verified OCR and translation model metadata
3ad33d42 fix(alibailian): add verified OCR models and correct native CNY tariffs
86ddc6ac fix(cloudflare): correct model-specific reasoning controls and limits
9732ce65 fix(fireworks): correct V4.1 Flash Standard serverless tariff
05af5bb2 fix(xunfei): correct free Lite tariff and WebSocket capability metadata
85f0edb9 fix(xunfeiv2): correct free Lite tariff and HTTP capability metadata
a088bc89 fix(anthropic): expose verified Opus 5.5 effort levels and default
be44431e fix(aws): apply verified Sonnet cache price and Opus effort metadata
9e80c43f feat(azure): expose verified Haiku 5.5 Foundry model
0d980e1e feat(vertexai): add verified Haiku 5.5 prices and refresh Sonnet cache rate
53702fe8 fix(baidu): identify retired v1 endpoint in retained model metadata
ecba1ab8 feat(baiduv2): add seven verified chat models with host pricing and tiers
3e32f88f fix(ollama): correct embedding precision and model-specific thinking metadata
684ffca4 feat(nvidia): add GLM 5.3 endpoints and preserve verified effort controls
2f865f19 feat(doubao): add ten verified Ark Standard model configurations
0fd016ac fix(tencent): distinguish retired legacy models from TokenHub migrations
5632f14b fix(baichuan): correct documented M-series output and sampling limits
```
