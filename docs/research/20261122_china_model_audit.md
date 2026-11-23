# Chinese provider adaptor model audit — 2026-11-22

This follow-up checks all 17 Chinese-provider adaptor directories against official
model, tariff, API and service-notice pages. It builds on the
[November 16 audit](20261116_china_model_audit.md), including subsequent fixes
already present in the working baseline. Two new exact IDs add three provider
catalogue entries. A DeepSeek billing-policy comment is corrected. Historical
IDs, operator overrides and provider-specific transport distinctions are retained.

Unless marked USD, prices below are **CNY per million tokens**. The order
`input / cache / output` means uncached input, cached input and generated output.
No unspecified price or limit is treated as zero.

## Implemented catalogue changes

| Exact model ID | Adaptor | Input / cache / output | Context / maximum output |
| --- | --- | --- | --- |
| `kimi-k3.1-pro-prime` | `ali` | 26 / 1.3 / 136 | 1,048,576 / 1,048,576 tokens |
| `kimi-k3.1-pro-prime` | `alibailian` | 26 / 1.3 / 136 | 1,048,576 / 1,048,576 tokens |
| `antdigital/Ling-3.1-flash` | `alibailian` only | 0.5 / 0.1 / 1.5 during the currently advertised promotion | 1,048,576 / 262,144 tokens |

### Alibaba-deployed Kimi Prime

