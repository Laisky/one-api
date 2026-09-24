package openrouter

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Model definitions are production Go code: edit the relevant models_*.go file.
// These defaults preserve the effective configuration at a8782e3dc0dad7704acb0d008e7d30a26096338d.
// Provider-specific prices, unknown fields, and retained historical IDs remain distinct.
// Source: https://openrouter.ai/api/v1/models
// ModelRatios contains each provider model exactly once, with no patch overlays.
var ModelRatios = adaptor.JoinModelCatalogs(
	aion_labsModels(),
	anthropicModels(),
	arcee_aiModels(),
	baiduModels(),
	bytedance_seedModels(),
	cohereModels(),
	deepseekModels(),
	googleModels(),
	inclusionaiModels(),
	metaModels(),
	meta_llamaModels(),
	minimaxModels(),
	mistralaiModels(),
	moonshotaiModels(),
	nex_agiModels(),
	nousresearchModels(),
	nvidiaModels(),
	openaiModels(),
	openai_2Models(),
	otherModels(),
	other_2Models(),
	poolsideModels(),
	qwenModels(),
	qwen_2Models(),
	tencentModels(),
	x_aiModels(),
	xiaomiModels(),
	z_aiModels(),
)

// OpenRouterToolingDefaults retains this provider's separately metered tool defaults.
var OpenRouterToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"openai_mini_native_search_high": adaptor.ToolPricingConfig{
			UsdPerCall: 0.03,
		},
		"openai_mini_native_search_low": adaptor.ToolPricingConfig{
			UsdPerCall: 0.025,
		},
		"openai_mini_native_search_medium": adaptor.ToolPricingConfig{
			UsdPerCall: 0.0275,
		},
		"openai_native_search_high": adaptor.ToolPricingConfig{
			UsdPerCall: 0.05,
		},
		"openai_native_search_low": adaptor.ToolPricingConfig{
			UsdPerCall: 0.03,
		},
		"openai_native_search_medium": adaptor.ToolPricingConfig{
			UsdPerCall: 0.035,
		},
		"perplexity_native_search_high": adaptor.ToolPricingConfig{
			UsdPerCall: 0.012,
		},
		"perplexity_native_search_low": adaptor.ToolPricingConfig{
			UsdPerCall: 0.005,
		},
		"perplexity_native_search_medium": adaptor.ToolPricingConfig{
			UsdPerCall: 0.008,
		},
		"perplexity_pro_native_search_high": adaptor.ToolPricingConfig{
			UsdPerCall: 0.014,
		},
		"perplexity_pro_native_search_low": adaptor.ToolPricingConfig{
			UsdPerCall: 0.006,
		},
		"perplexity_pro_native_search_medium": adaptor.ToolPricingConfig{
			UsdPerCall: 0.01,
		},
		"web_plugin_exa": adaptor.ToolPricingConfig{
			UsdPerCall: 0.02,
		},
	},
}

// nativeRate converts a USD per-million input-unit price into billing ratios.
// It accepts the published amount and returns the existing float64 calculation,
// preserving established rounding for token and character fallback tariffs.
func nativeRate(amount float64) float64 { return amount * ratio.MilliTokensUsd }
