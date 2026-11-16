package openrouter

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios assembles the openrouter catalog from stable model-family files.
// Each model ID has one definition; JoinModelCatalogs rejects duplicate IDs.
// Source: https://openrouter.ai/api/v1/models
var ModelRatios = modelCatalog()

// modelCatalog returns a fresh complete catalog with independently owned family metadata.
// It takes no arguments and rejects duplicate model IDs during assembly.
func modelCatalog() map[string]adaptor.ModelConfig {
	return adaptor.JoinModelCatalogs(
		aionLabsModels(),
		amazonModels(),
		anthropicModels(),
		arceeAiModels(),
		baiduModels(),
		bytedanceSeedModels(),
		cohereModels(),
		deepseekModels(),
		googleModels(),
		ibmGraniteModels(),
		inclusionaiModels(),
		liquidModels(),
		metaModels(),
		metaLlamaModels(),
		minimaxModels(),
		mistralaiModels(),
		moonshotaiModels(),
		nexAgiModels(),
		nousresearchModels(),
		nvidiaModels(),
		openaiModels(),
		openaiGpt5Models(),
		openaiGpt6Models(),
		openaiReasoningModels(),
		otherModels(),
		perplexityModels(),
		poolsideModels(),
		qwenModels(),
		qwenCoderModels(),
		qwenVisionModels(),
		sakanaModels(),
		sao10kModels(),
		stepfunModels(),
		tencentModels(),
		thedrummerModels(),
		thinkingmachinesModels(),
		unbiasedModels(),
		upstageModels(),
		xAiModels(),
		xiaomiModels(),
		zAiModels(),
	)
}

// OpenRouterToolingDefaults retains this provider's separately metered tool defaults.
var OpenRouterToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"openai_mini_native_search_high": {
			UsdPerCall: 0.03,
		},
		"openai_mini_native_search_low": {
			UsdPerCall: 0.025,
		},
		"openai_mini_native_search_medium": {
			UsdPerCall: 0.0275,
		},
		"openai_native_search_high": {
			UsdPerCall: 0.05,
		},
		"openai_native_search_low": {
			UsdPerCall: 0.03,
		},
		"openai_native_search_medium": {
			UsdPerCall: 0.035,
		},
		"perplexity_native_search_high": {
			UsdPerCall: 0.012,
		},
		"perplexity_native_search_low": {
			UsdPerCall: 0.005,
		},
		"perplexity_native_search_medium": {
			UsdPerCall: 0.008,
		},
		"perplexity_pro_native_search_high": {
			UsdPerCall: 0.014,
		},
		"perplexity_pro_native_search_low": {
			UsdPerCall: 0.006,
		},
		"perplexity_pro_native_search_medium": {
			UsdPerCall: 0.01,
		},
		"web_plugin_exa": {
			UsdPerCall: 0.02,
		},
	},
}

// nativeRate converts a USD per-million input-unit price into billing ratios.
// It accepts the published amount and returns the existing float64 calculation,
// preserving established rounding for token and character fallback tariffs.
func nativeRate(amount float64) float64 { return amount * ratio.MilliTokensUsd }
