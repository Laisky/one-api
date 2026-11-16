package baiduv2

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Model definitions are maintained by family in models_*.go.
// This file is the central catalog entry point.
// Provider-specific prices, unknown fields, and retained historical IDs remain distinct.
// Source: https://cloud.baidu.com/doc/qianfan/s/wmh4sv6ya
// ModelRatios contains each provider model exactly once, with no patch overlays.
var ModelRatios = adaptor.JoinModelCatalogs(deepseekModels(), ernieModels(), glmModels(), qwenModels())

// ModelList retains the existing channel discovery interface.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// BaiduV2ToolingDefaults retains this provider's separately metered tool defaults.
var BaiduV2ToolingDefaults = adaptor.ChannelToolConfig{}

// nativeRate converts a CNY per-million input-unit price into billing ratios.
// It accepts the published amount and returns the existing float64 calculation,
// preserving established rounding for token and character fallback tariffs.
func nativeRate(amount float64) float64 { return amount * ratio.MilliTokensRmb }
