# Gemini 3.8 speech compatibility

This implementation extends PR #438 beyond the initial catalog-only audit. The
transport limitations recorded in `../../GEMINI_PROVIDER_AUDIT_20260930.md` describe
the initial catalog commit, not this speech implementation.

## Endpoint and models

`POST /v1/audio/speech` accepts the standard speech request shape for
`gemini-3.8-flash-tts` and `gemini-3.8-flash-lite-tts`. Authentication, channel
selection, model aliases, quota admission, and billing use the existing gateway.
Google Gemini and Gemini OpenAI-compatible channels both use Google's native
GenerateContent speech protocol, not Google's OpenAI-compatible chat endpoint.
Vertex AI uses native project routing and ADC, but **requires explicit channel
input and output token prices** before dispatch. Configure Vertex cache and
promotional credit policy independently; Developer API prices are not an
entitlement or billing guarantee for Vertex projects.

```sh
curl "$ONE_API_BASE_URL/v1/audio/speech" \
  -H "Authorization: Bearer $ONE_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.8-flash-lite-tts",
    "input": "Hello! These words are spoken exactly as written.",
    "voice": "Kore",
    "instructions": "Warm and relaxed",
    "response_format": "wav",
    "extra_body": {"gemini": {"max_output_tokens": 1024}}
  }' --output speech.wav
```

`input` remains a verbatim transcript. `instructions` becomes
`parts[].speech_metadata.style`, never a prefix spoken aloud. Use Gemini voice
names or a project-authorized voice ID/key; OpenAI voice names are not silently
mapped. Single-speaker custom voice strings are forwarded without logging their
values. Creating/listing/cloning voices and consent management are not implemented
by this endpoint. OpenAI custom-voice **objects** are not accepted.

## Formats and streaming

Supported output formats are `mp3` (default), `opus` (Ogg), `aac` (ADTS), `flac`,
`wav`, and `pcm`. PCM is headerless signed 16-bit **little-endian**, mono, 24 kHz.
WAV parsing walks RIFF chunks and emits one exact-length header; it does not assume
that every upstream WAV has a 44-byte header. The native request explicitly asks
for `AUDIO_L16` at 24 kHz. Valid upstream WAV is also understood.

WAV/PCM at speed 1 work without ffmpeg. Other codecs and speed changes (0.25-4)
require ffmpeg on `PATH`, checked before reservation. Conversion uses a bounded,
context-cancellable process with fixed arguments and no shell, filenames, URLs,
transcripts, or caller-defined filters. It does not alter billable upstream tokens.

`stream: true` selects native `streamGenerateContent?alt=sse` and binary output.
`stream_format: "sse"` selects native streaming and standard-style
`speech.audio.delta` / `speech.audio.done` events with base64 audio. For low-latency
incremental delivery, choose **PCM at speed 1**. Other formats/speed changes buffer
the bounded native audio before conversion, then return chunks. They are not
advertised as low-latency transcoding. SSE error paths emit `error`, not a false
success event or the chat `[DONE]` sentinel. This is not the WebSocket Realtime or
Gemini Live protocol.

```json
{
  "model": "gemini-3.8-flash-tts",
  "input": "Hello there!",
  "voice": "Kore",
  "response_format": "pcm",
  "stream_format": "sse",
  "gemini": {"max_output_tokens": 512}
}
```

A valid `MAX_TOKENS` response is a **terminal truncated success**: buffered audio
is encoded and returned instead of discarded, and trailing usage is still read.
`speech.audio.done` includes `truncated: true`; unary and buffered responses also
include `X-Gemini-Finish-Reason: MAX_TOKENS`. An already-started binary PCM stream
cannot retroactively gain that header. Refusals, invalid audio, exceeded byte/token
budgets, and invalid or regressing usage remain errors.

## Optional two-speaker extension

The gateway accepts `gemini` at the top level or inside the existing `extra_body`
normalization. Unknown fields are rejected rather than silently discarded.
Exactly two prebuilt voices and 2-128 turns are supported; turn text concatenation
must equal `input` exactly. Omit the top-level `voice` in this mode.

```json
{
  "model": "gemini-3.8-flash-tts",
  "input": "Hello Jane!Hi Joe!",
  "response_format": "wav",
  "gemini": {
    "speakers": [
      {"speaker": "Joe", "voice": "Puck"},
      {"speaker": "Jane", "voice": "Kore"}
    ],
    "turns": [
      {"speaker": "Joe", "text": "Hello Jane!", "style": "cheerful"},
      {"speaker": "Jane", "text": "Hi Joe!", "style": "relaxed"}
    ],
    "max_output_tokens": 1024
  }
}
```

