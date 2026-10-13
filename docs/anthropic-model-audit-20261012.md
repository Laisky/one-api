# Anthropic model audit — 2026-10-12

Claude Opus 5.5 already exists with the correct native tariff, 1M context,
128K output and adaptive-only request policy, but its discoverable metadata
omits the five supported effort levels and default. Add low/medium/high/xhigh/max
and the provider's medium default directly to that existing model definition.
Do not change its prices, add invented latest/dated aliases, or turn effort into
a hard token budget. Native conversion already forwards output_config.effort;
the regression exercises all five levels through actual public conversion.

Sources:
- https://platform.claude.com/docs/en/build-with-claude/effort
- https://platform.claude.com/docs/en/models/opus-5-5/overview
- https://platform.claude.com/docs/en/models/overview
- https://platform.claude.com/docs/en/about-claude/pricing

Current Fable 5.1, Sonnet 5.5 and Haiku 5.5 are already represented. The separate
October 8 Haiku audit retains its input tiers and cache-aware usage accounting;
no blanket Claude-family pricing rewrite is performed. Opus 5.5's 300K output
beta is Batch-only, not the synchronous Messages limit. Bedrock, Vertex and
Microsoft Foundry IDs, availability and tariffs are independent and are not
inferred from native Claude metadata. Per-message effort beta transport is not
newly implemented or advertised by this catalogue correction.

The new public tests pin the effort set/default, clone isolation, all five
input/output/cache-read/cache-write prices, context/output bounds, and converted
wire values. The isolated runner requires old-code assertion failures and full
Anthropic/OpenAI/pricing/format race qualification before publishing. No paid
live API call or production deployment is performed. Revert this provider commit
to restore prior metadata; administrator overrides and database schema remain
unchanged.
