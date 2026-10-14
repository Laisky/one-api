# Gemini model refresh — October 13, 2026

## Research and change

Google's [release notes](https://ai.google.dev/gemini-api/docs/changelog) and
[model catalogue](https://ai.google.dev/gemini-api/docs/models) were reviewed on
October 13, 2026 against the October 12 catalogue refresh. The newly released
`gemini-4-argon` is added once to the shared Gemini catalogue. Native Gemini,
Gemini OpenAI-compatible, and Vertex model discovery receive the same entry.
The native system-instruction allowlist adds only this exact identifier.

The [Argon model reference](https://ai.google.dev/gemini-api/docs/models/gemini-4-argon)
and [migration guide](https://ai.google.dev/gemini-api/docs/latest-model) establish:

- A unified 1,048,576-token context shared by input, thinking, and output, with
  a 65,536-token synchronous output limit. The larger long-decode limit requires
  continuation; this patch neither implements nor advertises that workflow.
- Multimodal input and text-only output. No image/audio generation or Live API.
  The catalogue's `file` modality covers the documented PDF input, not arbitrary
  file types or a claim that every gateway request format transports every modality.
- Low/medium/high reasoning, default medium. No integer thinking-budget ceiling
  is inferred from Gemini 3. Sampling controls temperature/top_p/top_k are ignored;
  the new entry advertises only the verified max_tokens parameter.
- Implicit caching only. No explicit-cache storage tariff or cache-write fee is
  fabricated. Capability metadata is not certification of every native tool or
  account-specific entitlement.

## Standard token pricing

USD per million tokens, from the
[Developer API pricing page](https://ai.google.dev/gemini-api/docs/pricing):

| Effective period | Input | Output, including thinking | Cached input |
| --- | ---: | ---: | ---: |
| Through November 30, 2026 | 2.00 | 10.00 | 0.10 |
| From December 1, 2026 | 4.00 | 20.00 | 0.20 |

The existing production time-window resolver applies the transition at December
1, 00:00 UTC, following the repository's UTC convention for date-only tariffs.
Tests cover both sides of that boundary, equivalent instants in other offsets,
and later dates. There is no inherited 200K-token Pro surcharge. Batch, Flex,
and Priority prices are not substituted for ordinary synchronous relay rates.
The existing Gemini model-specific search default resolves to $0.014 per query;
pooled monthly free allowances are not presumed available to each request.

[Google Cloud pricing](https://cloud.google.com/gemini-enterprise-agent-platform/generative-ai/pricing)
independently confirms matching **effective net global-endpoint** token prices.
Cloud delivers the introductory reduction through credits back on net spend;
it is not a promise of the same gross invoice charge. Non-global endpoints cost
more. The existing Vertex global route is covered by regression tests and is
supported by the [Cloud endpoint reference](https://docs.cloud.google.com/gemini-enterprise-agent-platform/resources/locations).
Custom regional routing, negotiated discounts, credit eligibility, and tool-fee
overrides remain administrator responsibilities; Vertex tooling defaults are
not changed by this token-catalogue update.

## Compatibility and exclusions

Existing identifiers, legacy tariffs, lifecycle descriptions, administrator
configuration, and routing overrides remain unchanged. In particular, the
Gemini 3.6/3.7/3.8 Flash introductory period still ends on December 31, not
November 30. No aliases are silently redirected.

Nano Banana 2.1 remains excluded for the image-routing and billing qualification
identified by the October 12 audit. Restricted Cyber variants, autonomous agents,
and new native protocols are not added simply because they appear in a provider
list. This change does not expand image generation, Live, or continuation support.

## Regression commands

The new `TestGeminiOctober13*` cases cover catalogue/list initialization, actual
native Chat Completions and Claude Messages system-prompt conversion, synchronous
and SSE URLs, explicit API-version overrides, Vertex global routing, and effective
pricing. They need no provider credentials and make no paid provider requests.
Run with the repository's Go 1.27.2 validation compiler:

```sh
go test -count=1 -run '^TestGeminiOctober13' ./relay/adaptor/geminiOpenaiCompatible ./relay/adaptor/gemini ./relay/adaptor/vertexai
go test -race -count=1 ./relay/adaptor/gemini/... ./relay/adaptor/geminiOpenaiCompatible ./relay/adaptor/vertexai ./relay/pricing
go vet ./relay/adaptor/gemini/... ./relay/adaptor/geminiOpenaiCompatible ./relay/adaptor/vertexai ./relay/pricing
make test-quick
```

These are scoped regression commands, not evidence that live-provider, complete
repository database/race, or frontend qualification has run. Record actual run
results against the exact commit in the PR. Existing required CI gates remain.

## Release and rollback

No deployment, migration, database write, or channel auto-enablement is included.
After review and required checks, administrators may explicitly select the new
model and should verify their provider/account tariff. Removing the Argon entry
and its exact system-instruction allowlist addition rolls back this change;
move channels using Argon to a supported model before reverting. Existing model
settings and data do not need restoration or migration.
