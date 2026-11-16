# Chinese provider adaptor model audit — 2026-11-16

This review covers all 17 directories assigned to the Chinese-provider catalogue
review. Official model, pricing, API and service-notice pages were checked against
the existing production Go definitions. Changes are ordinary, independently owned
provider definitions; no external catalogue, runtime patching, model remapping,
account-entitlement filter or operator-override migration is introduced.

Prices below are per million tokens and in CNY unless explicitly marked USD.
`input / cache / output` denotes uncached input, cached input and output. An
unspecified cache or limit field is not a claim that the service is free.

## Implemented changes

### Alibaba: `ali` and `alibailian`

The two adaptors have independent definitions using **Beijing** tariffs. Region
and deployment differences remain operator configuration concerns.

| Exact new model ID | Input / cache / output | Metadata decision |
| --- | --- | --- |
| `qwen4-max` | 10 / 0.5 / 30 | 1,000,000 context; 262,144 combined thinking and answer tokens, or 131,072 answer tokens without thinking. |
| `MiniMax/MiniMax-M3.1-Flash` | 1 / 0.1 / 4 | Published 1M context; hosted output ceiling remains unspecified. |
| `kimi-k3.1-pro` | 13 / 1.3 / 68 | Alibaba deployment, 1,048,576 context/output; image input through the compatible adaptor. |
| `kimi/kimi-k3.1-pro` | 13 / 1.3 / 68 | Separate Moonshot-supplied route; no copied native-provider controls or limits. |
| `kimi/kimi-k3.1-pro-highspeed` | 26 / 1.3 / 136 | Cache price stays 1.3 while ordinary input/output doubles. |
| `glm-5.3-prime` | 16 / 4 / 56 | Beijing cache fraction is 25%; other regions differ. |
| `xiaomi/mimo-v2.6-pro-ultraspeed` | 30 / unspecified / 60 | Published price applies through 1M input; unknown output/cache limits remain unspecified. |
| `deepseek-v4.1-flash-1017` | 1 / 0.02 / 4 off peak; 2 / 0.04 / 8 peak | Daily 08:00–22:00 Asia/Shanghai peak, independent of DeepSeek's direct-provider schedule. |

The existing `MiniMax/MiniMax-M3` definition now applies 2.1 / 0.42 / 8.4 through
512,000 input tokens and 4.2 / 0.84 / 16.8 above that boundary. These are Alibaba's
published prices, not an import of a native MiniMax service-tier policy.

