# Amazon Bedrock model audit — 2026-10-12

AWS explicitly announced a 50% Sonnet 5.5 cache-read price reduction starting
October 7, 2026. Apply that to the existing USD 0.20/M default, yielding 0.10/M.
Other Sonnet input/output and cache-write defaults retain their existing
owner-approved prices; this is not a blanket inference of native-provider
pricing parity. No exact change instant was published in the inspected notice,
so no fabricated timezone-specific historical schedule is added.

AWS's Opus 5.5 card independently documents low/medium/high/xhigh/max effort,
medium default, always-on adaptive thinking, 1M context and 128K output. Fill
these missing effort fields in the existing Bedrock definition. Preserve AWS's
lack of native structured output and the distinct Sonnet high default.

Primary sources checked on October 12:
- https://aws.amazon.com/bedrock/pricing/
- https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-opus-5-5.html
- https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-sonnet-5-5.html
- https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-haiku-5-5.html

## Explicit admission boundary

Haiku 5.5's undated model ID and global cross-region profile are published by
AWS. The inspected dynamic pricing table did not provide extractable Haiku
numeric rates. Do not silently use native Claude prices or invent an in-region
Invoke route; the model card does not offer that route. Haiku profile mapping
and native cache-tier billing still need a separately verified Bedrock tariff
before a working default is admitted. Bedrock Mantle-hosted models also require
a different endpoint family; their presence in the overall product catalogue
does not make them supported by this existing adaptor.

The new test pins both complete price tuples, accepted effort values, defaults,
context/output limits, backend-specific exclusions and clone isolation. Existing
Sonnet request/routing regressions retain their assertions with the intentional
cache-price correction. The isolated runner requires baseline assertion failures
and complete AWS/pricing/format race suites before publication. No paid model
calls, IAM changes or deployment occurred. Revert this provider commit to roll
back; administrator channel overrides remain unchanged.
