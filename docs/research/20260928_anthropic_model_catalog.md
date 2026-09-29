# Claude catalog and protocol qualification — 2026-09-28

Base: `6e668a70fe611dd1c60b3907fed242b7ed56753c`, PR #431.

## Provider scope and default-price decision

Sonnet 5.5 is registered on Anthropic, Amazon Bedrock, Microsoft Foundry, Vertex AI and OpenRouter. Bedrock is **included**. The repository owner explicitly selected Claude-equivalent Bedrock default prices on September 28, 2026. No further AWS tariff confirmation or approval is a release gate. This is a deliberate default-pricing policy, not a claim of an independently reconciled AWS invoice. Existing operator overrides remain authoritative.

USD per million tokens for Sonnet 5.5: input **2**, output **10**, cache read **0.20**, five-minute write **2.50**, one-hour write **4**. Ordinary context/output limits are **1,000,000 / 128,000** tokens; the 300K Batch beta is not an ordinary Messages limit.

| Surface | Model registration and boundary |
| --- | --- |
| Anthropic | `claude-sonnet-5-5`; native Messages. The eight-model price regression protects prior Claude rates and model-specific cache discounts. |
| Bedrock | Public alias `claude-sonnet-5-5` maps to `anthropic.claude-sonnet-5-5`. The existing SDK invokes its documented global inference profile from 33 commercial source regions. Explicit operator ARNs win. No invented geographic profile or commercial routing from GovCloud. |
| Foundry / Azure | Sonnet 5.5, Opus 5.5, Opus 5, Fable 5.1 and Mythos 5.1 discovery entries. Claude uses `/anthropic/v1/messages`; opaque deployment names retain their wire identity while known origin metadata supplies compatibility. |
| Vertex AI | `claude-sonnet-5-5`, independent global rates and conservative provider features. Native Messages bodies are preserved, with only the transport-owned model/stream/version fields rewritten. |
| OpenRouter | `anthropic/claude-sonnet-5.5`, retaining OpenRouter's own parameters, efforts and price buckets. Its advertised `temperature` is not removed by native Claude policy. |

Copilot has no static catalog or verified subscription-to-token tariff mapping in this repository. It is not assigned invented IDs or token prices. Claude Platform on AWS is a separate transport from Bedrock; this change does not rename or replace the existing SDK implementation. Account entitlement and regional premiums remain operator/provider concerns, not model-discovery gates.

## Request contracts repaired

- Native Sonnet 5.5 accepts `adaptive` and `between_tools`; omitted thinking defaults to adaptive. Legacy `disabled` maps to `between_tools`, not adaptive. Legacy `enabled` loses its obsolete budget but does not invent an effort level. Existing Sonnet 5 `disabled` remains disabled.
- Adaptive effort supports `low`, `medium`, `high`, `xhigh`, `max`, default `high`. `between_tools` accepts only low/medium/high and no sibling thinking fields. Conflicting per-message effort is rejected, not rewritten.
- Forced tool selection and assistant prefill are rejected without silently weakening the caller's instructions. Supported `none`, stop constraints, maximum completion limits, complete tool schemas and explicit false values survive conversion. Malformed explicit tool schemas are rejected instead of replaced by an unrestricted empty schema.
- Chat, Responses-to-Chat, native Messages and final provider dispatch preserve output configuration and thinking. Final raw-body validation runs after extension merging. Unknown models and third-party namespaces do not inherit native restrictions.
- Azure resolves canonical compatibility separately from an opaque deployment name. Explicit operator-configured Bearer authorization selects Entra without forwarding inbound gateway credentials or also sending API-key credentials. Token acquisition/refresh remains the operator's existing credential-management responsibility.
- Vertex no longer converts native Claude history through a lossy OpenAI representation. Signed blocks, structured system content, full schemas, unknown fields and integers above 2^53 remain intact.

## Response and billing contracts repaired

