# Jina AI adaptor

Researched on **2026-09-17** against the [model catalog](https://jina.ai/models/),
[live inference schemas](https://api.jina.ai/openapi.json)
(version `2026.09.17.0130`) and [live model pricing](https://api.jina.ai/v1/models).
The inference schemas, rather than the list of downloadable weights, determine
which models are advertised.

## Configuration

Create a **Jina AI** channel (type **59**), enter the upstream Jina API key, and
select the required models. The default base URL is `https://api.jina.ai`;
a trailing `/v1` is also accepted. Clients continue using their **one-api token**,
not the upstream key. Existing channel IDs and stored channels are unchanged.

Jina base URLs and every endpoint override require **HTTPS**. Validation applies
when creating/updating/loading channel configuration and again at dispatch,
before credentials are attached to the final request URL. Base URLs cannot have
queries; endpoint overrides may. Userinfo and URL fragments are rejected.
Automatic redirects are disabled for Jina, including HTTPS-to-HTTPS redirects:
configure the final endpoint rather than replaying a potentially paid request.
Other providers retain their existing redirect policy.

For local development only, `ONEAPI_JINA_ALLOW_LOOPBACK_HTTP=true` explicitly
permits HTTP to literal loopback IPs such as `127.0.0.1` or `[::1]`. It does not
permit HTTP to private/public networks or DNS names, including `localhost`.
The exception is disabled by default. Tests use local TLS servers or explicitly
exercise this exception; production TLS validation is not disabled for tests.

| Client endpoint | Upstream behavior |
| --- | --- |
| `/v1/embeddings` | Native text/image embeddings with conservative admission |
| `/v1/rerank` | Native text/image ranking |
| `/v1/chat/completions` | Native `jina-ocr-v1` OCR chat, including streaming |
| `/v1/responses` | Existing one-api Chat Completions bridge |
| `/v1/messages` | Existing one-api Claude Messages conversion |

Jina does **not** expose a native Responses or Anthropic Messages endpoint in the
researched schema. Image input is not image generation. The BigModel/Z.AI
`/api/paas/v4/layout_parsing` endpoint is not enabled for this channel.
ReaderLM, Jina-VLM and catalog-only embedding weights are deliberately excluded.

## Models and prices

The adaptor registers **25 unique inference IDs**, with context limits,
modalities, descriptions and pricing in `constants.go`. Use the short IDs below,
not the catalog's `jina-ai/` prefix.

| Family | Registered IDs | Default USD / million input tokens |
| --- | --- | ---: |
| Embeddings v2 | `jina-embeddings-v2-base-{en,zh,de,es,code}` | 0.05 |
| Embeddings v3/v4 | `jina-embeddings-v3`, `jina-embeddings-v4` | 0.05 |
| Embeddings v5 small | `jina-embeddings-v5-text-small`, `jina-embeddings-v5-omni-small` | 0.05 |
| Embeddings v5 nano | `jina-embeddings-v5-text-nano`, `jina-embeddings-v5-omni-nano` | 0.02 |
| Code embeddings | `jina-code-embeddings-0.5b`, `jina-code-embeddings-1.5b` | 0.05 |
| CLIP | `jina-clip-v1`, `jina-clip-v2` | 0.05 |
| ColBERT (embeddings and rerank) | `jina-colbert-v1-en`, `jina-colbert-v2` | 0.05 |
| Reranker v1 | `jina-reranker-v1-{tiny,turbo,base}-en` | 0.05 |
| Later rerankers | `jina-reranker-v2-base-multilingual`, `jina-reranker-m0`, `jina-reranker-v3`, `jina-reranker-v3.5` | 0.05 |
| OCR chat | `jina-ocr-v1` | 0.50 input; 2.00 output |

Search models are **token-priced**, not billed per request. A verified response's
`usage.total_tokens` accounts for all search input, including image processing
and reranking work. It is normalized into `prompt_tokens` for the existing
billing pipeline. Missing, malformed, duplicate, negative, out-of-range or
all-zero usage on an admitted nonempty request is **not free**: a conservative
charge is retained and explicitly marked estimated. An upstream-response error
is also reported. Once output has started, its HTTP status cannot change and the
gateway does not append an unrelated error JSON object or replay the request.
Channel-specific pricing overrides remain available; published defaults are not
a guarantee of an individual account's negotiated or prepaid-credit costs.

## Request and response details

The gateway currently admits **text and supported image inputs only**. PDF,
audio, video and grouped inputs are rejected before upstream dispatch because
this integration has no verified billable-work bound for those forms. Catalog
modalities describe this gateway admission policy, not every upstream capability.
The adaptor forwards `task`,
`embedding_type`, `normalized`, `truncate`, `late_chunking`, `return_multivector`
and `return_tokenized_input`. These can be top-level fields or embedding
`extra_body` fields; top-level native fields take precedence. OpenAI
`encoding_format` maps to `embedding_type` unless a native value is supplied.
Explicit `false` values survive conversion. Model-specific constraints, such as
v4 multi-vector/dimension incompatibility, remain validated by Jina.

Rerank accepts strings and native `{ "text": "..." }` / `{ "image": "..." }`
documents; image queries and documents require a supporting model such as m0.
The model-aware check runs at admission and direct adaptor conversion.
Native `return_documents`, `max_doc_length` and `return_embeddings` are forwarded
from the top level. The legacy `max_tokens_per_doc` field maps to `max_doc_length`
for v3/v3.5 only. Other adaptors must explicitly opt into structured rerank input;
they never receive image-accounting placeholders as actual documents.

Dense, Base64, sparse and multi-vector response payloads and provider extensions
are preserved without decoding vectors into a fixed float slice. Clients using
native multi-vector or sparse output need a compatible response parser.
For OCR, legacy `max_tokens` maps to `max_completion_tokens`; an explicit native
limit wins. Streaming requests ask the upstream for usage to retain image-token
billing accuracy. OCR is not advertised as a general-purpose tool-calling model.

## Conservative billing contract

Paid Jina calls require a **synchronous database reservation** for every account,
including high-balance users and unlimited tokens. Unlimited means no token-level
limit, not a free user balance. Reservations bypass in-memory batch updates.
Text allowances include context and a byte-based margin; image allowances use
model context rather than URL length. Rerank budgets include every document and
repeated query work, regardless of `top_n`. OCR applies an explicit output limit
(up to 8,192, also its default) upstream so the budget and request agree.

Valid receipts settle at the configured flat token prices using decimal-rational
arithmetic, with a single upward rounding to the smallest quota unit. Normal
requests refund unused reservation. Missing/invalid receipts retain at least the
reservation, preserve higher observed usage evidence and label consume metadata
with `billing_estimated`, `estimated_charge` and `billing_estimate_reason`.
These allowances are conservative policy estimates, not a guarantee of an
arbitrary or changing provider invoice. Ambiguous calls require reconciliation.

The admission token limit also bounds **certifiable** receipt counters. Larger
counters are unverified evidence, not a successful measured receipt. They are
retained for conservative decimal settlement rather than discarded: a reservation
is not a cap forgiving higher observed costs. Final arithmetic never falls back
to an unchecked floating-point result. Costs above the monetary representability
ceiling (`1 << 52` quota units) retain that bounded estimated amount and emit a
reconciliation error; this is not a claim to represent an arbitrarily large bill.
OCR receipt certification also requires no active body read. Closing an active
reader leaves the receipt uncertain even before it returns later bytes or errors.

Requests rejected locally before dispatch are refundable. After dispatch, only
classified **pre-inference admission rejections** (401, 403, 404, 422 and 429)
qualify for a synchronous refund before retry. A generic **400 response is not
proof of no cost** and is not in that refundable class. Timeouts, 5xx responses,
broken streams and missing usage after possible dispatch retain a conservative
charge and **do not automatically replay paid work**. Failed refunds block replay
too. Client cancellation does not cancel settlement. Known consumed work that
exceeds the remaining balance is recorded as debt instead of silently discarded;
future admission remains guarded.

Only verified catalog models, bounded inputs and flat token pricing are admitted.
An intentional zero group/model rate remains free; invalid, non-finite or
unrepresentable admission rates are rejected. Tiered/per-call Jina overrides are
rejected until their reservation contracts are implemented. See the
[billing audit](../../../docs/audits/20260917-jina-billing.md) for tests and
important system-wide residual risks, including crash recovery and non-Jina paths.

## Examples

Send this body to one-api's `/v1/embeddings` with a one-api Bearer token:

```json
{
  "model": "jina-embeddings-v3",
  "input": ["A document to index"],
  "task": "retrieval.passage",
  "normalized": true,
  "encoding_format": "base64"
}
```

Text ranking through `/v1/rerank`:

```json
{
  "model": "jina-reranker-v3.5",
  "query": "Which document describes refunds?",
  "documents": ["Refund policy", "Opening hours"],
  "top_n": 1,
  "return_documents": false
}
```

Tests cover canonical model mapping, native options, false values, URL/auth
handling, shared Messages conversion, response shape preservation, usage errors,
structured-input fallback safety, catalog prices, and channel/UI registration.
No paid live API tests or new CI workflows are included.
