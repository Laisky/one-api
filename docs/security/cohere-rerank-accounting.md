# Cohere rerank search-unit accounting

This contract applies only to Cohere channels whose selected model pricing has
`PerCall` metadata. The tariff remains the operator/provider-selected per-search
ratio, multiplied by the selected group rate. Other providers and token-priced
Cohere contracts do not acquire a search-unit multiplier.

## Admission is an allowance, not a measured invoice

The gateway explicitly forwards `max_tokens_per_doc`, using the documented v2
default of 4096 when omitted. Nonpositive limits, more than 10,000 documents,
unsupported structured inputs, unknown model bounds and unrepresentable arithmetic
are rejected before reserving funds or contacting the provider.

The prepaid allowance is calculated using:

```
query_bound = model_context / 2
chunks_per_document = ceil((max_tokens_per_doc + query_bound + 4) / 500)
search_units = ceil(document_count * chunks_per_document / 100)
quota = ceil(search_units * selected_unit_ratio * group_ratio)
```

The four extra slots conservatively cover the special tokens of both supported
model families. All documents count regardless of `top_n`. The calculation uses
truncation caps rather than another model's tokenizer or an assumed bytes-to-token
conversion. Known catalog model bounds cannot be reduced by a pricing-only
operator override. A custom model requires an explicit operator context length
and the same truncation contract; a display name alone is not evidence of it.
Model mappings are resolved before selecting the bound.

This intentionally reserves more than the likely invoice for short inputs. For
example, 100 documents with the default cap on Rerank v3.5 reserve 13 search units,
even when all the actual documents are short. Three such documents reserve one
unit. Rerank v4's larger possible query yields a larger allowance. A lower explicit
document cap may reduce the hold but also changes provider-side truncation.

The 500-token billing rule is distinct from the model's physical context-window
chunking. This allowance implements the published billing/truncation contract;
local synthetic tests do not establish live invoice equivalence or compatibility
with a provider that uses a different private contract. Operators using private or
changed contracts must qualify their meter and tariff before enabling them.

User and finite-token balances reserve the complete quote atomically using the
shared SQL reservation API. Unlimited tokens still reserve the owner's balance.
Concurrent calls cannot authorize themselves from stale cached balances. An
explicit zero operator or group tariff remains free; invalid nonfinite or negative
rates are not silently treated as free admission.

## Settlement and uncertain usage

A validated positive `meta.billed_units.search_units` receipt is carried in a
server-only usage dimension and reconciled through the existing final ledger.
One decimal multiplication and final ceiling avoid intermediate rounding errors.
Measured usage may release the excess reservation or establish additional debt;
a receipt larger than the allowance is not discarded or clamped to the quote.

Missing, null, zero, negative, fractional, string, overflowing or otherwise invalid
receipts do not imply free service. Accepted requests retain the quoted allowance
with explicit estimate provenance when no usable measured receipt is available.
Accepted body-read, decoding, close and client-delivery failures preserve both the
real transport error and any available billing evidence. A provider rejection is
not converted into successful billable usage. Final cost and the consumption log
are reconciled once through the shared billing path.

Provider HTTP errors never create measured usage. Cohere's explicit admission
rejections (400 invalid body, 401/403 authentication, 402 billing limit, 404
unknown resource and 429 rate limit) release the complete hold and leave the
request retryable. Client cancellation (499) and every other error status
follows the existing Cohere
uncertain-execution policy: the quoted allowance is retained once with
`uncertain_upstream_admission_upstream_http_error` provenance and the request is
not replayed. Because the allowance is aggregate, that retained amount scales
with the submitted documents rather than one call.

## Primary provider references

Reviewed on 2026-10-05:

- [Cohere pricing FAQ](https://cohere.com/pricing): 100 documents per search unit,
  with document/query combinations above 500 tokens counting as multiple chunks.
- [Cohere v2 Rerank API](https://docs.cohere.com/v2/reference/rerank): explicit
  `max_tokens_per_doc` truncation, default 4096, and `top_n` result selection.
- [Cohere reranking best practices](https://docs.cohere.com/docs/reranking-best-practices):
  10,000-document limit, model context sizes and half-context query truncation.

## Numeric receipts and independent billing evidence

Search counts are parsed as exact bounded decimal integers: `3`, `3.0` and `3e0`
represent the same three searches. Fractional, nonpositive, out-of-range and
non-numeric values remain invalid. Parsing bounds the input and exponent before
allocating, and never rounds through floating point. Integral token counters also
accept decimal/scientific notation, matching Cohere's numeric metadata schema.

A complete JSON response can contain a valid search receipt alongside unusable
optional token/result fields. Those unrelated decoding errors are still returned,
but the search receipt is independently retained and settled exactly once. A
truncated or malformed JSON response cannot establish this independent receipt.

Primary numeric contracts checked during review:
[ApiMetaBilledUnits](https://github.com/cohere-ai/cohere-python/blob/main/src/cohere/types/api_meta_billed_units.py)
and [ApiMetaTokens](https://github.com/cohere-ai/cohere-python/blob/main/src/cohere/types/api_meta_tokens.py).

## Persisted operator contracts

A custom Cohere-compatible rerank model can declare `context_length` in its
persisted channel model configuration. This field survives JSON normalization
and the shared model resolver. It is used only for otherwise unknown models;
an override cannot shrink the published bound for a known Cohere model. The
operator must verify the same query-truncation contract before configuring a
custom context. An absent contract still rejects before a provider call.

An explicit free-search contract (`ratio: 0` together with
`per_call.usd_per_thousand_calls: 0`) remains free at group ratio one, including
zero-balance owners and finite tokens. The Cohere path recognizes this resolved
contract without falling back through the legacy nonzero-ratio resolver to a
paid provider default. Other provider and generic token pricing are unchanged.
