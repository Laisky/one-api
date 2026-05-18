package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// embeddingModelRatios captures pricing and metadata for Alibaba's text-embedding
// services (text-embedding-v1 / v2 / v3 / v4 plus their async/batch variants).
//
// Embeddings expose only an input modality; SupportedFeatures and
// SupportedSamplingParameters are intentionally empty because the embeddings
// endpoint does not accept tools/JSON-mode/sampling controls. ContextLength is
// 8192 tokens per Alibaba's documentation; text-embedding-v4 inherits the same
// context but adds flexible output dimensions (64-2048) and >100 languages.
//
// Pricing per Aliyun model-pricing docs (verified 2026-05-18): all current
// text-embedding tiers are billed at 0.5 CNY per 1M input tokens, encoded as
// 0.0005 * 1000 * ratio.MilliTokensRmb (= 0.5 RMB / 1M tokens at the project's
// 8 RMB/USD rate). The Embedding helper is populated with TextTokenRatio so the
// embedding billing path can resolve a modality-specific price without falling
// back to Ratio. Sources:
//   - https://help.aliyun.com/zh/model-studio/embedding
//   - https://help.aliyun.com/zh/model-studio/model-pricing
//   - https://www.alibabacloud.com/help/en/model-studio/text-embedding-synchronous-api
const aliEmbeddingRatePerKToken = 0.0005 // 0.5 CNY / 1M tokens = 0.0005 CNY / 1k tokens

var embeddingModelRatios = map[string]adaptor.ModelConfig{
	"text-embedding-v1": {
		Ratio:            aliEmbeddingRatePerKToken * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		InputModalities:  []string{"text"},
		OutputModalities: []string{},
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: aliEmbeddingRatePerKToken * 1000 * ratio.MilliTokensRmb,
		},
		Description: "DashScope text-embedding-v1: legacy embedding endpoint, 8192-token input.",
	},
	"text-embedding-v2": {
		Ratio:            aliEmbeddingRatePerKToken * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		InputModalities:  []string{"text"},
		OutputModalities: []string{},
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: aliEmbeddingRatePerKToken * 1000 * ratio.MilliTokensRmb,
		},
		Description: "DashScope text-embedding-v2: improved multilingual embedding endpoint, 8192-token input.",
	},
	"text-embedding-v3": {
		Ratio:            aliEmbeddingRatePerKToken * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		InputModalities:  []string{"text"},
		OutputModalities: []string{},
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: aliEmbeddingRatePerKToken * 1000 * ratio.MilliTokensRmb,
		},
		Description: "DashScope text-embedding-v3: 1024-dim embedding endpoint, 8192-token input, 50+ languages.",
	},
	"text-embedding-v4": {
		Ratio:            aliEmbeddingRatePerKToken * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		InputModalities:  []string{"text"},
		OutputModalities: []string{},
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: aliEmbeddingRatePerKToken * 1000 * ratio.MilliTokensRmb,
		},
		Description: "DashScope text-embedding-v4 (Qwen3-Embedding): flexible 64-2048 dim output, 8192-token input, 100+ languages.",
	},
	"text-embedding-async-v1": {
		Ratio:            aliEmbeddingRatePerKToken * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		InputModalities:  []string{"text"},
		OutputModalities: []string{},
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: aliEmbeddingRatePerKToken * 1000 * ratio.MilliTokensRmb,
		},
		Description: "DashScope text-embedding-async-v1: batched asynchronous embedding endpoint.",
	},
	"text-embedding-async-v2": {
		Ratio:            aliEmbeddingRatePerKToken * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		InputModalities:  []string{"text"},
		OutputModalities: []string{},
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: aliEmbeddingRatePerKToken * 1000 * ratio.MilliTokensRmb,
		},
		Description: "DashScope text-embedding-async-v2: batched asynchronous multilingual embedding endpoint.",
	},
}
