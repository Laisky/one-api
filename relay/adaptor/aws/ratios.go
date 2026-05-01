package aws

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// awsBedrockModelPricing is the canonical source of truth for AWS Bedrock model
// pricing and metadata. It is consumed by Adaptor.GetDefaultModelPricing.
//
// Pricing references:
//   - https://aws.amazon.com/bedrock/pricing/
//
// Capability and context references:
//   - https://docs.aws.amazon.com/bedrock/latest/userguide/models-supported.html
var awsBedrockModelPricing = map[string]adaptor.ModelConfig{
	// Claude Models on AWS Bedrock
	"claude-instant-1.2": {
		Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 3.0,
		CachedInputRatio: 0.08 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite1hRatio: 1.6 * ratio.MilliTokensUsd,
		ContextLength: 100000, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeLegacyFeatures, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Anthropic Claude Instant 1.2 on AWS Bedrock (legacy fast text model).",
	},
	"claude-2.0": {
		Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 3.0,
		CachedInputRatio: 0.8 * ratio.MilliTokensUsd, CacheWrite5mRatio: 10 * ratio.MilliTokensUsd, CacheWrite1hRatio: 16 * ratio.MilliTokensUsd,
		ContextLength: 100000, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeLegacyFeatures, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Anthropic Claude 2.0 on AWS Bedrock (legacy text model).",
	},
	"claude-2.1": {
		Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 3.0,
		CachedInputRatio: 0.8 * ratio.MilliTokensUsd, CacheWrite5mRatio: 10 * ratio.MilliTokensUsd, CacheWrite1hRatio: 16 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeLegacyFeatures, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Anthropic Claude 2.1 on AWS Bedrock with 200k context.",
	},
	"claude-3-haiku-20240307": {
		Ratio: 0.25 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.03 * ratio.MilliTokensUsd, CacheWrite5mRatio: 0.30 * ratio.MilliTokensUsd, CacheWrite1hRatio: 0.5 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 4096,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3 Haiku on AWS Bedrock (compact multimodal model).",
	},
	"claude-3-5-haiku-20241022": {
		Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.08 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite1hRatio: 1.6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3.5 Haiku on AWS Bedrock (fast multimodal model).",
	},
	"claude-3-sonnet-20240229": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 4096,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3 Sonnet on AWS Bedrock (balanced multimodal model).",
	},
	"claude-3-5-sonnet-latest": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3.5 Sonnet (latest alias) on AWS Bedrock.",
	},
	"claude-3-5-sonnet-20240620": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3.5 Sonnet (June 2024 release) on AWS Bedrock.",
	},
	"claude-3-5-sonnet-20241022": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3.5 Sonnet v2 (October 2024 release) on AWS Bedrock.",
	},
	"claude-3-7-sonnet-latest": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3.7 Sonnet (latest alias) on AWS Bedrock with extended thinking.",
	},
	"claude-3-7-sonnet-20250219": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3.7 Sonnet (February 2025 release) on AWS Bedrock with extended thinking.",
	},
	"claude-sonnet-4-0": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Sonnet 4 (alias) on AWS Bedrock.",
	},
	"claude-sonnet-4-20250514": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Sonnet 4 (May 2025 release) on AWS Bedrock.",
	},
	"claude-sonnet-4-5": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Sonnet 4.5 (alias) on AWS Bedrock.",
	},
	"claude-sonnet-4-5-20250929": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Sonnet 4.5 (September 2025 release) on AWS Bedrock.",
	},
	"claude-sonnet-4-6": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Sonnet 4.6 on AWS Bedrock with 1M-token context and extended thinking.",
	},
	"claude-3-opus-20240229": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 4096,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3 Opus on AWS Bedrock (premium multimodal model).",
	},
	"claude-opus-4-0": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 32000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Opus 4 (alias) on AWS Bedrock.",
	},
	"claude-opus-4-20250514": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 32000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Opus 4 (May 2025 release) on AWS Bedrock.",
	},
	"claude-opus-4-1": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 32000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Opus 4.1 (alias) on AWS Bedrock.",
	},
	"claude-opus-4-1-20250805": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 75.0 / 15,
		CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 32000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Opus 4.1 (August 2025 release) on AWS Bedrock with extended thinking.",
	},
	"claude-opus-4-5": {
		Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5,
		CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Opus 4.5 (alias) on AWS Bedrock.",
	},
	"claude-opus-4-5-20251101": {
		Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5,
		CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Opus 4.5 (November 2025 release) on AWS Bedrock.",
	},
	"claude-opus-4-6": {
		Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5,
		CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Opus 4.6 on AWS Bedrock with 1M-token context and extended thinking.",
	},
	"claude-opus-4-7": {
		Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5,
		CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Opus 4.7 on AWS Bedrock with 1M-token context and adaptive thinking.",
	},

	// Llama Models on AWS Bedrock
	// Llama 4 models - Pricing given per 1K tokens; normalize to $/1M then to $/token via ratio.MilliTokensUsd
	"llama4-maverick-17b-1m": {
		Ratio: 0.24 * ratio.MilliTokensUsd, CompletionRatio: 4.04,
		ContextLength: 1000000, MaxOutputTokens: 4096,
		InputModalities: awsVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsLlamaToolsFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "meta-llama/Llama-4-Maverick-17B-128E-Instruct",
		Description:   "Meta Llama 4 Maverick 17B with 1M-token context on AWS Bedrock.",
	},
	"llama4-scout-17b-3.5m": {
		Ratio: 0.17 * ratio.MilliTokensUsd, CompletionRatio: 3.88,
		ContextLength: 3500000, MaxOutputTokens: 4096,
		InputModalities: awsVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsLlamaToolsFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "meta-llama/Llama-4-Scout-17B-16E-Instruct",
		Description:   "Meta Llama 4 Scout 17B with 3.5M-token context on AWS Bedrock.",
	},

	// Llama 3.3 models
	"llama3-3-70b-128k": {
		Ratio: 0.72 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsLlamaToolsFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "meta-llama/Llama-3.3-70B-Instruct",
		Description:   "Meta Llama 3.3 70B Instruct with 128k context on AWS Bedrock.",
	},

	// Llama 3.2 models
	"llama3-2-1b-131k": {
		Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 131072, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsLlamaFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "meta-llama/Llama-3.2-1B-Instruct",
		Description:   "Meta Llama 3.2 1B Instruct on AWS Bedrock.",
	},
	"llama3-2-3b-131k": {
		Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 131072, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsLlamaFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "meta-llama/Llama-3.2-3B-Instruct",
		Description:   "Meta Llama 3.2 3B Instruct on AWS Bedrock.",
	},
	"llama3-2-11b-vision-131k": {
		Ratio: 0.16 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 131072, MaxOutputTokens: 4096,
		InputModalities: awsVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsLlamaFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "meta-llama/Llama-3.2-11B-Vision-Instruct",
		Description:   "Meta Llama 3.2 11B Vision Instruct on AWS Bedrock.",
	},
	"llama3-2-90b-128k": {
		Ratio: 0.72 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: awsVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsLlamaFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "meta-llama/Llama-3.2-90B-Vision-Instruct",
		Description:   "Meta Llama 3.2 90B Vision Instruct on AWS Bedrock.",
	},

	// Llama 3.1 models
	"llama3-1-8b-128k": {
		Ratio: 0.22 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsLlamaToolsFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "meta-llama/Llama-3.1-8B-Instruct",
		Description:   "Meta Llama 3.1 8B Instruct on AWS Bedrock.",
	},
	"llama3-1-70b-128k": {
		Ratio: 0.72 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsLlamaToolsFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "meta-llama/Llama-3.1-70B-Instruct",
		Description:   "Meta Llama 3.1 70B Instruct on AWS Bedrock.",
	},

	// Llama 3 models
	"llama3-8b-8192": {
		Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 2,
		ContextLength: 8192, MaxOutputTokens: 2048,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsLlamaFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "meta-llama/Meta-Llama-3-8B-Instruct",
		Description:   "Meta Llama 3 8B Instruct on AWS Bedrock.",
	},
	"llama3-70b-8192": {
		Ratio: 2.65 * ratio.MilliTokensUsd, CompletionRatio: 1.32,
		ContextLength: 8192, MaxOutputTokens: 2048,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsLlamaFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "meta-llama/Meta-Llama-3-70B-Instruct",
		Description:   "Meta Llama 3 70B Instruct on AWS Bedrock.",
	},

	// Amazon Nova Models
	"amazon-nova-micro": {
		Ratio: 0.035 * ratio.MilliTokensUsd, CompletionRatio: 4.28,
		ContextLength: 128000, MaxOutputTokens: 5120,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsNovaFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		Description: "Amazon Nova Micro fast text-only chat model.",
	},
	"amazon-nova-lite": {
		Ratio: 0.06 * ratio.MilliTokensUsd, CompletionRatio: 4.17,
		ContextLength: 300000, MaxOutputTokens: 5120,
		InputModalities: awsVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsNovaFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		Description: "Amazon Nova Lite multimodal model with 300k context.",
	},
	"amazon-nova-pro": {
		Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 300000, MaxOutputTokens: 5120,
		InputModalities: awsVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsNovaFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		Description: "Amazon Nova Pro multimodal flagship with 300k context.",
	},
	"amazon-nova-premier": {
		Ratio: 2.4 * ratio.MilliTokensUsd, CompletionRatio: 4.17,
		ContextLength: 1000000, MaxOutputTokens: 5120,
		InputModalities: awsVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsNovaFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		Description: "Amazon Nova Premier multimodal model with 1M-token context.",
	},

	// Titan Models
	"amazon-titan-text-lite": {
		Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 1.33,
		ContextLength: 4096, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedSamplingParameters: awsBasicSamplingParams,
		Description:                 "Amazon Titan Text Lite on AWS Bedrock.",
	},
	"amazon-titan-text-express": {
		Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 2,
		ContextLength: 8192, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedSamplingParameters: awsBasicSamplingParams,
		Description:                 "Amazon Titan Text Express on AWS Bedrock.",
	},
	"amazon-titan-embed-text": {
		Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength:   8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		Description: "Amazon Titan Text Embeddings model on AWS Bedrock.",
	},

	// Cohere Models
	"command-r": {
		Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsCohereFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "CohereLabs/c4ai-command-r-v01",
		Description:   "Cohere Command R on AWS Bedrock for RAG and tool-use.",
	},
	"command-r-plus": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5,
		ContextLength: 128000, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsCohereFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "CohereLabs/c4ai-command-r-plus",
		Description:   "Cohere Command R+ on AWS Bedrock for advanced RAG and tool-use.",
	},

	// Qwen Models
	"qwen3-235b": {
		Ratio: 0.22 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 256000, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsQwenFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "Qwen/Qwen3-235B-A22B-Instruct-2507",
		Description:   "Alibaba Qwen3 235B mixture-of-experts model on AWS Bedrock.",
	},
	"qwen3-32b": {
		Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsQwenFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "Qwen/Qwen3-32B",
		Description:   "Alibaba Qwen3 32B dense reasoning model on AWS Bedrock.",
	},
	"qwen3-coder-30b": {
		Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 256000, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsQwenFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "Qwen/Qwen3-Coder-30B-A3B-Instruct",
		Description:   "Alibaba Qwen3 Coder 30B model tuned for code on AWS Bedrock.",
	},
	"qwen3-coder-480b": {
		Ratio: 0.22 * ratio.MilliTokensUsd, CompletionRatio: 8.18,
		ContextLength: 256000, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsQwenFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "Qwen/Qwen3-Coder-480B-A35B-Instruct",
		Description:   "Alibaba Qwen3 Coder 480B mixture-of-experts model on AWS Bedrock.",
	},

	// AI21 Models
	"ai21-j2-mid": {
		Ratio: 12.5 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 8192, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedSamplingParameters: awsBasicSamplingParams,
		Description:                 "AI21 Jurassic-2 Mid (legacy) on AWS Bedrock.",
	},
	"ai21-j2-ultra": {
		Ratio: 18.8 * ratio.MilliTokensUsd, CompletionRatio: 1,
		ContextLength: 8192, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedSamplingParameters: awsBasicSamplingParams,
		Description:                 "AI21 Jurassic-2 Ultra (legacy) on AWS Bedrock.",
	},
	"ai21-jamba-1.5": {
		Ratio: 2 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 256000, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: []string{"tools"}, SupportedSamplingParameters: awsBasicSamplingParams,
		Description: "AI21 Jamba 1.5 hybrid SSM-Transformer model on AWS Bedrock.",
	},

	// DeepSeek Models
	"deepseek-r1": {
		Ratio: 1.35 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 128000, MaxOutputTokens: 32768,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsDeepSeekFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "deepseek-ai/DeepSeek-R1",
		Description:   "DeepSeek R1 reasoning model on AWS Bedrock.",
	},
	"deepseek-v3.1": {
		Ratio: 0.58 * ratio.MilliTokensUsd, CompletionRatio: 2.9,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsDeepSeekFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "deepseek-ai/DeepSeek-V3.1",
		Description:   "DeepSeek V3.1 hybrid reasoning model on AWS Bedrock.",
	},

	// Mistral Models
	"mistral-7b-instruct": {
		Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1.33,
		ContextLength: 32768, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsMistralFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "mistralai/Mistral-7B-Instruct-v0.2",
		Description:   "Mistral 7B Instruct on AWS Bedrock.",
	},
	"mistral-8x7b-instruct": {
		Ratio: 0.45 * ratio.MilliTokensUsd, CompletionRatio: 1.56,
		ContextLength: 32768, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsMistralFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "mistralai/Mixtral-8x7B-Instruct-v0.1",
		Description:   "Mixtral 8x7B Instruct mixture-of-experts on AWS Bedrock.",
	},
	"mistral-large": {
		Ratio: 4 * ratio.MilliTokensUsd, CompletionRatio: 3,
		ContextLength: 32768, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsMistralFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		Description: "Mistral Large flagship chat model on AWS Bedrock.",
	},
	"mistral-7b": {
		Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1.33,
		ContextLength: 32768, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsMistralFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "mistralai/Mistral-7B-Instruct-v0.2",
		Description:   "Mistral 7B alias on AWS Bedrock.",
	},
	"mixtral-8x7b": {
		Ratio: 0.45 * ratio.MilliTokensUsd, CompletionRatio: 1.56,
		ContextLength: 32768, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsMistralFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "mistralai/Mixtral-8x7B-Instruct-v0.1",
		Description:   "Mixtral 8x7B alias on AWS Bedrock.",
	},
	"mistral-small-2402": {
		Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 3,
		ContextLength: 32768, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsMistralFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		Description: "Mistral Small (February 2024 release) on AWS Bedrock.",
	},
	"mistral-large-2402": {
		Ratio: 4 * ratio.MilliTokensUsd, CompletionRatio: 3,
		ContextLength: 32768, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsMistralFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		Description: "Mistral Large (February 2024 release) on AWS Bedrock.",
	},
	"mistral-pixtral-large-2502": {
		Ratio: 2 * ratio.MilliTokensUsd, CompletionRatio: 3,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: awsVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsMistralFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		Description: "Mistral Pixtral Large multimodal vision model (February 2025) on AWS Bedrock.",
	},

	// OpenAI OSS Models
	"gpt-oss-20b": {
		Ratio: 0.07 * ratio.MilliTokensUsd, CompletionRatio: 4.29,
		ContextLength: 128000, MaxOutputTokens: 32768,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsOpenAIOSSFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "openai/gpt-oss-20b",
		Description:   "OpenAI gpt-oss 20B open-weight reasoning model on AWS Bedrock.",
	},
	"gpt-oss-120b": {
		Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 128000, MaxOutputTokens: 32768,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsOpenAIOSSFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		HuggingFaceID: "openai/gpt-oss-120b",
		Description:   "OpenAI gpt-oss 120B open-weight reasoning model on AWS Bedrock.",
	},

	// Writer Models
	"palmyra-x4": {
		Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4,
		ContextLength: 128000, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsWriterFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		Description: "Writer Palmyra X4 enterprise chat model on AWS Bedrock.",
	},
	"palmyra-x5": {
		Ratio: 0.6 * ratio.MilliTokensUsd, CompletionRatio: 10,
		ContextLength: 1000000, MaxOutputTokens: 8192,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsWriterFeatures, SupportedSamplingParameters: awsBasicSamplingParams,
		Description: "Writer Palmyra X5 enterprise chat model with 1M-token context on AWS Bedrock.",
	},
}
