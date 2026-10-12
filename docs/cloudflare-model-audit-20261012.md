# Cloudflare Workers AI model audit — 2026-10-12

Eight source definitions now match the provider's model-specific controls.
GLM 5.2 defaults to max and supports none/high/max plus documented aliases.
GLM 5.3 and Flash use low/high/max (default max); reasoning cannot be disabled,
even when the provider accepts none as an alias for max. Their current exact
context limit is 1,048,576, replacing the older 1,310,720 field.
Kimi K2.6 supports none/high and aliases, default high; K2.7 Code is always-on.
Gemma 4 and GLM 4.7 Flash do not document a top-level effort enum. Nemotron
explicitly uses chat_template_kwargs instead, not reasoning_effort. Remove
invented medium defaults and separately undocumented 8,192 output ceilings.
No new template-field transport support, account restriction, or model ID is added.

## Primary sources

Model cards and schemas checked on October 12:
- https://developers.cloudflare.com/workers-ai/models/glm-5.2/
- https://developers.cloudflare.com/workers-ai/models/glm-5.3/
- https://developers.cloudflare.com/workers-ai/models/glm-5.3-flash/
- https://developers.cloudflare.com/workers-ai/models/kimi-k2.6/
- https://developers.cloudflare.com/workers-ai/models/kimi-k2.7-code/
- https://developers.cloudflare.com/workers-ai/models/nemotron-3-120b-a12b/
- https://developers.cloudflare.com/workers-ai/models/gemma-4-26b-a4b-it/
- https://developers.cloudflare.com/workers-ai/models/glm-4.7-flash/
- https://developers.cloudflare.com/workers-ai/platform/pricing/

## Prices and scope

All numeric tariffs remain unchanged and eight independent input/cache/output
triples are pinned by regression tests. GLM 4.7 Flash's platform rate card says
USD 0.060/M input while the model page says 0.0605/M. Retain the platform tariff
and record the conflict rather than silently changing prices from a rounded or
inconsistent source. Native DeepSeek discounts and holidays are not Cloudflare
prices. Clef-family specialized tasks, audio generation/transcription, image
rendering and batch tariffs are not added as working Chat Completions models;
this adaptor does not implement those specialized upstream endpoint contracts.
Existing historical media metadata is not a new endpoint-support claim.

## Qualification and rollback

The new test verifies public discovery, exact contexts, output-limit unknowns,
effort preservation through actual request conversion and unchanged host tariffs.
The original GLM September test retains exact assertions at the updated values.
The isolated runner must demonstrate old-code assertion failures and complete
Cloudflare/OpenAI/pricing/format package race suites before publishing. See PR
receipts for actual results. No paid provider calls are used. Revert this provider
commit to restore prior metadata; no database, deployment or operator override
changes are involved.
