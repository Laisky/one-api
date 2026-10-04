# Native Claude document quota estimates

Native base64 PDF admission counts the decoded file size at a configurable linear
rate, plus the document's textual metadata. It does not tokenize the encoded
binary transport as literal prompt text. The provider payload is unchanged.

`CLAUDE_NATIVE_PDF_TOKENS_PER_KIB` is a trusted positive integer startup rate.
For each native `application/pdf` base64 source the estimate is:

```
ceil(decoded_bytes * tokens_per_KiB / 1024)
```

There is **no policy cap on estimated tokens**. The result increases with file
size at the configured ratio; integer token units are rounded up separately for
each PDF. Multiple documents add their estimates, including nested tool results.
ASCII space, tab, CR and LF in base64 transport do not increase decoded size or
change the provider payload. Missing/non-string/empty or malformed PDF base64
rejects before provider dispatch. Decoding streams into bounded discard storage;
no second decoded document buffer, parser or decompressor is introduced.

The **tentative default for review is 64 tokens/KiB**, equivalent to one token per
16 decoded bytes. Examples at that proposed rate:

| Decoded size | Estimated document tokens |
| --- | ---: |
| 1 byte | 1 |
| 16 bytes | 1 |
| 17 bytes | 2 |
| 64 KiB | 4096 |
| 64 KiB + 1 byte | 4097 |
| 128 KiB | 8192 |
| 1 MiB | 65536 |
| 32 MiB | 2097152 |

The user approved the linear policy shape; this numerical default remains a
**calibration choice awaiting review**, not an empirically measured rate or a
hard funding upper bound. Keep the PR in draft and settle the default rate before
merge. Operators can configure another positive representable integer rate;
there is no arbitrary maximum rate. Invalid explicit syntax, zero, negative,
fractional or unrepresentable integers fail startup rather than silently falling
back. Request-supplied page/token hints cannot change the trusted rate.

Existing request-body/resource limits remain separate from quotation. The relay
body budget (`MAX_REQUEST_BODY_SIZE_MB`, default 128) controls accepted request
and decompressed-body bytes; an operator can configure that limit. No extra PDF
size cap or estimate clamp is added here. If an estimate, aggregate token count
or priced quota cannot be represented, admission returns an error before
reservation/dispatch. Representability checks reject; they never clamp to a
smaller estimate or turn overflow into a one-unit paid hold.

Unknown-size URL/file-ID sources are **not fetched**. They retain the separate
`CLAUDE_NATIVE_DOCUMENT_TOKEN_ALLOWANCE` fallback (default 32768, startup range
1..1048576). Non-PDF opaque sources also retain that existing fallback. Its range
limits the configurable unknown-size fallback, not the linear PDF estimate.
Titles, context, citations and other textual metadata remain charged. Text
documents remain text. Converted routes that actually serialize documents or
base64 into prompt text retain quotation of that literal outgoing text.

Complete measured upstream usage remains authoritative and reconciles the hold
once, including a smaller measured receipt. Missing/invalid usage retains the
existing estimated hold and provenance; the native incomplete-receipt response
contract remains unchanged. Existing explicit-free tariffs stay free. Ordinary
paid admission keeps its existing truncation/minimum behavior; native MCP rounds
keep their existing truncation without introducing a new minimum.

FIXME: Decoded PDF size is an imperfect content proxy. Compression, image/page
content, density and inert padding can cause both under- and overestimation.
Equal rendered pages can intentionally receive different size-based estimates.
Replace or calibrate this heuristic with capability-aware provider counting or
bounded extraction and model-specific page accounting when available. It is not
a guaranteed cost ceiling for all PDFs. No provider counting request or local
PDF executable is introduced by this change.

References for native processing and optional provider counting:

- https://platform.claude.com/docs/en/build-with-claude/pdf-support
- https://platform.claude.com/docs/en/build-with-claude/token-counting

Retained regressions use independent byte/rate arithmetic, valid raw/Flate PDFs,
actual bounded local HTTP dispatch and isolated SQLite settlement. They verify
policy behavior, rounding, overflow, payload preservation and receipt authority;
they do not assert live provider invoice equivalence. The earlier lazy traversal
and zero-allocation empty-document scan regressions remain retained.
