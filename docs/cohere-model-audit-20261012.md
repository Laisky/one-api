# Cohere model audit checkpoint — 2026-10-12

## Verified correction

These four official model cards explicitly state that public API access is free
within rate limits for both trial and production keys:

| API model ID | Official evidence |
| --- | --- |
| `command-a-plus-05-2026` | https://docs.cohere.com/docs/command-a-plus |
| `command-a-reasoning-08-2025` | https://docs.cohere.com/docs/command-a-reasoning |
| `command-a-translate-08-2025` | https://docs.cohere.com/docs/command-a-translate |
| `command-a-vision-07-2025` | https://docs.cohere.com/docs/command-a-vision |

The existing catalogue used estimated USD 2.50 input / 10.00 output per million
tokens copied from Command A. Replace those estimates with an explicit zero API
tariff and a completion multiplier of one. The model descriptions distinguish
this rate-limited API offering from separately priced Model Vault and negotiated
production deployments. No inference about unlimited free production hosting is
made. No new model IDs, endpoint conversions, account permissions, transport
capabilities, or context limits are introduced by this checkpoint.

The initialization update follows the package's existing catalogue composition
pattern. It clones each existing record and changes only Ratio, CompletionRatio,
and Description. The public adaptor computes discovery directly from ModelRatios,
so there is no separately initialized model list to rebuild. Existing operator
channel overrides are not modified.

## Regression coverage

`TestCommandResearchAPITariffs` covers all four IDs through public discovery and
pricing methods, including the distinction between explicit zero and the
unknown-model fallback. It also checks there are no cached or tiered residual
prices. `TestCommandResearchPricingDoesNotChangePaidModels` independently pins
six neighboring paid models, preventing a blanket free-Command regression.

Both files were gofmt-formatted. Package tests have not been executed in this
container; run `go test ./relay/adaptor/cohere` with the repository toolchain and
required CI gates before merging. No live-provider or complete billing-pipeline
qualification is claimed.

## Remaining catalogue work

The current model list also includes `embed-v5.0-pro`, `embed-v5.0-fast`,
`parse-v5.0`, and `cohere-transcribe-arabic-07-2026`:
https://docs.cohere.com/docs/models

These are not blindly added in this checkpoint. The retrieved pricing page
https://cohere.com/pricing lists dedicated Embed 5 instance-hour tariffs, not
public per-token prices; those units must not be converted into invented token
prices or unknown-as-free defaults. Parse and additional audio entries also
require endpoint and usage/billing compatibility validation. Existing estimates
for other research models and capability metadata need a separate complete audit.

## Release and rollback

This is a billing-default correction for four explicitly named models, not a
schema migration. Operators using a negotiated deployment should confirm their
channel-specific pricing overrides before rollout. Revert this provider commit
to restore the prior defaults; no data migration or secret rotation is required.
