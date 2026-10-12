# Adaptor model research checkpoint — 2026-10-12

## Scope and safety

This is a partial all-adaptor audit, not an all-provider completion claim.
Research uses the provider's own API documentation, not an aggregator's tariff.
Each completed provider change is committed separately. Existing model IDs,
channel overrides, request conversion and historical pricing are preserved unless
a source-backed change explicitly says otherwise. Audit status does not imply
live-provider qualification.

## Committed provider checkpoints

| Provider | Commit | Implemented scope |
| --- | --- | --- |
| Mistral | `3966bd75d2c7b6a15e3f55605accdfea8408aa64` | Three lifecycle descriptions; no numeric tariff changes |
| Cohere | `594c28009d7c6fa1b35c2d6b5b2f37fdfbfecda5` | Four explicitly free, rate-limited API tariffs and regression tests |
| Gemini shared catalogue | `bd76940fdcf0e1863d70739e3661aecbcdea3dd4` | Three October lifecycle descriptions; shared/native regression tests |

See `cohere-model-audit-20261012.md` and `gemini-model-audit-20261012.md` in this
directory for their evidence, billing boundaries, and remaining integration gaps.

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
- Cerebras' public catalogue already matches the two current model IDs and
  their published base prices and limits. No catalogue edit was justified.
  Sources: https://api.cerebras.ai/public/v1/models and
  https://inference-docs.cerebras.ai/models/overview

These are scoped comparisons, not a full audit of every capability, legacy ID,
pricing schedule, or live account entitlement for those providers. Remaining
adaptors still require individual source-to-code review; no blanket approval is
inferred from an existing September or October audit comment.

## Executed validation

The complete Mistral, Cohere, shared Gemini, native Gemini, and Vertex package
suites passed with Go 1.27.2 at `bfc5d0288b2d926ae9479f2ee118bd380fb29987`:

```sh
go test -count=1 -timeout=5m \
  ./relay/adaptor/mistral \
  ./relay/adaptor/cohere \
  ./relay/adaptor/geminiOpenaiCompatible \
  ./relay/adaptor/gemini \
  ./relay/adaptor/vertexai
```

Receipt: https://github.com/Laisky/one-api/actions/runs/38272009063/job/114888567875

The subsequent cleanup changes only documentation, Go whitespace, and removes
the temporary audit workflow. It does not change the tested production code.
The temporary workflow violated the repository's two-workflow policy and is
removed rather than weakening that policy. There is no final workflow change
relative to the base branch. The original quick-test selection and retained
manual full-qualification commands remain unchanged.

Required automatic CI must pass on the final head. The package receipt above is
not a claim of full-repository race testing, a complete billing-pipeline test,
pre-fix negative-control execution, or live-provider calls. No such qualification
was performed for this checkpoint. See the PR's checks for final-head CI results.
