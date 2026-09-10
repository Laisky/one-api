# DeepSeek model defaults

Verified against the official documentation on **2026-09-10**.

## API names

`deepseek-flash` is the recommended name for **DeepSeek-V4.1-Flash**.
`deepseek-v4-flash` and `deepseek-v4-flash-vision-exp` remain accepted aliases,
but their former models are retired. All three names have the same current
Flash prices, text/image input support, 1,048,576-token context limit, and
393,216-token maximum output. The `file` modality denotes uploaded **image**
references, not arbitrary document ingestion. Images are billed using upstream
prompt-token usage. Pre-consume estimation conservatively reserves up to **1024
tokens per image** for every Flash alias without downloading image dimensions.
This is not a fixed final image charge: upstream usage, including cache hits,
remains authoritative. There is no image-generation price.

`deepseek-v4-pro` currently serves **DeepSeek-V4-Pro-0813**, with text input only
and the same context/output limits. Its announced transition is described below.
The retired `deepseek-chat` and `deepseek-reasoner` names remain excluded.

## Current token prices

All values are **USD per million tokens**.

| Model | Period | Cache-miss input | Cache-hit input | Output |
| --- | --- | ---: | ---: | ---: |
| Flash and its aliases | Off-peak | 0.15 | 0.003 | 0.60 |
| Flash and its aliases | Peak | 0.30 | 0.006 | 1.20 |
| Pro before its transition | Off-peak | 0.66 | 0.022 | 1.98 |
| Pro before its transition | Peak | 1.32 | 0.044 | 3.96 |

Peak hours are **01:00–04:00 and 06:00–10:00 UTC, Monday–Friday**, with inclusive
starts and exclusive ends. All remaining hours, including weekends, are off-peak.
Base ratios represent current off-peak defaults. These configurations are not a
historical price archive; retired price schedules are not retained for replay.

At **2026-09-14 04:00 UTC** (12:00 Beijing time), DeepSeek will route the Pro name
to V4.1 Flash and charge Flash rates, until V4.1 Pro is released. The adapter
encodes this pricing change with the existing first-match time-window mechanism;
it neither rewrites the requested model nor switches prices early. Pro capability
metadata describes the currently available model; the scheduled overlay changes
pricing only. Review the catalog when the future V4.1 Pro release is announced;
no unannounced model name, release date, or future price is assumed.

## Capability corrections

Function tools remain supported. The current Responses API ignores built-in
`web_search`, so the model defaults no longer advertise native search or a
zero-cost built-in search policy. This does not remove user-defined search functions.
Old V4 open-weight IDs and quantization claims are not carried over to V4.1 aliases.

Reasoning defaults to `high`. The documented mapping is `minimal`/`low` → `low`,
`medium`/`high`/`xhigh` → `high`, and `max`/`ultra` → `max`.
Temperature has no effect in thinking mode. In thinking mode `top_p` has a 0.95
floor; in non-thinking mode it is fixed at 1.0. The gateway leaves these controls
to the provider rather than introducing local sampling rules.

## Verification

Run with the repository's required Go toolchain:

```sh
go test -race ./relay/adaptor/deepseek ./relay/adaptor/openai ./relay/controller ./relay/model ./relay/tooling ./relay/pricing
go test -race ./...
```

Tests cover the catalog and aliases, modalities, unsupported capabilities,
reasoning aliases, both pricing resolution paths, all UTC peak boundaries,
weekends, caller timezones, and the exact Pro transition without mutating defaults.
Behavioral review regressions also cover native routing, every Flash image source,
no-fetch reservations, actual-usage final charging, validating JSON boundaries,
all alias sampling paths, and unsupported built-ins versus user function tools.
The DeepSeek regression workflow runs these and the complete affected packages.

## Official sources

- [Models and pricing](https://api-docs.deepseek.com/quick_start/pricing/)
- [Thinking mode](https://api-docs.deepseek.com/guides/thinking_mode/)
- [Vision](https://api-docs.deepseek.com/guides/vision/)
- [Responses API](https://api-docs.deepseek.com/guides/responses_api/)
- [Current Responses API reference](https://api-docs.deepseek.com/api/create-response/)
