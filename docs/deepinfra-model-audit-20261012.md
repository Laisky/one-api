# DeepInfra model audit — 2026-10-12

Sources: https://api.deepinfra.com/models/list and https://deepinfra.com/pricing

The 386-row public catalogue was compared with initialized defaults. Private and
deprecated entries are excluded from new admissions. Four current public
text-generation IDs are added and six existing token tariffs are refreshed.
Historical IDs already configured by operators are retained.

Evidence SHA-256: `86257823636e442c60a21ea603faa8ff0c50b58a34694e63b8b445c1bca8acb4`.

| Public model ID | Input USD/M | Cached USD/M | Output USD/M |
| --- | ---: | ---: | ---: |
| `anthropic/claude-sonnet-5-5` | 2 | Unspecified | 10 |
| `tencent/Hy4-preview` | 0.834 | 0.04199999814 | 2.501 |
| `XiaomiMiMo/MiMo-V2.6-Pro` | 0.43 | 0.0035999987 | 0.87 |
| `XiaomiMiMo/MiMo-V2.6-Flash` | 0.14 | 0.0028 | 0.28 |
| `nvidia/NVIDIA-Nemotron-3.5-Lightning` | 0.06 | 0.03 | 0.16 |
| `google/gemini-3.5-flash` | 1.5 | Unspecified | 9 |
| `mistralai/Mistral-Nemo-Instruct-2407` | 0.029 | Unspecified | 0.03 |
| `Qwen/Qwen3.6-35B-A3B` | 0.1 | 0.1 | 0.95 |
| `google/gemma-4-31B-it` | 0.2 | Unspecified | 0.4 |
| `google/gemini-3.7-flash` | 0.75 | Unspecified | 3.75 |

## Units, discounts and unknown prices

`cents_per_input_token` and `cents_per_output_token` become USD/M by multiplying
by 10,000. Apply `(1 - discount)` exactly once. The cached-input rate is a
multiplier on the discounted input price, not an absolute token price. Decimal
precision from the host is retained rather than copying rounded webpage figures.
MiMo Pro input is USD 0.43 here, independently of Novita's USD 0.435.

The baseline already correctly applies the published GLM 5.2/5.3, Qwen 3.8, and
DeepSeek promotions. Their cheaper effective prices are preserved and protected
by regressions. Missing discount end dates are not guessed. Priority and Flex
multipliers remain separate from standard default prices.

Sonnet 5.5 is an active public partner endpoint; `no-free-anon` does not mean
private or free API pricing. Its cache price is unpublished and stays unspecified
(normal-input fallback), not a borrowed Anthropic discount or explicit free
tariff. Unpublished output limits and discrete reasoning controls for the other
new IDs are not inferred. New capabilities follow host tags and modalities; this
change does not implement additional protocol conversions or media billing.

## Validation and rollout

Independent regression assertions cover all ten prices, preserved discounts,
provider isolation, discovery, unknown-field semantics, allocations, and request
IDs/tools. Old-code failures and full affected package race-test receipts are
recorded in the PR. Existing negotiated channel overrides are not changed.
No database or deployment change and no paid live-provider request is included.
Revert this provider commit to restore the preceding defaults.
