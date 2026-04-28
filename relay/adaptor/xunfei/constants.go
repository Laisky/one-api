package xunfei

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Xunfei pricing: https://www.xfyun.cn/doc/spark/Web.html#_1-%E6%8E%A5%E5%8F%A3%E8%AF%B4%E6%98%8E
var ModelRatios = map[string]adaptor.ModelConfig{
	// Spark Lite Models
	"Spark-Lite": {Ratio: 0.3 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Spark Pro Models
	"Spark-Pro":      {Ratio: 1.26 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"Spark-Pro-128K": {Ratio: 1.26 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Spark Max Models
	"Spark-Max":     {Ratio: 2.1 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"Spark-Max-32K": {Ratio: 2.1 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Spark 4.0 Ultra Models
	"Spark-4.0-Ultra": {Ratio: 5.6 * ratio.MilliTokensRmb, CompletionRatio: 1},
}

// XunfeiToolingDefaults notes that public Spark pricing omits tool-specific charges (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://www.xfyun.cn/doc/spark/Web.html
var XunfeiToolingDefaults = adaptor.ChannelToolConfig{}
