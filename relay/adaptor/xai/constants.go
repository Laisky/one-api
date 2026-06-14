package xai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	ratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// Shared metadata helpers for xAI Grok models. These slices are reused across
// many ModelRatios entries to keep the table compact and consistent.
//
// Sources:
//   - https://docs.x.ai/developers/models
//   - https://docs.x.ai/developers/models/grok-4.3
//   - https://docs.x.ai/developers/models/grok-4.20-0309-reasoning
//   - https://docs.x.ai/developers/models/grok-4.20-0309-non-reasoning
//   - https://docs.x.ai/developers/models/grok-4.20-multi-agent-0309
//   - https://docs.x.ai/developers/models/grok-imagine-image
//   - https://docs.x.ai/developers/models/grok-imagine-image-quality
//   - https://docs.x.ai/developers/migration/may-15-retirement
//   - https://docs.x.ai/developers/model-capabilities/text/reasoning
//   - https://docs.x.ai/developers/model-capabilities/text/multi-agent
//   - https://docs.x.ai/developers/tools/web-search
var (
	// grokTextInputs lists the input modalities for text-only Grok chat models.
	grokTextInputs = []string{"text"}
	// grokVisionInputs lists the input modalities for vision-capable Grok chat models.
	grokVisionInputs = []string{"text", "image"}
	// grokTextOutputs lists the output modalities for all Grok chat models.
	grokTextOutputs = []string{"text"}

	// grokFeaturesBase advertises the standard chat capability set: tool calling,
	// JSON mode, and structured outputs.
	grokFeaturesBase = []string{"tools", "json_mode", "structured_outputs"}
	// grokFeaturesReasoning extends grokFeaturesBase with reasoning. Used for
	// Grok models that emit thinking traces or run reasoning by default
	// (grok-3-mini, grok-4.3, grok-4.20-*-reasoning, multi-agent).
	grokFeaturesReasoning = []string{"tools", "json_mode", "structured_outputs", "reasoning"}
	// grokFeaturesLegacy advertises the limited capability set for legacy Grok 2
	// models that do not support reasoning.
	grokFeaturesLegacy = []string{"tools", "json_mode", "structured_outputs"}

	// grokSamplingParams lists the OpenAI-compatible sampling parameters that
	// xAI Grok chat completions accept. Reasoning-only Grok models may ignore
	// some of these (notably temperature/top_p). See xAI docs for details.
	grokSamplingParams = []string{
		"temperature",
		"top_p",
		"frequency_penalty",
		"presence_penalty",
		"stop",
		"seed",
		"max_tokens",
	}
	// grokReasoningSamplingParams is the conservative sampling-parameter set for
	// reasoning Grok models (grok-4.3, grok-4.20-*-reasoning, multi-agent) which
	// reject presence_penalty, frequency_penalty, and stop per the xAI API ref.
	// Only temperature, top_p, seed, and max_tokens are honored.
	// Source: https://docs.x.ai/docs/api-reference#chat-completions
	grokReasoningSamplingParams = []string{"temperature", "top_p", "seed", "max_tokens"}

	// grokMiniReasoningEfforts lists the reasoning_effort values historically
	// accepted by Grok 3 mini ({"low", "high"}). After May 15, 2026, grok-3-mini
	// is auto-redirected to grok-4.3 which accepts the full {none, low, medium, high}
	// set; the wider set is preferred for the redirected slug.
	// Source: https://docs.x.ai/developers/model-capabilities/text/reasoning
	grokMiniReasoningEfforts = []string{"low", "high"}

	// grokFullReasoningEfforts lists the reasoning_effort values accepted by
	// Grok 4.3 and the Grok 4.20 reasoning variants. Default is "low".
	// "none" disables reasoning entirely.
	// Source: https://docs.x.ai/developers/model-capabilities/text/reasoning
	grokFullReasoningEfforts = []string{"none", "low", "medium", "high"}

	// grokMultiAgentEfforts lists the reasoning.effort values accepted by
	// grok-4.20 multi-agent variants. For multi-agent the effort level
	// controls how many agents collaborate (4 on low/medium, 16 on high/xhigh)
	// rather than reasoning depth.
	// Source: https://docs.x.ai/developers/model-capabilities/text/multi-agent
	grokMultiAgentEfforts = []string{"low", "medium", "high", "xhigh"}
)

