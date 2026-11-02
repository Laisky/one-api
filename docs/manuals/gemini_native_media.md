# Native Gemini media through the gateway

The native Gemini and Vertex Gemini adapters share the same media converter.
Model names are resolved through channel mapping before capabilities or reasoning
levels are checked. Text/media ordering is preserved and unsupported content is
rejected before generation; it is not silently dropped.

## Client inputs

Chat Completions accepts a video part:

```json
{"type":"video_url","video_url":{"url":"data:video/mp4;base64,..."}}
```

A public HTTPS URL can replace the data URL. Supply `mime_type` in the `video_url`
object when the URL does not identify its container. PDFs use the standard nested
Chat file shape, while existing flat gateway file fields remain accepted:

```json
{"type":"file","file":{"filename":"brief.pdf","file_data":"data:application/pdf;base64,..."}}
```

`file_url` is supported for public HTTPS references. Raw base64 requires an
explicit `mime_type` or a recognized filename. Exactly one of `file_data`,
`file_url`, or `file_id` may be supplied; unresolved `file_id` is rejected.

Responses requests can use `input_file` with `file_data`/`file_url`, and the
**gateway extension** `input_video` with `video_url`. These media extensions are
opt-in for native Google channels: other Responses fallbacks retain their
existing portability restrictions.

Claude Messages accepts standard PDF `document` sources (`base64` or `url`).
The **gateway extensions** `video` and `audio` use the same source envelope with
a matching `media_type`; these are not claims about Anthropic's native API.
Supported `input_audio` Chat/Responses parts are forwarded only to Gemini models
whose catalog permits audio. Files cannot disguise unsupported modalities.

## Limits and ownership

Inline media is validated without downloading or copying another decoded payload.
The converter bounds aggregate new inline media at 100,000,000 decoded bytes and
individual PDFs at 50,000,000 bytes; the gateway HTTP body limit (including base64
expansion) and Google's model/provider limits also apply. Oversized inputs should
use public references within the provider's documented URL limits.

The gateway does not fetch these media URLs. Only credential-free public HTTPS
references are forwarded. Local/private addresses and private Google Files API
URLs, opaque file IDs and `gs://` references are rejected: using a shared channel
credential is not evidence that the caller owns a private upstream file. An
ownership-aware upload/file resolver is outside this change.

## Reasoning and accounting

`gemini-3.1-flash-lite-image` defaults to `minimal` and accepts explicit `minimal`
or `high`. Unsupported levels produce a validation error. Chat `reasoning_effort`,
Responses `reasoning.effort`, and Claude `output_config.effort` converge on the
same native `thinkingConfig.thinkingLevel`; conflicting controls are rejected.

Video/audio/PDF prompts use the mapped provider's authenticated `countTokens`
endpoint before quota reservation. Counting failures stop generation rather than
falling back to a text-only quote. The counted native payload is reused for
Responses generation, and output limits are quoted from the prepared payload.
Measured native audio receipts apply the configured audio/text rate multiplier;
missing cache/modalities remain explicitly estimated, not silently priced as text.
Authoritative usage settles the request once. Streaming and non-streaming image
outputs retain the gateway's existing text/data-URL representation, including
Claude-compatible output; this change does not introduce a new output schema.

The catalog is not blanket authorization for every provider tool. Existing
built-in tool admission, model-specific restrictions and private-file ownership
checks remain in force. No new Google Search implementation is included here.

## References (checked 2026-11-02)

- https://ai.google.dev/gemini-api/docs/models/gemini-3.1-flash-image
- https://ai.google.dev/gemini-api/docs/models/gemini-3.1-flash-lite-image
- https://ai.google.dev/gemini-api/docs/generate-content/file-input-methods
- https://ai.google.dev/gemini-api/docs/generate-content/thinking
- https://ai.google.dev/api/tokens
- https://ai.google.dev/api/generate-content
- https://docs.cloud.google.com/vertex-ai/generative-ai/docs/model-reference/count-tokens
