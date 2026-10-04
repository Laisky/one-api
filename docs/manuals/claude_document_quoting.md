# Native Claude document quota estimates

Native PDF/document input is not a literal base64 prompt. Admission counts the
document's textual metadata and reserves a source allowance without tokenizing
binary transport data or opaque file/URL handles. The original provider request
is unchanged. Text documents remain text. Converted tool results that actually
send serialized document/base64 JSON as prompt text are quoted as that text;
they do not use the native allowance.

`CLAUDE_NATIVE_DOCUMENT_TOKEN_ALLOWANCE` is an operator-only startup setting in
input-token units **per opaque document**, including documents in tool results.
The default is **32768** and the inclusive allowed range is **1..1048576**.
An unset value uses the default; empty, zero, negative, fractional, overflowing
or out-of-range explicit values fail startup. A request's page/token hints cannot
change the setting. Multiply the allowance by the configured model and group
input tariff; ordinary output reservation is additional. Explicit free tariffs
remain free. Multiple documents consume additive allowances.

This is a **prepaid gateway estimate**, not a new provider fee, an exact page
count, or a guaranteed maximum cost. It is deliberately independent of file
compression and encoding size. Select the value for the operator's admitted
document workload, model and risk tolerance. Complete measured provider usage
reconciles the hold once; missing/invalid usage retains the existing conservative
hold and estimated-billing provenance. A valid larger receipt may establish debt
for already performed work. This setting alone does **not** establish a strict
aggregate spending ceiling for arbitrarily large/dense documents.

No remote URL/file fetch, PDF parser, decompressor, credential-bearing counting
request or external executable is introduced into admission. Enforce document
limits appropriate to the selected provider separately. The provider's native
PDF processing uses extracted text and page images; transport byte length is not
a semantic token count. Official references, checked 2026-10-04:

- https://platform.claude.com/docs/en/build-with-claude/pdf-support
- https://platform.claude.com/docs/en/build-with-claude/token-counting

Regression coverage uses two valid one-page PDFs with identical pixels/text but
raw versus Flate image encoding, plus actual local native/converted HTTP and
owner/token/log settlement. These fixtures prove gateway semantics, not live
provider invoice equivalence.
