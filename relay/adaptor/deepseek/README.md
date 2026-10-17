# DeepSeek model defaults

Verified against the official documentation on **2026-10-17**. This is a source metadata update, not a live channel or billing-policy migration.

## API names

`deepseek-flash` is the recommended name for **DeepSeek-V4.1-Flash-1017**. The October 17 upgrade changes neither the API identifier nor the published Flash rates.
`deepseek-v4-flash` and `deepseek-v4-flash-vision-exp` remain temporarily accepted compatibility aliases,
but their former models are retired. All three names have the same current
Flash prices, text/image input support, 1,048,576-token context limit, and
393,216-token maximum output. The `file` modality denotes uploaded **image**
references, not arbitrary document ingestion. Images are billed using upstream
prompt-token usage. Pre-consume estimation conservatively reserves up to **1024
tokens per image** for every Flash alias without downloading image dimensions.
This is not a fixed final image charge: upstream usage, including cache hits,
remains authoritative. There is no image-generation price.

`deepseek-v4-pro` currently serves **DeepSeek-V4-Pro-0813**, with text input only
and the same context/output limits. The September 14 Pro-to-Flash migration was canceled; Pro remains a separate model with Pro prices.

The provider announced discontinuation of `deepseek-chat` and `deepseek-reasoner` for July 24, 2026. They remain excluded from the default catalog. Existing user channel mappings and generic compatibility paths are unchanged; this update neither rewrites legacy identifiers nor silently enables thinking for legacy chat requests. No paid upstream probe of legacy names was made.

## Current token prices

The official English rate card prints **$ per million tokens** without a currency code in the inspected table. The existing gateway ratios are denominated in USD; this update introduces no FX assumption, ratio change, retail price decision, or recalculation of settled usage. The provider amounts below match the existing numeric defaults and are informational.

| Model | Period | Cache-miss input | Cache-hit input | Output |
| --- | --- | ---: | ---: | ---: |
| Flash and its aliases | Off-peak | 0.15 | 0.003 | 0.60 |
| Flash and its aliases | Peak | 0.30 | 0.006 | 1.20 |
| Pro | Off-peak | 0.66 | 0.022 | 1.98 |
| Pro | Peak | 1.32 | 0.044 | 3.96 |

Peak hours are **01:00–04:00 and 06:00–10:00 UTC, Monday–Friday**, with inclusive
starts and exclusive ends, excluding Chinese public holidays. All remaining hours, including weekends and Chinese public holidays in full, are off-peak. The existing schedule contains reviewed 2026 holiday dates; later years need a fresh calendar review.
Base ratios represent current off-peak defaults; billing resolves the schedule
using the request start time, not the wall-clock time at settlement.

The Flash rates above take effect at **2026-09-10 04:00 UTC** (12:00 Beijing time).
Before that instant, the immediately preceding Flash defaults are retained:
**0.22 / 0.007 / 0.66** off-peak and **0.44 / 0.014 / 1.32** peak, ordered as
cache-miss input / cache-hit input / output. This preserves the announced boundary
and in-flight request accounting; it is not a complete archive of earlier prices.
The preceding rates come from the pre-update defaults at commit `e000b8eb`.

## Gateway capability boundary

Both upstream models support Chat Completions, native Responses and Anthropic
formats, JSON output and function tools. The gateway retains its existing
Chat Completions, Responses routing/conversion and Claude Messages conversion;
this refresh adds no API family or native tool integration. Upstream base URLs
are `https://api.deepseek.com` (OpenAI format) and
`https://api.deepseek.com/anthropic` (Anthropic format); this is not a change to
saved channel URLs.

Flash supports JPEG, PNG, GIF and WebP images; Pro is text-only. The Models page
metadata and Modern capability controls distinguish the current API names from
historical or other providers' DeepSeek model identifiers. The two Flash aliases
are compatibility routes to the same model, not separate current releases.

The Modern UI exposes function tools, reasoning effort and Flash vision. Its
generic Anthropic budget-based thinking toggle remains unavailable for these
native names: omitting `thinking` does not disable DeepSeek's default thinking.
Use an API request with an explicit `thinking.type` when switching modes; no
request-builder or saved playground default is changed by this refresh.

