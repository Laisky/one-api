# GPT provider audit — 2026-09-30

## Scope

This audit extends PR #435 beyond the native OpenAI adaptor. Repository inspection
started at `f3014997a169bb98f22eee1aa999e9895b09db2d`; the follow-up changes retain
that commit and the existing provider catalogs. Search matches were classified
as closed OpenAI GPT, hosted GPT-OSS, configurable proxies, or unrelated names.
This is not an assertion that every historical model, capability flag, regional
price, or account entitlement has been revalidated. Unresolved items are explicit
below. A model-creator release is not sufficient evidence of reseller support.

## Implemented changes

### Azure routing

The shared OpenAI adaptor previously selected Azure Responses only for names
beginning with `gpt-5`. Add the exact published `gpt-6-astra`, `gpt-6-sol`,
`gpt-6-luna`, and `gpt-6.1-sol` IDs to the tool-safe Responses route. This updates
both endpoint construction and conversion decisions used by the controllers.
Do not classify GPT-OSS, an arbitrary `gpt-60` name, or a tenant deployment alias
as a published model. The GPT-5 policy is preserved.

The typed sampling transformation now recognizes Azure requests targeting
Responses and removes unsupported temperature instead of inserting a Chat-only
fallback. Tests exercise all four models across three endpoint inputs, actual
Chat-to-Responses function-tool conversion, effort preservation, and negative
controls for unrelated deployments.

Source: https://learn.microsoft.com/en-us/azure/foundry/foundry-models/concepts/models-sold-directly-by-azure

Azure still inherits the repository's OpenAI pricing fallback. This PR does not
establish an Azure regional/deployment tariff: the Azure pricing calculator was
not accessible in this research environment. Configure contracted prices and
map custom deployment names explicitly; catalog membership is not provisioning.

### OpenRouter catalog

Add `openai/gpt-6.1-sol` and `openai/gpt-6.1-sol-pro` from OpenRouter's public
models API. Pro is its alias for Sol with `reasoning.mode=pro`, not a new native
OpenAI ID. Both have independently owned metadata and these Standard prices:

| Provider input range | Input | Cached input | Cache write | Output |
| --- | ---: | ---: | ---: | ---: |
| Below 272,000 tokens | 2.00 | 0.10 | 2.50 | 10.00 |
| From 272,000 tokens | 4.00 | 0.20 | 5.00 | 15.00 |

Amounts are USD per million tokens. OpenRouter publishes an inclusive
`min_prompt_tokens: 272000` override. Preserve that provider contract rather than
silently changing it to native OpenAI's strictly-greater-than-272K threshold
(`272_001`). Tests cover both boundaries and keep the native rate independent.

The API advertises file/image/text input, 1,050,000 context, 128,000 output,
and low/medium/high/xhigh/max reasoning without none/minimal. OpenRouter's Chat
Completions tools remain on its own endpoint; native OpenAI Responses routing
is not imposed on this provider. Keep the Pro alias intact upstream.

Sources:
- https://openrouter.ai/api/v1/models
- https://openrouter.ai/openai/gpt-6.1-sol
- https://openrouter.ai/openai/gpt-6.1-sol-pro

The `:batch` variants are excluded from this synchronous adaptor. Their service
requires asynchronous submission and polling through `/api/v1/batches`.
Canonical slugs are not automatically promoted to public request IDs.

Batch source: https://openrouter.ai/openai/gpt-6.1-sol%3Abatch

## Cross-provider results

“Preserved” means no speculative catalog or tariff change was made. It is not a
blanket certification of all legacy entries or all transport capabilities.

| Adaptor | Inspected result and disposition |
| --- | --- |
| OpenAI | Native `gpt-6.1-sol` entry and existing native regression suite remain in this PR. See `openai/MODELS_20260930.md`. |
| Azure | Fixed GPT-6 routing. Catalog/prices continue to inherit native compatibility defaults; provider-specific prices were not independently established. |
| OpenRouter | Added two synchronous, provider-prefixed Sol 6.1 IDs with provider prices, boundaries, capabilities, and ownership tests. |
| AIProxy | `aiproxy/constants.go` already shares OpenAI's map; `adaptor.go` exposes its price getters. Added inheritance tests, not duplicate definitions. The `/api/library/ask` transport and its configured library are not evidence of upstream model availability or a reseller tariff. |
| AWS Bedrock | The current `aws/openai` mapping implements GPT-OSS only. Upstream now documents GPT-6.1 Sol, but catalog-only activation would bypass missing transport and billing work. Remains an explicit implementation gap, detailed below. |
| Replicate | Inspected `constants_language.go` and public OpenAI publisher/model pages. Preserved hosted GPT-5.6/OSS entries; no verified Replicate Sol 6.1 contract was found. Existing GPT-5.6 context/output values are explicitly unconfirmed in the code and remain unresolved, not newly certified. |
| Groq | GPT-OSS prices remain provider-specific. Verified 120B Standard pricing and protected it with a resolver regression; no closed GPT-6.1 catalog addition. |
| Cerebras | Public models API confirms GPT-OSS-120B pricing and 131,072/40,960 limits. Preserved the existing row and protected its price. |
| Fireworks | GPT-OSS-120B Standard prices match. The 20B card is on-demand-only; retained historical serverless prices are not a current dedicated quote. |
| DeepInfra | Checked 120B, Turbo, Ultra, and 20B cards against `models_openai.go`. Kept the distinct tariffs; a base 120B price regression prevents accidental homogenization. |
| Together AI | Current serverless 120B prices match. The retained 20B row was not re-established as a current serverless offering; no forced deletion of configured/historical IDs. |
| Novita | Current pricing page confirms 120B and 20B rates. Do not restore the higher 2025 launch prices. The model API could not be read here; existing image-input and other capability metadata are not recertified by the pricing page. |
| Vertex AI | Official Google pricing confirms the existing GPT-OSS MaaS token rates. Kept MaaS IDs separate from native OpenAI. No new exact output-limit verification is claimed. |
| Cloudflare | Workers AI cards confirm both GPT-OSS Standard rates and 128,000 context. Price isolation is tested. Existing tool-capability metadata needs a separate transport check; this pass does not certify or change that flag. |
| SiliconFlow | Inspected GPT-OSS presence and the CNY-based `nativeRate` helper. The primary pricing page timed out; no currency conversion, repricing, or closed-model addition is inferred. |
| NVIDIA | Inspected the GPT-OSS catalog identity and public model card. No production per-token quote was established; existing deployment/trial defaults are not replaced by OpenAI API prices. |
| Ollama | Checked GPT-OSS family/tag identity against the library. Local deployment billing defaults are not an upstream OpenAI tariff; preserved them. |
| Copilot | `GetModelList()` returns no compiled list. Preserve account-dependent selection rather than importing native OpenAI defaults or subscription UI names. |
| Generic OpenAI-compatible transports | Existing endpoint/configuration policy is retained. No universal availability or price is inferred for arbitrary compatible servers or GitHub Models endpoints. |
| AI360 | `360GPT_*` belongs to AI360, not OpenAI. Excluded from OpenAI model propagation; preserved its explicitly unverified historical tariffs. |