The native/converted HTTP paths now share presence-aware cumulative usage handling. Repeated snapshots are not summed; input, output, cache reads and both cache-write TTLs remain separate. Invalid counters are rejected atomically without destroying earlier verified receipts. An aggregate-only update does not erase a prior TTL split.

Streaming requires an ordered start/content/final-delta/stop sequence and a clean transport close before a successful terminal marker. Truncation, cancellation, malformed frames and failed or short downstream writes return errors. Native named events and opaque response fields survive; converted tool indices remain stable across non-consecutive content-block indices. The Responses bridge receives the same checked usage and completion boundary.

Valid final receipts survive downstream delivery or later transport failure and settle through existing pricing. A stream that stops before a final usage phase retains its observed counters but is marked estimated, so existing settlement retains at least the reservation. Missing receipts are not invented zero-cost rejections. No transparent paid retry is introduced. HTTP objects have a 32 MiB per-object guard, not a total-stream size cap.

Database-backed acceptance covers native/converted JSON and streaming, final-receipt truncation, partial receipts, missing receipts and failed delivery. It compares user and finite-token debits with an independent five-bucket arithmetic oracle and checks that retry reset cannot refund a settled request. This verifies these in-process scenarios, not a new durable exactly-once ledger across process death or database outages.

Sonnet 5.5 image admission reserves up to **4,784 visual tokens per image**, not OpenAI's 85-token low-detail value or the older 853/1,568 estimates. URL, inline, file-backed and nested native tool-result images are covered without dimension fetches for estimation. Large-body estimates retain image allowances. Opaque portable file parts receive an allowance, not a promise to count every page of a PDF. Final image charging still uses provider receipts, never a fixed 4,784-token fee.

## Reproduction and qualification

The original production tree was unchanged in [negative-control run 36502347169](https://github.com/Laisky/one-api/actions/runs/36502347169). Five added test files reproduced **30 failing leaf cases** with Go 1.27.1 and race detection. The same assertions remain in the repaired tree.

Additional tests exercise final AWS SDK and Vertex body preparation, Azure authentication and aliases, all five adaptive efforts, the real Responses fallback, signed/raw field preservation, image reservation and database settlement. Existing incomplete success fixtures were made protocol-complete; incomplete-stream cases now assert failure. No CI check or assertion was skipped to obtain a green result.

Qualification uses the repository's Go 1.27.1 toolchain, including local compilation, affected package race tests, repository-wide vet and build. Exact final-head CI and evidence links are recorded in PR #431; earlier-head green checks are not substituted for final-head acceptance.

```sh
go test -race -count=1 ./relay/adaptor/anthropic ./relay/adaptor/aws/... ./relay/adaptor/azure ./relay/adaptor/vertexai/... ./relay/adaptor/openrouter ./relay/adaptor/deepseek ./relay/adaptor/openai ./relay/controller ./relay ./relay/pricing ./relay/model
go vet ./...
go test -race ./...
go build -o one-api .
```

No paid inference probes, new dependencies, data migration, production deployment or automatic merge. Temporary editing/qualification workflows are not included in the PR's production commits. Provider invoice reconciliation and access to any specific customer account are not claimed.

## Primary sources

- https://platform.claude.com/docs/en/models/sonnet-5-5/overview
- https://platform.claude.com/docs/en/models/sonnet-5-5/migration-guide
- https://platform.claude.com/docs/en/about-claude/pricing
- https://platform.claude.com/docs/en/build-with-claude/streaming
- https://platform.claude.com/docs/en/build-with-claude/prompt-caching
- https://platform.claude.com/docs/en/build-with-claude/vision
- https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-sonnet-5-5.html
- https://learn.microsoft.com/en-us/azure/foundry/foundry-models/concepts/claude-models
- https://platform.claude.com/docs/en/build-with-claude/claude-in-microsoft-foundry
- https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai
- https://cloud.google.com/vertex-ai/generative-ai/pricing
- https://openrouter.ai/api/v1/models
- https://docs.github.com/en/copilot/reference/copilot-billing/models-and-pricing
