# OpenRouter model audit — 2026-10-12

Sources: https://openrouter.ai/api/v1/models and https://openrouter.ai/docs/guides/best-practices/prompt-caching

The 459-row public catalogue was checked against the compiled provider defaults.
This commit adds 15 synchronous text-output IDs and refreshes 60 existing entries
with verified price drift or newly published overrides. Definitions remain plain
Go with one owner per ID; no runtime importer or initialization override is added.

Evidence SHA-256: `4e80515b15d51d4e3632f8e38c9c473d851fd215077b04e8025caf640c7db517`.

## Published routing-default tariffs

USD per million tokens. Unpublished cache prices remain unspecified (normal-input
fallback), not free. These default prices do not quote every selected provider.
Account routing, optional tools, BYOK and negotiated rates can require overrides.

| Public ID | Input | Cached input | Output |
| --- | ---: | ---: | ---: |
| `meta/muse-wonder` | 4 | 0.2 | 20 |
| `stepfun/step-5-preview` | 1 | 0.05 | 2.7 |
| `anthropic/claude-haiku-5.5` | 0.1 | 0.01 | 0.5 |
| `mistralai/mistral-large-4-0` | 0.68 | 0.07 | 2.09 |
| `inclusionai/ling-3.1-flash` | 0 | 0 | 0 |
| `apodex/apodex-1.1-mini:free` | 0 | 0 | 0 |
| `unbiased/pareto-26.10-preview` | 0.8 | 0.03 | 3.2 |
| `anthropic/claude-sonnet-5.5` | 2 | 0.1 | 10 |
| `perceptron/perceptron-mk1.5` | 0.15 | 0 | 1.5 |
| `fireworks/ember-1` | 3 | 0.3 | 15 |
| `x-ai/grok-4.7` | 2 | 0.5 | 6 |
| `prism-ml/ternary-bonsai-2-27b` | 0.075 | 0.0375 | 0.5 |
| `~deepseek/deepseek-pro-latest` | 0.24948 | 0.007938 | 0.74844 |
| `~deepseek/deepseek-flash-latest` | 0.0336 | 0.0336 | 1.188 |
| `~openai/gpt-sol-latest` | 2 | 0.1 | 10 |
| `inclusionai/ling-3.0-flash-vl` | 0.021 | 0.0042 | 0.0616 |
| `deepseek/deepseek-v4.1-flash` | 0.15 | 0.003 | 0.6 |
| `inclusionai/ling-3.0-flash-sante` | 0.042 | 0.0084 | 0.1232 |
| `google/gemini-3.8-flash` | 0.75 | 0.075 | 3.75 |
| `tencent/hy4-preview` | 0.7506 | 0.0378 | 2.2509 |
| `inclusionai/ling-3.0-flash-fin` | 0.042 | 0.0084 | 0.1232 |
| `~z-ai/glm-flash-latest` | 0.0232 | 0.0232 | 1.2 |
| `z-ai/glm-5.3-flash` | 0.0232 | 0.0232 | 1.2 |
| `deepseek/deepseek-v4-flash-vision-exp` | 0.2156 | 0.00686 | 0.6468 |
| `~z-ai/glm-latest` | 0.055 | 0.06 | 5 |
| `z-ai/glm-5.3` | 0.079 | 0.059 | 4.8 |
| `qwen/qwen3.8-27b` | 0.04 | 0.0198 | 2.3 |
| `google/gemini-3.7-flash` | 0.75 | 0.075 | 3.75 |
| `deepseek/deepseek-v4-pro-0813` | 0.66 | 0.022 | 1.98 |
| `nvidia/nemotron-3.5-lightning` | 0.07 | 0.04 | 0.2 |
| `~deepseek/deepseek-v4-flash-latest` | 0.0128 | 0.0128 | 0.768 |
| `deepseek/deepseek-v4-flash-0731` | 0.016 | 0.016 | 0.64 |
| `google/gemini-3.6-flash` | 0.75 | 0.075 | 3.75 |
| `google/gemini-3.5-flash-lite` | 0.3 | 0.03 | 2.5 |
| `moonshotai/kimi-k3` | 0.73 | 0.55 | 10 |
| `~x-ai/grok-latest` | 2 | 0.5 | 6 |
| `tencent/hy3` | 0.0825 | 0.020625 | 0.33 |
| `z-ai/glm-5.2` | 0.06 | 0.059 | 7 |
| `moonshotai/kimi-k2.7-code` | 0.6712 | 0.18 | 3.35 |
| `~anthropic/claude-haiku-latest` | 0.1 | 0.01 | 0.5 |
| `~google/gemini-pro-latest` | 2 | 0.2 | 12 |
| `~moonshotai/kimi-latest` | 0.72 | 0.3 | 24 |
| `~google/gemini-flash-latest` | 0.75 | 0.075 | 3.75 |
| `~anthropic/claude-sonnet-latest` | 2 | 0.1 | 10 |
| `qwen/qwen3.6-35b-a3b` | 0.1 | 0.1 | 0.95 |
| `qwen/qwen3.6-27b` | 0.32 | 0 | 3.2 |
| `deepseek/deepseek-v4-pro` | 0.41238 | 0.034365 | 0.82476 |
| `deepseek/deepseek-v4-flash` | 0.03 | 0.03 | 1.28 |
| `moonshotai/kimi-k2.6` | 0.65 | 0.15 | 3.41 |
| `minimax/minimax-m2.7` | 0.21 | 0.042 | 0.84 |
| `qwen/qwen3.5-35b-a3b` | 0.08 | 0.04 | 0.75 |
| `qwen/qwen3.5-27b` | 0.26 | 0 | 2.6 |
| `google/gemini-3.1-pro-preview-customtools` | 2 | 0.2 | 12 |
| `google/gemini-3.1-pro-preview` | 2 | 0.2 | 12 |
| `moonshotai/kimi-k2.5` | 0.5 | 0.15 | 2.5 |
| `z-ai/glm-4.7` | 0.6 | 0.11 | 2.2 |
| `nvidia/nemotron-3-nano-30b-a3b` | 0.06 | 0 | 0.24 |
| `deepseek/deepseek-v3.2` | 0.259 | 0.14 | 0.8 |
| `qwen/qwen3-vl-8b-instruct` | 0.25 | 0.12 | 0.75 |
| `qwen/qwen3-vl-30b-a3b-thinking` | 0.29 | 0 | 1 |
| `qwen/qwen3-vl-30b-a3b-instruct` | 0.15 | 0 | 0.6 |
| `z-ai/glm-4.6` | 0.5 | 0.1 | 2 |
| `openai/gpt-oss-120b` | 0.037 | 0 | 0.17 |
| `openai/gpt-oss-20b` | 0.018 | 0.009 | 0.09 |
| `qwen/qwen3-30b-a3b-instruct-2507` | 0.1 | 0 | 0.3 |
| `qwen/qwen3-235b-a22b-thinking-2507` | 0.45 | 0 | 3.5 |
| `qwen/qwen3-235b-a22b-2507` | 0.09 | 0 | 0.55 |
| `minimax/minimax-m1` | 0.55 | 0 | 2.2 |
| `google/gemini-2.5-pro` | 1.25 | 0.125 | 10 |
| `google/gemini-2.5-pro-preview` | 1.25 | 0.125 | 10 |
| `meta-llama/llama-4-maverick` | 0.1875 | 0.05 | 0.6525 |
| `meta-llama/llama-4-scout` | 0.18 | 0 | 0.59 |
| `deepseek/deepseek-chat-v3-0324` | 0.29 | 0.11 | 1.14 |
| `meta-llama/llama-3.3-70b-instruct` | 0.22 | 0.11 | 0.5 |
| `mistralai/mistral-nemo` | 0.029 | 0 | 0.03 |

