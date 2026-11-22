# Aggregator model audit — 2026-11-22

## Scope and evidence

Reviewed OpenRouter, DeepInfra, Novita, Together AI, Fireworks and SiliconFlow
against their own public catalogs and documentation. The entries remain ordinary
Go definitions in the existing family files. Existing configured IDs are retained.
A missing public row, an unpublished rate, and an explicitly free model remain
different cases. This audit does not qualify authenticated generation, regional
availability, private deployments or negotiated tariffs.

| Adaptor | Disposition | Primary evidence |
| --- | --- | --- |
| `openrouter` | Add eight compatible models; update 32 existing input/cache/output quotes, 28 published context/output fields, ten parameter lists, and two schedules | [Public catalog](https://openrouter.ai/api/v1/models), [prompt caching](https://openrouter.ai/docs/guides/best-practices/prompt-caching), [Neon model page](https://openrouter.ai/google/gemini-4-neon?view=api), [Neon endpoint details](https://openrouter.ai/api/v1/models/google/gemini-4-neon-20261118/endpoints) |
| `deepinfra` | Add Hy4 and DaisyRose; update GLM 5.3 Flash's current discount | [Public catalog](https://api.deepinfra.com/models/list), [Hy4 model API](https://api.deepinfra.com/models/tencent/Hy4), [DaisyRose model API](https://api.deepinfra.com/models/RadixArk/DaisyRose-27B), [DaisyRose card](https://deepinfra.com/RadixArk/DaisyRose-27B), [GLM Flash card](https://deepinfra.com/zai-org/GLM-5.3-Flash) |
| `novita` | Update Hy4 Flash's published discounted tariff; defer ambiguous new tiered model | [Public catalog](https://api.novita.ai/v3/openai/models), [Hy4 Flash card](https://novita.ai/models/model-detail/tencent-hy4-flash), [Qwen3 VL Plus card](https://novita.ai/models/model-detail/qwen-qwen3-vl-plus?from=pricing) |
| `togetherai` | Add Qwen4 Max and Hy4; fill newly published Ripple cache, tools, structured outputs and quantization | [Standard serverless catalog](https://docs.together.ai/docs/serverless/models.md) |
| `fireworks` | Reviewed; the verified compatible Standard serverless additions and prices are already represented | [Pricing](https://docs.fireworks.ai/serverless/pricing), [model library](https://fireworks.ai/models), [Ember card](https://fireworks.ai/models/fireworks/ember-1) |
| `siliconflow` | Reviewed; the latest published release notes still end on November 16 and the compatible additions/retirements are already represented | [Release notes](https://docs.siliconflow.cn/docs/release-notes/overview), [image API](https://docs.siliconflow.cn/docs/api/images-generations-post) |

The three JSON catalogs and the Together/Fireworks HTML were fetched directly
from the official public endpoints on November 22 at 01:54:49–01:54:53 UTC.
Browser-indexed results were cross-checked with these direct responses. These
are time-stamped observations of mutable quotes, not promises about future
routing or permanent discounts. SHA-256 fingerprints identify the exact responses:

| Source | SHA-256 |
| --- | --- |
| openrouter | `88b49666d52a2c84366a0db0d3603f45fa2b5561d52679adc724b6dccf6d4def` |
| deepinfra | `44bb29d43d166a0e86d794c498310bc376f92f30c7c4f1fac8e442f101eaa4b9` |
| novita | `f20c0d954ba685f6b521ce7b27131152924c096a7da8892f71fc0860acd4162f` |
| togetherai | `57e383081041e82bc0abe6e396ece975335f66fb770f085fd701a28f432ef29e` |
| fireworks | `926a3a7e48e724781e8210c7f0f9eefb1fde54b70897d4222b119319fcb6245d` |
| siliconflow | `f93f7513df717c957b40f6f114a72447cc2aae41821a82660e11c02e5f5be6ba` |

Neon's endpoint-specific response was additionally fetched at 02:09:44 UTC
on November 22. Its SHA-256 is
`32a980f1257440c96057e7e2a3f20d015af12972af47550d6cdd0b26f6a8dbbc`.

## New text-output models

All prices below are USD per million tokens, in input / cached-input / output
order. Unknown values stay unspecified. Counts after this change are OpenRouter
549, DeepInfra 207, Novita 158 and Together 100; no historical ID is deleted.

| Provider / exact API ID | Input / cache / output | Context / max output |
| --- | --- | --- |
| OpenRouter `openai/gpt-6.2-astra` | 8 / 0.4 / 40 | 1,050,000 / 128,000 |
| OpenRouter `openai/gpt-6.2-astra-pro` | 8 / 0.4 / 40 | 1,050,000 / 128,000 |
| OpenRouter `openai/gpt-6.2-sol` | 2 / 0.1 / 10 | 1,050,000 / 128,000 |
| OpenRouter `openai/gpt-6.2-sol-pro` | 2 / 0.1 / 10 | 1,050,000 / 128,000 |
| OpenRouter `google/gemini-4-neon` | 3 / 0.15 / 15 | 1,048,576 / 65,536 |
| OpenRouter `thinkingmachines/inkling-2-mini` | 0.075 / 0.015 / 0.3 | 1,048,576 / 65,536 |
| OpenRouter `radixark/daisyrose-27b` | 0.15 / 0.0375 / 1.875 | 262,144 / 235,929 |
| OpenRouter `mistralai/pixtral-large-2502` | 2 / unspecified / 6 | 128,000 / 102,400 |
| DeepInfra `RadixArk/DaisyRose-27B` | 0.15 / 0.0375 / 1.875 | 262,144 / unspecified |
| DeepInfra `tencent/Hy4` | 0.834 / 0.04199999814 / 2.501 | 1,048,576 / 131,072 |
| Together `Qwen/Qwen4-Max` | 1.425 / 0.07125 / 4.275 | 1,000,000 / unspecified |
| Together `tencent/Hy4` | 0.834 / 0.042 / 2.501 | 1,048,576 / unspecified |

### Provider-specific details

- OpenRouter GPT-6.2 applies the expensive tier only above **272,000 prompt
  tokens** (`InputTokenThreshold = 272001`). Astra then charges 16 / 0.8 / 60
  and Sol 4 / 0.2 / 15. Published cache writes are 10 and 2.5 below the boundary,
  doubling above it. Pro IDs are OpenRouter host aliases using `reasoning.mode=pro`;
  they are not inferred native OpenAI IDs. The published reasoning ladder is
  `max`, `xhigh`, `high`, `medium`, `low`, with `medium` default and no `none`.
- OpenRouter Neon accepts text, images, files, audio and video and emits text.
  Audio input uses the same $3/M price as text. Its FAQ and standard AI Studio
  endpoint publish 65,536 ordinary completion tokens. The aggregate catalog and
  standard Vertex endpoint instead publish 943,718, while the model page says
  long decode requires continuation. This refresh uses the conservative 65,536
  advisory ceiling and introduces no continuation protocol. Its effort ladder
  includes `xhigh` but not `none`. No audio-output tariff is invented.
  The host publishes a $1/12-per-million storage component. For reported cache
  writes, the configured rate normalizes it with the $3/M input charge under
  OpenRouter's documented Gemini formula. This tariff does not qualify explicit
  cache availability: the generic guide defers model support to Google, and the
  current endpoint responses mark `supports_implicit_caching` false. Cache-mode
  availability remains unqualified; no cache capability flag is added.
- OpenRouter Pixtral 2502 publishes no tools capability or cached-input tariff;
  neither is copied from a Mistral sibling. Inkling 2 Mini has its own 65,536
  output ceiling and supports disabling reasoning.
- DeepInfra DaisyRose applies the published 25% discount once. FP8 serving is
  established by its own model card; the list API leaves quantization null.
  Its output ceiling and complete effort vocabulary remain unspecified.
- DeepInfra Hy4's model-detail API publishes 131,072 max output. The cached
  price retains the catalog's 0.05035971 multiplier of the $0.834 input quote;
  Together independently publishes a rounded $0.042 cached quote and no output
  ceiling. Creator limits and another host's prices are never substituted.
- Together Qwen4 Max and Hy4 are listed in both the chat and vision sections.
  Ripple now explicitly lists $0.02/M cached input, function calling, structured
  outputs and NVFP4, mapped to the gateway's existing `fp4` vocabulary. US-only,
  priority, dedicated and private contracts remain separate.

## Existing price changes

The following are base/default catalog prices. DeepSeek Flash 1017 stores its
published **peak** tariff as the base, with explicit off-peak windows described
below; its current Sunday API quote is $0.15 / $0.003 / $0.60.
A zero cached price in this table means **unspecified**, so the normal input
price applies unless another configured policy supplies a rate; it does not mean
free cache reads. Missing cache prices are removed rather than carried forward
from a previously selected endpoint.

| Provider / ID | Previous input / cache / output | Current input / cache / output |
| --- | --- | --- |
| openrouter `particle-ai/particle-1` | 0.06 / 0.003 / 0.5 | 0.07 / 0.006 / 0.5 |
| openrouter `moonshotai/kimi-k3.1-pro` | 0.2403 / 0.2 / 8.4023 | 0.253 / 0.18 / 8.8 |
| openrouter `deepseek/deepseek-v4.1-flash-1017` | 0.036 / 0.01 / 0.5 | 0.3 / 0.006 / 1.2 |
| openrouter `reflection/beam-1.0-501b-a23b` | 0.3 / 0.04 / 1 | 0.25 / 0.05 / 1 |
| openrouter `nvidia/nemotron-3.5-super-vl` | 0.15 / 0.02 / 0.5 | 0.15 / 0.02 / 0.47 |
| openrouter `inclusionai/ling-3.1-flash` | 0.045 / 0.009 / 0.135 | 0.06 / 0.012 / 0.18 |
| openrouter `deepseek/deepseek-v4.1-flash` | 0.0507 / 0.001014 / 0.2028 | 0.0498 / 0.000996 / 0.1992 |
| openrouter `~z-ai/glm-flash-latest` | 0.15 / 0.03 / 0.5 | 0.0399 / 0.03 / 0.0784 |
| openrouter `z-ai/glm-5.3-flash` | 0.15 / 0.03 / 0.5 | 0.0399 / 0.03 / 0.0784 |
| openrouter `deepseek/deepseek-v4-flash-vision-exp` | 0.44 / 0.014 / 1.32 | 0.2156 / 0.00686 / 0.6468 |
| openrouter `qwen/qwen3.8-27b` | 0.06 / 0.059 / 2.2 | 0.035 / 0.0175 / 1.5 |
| openrouter `deepseek/deepseek-v4-pro-0813` | 0.111 / 0.097 / 4.44 | 1.32 / 0.044 / 3.96 |
| openrouter `~deepseek/deepseek-v4-flash-latest` | 0.0434 / 0.035 / 0.3 | 0.044 / 0.0014 / 0.132 |
| openrouter `deepseek/deepseek-v4-flash-0731` | 0.06 / 0.015 / 0.18 | 0.0443 / 0.035 / 0.3 |
| openrouter `tencent/hy3` | 0.132 / 0.033 / 0.528 | 0.14 / 0.035 / 0.58 |
| openrouter `z-ai/glm-5.2` | 0.007 / 0.006 / 6.8 | 0.62 / 0.14 / 2.42 |
| openrouter `moonshotai/kimi-k2.7-code` | 0.7 / 0.17 / 3.75 | 0.6 / 0.12 / 3 |
| openrouter `deepseek/deepseek-v4-pro` | 0.9 / 0.25 / 1.5 | 0.9 / 0.2 / 1.5 |
| openrouter `z-ai/glm-5.1` | 1.4 / 0.26 / 4.4 | 1.19 / 0.6 / 3.74 |
| openrouter `google/gemma-4-26b-a4b-it` | 0.09 / 0.05 / 0.3 | 0.0675 / 0.0375 / 0.225 |
| openrouter `minimax/minimax-m2.7` | 0.21 / 0.042 / 0.84 | 0.6 / 0 / 1.8 |
| openrouter `qwen/qwen3.5-9b` | 0.17 / 0.153 / 0.25 | 0.17 / 0 / 0.25 |
| openrouter `qwen/qwen3.5-27b` | 0.3 / 0 / 2.4 | 0.26 / 0 / 2.6 |
| openrouter `minimax/minimax-m2.5` | 0.295 / 0.06 / 1.2 | 0.27 / 0.03 / 0.95 |
| openrouter `z-ai/glm-4.6v` | 0.3 / 0.05 / 0.9 | 0.3 / 0.055 / 0.9 |
| openrouter `openai/gpt-oss-120b` | 0.15 / 0.075 / 0.6 | 0.09 / 0 / 0.36 |
| openrouter `openai/gpt-oss-20b` | 0.018 / 0.009 / 0.09 | 0.017 / 0.0085 / 0.085 |
| openrouter `qwen/qwen3-coder-30b-a3b-instruct` | 0.15 / 0 / 0.6 | 0.07 / 0 / 0.28 |
| openrouter `deepseek/deepseek-chat-v3-0324` | 0.29 / 0.11 / 1.14 | 0.25 / 0 / 1 |
| openrouter `google/gemma-3-4b-it` | 0.05 / 0 / 0.1 | 0.04 / 0.02 / 0.05 |
| openrouter `meta-llama/llama-3.2-3b-instruct` | 0.0509 / 0 / 0.335 | 0.05 / 0 / 0.33 |
| openrouter `gryphe/mythomax-l2-13b` | 0.08 / 0 / 0.11 | 0.3 / 0.2 / 0.5 |
| deepinfra `zai-org/GLM-5.3-Flash` | 0.15 / 0.03 / 0.5 | 0.1005 / 0.0201 / 0.335 |
| novita `tencent/hy4-flash` | 0.07 / 0.003 / 0.281 | 0.035 / 0.0015 / 0.1405 |
| Together `ricursive-ai/Ripple-65B-Preview` | 0.2 / unspecified / 0.6 | 0.2 / 0.02 / 0.6 |

DeepInfra GLM 5.3 Flash's **33% discount** produces $0.1005 / $0.0201 / $0.335;
the page rounds its displayed input/cache values. No end date is published.
The existing deprecated-model safety floor automatically follows this current
replacement tariff for GLM 4.7 Flash, while retaining the larger original
$0.40/M output price. No deprecation or forwarding policy is changed.
Novita Hy4 Flash's nested decimal fields already include the **50% discount**;
no second multiplier is applied.

### Time windows and mutable aliases

OpenRouter Flash 1017 again publishes weekday peak times of 01:00–04:00 and
06:00–10:00 UTC at $0.30 / $0.006 / $1.20. Remaining weekday hours and all
weekends use $0.15 / $0.003 / $0.60. The base retains the highest published
price for conservative preflight paths, and explicit off-peak windows implement
the discounts. This supersedes the November 18 flat quote without claiming a
retroactive provider change date.

OpenRouter Hy3 now publishes a flat $0.14 / $0.035 / $0.58, with no time override.
Its former 16:00 UTC discount is removed. Hy4 Preview and MiniMax M3.1 Flash
retain their independently published schedules.

`~deepseek/deepseek-flash-latest` still publishes its own flat $0.036 / $0.01 /
$0.50. The pinned Flash 1017 ID chooses a different host quote. Regression tests
therefore check each advertised routing tariff rather than forcing aliases to
share a pinned model's price. GLM Flash's alias and pinned ID currently agree at
$0.0399 / $0.03 / $0.0784. Existing requests naming these mutable IDs see the
new current defaults; channel/operator overrides keep their existing precedence.

### Automatic OpenAI cache writes through OpenRouter

OpenRouter's own caching guide states that GPT-5.6 and later charge automatic
writes at 1.25 times input, with no opt-in required. The host-specific
`ImplicitPromptCacheWrites` predicate recognizes cataloged GPT-5.6/GPT-6 IDs and
the four verified Astra/Sol/Terra/Luna aliases only when their own published
cache-write rate is positive. Unknown IDs, native IDs, other providers, older
families and the unverified legacy `~openai/gpt-latest` target do not qualify.
This permits admission to fund OpenRouter's own write quote without importing
native OpenAI tariffs or applying its billing policy to unrelated hosts.

## Published host limits

These numeric fields were explicitly present in OpenRouter's current response.
They describe the host's advertised context/output limits and remain advisory;
no new request cap is imposed.

| OpenRouter ID | Field | Previous | Current |
| --- | --- | ---: | ---: |
| `gryphe/mythomax-l2-13b` | `MaxOutputTokens` | 3,686 | 7,372 |
| `reflection/beam-1.0-501b-a23b` | `MaxOutputTokens` | 943,718 | 131,072 |
| `deepseek/deepseek-chat-v3-0324` | `MaxOutputTokens` | 115,200 | 147,456 |
| `deepseek/deepseek-r1-0528` | `MaxOutputTokens` | 32,768 | 32,000 |
| `deepseek/deepseek-v4-flash-0731` | `MaxOutputTokens` | 384,000 | 943,718 |
| `deepseek/deepseek-v4-flash-vision-exp` | `MaxOutputTokens` | 943,717 | 262,144 |
| `~deepseek/deepseek-v4-flash-latest` | `MaxOutputTokens` | 943,718 | 131,072 |
| `tencent/hy3` | `MaxOutputTokens` | 128,000 | 235,929 |
| `openai/gpt-oss-120b` | `MaxOutputTokens` | 65,536 | 117,964 |
| `openai/gpt-oss-20b` | `MaxOutputTokens` | 65,536 | 117,964 |
| `meta/muse-glimmer-30b` | `MaxOutputTokens` | 16,384 | 117,964 |
| `qwen/qwen3-next-80b-a3b-instruct` | `MaxOutputTokens` | 16,384 | 235,929 |
| `qwen/qwen3.5-9b` | `MaxOutputTokens` | 40,960 | 235,929 |
| `qwen/qwen3.8-2.4t-a95b` | `MaxOutputTokens` | 131,072 | 65,536 |
| `qwen/qwen3.8-27b` | `MaxOutputTokens` | 235,929 | 131,072 |
| `mistralai/mistral-small-3.2-24b-instruct` | `MaxOutputTokens` | 26,214 | 16,384 |
| `minimax/minimax-m2.5` | `MaxOutputTokens` | 176,947 | 32,768 |
| `minimax/minimax-m2.7` | `MaxOutputTokens` | 176,947 | 131,072 |
| `nvidia/nemotron-3-ultra-550b-a55b` | `MaxOutputTokens` | 182,520 | 16,384 |
| `nvidia/nemotron-3.5-lightning` | `ContextLength` | 262,144 | 1,048,576 |
| `nvidia/nemotron-3.5-super-vl` | `MaxOutputTokens` | 235,929 | 943,718 |
| `z-ai/glm-5.1` | `MaxOutputTokens` | 182,476 | 131,072 |
| `xiaomi/mimo-v2.5-pro` | `ContextLength` | 1,024,000 | 262,144 |
| `xiaomi/mimo-v2.5-pro` | `MaxOutputTokens` | 131,072 | 235,929 |
| `xiaomi/mimo-v2.6-flash` | `ContextLength` | 1,050,000 | 1,048,576 |
| `xiaomi/mimo-v2.6-pro` | `ContextLength` | 1,050,000 | 1,048,576 |
| `google/gemma-3-4b-it` | `MaxOutputTokens` | 16,384 | 117,964 |
| `meta-llama/llama-3.2-3b-instruct` | `MaxOutputTokens` | 72,000 | 117,964 |

## Published host parameters

Ten updated rows also changed their explicit nonempty `supported_parameters`
lists. Those lists are synchronized with the captured response, together with
the corresponding `tools`, `json_mode`, `structured_outputs`, `reasoning` and
`logprobs` feature declarations. An omitted advertised parameter does not prove
that every native model or provider deployment lacks the capability; it is no
longer claimed in this routing-default metadata. No request filter is added.

| OpenRouter ID | No longer advertised | Newly advertised |
| --- | --- | --- |
| `deepseek/deepseek-chat-v3-0324` | `logprobs`, `top_logprobs`, `seed` | — |
| `deepseek/deepseek-v4-flash-vision-exp` | `logprobs`, `top_logprobs` | — |
| `minimax/minimax-m2.7` | `response_format` | — |
| `meta-llama/llama-3.2-3b-instruct` | `min_p` | — |
| `minimax/minimax-m2.5` | `logit_bias`, `min_p`, `repetition_penalty`, `seed` | — |
| `moonshotai/kimi-k2.7-code` | `min_p` | — |
| `z-ai/glm-5.1` | `logit_bias`, `min_p` | — |
| `z-ai/glm-5.2` | `parallel_tool_calls` | — |
| `qwen/qwen3.8-2.4t-a95b` | — | `logit_bias` |
| `qwen/qwen3-coder-30b-a3b-instruct` | — | `frequency_penalty`, `response_format`, `structured_outputs` |

The DeepSeek rows no longer advertise the `logprobs` feature, MiniMax M2.7 no
longer advertises `json_mode`, and Qwen3 Coder 30B now advertises JSON mode and
structured outputs. MiniMax M2.7 continues to omit structured outputs.

## Deliberate exclusions and current limits

- OpenRouter `:batch` products, audio/image/video output protocols and dynamic
  routers without a non-negative ordinary token tariff are not introduced into
  the synchronous text catalog. The eight additions have published text-output
  contracts. Absent legacy IDs remain configurable.
- Novita's new `qwen/qwen3-vl-plus` advertises a 262,000-token context, while
  its current detailed tiers cover `1 <= input < 32000`, `32000 <= input <
  128000`, and `128000 <= input < 256000`. The API also returns generic $1/M
  top-level prices that conflict with those tiers. Neither the card nor the
  API explains a tariff at or above 256,000 input tokens. Adding a last tier
  that applies forever, inventing an input cap from context metadata, or treating
  the generic price as a verified fallback would overstate support. The model
  is deferred until its complete billing contract is established.
- Together's current documentation distinguishes true `megapixel` rates from
  `image` rows, which are estimates passed through from upstream and vary by
  request parameters. No additional such image model is registered here. The
  existing `perImageAreaTariff` extrapolation is an unqualified historical
  billing assumption, not a source-certified tariff; resolving it requires a
  separate receipt or explicit operator fallback policy.
- Fireworks Standard rates remain distinct from Priority, Fast, Ultra-fast,
  Flex, regional capacity and native System One. Existing entries already
  cover Ember, Kimi K3.1, MiniMax M3.1, Flash 1017, Muse Spark and Ultramarine.
- SiliconFlow's latest published notes still describe the November 16 dynamic
  limiter and November 3 notices already recorded in the previous audit. No
  duplicate Flash `-1017` ID, unsupported native video contract or invented
  post-November-16 release is added.

## Validation

Updated existing provider fixtures against the new sources. Extended public
model discovery for every new ID and request-preservation coverage for the new
tool-capable IDs. Added production-resolver tests for the GPT-6.2 prompt boundary
and cache writes, plus checks for host-specific capabilities, Neon audio/cache
normalization and conservative ordinary-output ceiling, Pixtral's missing tools
and cache tariff, current parameter additions/removals, automatic-write host
classification, and exact weekday/weekend schedule transitions. Existing
conservative peak-base, cache fallbacks, deprecation floors and constructor
isolation assertions remain in force.

Formatting and `git diff --check` pass. The following six focused suites pass
with Go 1.27.4. The coordinating PR records the cross-provider catalog checks:

```text
go test ./relay/adaptor/openrouter ./relay/adaptor/deepinfra ./relay/adaptor/novita ./relay/adaptor/togetherai ./relay/adaptor/fireworks ./relay/adaptor/siliconflow
```

No authenticated inference, full race/database qualification or provider-specific
regional deployment certification is claimed.
