# Cross-channel catalog, request, and image billing contract audit

## Scope

This audit extends PR #417 beyond its initial xAI catalog refresh. It checks the registered channel registry and the shared paths used for reasoning query defaults, native Responses payload construction, controlled `extra_body` forwarding, image request preparation, and image pricing resolution.

It is not a claim that every feature in the repository, every provider endpoint, or every published price has been independently verified. The tests are deterministic repository contract tests. They do not make billable provider inference requests. Native Anthropic and Bedrock reasoning use a separate budget-oriented path and are excluded from the discrete query-effort matrix.

## Reproduced baseline

Tests-only commit: `a9c94985c3d0ac9b769d8f88869f84caf09fd48f`.

GitHub Actions run: https://github.com/Laisky/one-api/actions/runs/35663475983

The `packages` shard failed while static checks passed. Its archived `events.jsonl` distinguishes test failures from compilation failures and parent-suite summaries.

| Contract | Exercised cases / verified failing leaf cases |
| --- | --- |
| Reasoning query injection | 60 channel IDs; 1,074 model/channel pairs; 3,848 advertised efforts; 7,696 Chat/Responses cases; 1,672 failing cases |
| Channel-to-API adaptor fallback | 60 failing cases |
| Typed reasoning to final Responses JSON | 12 failing cases |
| Normalized image tier to provider JSON | 7 failing cases |
| Request-default-only image override | 2 failing cases |

These are 1,753 failing input combinations, not 1,753 independent bugs. Additional controls cover typed extension injection, caller precedence, null-object handling, immutable conversion snapshots, resolution/billing agreement, and native xAI Responses reasoning.

## Fixes and invariants

### Selected provider capabilities drive query defaults

`thinking_catalog.go` resolves capabilities from the actual channel and mapped model. Every explicitly advertised effort remains available through both query-injection paths. An explicit body value wins over query defaults. Price-only operator overrides do not erase capability metadata. Unknown custom models retain the existing fallback; another provider's global pricing entry cannot impose an API vocabulary.

### Channel IDs are not API IDs

`resolvePricingAdaptor` translates a fallback channel through `channeltype.ToAPIType` and binds `ChannelTypeAware` before pricing. Image pricing now uses this channel-aware resolver before reading defaults, rather than waiting until request dispatch to initialize provider selection.

### Normalized values reach the wire

`response_io.go` synchronizes typed reasoning effort and summary regardless of whether summary normalization changed. Merging uses `json.RawMessage` and preserves unknown nested fields and integers above JavaScript's exact-integer range. Normalization is idempotent. A JSON `null` root returns an error instead of causing a nil-map panic.

Native xAI Responses bypasses chat request conversion, so its known-model effort check is performed on that path as well. Other providers and unknown xAI models are not restricted by xAI catalog rules.

### Controlled extensions retain caller precedence

The passthrough merger collects typed `extra_body` defaults before deleting the transport wrapper. Responses query-added Qwen/vLLM thinking settings therefore reach the upstream payload. Explicit root or original extension values retain precedence; disallowed routing and billing keys cannot be injected through `extra_body`.

### Image validation, conversion, and billing agree

Configured image sizes and qualities use the same canonical values before validation and conversion. Validation uses the same endpoint-effective quality as billing. Known xAI `resolution` selectors are reconciled with the billing size before defaults and quota reservation; conflicting selectors and unrepresentable sizes return a client error. The currently documented resolution enum is `1k` / `2k`; historical intermediate tariff rows are not translated into an invented `1.5k` selector.

Every controller-managed image converter receives an independent request copy, including the optional string pointers. Provider conversion can no longer clear the size/quality keys used by channel overrides or billing logs. Unknown, unconfigured image models retain their provider-specific values.

Source for xAI wire behavior: https://docs.x.ai/developers/model-capabilities/images/generation

### Request defaults and tariffs have distinct precedence

`image_resolver.go` overlays request-only channel settings on the resolved provider/global tariff, after applying its dated window. An explicit channel tariff still replaces billing metadata; it inherits missing request defaults, not an unexpected extra provider price multiplier. Every returned configuration is private, and neither provider catalogs nor persisted channel settings are mutated.

## False-positive controls

The original broad diagnostic found 59 additional failing catalog assertions. They were investigated rather than converted into unsupported model exposure changes.

- Azure merges OpenAI and Anthropic pricing fallbacks but deliberately advertises a selected Foundry routing catalog (`relay/adaptor/azure/constants.go`).
- Bedrock advertises its actual adaptor registry, not every legacy pricing key (`relay/adaptor/aws/adaptor.go`).
- Groq retains historical and enterprise-compatible tariffs outside its current public list (`relay/adaptor/groq/constants.go`).
- A fixed informational reasoning default does not imply that the effort is user-tunable. Membership is required when a non-empty effort vocabulary is advertised.

The catalog test retains strict price/list consistency for table-driven providers and validates advertised effort defaults. It does not conflate a billing fallback with provider routing support.

## Regression suites

- `systematic_catalog_contract_test.go`: channel registry, full query-effort matrix, final JSON, tier normalization, sparse defaults, and catalog invariants.
- `systematic_wire_controls_test.go`: caller precedence, unknown extensions, Qwen/vLLM wire settings, custom models, and null input.
- `systematic_image_overrides_test.go`: explicit tariff replacement, dated pricing through sparse overrides, and copy isolation.
- `systematic_image_provider_test.go`: real xAI conversion, resolution/size conflicts, channel tier keys, and destructive-converter isolation.
- `systematic_response_provider_test.go`: native xAI Responses effort matrix and provider isolation.

Run focused checks with:

```sh
go test -race ./relay/controller -run '^TestSystematic' -count=1
go test -race ./relay/pricing ./relay/adaptor/xai -count=1
go vet ./...
```

The existing CI runs the full package/model shards with race detection and atomic coverage, then checks test completeness. No new workflow, scheduled job, or CI gate is introduced. Final head-specific execution results belong in the PR verification comment; the failed baseline remains available as the negative control.
