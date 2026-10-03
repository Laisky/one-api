# Ali trial/legacy model pricing admission

Provider trial eligibility is not an unlimited customer quota policy. The six exceptional zero-default models below require explicit channel-owned input and output prices before an upstream request can be sent:

- `qwen-audio-chat`, `qwen-audio-turbo`, `qwen2-audio-instruct`.
- `qwen2.5-0.5b-instruct`, `qwen2.5-1.5b-instruct`, `qwen2.5-math-1.5b-instruct`.

Configure the actual upstream model name after mapping in the channel's unified `model_configs`. Set positive finite `ratio` and `completion_ratio` values; output price is their product. Use the channel's intended customer tariff and current provider contract, not an invented default attributed to Alibaba. Every active token tier must contain complete positive prices. Request-time overlays are resolved by the same resolver used for settlement.

Missing, zero, incomplete or alias-only pricing is rejected before REST dispatch. No upstream work is performed and the controller's existing reservation/refund behavior remains in charge. This is an intentional admission change for these six models; other priced Qwen models are unaffected. Administrators must configure these entries before upgrading when their channels expose them.

The current split model catalog is preserved. The old PR's edits to removed `constants_qwen_closed.go` / `constants_qwen_open.go` files and arbitrary internal floors are superseded by this explicit policy and actual HTTP dispatch regressions.
