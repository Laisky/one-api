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

### End-to-end probe

`cmd/test` drives this endpoint against a running server and a real Gemini
channel:

```bash
API_BASE=http://127.0.0.1:3000 API_TOKEN=sk-... go run ./cmd/test live
```

The default suite runs `conversation` (two native turns and persisted token
accounting), `thinking` (Extended Thinking plus rejection of a thinking level on
the ordinary model), `setup-guard` (model switch, TEXT modality, unsupported
setup field, missing setup), and `subprotocol` (browser handshake). Select a
subset with `--scenarios`; duplicate names run only once. `--token` works without
`API_TOKEN`. `--verify-billing=false` disables only the conversation settlement
check, not receipt validation. Conversation and thinking scenarios spend real
provider quota; setup-validation scenarios also establish upstream connections.

The REST negative test is **opt-in and channel-pinned**. In a mixed deployment,
a third-party bridge may legitimately accept the same model over REST. Supply an
unsuffixed admin API token and the ID of a configured Google channel:

```bash
API_BASE=http://127.0.0.1:3000 API_TOKEN=sk-... \
  go run ./cmd/test live --scenarios rest-guard --rest-channel 42
```

`--rest-channel` uses the existing admin-only token channel suffix for this REST
request only. It does not change the credentials of the Live scenarios. An
explicit `rest-guard` without a channel ID fails argument validation before any
provider work. With `--rest-channel` and no explicit scenario list, all five
scenarios run. Only HTTP **400** with `error.code=unsupported_model_transport`
passes; 401, 403, 404, 429, 5xx and a successful third-party bridge do not pass.

Settlement is matched by the upgrade's `X-Oneapi-Request-Id`, not timestamps,
model names or receipt counts. The token-scoped log search is bounded to 100
pages of 20 rows within the settlement deadline. Model aliases do not become log
filters, and unrelated concurrent sessions cannot satisfy the assertion. A
provisional reservation row is polled; a partially reconciled final row fails.
The probe uses the production receipt decoder to check persisted token totals;
independent hand-calculated vectors in the regression tests check normalization.
It is **not** an independent Google invoice or monetary-price oracle. The
thinking scenario validates lifecycle and receipts, not persisted settlement.

The command is split into options, transport, protocol, scenarios and settlement
files. `IN_PROGRESS` followed by `IDLE` is terminal even without a spoken
`turnComplete`; receipt-before-boundary and receipt-after-boundary orders both
work. Transcripts are limited to a 4 KiB preview, and scenario failures are
returned for the command entry point to log once.

### Browser handshakes and operator trust

Browser clients using subprotocol authentication must also offer a non-secret
protocol, for example `gemini-live` alongside
`openai-insecure-api-key.<ONE_API_TOKEN>`. The upgrade reuses the existing
Realtime negotiation helper: it selects a non-authentication protocol and never
echoes or forwards the authentication protocol to Google. The auth-only case
intentionally selects no protocol and is not a supported browser handshake.
The selected label does not translate Gemini frames into OpenAI events.

`base_url` and `endpoint_urls.realtime` are trusted channel-administrator
configuration. The `/api/channel` management route requires `AdminAuth`;
ordinary relay callers cannot choose an upstream destination through query,
body or forwarded-host headers. Administrators intentionally may configure
private gateways. They must be trusted with both channel keys and server egress:
this feature is **not** a network sandbox for hostile administrators. Restrict
admin access and enforce deployment egress rules when internal destinations must
be prohibited. A Google-host-only restriction would break supported gateways.

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
normalizer uses aggregate conservation before refining modality partitions. It
never adds thinking once as output and again as a separate reasoning fee.
Negative/fractional counts, a modality count larger than its parent, and totals
that reconcile with neither `prompt + response` nor
`prompt + response + thoughts + tool` are still rejected. Protobuf omission of a
zero scalar is accepted as zero.

An unexplained modality remainder equal to the tool or thinking count is **not**
proof that the independent counter is inclusive. Explicit additional totals take
precedence over such numerical coincidences. For inclusive input, known tool
modalities refine only their proven deficit from unallocated prompt tokens;
unknown tool modalities remain unallocated. This preserves aggregate counts
without inventing overlap or adding the tool input twice.

**Live receipts do not close their modality breakdown, and their aggregate does
not always include thinking.** Both were verified against the production Live API
on 2026-09-18 and are the normal case, not an error:

- `promptTokensDetails` reports less than `promptTokenCount`. One captured turn
  reported 548 prompt tokens against TEXT 306 + AUDIO 222, and the remainder grew
  by a fixed amount per turn. Google documents the field as "modalities that were
  processed", never as an exhaustive partition.
- `thoughtsTokenCount` can exceed `responseTokenCount` and sit outside
  `totalTokenCount` (100 thinking tokens against an AUDIO-only response of 61).
  Google's Live reference defines the total as prompt + response candidates while
  its REST reference defines it as prompt + thoughts + response candidates; live
  sessions produce both shapes, sometimes within one session.

Demanding closure rejected **every** real receipt in the captured production
incident: a genuine conversation settled at zero, and the metering error closed
the socket after the first turn. The normalizer therefore keeps what the provider
did not attribute in explicit `unallocated_tokens` / `output_unallocated_tokens`
buckets and prices them at the cheapest chargeable modality of that direction,
which for these models is text input and text output. A modality this build does
not price, such as a future `DOCUMENT` or `MODALITY_UNSPECIFIED` bucket, lands in
the same remainder rather than ending a paid session. Thinking first refines
possible inclusive output; only the part that existing text and unallocated
output cannot contain is added outside an inclusive aggregate.

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
`IDLE`. Aggregate input and output must not decrease within that unfinished
scope. Unallocated counts may move into known modalities without falsely marking
the refinement as a regression. A genuine regression retains the earlier,
larger accepted snapshot and marks the evidence incomplete.

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

