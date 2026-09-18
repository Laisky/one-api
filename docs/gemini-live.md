# Gemini Live: native conversations and receipt-based billing

Research date: **2026-09-18**. This extends the catalog-only scope of
`gemini-models-20260918.md`. The REST generation guard remains intentional;
bidirectional conversations use a separate WebSocket transport.

## Public contract

Connect to `GET /v1/realtime?model=gemini-3.8-live` or
`gemini-3.8-live-extended-thinking`, using an ordinary **one-api** bearer token.
Native Gemini and Gemini OpenAI-compatible channels expose this endpoint.
Normal authentication, token model permissions, channel selection, model mapping,
quota reservations and durable settlement remain in the existing request path.
Channels with an explicit endpoint allowlist must add `realtime`; existing
operator-owned restrictions are not overwritten.

The frames are **Gemini-native JSON, not OpenAI Realtime events**. Reusing the
`/v1/realtime` route does not make OpenAI's `session.update`, `response.create`,
or `input_audio_buffer.append` valid Gemini operations. The channel selects the
wire protocol, even when its other endpoints are OpenAI-compatible.

The first frame is `setup`. Its model is pinned to the model authorized and
mapped during the HTTP handshake. The caller can supply the public alias or the
bound model, but cannot change the model inside an established connection.
Wait for Google's `setupComplete` before sending conversation frames.

```json
{"setup":{"model":"gemini-3.8-live","generationConfig":{"responseModalities":["AUDIO"]},"inputAudioTranscription":{},"outputAudioTranscription":{}}}
```

Send text as a native realtime input:

```json
{"realtimeInput":{"text":"Hello. Please introduce yourself."}}
```

Send PCM audio as base64 inside JSON, not as raw binary audio frames:

```json
{"realtimeInput":{"audio":{"mimeType":"audio/pcm;rate=16000","data":"BASE64_PCM16_MONO"}}}
```

Use `audioStreamEnd` when input stops. Receive native audio parts, transcripts,
interruption/turn-completion events, function calls, cancellations and `goAway`.
The server audio format is PCM16 mono at 24 kHz; input PCM is normally 16 kHz.
Text responses are audio transcriptions, not a request for `TEXT` response
modality. Preserve client playback cancellation on `interrupted`.

For the Extended Thinking model only, `generationConfig.thinkingConfig` accepts
`thinkingLevel: "LOW"`, `"MEDIUM"`, or `"HIGH"`. The ordinary model thinks
automatically and rejects a configurable thinking level. Do not interpret an
`IN_PROGRESS` interaction as finished simply because the model completed a
short spoken acknowledgement. Background function calls and later speech must
continue to flow; `IDLE` ends the interaction.

Google's current 3.8 thinking guide uses `v1alpha`; that is the default upstream
version. An administrator can explicitly select `v1beta`. Upstream URLs are
constructed from the channel base URL or its existing endpoint override. TLS is
required except for literal loopback addresses used by tests. Authentication is
sent as `x-goog-api-key`, never in the URL or by forwarding the user's token.

## Pricing: do not copy GPT-Realtime's formula blindly

Published standard paid-tier prices for both supported models, **USD per million
tokens**:

| Bucket | Price | Accounting rule |
| --- | ---: | --- |
| Input text | 0.75 | Includes text context reported by Google. |
| Input audio | 3.00 | Audio retained in conversation history remains audio input. |
| Input image | 1.00 | Input-token charge, not a generated-image fee. |
| Input video | 1.00 | Input-token charge, not a video-output per-second fee. |
| Output text | 4.50 | Includes generated transcription and thinking when reported in this partition. |
| Output audio | 12.00 | Separate from text/transcription output. |

The repository's realtime audio formula is:

```text
text_input_rate  = model_ratio
text_output_rate = model_ratio * completion_ratio
audio_input_rate = model_ratio * audio.prompt_ratio
audio_output_rate = audio_input_rate * audio.completion_ratio
```

Therefore `audio.completion_ratio` must be **12 / 3 = 4**, not 12 / 4.5.
The latter produced an actual $8/M audio-output charge. The test-only commit
`a2d72a1fa25718a34f8043f0225760fafa59ae10` proved this through `quota.Compute`,
not by repeating the metadata's mistaken multiplier convention. Its image-input
case also exposed an unresolved input price, while text and audio-input controls
passed. Red CI: **35364947351**, packages artifact **10556415885**.

Visual input prices use a dedicated realtime supplement. The image-generation
and video-output configuration fields are not repurposed to invent output fees.
Channel input-price markups and group multipliers remain effective; a positive
image-input prompt-ratio override applies to the shared image/video input rate.
No GPT cache discount is inferred for Gemini Live.

### Transcripts, history and thinking

**Both input-audio and output-audio transcription generate billable output
text.** This is in addition to audio usage. The transcript event itself is not a
second receipt. Do not count its characters, feed it to a local tokenizer, charge
it at an independent Whisper model price, or add an audio surcharge after receipt
pricing. Only Google's modality counters determine the token quantities.

**Prior context is billed again on subsequent turns.** Sum authoritative turns;
do not use the last prompt count as the entire session charge, or subtract the
previous turn's prompt count. Context-window compression can reduce the next
turn's context; that is not a negative charge or a refund of a previous turn.

