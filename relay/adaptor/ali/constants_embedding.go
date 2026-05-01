package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// embeddingModelRatios captures pricing and metadata for Alibaba's text-embedding
// services (text-embedding-v1 / v2 / v3 plus their async variants).
//
// Embeddings expose only an input modality; SupportedFeatures and
// SupportedSamplingParameters are intentionally empty because the embeddings
// endpoint does not accept tools/JSON-mode/sampling controls. ContextLength is
// 8192 tokens per Alibaba's documentation (verified 2026-05-01 at
// https://help.aliyun.com/zh/model-studio/getting-started/models).
var embeddingModelRatios = map[string]adaptor.ModelConfig{
	"text-embedding-v1": {
		Ratio:            0.5 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		InputModalities:  []string{"text"},
		OutputModalities: []string{},
		Description:      "DashScope text-embedding-v1: legacy embedding endpoint, 8192-token input.",
	},
	"text-embedding-v2": {
		Ratio:            0.5 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		InputModalities:  []string{"text"},
		OutputModalities: []string{},
		Description:      "DashScope text-embedding-v2: improved multilingual embedding endpoint, 8192-token input.",
	},
	"text-embedding-v3": {
		Ratio:            0.5 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		InputModalities:  []string{"text"},
		OutputModalities: []string{},
		Description:      "DashScope text-embedding-v3: latest text embedding endpoint, 8192-token input.",
	},
	"text-embedding-async-v1": {
		Ratio:            0.5 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		InputModalities:  []string{"text"},
		OutputModalities: []string{},
		Description:      "DashScope text-embedding-async-v1: batched asynchronous embedding endpoint.",
	},
	"text-embedding-async-v2": {
		Ratio:            0.5 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		InputModalities:  []string{"text"},
		OutputModalities: []string{},
		Description:      "DashScope text-embedding-async-v2: batched asynchronous multilingual embedding endpoint.",
	},
}