// ModelRatios contains all supported models and their pricing ratios.
// Model list is derived from the keys of this map, eliminating redundancy.
//
// Note on metadata fields: all Grok models are closed-weight, so Quantization
// and HuggingFaceID are intentionally left empty for every entry. xAI does not
// publish a separate MaxOutputTokens limit for chat completions — the published
// context window is the cap for input+output combined — so MaxOutputTokens is
// left at zero (unspecified) and callers should fall back to ContextLength.
//
// Retirement notice (May 15, 2026): the following slugs were retired by xAI
// and now auto-redirect to grok-4.3 (billed at grok-4.3 rates):
//   - grok-4-1-fast-reasoning, grok-4-1-fast-non-reasoning
//   - grok-4-fast-reasoning, grok-4-fast-non-reasoning
//   - grok-4-0709, grok-code-fast-1, grok-3
//   - grok-imagine-image-pro (redirects to grok-imagine-image-quality)
//
// We keep the retired slugs in this table so existing clients continue to be
// priced correctly, but their metadata (context, modalities, features, pricing)
// mirrors grok-4.3 to reflect the actual upstream behavior.
//
// Official sources:
//   - https://docs.x.ai/developers/models
//   - https://docs.x.ai/developers/migration/may-15-retirement
//   - https://docs.x.ai/developers/model-capabilities/text/multi-agent
//   - https://docs.x.ai/developers/tools/web-search
var ModelRatios = map[string]adaptor.ModelConfig{
	// ============================================================
	// Current flagship — Grok 4.3 (released May 6, 2026)
	// $1.25 input / $0.20 cached input / $2.50 output per 1M tokens
	// Source: https://docs.x.ai/developers/models/grok-4.3
	// ============================================================
	"grok-4.3": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
		SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
		Description: "Grok 4.3 is xAI's flagship reasoning model with a 1M-token context, vision input, agentic tool calling, structured outputs, and toggleable reasoning_effort {none,low,medium,high}.",
	},

	// ============================================================
	// Grok 4.20 (March 9, 2026 snapshot) — 1M context reasoning/non-reasoning
	// and 2M context multi-agent variant.
	// $1.25 input / $0.20 cached input / $2.50 output per 1M tokens
	// Source: https://docs.x.ai/developers/models
	// ============================================================
	"grok-4.20-0309-reasoning": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
		SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
		Description: "Grok 4.20 (March 9 snapshot, reasoning) — 1M-context model with extended thinking and reasoning_effort {none,low,medium,high}.",
	},
	"grok-4.20-0309-non-reasoning": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesBase, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 4.20 (March 9 snapshot, non-reasoning) — 1M-context model with reasoning disabled for low-latency tool calling.",
	},
	"grok-4.20-multi-agent-0309": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
		// Multi-agent reasoning.effort governs agent count (4 on low/medium, 16 on high/xhigh), not depth.
		SupportedReasoningEfforts: grokMultiAgentEfforts, DefaultReasoningEffort: "low",
		Description: "Grok 4.20 Multi-Agent (March 9 snapshot) — 1M-context parallel-agent variant for deep research (4 agents on low/medium, 16 on high/xhigh).",
	},

	// ============================================================
	// Legacy Grok 4.20 aliases (no -0309 snapshot suffix). These are
	// not separately listed on docs.x.ai/developers/models as of May 2026
	// but were previously supported; retained for backwards compatibility
	// at the same per-token pricing as the -0309 snapshots.
	// UNVERIFIED: official docs no longer list these without the -0309 suffix.
	// ============================================================
	"grok-4.20": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
		SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
		Description: "Grok 4.20 (legacy alias) — 1M-context model with toggleable reasoning. Prefer the -0309 snapshot slug.",
	},
	"grok-4.20-reasoning": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
		SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
		Description: "Grok 4.20 (legacy alias, reasoning enabled) — 1M-context model with extended thinking.",
	},
	"grok-4.20-non-reasoning": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesBase, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 4.20 (legacy alias, non-reasoning) — 1M-context model with reasoning disabled.",
	},
	"grok-4.20-multi-agent": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
		SupportedReasoningEfforts: grokMultiAgentEfforts, DefaultReasoningEffort: "low",
		Description: "Grok 4.20 Multi-Agent (legacy alias) — 1M-context parallel-agent variant. Prefer the -0309 snapshot slug.",
	},

	// ============================================================
	// Retired models (May 15, 2026) — slugs still resolve and auto-redirect
	// to grok-4.3 at grok-4.3 pricing. Kept here so existing channels keep
	// computing the correct quota until callers migrate.
	// Source: https://docs.x.ai/developers/migration/may-15-retirement
	// ============================================================
	"grok-4-0709": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
		// Retired May 15, 2026 — auto-redirects to grok-4.3 with reasoning_effort=low.
		Description: "Grok 4 (0709) — RETIRED May 15, 2026; requests auto-redirect to grok-4.3 at grok-4.3 pricing.",
	},
	"grok-4-1-fast-reasoning": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
		SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
		Description: "Grok 4.1 Fast (reasoning) — RETIRED May 15, 2026; auto-redirects to grok-4.3 with reasoning_effort=low.",
	},
	"grok-4-1-fast-non-reasoning": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesBase, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 4.1 Fast (non-reasoning) — RETIRED May 15, 2026; auto-redirects to grok-4.3 with reasoning_effort=none.",
	},
	"grok-4-fast-reasoning": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
		SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
		Description: "Grok 4 Fast (reasoning) — RETIRED May 15, 2026; auto-redirects to grok-4.3 with reasoning_effort=low.",
	},
	"grok-4-fast-non-reasoning": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesBase, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 4 Fast (non-reasoning) — RETIRED May 15, 2026; auto-redirects to grok-4.3 with reasoning_effort=none.",
	},
	"grok-code-fast-1": {
		Ratio: 1.0 * ratio.MilliTokensUsd, CompletionRatio: 2.0 / 1.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   256000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
		SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
		Description:               "Grok Code Fast 1 — RETIRED May 15, 2026; auto-redirects to grok-build-0.1 ($1.00 in / $0.20 cached / $2.00 out, 256K context).",
	},
	// grok-build-0.1 is the GA agentic coding model; grok-code-fast-1 / grok-code-fast
	// redirect to it. Source: https://docs.x.ai/developers/models/grok-build-0.1
	"grok-build-0.1": {
		Ratio: 1.0 * ratio.MilliTokensUsd, CompletionRatio: 2.0 / 1.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   256000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
		SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
		Description:               "Grok Build 0.1 — fast agentic coding model (256K context, vision input); $1.00 in / $0.20 cached / $2.00 out. Aliases: grok-code-fast-1, grok-code-fast, grok-code-fast-1-0825.",
	},
	"grok-3": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesBase, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 3 — RETIRED May 15, 2026; auto-redirects to grok-4.3 with reasoning_effort=none.",
	},

	// ============================================================
	// Grok 3 Mini — page on docs.x.ai/developers/models/grok-3-mini now
	// resolves as an alias for grok-4.3 (1M context, vision, full effort set).
	// Pricing therefore mirrors grok-4.3 ($1.25/$0.20/$2.50) post-redirect.
	// UNVERIFIED: legacy callers may still see the historical $0.30/$0.50
	// pricing on direct xAI invoices; if so, override at the channel level.
	// ============================================================
	"grok-3-mini": {
		Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 1.25, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   1000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
		// xAI legacy docs published {"low","high"} for grok-3-mini; post-redirect to grok-4.3
		// the full {none,low,medium,high} set is accepted. We expose the wider set.
		SupportedReasoningEfforts: grokFullReasoningEfforts, DefaultReasoningEffort: "low",
		Description: "Grok 3 Mini — alias for grok-4.3 post May 15, 2026 (1M context, vision). Historical reasoning_effort {low,high} is now accepted as a subset of {none,low,medium,high}.",
	},

	// ============================================================
	// Legacy Grok 2 vision/text — not explicitly retired by xAI but no longer
	// listed on docs.x.ai/developers/models. UNVERIFIED but preserved at
	// historical pricing.
	// ============================================================
	"grok-2-vision-1212": {
		Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 10.0 / 2.0, // $2.00 input, $10.00 output
		ContextLength:   32768,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesLegacy, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 2 Vision 1212 — multimodal model with image understanding (UNVERIFIED: no longer listed in docs.x.ai/developers/models as of May 2026).",
	},

	// ============================================================
	// Image generation models. Current xAI line-up:
	//   grok-imagine-image            $0.02/image (standard)
	//   grok-imagine-image-quality    $0.05/image (high fidelity, replaces grok-imagine-image-pro)
	// Retired:
	//   grok-imagine-image-pro        → redirects to grok-imagine-image-quality
	// Legacy aliases retained (no longer in docs):
	//   grok-2-image, grok-2-image-1212  ($0.07/image historical)
	// Source: https://docs.x.ai/developers/models/grok-imagine-image
	//         https://docs.x.ai/developers/models/grok-imagine-image-quality
	// ============================================================
	"grok-imagine-image": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.02,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        10,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
			},
		},
		ContextLength:    4000,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      "Grok Imagine Image — fast text+image-to-image generation at $0.02/image (alias: grok-imagine-image-2026-03-02).",
	},
	"grok-imagine-image-2026-03-02": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.02,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        10,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
			},
		},
		ContextLength:    4000,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      "Grok Imagine Image snapshot (2026-03-02) — fast text+image-to-image generation at $0.02/image; alias of grok-imagine-image.",
	},
	"grok-imagine-image-quality": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.05,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        10,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
				"2048x2048": 1.4, // 2K = $0.07/image
			},
		},
		ContextLength:    4000,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      "Grok Imagine Image Quality — higher-fidelity text+image-to-image generation at $0.05/image (1K) / $0.07/image (2K); replaces grok-imagine-image-pro.",
	},
	"grok-imagine-image-pro": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.05,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        10,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
			},
		},
		ContextLength:    4000,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      "Grok Imagine Image Pro — RETIRED May 15, 2026; auto-redirects to grok-imagine-image-quality (text+image-to-image) at $0.05/image (1K) / $0.07 (2K).",
	},
	"grok-2-image-1212": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.07,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        10,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
			},
		},
		ContextLength:    4000,
		InputModalities:  grokTextInputs,
		OutputModalities: []string{"image"},
		Description:      "Grok 2 Image 1212 — legacy image generation snapshot at $0.07/image (UNVERIFIED: no longer listed in docs.x.ai/developers/models).",
	},
	"grok-2-image": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.07,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        10,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
			},
		},
		ContextLength:    4000,
		InputModalities:  grokTextInputs,
		OutputModalities: []string{"image"},
		Description:      "Grok 2 Image — legacy image generation alias at $0.07/image (UNVERIFIED: no longer listed in docs.x.ai/developers/models).",
	},

	// ============================================================
	// Video generation
	// Source: https://docs.x.ai/developers/model-capabilities/imagine
	// ============================================================
	"grok-imagine-video": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Video: &adaptor.VideoPricingConfig{
			PerSecondUsd:   0.05, // 480p base
			BaseResolution: "480p",
			ResolutionMultipliers: map[string]float64{
				"720p": 1.4, // $0.07/sec
			},
		},
		InputModalities:  grokTextInputs,
		OutputModalities: []string{"video"},
		Description:      "Grok Imagine Video — text-to-video generation; 480p $0.05/sec, 720p $0.07/sec.",
	},
	// grok-imagine-video-1.5-preview is image-to-video only (no text-to-video).
	// Source: https://docs.x.ai/developers/models/grok-imagine-video-1.5-preview
	"grok-imagine-video-1.5-preview": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Video: &adaptor.VideoPricingConfig{
			PerSecondUsd:   0.08, // 480p base
			BaseResolution: "480p",
			ResolutionMultipliers: map[string]float64{
				"720p": 1.75, // $0.14/sec
			},
		},
		InputModalities:  []string{"image"},
		OutputModalities: []string{"video"},
		Description:      "Grok Imagine Video 1.5 Preview — image-to-video (no text-to-video); 480p $0.08/sec, 720p $0.14/sec, image input $0.01/img.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// XAIToolingDefaults captures xAI's published tool invocation fees.
// Source: https://docs.x.ai/developers/tools/web-search
//
//	https://docs.x.ai/developers/models  (toolings section)
var XAIToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"web_search":         {UsdPerCall: 0.005},  // $5 / 1k calls
		"x_search":           {UsdPerCall: 0.005},  // $5 / 1k calls
		"code_execution":     {UsdPerCall: 0.005},  // $5 / 1k calls
		"code_interpreter":   {UsdPerCall: 0.005},  // alias of code_execution in Responses API
		"attachment_search":  {UsdPerCall: 0.01},   // $10 / 1k calls (File Attachments)
		"collections_search": {UsdPerCall: 0.0025}, // $2.50 / 1k calls
		"file_search":        {UsdPerCall: 0.0025}, // alias of collections_search in Responses API
	},
}
