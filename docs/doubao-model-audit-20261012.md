# Doubao / Ark Standard catalogue audit — 2026-10-12

Sources: https://www.volcengine.com/docs/82379/1330310 and
https://www.volcengine.com/docs/82379/1544106 (model IDs versus pricing labels).

Ten exact chat IDs are added: Seed Evolving, four dated Seed 2.1 IDs, two hosted
GLM IDs, two DeepSeek V4 GA IDs and Seed Translation. Host CNY per-million
Standard prices are independent of creator prices, Batch, low-latency and
low-priority offers. The Flash GA pre-August21 price is not revived. Legacy
undated catalogue entries and their historical tariffs remain unchanged; they
are not silently remapped to the newer model card. Advisory K limits retain the
existing decimal-thousand convention and introduce no MaxTokens admission cap.

Only non-audio input is advertised for Seed 2.1 Lite. The native audio tariff is
CNY12 input/2.4 cached per million; cached-audio attribution needs separate
settlement support, not the text rate. Audio, cache storage, image/video generation
and alternative service tiers are outside these chat defaults. No such billing
capability is implied by the model ID. GLM models do not inherit creator effort
levels or structured-output claims absent from Ark's table.

DeepSeek V4.1 Flash remains excluded from built-in defaults: Ark chooses peak/
off-peak pricing by REQUEST COMPLETION TIME, whereas the common time-window
resolver uses request start time. Adding start-time windows would misbill requests
crossing a boundary. Native DeepSeek and TokenHub use different pricing rules;
none can safely be copied here. Operators can configure explicit channel rates.

Behavior tests cover actual OpenAI-compatible dispatch, /api/v3/chat/completions,
exact model IDs, preserved output budgets, host tariffs, exclusions, constructor
ownership and historical Seed 1.6 boundary integrity. Publish only after baseline
assertion failures and affected-package race tests. No upstream calls, database
migration or deployment changes. Revert this provider commit to roll back.