Sources: [model pricing](https://help.aliyun.com/zh/model-studio/model-pricing),
[cache policy](https://help.aliyun.com/zh/model-studio/context-cache),
[Qwen4-Max](https://help.aliyun.com/zh/model-studio/qwen4-max),
[Kimi K3.1 Pro](https://help.aliyun.com/zh/model-studio/kimi-k3-1-pro),
[DashScope API](https://help.aliyun.com/zh/model-studio/qwen-api-via-dashscope), and
[compatible Chat API](https://help.aliyun.com/zh/model-studio/qwen-api-via-openai-chat-completions).

The native Ali converter serializes message content as text, so its new entries
advertise text only. AliBailian preserves image/video parts for Qwen4-Max and
MiniMax Flash; Alibaba-deployed Kimi advertises text/image only. Neither path
currently forwards these models' adjustable reasoning effort. Qwen4's published
5m/1h write tariffs, 12.5/20, are documented in descriptions but not activated as
TTL-specific gateway billing: native cache blocks/write receipts and compatible
TTL normalization need separate qualification. Cached-input billing is retained.
The [Token Plan preview](https://help.aliyun.com/zh/model-studio/qwen4-max-preview)
has a different ID and no pay-as-you-go tariff, so no speculative preview price
is added. Existing historical IDs and their unrelated tiers/windows remain.

### MiniMax: `minimax`

Added released `MiniMax-M3.1-Flash`: 1 / 0.1 / 4; 1M context; maximum 524,288 output
tokens, with 131,072 recommended. Reasoning is always on, with
`low`, `medium`, `high`, `xhigh`, `max` and default `max`. Temperature/top-p are
fixed at 1/0.95; Flash does not support `service_tier`. Corrected MiniMax-M3's
maximum output to 524,288 and removed ineffective sampling controls from its
metadata. M3's permanent Standard discount, 512K tier and Priority caveat remain;
M2 access/retirement notices remain. This native adaptor is still catalogue-only,
which its model descriptions explicitly state.

Sources: [pricing](https://platform.minimax.cn/docs/pricing/overview),
[Chat API](https://platform.minimax.cn/docs/api-reference/text-chat-openai), and
[OpenAI compatibility](https://platform.minimax.cn/docs/api-reference/text-openai-api).

### BigModel and Z.AI: `zhipu`, `zai`

Added exact ID `glm-5.3-flashx-legal` to both providers. BigModel charges
2 / 0.57 / 7 CNY; Z.AI independently charges 0.37 / 0.075 / 1.25 USD. Both publish
1M context, 128K output, text/image/video/file input and always-on thinking with
`low`, `high`, `max` (default `max`). Existing v4 conversion preserves the exact
ID, image parts and effort values. No Flash launch discount, strict JSON-schema
claim, proprietary legal-tool integration or subscription entitlement is copied.

Sources: BigModel [shared Flash model page](https://docs.bigmodel.cn/cn/guide/models/vlm/glm-5.3-flash)
and [pricing](https://docs.bigmodel.cn/cn/guide/start/pricing); Z.AI
[shared Flash model page](https://docs.z.ai/guides/vlm/glm-5.3-flash) and
[pricing](https://docs.z.ai/guides/overview/pricing).

### Qianfan: `baiduv2`

Added published lowercase IDs `ernie-5.5-flash-preview`, `ernie-light-dt-1.0`,
`deepseek-v4.1-flash` and `deepseek-v4.1-flash-1017`. The existing lowercase
`ernie-5.1` is updated in place: the expensive bracket begins at 32,001 tokens,
consistent with the provider's 32K boundary and this catalogue's decimal-K
convention. Its advisory context is 128,000; output remains 65,536. The older
uppercase entries are preserved separately.

ERNIE 5.5 list prices are 2 / 0.2 / 8 during 08:00–22:00 Asia/Shanghai and
1 / 0.1 / 4 otherwise. A 50% promotion ends on **2026-11-30 at 23:59:59
Asia/Shanghai**. The pricing engine's exclusive `DateTo` is therefore
`2026-12-01`, equivalent to the transition at `2026-11-30T16:00:00Z`. The preview
publishes 1M context/output, with 64K default output. Light DT has flat
8 / 0.8 / 28, 1M context and advisory 128,000 output. Hosted Flash has 1M context,
393,216 output, and daily peak/off-peak prices of 2 / 0.04 / 8 and 1 / 0.02 / 4.
Its new entries do not inherit the expired September holiday discount.

Sources: [model IDs and limits](https://cloud.baidu.com/doc/qianfan/s/rmh4stp0j),
[prices](https://cloud.baidu.com/doc/qianfan/s/wmh4sv6ya), and
[promotion expiry and new-model announcement](https://cloud.baidu.com/product/qianfan_home/model_3.html).
Effort controls are not advertised where the shared compatible converter drops
them. Dedicated video protocols and per-second charges remain outside this text
catalogue refresh.

### Ark: `doubao`

Added `kimi-k3-1-pro-261024` at Ark's flat Standard 13 / 1.3 / 68 tariff, with
1,024,000 advisory context/output under the existing decimal-K convention.
Image/video input requires Base64; temperature is fixed at 1. The gateway does
not currently forward its effort setting. Cache storage, native Moonshot cache
writes, batch and Coding Plan multipliers remain separate.

Sources: [model list](https://docs.volcengine.com/docs/ark/model-list?lang=zh),
[prices](https://docs.volcengine.com/docs/ark/model-pricing?lang=zh&redirect=1),
[Chat API](https://docs.volcengine.com/docs/ark/chat-api?lang=zh&redirect=1),
[image inputs](https://docs.volcengine.com/docs/ark/image-understanding?lang=zh), and
[video inputs](https://docs.volcengine.com/docs/ark/video-understanding?lang=zh).

The October 17 Flash and Economy IDs are published, but their **completion-time**
tariff conflicts with the gateway's request-start pricing clock. The existing
exclusion for the September Flash snapshot remains. Standard off-peak Flash is
1 / 0.02 / 4; Economy is 0.8 / 0.016 / 3.2. Both double during weekdays
09:00–12:00 and 14:00–18:00 Asia/Shanghai. No incorrect request-start windows were
added. The [release notice](https://docs.volcengine.com/docs/ark/model-release-announcement?lang=zh)
also lists Seedream 5.5 and Seedance 2.5 Flash; these need their own image/video
protocol and unit qualification.

### Lingyi and AI360

`lingyiwanwu`: the [official service notice](https://platform.lingyiwanwu.com/)
announces public API closure at the end of 2026-09-03 and a refund application
window through the end of 2026-12-03. Both Yi model descriptions now identify
their historical status. IDs and historical prices remain configurable.

`ai360`: corrected the stale claim that no public tariffs exist. The new
[model catalogue](https://ai.360.com/open/zh/models) and
[API documentation](https://ai.360.com/docs/quick-start) are public. Router cards
show different supplier prices and changing aggregate summaries, while the
documented API is `api.360.cn` and the legacy channel defaults to `ai.360.cn`.
The [Qwen hybrid card](https://ai.360.com/open/zh/models/qwen-plus-360gpt-pro)
and [DeepSeek hybrid card](https://ai.360.com/open/zh/models/deepseek-v3-360gpt-pro)
do not establish one invariant routing tariff. Historical defaults are retained
as unverified; no guessed price or routing migration is introduced. The
[cache receipt documentation](https://ai.360.com/docs/cache) was also reviewed.

## Other provider dispositions

| Directory | Official evidence and decision |
| --- | --- |
| `deepseek` | [Current price/alias table](https://api-docs.deepseek.com/quick_start/pricing/) agrees with the existing Flash 1017 and Pro 0813 defaults, limits, cache prices and peak/holiday windows. Updated the tooling comment for the [November 13 built-in tools announcement](https://api-docs.deepseek.com/news/news261113/); no unsupported server-tool protocol or per-call policy is enabled. |
| `moonshot` | [Kimi pricing](https://platform.kimi.com/docs/pricing/chat) agrees with existing K3.1 Pro and Highspeed prices, including unchanged cached-input prices. Existing K3/K2 compatibility notices and native-cache qualifications remain. |
| `stepfun` | [Current price details](https://platform.stepfun.com/docs/zh/guides/pricing/details.md) retain Step-3.7 Flash and Step-5 Preview rates. Existing Step-5 grandfathered versus new-account cache pricing is preserved. Newly announced audio pricing effective November 23 uses native audio/character/duration meters outside this adaptor's qualified text transport. |
| `baidu` | The [Qianfan documentation](https://cloud.baidu.com/doc/qianfan/s/wmh4sv6ya) links the V1 shutdown/migration notice. This legacy OAuth/V1 adaptor retains historical entries; current lowercase V2 additions belong only in `baiduv2`. |
| `baichuan` | [Public prices](https://platform.baichuan-ai.com/prices) agree with existing M3 Plus 5/9, M3 10/30 and older model tariffs. Retain independent embedding, per-call search and historical daily-price handling. |
| `tencent` | [Hunyuan pricing and migration notice](https://cloud.tencent.com/document/product/1729/97731) distinguishes the legacy platform from TokenHub. Existing TC3-channel tariffs remain; new TokenHub IDs/authentication are not silently placed on the legacy route. |
| `xunfei` | [WebSocket API](https://www.xfyun.cn/doc/spark/Web.html) remains a separate transport. Preserve Lite's explicit free price and legacy identifiers/capabilities; current MaaS marketing names do not prove WebSocket compatibility. |
| `xunfeiv2` | [Legacy Spark HTTP API](https://www.xfyun.cn/doc/spark/X1http.html) and [MaaS documentation](https://maas.xfyun.cn/doc/) are distinct. New `spark-x2.6` uses `maas-api.cn-huabei-1.xf-yun.com/v2/chat/completions`; legacy `spark-x` is not an automatic alias. MaaS X2.5 retirement on November 24 does not establish retirement of the legacy X2 route. No incompatible IDs or promotional MaaS rates are copied. |

## Verification and remaining limits

Focused regressions check public model listing, exact request IDs and URLs,
compatible image/video preservation, independent BigModel/Z.AI currencies,
MiniMax's 512K boundary, Qianfan peak/off-peak transitions, and the final
nanosecond before promotion expiry. Existing tests retain historical IDs,
provider isolation, cache accounting and incompatible-protocol exclusions.

No live authenticated provider call is claimed. Unknown hosted limits remain
unknown; regional/negotiated tariffs require operator configuration. Explicit
cache-write receipts, server-side tools, Ark completion-time settlement and
new-provider transports require separate implementation and qualification.

Validation completed with the Go 1.27.4 toolchain:

```sh
go test ./relay/adaptor/{ali,alibailian,deepseek,moonshot,minimax,stepfun,zhipu,zai,doubao,baidu,baiduv2,baichuan,tencent,xunfei,xunfeiv2,lingyiwanwu,ai360}
git diff --check
```

Both commands passed. The 15 packages with tests passed; `lingyiwanwu` and
`ai360` compiled and reported no test files. Changed Go files pass `gofmt`.
This scoped run does not claim database/integration or live-provider qualification.
