# Adaptor model research checkpoint — 2026-10-12

## Scope and safety

Research uses the provider's own API documentation, not an aggregator's tariff.
Each completed provider change is committed separately. Existing model IDs,
channel overrides, request conversion and historical pricing are preserved unless
a source-backed change explicitly says otherwise. Audit status is not a claim
that a live API call or a full test suite has passed.

## Mistral lifecycle checkpoint

Sources inspected on 2026-10-12:

- https://docs.mistral.ai/resources/changelogs
- https://docs.mistral.ai/inference/pricing
- https://docs.mistral.ai/models/mistral-large-4-0

Changes:

- `zai-glm-5-3` became Generally Available on 2026-09-28; it is no longer described
  as a public preview. The Mistral-hosted USD 1.40 / 0.14 / 4.40 input / cached
  input / output tariff per million tokens is unchanged.
- `zai-glm-5-2` is deprecated, with retirement scheduled for 2026-10-31. The
  provider recommends `zai-glm-5-3` at the same tariff.
- `labs-leanstral-1-5` retired on 2026-09-30. Preserve its existing metadata and
  historical free tariff for compatibility, but explicitly identify it as retired.

`TestMistralOctoberLifecycle` asserts these descriptions through the public
adaptor pricing method and independently pins the pre-existing numeric prices.

### Deliberately unresolved

Mistral Large 4 (`mistral-large-4`) launched on 2026-10-06 in public preview.
Its current USD prices are 0.68 / 0.07 / 2.09 per million tokens; the standard
prices are 1.36 / 0.14 / 4.18. The changelog says the launch discount lasts two
weeks but the inspected pages do not state the precise expiry instant/timezone.
Do not silently make this discount permanent or invent a billing switch instant.
Adding this model with a safely qualified promotional schedule remains pending.
No `mistral-large-latest` remapping or unsupported OCR transport is introduced.

## Other verified observations, not completion claims

- DeepSeek's current public rate card matches the existing Flash/Pro base rates;
  the preceding pre-V4.1 rates must not overwrite them.
  Source: https://api-docs.deepseek.com/quick_start/pricing/
- Grok 4.7 is already installed by `xai/catalog_refresh_20260921.go`; looking only
  at `xai/constants.go` produces a false missing-model report.
  Source: https://docs.x.ai/developers/models

## Validation status

The added regression test is gofmt-formatted. A full repository checkout and
Go 1.27 test environment are not available in the current container; no executed
package-test, vet, race or live-provider qualification is claimed at this checkpoint.
Run `go test ./relay/adaptor/mistral` and the repository's required CI gates before
merging. This checkpoint does not complete the all-adaptor audit.
