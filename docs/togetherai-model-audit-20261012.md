# Together AI model audit — 2026-10-12

Sources:

- https://docs.together.ai/docs/serverless/models
- https://docs.together.ai/reference/audio-speech

Evidence SHA-256 (serverless table): `0ec1556338ed532b3cad0fecbb9fb6ac19be557159c3dd78f5012fd710557b57`.

All 18 listed serverless text models already have API IDs in the compiled
catalogue. This change refreshes their explicitly published capability, context
and quantization fields without erasing metadata where the source has a dash.
Three chat prices change: Qwen3.7-Max to USD 2.50/7.50 input/output, Kimi-K3 to
USD 2.70/13.50, and Qwen3.8-Flash to USD 0.15/0.47 per million tokens. Published
cache prices remain provider-specific. Structured outputs and tool discovery
are corrected where advertised. Kimi K3 gains the image-input metadata from
Together's separate Vision table, not an inferred native-creator feature set.

Kokoro changes from USD 10 to USD 4 per million input characters. Five existing
speech models now explicitly carry character-unit pricing and text-to-audio
modalities. The scalar fallback uses the same per-character charge; operator
ratio-only overrides keep their prior semantics. Tev1 output is explicitly
free and is preserved as CompletionRatio zero, not silently defaulted to one.

## Deliberate exclusions

The Qwen3 TTS CustomVoice model appears in the serverless table but not the
retrieved speech schema's accepted-model enumeration. No new voice or endpoint
compatibility is asserted. Existing configured Cartesia variants are retained.
New image prices marked `varies` are starting estimates rather than complete
per-request tariffs; new video rows without duration/resolution units are not
converted to token rates. Embedding, rerank and moderation have no current
serverless offering; existing dedicated compatibility entries are not removed.

Tests pin all 18 text tariffs, all five speech tariffs, exact units, public and
generic discovery, tool/image metadata and unchanged request IDs. Old-code
negative controls and affected package race-test receipts are retained in the
PR. No paid live request, database migration, deployment change, or operator
override change is included. Revert this provider commit to restore old defaults.
