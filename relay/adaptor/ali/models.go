package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Model definitions are maintained by provider family in models_*.go.
// This file is the central catalog entry point; add a family here only when needed.
// Provider-specific prices, unknown fields, and retained historical IDs remain distinct.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
// ModelRatios contains each provider model exactly once, with no patch overlays.
var ModelRatios = adaptor.JoinModelCatalogs(
	deepseekModels(),
	embeddingModels(),
	glmModels(),
	imageModels(),
	kimiModels(),
	mimoModels(),
	minimaxModels(),
	qwenAudioModels(),
	qwenChatModels(),
	qwenCoderModels(),
	qwenImageModels(),
	qwenMathModels(),
	qwenOpenWeightsModels(),
	qwenOpenWeightsLegacyModels(),
	qwenTranslationModels(),
	qwenVisionModels(),
	stepfunModels(),
	utilityModels(),
	wanModels(),
)

// AliToolingDefaults retains this provider's separately metered tool defaults.
var AliToolingDefaults = adaptor.ChannelToolConfig{}

// nativeRate converts a CNY per-million input-unit price into billing ratios.
// It accepts the published amount and returns the existing float64 calculation,
// preserving established rounding for token and character fallback tariffs.
func nativeRate(amount float64) float64 { return amount * ratio.MilliTokensRmb }