Alibaba's [release history](https://help.aliyun.com/zh/model-studio/newly-released-models)
announces Prime on **November 19**, with the same capabilities and limits as
Pro and faster output. The [Beijing tariff table](https://help.aliyun.com/zh/model-studio/model-pricing)
prices ordinary input/output at twice Pro's rate while keeping cached input at
1.3. The separate Singapore prices are not imported into these Beijing defaults.
The [Pro model page](https://help.aliyun.com/zh/model-studio/kimi-k3-1-pro)
supplies the shared limits and text/image capability.

Both entries belong in the existing Kimi family files and own their metadata
independently. The native Ali converter forwards text, so its discovery metadata
remains text-only. AliBailian preserves text/image inputs. Neither entry advertises
video or adjustable reasoning effort. The Alibaba deployment ID is distinct from
the already configured supplier route `kimi/kimi-k3.1-pro-highspeed`; no alias
replacement or silent route change occurs.

### AntDigital Ling through AliBailian

The [Ling model card](https://help.aliyun.com/zh/model-studio/ling-3-1-flash-by-antdigital)
publishes text input/output, hybrid thinking, the limits above, and list tariffs
of 2 / 0.4 / 6. The current pricing table advertises input/output at **25% of list
price**. The [cache policy](https://help.aliyun.com/zh/model-studio/context-cache)
charges Ling cache hits at **20% of the input price**: current input 0.5 therefore
gives cached input 0.1, with output 1.5. This is an application of the published
discount and cache fraction, not an independently invented promotional cache
rate. No expiry is published, so no speculative date window is created. A later
tariff change will need a new catalogue update.

The [provider-specific API guide](https://help.aliyun.com/en/model-studio/antdigital)
documents the workspace-specific Beijing compatible endpoint. Operators must set
the existing channel BaseURL to
`https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com`; the adaptor appends
`/compatible-mode/v1/chat/completions`. The description records this requirement.
Native DashScope support is not documented, so the native Ali catalogue receives
no Ling entry. A stable `models_ling.go` family is assembled in `models.go`.

The guide supports function tools and JSON object output but excludes strict
JSON Schema, `reasoning_effort`, parallel tool calls and web search. The new
metadata reflects those limits. Thinking is enabled upstream by default; this
change does not introduce a new thinking-control conversion or endpoint.

### DeepSeek tool-fee clarification

The [current pricing page](https://api-docs.deepseek.com/quick_start/pricing/)
states that model-triggered Search/Fetch no longer incurs a separate tool fee
from **2026-11-18T04:00:00Z**; returned context remains token-billed. Standalone
Search/Fetch remains **USD 0.75 per 1,000 successful requests**. The comment on
the empty built-in-tool policy is updated accordingly. Unsupported server tools
and standalone endpoints are not enabled, and existing function-tool accounting
is unchanged. Flash/Pro token rates, peak windows and aliases remain unchanged.

## Complete provider dispositions

| Directory | Evidence and disposition |
| --- | --- |
| `ali` | Added Beijing Kimi Prime as above. The [release history](https://help.aliyun.com/zh/model-studio/newly-released-models) also includes newer image/video deployments, which require separate protocol and unit qualification. Qwen4-Max was already present. The [Qwen4 preview](https://help.aliyun.com/en/model-studio/qwen4-max-preview) is Token Plan-only and has no pay-as-you-go tariff to add. |
| `alibailian` | Added independent Kimi Prime and compatible-only Ling definitions. Existing region defaults, M3 tiers and provider-specific cache prices are preserved. |
| `baidu` | The official [V1 shutdown/migration notice](https://cloud.baidu.com/doc/qianfan/s/rmse4vxp8) concerns the legacy interface. Retain historical OAuth/V1 entries; current lowercase model IDs belong to `baiduv2`. No implicit migration is introduced. |
| `baiduv2` | The [model list](https://cloud.baidu.com/doc/qianfan/s/rmh4stp0j) and [tariffs](https://cloud.baidu.com/doc/qianfan/s/wmh4sv6ya) confirm existing ERNIE 5.5 Flash Preview, Light DT and Flash 1017 entries. ERNIE 5.5 list peak/off-peak prices remain 2/0.2/8 and 1/0.1/4, with the existing half-price promotion through November 30 in Shanghai. Older DeepSeek promotion labels with expired dates do not establish replacement tariffs; no guessed historical price is added. |
| `baichuan` | [Public prices](https://platform.baichuan-ai.com/prices) retain M3 Plus input/output 5/9 and M3 10/30. Independent embedding and per-call medical-search billing remain separate. No catalogue change. |
| `deepseek` | Corrected the tool-fee comment. [Published aliases and tariffs](https://api-docs.deepseek.com/quick_start/pricing/) retain Flash 1017 at USD 0.15/0.003/0.60 and Pro 0813 at 0.66/0.022/1.98 off peak, doubled during the documented peak windows. No catalogue-rate change. |
| `doubao` | [Ark prices](https://docs.volcengine.com/docs/ark/model-pricing?lang=zh&redirect=1) retain Seed Evolving 6/1.2/30 and hosted Kimi Pro 13/1.3/68. The [release history](https://docs.volcengine.com/docs/ark/model-release-announcement?lang=zh) updates the rolling Seed Evolving deployment without a new model ID. Flash 1017 settlement uses request completion time, incompatible with the current request-start price clock; its existing exclusion remains. |
| `lingyiwanwu` | The [official site](https://platform.lingyiwanwu.com/) did not return readable notice text in this review. Preserve the existing historical-service descriptions and prices from the earlier audit; do not claim a fresh independent verification of the closure/refund dates or invent a replacement tariff. |
| `minimax` | [Current pricing](https://platform.minimax.cn/docs/pricing/overview) and [model documentation](https://platform.minimax.cn/docs/guides/models-intro) retain M3.1 Flash 1/0.1/4 and M3 Standard 2.1/0.42/8.4 through 512K, doubled above 512K. The current list omits M2; historical IDs and existing retirement notices remain. H3 image/video needs dedicated protocol/unit work. This native adaptor remains catalogue-only. |
| `moonshot` | The [official platform](https://platform.kimi.com/?from=footer_nav) retains K3.1 Pro 13/1.3/68 and Highspeed 26/1.3/136. Native cache creation is 13 and 26 respectively; supplier-route cache-write prices are not imported. [K3.1 documentation](https://platform.kimi.com/docs/guide/kimi-k3-1-pro-quickstart) and [cache guidance](https://platform.kimi.com/docs/guide/context-caching) leave existing limits, efforts and cache qualifications unchanged. |
| `stepfun` | [Pricing details](https://platform.stepfun.com/docs/zh/guides/pricing/details.md) retain Step-5 Preview 7/0.7/20 and Step-3.7 Flash 1.35/0.27/8.1. Existing pre-October-14 cache grandfathering is retained. Announced November 23 audio rates are not yet effective and require native audio/character/duration meters. |
| `tencent` | [Hunyuan pricing and migration guidance](https://cloud.tencent.com/document/product/1729/97731) distinguish the legacy platform from TokenHub. Existing TC3-channel rates remain. No TokenHub model, authentication or endpoint is silently substituted. |
| `xunfei` | The [WebSocket API](https://www.xfyun.cn/doc/spark/Web.html) is a distinct transport. Preserve its legacy IDs, capabilities and explicit free Lite rate; MaaS marketing names do not prove compatibility. |
| `xunfeiv2` | The [legacy HTTP API](https://www.xfyun.cn/doc/spark/X1http.html) and [MaaS API](https://maas.xfyun.cn/doc/) use different hosts/IDs. The newer MaaS `spark-x2.6` is not an alias of legacy `spark-x`. MaaS X2.5 retirement on November 24 does not establish retirement of the legacy X2 route. No incompatible ID or promotional rate is added. |
| `zhipu` | [BigModel pricing](https://docs.bigmodel.cn/cn/guide/start/pricing) retains GLM-5.3 at 8/2/28, Flash at 0.8/0.23/2.8 and FlashX/Legal at 2/0.57/7. Existing provider-specific tiers and transport qualifications remain. |
| `zai` | [Z.AI pricing](https://docs.z.ai/guides/overview/pricing) independently retains USD 1.4/0.26/4.4 for GLM-5.3, 0.15/0.03/0.5 for Flash and 0.37/0.075/1.25 for FlashX/Legal. No BigModel CNY tariff or promotion is copied. |
| `ai360` | The [router catalogue](https://ai.360.com/open/zh/models) shows supplier-specific pricing. The [Chat API](https://ai.360.com/docs/413291990e0) uses `api.360.cn`, while the legacy channel uses `ai.360.cn`. Preserve explicitly unverified historical defaults; current router cards do not establish one fixed legacy 360GPT tariff or authorize a routing migration. |

## Verification and limits

Extended public catalogue/pricing tests cover Prime in both Alibaba adaptors,
unchanged cache pricing, exact request IDs, image preservation and independent
metadata ownership. New Ling tests cover discovery, promotional/cache rates,
limits, compatible JSON object requests, workspace-specific URL construction and
exclusion from native Ali. They do not contact a provider.

Changed Go files were formatted with the Go 1.27.4 toolchain, and the scoped
`git diff --check` passed. The following focused test command passed:

```sh
go test ./relay/adaptor/alibailian -run 'TestNovemberAlibaba|TestLing' -count=1
```

The main refresh report records integrated validation. No live authenticated
provider call, account entitlement or negotiated price is certified. Ling's
unpublished promotion end, unqualified native media/server-tool protocols, old
provider notices that no longer render, and Ark completion-time settlement remain
explicit limitations.
