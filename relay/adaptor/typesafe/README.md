# TypeSafe System One adaptor

Research checked on **2026-09-18** against the official [API reference](https://docs.typesafe.ai/api), [model catalog](https://docs.typesafe.ai/models), and [advanced structures guide](https://docs.typesafe.ai/primitives/advanced).

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

The gateway retains native `model`, `answers`, and `usage.input_tokens` / `usage.output_tokens` response fields. It preserves nested JSON, nullable question instructions/criterion descriptions, arbitrary question IDs, probabilities and integer precision. Unknown envelope fields, duplicate routing/question keys and malformed primitives are rejected before dispatch rather than silently discarded. State accepts text, objects or arrays; score requires at least two ordered criteria.

**This is not an OpenAI-compatible chat model.** Chat Completions, Responses, Claude Messages, embeddings, image generation and streaming are deliberately not advertised or converted: a conversation does not define the typed evaluation questions required by Jev. This is an explicit native-only exception to the general conversational adaptor pattern. Use `/v1/systemone`, not `/v1/chat/completions`. The gateway's authenticated `/v1/models` remains its standard catalog format, not TypeSafe's native model-list envelope.

## Models and price

| Model | Role | Input USD / million tokens | Output price |
| --- | --- | ---: | ---: |
| `jev-1.13.0` | Pinned version | 0.042 | Free |
| `jev-latest` | Mutable production alias | 0.042 | Free |
| `jev-preview` | Mutable preview alias | 0.042 | Free |

The aliases currently point to `jev-1.13.0`; they can change independently of this gateway release. The documented price is **$42 per billion input tokens**, not $42 per million. The internal input ratio is `0.042 * ratio.MilliTokensUsd`; output usage is observable but never included in the native charge, even when a generic completion-ratio override is present. Group and input-price overrides still apply. Unknown future models require an explicit verified flat input-price configuration.

The provider documents **64k tokens for state plus all questions** and **32k for state plus the longest question**. The catalog records the former and describes the latter. Exact tokenization and both limits are enforced by the provider; the gateway does not pretend a generic OpenAI tokenizer is Jev's tokenizer. Images, audio and video are not supported modalities.

## Billing and failure behavior

Every request uses existing authentication, model/group distribution, request-body limits, rate limits and shared HTTP transport. The native handler makes **one upstream attempt** and does not follow redirects. Provider credentials are attached only to validated HTTPS URLs; arbitrary client `X-*` headers are not copied upstream.

Admission physically reserves a conservative **65,536-input-token allowance**, including for trusted/unlimited API tokens. At default pricing/group ratio this is **1,377 internal quota units**; the hold is not a claim that the request actually used that many tokens. A verified receipt replaces the allowance with the actual input charge using checked decimal arithmetic and one upward rounding. For example, 312 input tokens cost 7 quota units regardless of output token count. Settlement applies only `actual - reserved`, preventing double debits, and native output is not committed until the durable balance adjustment succeeds.

Documented admission errors **401, 422, 429 and 529** release the reservation. Other upstream failures, timeouts, interrupted bodies, missing usage or malformed input counters can represent paid work; their reservation is retained and explicitly marked as **estimated**, with `billing_estimated`, `billing_estimate_reason`, and `X-OneAPI-Billing-Estimated: true`. Operators must reconcile these estimates against provider records; they are not claimed to be actual usage. A malformed answer with a valid input receipt still settles the measured input usage. Unrepresentable monetary values or failed durable settlement leave the reservation unresolved and return an error rather than emitting a successful answer.

Native JSON errors and `Retry-After` are preserved when available. Callers may choose explicit backoff for rate limits/capacity errors. Do not blindly retry ambiguous failures: the first attempt may already have incurred a charge. Responses are buffered with an 8 MiB gateway safety cap; an oversized or incomplete response is not silently treated as free work.

## Validation

The regression suite covers request primitives and structured/null descriptions, duplicate/unknown fields, model registry and prices, lossless response forwarding, malformed billing receipts, documented admission errors, exact input-only arithmetic, HTTPS credential handling and no-redirect replay. Tests use local fixtures and TLS mock servers, not paid provider requests. No dependencies or CI workflows are added.

Run with the repository's supported Go toolchain:

```bash
go test ./relay/adaptor/typesafe ./relay ./relay/controller ./router ./controller
go test -race ./relay/adaptor/typesafe ./relay/controller -run 'TypeSafe|SystemOne|NativeResponse|InputQuota|DecodeRequest|ResponseEvidence|HTTPRejections|Redirects|Catalog|URLs'
```
