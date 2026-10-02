# OpenAI Realtime billing audit

Reviewed: 2026-09-09. Base: `91afd3cb28f113c17bcdf8e203417ed4e09169ec`.

## Scope and billing contract

This change fixes accounting for server usage receipts observed by one-api's
OpenAI WebSocket Realtime proxy. It does not meter a WebRTC/SIP connection that
bypasses the proxy, implement a new transport, or invent usage after a disconnect.
Zhipu keeps its existing collector/surcharge path; ordinary completion billing is
unchanged. No database migration or paid upstream call is required for the tests.

For each response, cached input is a subset of its modality, not extra tokens:

```
response_cost = (text_in - cached_text) * text_input_price
              + cached_text * cached_text_price
              + (audio_in - cached_audio) * audio_input_price
              + cached_audio * cached_audio_price
              + (image_in - cached_image) * image_input_price
              + cached_image * cached_image_price
              + text_out * text_output_price
              + audio_out * audio_output_price

asr_token_cost = asr_text_in * asr_text_price
               + asr_audio_in * asr_audio_price
               + asr_text_out * asr_output_price
asr_duration_cost = reported_seconds * asr_price_per_second

quota = ceil(group_ratio * sum(all response and ASR costs in quota units))
      + already_grouped_tools_quota
settlement_delta = quota - reservation
```

The pre-consumption reservation is not a minimum session fee. An observed idle
session costs zero and refunds its reservation through normal settlement. A
Whisper duration receipt can cost money while both token counts are zero.

Text/audio prices continue to use the existing channel/provider/global resolver,
context tiers, request-start time windows and scalar overrides. Each response
resolves its own context tier; session-wide accumulated context is not one prompt.
Independent ASR uses its own model configuration, not the conversation model.
Audio cache discounts and image/text input multipliers supplement dimensions
missing from the existing media schema. Configured modality markups scale these
published discounts; independently overriding cached audio/image is not a new
configuration feature in this patch.

## Verified rules

Only final `response.done` usage and
`conversation.item.input_audio_transcription.completed` usage are chargeable.
`completed`, `cancelled`, `failed` and `incomplete` response statuses do not cancel
usage actually returned by the provider. Response IDs and item/content-index
pairs have separate deduplication namespaces. A malformed receipt cannot consume
an ID before a later valid receipt arrives.

Input transcription is optional, asynchronous and an additional service. It is
not how the native audio conversation model hears the user. Disabling it removes
the independent ASR charge, not the conversation's audio input charge. A server
session acknowledgement determines the transcription model, which is captured
when an input audio item is committed/created. Later acknowledgements cannot
reprice an outstanding item. Assistant output transcript events are not another
ASR invoice; bill the response's reported text/audio output counts once.

Full conversation history can be charged as input again on later responses,
with cache discounts only where reported. VAD, retention, truncation and audio
playback acknowledgements are not separate line items and cannot retroactively
refund generated usage. Do not count transcript characters or estimate final
charges from token-per-second examples. Session/network idle time is not itself
a conversation-token charge.

### Standard USD prices per million tokens

| Model family | Text in/cache/out | Audio in/cache/out | Image in/cache |
| --- | --- | --- | --- |
| gpt-realtime; gpt-realtime-1.5 | 4 / 0.40 / 16 | 32 / 0.40 / 64 | 5 / 0.50 |
| gpt-realtime-2; gpt-realtime-2.1 | 4 / 0.40 / 24 | 32 / 0.40 / 64 | 5 / 0.50 |
| gpt-realtime-mini; gpt-realtime-2.1-mini | 0.60 / 0.06 / 2.40 | 10 / 0.30 / 20 | 0.80 / 0.08 |
| gpt-4o-realtime-preview | 5 / 2.50 / 20 | 40 / 2.50 / 80 | Unsupported |
| gpt-4o-mini-realtime-preview | 0.60 / 0.30 / 2.40 | 10 / 0.30 / 20 | Unsupported |

`gpt-4o-transcribe` costs $2.50/M text input, $6/M audio input and $10/M text
output; mini costs $1.25/$3/$5 respectively. `whisper-1` costs $0.006/minute.
Duration receipts preserve fractional seconds. The independent duration pricing
resolver also recognizes the catalog's `gpt-realtime-whisper` $0.017/minute; that
unit test is not a claim that its dedicated live transcription transport is
implemented. `gpt-realtime-translate` is a separate $0.034/minute product, not a
conversation response-token price.

## Defects addressed

1. The old collector lost per-modality cache counts, ignored ASR completion
   receipts, and accepted response usage without requiring the final event.
2. The existing top-level controller already added an audio surcharge. It applied
   that full surcharge to cached audio too. For one million fully cached input
   audio tokens on gpt-realtime, the old text-cache plus audio-surcharge calculation
   is $28.40, versus the published $0.40. Receipt pricing bypasses the old surcharge
   rather than adding another charge on top of it.
3. Image input/cache pricing was flattened into the generic text calculation.
4. Zero-token handling retained idle reservations and could erase duration-only
   ASR charges. Explicit ledgers now reach final settlement even with zero tokens.
