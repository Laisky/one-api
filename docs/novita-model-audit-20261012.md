# Novita model audit — 2026-10-12

Source: https://api.novita.ai/v3/openai/models

The 94-row live catalogue was compared with the initialized Novita defaults.
Nine missing priced public text-output models are added; all existing priced chat
rows match the public input/cache/output amounts. Historical identifiers remain
configurable and are not automatically deleted when absent from discovery.

Evidence SHA-256: `88548bc6caa134d3d5c7c5fd5f29e2cd5f7a0fb9b29486cd5a4ba0ee73089a4c`.

| API ID | Input USD/M | Cached USD/M | Output USD/M | Context | Max output |
| --- | ---: | ---: | ---: | ---: | ---: |
| `deepseek/deepseek-v4-pro-0813-p` | 1.32 | 0.044 | 3.96 | 1048576 | 393216 |
| `deepseek/deepseek-v4.1-flash-dst` | 0.3 | 0.006 | 1.2 | 1048576 | 393216 |
| `deepseek/deepseek-v4.1-flash-p` | 0.3 | 0.006 | 1.2 | 1048576 | 393216 |
| `xiaomimimo/mimo-v2.6-flash` | 0.14 | 0.0028 | 0.28 | 1048576 | 131072 |
| `xiaomimimo/mimo-v2.6-pro` | 0.435 | 0.0036 | 0.87 | 1048576 | 131072 |
| `moonshotai/kimi-k3-p` | 3 | 0.3 | 15 | 1048576 | 1048576 |
| `deepseek/deepseek-v4-flash-0731-p` | 0.44 | 0.0148 | 1.32 | 1048576 | 393216 |
| `inclusionai/ling-3.0-flash-fin` | 0.075 | 0.015 | 0.22 | 262144 | 32768 |
| `inclusionai/ling-3.0-flash-sante` | 0.075 | 0.015 | 0.22 | 262144 | 32768 |

## Provider and transport boundaries

Use the nested `price_per_m_decimal` USD fields. The older integer amounts are
scaled by 10,000; treating them as dollars would overcharge by four orders of
magnitude. MiMo Pro input is USD 0.435 here, not DeepInfra's USD 0.43. The source
reports no tiers for these nine records. No guessed DeepSeek peak/off-peak,
priority surcharge, native creator tariff, quantization, or weight ID is added.
Capabilities follow the host features. Discrete reasoning controls remain
unspecified because the native Novita adaptor removes `reasoning_effort`.
Multimodal inputs are host metadata, not new server-side media conversion code.

`apodex/apodex-1.1-mini`, `inclusionai/ling-3.1-flash`, `bunny`, and `dev/glm46`
have null pricing and are not treated as free. Ming image-generation entries
require separate endpoint/unit qualification and are not inserted as token-priced
chat models. Existing provider rates, channel overrides, request formats and
deployment configuration are unchanged. No paid live-provider calls were made.

Tests independently pin all nine native/generic public discovery records, token
prices, limits, absence of speculative tariffs/controls, and request/tool
preservation. Old-code assertion failures and complete affected race-test
receipts are retained in the PR qualification evidence. Roll back by reverting
this provider commit; no database migration is needed.
