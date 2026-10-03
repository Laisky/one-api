# Historical Grok tariff admission

The seven historical slugs targeted by this review are `grok-3-fast`, `grok-3-mini-fast`, `grok-2-1212`, `grok-beta`, `grok-2`, `grok-2-latest`, and `grok-vision-beta`. A current public tariff and redirect destination for each of these were not established from the provider documentation. A documentation-page redirect is not proof of an API redirect or of its invoiced price.

These slugs therefore require explicit channel `model_configs` entries keyed by the actual upstream model name after mapping. Supply positive finite `ratio` and `completion_ratio` values, with output price equal to their product, and complete positive prices for active token tiers. Obtain the intended customer tariff from the relevant deployment contract. Request-time overlays use the same resolver as settlement. Missing, zero, incomplete, or alias-only prices are rejected before any upstream work.

Do not substitute the ordinary Grok 3/Grok 3 Mini tariffs for the historical Fast tariffs, or publish a guessed historical price as a verified current default. The previous PR's speculative catalog additions are superseded by this admission rule. This is an intentional migration requirement for administrators still exposing these names.

The existing, documented `grok-3` redirect to `grok-4.3` retains its current input/output/cache/tier pricing. Source: https://docs.x.ai/developers/migration/may-15-retirement . The regression suite checks complete tariff equivalence and exercises real HTTP dispatch for each exceptional slug, with both denied and correctly priced positive controls.