## Billing and failure behavior

Speech is billed by text input, cached input, and audio output **tokens**, not
input characters. The pricing snapshot is frozen at request start and uses the
existing channel/provider/global resolution and time windows. Explicit audio
multipliers and legacy scalar/completion overrides remain effective. Buckets are
summed with decimal arithmetic and rounded once. The temporary reservation covers
8,192 input tokens and the requested output budget (default 16,384); unused quota
is released at settlement. The shared trusted-balance reservation optimization
remains in effect. Accepted requests are charged except for the explicit gateway
encoder-failure credit below. Audio overrides are resolved from the full effective
configuration, including time windows; an empty audio block is not an override.

Cumulative upstream usage snapshots replace earlier snapshots instead of being
added per audio chunk. Missing, negative, regressing, or oversized counters cannot
be treated as authoritative complete usage. Successfully received audio marks the
existing accepted-work flag **before** writing downstream, preventing automatic
replay and erroneous refunds after client disconnect. Before any valid audio is
received, failures release the reservation. Settlement runs on the existing
lifecycle-tracked detached billing path, with request/user/token ledgers reconciled.

If the gateway encoder fails before any audio is delivered, the gateway absorbs
the provider cost and **credits the entire customer charge**. The same settlement
path refunds a reservation or records zero charge for a trusted/no-hold request.
The consume log retains input/cache/output usage and `encoding_failed_refund=true`;
provider acceptance stays recorded so automatic retries cannot duplicate paid work.
Caller cancellation and downstream write failures do not receive this codec credit.
A codec timeout with an otherwise live request is a gateway failure.

When a stream ends before complete final usage, the receipt is explicitly marked
`usage_complete=false`; missing output is estimated from received PCM at 25 audio
tokens/second, keeping any higher validated cumulative output count. Missing input
remains unpriced (zero), not fabricated. Such a charge is an **estimate**, not a
claim of an exact upstream invoice. The consume log and warning identify the
incomplete receipt. Unreceived provider work cannot be reconstructed from a
broken connection. Original-model labels and actual-model prices remain distinct.

## Bounds and compatibility

The standard endpoint accepts up to 4,096 input characters, up to 4,096 combined
style characters, and a normalized body up to 128 KiB. Native responses are capped
at 64 MiB wire / 32 MiB PCM and by the requested output token budget. The provider
continues to enforce the actual 8,192-token input limit; a character limit is not a
claim to reproduce Google's tokenizer. No new service or SDK dependency is added.

Older Gemini 2.5/3.1 TTS IDs remain catalog entries but are not silently converted
to the 3.8 protocol. Chat/Responses/Messages tool conversion, embedding routes,
Gemini Live, and non-Google speech paths are unchanged. Administrators with an
explicit channel endpoint allowlist must enable `audio_speech`; adding a default
does not override that policy. Paid provider calls and project entitlement checks
are intentionally absent from the regression suite.

## Validation

```sh
go test -race ./relay/adaptor/gemini/tts ./relay/adaptor/geminiOpenaiCompatible ./relay/channeltype
go test -race ./relay/controller -run 'TestGeminiSpeech|TestVertexSpeech'
go vet ./...
go test -race ./...
```

The controller suite uses the repository's existing SQLite billing fixture and a
local TLS upstream. Codec tests invoke actual ffmpeg and skip explicitly when it
is not installed. Unit tests cover payloads, cumulative SSE, late usage, terminal
errors, malformed base64/WAV, output bounds, encoder cancellation, and short writes.
No added test is represented as passing until its execution result is available.

## Official protocol sources (reviewed 2026-09-30)

- https://ai.google.dev/gemini-api/docs/generate-content/speech-generation
- https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash-tts
- https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash-lite-tts
- https://ai.google.dev/gemini-api/docs/pricing
- https://cloud.google.com/gemini-enterprise-agent-platform/generative-ai/pricing
- https://developers.openai.com/api/reference/resources/audio/subresources/speech/methods/create

The repository's Gemini adaptor already uses GenerateContent, so this extension
uses the documented GenerateContent speech surface rather than introducing the
separate Interactions API.
