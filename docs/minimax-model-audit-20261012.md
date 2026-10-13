# MiniMax catalogue audit — 2026-10-12

Sources:

- https://platform.minimax.cn/docs/guides/models-intro
- https://platform.minimax.cn/docs/guides/pricing-paygo
- https://platform.minimax.cn/docs/api-reference/text-openai-api

The eight current/pay-as-you-go M-series entries already have the published
Standard CNY rates. Preserve those rates, cache-write fees, long-context threshold,
limits and IDs. Split current and legacy complete definitions with duplicate-ID
rejection instead of introducing another initialization-time price override.

Correct M2.x thinking metadata: it cannot be disabled; accepting a disabled
parameter does not mean it changes behavior. M3 allows disabling thinking but
has no tunable reasoning_effort. Add max_completion_tokens and stop advertising
ignored penalties or unverified seed/stop controls for these current models.

M2.5 and M2.7 families close to new PAYG users on October 19 and retire November
18. Subscription requests are separately rerouted on October 19; do not rewrite
ordinary PAYG requests or prices. M3.1 Flash Preview is currently subscription /
MiniMax Code only. Its low/medium/high/xhigh/max efforts must not be inherited by
M3 or M2, and no PAYG price is guessed. M3's permanent discount is not given an
invented expiry. Priority requests are 1.5x Standard, not another model ID.

## Endpoint boundary

The existing native MiniMax Adaptor is a stub: request and response conversion
are not implemented. These catalogue changes do not claim working native chat,
Responses, Claude, TTS, ASR, image, or H3 video integration. Do not advertise new
media IDs as functional default endpoints. H3 billing additionally includes input
video and image fees; output-seconds alone would undercharge. Speech charges use
provider character counts, not ordinary language-model token counts.

The published server web_search fee is CNY 0.03/call. Native tool transport is not
implemented, so the empty tooling default is retained with an accurate comment.
Historical legacy prices are unchanged and are not newly verified live offers.

Regression tests cover discovery, current Standard prices, cache-write rates,
long-context boundary, effective controls, lifecycle distinctions and constructor
isolation. No provider calls are made. See PR #560 for executed qualification.
Rollback by reverting this provider commit; no database or deployment change.
