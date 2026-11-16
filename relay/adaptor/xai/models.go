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
//   - https://docs.x.ai/developers/pricing
//
// Sources checked in the existing August and September 2026 reviews.
// Model-specific effective pricing windows live alongside their definitions.
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
	// grokXHighReasoningEfforts is the non-disableable effort ladder used by
	// Grok 4.5, Grok 4.6, and Grok 4.7 (verified 2026-09-21).
	grokXHighReasoningEfforts = []string{"low", "medium", "high", "xhigh"}

	// grokMultiAgentEfforts lists the reasoning.effort values accepted by
	// grok-4.20 multi-agent variants. For multi-agent the effort level
	// controls how many agents collaborate (4 on low/medium, 16 on high/xhigh)
	// rather than reasoning depth.
	// Source: https://docs.x.ai/developers/model-capabilities/text/multi-agent
	grokMultiAgentEfforts = []string{"low", "medium", "high", "xhigh"}
)

// grokLongContextTier returns the published xAI surcharge for requests with
// at least 200K input tokens. Parameters are the long-context input, output,
// and cached-input prices in USD per million tokens. Returns a pricing tier
// that can be attached to a model's short-context base pricing.
func grokLongContextTier(input, output, cached float64) adaptor.ModelRatioTier {
	return adaptor.ModelRatioTier{
		Ratio:               input * ratio.MilliTokensUsd,
		CompletionRatio:     output / input,
		CachedInputRatio:    cached * ratio.MilliTokensUsd,
		InputTokenThreshold: 200_000,
	}
}

// ModelRatios contains all supported models and their pricing ratios.
// Model list is derived from the keys of this map, eliminating redundancy.
//
// Note on metadata fields: all Grok models are closed-weight, so Quantization
// and HuggingFaceID are intentionally left empty for every entry. xAI does not
// publish a separate MaxOutputTokens limit for chat completions — the published
// context window is the cap for input+output combined — so MaxOutputTokens is
// left at zero (unspecified) and callers should fall back to ContextLength.
//
// Retirement notice (May 15, 2026): most legacy chat slugs below were retired
// by xAI and now auto-redirect to grok-4.3 (billed at grok-4.3 rates):
//   - grok-4-1-fast-reasoning, grok-4-1-fast-non-reasoning
//   - grok-4-fast-reasoning, grok-4-fast-non-reasoning
//   - grok-4-0709, grok-3
//
// grok-code-fast-1 instead redirects to grok-build-0.1.
//   - grok-imagine-image-pro (redirects to grok-imagine-image-quality)
//
// We keep the retired slugs in this table so existing clients continue to be
// priced correctly, but their metadata (context, modalities, features, pricing)
// mirrors grok-4.3 to reflect the actual upstream behavior.
//
// Official sources:
//   - https://docs.x.ai/developers/models
//   - https://docs.x.ai/developers/pricing
//   - https://docs.x.ai/developers/migration/may-15-retirement
//   - https://docs.x.ai/developers/model-capabilities/text/multi-agent
//   - https://docs.x.ai/developers/tools/web-search
var ModelRatios = adaptor.JoinModelCatalogs(chatModels(), imageModels(), videoModels())

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