5. Per-surcharge rounding and mutable ToolsCost could introduce extra quota units
   or duplicate charges. Receipt calculations are immutable and round once.
6. The mini preview audio multiplier was rounded to 16.67 instead of 10/0.6.
   Both the alias and its dated snapshot now use the exact expression.

## Test coverage and reproduction

| Layer | Files | Cases |
| --- | --- | --- |
| Protocol + arithmetic | relay/realtime/*_test.go | Final-status variants, official usage samples, replay, malformed/overflowing counts, cache partitions, GA/beta ASR settings, disable/model changes, out-of-order ASR, fractional duration, pending disconnect, non-receipts, rounding, free/group/tool charges and fuzz seeds |
| Actual catalog/resolver | relay/quota/realtime_test.go | 72 independent published modality-rate cases across 10 aliases/snapshots, mixed modalities, ASR models, duration, group/channel overrides, no per-event rounding, unknown model diagnostics, client JSON isolation |
| WebSocket transport | relay/adaptor/openai/realtime_meter_test.go | Two real loopback WebSocket pairs, client-forged usage, binary frames, duplicate finals, late ASR, cancellation and receipt retention before client-write failure |
| Production controller | controller/realtime_receipts_test.go | No double audio surcharge, immutable repeat computation, explicit idle versus legacy missing usage, duration-only charge and reconciliation metadata |

Run in the full repository with its declared Go toolchain:

```sh
gofmt -l controller/realtime*.go relay/realtime relay/quota/realtime*.go \
  relay/adaptor/openai/realtime*.go relay/model/misc.go relay/model/image_usage.go

go test -race ./relay/realtime ./relay/quota ./relay/adaptor/openai ./controller

go vet ./...
go test -race ./...
```

Actually executed in the editing environment (Go 1.23.2, isolated dependency-free
core, without a downloaded repository dependency graph):

```sh
cd relay/realtime
GO111MODULE=off go test -race -cover -v .
# PASS; 93.3% statement coverage of this package, not of the whole gateway.
GO111MODULE=off go vet .
# PASS
GO111MODULE=off go test -run '^$' -fuzz '^FuzzLedgerUsage$' -fuzztime 3s -parallel 2 .
# PASS; 137,458 executions in this run.
```

The catalog, WebSocket and controller tests were written but were not executed
locally: the full checkout/dependencies and repository toolchain were unavailable.
Do not interpret the core result as integration or database-settlement success.
The PR must remain unapproved for deployment until full-repository CI passes,
including the existing billing tests. No paid OpenAI session/invoice reconciliation
was performed.

## Explicit limits and operational review

`metadata.realtime_usage` retains numeric receipts and model names, not audio,
transcripts, API keys or ephemeral secrets. `realtime_billing_complete=false` and
`realtime_billing_issues` identify missing, ambiguous, malformed or unpriceable
receipts. Valid observed receipts are charged; unpriceable receipts are not guessed
at the generic model fallback. Operators must reconcile such sessions; the metadata
flag is not an automatic provider-invoice reconciliation job.

Input/output overhead not assigned to a media modality is treated as text.
A mixed-modality cached count without its split is ambiguous and is flagged rather
than discounting arbitrary audio/image tokens. Pending server responses/ASR at
connection close are flagged. Usage never delivered to the proxy cannot be recovered;
a disconnect before the first server acknowledgement is not fully observable.
Durable crash recovery and mid-session quota enforcement are not added here. The
existing two-minute reservation does not cap a long-running session's total spend.

The existing `/v1/realtime/sessions` ephemeral-token flow gives clients a direct
upstream connection. Subsequent WebRTC/SIP usage bypasses this WebSocket meter.
Neither a session-creation response nor client-reported totals prove its final
usage. Properly billing that route needs authoritative server-side observation or
provider reconciliation; charging an invented flat fee would not fix it. Dedicated
live transcription/translation protocols must receive their own transport and
usage-contract tests before being claimed as covered.

## Primary sources

Verified 2026-09-09; price assertions above are fixtures, not live web-scraping tests.

- https://developers.openai.com/api/docs/guides/realtime-costs
- https://developers.openai.com/api/docs/pricing
- https://platform.openai.com/docs/api-reference/realtime-server-events
- https://developers.openai.com/api/docs/models/gpt-realtime
- https://developers.openai.com/api/docs/models/gpt-realtime-2.1
- https://developers.openai.com/api/docs/models/gpt-realtime-2.1-mini
- https://developers.openai.com/api/docs/models/gpt-realtime-mini
- https://developers.openai.com/api/docs/models/gpt-4o-realtime-preview
- https://developers.openai.com/api/docs/models/gpt-4o-mini-realtime-preview
- https://developers.openai.com/api/docs/models/gpt-4o-transcribe
- https://developers.openai.com/api/docs/models/gpt-4o-mini-transcribe
- https://developers.openai.com/api/docs/models/whisper-1
- https://developers.openai.com/api/docs/models/gpt-realtime-whisper
- https://developers.openai.com/api/docs/models/gpt-realtime-translate
