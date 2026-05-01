package xunfeiv2

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// iFlytek Spark v2 (HTTP / OpenAI-compatible) reuses the same closed-weight
// model lineup as the websocket adaptor but addresses models by the upstream
// "domain" identifier (lite / generalv3 / pro-128k / generalv3.5 / max-32k /
// 4.0Ultra). Reference docs:
//   - https://www.xfyun.cn/doc/spark/HTTP%E8%B0%83%E7%94%A8%E6%96%87%E6%A1%A3.html

// xunfeiv2TextInputs is the input modality set for Spark v2 chat models.
var xunfeiv2TextInputs = []string{"text"}

// xunfeiv2TextOutputs is the text-only output modality used by Spark v2.
var xunfeiv2TextOutputs = []string{"text"}

// xunfeiv2BasicFeatures captures the feature set of the cost-optimized Spark
// Lite tier (no function calling).
var xunfeiv2BasicFeatures = []string{}

// xunfeiv2ChatFeatures advertises tool / JSON mode capabilities for Spark
// generalv3 / generalv3.5 / max / 4.0Ultra tiers.
var xunfeiv2ChatFeatures = []string{"tools", "json_mode"}

// xunfeiv2SamplingParams enumerates sampling parameters accepted by Spark v2
// chat completions exposed through the HTTP API.
var xunfeiv2SamplingParams = []string{
	"temperature",
	"top_p",
	"top_k",
	"max_tokens",
	"stop",
}

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Xunfei pricing: https://www.xfyun.cn/doc/spark/HTTP%E8%B0%83%E7%94%A8%E6%96%87%E6%A1%A3.html#_3-%E8%AF%B7%E6%B1%82%E8%AF%B4%E6%98%8E
var ModelRatios = map[string]adaptor.ModelConfig{
	// Xunfei Spark Models - Based on https://www.xfyun.cn/doc/spark/
	"lite": {
		Ratio:                       0.3 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               4096,
		MaxOutputTokens:             4096,
		InputModalities:             xunfeiv2TextInputs,
		OutputModalities:            xunfeiv2TextOutputs,
		SupportedFeatures:           xunfeiv2BasicFeatures,
		SupportedSamplingParameters: xunfeiv2SamplingParams,
		Description:                 "iFlytek Spark Lite (HTTP API domain `lite`) cost-optimized closed-weight chat model.",
	},
	"generalv3": {
		Ratio:                       2.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             xunfeiv2TextInputs,
		OutputModalities:            xunfeiv2TextOutputs,
		SupportedFeatures:           xunfeiv2ChatFeatures,
		SupportedSamplingParameters: xunfeiv2SamplingParams,
		Description:                 "iFlytek Spark Pro (HTTP API domain `generalv3`) closed-weight chat model with function calling.",
	},
	"pro-128k": {
		Ratio:                       5.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             xunfeiv2TextInputs,
		OutputModalities:            xunfeiv2TextOutputs,
		SupportedFeatures:           xunfeiv2ChatFeatures,
		SupportedSamplingParameters: xunfeiv2SamplingParams,
		Description:                 "iFlytek Spark Pro long-context (HTTP API domain `pro-128k`) with 128k context window.",
	},
	"generalv3.5": {
		Ratio:                       2.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             xunfeiv2TextInputs,
		OutputModalities:            xunfeiv2TextOutputs,
		SupportedFeatures:           xunfeiv2ChatFeatures,
		SupportedSamplingParameters: xunfeiv2SamplingParams,
		Description:                 "iFlytek Spark Max (HTTP API domain `generalv3.5`) closed-weight chat model with function calling.",
	},
	"max-32k": {
		Ratio:                       5.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             xunfeiv2TextInputs,
		OutputModalities:            xunfeiv2TextOutputs,
		SupportedFeatures:           xunfeiv2ChatFeatures,
		SupportedSamplingParameters: xunfeiv2SamplingParams,
		Description:                 "iFlytek Spark Max long-context (HTTP API domain `max-32k`) with 32k context window.",
	},
	"4.0Ultra": {
		Ratio:                       5.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             xunfeiv2TextInputs,
		OutputModalities:            xunfeiv2TextOutputs,
		SupportedFeatures:           xunfeiv2ChatFeatures,
		SupportedSamplingParameters: xunfeiv2SamplingParams,
		Description:                 "iFlytek Spark 4.0 Ultra (HTTP API domain `4.0Ultra`) flagship closed-weight chat model.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// XunfeiV2ToolingDefaults notes that iFLYTEK Spark HTTP documentation lists no tool-specific pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://www.xfyun.cn/doc/spark/HTTP%E8%B0%83%E7%94%A8%E6%96%87%E6%A1%A3.html
var XunfeiV2ToolingDefaults = adaptor.ChannelToolConfig{}
