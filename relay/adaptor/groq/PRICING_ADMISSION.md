# Groq exceptional pricing admission

The catalog describes provider capabilities and verified public tariffs; it is not an authorization to give away upstream trial or contract usage. A missing public tariff must not be replaced by a guessed nonzero price.

`minimaxai/minimax-m2.7` requires an administrator-configured tariff in the channel's unified `model_configs`, keyed by the actual upstream model name after mapping. Supply positive finite `ratio` and `completion_ratio` values, where output price equals their product. Active request-time overlays are applied with the normal pricing resolver. Every active token tier must also contain complete positive prices. Consult the channel contract; this change does not prescribe a provider price.

`groq/compound` and `groq/compound-mini` were decommissioned on September 21, 2026 according to https://console.groq.com/docs/deprecations. They are rejected even when a channel contains an old tariff. Configure a supported replacement instead. Do not restore retired tooling tariffs as if the retired products were available.

The check runs before shared REST dispatch and before the possibly-forwarded marker. Rejected requests perform no upstream work; controller-owned reservation/refund behavior remains unchanged. Other models and explicit unmetered proxy routes are not reclassified by this policy.

The transport regression matrix covers absent/zero/incomplete prices, alias-only mismatches, retired models, and a correctly priced positive control. The previous catalog-only assertions were replaced because a positive number alone does not prove safe billing.
