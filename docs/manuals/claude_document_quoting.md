# Native Claude document quota estimates

Native Claude Messages requests (Anthropic, AWS Bedrock and Vertex passthrough)
can carry `document` blocks. Before dispatch, one-api reserves quota for them
with a gateway estimate; complete provider usage receipts settle the real cost
afterwards. The provider payload is never modified by the estimate.

## Provider facts (verified 2026-10)

- [PDF support](https://platform.claude.com/docs/en/build-with-claude/pdf-support):
  every page is rasterized to an image and its text is extracted; text
  "typically uses 1,500-3,000 tokens per page depending on content density",
  and the page image is billed like any image. Limits: 600 pages per request
  (100 when the context window is under 1M tokens) and 32 MB per request
  ([Bedrock 20 MB, Google Cloud 30 MB](https://platform.claude.com/docs/en/api/overview)).
- [Vision](https://platform.claude.com/docs/en/build-with-claude/vision): one
  image costs at most 4,784 tokens on the high-resolution tier (Claude 4.7 and
  later) and 1,568 tokens on the standard tier.
- [Token counting](https://platform.claude.com/docs/en/build-with-claude/token-counting):
  the Claude 4.7+ tokenizer produces about 30 percent more tokens.
- [Context windows](https://platform.claude.com/docs/en/build-with-claude/context-windows):
  the largest window is 1M tokens; an input larger than the model's window is
  rejected, not billed.

## Native base64 PDFs: page-based estimate

```
pages_per_document = conservative page signal (see below), 1..600
request_pages      = min(sum(pages_per_document), 600)
pdf_tokens         = min(request_pages * CLAUDE_NATIVE_PDF_TOKENS_PER_PAGE, 1,000,000)
```

`CLAUDE_NATIVE_PDF_TOKENS_PER_PAGE` (operator-only, integer 1..1048576,
default **8,684** = 3,000 text tokens x 1.3 tokenizer growth + 4,784 for the
largest page image) prices one page. Deployments that only serve
standard-resolution models may lower it (for example 3,900 + 1,568 = 5,468).
Document titles, context and citations are still counted as text.

The page signal is a bounded lexical scan of the decoded bytes, without
rendering or following references:

- structural `/Type /Page` dictionaries (with `#xx` name escapes and comments
  handled as PDF readers do);
- the largest `/Count` of a page-tree dictionary (outline counts are ignored);
- every reference in page-tree `/Kids` arrays, so one page object referenced
  many times counts once per reference (name-tree and form-field `/Kids` are
  excluded unless the dictionary also looks like a page tree);
- compressed object streams (PDF 1.5) are inflated with zlib under a shared
  per-request budget (16 MiB output, 4,096 streams).

The estimate is the maximum of these signals. A document is treated as using
the full 600-page limit when the scan cannot see its page objects: encryption,
object streams with other filters or predictors, an object stream that cannot be
decoded or was not inspected, an indirect `/Kids` array, an exhausted
decompression budget, more than 32 MiB of decoded bytes, or no page evidence at
all. Malformed, empty or non-string base64 rejects before dispatch.

Calibration against `pdfinfo` page counts on 54 real PDFs (papers, slides,
reports, a 758-page book capped at 600): no under-estimate, no false fallback,
median page ratio 1.15, maximum 2.46 (incrementally updated files whose page
objects appear in two revisions); the scan took at most 0.2 s per file. A
valid 1.6 KB PDF declaring 100 text-dense pages, which the former
64 tokens/KiB rule quoted at 155 tokens, now reserves 868,457 tokens. Raw and
compressed encodings of the same page now reserve the same amount.

## Other document sources

Unknown-size URL and file-ID sources are not fetched, and non-PDF binary
documents are not parsed. They keep the separate
`CLAUDE_NATIVE_DOCUMENT_TOKEN_ALLOWANCE` (default 32,768, range 1..1048576)
per document. Text documents remain text. Converted routes that actually
serialize documents or base64 into prompt text keep quoting that literal
outgoing text.

## Settlement and residual risk

Complete measured upstream usage remains authoritative and reconciles the hold
once, including a smaller receipt. Missing or invalid usage keeps the estimated
hold with estimate provenance. Existing free tariffs stay free; unrepresentable
token or quota sums reject before reservation.

The estimate is not a guaranteed bound. Pages denser than the documented
typical text cost, and PDFs crafted to exploit reader-specific leniency (for
example stream-length mismatches or page trees disguised as name trees), can
still cost more than the reservation. That exposure is bounded per request by
the provider's context window, and complete receipts still bill the real
usage. File-ID and URL PDFs can reach the page limit while reserving only the
fallback allowance.
