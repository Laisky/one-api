# Gemini catalog refresh — 2026-09-18

## Scope

Refresh the shared Gemini catalog used by the native Gemini adaptor and its
OpenAI-compatible pricing metadata. Gemini 3.8 Flash, 3.7 Flash, Robotics ER 2,
Omni 1.1, transcription, and the January 2027 Flash price transition already
existed on `main`; this change does not add duplicate entries or alter those
prices. No provider credentials, network probes, dependencies, or CI workflows
are added.

## September 15 Live release

Google's [changelog](https://ai.google.dev/gemini-api/docs/changelog) and model
cards list two new stable models:

| Model ID | Input token limit | Output token limit | Configurable thinking |
| --- | ---: | ---: | --- |
| `gemini-3.8-live` | 131,072 | 65,536 | None; thinking is automatic |
| `gemini-3.8-live-extended-thinking` | 131,072 | 65,536 | `low`, `medium`, `high` |

Sources: [Live](https://ai.google.dev/gemini-api/docs/models/gemini-3.8-live),
[Extended Thinking](https://ai.google.dev/gemini-api/docs/models/gemini-3.8-live-extended-thinking).
Both accept text, image, audio, and video input. Audio responses can include
transcription text; this does not mean a text-only response modality is accepted.
Neither card advertises context caching or structured output. Do not inherit
Flash's thinking-budget limit, default thinking level, or promotional overlay.

The [standard paid-tier prices](https://ai.google.dev/gemini-api/docs/pricing)
for both models, in USD per million tokens, are text input 0.75, text output
4.50, audio input 3.00, and audio output 12.00. The existing audio pricing
multipliers represent these four token prices. Search grounding remains 14 USD
per thousand upstream search queries before any account-level free allowance.

**Transport and billing boundaries:** these are catalog entries, following the
existing Live-model catalog convention, not a Live API implementation. The
native adaptor still sends REST `generateContent`/`streamGenerateContent`
requests, not bidirectional Live sessions. Do not route these models through
that path or advertise working real-time audio support on the strength of this
PR. A separate Live transport implementation is required. Google's separate
1.00 USD/million image/video input-token price is not representable by the
current image-generation/video-output pricing fields and is not approximated
with them. Accurate Live image/video billing requires a separate billing change.
The descriptions expose the transport limitation to catalog consumers. The shared
REST dispatcher now rejects both new Live-only IDs on native Gemini, Vertex AI,
and Gemini OpenAI-compatible channels before URL preparation, credential setup,
body reads, or network dispatch. It uses the mapped upstream model name. Ordinary
Flash requests and unrelated providers' own REST bridges are not blocked. This
change does not add or alter a WebSocket Live transport.

## Existing-model corrections

The [migration guide](https://ai.google.dev/gemini-api/docs/latest-model) and
[changelog](https://ai.google.dev/gemini-api/docs/changelog) deprecate
`temperature`, `top_p`, and `top_k` for the newer Flash models. The shared
3.6/3.7/3.8 Flash metadata no longer advertises them; legacy 2.5 metadata stays
unchanged. This is discovery metadata, not a new runtime parameter filter.

The native system-instruction allowlist now includes stable image IDs
`gemini-3-pro-image`, `gemini-3.1-flash-image`, and
`gemini-3.1-flash-lite-image`, the dated
`gemini-2.5-computer-use-preview-10-2025` ID, and
`gemini-robotics-er-2-preview`. This closes the gap between the
[model catalog](https://ai.google.dev/gemini-api/docs/models) and native request
conversion. Regression tests cover Chat Completions and Claude Messages input,
checking that the system prompt remains outside ordinary conversation turns.

The [deprecation schedule](https://ai.google.dev/gemini-api/docs/deprecations)
is reflected in descriptions, without removing model IDs or changing retained
pricing. Updated entries include the old 3 Pro preview, 3.1 Flash-Lite preview,
both 3.x image previews, 2.5 Flash Image and its preview, and the 3.1 Live
replacement recommendation. October 2, 2026 is an **earliest** shutdown date for
2.5 Flash Image, not a claim that it is already unavailable. Preview replacement
recommendations point to current stable image IDs instead of retired previews.
The changelog confirms that Gemini 3 Pro preview, Gemini 3.1 Flash-Lite preview,
and Gemini 2.5 Flash Image preview were shut down on March 9, May 25, and January
15, 2026 respectively. Their descriptions now state the confirmed shutdowns.
The Gemini 3 Pro preview identifier points to Gemini 3.1 Pro preview; the
identifier's continued alias behavior is not the old model's continued availability.

## Validation

New tests cover absolute text/audio prices, search pricing, limits, unsupported
features, isolation of mutable metadata, derived lists after initialization,
lifecycle guidance, and native system-prompt conversion. Existing September
catalog and pricing-boundary tests remain unchanged.

Run in a complete checkout with the repository's required Go toolchain:

```sh
go test -race ./relay/adaptor/gemini ./relay/adaptor/geminiOpenaiCompatible
go vet ./...
go test -race ./...
```

No paid upstream call is necessary for these deterministic regression tests.
The metadata reflects public documentation; account-specific availability was
not verified with a provider API key.

## Review and CI reproduction evidence

The test-only commit `46a32b6a063c3ef76f5f7bc357b7bb0e7f179f01` changed no
production code or CI workflow. Existing CI run `35355460095` tested its merge
with main `7f3449fd6daea8b603b73d3fcc3a505b7769b4da`, at merge revision
`6eeca65eafc135f821f14fb7b6426a797fa9e092`, using Go 1.27.1 and race detection.
The `go-tests-packages` artifact contains the complete JSON events and logs.

- **Confirmed transport defect:** all 12 Google-channel Live cases (three
  channel types, two model IDs, streaming on/off) reached the local HTTP server
  before the fix. Each recorded one URL preparation, one header preparation,
  two body reads, and one network request, with no error. All 20 positive
  controls for ordinary Flash or third-party REST bridges passed.
- **Confirmed metadata defect:** all six final-catalog checks (three retired
  models in native and OpenAI-compatible catalogs) exposed ambiguous earliest-date
  wording rather than confirmed shutdowns. The regression also checks the
  Gemini 3 Pro alias and preserves future earliest-date guidance.
- **Excluded billing false positive:** the original `free_input` assertion
  expected a channel ModelConfig ratio of zero to make a request free. The
  behavior probe observed reservation 1,377 and final charge 7 quota. Main's
  concurrent `fc40b04abf2dd0da13d666750edd0d58dbb857d0` correction explicitly
  documents that zero means an unset/inherited channel ratio; group ratio zero
  is the supported free-charge configuration. Its corrected original integration
  case already passed in the red run. No production billing change is warranted.
  The additional probe is retained as `TestSystemOnePriceContractBehavior`,
  testing inherited prices, direct/mapped names, unlimited tokens, positive
  overrides, and genuinely free groups across admission, balances, receipts,
  and consumption logs. This is a corrected test expectation, not a billing fix.

The transport and lifecycle reproducers are retained unchanged as regressions.
The red run reported no race warnings; its three top-level failures were the
transport test, lifecycle test, and the subsequently excluded billing hypothesis.
After applying the fixes, acceptance must use the existing full CI run, including
all test shards and static guardrails; a test-only red run is not acceptance.
