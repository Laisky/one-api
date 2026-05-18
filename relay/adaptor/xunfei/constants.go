package xunfei

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// iFlytek Spark (讯飞星火) is a closed-weight Chinese cloud LLM family.
// Reference docs (retrieved 2026-05-18):
//   - https://www.xfyun.cn/doc/spark/Web.html (websocket API; context/output sizes)
//   - https://www.xfyun.cn/doc/spark/HTTP%E8%B0%83%E7%94%A8%E6%96%87%E6%A1%A3.html (HTTP API; FunctionCall)
//   - https://xinghuo.xfyun.cn/sparkapi (consumer pricing page; CNY per 1M tokens)
//
// All Spark SKUs are closed-weight (HuggingFaceID / Quantization stay empty).
// Per the HTTP API doc only Spark 4.0 Ultra / Max / Max-32K support FunctionCall;
// Spark Lite / Pro / Pro-128K are advertised as chat-only SKUs.
// The Max tier is scheduled to be deprecated on 2026-03-10 (its credits will be
// upgraded to 4.0 Ultra) — kept in the map for now per "don't remove unretired".

// xunfeiTextInputs is the input modality set for Spark chat models (text-only).
var xunfeiTextInputs = []string{"text"}

// xunfeiTextOutputs is the text-only output modality used by Spark.
var xunfeiTextOutputs = []string{"text"}

// xunfeiBasicFeatures captures features available on tiers without FunctionCall
// (Spark Lite / Pro / Pro-128K per the HTTP API doc).
var xunfeiBasicFeatures = []string{}

// xunfeiChatFeatures advertises tool calling and JSON mode for Spark Max,
// Max-32K, and Spark 4.0 Ultra tiers (the HTTP doc lists these as supporting
// `system` and FunctionCall).
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
	// Spark Lite Models — context/output per WS doc (8K/4K); Lite is free per
	// the consumer pricing page so we retain a token-quota-friendly nominal rate.
	"Spark-Lite": {
		Ratio:                       0.3 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiBasicFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark Lite cost-optimized closed-weight chat model (free tier per xinghuo.xfyun.cn pricing).",
	},

	// Spark Pro Models — WS doc lists 8K context / 8K output.
	"Spark-Pro": {
		Ratio:                       1.26 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiBasicFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark Pro closed-weight chat model (HTTP doc: no FunctionCall on Pro tier).",
	},
	"Spark-Pro-128K": {
		Ratio:                       1.26 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             32768,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiBasicFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark Pro long-context tier with 128k context window (HTTP doc: no FunctionCall).",
	},

	// Spark Max Models — WS doc lists 8K / 8K; HTTP doc confirms FunctionCall.
	// Max is scheduled to be deprecated on 2026-03-10 in favor of 4.0 Ultra.
	"Spark-Max": {
		Ratio:                       2.1 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiChatFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark Max higher-capability closed-weight chat model with FunctionCall (scheduled deprecation 2026-03-10).",
	},
	"Spark-Max-32K": {
		Ratio:                       2.1 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             32768,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiChatFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark Max with extended 32k context window and FunctionCall (scheduled deprecation 2026-03-10).",
	},

	// Spark 4.0 Ultra Models — WS doc lists 32K context / 32K output.
	"Spark-4.0-Ultra": {
		Ratio:                       5.6 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             32768,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiChatFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark 4.0 Ultra flagship closed-weight chat model with FunctionCall.",
	},
}

// XunfeiToolingDefaults notes that public Spark pricing omits tool-specific charges (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://www.xfyun.cn/doc/spark/Web.html
var XunfeiToolingDefaults = adaptor.ChannelToolConfig{}
