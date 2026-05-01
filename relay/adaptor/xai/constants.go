package xai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	ratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// Shared metadata helpers for xAI Grok models. These slices are reused across
// many ModelRatios entries to keep the table compact and consistent.
//
// Sources:
//   - https://docs.x.ai/docs/models
//   - https://openrouter.ai/x-ai (per-model context windows and capabilities)
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
	// (grok-3-mini, grok-4, grok-code-fast-1, *-reasoning variants).
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
	// always-on reasoning Grok models (e.g. grok-4) which reject temperature
	// /top_p tuning. Only stop, seed, and max_tokens are reliably honored.
	grokReasoningSamplingParams = []string{"stop", "seed", "max_tokens"}
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
// Official sources:
//   - https://docs.x.ai/docs/models
//   - https://docs.x.ai/developers/models
//   - https://docs.x.ai/developers/model-capabilities/text/multi-agent
//   - https://docs.x.ai/developers/tools/web-search
//   - https://openrouter.ai/x-ai (per-model context windows and capabilities)
var ModelRatios = map[string]adaptor.ModelConfig{
	// Grok Models
	//
	// Note: Some prices are the same because they are aliases or stable/snapshot pairs.
	"grok-code-fast-1": {
		Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 1.5 / 0.2, CachedInputRatio: 0.02 * ratio.MilliTokensUsd, // $0.20 input, $0.02 cached input, $1.50 output
		ContextLength:   256000,
		InputModalities: grokTextInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok Code Fast 1 is a speedy and economical reasoning model from xAI optimized for agentic coding with visible reasoning traces.",
	},
	"grok-4-0709": {
		Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 15.0 / 3.0, CachedInputRatio: 0.75 * ratio.MilliTokensUsd,
		ContextLength:   256000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokReasoningSamplingParams,
		Description: "Grok 4 (0709) is xAI's reasoning flagship with 256k context, parallel tool calling, and image+text inputs; reasoning is always on and not exposed.",
	},
	"grok-4.20": {
		Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   2000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 4.20 is xAI's flagship model with 2M context, agentic tool calling, and toggleable reasoning.",
	},
	"grok-4.20-reasoning": {
		Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   2000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 4.20 (reasoning enabled) — flagship 2M-context model with active extended thinking.",
	},
	"grok-4.20-non-reasoning": {
		Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   2000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesBase, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 4.20 (reasoning disabled) — flagship 2M-context model with reasoning turned off for lower latency.",
	},
	"grok-4.20-multi-agent": {
		Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   2000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 4.20 Multi-Agent: parallel-agent variant for deep research and multi-step agentic workflows (4 agents on low/medium, 16 on high/xhigh).",
	},
	"grok-4.20-0309-reasoning": {
		Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   2000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 4.20 (March 9 snapshot, reasoning) — pinned 2M-context release with extended thinking.",
	},
	"grok-4.20-0309-non-reasoning": {
		Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   2000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesBase, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 4.20 (March 9 snapshot, no reasoning) — pinned 2M-context release with reasoning disabled.",
	},
	"grok-4.20-multi-agent-0309": {
		Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		ContextLength:   2000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 4.20 Multi-Agent (March 9 snapshot) — pinned parallel-agent release for deep research workflows.",
	},
	"grok-4-1-fast": {
		Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.2, CachedInputRatio: 0.05 * ratio.MilliTokensUsd,
		ContextLength:   2000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 4.1 Fast: xAI's cost-efficient agentic tool-calling model with 2M context and toggleable reasoning.",
	},
	"grok-4-1-fast-reasoning": {
		Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.2, CachedInputRatio: 0.05 * ratio.MilliTokensUsd,
		ContextLength:   2000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 4.1 Fast (reasoning enabled) — 2M-context cost-efficient model with extended thinking.",
	},
	"grok-4-1-fast-non-reasoning": {
		Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.2, CachedInputRatio: 0.05 * ratio.MilliTokensUsd,
		ContextLength:   2000000,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesBase, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 4.1 Fast (reasoning disabled) — 2M-context cost-efficient model tuned for low-latency tool calling.",
	},
	"grok-3": {
		Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0 / 3.0, CachedInputRatio: 0.75 * ratio.MilliTokensUsd,
		ContextLength:   131072,
		InputModalities: grokTextInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesBase, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 3 is xAI's flagship enterprise model for data extraction, coding, and summarization with deep domain knowledge.",
	},
	"grok-3-mini": {
		Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.3, CachedInputRatio: 0.075 * ratio.MilliTokensUsd,
		ContextLength:   131072,
		InputModalities: grokTextInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesReasoning, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 3 Mini is a lightweight thinking model great for logic-heavy tasks; raw thinking traces are accessible.",
	},
	// "grok-3-fast":               {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.75 * ratio.MilliTokensUsd},        // $3.00 input, $0.75 cached input, $15.00 output
	// "grok-3-mini-fast":          {Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.3, CachedInputRatio: 0.075 * ratio.MilliTokensUsd}, // $0.30 input, $0.075 cached input, $0.50 output
	"grok-2-vision-1212": {
		Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 10.0 / 2.0, // $2.00 input, $10.00 output
		ContextLength:   32768,
		InputModalities: grokVisionInputs, OutputModalities: grokTextOutputs,
		SupportedFeatures: grokFeaturesLegacy, SupportedSamplingParameters: grokSamplingParams,
		Description: "Grok 2 Vision 1212 — multimodal model with image understanding, instruction following, and multilingual support.",
	},
	// "grok-2-1212":        {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0},        // $2.00 input, $10.00 output

	// Image generation model (no per-token charge)
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
		InputModalities:  grokTextInputs,
		OutputModalities: []string{"image"},
		Description:      "Grok Imagine (image) — fast image generation at $0.02/image.",
	},
	"grok-imagine-image-pro": {
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
		Description:      "Grok Imagine Pro — high-fidelity image generation at $0.07/image.",
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
		Description:      "Grok 2 Image 1212 — image generation snapshot priced at $0.07/image.",
	}, // $0.07 per image
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
		Description:      "Grok 2 Image — image generation alias priced at $0.07/image.",
	}, // $0.07 per image

	"grok-imagine-video": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Video: &adaptor.VideoPricingConfig{
			PerSecondUsd: 0.05,
		},
		InputModalities: grokTextInputs,
		Description:     "Grok Imagine Video — video generation priced at $0.05 per rendered second.",
	}, // $0.05 per second

	// Legacy aliases for backward compatibility
	// "grok-beta":        {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0}, // Updated to match grok-2-1212
	// "grok-2":           {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0}, // Updated to match grok-2-1212
	// "grok-2-latest":    {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0}, // Updated to match grok-2-1212
	// "grok-vision-beta": {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0}, // Updated to match grok-2-vision-1212
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// XAIToolingDefaults captures xAI's published tool invocation fees.
// Source: https://docs.x.ai/developers/models
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