Thinking is enabled by default with effort `high`; explicitly disable it with
`thinking.type=disabled`. Reasoning aliases remain `minimal`/`low` to `low`,
`medium`/`high`/`xhigh` to `high`, and `max`/`ultra` to `max`. Temperature and
frequency/presence penalties have no effect in thinking mode. Sampling details,
especially `top_p`, are model-specific and remain provider-validated. Forced
`required` or named tool choice is unsupported in thinking mode. JSON mode also
requires an instruction to produce JSON. Output defaults depend on thinking and
effort; the 393,216-token limit is a ceiling, and input plus output must fit the
context. These are upstream constraints, not newly imposed local defaults.

Native Responses is stateless upstream: persisted conversations,
`previous_response_id`, `store`, and `background` are unsupported. Pro supports
upstream `web_search`, while Flash ignores it. The gateway's current default
built-in policy remains empty and does not advertise this as an enabled or free
native integration; user function tools retain their existing behavior. Beta
prefix completion and non-thinking FIM (4K output) are upstream features, not
newly qualified gateway capabilities in this change.

## Replayed history contract

DeepSeek validates whole-conversation structure before it looks at the prompt.
Two rules decide whether replayed agent history is accepted at all, both verified
live against `deepseek-flash` on 2026-09-18:

- Every `tool_call_id` in an assistant message must be answered by one of the tool
  messages directly following it, even when that assistant message is the last one
  (OpenAI accepts that as a prefill; DeepSeek answers `An assistant message with
  'tool_calls' must be followed by tool messages responding to each 'tool_call_id'`).
  IDs must also be unique inside one `tool_calls` array.
- Thinking mode is the default, and every assistant message positioned after the
  last `user` message must carry a `reasoning_content` field unless it carries
  `tool_calls`; an empty string satisfies it. A missing field fails with
  ``The `reasoning_content` in the thinking mode must be passed back to the API``,
  whether or not the conversation uses tools. Earlier turns are exempt.

`deepseekcompat.EnforceHistoryContract` repairs both without inventing tool results
or overwriting replayed thinking, and runs on every DeepSeek-contract path
(dedicated channel, Claude Messages conversion, and the Responses -> Chat
fallback). `relay/adaptor/openai` carries the live suite that pins this against the
real endpoint; it is skipped unless a key is supplied:

```sh
ONEAPI_DEEPSEEK_LIVE_KEY=sk-... go test ./relay/adaptor/openai/ -run DeepSeekLive -v
```

## Verification

Run with the repository's required Go toolchain:

```sh
go test -race ./relay/adaptor/deepseek ./relay/adaptor/openai ./relay/controller ./relay/model ./relay/tooling ./relay/pricing
go test -race ./...
```

Tests cover the catalog and aliases, modalities, unsupported capabilities,
reasoning aliases, both pricing resolution paths, all UTC peak boundaries,
weekends, holidays, caller timezones, the Flash transition and canceled Pro transition without mutating defaults. Catalog regressions pin the October 17 Flash version while retaining aliases and the distinct Pro release.
`TestDeepSeekReviewPricingNotice` additionally checks final quota calculation for
cache misses, cache hits, output-only and mixed usage, including the nanosecond
before, exact instant of, and nanosecond after both Beijing-noon changes.
Behavioral review regressions also cover native routing, every Flash image source,
no-fetch reservations, actual-usage final charging, validating JSON boundaries,
all alias sampling paths, and unsupported built-ins versus user function tools.
Qualify the affected packages with the commands above; see `docs/ci-testing.md` for automatic versus manual coverage.

## Official sources

- [Release notes](https://api-docs.deepseek.com/updates/)
- [Chat Completions API](https://api-docs.deepseek.com/api/create-chat-completion/)
- [Models and pricing](https://api-docs.deepseek.com/quick_start/pricing/)
- [Thinking mode](https://api-docs.deepseek.com/guides/thinking_mode/)
- [Vision](https://api-docs.deepseek.com/guides/vision/)
- [Responses API](https://api-docs.deepseek.com/guides/responses_api/)
- [Current Responses API reference](https://api-docs.deepseek.com/api/create-response/)