`thoughtsTokenCount` is retained as a subset of normalized output text. The
normalizer requires aggregate conservation to decide whether thinking/tool-use
counts are already inclusive or explicitly additional. It never adds thinking
once as output and again as a separate reasoning fee. Unknown residuals, missing
nonzero modality splits, negative/fractional counts and contradictory totals
require reconciliation rather than a guessed cheap-text allocation. Protobuf
omission of a zero scalar is accepted only when totals and partitions reconcile.

Example: 100K text, 200K audio, 300K image and 400K video input tokens, plus
200K text and 300K audio output tokens, cost **$5.875**. A 100K thinking subset
inside that text output does not add another $0.45. Group ratio 2 yields $11.75;
ratio 0 remains free. Session rounding occurs once, not once per streamed chunk.

### Receipt lifecycle and uncertainty

The collector accepts **only upstream** `usageMetadata`. Client-shaped usage is
rejected before forwarding. Audio chunks, transcript deltas, cancellations and
function arguments/results are passed through but are never usage receipts.

The implemented receipt contract coalesces monotonic refinements within an
unfinished server turn and commits its final snapshot at completion. Duplicate
suppression is turn-local: two different turns can legitimately have identical
counts. Extended Thinking's `IN_PROGRESS` spoken segments remain pending until
`IDLE`. Decreasing counters within that unfinished scope are treated as an
unsupported/ambiguous receipt sequence, not silently subtracted or double-added.

**Public documentation does not provide a unique receipt ID or a complete
ordering guarantee for every usage/transcription/background-thinking frame.**
The tests exercise this explicit normalization contract; they are not captured
proof of every production backend sequence. In particular, a fully replayed
terminal frame with no identity cannot be distinguished from a new equal-cost
terminal turn. There is no automatic reconnect or paid-input replay. A schema or
sequence that fails validation closes the metered session and is flagged for
reconciliation instead of continuing unpriced work indefinitely.

Both readers join before settlement. After downstream loss, the upstream reader
has a bounded two-second drain for final usage. Already measured partial work is
retained. Known idle/zero sessions refund the reservation. Unresolved work keeps
`max(reservation, observed charge)` as an explicitly labeled **estimate**, not an
invented exact bill. Concurrent input after the start of a final response can
remain unresolved without a later receipt; clients should end input and wait
for the final usage/turn boundary before closing. Inspect
`realtime_billing_complete`, billing issues and estimated-charge metadata.

Gemini reservations use a 120-second allowance at 25 audio tokens/second in both
directions, priced by the actual resolver. This is not a minimum session fee.
Unlike the historical trusted-user optimization, Live retains this reservation
so missing evidence does not become a fabricated free session. Invalid or
unpriceable reservation configurations are rejected before provider work.

## Explicit boundaries

- **Vertex Live is not enabled.** Its endpoint, credentials and pricing require a
  separate implementation; Developer API prices are not silently reused.
- Only the two named 3.8 models are admitted. Older Live/transcription models in
  the catalog are not implicitly declared compatible.
- Client-executed `functionDeclarations` and matching `toolResponse` messages are
  supported, including non-blocking calls. Responses are bound to outstanding
  call IDs within the connection. Unsolicited, duplicate or cancelled responses
  are rejected. Streaming partial function-response updates are not enabled.
- Paid built-in search/code/URL tools are rejected in setup. Google lists search
  at $14/1,000 queries after a shared allowance, but the adapter has no reliable
  per-session query receipt for this path. It does not silently allow unbilled
  search or assume each tenant owns the account's free allowance.
- Session resumption and ephemeral-token/WebRTC minting are not enabled. A client
  cannot inject a resumption handle from another session. Reconnect explicitly
  with a fresh authorized session; no hidden upstream retry is performed.

Each socket has a 4 MiB message limit, bounded setup and write deadlines, a
15-minute session ceiling, bounded pending function IDs and the shared receipt
capacity. JSON duplicate keys and excessive nesting are rejected. Logs/audits
contain counters and bounded diagnostics, not audio, transcripts, keys or tool
payloads. Upstream error text is not used as a close-frame diagnostic.

## Validation and source record

The test suite covers real bidirectional local WebSockets; model pinning and
credential isolation; text/audio/transcript/function/cancellation frames; binary
JSON; late usage after disconnect; forged client receipts and repeated setup;
price vectors through the production resolver; history/compression/duplicates;
zero omissions and invalid counters; thinking/tool conservation; partial/idle
settlement; group/override/rounding behavior; and bounded ledger/tool state.

These tests use existing Go/CI dependencies and local fixtures. **No paid Google
request or account-specific availability check has been made.** Before relying
on exact production invoices, reconcile a consented small real session against
Google's raw modality receipts and billing export, especially Extended Thinking
and proactive silence. Do not replace that evidence with a mocked-test claim.

Official sources checked on the research date:

- [Pricing](https://ai.google.dev/gemini-api/docs/pricing)
- [Live API reference](https://ai.google.dev/api/live)
- [Live best practices: billing, context, transcription](https://ai.google.dev/gemini-api/docs/live-api/best-practices)
- [Live thinking and interaction status](https://ai.google.dev/gemini-api/docs/live-api/thinking)
- [Gemini 3.8 Live](https://ai.google.dev/gemini-api/docs/models/gemini-3.8-live)
- [Extended Thinking](https://ai.google.dev/gemini-api/docs/models/gemini-3.8-live-extended-thinking)
- [Google Go SDK Live transport and authentication](https://github.com/googleapis/go-genai/blob/main/live.go)
- [Google ADK native Live event handling](https://github.com/google/adk-python/blob/main/src/google/adk/models/gemini_llm_connection.py)