## Billing and capability boundaries

Haiku 5.5 switches all five tariffs at the inclusive 100,000-input-token minimum.
Grok 4.7 and aliases retain complete 200K long-context tiers. DeepSeek and Tencent
use the published host UTC weekday/clock overrides, not a borrowed native tariff.
Google explicit cache writes include input plus the published five-minute storage
component; the same rule applies within tiers. Implicit cache usage has no write
surcharge. Audio input has its own anchor; these entries do not generate audio.
OpenRouter no longer advertises Sonnet 5.5 temperature, so its metadata and prior
regression assertion are corrected together with the cache-read rate.

Mistral Large 4 uses the current OpenRouter quote. Direct Mistral promotion dates
are not automatically imported across providers. Legacy IDs remain configurable;
retaining them is not a guarantee of availability. Existing account overrides and
request formats are untouched.

## Exclusions and validation

Batch IDs, dynamic routers with -1 prices, canonical slugs that are not API IDs,
and new image/audio-output models without qualified modality-specific billing
are not inserted as synchronous cheap text models. Optional tool charges are not
folded into token rates. Tests cover all changed IDs, public/generic discovery,
tier/clock boundaries, independent metadata, and request/tool preservation.
The PR records actual negative-control and race-test receipts. Reverting this
provider commit rolls back these defaults without a database migration.
