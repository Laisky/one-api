package siliconflow

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Model definitions are production Go code: edit the relevant models_*.go file.
// These defaults preserve the effective configuration at a8782e3dc0dad7704acb0d008e7d30a26096338d.
// Provider-specific prices, unknown fields, and retained historical IDs remain distinct.
// Source: https://siliconflow.cn/pricing
// ModelRatios contains each provider model exactly once, with no patch overlays.
var ModelRatios = adaptor.JoinModelCatalogs(
	deepseek_aiModels(),
	moonshotaiModels(),
	otherModels(),
	proModels(),
	qwenModels(),
	zai_orgModels(),
)

// ModelList retains the existing channel discovery interface.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// SiliconFlowToolingDefaults retains this provider's separately metered tool defaults.
var SiliconFlowToolingDefaults = adaptor.ChannelToolConfig{}

// nativeRate converts a CNY per-million input-unit price into billing ratios.
// It accepts the published amount and returns the existing float64 calculation,
// preserving established rounding for token and character fallback tariffs.
func nativeRate(amount float64) float64 { return amount * ratio.MilliTokensRmb }
