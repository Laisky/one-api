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
// The Max package retired on 2026-03-10; the provider upgraded its backend
// and quota to Ultra. Historical identifiers remain configurable.

// xunfeiTextInputs is the input modality set for Spark chat models (text-only).
var xunfeiTextInputs = []string{"text"}

// xunfeiTextOutputs is the text-only output modality used by Spark.
var xunfeiTextOutputs = []string{"text"}

// xunfeiBasicFeatures captures features available on tiers without FunctionCall
// (Spark Lite / Pro / Pro-128K per the HTTP API doc).
var xunfeiBasicFeatures = []string{}

// xunfeiChatFeatures is empty because the current WebSocket contract does not
// advertise HTTP FunctionCall or JSON response_format support.
var xunfeiChatFeatures = []string{}

// xunfeiSamplingParams lists documented controls forwarded by this adaptor.
// Native top_k is not advertised: the legacy converter maps N, not TopK.
var xunfeiSamplingParams = []string{"temperature", "max_tokens"}

// ModelRatios contains provider defaults. Lite and transport-specific metadata
// were rechecked on 2026-10-12; paid compatibility tariffs are preserved.
// Sources: https://www.xfyun.cn/doc/spark/Web.html and
// https://www.xfyun.cn/doc/spark/HTTP%E8%B0%83%E7%94%A8%E6%96%87%E6%A1%A3.html
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Xunfei pricing: https://www.xfyun.cn/doc/spark/Web.html#_1-%E6%8E%A5%E5%8F%A3%E8%AF%B4%E6%98%8E
var ModelRatios = map[string]adaptor.ModelConfig{
	// Spark Lite Models — context/output per WS doc (8K/4K); Lite is free per
	// the official API documentation; use an explicit zero tariff, not a nominal fee.
	"Spark-Lite": {
		Ratio:                       0,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiBasicFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "Spark Lite public API is explicitly free; no nominal paid input or output tariff is applied. Account concurrency and negotiated service terms remain upstream-controlled.",
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
		Ratio:                       5.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             131072,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiBasicFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark Pro long-context tier with 128k context window (HTTP doc: no FunctionCall).",
	},

	// Spark Max Models — WS doc lists 8K / 8K; HTTP doc confirms FunctionCall.
	// The Max package retired on 2026-03-10 in favor of 4.0 Ultra.
	"Spark-Max": {
		Ratio:                       2.1 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiChatFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark Max higher-capability closed-weight chat model over WebSocket; package retired 2026-03-10 (backend upgraded to Spark 4.0 Ultra, quota merged into Ultra). Kept for backward compatibility.",
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
		Description:                 "iFlytek Spark Max with extended 32k context window over WebSocket; package retired 2026-03-10 (backend upgraded to Spark 4.0 Ultra, quota merged into Ultra). Kept for backward compatibility.",
	},

	// Spark 4.0 Ultra Models — WS doc lists 32K context / 32K output.
	"Spark-4.0-Ultra": {
		Ratio:                       0.8 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             32768,
		InputModalities:             xunfeiTextInputs,
		OutputModalities:            xunfeiTextOutputs,
		SupportedFeatures:           xunfeiChatFeatures,
		SupportedSamplingParameters: xunfeiSamplingParams,
		Description:                 "iFlytek Spark 4.0 Ultra flagship closed-weight chat model over WebSocket; FunctionCall requires HTTP.",
	},
}

// XunfeiToolingDefaults notes that public Spark pricing omits tool-specific charges (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://www.xfyun.cn/doc/spark/Web.html
var XunfeiToolingDefaults = adaptor.ChannelToolConfig{}