### Independently checked GPT-OSS-120B Standard prices

USD per million tokens; these are different hosted endpoints, not one interchangeable
price table. The regression uses actual provider configs and the production resolver.
It does not assume unlisted caching prices are free.

| Provider | Input | Output | Primary source |
| --- | ---: | ---: | --- |
| Groq | 0.15 | 0.60 | https://console.groq.com/docs/model/openai/gpt-oss-120b |
| Cerebras | 0.35 | 0.75 | https://api.cerebras.ai/public/v1/models |
| Fireworks | 0.15 | 0.60 | https://fireworks.ai/models/fireworks/gpt-oss-120b |
| DeepInfra | 0.037 | 0.17 | https://deepinfra.com/openai/gpt-oss-120b |
| Together AI | 0.15 | 0.60 | https://docs.together.ai/docs/serverless/models |
| Novita | 0.05 | 0.25 | https://novita.ai/pricing |
| Vertex AI MaaS | 0.09 | 0.36 | https://cloud.google.com/vertex-ai/generative-ai/pricing |
| Cloudflare | 0.35 | 0.75 | https://developers.cloudflare.com/workers-ai/models/gpt-oss-120b/ |

Additional inspected sources:
- https://replicate.com/openai
- https://replicate.com/openai/gpt-5.6-sol
- https://replicate.com/openai/gpt-5.6-terra
- https://replicate.com/openai/gpt-5.6-luna
- https://fireworks.ai/models/fireworks/gpt-oss-20b
- https://deepinfra.com/openai/gpt-oss-120b-Turbo
- https://deepinfra.com/openai/gpt-oss-120b-Ultra
- https://deepinfra.com/openai/gpt-oss-20b
- https://developers.cloudflare.com/workers-ai/models/gpt-oss-20b/
- https://build.nvidia.com/openai/gpt-oss-120b/modelcard
- https://ollama.com/library/gpt-oss
- https://docs.github.com/en/copilot/reference/ai-models/supported-models

## Bedrock: confirmed upstream support, incomplete repository support

The official GPT-6.1 Sol card documents Runtime and Mantle support, but the
repository's existing OpenAI Converse path only maps four GPT-OSS IDs. Its request
conversion lacks function-tool/multimodal preservation and tunable effort; the
response usage path does not carry all cache billing dimensions.

The Runtime model requires `us.openai.gpt-6.1-sol`, not an invented global profile
or direct in-region ID. Mantle uses `openai.gpt-6.1-sol` in `us-east-1` under the
`/openai/v1` base path. Standard short-context US pricing is $2.20 input,
$0.11 cache read, $2.75 cache write, and $11 output per million tokens. Long-context
prices are $4.40/$0.22/$5.50/$16.50 for the same dimensions. These include the
10% premium; do not add it twice. The card explicitly says that listed cache
prices do not imply explicit prompt-caching feature support.

Before advertising this model through the AWS adaptor, implement the chosen
transport/profile mapping, preserve tools/images/effort, propagate usage including
cache dimensions, and test streaming/non-streaming conversion plus provider-specific
billing. This PR does not claim that work is complete.

Source: https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-openai-gpt-6-1-sol.html

## Validation and safety boundaries

The follow-up adds ten test functions: three Azure route/conversion checks,
four OpenRouter catalog/ownership/transport/pricing checks, and three cross-provider
inheritance/isolation/pricing checks. The original six native tests remain.

Formatting was checked with `gofmt`. An isolated test of the extracted, actual
Azure route predicate failed against the old function and passed after the fix.
That narrow red/green result is not a substitute for running repository tests.
Local full tests remain blocked by Go 1.23.2 versus required Go 1.27.1 and the
unavailable toolchain download. GitHub CI is the full-repository execution path;
consult the current PR checks rather than treating this document as a green badge.

```sh
go test -race ./relay/adaptor/openai ./relay/adaptor/openrouter ./relay/adaptor \
  -run 'Test(AzureGPT6|GPT61|GPTProviderCatalogBoundaries20260930|GPTOSSProviderPrices20260930)' -count=1
go test -race ./relay/adaptor/... ./relay/pricing/...
go vet ./...
go test -race ./...
make build-frontend-modern
```

No paid generation, account-token discovery, channel migration, custom-price reset,
retired-ID deletion, or change to provider tooling tariffs was performed.
