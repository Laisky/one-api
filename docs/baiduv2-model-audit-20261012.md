# Qianfan v2 catalogue — 2026-10-12

Added exact API parameters glm-5.3-flash, glm-5.3, glm-5.2 and four Qwen3.5
sizes: 397B-A17B, 122B-A10B, 27B, 35B-A3B. Pricing display names are uppercase;
the model-parameter list is lowercase. No silent request rewrite is introduced.
The old GLM-5.3 entry and its finite September holiday windows remain unchanged.

Sources:
- https://cloud.baidu.com/doc/qianfan/s/rmh4stp0j (updated 2026-10-08)
- https://cloud.baidu.com/doc/qianfan/s/wmh4sv6ya
- https://cloud.baidu.com/doc/qianfan-docs/s/Vm8r4fs0q (completion budget)

GLM current CNY per-million input/cache/output: Flash 0.8/0.23/2.8;
5.3 and 5.2 both 8/2/28. Context is 1M; output is exactly 131072 including
reasoning. These are host prices, not native Z.AI or BigModel tariffs.
Qwen3.5 input/output before and after 128K prompt tokens:
397B 1.2/7.2 to 3/18; 122B 0.8/6.4 to 2/16;
27B 0.6/4.8 to 1.8/14.4; 35B 0.4/3.2 to 1.6/12.8.
The advisory defaults explicitly normalize K to 1000, consistent with the
catalogue's existing provider-price convention; no MaxTokens cap is introduced.
Qwen total output is 65536. Unknown host cache/effort/vision/tool controls are
not inherited from Alibaba. No expired holiday discount is made permanent.

Behavior tests exercise the actual OpenAI-compatible adaptor selected for
BaiduV2, its /v2/chat/completions route, exact model passthrough and max_tokens-to-max_completion_tokens normalization,
prices/discovery, 128000/128001 tier boundaries and independent mutable metadata.
The baiduv2.Adaptor conversion stubs are not mistaken for production transport.

GLM-5.2-Fast is not added from a pricing display name alone. GLM-5.1's 32K
equality notation is ambiguous. New OCR/image/embedding/rerank models require
separate dispatch/usage integration rather than a chat-only catalogue entry.
The DeepSeek cache promotion ends October 30 without a published replacement
cached rate; no post-promotion price is fabricated. Existing older ERNIE records
are retained without claiming this commit requalifies their historical tariffs.

Qualification requires baseline assertion failures and affected package race
tests before publishing. No live provider calls or database migration. Revert
this provider commit to remove new defaults; existing identifiers remain intact.
