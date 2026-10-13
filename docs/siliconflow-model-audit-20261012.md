# SiliconFlow model audit — 2026-10-12

Source: https://www.siliconflow.cn/pricing

The public provider rate card now lists `zai-org/GLM-5.3-Flash`, absent from the
existing catalogue. Add its CNY-per-million input / cache-hit / output prices:
0.80 / 0.23 / 2.80. This is a paid hosted model, not a free Flash-family model,
and not Z.AI's USD price. The model uses the existing compatible text-chat route.

The linked model detail page redirected to authentication. Do not import the
creator's context limit, modalities, precision, reasoning controls, or promotion
as if they were verified SiliconFlow deployment settings. Unknown limits stay
zero (unspecified); only the conservative text-chat path is advertised. Existing
operator overrides and request admission behavior remain unchanged.

The same page's visible GLM 5.3/5.2, Hy4, LongCat 2.0, Qwen 3.8 27B, Kimi 2.7
Code, BGE and ordinary image prices already match the corresponding definitions.
The page collapses additional models and does not prove every legacy model's
availability. No such completeness claim is made and no legacy ID is removed
solely because it was hidden in a collapsed section. Existing video, image,
character, time-window and per-call billing units remain untouched.

Tests independently check native-currency input/cache/output amounts, discovery,
request preservation, unchanged full GLM 5.3 prices, and absence of invented
limits or promotions. See PR #560 for executed test receipts. No paid upstream
call was made. Rollback is a revert of this provider commit.
