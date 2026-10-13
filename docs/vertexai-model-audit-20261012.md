# vertexai model audit — 2026-10-12

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

Google's own global Standard table independently publishes the same Haiku
rates: input/output 0.10/0.50 through 100K and 0.50/2.50 above 100K; cached input
0.01/0.05, 5m writes 0.125/0.625 and 1h writes 0.20/1.00, USD per million tokens.
Use a complete independent definition with conservative existing Claude features,
not a native-price assumption. The entire prompt, including cache buckets,
selects the tier. Regional/multi-region rates have a separate 10% premium.
The existing Sonnet 5.5 cache default is corrected from 0.20 to the independently
published 0.10; other Sonnet prices and its wire regression remain unchanged.
No unpublished transition timestamp is invented for that current-default fix.

The new regression exercises production tier resolution at 99,999, 100,000,
100,001 and 1,000,000 input tokens and checks nested allocation isolation.
The isolated runner must show baseline assertion failures and pass the complete
affected package race suites before publishing. Existing native Claude usage
accounting and conversion suites remain in the qualification set. No live model
call or deployment is performed. Revert the provider commit for rollback; no
schema, credentials or existing channel overrides are changed.
