# azure model audit — 2026-10-12

## Verified Haiku 5.5 admission

Expose the official undated `claude-haiku-5-5` ID through the existing native
Messages integration, not a guessed dated alias or generic Chat Completions
endpoint. Test public provider discovery, the real publisher/deployment URL,
1M context, 128K output, five effort levels and the medium default.

Sources:
- https://learn.microsoft.com/en-us/azure/foundry/foundry-models/concepts/claude-models
- https://platform.claude.com/docs/en/about-claude/pricing#claude-in-microsoft-foundry-pricing
- https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai
- https://cloud.google.com/gemini-enterprise-agent-platform/generative-ai/pricing

Foundry now lists Haiku 5.5 as GA on both hosting versions. Its Anthropic-hosted
CCU pricing explicitly rates tokens at native Claude Standard rates, so reuse
the existing per-family pricing dispatch, including the >100K prompt tier.
This addition changes discovery only: it does not assume a chosen deployment
name, issue Entra tokens, change API keys, or claim all organizations can deploy.
US Data Zone and negotiated discounts require operator overrides.

The new regression exercises production tier resolution at 99,999, 100,000,
100,001 and 1,000,000 input tokens and checks nested allocation isolation.
The isolated runner must show baseline assertion failures and pass the complete
affected package race suites before publishing. Existing native Claude usage
accounting and conversion suites remain in the qualification set. No live model
call or deployment is performed. Revert the provider commit for rollback; no
schema, credentials or existing channel overrides are changed.