`realtime_billing_complete` describes **receipt/lifecycle completeness**, not
exact modality pricing. Ordinary input or output unallocated tokens also set
`realtime_pricing_lower_bound=true`, just like uncertain cache allocation.
The current settlement policy finalizes complete measured receipts using their
minimum-rate unallocated buckets. It does not automatically replace that charge
with a reservation estimate or later increase it to a guessed modality price.
A row may therefore be billing-complete **and** a pricing lower bound. Missing
receipt evidence is a separate condition and retains the estimate policy above.
Do not present a lower-bound row as an exact provider invoice.

A rejected receipt at a committed boundary permanently marks that turn as
incomplete, even when a later turn has valid usage. Its earlier measured snapshot
is retained, not silently presented as the complete final amount. The original
decode error also reaches the socket pump so metering can stop the session.
A corrected snapshot within the same still-unfinished turn can repair that
collector state; a different turn cannot. Neither path adds transcript charges.

Gemini reservations use a 120-second allowance at 25 audio tokens/second in both
directions, priced by the actual resolver. This is not a minimum session fee.
Unlike the historical trusted-user optimization, Live retains this reservation
so missing evidence does not become a fabricated free session. Invalid or
unpriceable reservation configurations are rejected before provider work.

## Explicit boundaries

- **Vertex Live is not enabled.** Its endpoint, credentials and pricing require a
  separate implementation; Developer API prices are not silently reused.
- Only the two named 3.8 models are admitted for Live sessions. Older
  Live/transcription models in the catalog are not implicitly declared
  compatible. They are, however, covered by the REST guard: every model whose
  only advertised generation method is `bidiGenerateContent`
  (`gemini-3.1-flash-live-preview`, `gemini-3.5-live-translate-preview`,
  `gemini-3.5-transcribe-live`) is incompatible with a Google REST channel.
  For an unpinned REST request, the relay skips such channels and may use a
  compatible third-party bridge. Local mismatches neither consume nor expand the
  provider retry budget. A terminal provider failure stops replay; a later local
  mismatch cannot hide that real failure behind a transport error. HTTP **400**
  with `unsupported_model_transport` is returned when no compatible provider was
  attempted, or the caller explicitly pinned an incompatible channel. Remediation
  URLs retain and escape the caller's model alias. Internal one-api errors were
  already excluded from channel-health penalties; the fix is not a new exemption.
- Pricing metadata, static model suggestions, and operator-configured model
  catalogs are distinct surfaces. A price entry is not a promise that a model
  supports every transport. Vertex's static suggestions exclude Live-only IDs;
  existing operator configuration is not silently rewritten by this patch.
- Client-executed `functionDeclarations` and matching `toolResponse` messages are
  supported, including non-blocking calls. For Extended Thinking, omitted function
  `behavior` defaults to `NON_BLOCKING`; explicit `BLOCKING` is rejected during
  setup. Responses are bound to outstanding
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

These automated tests use existing Go/CI dependencies and local fixtures. The
captured production receipts above are retained evidence, not fresh provider
calls made by this review. **No paid Google request or account-specific
availability check was made during the `fix/live` follow-up.** Before relying on
exact production invoices, reconcile a consented small real session against
Google's raw modality receipts and billing export, especially Extended Thinking
and proactive silence. Do not replace that evidence with a mocked-test claim.

Additional review regressions cover malformed final usage plus a later valid
turn (ordinary, separate-boundary and background-IDLE cases), reservation-floor
preservation through the production billing resolver, same-turn correction,
actual WebSocket subprotocol responses, ephemeral-token error remediation and
the administrator-only channel-update boundary. Test-only revision
`edccca4e1529a5134c8ea8455291744d31572b0e` precedes the corresponding production
fixes. CI run **35377875469** records the red evidence; final acceptance is
recorded in PR #409 after the fix revision's existing CI completes.

The `fix/live` follow-up preserves the predecessor's fixes and adds test-only
revision `2bbb865e6bd24e89cb1f78355b1bbd19f1349ae0`. Existing CI run
**35414178370**, artifact **10575027928**, records nine top-level assertion
failures before implementation: additional-total ambiguity, inclusive tool
refinement, four retry scenarios in both cache/database modes, malformed probe
receipts, the drifting thinking oracle, and IDLE-only completion. The output
regression preservation control passed unchanged. Accounting, routing and probe
fixes are separate commits; final acceptance and remaining validation limits
are recorded in PR #412. No CI workflow was added or weakened.

Official sources checked on the research date:

- [Pricing](https://ai.google.dev/gemini-api/docs/pricing)
- [Live API reference](https://ai.google.dev/api/live)
- [Live best practices: billing, context, transcription](https://ai.google.dev/gemini-api/docs/live-api/best-practices)
- [Live thinking and interaction status](https://ai.google.dev/gemini-api/docs/live-api/thinking)
- [Gemini 3.8 Live](https://ai.google.dev/gemini-api/docs/models/gemini-3.8-live)
- [Extended Thinking](https://ai.google.dev/gemini-api/docs/models/gemini-3.8-live-extended-thinking)
- [Google Go SDK Live transport and authentication](https://github.com/googleapis/go-genai/blob/main/live.go)
- [Google ADK native Live event handling](https://github.com/google/adk-python/blob/main/src/google/adk/models/gemini_llm_connection.py)
- [Browser WebSocket handshake rules](https://websockets.spec.whatwg.org/)
