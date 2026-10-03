# TypeSafe System One adaptor

Research checked on **2026-09-18** against the official [API reference](https://docs.typesafe.ai/api), [model catalog](https://docs.typesafe.ai/models), and [advanced structures guide](https://docs.typesafe.ai/primitives/advanced), and **verified the same day against the live service** with a real provider key. Where the reference and the running service disagree, this adaptor follows the service and the difference is called out below.

## Configure and call

Create a **TypeSafe** channel (type **60**), enter its provider API key, keep the default base URL `https://api.typesafe.ai`, and select the desired models and user groups. HTTPS custom bases, an optional `/v1` suffix, administrator endpoint overrides, and model mappings are supported. The only default supported endpoint is `systemone`.

Clients authenticate using a **one-api token**, not the provider key:

```bash
curl "$ONE_API_BASE_URL/v1/systemone" \
  -H "Authorization: Bearer $ONE_API_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "jev-latest",
    "state": {"review": "The food was excellent and service was quick."},
    "questions": {
      "positive": {"type": "noul", "instructions": "Is the review positive?"},
      "topic": {
        "type": "choice", "instructions": "Choose the primary topic.",
        "criteria": {"food": "Food quality", "service": "Service quality"}
      },
      "quality": {
        "type": "score", "instructions": "Rate the overall experience.",
        "criteria": ["Poor", "Average", "Excellent"]
      }
    }
  }'
```

The gateway retains native `model`, `answers`, and `usage.input_tokens` / `usage.output_tokens` response fields. It preserves nested JSON, nullable question instructions/criterion descriptions, arbitrary question IDs, probabilities and integer precision. Answer shapes differ per primitive and are forwarded untouched: `noul` returns only a probability and carries no `confidence`, while `choice` and `score` return `confidence` plus `probabilities`, and `score` adds a `legend` whose keys are stringified level indices and whose values may be arbitrary JSON.

Unknown envelope fields, duplicate routing/question keys and malformed primitives are rejected before dispatch rather than silently discarded. The service itself ignores unknown fields and accepts duplicate keys, so this is a deliberate gateway-side strictness: the request actually forwarded upstream is the re-marshalled one, so a field that is accepted here but dropped there would be a silent behavior change. State accepts text, objects or arrays — not numbers, booleans or null.

**This is not an OpenAI-compatible chat model.** Chat Completions, Responses, Claude Messages, embeddings, image generation and streaming are deliberately not advertised or converted: a conversation does not define the typed evaluation questions required by Jev. This is an explicit native-only exception to the general conversational adaptor pattern. Use `/v1/systemone`, not `/v1/chat/completions`. The gateway's authenticated `/v1/models` remains its standard catalog format, not TypeSafe's native model-list envelope.

The existing administrator channel test is conversation-oriented and skips channels that do not offer a conversational endpoint. TypeSafe is therefore skipped by that generic health check; a skipped test is not evidence of provider connectivity. Use the native request above to verify connectivity, model routing and billing with a valid provider key. This is an explicit, potentially paid inference request, not an automatic background probe.

## Question primitives

| Type | `instructions` | `criteria` |
| --- | --- | --- |
| `noul` | string, object, array or null | optional object with optional `true` / `false` descriptions |
| `choice` | string, object, array or null | **required** nonempty object of option → description (description may be null) |
| `score` | string, object, array or null | **required** nonempty array of ordered level descriptions |
| `bounding_box` | optional | opaque; forwarded verbatim |

The gateway validates **shape**, not **magnitude**. The reference states that score takes "at least two levels" and "up to 10", and that choice accepts "up to 255 options", but the service enforces only a nonempty collection at the schema layer and applies the upper caps itself (`400 Too many score levels. Must have at most 10 levels.`). A single-level score is genuinely accepted and answered. Enforcing the documented bounds locally would reject requests the service accepts today and would silently break if a cap changed, so those limits are left to the provider.

Likewise, the service requires a noul question to carry *some* content — neither `instructions: null` nor an empty/all-null `criteria` alone is enough — and rejects empty-string instructions. That is a content rule whose exact truthiness semantics are the provider's, so it is not reimplemented here; the provider's own message is forwarded, and the rejection is refunded. Empty question IDs are rejected locally: the service rejects them unconditionally and no answer could ever be addressed by one.

`bounding_box` is a fourth type advertised by the service's own discriminator error but absent from the public reference, and it is enabled per organization. Its criteria schema is therefore not knowable here, so the envelope is validated and the payload forwarded verbatim; organizations without the feature receive the provider's own `400`, which releases the reservation.

## Models and price

| Model | Role | Input USD / million tokens | Output price |
| --- | --- | ---: | ---: |
| `jev-1.13.0` | Pinned version | 0.042 | Free |
| `jev-latest` | Mutable production alias | 0.042 | Free |
| `jev-preview` | Mutable preview alias | 0.042 | Free |

The aliases currently point to `jev-1.13.0`; they can change independently of this gateway release. The provider's own `GET /v1/models` lists **only the two aliases**, yet `jev-1.13.0` is accepted by the `model` field — verified live — so the catalog keeps it. Responses report the versioned ID that answered, which is why a request for `jev-latest` comes back as `"model": "jev-1.13.0"`.

The documented price is **$42 per billion input tokens**, not $42 per million. The internal input ratio is `0.042 * ratio.MilliTokensUsd`; output usage is observable but never included in the native charge, even when a generic completion-ratio override is present. Group and input-price overrides still apply. Note that a channel model ratio of `0` means "not configured" to `pricing.ResolveModelRatioAt` and falls back to this catalog; a **group ratio** of `0` is the supported way to make a channel free. Unknown future models require an explicit verified flat input-price configuration.

The provider documents **64k tokens for state plus all questions** and **32k for state plus the longest question**. The catalog records the former and describes the latter. Both are enforced by the provider, which returns `400 {"detail":{"error_type":"max_tokens_exceeded"}}`; the gateway does not pretend a generic OpenAI tokenizer is Jev's tokenizer. State is counted once no matter how many questions share it. Published rate limits are 250,000 tokens per second and 1,200 requests per minute. Images, audio and video are not supported modalities.

## Billing and failure behavior

Every request uses existing authentication, model/group distribution, request-body limits, rate limits and shared HTTP transport. The native handler makes **one upstream attempt** and does not follow redirects. Provider credentials are attached only to validated HTTPS URLs; arbitrary client `X-*` headers are not copied upstream.

Admission physically reserves a conservative **65,536-input-token allowance**, including for trusted/unlimited API tokens. At default pricing/group ratio this is **1,377 internal quota units**; the hold is not a claim that the request actually used that many tokens. A verified receipt replaces the allowance with the actual input charge using checked decimal arithmetic and one upward rounding. For example, 370 input tokens cost 8 quota units regardless of output token count. Settlement applies only `actual - reserved`, preventing double debits, and native output is not committed until the durable balance adjustment succeeds.

**Every 4xx status, plus the documented `529 Overloaded`, releases the reservation.** System One is a single-shot, non-streaming API that reports usage only on a successful evaluation, and every client error observed live is raised by the admission layer with no usage receipt:

| Status | Observed body | Cause |
| --- | --- | --- |
| 400 | `{"detail":{"error_type":"max_tokens_exceeded"}}` | context budget exceeded |
| 400 | `{"detail":{"error_type":"api_usage_error","message":"Unknown model: …"}}` | unknown model, or a feature not enabled for the organization |
| 400 | `{"detail":"Noul question must have criteria or instructions: q"}` | primitive content rules and documented caps |
| 401 | `{"detail":{"error_type":"authentication_error", …}}` | invalid API key |
| 403 | `{"detail":{"error_type":"authentication_error", …}}` | absent API key |
| 404 / 405 | `{"detail":"Not Found"}` | base URL missing `/v1`, or wrong method |
| 422 | `{"detail":[{"type":"missing","loc":["body","state"], …}]}` | schema validation |
| 429 | — | rate limit |

The provider's published error table lists only 401/422/429/529. Restricting the refund to that subset charged the full reservation for the most common real failures — an oversized state, a mistyped model, a channel pointed at the wrong base URL — so classification now follows the status class rather than that table. If a rejection ever does carry a usage receipt, the measurement wins over the classification and that input is billed.

Ambiguous outcomes — 5xx other than 529, timeouts, interrupted bodies, missing usage or malformed input counters — can represent paid work. Their reservation is retained and explicitly marked as **estimated**, with `billing_estimated`, `billing_estimate_reason`, and `X-OneAPI-Billing-Estimated: true`. Operators must reconcile these estimates against provider records; they are not claimed to be actual usage. To make that reconciliation possible, the provider's `x-typesafe-request-id` is recorded in the consume log's `upstream_request_id` metadata and returned to the caller as `X-Typesafe-Request-Id`, including on the failure paths where the gateway rejects the provider's answer. A malformed answer with a valid input receipt still settles the measured input usage. Unrepresentable monetary values or failed durable settlement leave the reservation unresolved and return an error rather than emitting a successful answer.

Native JSON errors are preserved, along with `Retry-After` and the `retry-after-ms` hint TypeSafe's own SDKs honour; neither is guaranteed to be present. Callers may choose explicit backoff for rate limits/capacity errors. Do not blindly retry ambiguous failures: the first attempt may already have incurred a charge. Responses are buffered with an 8 MiB gateway safety cap; an oversized or incomplete response is not silently treated as free work.

## Validation

The regression suite covers request primitives and structured/null descriptions, duplicate/unknown fields, model registry and prices, lossless response forwarding, malformed billing receipts, admission-rejection classification, exact input-only arithmetic, upstream request-id propagation, HTTPS credential handling and no-redirect replay. Request and response fixtures are payloads captured from the live service, including the three primitives' real answer shapes and each rejection body in the table above. Native-handler integration tests use actual SQLite balances to check reservations, settlement, refunds, free/group/custom input pricing, unlimited-token behavior, model mappings and estimated-usage logs. Tests use local fixtures and TLS mock servers, not paid provider requests. No dependencies or CI workflows are added.

Run with the repository's supported Go toolchain:

```bash
go test ./relay/adaptor/typesafe ./relay ./relay/controller ./router ./controller
go test -race ./relay/adaptor/typesafe ./relay/controller -run 'TypeSafe|SystemOne|NativeResponse|InputQuota|DecodeRequest|ResponseEvidence|HTTPRejections|Redirects|Catalog|URLs|AdmissionRejections|UpstreamRequestID|MeasuredUsage|LiveAnswerShapes'
```
