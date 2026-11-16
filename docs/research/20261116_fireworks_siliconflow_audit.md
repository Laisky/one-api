# Fireworks and SiliconFlow review — 2026-11-16

## Fireworks

Add these exact published Standard serverless resource names. Prices below are
USD per million tokens in input/cached-input/output order. Retain legacy IDs,
existing rate windows and operator overrides.

| API ID suffix under `accounts/fireworks/models/` | Input / cache / output | Context | Official model card |
| --- | --- | --- | --- |
| `kimi-k3p1-pro` | 2.00 / 0.20 / 10.00 | 1,048,576 | [Kimi K3.1 Pro](https://fireworks.ai/models/fireworks/kimi-k3p1-pro) |
| `minimax-m3p1-flash` | 0.15 / 0.015 / 0.60 | 1,048,576 | [MiniMax M3.1 Flash](https://fireworks.ai/models/fireworks/minimax-m3p1-flash) |
| `deepseek-v4p1-flash-1017` | 0.30 / 0.006 / 1.20 | 1,048,576 | [DeepSeek V4.1 Flash 1017](https://fireworks.ai/models/deepseek-ai/deepseek-v4p1-flash-1017) |
| `muse-spark-open` | 1.25 / 0.15 / 4.25 | 1,048,576 | [Muse Spark Open](https://fireworks.ai/models/fireworks/muse-spark-open) |
| `ultramarine-1-flash-us` | 0.15 / 0.03 / 0.50 | Card rounds to approximately 1M; exact ceiling left unspecified | [Ultramarine 1 Flash](https://fireworks.ai/models/fireworks/ultramarine-1-flash-us) |

The [Standard pricing table](https://docs.fireworks.ai/serverless/pricing) and
[model library](https://fireworks.ai/models?featured=true&serverless=true) were
cross-checked against the cards. MiniMax M3.1 Flash's own card supplies its current
tariff. Priority, Fast, Ultra-fast, regional variants and private deployment
pricing are separate from these defaults. Muse Spark Open's US-only capacity
explicitly waives the usual regional premium.

An earlier search-index copy of the Ultramarine card showed
$0.225/$0.045/$0.75 and US capacity. On November 16, the freshly opened card and
direct public HTML both show $0.15/$0.03/$0.50 and global capacity, agreeing with
the central table. The exact `-us` API ID is retained. Its card explicitly says
function calling is unsupported, so the entry does not inherit tools, reasoning
effort levels or precision from GLM 5.3 Flash. This is a current observation,
not a claim about when the provider changed the tariff or deployment region.

All five accept text and images and emit text. The first four document tools;
the available cards do not establish an exact hosted output ceiling or complete
effort vocabulary, so those values are left unspecified. Catalog assembly now
uses the shared duplicate-rejecting `JoinModelCatalogs` instead of silently
overwriting duplicate family keys. The existing `qwen3p8-2p4t-a95b` API name
remains; the pricing-page link slug `qwen3p8-max` is not added as an API alias.

Fireworks Rune remains a typed System One endpoint and is not added to this
chat catalog. Native per-model image workflow APIs also remain outside the
adaptor's current generic image contract.

## SiliconFlow

The [release notes](https://docs.siliconflow.cn/docs/release-notes/overview) and
[provider announcement](https://www.siliconflow.cn/news/yj02gsluu233nit05mtf8m9m)
confirm `OpenMOSS-Team/MOSS-Image`, launched November 3 at **CNY 0.30 per image**.
The existing [JSON image API](https://docs.siliconflow.cn/docs/api/images-generations-post)
accepts the canonical model name. Register an image tariff using the repository's
fixed 7 CNY/USD convention, with the existing gateway contract of one output per
call. No new image-editing path, model-specific default size, or token-priced
chat behavior is implied.

The same release notes announce November 10/16 retirements. Annotate the six
matching existing catalog entries while preserving their rates and discovery:

- November 10: `Pro/zai-org/GLM-5.1`.
- November 16: `tencent/Hunyuan-A13B-Instruct`, `inclusionAI/Ling-mini-2.0`,
  `stepfun-ai/Step-3.5-Flash`, `zai-org/GLM-4.5V`, and
  `Qwen/Qwen3.5-122B-A10B`.

The other announced retired IDs were absent from this compiled catalog, so no
new historical definitions or prices are invented. The October 21 DeepSeek
upgrade explicitly retains `deepseek-ai/DeepSeek-V4.1-Flash`; there is no separate
`-1017` API ID. Neither version is added without a verified SiliconFlow tariff.
MiniMax H3 video and TypeSafe/System One releases require different native
contracts and are not represented as chat models. Dynamic upstream rate limits
do not become fixed gateway model limits.

## Validation and limits

The following passed using Go 1.27.4:

```text
go test ./relay/adaptor/fireworks ./relay/adaptor/siliconflow
```

Focused tests exercise absolute input/cache/output prices, canonical IDs,
Chat/Responses/Claude native passthrough, the Ultramarine tool restriction,
MOSS's CNY conversion and actual request/response adapter helpers, rejection of
multiple image outputs, and retention of the announced retired IDs. The tests
do not call authenticated provider inference. Full repository qualification is
reported separately by the coordinating PR review.
