# Groq model metadata audit

Audited on **2026-10-12** against Groq's public documentation. This is a
provider-documentation snapshot, not a live account-entitlement check. The
existing discovery list already matches the 13 published IDs, so this update
changes audio metadata rather than inventing new models or repricing usage.

## Catalog and rates

Source: [supported models](https://console.groq.com/docs/models).
Token prices below are USD per million input/output tokens; audio uses the
explicit units in its price column. Enterprise prices are not public quotes.

| Model ID | Availability | Price | Context / maximum output tokens |
| --- | --- | --- | --- |
| `llama-3.1-8b-instant` | Enterprise production | Contact sales | 131,072 / 131,072 |
| `llama-3.3-70b-versatile` | Enterprise production | Contact sales | 131,072 / 32,768 |
| `openai/gpt-oss-120b` | Production | $0.15 / $0.60 | 131,072 / 65,536 |
| `openai/gpt-oss-20b` | Production | $0.075 / $0.30 | 131,072 / 65,536 |
| `whisper-large-v3` | Production | $0.111/audio-hour | Not published |
| `whisper-large-v3-turbo` | Production | $0.04/audio-hour | Not published |
| `canopylabs/orpheus-arabic-saudi` | Preview | $40/million characters | 4,000 / 50,000 |
| `canopylabs/orpheus-v1-english` | Preview | $22/million characters | 4,000 / 50,000 |
| `meta-llama/llama-prompt-guard-2-22m` | Preview | $0.03 / $0.03 | 512 / 512 |
| `meta-llama/llama-prompt-guard-2-86m` | Preview | $0.04 / $0.04 | 512 / 512 |
| `minimaxai/minimax-m2.7` | Enterprise preview | Contact sales | 196,608 / 131,072 |
| `openai/gpt-oss-safeguard-20b` | Preview | $0.075 / $0.30 | 131,072 / 65,536 |
| `qwen/qwen3.8-27b` | Preview | $0.80 / $4.00 | 131,072 / 16,384 |

## Corrected audio metadata

The [speech-to-text guide](https://console.groq.com/docs/speech-to-text) and
[transcription API reference](https://console.groq.com/docs/api-reference#audio)
confirm `timestamp_granularities` for both Whisper models. The metadata uses the
SDK parameter name; multipart form fields use `timestamp_granularities[]`.
Word and segment timestamps require `response_format=verbose_json` and describe
transcription, not translation. Whisper large-v3 supports translation to English;
Turbo is transcription-only. Both retain the ten-second minimum audio charge.

The [Orpheus-specific guide](https://console.groq.com/docs/text-to-speech/orpheus)
is authoritative for the two current speech models: `voice` is required,
`response_format` defaults to and only supports `wav`, and input is limited to
200 characters. English supports bracketed vocal directions; Saudi Arabic does
not. These controls and caveats are now present in the returned model metadata.
The catalog's 4,000/50,000 **token** limits are retained; replacing them with a
200-**character** endpoint limit would mix units.

The generic speech API reference still uses retired `playai-tts` examples and
lists additional formats, `speed`, and `sample_rate`. Those fields are not
advertised for Orpheus without model-specific confirmation. This change does
not add endpoint validation, change audio transport, or promise that every
listed parameter applies to every endpoint of a model.

## Revalidated, deliberately unchanged

[Prompt caching](https://console.groq.com/docs/prompt-caching) publishes a 50%
input discount for GPT-OSS 120B, GPT-OSS 20B, and GPT-OSS Safeguard 20B only. The
existing exact cached rates, including $0.0375 rather than rounded $0.037,
remain intact. No discounted Qwen or MiniMax cache tariff is invented.

[Structured outputs](https://console.groq.com/docs/structured-outputs) lists
strict support for GPT-OSS 120B/20B and Qwen3.8-27B, with best-effort support also
for Safeguard. The existing generic capability flag is retained; it is not a
promise of strict mode or simultaneous streaming/tool use for every model.

[Groq's chat API](https://console.groq.com/docs/api-reference#chat) remains the
source for Qwen3.8's `none/default/low/medium/high` effort vocabulary and `none`
default. The [MiniMax model card](https://console.groq.com/docs/model/minimaxai/minimax-m2.7)
does not publish a discrete effort vocabulary or public price. Its existing
contract-pricing admission gate and zero placeholder are unchanged; zero is not
a free tariff. Historical Llama prices are compatibility defaults, not quotes.

According to [deprecations](https://console.groq.com/docs/deprecations), Compound
and Compound Mini stopped serving on 2026-09-21. Qwen3.6 public access ended on
2026-09-14; Llama3.1/3.3 on 2026-08-16; Qwen3-32B and Llama4 Scout on 2026-07-17.
The applicable enterprise exceptions and all retained historical metadata stay
unchanged. Retired IDs are not reintroduced into discovery.

## Regression coverage and release

`audio_metadata_test.go` checks the four audio models through `Adaptor` methods:
exact parameter sets, discovery, modalities, caller-facing caveats, billing-unit
separation, and no audio-control leakage into other current or retained models.
Existing catalog, price, reasoning conversion, vision, and admission tests are
retained unchanged. The new metadata tests must fail against pre-fix constants,
then pass alongside the complete Groq package after the fix.

```sh
go test -race -count=1 ./relay/adaptor/groq ./relay/format
go vet ./relay/adaptor/groq ./relay/format
make test-quick
```

Run evidence belongs in the PR, not in a claimed result inferred from these
commands. No live paid Groq request or full-repository qualification is implied.
No schema, dependencies, billing rules, deployment configuration, or permanent
CI changes are required. Deploy through the normal review/release process;
rollback is a revert of this metadata update, with no data migration.
