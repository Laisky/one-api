# PR 622 schema release qualification (2026-11-15 UTC)

## Verified fixes

The registered standard OpenAI structured-output path previously forwarded Boolean `exclusiveMinimum`/`exclusiveMaximum`. Loopback provider validation reproduced HTTP 400 for both Chat-to-Responses fallback and native Responses, for true and false flags. Existing numeric thresholds and property-name/literal-data controls passed. The shared schema-node normalizer now translates valid legacy true-plus-inclusive-bound pairs into the identical numeric exclusive threshold, removes false flags, and retains inclusive bounds. It preserves exact decoded numbers, caller ownership and repeat-conversion stability. Invalid unpaired Boolean forms retain conservative removal; no threshold is invented. Destinations without qualified exclusive support retain their previous policy.

Whole-document substring precision assertions could mistake a nested fractional prefix for an integer bound. Permanent tests now inspect complete raw number tokens at their schema paths. Deliberately rounded integer and fractional mutations must fail; defaults, property names and other schema branches cannot supply a matching decoy.

## Contract evidence

Checked November 15, 2026:

- [Draft 4 validation sections 5.1.2 and 5.1.3](https://json-schema.org/draft-04/draft-fge-json-schema-validation-00): legacy exclusive flags are Boolean and depend on the matching numeric inclusive bound.
- [Modern JSON Schema numeric validation](https://json-schema.org/draft/2020-12/json-schema-validation#name-exclusiveminimum): exclusive bounds are numeric thresholds.
- [OpenAI supported structured schemas](https://developers.openai.com/api/docs/guides/structured-outputs#supported-schemas): documents numeric bounds and model-specific restrictions. The existing registered-model policy remains in force; this change does not widen fine-tuned, unknown or Azure support.

## Qualification and limits

Behavioral tests use local transports, not paid provider calls. They cover exact wire numbers, nested constraints, mapped upstream models, source ownership, repeat conversion, provider isolation, and rejection without an additional usage receipt or additional charge beyond the existing dispatch reservation. Precision mutation tests cover schema ingress, compatible conversion, Google transport and stored MCP schemas. CI remains lean; these regressions stay available for manual broader qualification.

Gateway token preservation does not prove arbitrary-precision enforcement upstream. Full database/race matrix, frontend/browser and authenticated live entitlement/billing qualification are not claimed. Existing JSON-reconstruction provenance limits and the earlier unexplained DeepSeek concurrent-funding result remain documented in the PR; they are not reclassified as proven flakes. No merge, release or deployment is authorized here.

FileIO tools were unavailable: intended history terms included one-api, 622, exclusiveMinimum, exclusiveMaximum, numeric precision and release schema. No FileIO search, save or readback is claimed. Non-sensitive execution evidence remains in the existing local task evidence and dev `.test-results/pr622-p2/` directories.

## Check receipts

Passed with Go 1.27.4: the full selected 18-package provider/ingress acceptance suite (plus one package with no tests); Boolean bound unit/HTTP acceptance; exact-token mutation controls; focused race tests across OpenAI, relay model and both controller packages; all 28 `make test-quick` tests; `go vet ./...`; entity-response and goroutine guardrails. `govulncheck ./...` reported zero reachable/imported-package vulnerabilities and three uncalled required-module advisories.

The initial broad/race runs compiled the deliberately failing legacy precision validator while its regression was being demonstrated. Their failures identify the expected test-only mutation counterexamples, not a production failure. Those logs remain preserved; final qualification uses the stable exact-token validator.
