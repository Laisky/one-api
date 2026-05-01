package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// rerankModels contains Qwen3 reranker variants served by Fireworks. Pricing
// uses the embedding tier (per-input-token) since rerank scores are
// classification outputs rather than completions.
var rerankModels = map[string]adaptor.ModelConfig{
	"accounts/fireworks/models/qwen3-reranker-8b": {
		Ratio:                       0.10 * ratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               32768,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedSamplingParameters: fwRerankSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "Qwen/Qwen3-Reranker-8B",
		Description:                 "Alibaba Qwen3 Reranker 8B for cross-encoder relevance scoring with 32K context.",
	},
	"accounts/fireworks/models/qwen3-reranker-4b": {
		Ratio:                       0.016 * ratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               32768,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedSamplingParameters: fwRerankSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "Qwen/Qwen3-Reranker-4B",
		Description:                 "Alibaba Qwen3 Reranker 4B mid-size cross-encoder relevance model with 32K context.",
	},
	"accounts/fireworks/models/qwen3-reranker-0p6b": {
		Ratio:                       0.008 * ratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               32768,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedSamplingParameters: fwRerankSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "Qwen/Qwen3-Reranker-0.6B",
		Description:                 "Alibaba Qwen3 Reranker 0.6B compact cross-encoder for low-latency relevance scoring.",
	},
}
