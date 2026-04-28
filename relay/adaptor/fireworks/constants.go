package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains Fireworks serverless models with their per-token pricing.
//
// Fireworks model IDs always use the "accounts/fireworks/models/<slug>" resource name.
// Pricing reference: https://fireworks.ai/pricing#serverless-pricing (retrieved 2026-04-28).
//
// Pricing buckets:
//   - <4B dense: $0.10/1M flat
//   - 4B-16B dense: $0.20/1M flat
//   - >16B dense: $0.90/1M flat
//   - MoE 0-56B: $0.50/1M flat
//   - MoE 56.1-176B: $1.20/1M flat
//
// Popular flagship models use per-model pricing listed below.
var ModelRatios = map[string]adaptor.ModelConfig{
	// DeepSeek V4 Pro — $1.74 in / $3.48 out, discounted cached input listed separately
	"accounts/fireworks/models/deepseek-v4-pro": {
		Ratio:            1.74 * ratio.MilliTokensUsd,
		CompletionRatio:  3.48 / 1.74,
		CachedInputRatio: 0.15 * ratio.MilliTokensUsd,
	},

	// DeepSeek family — $0.56 in / $1.68 out, 50% cached discount
	"accounts/fireworks/models/deepseek-v3": {
		Ratio:            0.56 * ratio.MilliTokensUsd,
		CompletionRatio:  1.68 / 0.56,
		CachedInputRatio: 0.28 * ratio.MilliTokensUsd,
	},
	"accounts/fireworks/models/deepseek-v3p1": {
		Ratio:            0.56 * ratio.MilliTokensUsd,
		CompletionRatio:  1.68 / 0.56,
		CachedInputRatio: 0.28 * ratio.MilliTokensUsd,
	},
	"accounts/fireworks/models/deepseek-v3p2": {
		Ratio:            0.56 * ratio.MilliTokensUsd,
		CompletionRatio:  1.68 / 0.56,
		CachedInputRatio: 0.28 * ratio.MilliTokensUsd,
	},
	"accounts/fireworks/models/deepseek-r1-0528": {
		Ratio:            0.56 * ratio.MilliTokensUsd,
		CompletionRatio:  1.68 / 0.56,
		CachedInputRatio: 0.28 * ratio.MilliTokensUsd,
	},

	// GLM family
	"accounts/fireworks/models/glm-4p7": {
		Ratio:           0.60 * ratio.MilliTokensUsd,
		CompletionRatio: 2.20 / 0.60,
	},
	"accounts/fireworks/models/glm-5": {
		Ratio:            1.00 * ratio.MilliTokensUsd,
		CompletionRatio:  3.20 / 1.00,
		CachedInputRatio: 0.20 * ratio.MilliTokensUsd,
	},
	"accounts/fireworks/models/glm-5p1": {
		Ratio:            1.40 * ratio.MilliTokensUsd,
		CompletionRatio:  4.40 / 1.40,
		CachedInputRatio: 0.26 * ratio.MilliTokensUsd,
	},

	// Moonshot Kimi family
	"accounts/fireworks/models/kimi-k2p5": {
		Ratio:            0.60 * ratio.MilliTokensUsd,
		CompletionRatio:  3.00 / 0.60,
		CachedInputRatio: 0.10 * ratio.MilliTokensUsd,
	},
	"accounts/fireworks/models/kimi-k2p6": {
		Ratio:            0.95 * ratio.MilliTokensUsd,
		CompletionRatio:  4.00 / 0.95,
		CachedInputRatio: 0.16 * ratio.MilliTokensUsd,
	},

	// GPT-OSS family (OpenAI-licensed open weights)
	"accounts/fireworks/models/gpt-oss-120b": {
		Ratio:           0.15 * ratio.MilliTokensUsd,
		CompletionRatio: 0.60 / 0.15,
	},
	"accounts/fireworks/models/gpt-oss-20b": {
		Ratio:           0.07 * ratio.MilliTokensUsd,
		CompletionRatio: 0.30 / 0.07,
	},

	// Qwen family
	"accounts/fireworks/models/qwen3-vl-30b-a3b-thinking": {
		Ratio:           0.15 * ratio.MilliTokensUsd,
		CompletionRatio: 0.60 / 0.15,
	},

	// MiniMax family
	"accounts/fireworks/models/minimax-m2p5": {
		Ratio:            0.30 * ratio.MilliTokensUsd,
		CompletionRatio:  1.20 / 0.30,
		CachedInputRatio: 0.03 * ratio.MilliTokensUsd,
	},
	"accounts/fireworks/models/minimax-m2p7": {
		Ratio:            0.30 * ratio.MilliTokensUsd,
		CompletionRatio:  1.20 / 0.30,
		CachedInputRatio: 0.06 * ratio.MilliTokensUsd,
	},

	// Llama family (tiered pricing)
	"accounts/fireworks/models/llama-v3p3-70b-instruct": {
		Ratio:           0.90 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/llama-v3p1-70b-instruct": {
		Ratio:           0.90 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/llama-v3p1-8b-instruct": {
		Ratio:           0.20 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/llama-v3p2-3b-instruct": {
		Ratio:           0.10 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/llama-v3p2-1b-instruct": {
		Ratio:           0.10 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/llama-v3p2-11b-vision-instruct": {
		Ratio:           0.20 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/llama-v3p2-90b-vision-instruct": {
		Ratio:           0.90 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},

	// Qwen dense (tiered)
	"accounts/fireworks/models/qwen2p5-72b-instruct": {
		Ratio:           0.90 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/qwen3-8b": {
		Ratio:           0.20 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/qwen3-0p6b": {
		Ratio:           0.10 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},

	// Mistral / Mixtral MoE (tiered)
	"accounts/fireworks/models/mistral-7b-instruct-v0.3": {
		Ratio:           0.20 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/mixtral-8x7b-instruct": {
		Ratio:           0.50 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/mixtral-8x22b-instruct": {
		Ratio:           1.20 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/dbrx-instruct": {
		Ratio:           1.20 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},

	// Rerank models — per-input-token pricing on the embedding tier
	"accounts/fireworks/models/qwen3-reranker-8b": {
		Ratio:           0.10 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/qwen3-reranker-4b": {
		Ratio:           0.016 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/qwen3-reranker-0p6b": {
		Ratio:           0.008 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},

	// Embedding models — priced per 1M input tokens (completion_ratio unused).
	// Qwen3 embedding family — $0.10/1M
	"accounts/fireworks/models/qwen3-embedding-8b": {
		Ratio:           0.10 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/qwen3-embedding-4b": {
		Ratio:           0.016 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"accounts/fireworks/models/qwen3-embedding-0p6b": {
		Ratio:           0.008 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	// Legacy BERT embeddings — <150M = $0.008/1M, 150-350M = $0.016/1M
	"nomic-ai/nomic-embed-text-v1.5": {
		Ratio:           0.008 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"nomic-ai/nomic-embed-text-v1": {
		Ratio:           0.008 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"WhereIsAI/UAE-Large-V1": {
		Ratio:           0.016 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"thenlper/gte-large": {
		Ratio:           0.016 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"thenlper/gte-base": {
		Ratio:           0.008 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"BAAI/bge-base-en-v1.5": {
		Ratio:           0.008 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"BAAI/bge-small-en-v1.5": {
		Ratio:           0.008 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"mixedbread-ai/mxbai-embed-large-v1": {
		Ratio:           0.016 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"sentence-transformers/all-MiniLM-L6-v2": {
		Ratio:           0.008 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
	"sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2": {
		Ratio:           0.008 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
	},
}

// FireworksToolingDefaults records that Fireworks does not publish provider-level
// built-in tool pricing (retrieved 2026-04-21).
var FireworksToolingDefaults = adaptor.ChannelToolConfig{}
