package xunfeiv2

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Xunfei pricing: https://www.xfyun.cn/doc/spark/HTTP%E8%B0%83%E7%94%A8%E6%96%87%E6%A1%A3.html#_3-%E8%AF%B7%E6%B1%82%E8%AF%B4%E6%98%8E
var ModelRatios = map[string]adaptor.ModelConfig{
	// Xunfei Spark Models - Based on https://www.xfyun.cn/doc/spark/
	"lite":        {Ratio: 0.3 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"generalv3":   {Ratio: 2.0 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"pro-128k":    {Ratio: 5.0 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"generalv3.5": {Ratio: 2.0 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"max-32k":     {Ratio: 5.0 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"4.0Ultra":    {Ratio: 5.0 * ratio.MilliTokensRmb, CompletionRatio: 1},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// XunfeiV2ToolingDefaults notes that iFLYTEK Spark HTTP documentation lists no tool-specific pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://www.xfyun.cn/doc/spark/HTTP%E8%B0%83%E7%94%A8%E6%96%87%E6%A1%A3.html
var XunfeiV2ToolingDefaults = adaptor.ChannelToolConfig{}
