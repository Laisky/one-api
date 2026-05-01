package xunfei

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// iFlytek Spark (讯飞星火) is a closed-weight Chinese cloud LLM family.
// Reference docs:
//   - https://www.xfyun.cn/doc/spark/Web.html (websocket API)
//   - https://www.xfyun.cn/doc/spark/HTTP%E8%B0%83%E7%94%A8%E6%96%87%E6%A1%A3.html (HTTP API)
//
// All Spark SKUs are closed-weight (HuggingFaceID / Quantization stay empty).
// Spark Pro / Max / 4.0 Ultra advertise function-calling support; Spark Lite is
// the cost-optimized SKU without function calling.

// xunfeiTextInputs is the input modality set for Spark chat models (text-only).
var xunfeiTextInputs = []string{"text"}

// xunfeiTextOutputs is the text-only output modality used by Spark.
var xunfeiTextOutputs = []string{"text"}

// xunfeiBasicFeatures captures features available on the cost-optimized
// Spark-Lite tier (no function calling).
var xunfeiBasicFeatures = []string{}

// xunfeiChatFeatures advertises tool calling and JSON mode for Spark Pro/Max
// and Spark 4.0 Ultra tiers.
var xunfeiChatFeatures = []string{"tools", "json_mode"}

// xunfeiSamplingParams enumerates sampling parameters accepted by Spark
// chat completions exposed through the HTTP API.
var xunfeiSamplingParams = []string{
	"temperature",
	"top_p",
	"top_k",
	"max_tokens",
	"stop",
}

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Xunfei pricing: https://www.xfyun.cn/doc/spark/Web.html#_1-%E6%8E%A5%E5%8F%A3%E8%AF%B4%E6%98%8E
var ModelRatios = map[string]adaptor.ModelConfig{
	// Spark Lite Models
	"Spark-Lite": {
		Ratio:                       0.3 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               4096,
		MaxOutputTokens:             4096,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiBasicFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark Lite cost-optimized closed-weight chat model.",
	},

	// Spark Pro Models
	"Spark-Pro": {
		Ratio:                       1.26 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiChatFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark Pro closed-weight chat model with function calling support.",
	},
	"Spark-Pro-128K": {
		Ratio:                       1.26 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiChatFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark Pro long-context tier with 128k context window.",
	},

	// Spark Max Models
	"Spark-Max": {
		Ratio:                       2.1 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiChatFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark Max higher-capability closed-weight chat model.",
	},
	"Spark-Max-32K": {
		Ratio:                       2.1 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiChatFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark Max with extended 32k context window.",
	},

	// Spark 4.0 Ultra Models
	"Spark-4.0-Ultra": {
		Ratio:                       5.6 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiChatFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark 4.0 Ultra flagship closed-weight chat model.",
	},
}

// XunfeiToolingDefaults notes that public Spark pricing omits tool-specific charges (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://www.xfyun.cn/doc/spark/Web.html
var XunfeiToolingDefaults = adaptor.ChannelToolConfig{}
