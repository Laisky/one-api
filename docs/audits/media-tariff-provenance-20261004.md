# Zero-ratio media tariff investigation (#474)

Reviewed on October 4, 2026. No paid provider call or production request was made.
Loopback providers establish relay behavior; official public sources establish
published tariffs. Actual account entitlement and private contract discounts
remain operator configuration.

## Findings and units

| Model | Reviewed provider source | Contract | Relay representation |
| --- | --- | --- | --- |
| `XiaomiMiMo/MiMo-V2.5-tts` | [DeepInfra](https://deepinfra.com/XiaomiMiMo/MiMo-V2.5-tts/api) | $0 per million characters | Explicit character tariff; promotional free |
| `XiaomiMiMo/MiMo-V2.5-tts-voiceclone` | [DeepInfra](https://deepinfra.com/XiaomiMiMo/MiMo-V2.5-tts-voiceclone) | $0 per million characters | Explicit character tariff; promotional free |
| `XiaomiMiMo/MiMo-V2.5-tts-voicedesign` | [DeepInfra](https://deepinfra.com/XiaomiMiMo/MiMo-V2.5-tts-voicedesign) | $0 per million characters | Explicit character tariff; promotional free |
| `google/lyria-3-clip-preview` | [OpenRouter](https://openrouter.ai/google/lyria-3-clip-preview) | $0.04 per generated clip | `per_call.usd_per_thousand_calls = 40` |
| `google/lyria-3-pro-preview` | [OpenRouter](https://openrouter.ai/collections/audio-models) | $0.08 per generated song | `per_call.usd_per_thousand_calls = 80` |

DeepInfra explicitly describes the MiMo speech offer as temporary, without a
published end date in the reviewed pages. The previous paid-cost exploitation
claim is therefore not established for this currently free offer. No positive
MiMo price or invented expiry was added. A future catalog update must change the
state/tariff when the promotion ends; this patch does not poll provider pages at
runtime or guarantee a price remains current indefinitely.

OpenRouter's Lyria catalog previously advertised paid generation prices in prose
while the billing path interpreted the zero token rate as a free request. The
current official provider pages corroborate the per-generation denomination.
A song price is not an audio-second price or a token ratio.

## Behavioral reproduction and remediation

The RED test commit `46fa6803` submitted mapped model aliases to the production
Chat Completions, Responses fallback and Claude Messages relay helpers with real
TLS loopback provider sockets and SQLite user/token balances. Paid clip/song
requests reached the provider without a debit, and a balance of 19,999 quota units
could dispatch a clip requiring 20,000. Group-free and operator-free controls were
kept separate from paid cases. No external model actually generated music.

The provider catalog now records tariff state, unit, source and verification date.
States `unknown` and `contract`, known expiry boundaries, and missing paid units
fail before dispatch. Existing legacy models without provenance are unchanged;
this is a scoped correction to the five reviewed entries, not an inferred price
for every zero in the catalog.

Lyria admission reserves one complete generation through the existing atomic
user/finite-token transaction. Quota uses exact decimal arithmetic and applies the
group multiplier once. Settlement uses the same configured model and per-call
rate instead of multiplying the song price by token counts. Missing usage retains
the reservation, and a client write failure still reaches final settlement.
The Responses fallback previously kept the debit but skipped its final request
cost on delivery failure; it now completes the same final ledger path.

One generation per request is the supported paid contract. Batches and tool loops
are rejected before upstream dispatch rather than forwarding unreserved extra
generations. Ordinary streaming/nonstreaming protocol adapters retain their own
response behavior. Responses and Claude callers retain their existing format
conversion; this change does not assert that those formats can render every native
audio feature.

The Modern catalog already understands per-call prices and now receives the same
$0.04/$0.08 numbers used for admission and billing. MiMo speech retains zero cost
and correct character units. Explicit administrator `per_call: {}` remains free;
an explicit ratio-zero/completion configuration also retains free policy. Paid
operator generation overrides must use `per_call`, because token ratios cannot
represent a song price. A free group remains free after validating the request
contract. A group multiplier does not turn an unknown tariff into a known one.

## Validation scope

Behavioral regressions assert provider-observed balances before dispatch, denied
user/token admission, owner/token balances after settlement, consume logs, per-call
catalog display, unknown/expired contracts, batch/tool rejection, promotional
MiMo speech, explicit operator/free-group controls and client-delivery failure.
Existing reservation contention tests supply the shared transaction guarantee.
Account-specific provider receipts and promotional end-date monitoring are outside
these local tests; no claim of actual provider acceptance or invoice validation is
made.

The public [OpenRouter model catalog JSON](https://openrouter.ai/api/v1/models)
was also checked read-only. Both Lyria entries reported only zero `prompt` and
`completion` prices, omitting the per-generation charge described on the provider
pages. That machine-readable omission must not override the explicit song/clip
contract. No API credential was used for this catalog check.
