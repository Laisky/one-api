# Aggregator model audit — 2026-11-16

This audit covers OpenRouter, DeepInfra, Novita and Together AI. Each provider was
compared with its own public catalog and pricing representation on November 16,
2026. No authenticated generation or account entitlement was used to determine
discovery. The resulting definitions are ordinary Go source maintained in place;
there is no runtime catalog import, generated data dependency or patch overlay.

## Catalog reconciliation

| Provider | Before | After | Added IDs | Updated existing definitions | Official source |
| --- | ---: | ---: | ---: | ---: | --- |
| OpenRouter | 498 | 541 | 43 | 198 | [Catalog](https://openrouter.ai/api/v1/models) |
| DeepInfra | 200 | 205 | 5 | 67 | [Catalog](https://api.deepinfra.com/models/list) |
| Novita | 148 | 158 | 10 | 4 | [Catalog](https://api.novita.ai/v3/openai/models) |
| Together AI | 89 | 98 | 9 | 9 | [Catalog](https://docs.together.ai/docs/serverless/models) |

All historical IDs are retained. A model missing from the current public listing
is not automatically retired, remapped, or made free. Counts include capability,
limit, quantization, cache and description changes, not only base-price changes.
Existing operator configuration and pricing precedence are untouched.

### Evidence capture times

Public catalog snapshots were captured on 2026-11-16 at 15:50:42 UTC
(OpenRouter), 15:50:53 UTC (DeepInfra and Novita), and 15:53:03 UTC (Together).
An independent OpenRouter recheck at 16:02:58 UTC changed two quotes, which are
included in the final definitions and tests: `tencent/hy4-flash` now uses
$0.04375/$0.001875/$0.175625 input/cache/output, and `qwen/qwen3.5-9b` uses
$0.115/$0.104/$0.169. Other comparisons agreed. These are captured public
routing defaults, not a guarantee that a live aggregator quote cannot change.

## Units and interpretation

- OpenRouter quotes USD per token. Multiply by 1,000,000 before converting to the
  gateway token ratio. `pricing.overrides.min_prompt_tokens` is an inclusive
  threshold. UTC day/time overrides remain provider-specific; absent overrides
  remove a stale current default, without inventing a retrospective cutover date.
- DeepInfra quotes cents per token. Multiply by 10,000 for USD per million, then
  apply `1 - discount` exactly once. `rate_per_input_token_cached` is a multiplier
  of the discounted input price, not a separate cents amount. Published multiplier
  precision is retained (for example Qwen4 cache is $0.0710000001/M). A missing
  output ceiling stays unspecified; it is not copied from the creator model.
- Novita uses `pricing.*.price_per_m_decimal` as its USD-per-million quote.
  Nested decimal values already include the displayed price; integer fields or
  discount labels are not applied a second time. Null pricing does not mean free.
- Together uses the Standard serverless table. The US-only endpoint surcharge
  and dedicated deployment prices are separate contracts. NVFP4/MXFP4 map to
  the existing `fp4` metadata vocabulary; exact checkpoint capabilities are not
  inferred from quantization. An unpriced cached-input field stays unspecified.

`CachedInputRatio = -1` represents an explicitly free cache bucket; zero means
unspecified. Zero input/output prices are admitted only when published explicitly.
Context/output metadata is advisory and does not introduce a new request cap.
Supported parameter vocabularies come from that host; account/region limitations
remain upstream decisions. Partial tags on DeepInfra supplement established
positive capabilities rather than deleting fields that its listing omits.

## New synchronous text-output models

All monetary columns below are USD per million tokens. An em dash means the
provider did not publish a distinct cached-input price. These are base/default
quotes; the tier and clock exceptions described below still apply.

### OpenRouter

| Exact API ID | Input | Cache | Output | Context | Max output |
| --- | ---: | ---: | ---: | ---: | ---: |
| `anthropic/claude-fable-5.2` | 8 | 0.2 | 40 | 1,000,000 | 128,000 |
| `baidu/ernie-5.5-flash-preview:free` | 0 | — | 0 | 1,024,000 | 921,600 |
| `baidu/ernie-light-dt-1.0-intl` | 1.13 | 0.113 | 3.94 | 1,024,000 | 131,072 |
| `deepseek/deepseek-v4.1-flash-1017` | 0.15 | 0.003 | 0.6 | 1,048,576 | 393,216 |
| `elorian/glimpse` | 0.6 | 0.06 | 3 | 262,144 | 256,000 |
| `google/gemini-4-argon` | 2 | 0.1 | 10 | 1,048,576 | 943,718 |
| `google/gemini-4-carbon` | 2 | 0.1 | 10 | 1,048,576 | 943,718 |
| `google/gemini-4-hydrogen` | 1.5 | 0.075 | 7.5 | 1,048,576 | 943,718 |
| `inclusionai/ling-3.0-tiny` | 0.015 | 0.003 | 0.045 | 262,144 | 250,000 |
| `inclusionai/ling-3.2-flash:free` | 0 | — | 0 | 1,048,576 | 262,144 |
| `kt/midm-k-3.0-pro` | 0.2 | 0.04 | 0.8 | 261,795 | 235,615 |
| `liquid/lfm-2.5-24b-a2b` | 0.05 | 0.025 | 0.7 | 128,000 | 8,192 |
| `liquid/lfm-3-1.7b` | 0.08 | 0.02 | 0.2 | 262,144 | 8,192 |
| `liquid/lfm-3-1.7b:free` | 0 | — | 0 | 262,144 | 8,192 |
| `meta/muse-ember` | 0.1 | 0.004 | 0.5 | 1,048,576 | 943,718 |
| `meta/muse-spark-open` | 1.25 | 0.15 | 4.25 | 1,048,576 | 943,718 |
| `minimax/minimax-m3.1-flash` | 0.075 | 0.0075 | 0.3 | 1,048,576 | 524,288 |
| `moondream/moondream3-preview` | 0.3 | — | 2.5 | 32,000 | 4,096 |
| `moondream/moondream3.1-9b-a2b` | 0.3 | — | 2.5 | 32,000 | 4,096 |
| `moonshotai/kimi-k3.1-pro` | 0.2403 | 0.2 | 8.4023 | 1,048,576 | 943,718 |
| `motif-technologies/motif-3` | 0.3 | 0.06 | 1.2 | 262,144 | 235,929 |
| `nex-agi/nex-n2.6` | 0.12 | 0.024 | 0.4 | 262,144 | 235,929 |
| `nvidia/nemotron-3.5-super-vl` | 0.15 | 0.02 | 0.5 | 1,048,576 | 235,929 |
| `nvidia/nemotron-3.5-super-vl:free` | 0 | — | 0 | 1,000,000 | 131,072 |
| `openai/gpt-6.1-terra` | 1 | 0.05 | 5 | 1,050,000 | 128,000 |
| `openai/gpt-6.1-terra-pro` | 1 | 0.05 | 5 | 1,050,000 | 128,000 |
| `particle-ai/particle-1` | 0.06 | 0.003 | 0.5 | 1,048,576 | 131,072 |
| `particle-ai/particle-1:free` | 0 | — | 0 | 1,048,576 | 131,072 |
| `prism-ml/ternary-bonsai-2-9b` | 0.08 | 0.04 | 0.13 | 262,144 | 65,536 |
| `qwen/qwen4-max` | 1.5 | 0.075 | 4.5 | 1,000,000 | 131,072 |
| `recursive/flint-v1.0:free` | 0 | — | 0 | 262,144 | 235,929 |
| `reflection/beam-1.0-501b-a23b` | 0.3 | 0.04 | 1 | 1,048,576 | 943,718 |
| `ricursiveai/ripple-65b-preview` | 0.1 | 0.025 | 0.3 | 1,048,576 | 131,072 |
| `scaledown/extract` | 0.04 | — | 0 | 1,000,000 | 900,000 |
| `scaledown/summarize` | 0.04 | — | 0 | 1,000,000 | 4,000 |
| `tencent/hy4` | 0.417 | 0.021 | 1.2505 | 1,048,576 | 65,536 |
| `tencent/hy4-flash` | 0.04375 | 0.001875 | 0.175625 | 1,048,576 | 65,536 |
| `thinkingmachines/inkling-2-small` | 0.3 | 0.03 | 1 | 1,048,576 | 943,718 |
| `thinkingmachines/inkling-2-small:free` | 0 | — | 0 | 1,048,576 | 131,072 |
| `unbiased/pareto-26.10` | 0.8 | 0.03 | 3.2 | 1,048,576 | 131,072 |
| `unbiased/pareto-26.9` | 2.5 | 0.25 | 7.5 | 262,144 | 131,072 |
| `z-ai/glm-5.3-flashx-legal` | 0.37 | 0.075 | 1.25 | 1,048,576 | 131,072 |
| `~unbiased/pareto-latest` | 0.8 | 0.03 | 3.2 | 1,048,576 | 131,072 |

### DeepInfra

| Exact API ID | Input | Cache | Output | Context | Max output |
| --- | ---: | ---: | ---: | ---: | ---: |
| `Qwen/Qwen4-Max` | 1.413 | 0.0710000001 | 4.24 | 1,000,000 | unspecified |
| `deepseek-ai/DeepSeek-V4.1-Flash-1017` | 0.098 | 0.00294 | 0.294 | 1,048,576 | unspecified |
| `moonshotai/Kimi-K3.1-Pro` | 1.9 | 0.19 | 9.5 | 1,048,576 | unspecified |
| `nvidia/NVIDIA-Nemotron-3.5-Super-VL` | 0.15 | 0.0199999995 | 0.5 | 262,144 | unspecified |
| `thinkingmachines/Inkling-2-Small` | 0.285 | 0.0285 | 0.95 | 1,048,576 | unspecified |

### Novita

| Exact API ID | Input | Cache | Output | Context | Max output |
| --- | ---: | ---: | ---: | ---: | ---: |
| `deepseek/deepseek-v4.1-flash-1017` | 0.3 | 0.006 | 1.2 | 1,048,576 | 393,216 |
| `inclusionai/ling-3.1-flash` | 0.3 | 0.06 | 0.9 | 1,000,000 | 262,144 |
| `minimax/minimax-m3.1-flash` | 0.15 | 0.015 | 0.6 | 1,000,000 | 524,288 |
| `moonshotai/kimi-k3.1-pro` | 2 | 0.2 | 10 | 1,048,576 | 1,048,576 |
| `moonshotai/kimi-k3.1-pro-highspeed` | 4 | 0.2 | 20 | 1,048,576 | 1,048,576 |
| `qwen/qwen4-max` | 1.413 | 0.071 | 4.24 | 1,000,000 | 131,000 |
| `stepfun/step-5-preview` | 1 | 0.1 | 2.7 | 1,000,000 | 64,000 |
| `tencent/hy4` | 0.417 | 0.021 | 1.2505 | 1,000,000 | 64,000 |
| `tencent/hy4-flash` | 0.07 | 0.003 | 0.281 | 1,000,000 | 64,000 |
| `zai-org/glm-5.3-flashX` | 0.37 | 0.075 | 1.25 | 1,048,576 | 131,072 |

### Together AI

| Exact API ID | Input | Cache | Output | Context | Max output |
| --- | ---: | ---: | ---: | ---: | ---: |
| `MiniMaxAI/MiniMax-M3.1-Flash` | 0.15 | 0.015 | 0.6 | 1,048,576 | unspecified |
| `deepseek-ai/DeepSeek-V4.1-Flash-1017` | 0.3 | 0.006 | 1.2 | 1,000,000 | unspecified |
| `elorian-labs/glimpse-1.0` | 0.6 | 0.06 | 3 | 262,144 | unspecified |
| `meta-models/Muse-Spark-Open-816B-A42B` | 1.25 | 0.15 | 4.25 | 1,048,576 | unspecified |
| `moonshotai/Kimi-K3.1-Pro` | 2 | 0.2 | 10 | 1,048,576 | unspecified |
| `nvidia/NVIDIA-Nemotron-3.5-Super-VL` | 0.15 | 0.02 | 0.5 | 262,144 | unspecified |
| `reflection/Beam-1.0-501B-A23B` | 0.4 | 0.04 | 1.2 | 1,048,576 | unspecified |
| `ricursive-ai/Ripple-65B-Preview` | 0.2 | — | 0.6 | 131,072 | unspecified |
| `thinkingmachines/Inkling-2-Small` | 0.3 | 0.03 | 1 | 1,048,576 | unspecified |

## Current price changes for existing IDs

The following rows changed at least one base input/cache/output quote. Their
current provider source is linked above. Source-specific cache writes and tiers
are discussed separately; the historical audit remains unchanged.

| Provider / API ID | Previous input / cache / output | Current input / cache / output |
| --- | --- | --- |
| OpenRouter: `anthracite-org/magnum-v4-72b` | 2.5 / — / 5 | 2.5 / 2.5 / 5 |
| OpenRouter: `deepseek/deepseek-chat` | 0.32 / — / 0.89 | 0.2574 / — / 1.0287 |
| OpenRouter: `deepseek/deepseek-v3.2` | 0.259 / 0.14 / 0.8 | 0.26 / 0.13 / 0.38 |
| OpenRouter: `deepseek/deepseek-v4-flash` | 0.03 / 0.03 / 1.28 | 0.09 / 0.018 / 0.18 |
| OpenRouter: `deepseek/deepseek-v4-flash-0731` | 0.016 / 0.016 / 0.64 | 0.06 / 0.015 / 0.18 |
| OpenRouter: `deepseek/deepseek-v4-flash-vision-exp` | 0.2156 / 0.00686 / 0.6468 | 0.44 / 0.014 / 1.32 |
| OpenRouter: `deepseek/deepseek-v4-pro` | 0.41238 / 0.034365 / 0.82476 | 0.9 / 0.25 / 1.5 |
| OpenRouter: `deepseek/deepseek-v4-pro-0813` | 0.66 / 0.022 / 1.98 | 0.111 / 0.097 / 4.44 |
| OpenRouter: `deepseek/deepseek-v4.1-flash` | 0.15 / 0.003 / 0.6 | 0.14 / 0.0042 / 0.42 |
| OpenRouter: `google/gemini-3.8-flash` | 0.75 / 0.075 / 3.75 | 0.825 / 0.0825 / 4.125 |
| OpenRouter: `google/gemma-3-27b-it` | 0.08 / 0.04 / 0.45 | 0.08 / 0.04 / 0.35 |
| OpenRouter: `google/gemma-4-31b-it` | 0.09 / 0.05 / 0.34 | 0.2 / 0.05 / 0.4 |
| OpenRouter: `ibm-granite/granite-4.2-8b` | 0.06 / 0.015 / 0.25 | 0.06 / 0.015 / 0.15 |
| OpenRouter: `inclusionai/ling-3.0-flash-vl` | 0.021 / 0.0042 / 0.0616 | 0.06 / 0.012 / 0.18 |
| OpenRouter: `inclusionai/ling-3.1-flash` | 0 / — / 0 | 0.045 / 0.009 / 0.135 |
| OpenRouter: `mancer/weaver` | 0.4 / — / 0.75 | 0.3 / 0.3 / 0.5 |
| OpenRouter: `meta-llama/llama-3.1-70b-instruct` | 0.4 / — / 0.4 | 0.72 / — / 0.72 |
| OpenRouter: `meta-llama/llama-3.1-8b-instruct` | 0.05 / 0.025 / 0.08 | 0.02 / 0.025 / 0.04 |
| OpenRouter: `meta-llama/llama-3.2-3b-instruct` | 0.05 / — / 0.33 | 0.0509 / — / 0.335 |
| OpenRouter: `meta-llama/llama-3.3-70b-instruct` | 0.22 / 0.11 / 0.5 | 0.71 / 0.71 / 0.71 |
| OpenRouter: `meta-llama/llama-4-maverick` | 0.1875 / 0.05 / 0.6525 | 0.35 / 0.17 / 1 |
| OpenRouter: `meta-llama/llama-4-scout` | 0.18 / — / 0.59 | 0.1 / — / 0.3 |
| OpenRouter: `minimax/minimax-m2.5` | 0.27 / 0.027 / 1.08 | 0.295 / 0.06 / 1.2 |
| OpenRouter: `mistralai/mistral-large-4-0` | 0.68 / 0.07 / 2.09 | 0.7 / 0.07 / 2.1 |
| OpenRouter: `mistralai/mistral-small-3.2-24b-instruct` | 0.09375 / — / 0.25 | 0.1 / 0.01 / 0.3 |
| OpenRouter: `moonshotai/kimi-k2.7-code` | 0.6712 / 0.18 / 3.35 | 0.6 / 0.16 / 3.5 |
| OpenRouter: `moonshotai/kimi-k3` | 0.73 / 0.55 / 10 | 0.192 / 0.182 / 16 |
| OpenRouter: `nousresearch/hermes-4-405b` | 1 / — / 3 | 1 / 0.2 / 3 |
| OpenRouter: `nvidia/nemotron-3-nano-30b-a3b` | 0.06 / — / 0.24 | 0.06 / 0.012 / 0.24 |
| OpenRouter: `nvidia/nemotron-3-super-120b-a12b` | 0.08 / — / 0.45 | 0.075 / — / 0.4 |
| OpenRouter: `openai/gpt-4.1-nano` | 0.1 / 0.025 / 0.4 | 0.1 / 0.03 / 0.4 |
| OpenRouter: `openai/gpt-5.6-sol` | 2 / 0.2 / 10 | 4 / 0.4 / 20 |
| OpenRouter: `openai/gpt-5.6-sol-pro` | 2 / 0.2 / 10 | 4 / 0.4 / 20 |
| OpenRouter: `openai/gpt-oss-120b` | 0.037 / — / 0.17 | 0.15 / 0.075 / 0.6 |
| OpenRouter: `openai/gpt-oss-20b` | 0.018 / 0.009 / 0.09 | 0.075 / 0.0375 / 0.3 |
| OpenRouter: `qwen/qwen3-235b-a22b-2507` | 0.09 / — / 0.55 | 0.14 / 0.05 / 0.8 |
| OpenRouter: `qwen/qwen3-30b-a3b-instruct-2507` | 0.1 / — / 0.3 | 0.1 / 0.1 / 0.3 |
| OpenRouter: `qwen/qwen3-coder-30b-a3b-instruct` | 0.07 / — / 0.28 | 0.15 / — / 0.6 |
| OpenRouter: `qwen/qwen3-next-80b-a3b-instruct` | 0.1 / 0.07 / 1.1 | 0.09 / 0.07 / 1.1 |
| OpenRouter: `qwen/qwen3.5-35b-a3b` | 0.08 / 0.04 / 0.75 | 0.14 / 0.05 / 1 |
| OpenRouter: `qwen/qwen3.5-397b-a17b` | 0.55 / 0.225 / 3.5 | 0.45 / 0.22 / 3 |
| OpenRouter: `qwen/qwen3.5-9b` | 0.1 / — / 0.15 | 0.115 / 0.104 / 0.169 |
| OpenRouter: `qwen/qwen3.6-35b-a3b` | 0.1 / 0.1 / 0.95 | 0.1 / 0.025 / 0.9 |
| OpenRouter: `qwen/qwen3.8-27b` | 0.04 / 0.0198 / 2.3 | 0.15 / 0.0375 / 1.875 |
| OpenRouter: `stepfun/step-5-preview` | 1 / 0.05 / 2.7 | 1 / 0.1 / 2.7 |
| OpenRouter: `tencent/hy3` | 0.0825 / 0.020625 / 0.33 | 0.132 / 0.033 / 0.528 |
| OpenRouter: `tencent/hy4-preview` | 0.7506 / 0.0378 / 2.2509 | 0.834 / 0.042 / 2.501 |
| OpenRouter: `undi95/remm-slerp-l2-13b` | 0.35 / — / 0.65 | 0.3 / 0.2 / 0.5 |
| OpenRouter: `upstage/solar-pro4` | 0.09 / 0.018 / 0.36 | 0.15 / 0.03 / 0.6 |
| OpenRouter: `xiaomi/mimo-v2.5` | 0.14 / 0.0028 / 0.28 | 0.4 / 0.08 / 2 |
| OpenRouter: `z-ai/glm-4.7` | 0.6 / 0.11 / 2.2 | 0.4 / 0.08 / 1.75 |
| OpenRouter: `z-ai/glm-5` | 0.6 / 0.12 / 1.92 | 1 / 0.2 / 3.2 |
| OpenRouter: `z-ai/glm-5.1` | 0.966 / 0.1794 / 3.036 | 1.4 / 0.26 / 4.4 |
| OpenRouter: `z-ai/glm-5.2` | 0.06 / 0.059 / 7 | 0.007 / 0.006 / 6.8 |
| OpenRouter: `z-ai/glm-5.3` | 0.079 / 0.059 / 4.8 | 0.0699 / 0.0689 / 1.4 |
| OpenRouter: `z-ai/glm-5.3-flash` | 0.0232 / 0.0232 / 1.2 | 0.0399 / 0.03 / 0.0784 |
| OpenRouter: `z-ai/glm-5.3-flashx` | 0.37 / 0.09 / 1.25 | 0.37 / 0.075 / 1.25 |
| OpenRouter: `~anthropic/claude-fable-latest` | 10 / 0.25 / 50 | 8 / 0.2 / 40 |
| OpenRouter: `~deepseek/deepseek-flash-latest` | 0.0336 / 0.0336 / 1.188 | 0.036 / 0.01 / 0.5 |
| OpenRouter: `~deepseek/deepseek-pro-latest` | 0.24948 / 0.007938 / 0.74844 | 0.0999 / 0.0999 / 8 |
| OpenRouter: `~deepseek/deepseek-v4-flash-latest` | 0.0128 / 0.0128 / 0.768 | 0.0434 / 0.035 / 0.3 |
| OpenRouter: `~google/gemini-flash-latest` | 0.75 / 0.075 / 3.75 | 1.5 / 0.075 / 7.5 |
| OpenRouter: `~google/gemini-pro-latest` | 2 / 0.2 / 12 | 2 / 0.1 / 10 |
| OpenRouter: `~moonshotai/kimi-latest` | 0.72 / 0.3 / 24 | 0.1824 / 0.18 / 11.0055 |
| OpenRouter: `~openai/gpt-terra-latest` | 2 / 0.2 / 12 | 1 / 0.05 / 5 |
| OpenRouter: `~z-ai/glm-flash-latest` | 0.0232 / 0.0232 / 1.2 | 0.0399 / 0.03 / 0.0784 |
| OpenRouter: `~z-ai/glm-latest` | 0.055 / 0.06 / 5 | 0.0699 / 0.0689 / 1.4 |
| DeepInfra: `XiaomiMiMo/MiMo-V2.6-Pro` | 0.43 / 0.0035999987 / 0.87 | 0.387 / 0.00323999883 / 0.783 |
| DeepInfra: `nvidia/NVIDIA-Nemotron-3-Super-120B-A12B` | 0.085 / — / 0.4 | 0.075 / — / 0.4 |
| DeepInfra: `openai/gpt-oss-120b` | 0.037 / — / 0.17 | 0.037 / 0.0185 / 0.17 |
| DeepInfra: `openai/gpt-oss-20b` | 0.03 / — / 0.14 | 0.03 / 0.015 / 0.14 |
| DeepInfra: `thinkingmachines/Inkling-Small` | 0.45 / 0.1 / 1.2 | 0.45 / 0.099999999 / 1.2 |
| DeepInfra: `zai-org/GLM-5.3-Flash` | 0.075 / 0.015 / 0.25 | 0.15 / 0.03 / 0.5 |
| Together AI: `moonshotai/Kimi-K3` | 2.7 / 0.3 / 13.5 | 3 / 0.3 / 15 |

## Billing and compatibility details

### OpenRouter

- Terra 6.1 and its Pro alias use $1/$0.05/$5 input/cache/output and $1.25 cache
  writes below 272,000 prompt tokens; at the inclusive 272,000 minimum they use
  $2/$0.10/$7.50 and $2.50 writes. This host boundary intentionally differs from
  native OpenAI. `none` is available for Terra, not the Pro alias.
- DeepSeek Flash 1017 uses $0.15/$0.003/$0.60 off peak. On weekdays, 01:00–04:00
  and 06:00–10:00 UTC double all three rates. Weekends stay off peak.
- MiniMax M3.1 Flash uses $0.075/$0.0075/$0.30 from 15:00 through 01:00 UTC and
  $0.15/$0.015/$0.60 from 01:00 through 15:00. Existing Tencent schedules remain.
- The older DeepSeek V4.1 Flash and Pro 0813 rows no longer contain public API
  overrides. Remove their stale schedules and use their current flat quotes.
- [OpenRouter prompt caching](https://openrouter.ai/docs/guides/best-practices/prompt-caching)
  specifies that explicit Gemini cache writes include input processing plus five
  minutes of storage. The API storage component is therefore added to the base
  or tier input price. For Gemini 2.5 Pro, the short/long write prices are
  $1.625/$2.875; for Gemini 4 Carbon they are $2.1875. Implicit caches have no
  write surcharge. These are OpenRouter normalization rules, not a claim that
  the native Gemini API supports the same cache contract.
- Audio input prices are anchored relative to the corresponding text input
  price. Gemini Flash can have a different audio price (for example 2.5 Flash:
  $0.30 text versus $1 audio); audio output is not inferred. Gemini cache-read
  rates remain those published by OpenRouter, including in long-context tiers.
- Pro aliases remain host aliases. Sol Pro retains its description of the
  `reasoning.mode=pro` behavior and potentially higher reasoning-token usage.
- The synchronous catalog excludes `:batch`, unsupported media-output contracts,
  and dynamic negative-price routers such as `typesafe/jev-router`,
  `nvidia/switchyard`, `openrouter/fusion` and `openrouter/bodybuilder`. Their
  absence is about missing ordinary billing/transport contracts, not entitlement.
  Legacy entries are not deleted merely because they are absent from today’s API.
- Sonar Deep Research still publishes a distinct $3/M internal-reasoning meter
  versus $8/M visible output. The current shared catalog/receipt contract cannot
  fully represent that split. This is a recorded existing limitation; this refresh
  does not claim to have qualified its separate reasoning settlement.

### DeepInfra

The Flash 1017 quote incorporates the published 51% discount; Kimi K3.1 and
Inkling 2 Small incorporate 5%. MiMo V2.6 Pro now incorporates 10%. GLM 5.3 Flash
has no current discount, so its expired half-price default is removed. No expiry
date is invented where the source supplies only a present discount. New models
are limited to public, active, token-priced text-generation entries with the
compatible OpenAI endpoint tag. Embedding/image/video/dedicated units remain
separate. Discovery tests use the registered DeepInfra adaptor; constructing a
generic OpenAI adaptor with that channel enum is not its registered dispatch path.

The public source additionally advertises explicit retention writes at 1.25x/2x
input for 5m/1h on `nvidia/NVIDIA-Nemotron-3-Ultra-550B-A55B`,
`deepseek-ai/DeepSeek-V4-Flash`, `zai-org/GLM-5.2`, and `moonshotai/Kimi-K3`.
The [native retention contract](https://docs.deepinfra.com/chat/prompt-cache-retention)
requires `prompt_cache_options` and `prompt_cache_key`, and reports an
undifferentiated `cache_write_tokens` count. The current adaptor does not qualify
TTL-specific request/receipt normalization. The model descriptions record the
published multipliers, while the write fields remain unspecified to avoid
claiming that one-hour writes are safely handled by the shorter bucket.

### Novita

Ling 3.1 Flash now has an explicit $0.30/$0.06/$0.90 quote and is admitted.
`apodex/apodex-1.1-mini`, `bunny`, `nvrouter/auto`, `nvrouter/fusion` and
`dev/glm46` still lack an ordinary published tariff and are not introduced as
free models. Exact case is preserved for `zai-org/glm-5.3-flashX`.

Cache-write updates include Kimi K3 ($3/$6 for 5m/1h), Kimi K3.1 Pro ($2/$4),
Highspeed ($4/$8), and Qwen4 Max ($1.767/$2.827). Highspeed keeps the same $0.20
cached-input rate as ordinary Kimi K3.1. These values record distinct host write tariffs without adding a new native
cache-control transport. Unknown effort ladders stay empty.

### Together AI

Kimi K3 now uses the public $3/$0.30/$15 quote. Ripple’s missing cache, tools and
structured-output fields remain unspecified. New output ceilings are not copied
from first-party providers. Existing explicit free-output semantics are retained
for Prism and Tev1, including when an operator supplies an input-price override.
The current docs no longer list public serverless embedding/rerank products;
historical configured entries remain instead of being silently removed.

## Verification

The provider suites verify exact IDs through public discovery and pricing,
provider-specific limits, untouched tool-bearing requests, null-versus-free
distinctions, explicit cache-write prices, Gemini audio anchors, Terra thresholds,
DeepSeek weekday/weekend changes and MiniMax midnight transitions. Prior fixture
quotes were refreshed from the official source independently of the Go values.
Every pre-existing ID remains in the compiled catalogs. Full validation commands
and actual results are recorded in the [main audit](../adaptor-model-refresh-20261116.md).

No authenticated provider generation, full database/race qualification, region
certification or negotiated-tariff validation is claimed. Operator-selected
upstreams, optional server tools, storage durations and private deployments can
carry separate charges. This change leaves the existing pricing override layers
and unrelated protocol behavior in place.
