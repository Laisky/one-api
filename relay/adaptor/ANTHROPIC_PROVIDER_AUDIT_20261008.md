# Anthropic model audit — October 8, 2026

## Scope and primary sources

This update targets the native Anthropic adaptor's public model catalog, standard
Messages pricing, and the request/billing behavior needed by Claude Haiku 5.5.
Existing model IDs and administrator pricing overrides are retained. Adding a
catalog entry is not a guarantee of account or cloud-region entitlement.

Official sources reviewed on October 8, 2026:

- [Model catalog](https://platform.claude.com/docs/en/models/overview).
- [Standard pricing](https://platform.claude.com/docs/en/about-claude/pricing).
- [Haiku 5.5 specifications](https://platform.claude.com/docs/en/models/haiku-5-5/overview).
- [Haiku migration](https://platform.claude.com/docs/en/models/haiku-5-5/migration-guide).
- [Effort compatibility](https://platform.claude.com/docs/en/build-with-claude/effort).
- [Sonnet 5.5 specifications](https://platform.claude.com/docs/en/models/sonnet-5-5/overview).
- [Lifecycle table](https://platform.claude.com/docs/en/about-claude/model-deprecations).

## Catalog and standard tariffs

Haiku 5.5, released October 7, is added using the official undated ID
`claude-haiku-5-5`. It accepts text, images and document inputs and produces text.
The ordinary context/output limits are 1,000,000/128,000 tokens. The 300K output
limit belongs to a separate Batch beta and is not advertised for Messages.
Adaptive thinking defaults to medium effort; all five effort levels are listed.
No dated, `-latest`, Bedrock-prefixed or Vertex-suffixed aliases are invented.

Prices below are USD per million tokens. The base tier includes exactly 100,000
prompt tokens. Above that boundary, the upper tariff applies to all buckets,
including output, rather than only to tokens above the threshold.

| Model / prompt length | Input | Output | Cache read | 5m write | 1h write |
| --- | ---: | ---: | ---: | ---: | ---: |
| Haiku 5.5, <=100K | 0.10 | 0.50 | 0.01 | 0.125 | 0.20 |
| Haiku 5.5, >100K | 0.50 | 2.50 | 0.05 | 0.625 | 1.00 |
| Sonnet 5.5 | 2.00 | 10.00 | **0.10** | 2.50 | 4.00 |

Sonnet 5.5 previously charged $0.20/MTok for cache reads. The published rate is
$0.10, or 5% of ordinary input. Sonnet 5 remains $0.20; Opus 5.5 remains $0.20;
Fable/Mythos 5.1 remain $0.25. This is not a blanket cache multiplier change.
Batch, fast-mode, data-residency, regional and contractual adjustments are not
folded into standard model defaults. Existing paid-tool admission is unchanged.

## Cache-aware tier selection

The existing Claude usage receipt keeps `input_tokens` separate from
`cache_read_input_tokens` and cache-write buckets. Previously `quota.Compute`
selected tiers using only `PromptTokens`, even though charging correctly treated
Claude cache buckets separately. A receipt with 1 new input token and 100,000
cached tokens would therefore use the cheaper Haiku tier incorrectly.

Tier selection now adds the nonnegative cache read, 5m write and 1h write counts
for Claude models, using the same Claude-name convention as existing charging.
The sum saturates on integer overflow. Charged token buckets and receipt fields
are not rewritten. Non-Claude inclusive prompt counts remain unchanged. Both
normal pricing and the legacy flat-input-override tier path use the full count.
This correction also applies to existing Claude models with configured tiers.

## Request compatibility

Chat conversion and final prepared native Messages bodies recognize Haiku 5.5
independently of Sonnet 5.5. Legacy `enabled` thinking is normalized to adaptive;
manual budgets and unsupported sampling controls are removed. Omitted thinking
stays omitted, preserving upstream defaults. Generic reasoning effort translates
through the existing output-config path.

Haiku preserves explicit disabled thinking at low/medium/high effort and permits
forced tool choice. It does not inherit Sonnet's `between_tools` conversion or
forced-tool prohibition. Disabled thinking with xhigh/max, unsupported thinking
modes, assistant prefill, and effort changes while thinking is disabled fail
before dispatch. Native message blocks, signatures, exact integer values and
unrelated extensions are preserved by the raw-message boundary.

Azure's existing known-origin compatibility resolution also applies to opaque
Haiku deployment names without changing their wire IDs. Bedrock/Google Cloud
routing, catalogs and region-specific prices are not expanded by this update.
This is not a certification of every new computer/browser toolset or paid beta.

## Lifecycle notes

The current native API table lists Sonnet 4.5 as deprecated, with retirement on
November 30, 2026; it is not yet retired on this audit date. Opus 4.1 retired on
August 5, and Opus 4/Sonnet 4 retired on June 15. Mythos Preview retirement is TBA.
Haiku 4.5 remains active; an earliest-retirement commitment is not a scheduled
shutdown. Historical catalog entries are retained because cloud lifecycles and
administrator mappings differ. This change does not automatically migrate an
existing channel or rewrite all historical model descriptions.

## Regression coverage and validation

Added tests exercise public catalog/pricing resolution at 99,999/100,000/100,001
input tokens; all cache tariffs; exact quota settlement with each cache bucket;
long outputs without long prompts; channel and legacy overrides; overflow and
non-Claude accounting; Chat conversion; native-body preservation; invalid
controls; forced-tool independence; and Azure origin mapping.

The pre-change Sonnet tariff and cache-tier defect were confirmed by source
inspection and encoded as regression assertions. Local validation checks Go
formatting, syntax parsing and the exact Git blob hashes of all six edited
baseline files. The authoring container has Go 1.23.2, cannot resolve GitHub DNS,
and lacks a full checkout/dependencies; the repository targets Go 1.27. Full
project tests and live provider calls were **not** run locally. Added tests must
not be described as passing until CI or a complete checkout executes them.

Acceptance commands:

```sh
go test -race ./relay/adaptor/anthropic ./relay/adaptor/azure ./relay/pricing ./relay/quota
go vet ./...
go test -race ./...
make build-frontend-modern
```
