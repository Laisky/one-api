package aws

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Catalog sources (existing verified contracts):
// - https://aws.amazon.com/bedrock/pricing/
// - https://aws.amazon.com/nova/pricing/
// - https://docs.aws.amazon.com/bedrock/latest/userguide/models-supported.html
// - https://docs.aws.amazon.com/bedrock/latest/userguide/model-cards.html
// - https://platform.claude.com/docs/en/models/overview
// - https://platform.claude.com/docs/en/about-claude/pricing
// - https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-fable-5-2.html
// - https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-mythos-5-2.html
// - https://platform.claude.com/docs/en/models/fable-5-2/whats-new-fable-5-2
// - https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-opus-5-5.html
// - https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-sonnet-5-5.html
// - https://platform.claude.com/docs/en/models/sonnet-5-5/whats-new-sonnet-5-5#pricing

// awsClaudeModelPricing contains complete Bedrock claude model defaults.
//
// Cache dimensions follow the usage types Bedrock actually sells (AWS Price List
// API, offer AmazonBedrockFoundationModels, us-east-1 and us-west-2, publication
// 2026-11-09T22:43:12Z, fetched 2026-11-18). Claude Instant, Claude 2.0/2.1,
// Claude 3 Haiku/Sonnet/Opus and Claude 3.5 Sonnet (June 2024) have no cache
// usage type, so they carry no cache ratios: a zero ratio inherits the input
// rate, the only SKU Bedrock could meter such tokens under, and never undercuts
// it with an invented discount. Claude 3.5 Haiku, 3.5 Sonnet v2, 3.7 Sonnet,
// Sonnet 4 and Opus 4/4.1 sell 5-minute writes only (Bedrock documents that a
// ttl field on these models may raise a ValidationException), so a reported
// 1-hour write bills at their only cache-write SKU: below it would undercut
// Bedrock's write charge, above it would invent Anthropic's first-party 1h rate.
// Lifecycle dates come from
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-lifecycle-legacy.html
// (fetched 2026-11-18); end-of-life IDs stay configurable for other Regions.
var awsClaudeModelPricing = map[string]adaptor.ModelConfig{
	"claude-instant-1.2": {
		Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 3.0,
		ContextLength: 100000, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeLegacyFeatures, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Anthropic Claude Instant 1.2 on AWS Bedrock (legacy fast text model). No Bedrock prompt-cache SKU, so cache tokens bill as input.",
	},
	"claude-2.0": {
		Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 3.0,
		ContextLength: 100000, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeLegacyFeatures, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Anthropic Claude 2.0 on AWS Bedrock (legacy text model). No Bedrock prompt-cache SKU, so cache tokens bill as input.",
	},
	"claude-2.1": {
		Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 3.0,
		ContextLength: 200000, MaxOutputTokens: 4096,
		InputModalities: awsTextInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeLegacyFeatures, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Anthropic Claude 2.1 on AWS Bedrock with 200k context. No Bedrock prompt-cache SKU, so cache tokens bill as input.",
	},
	"claude-3-haiku-20240307": {
		Ratio: 0.25 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		ContextLength: 200000, MaxOutputTokens: 4096,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3 Haiku on AWS Bedrock (compact multimodal model). Bedrock end-of-life: 2026-09-10 in us-east-1, us-east-2, us-west-2, ap-northeast-1, ap-southeast-2, eu-central-1, eu-west-1, eu-west-2 and eu-west-3, and 2026-11-13 in ap-northeast-2, ap-south-1 and ap-southeast-1; requests there fail. Kept only for other Regions or private access. No Bedrock prompt-cache SKU, so cache tokens bill as input.",
	},
	"claude-3-5-haiku-20241022": {
		Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.08 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite1hRatio: 1.0 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3.5 Haiku on AWS Bedrock (fast multimodal model). Bedrock sells 5-minute cache writes only; a 1-hour write bills at the 5-minute write SKU.",
	},
	// claude-3-5-haiku-latest invokes the same Bedrock model as the dated ID
	// (AwsModelIDMap: anthropic.claude-3-5-haiku-20241022-v1:0), so it carries
	// Bedrock's own on-demand tariff instead of falling through to the global
	// (Anthropic first-party) price. Bedrock Price List API, offer
	// AmazonBedrockFoundationModels us-east-1 (publication 2026-11-09, fetched
	// 2026-11-18): input $0.80, output $4.00, cache write $1.00, cache read $0.08
	// per 1M tokens; the model is absent from the Bedrock Legacy/EOL tables
	// (docs.aws.amazon.com/bedrock/latest/userguide/model-lifecycle-legacy.html).
	// Bedrock publishes no 1h cache-write SKU for this model; like the dated
	// entry, a 1h write bills at the only cache-write SKU (see the note above
	// awsClaudeModelPricing).
	"claude-3-5-haiku-latest": {
		Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.08 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite1hRatio: 1.0 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3.5 Haiku (latest alias of claude-3-5-haiku-20241022) on AWS Bedrock; Bedrock on-demand tariff checked 2026-11-18. Bedrock sells 5-minute cache writes only; a 1-hour write bills at the 5-minute write SKU.",
	},
	"claude-haiku-4-5": {
		Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.1 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 2 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 60000,
		Description:        "Claude Haiku 4.5 (alias) on AWS Bedrock with extended thinking.",
	},
	"claude-haiku-4-5-20251001": {
		Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.1 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 2 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 60000,
		Description:        "Claude Haiku 4.5 (October 2025 release) on AWS Bedrock with extended thinking.",
	},
	"claude-3-sonnet-20240229": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		ContextLength: 200000, MaxOutputTokens: 4096,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3 Sonnet on AWS Bedrock (balanced multimodal model). Bedrock end-of-life: 2026-11-13 in its remaining listed Regions (ap-south-1, ap-southeast-1, eu-west-2); requests there fail. Kept only for other Regions or private access. No Bedrock prompt-cache SKU, so cache tokens bill as input.",
	},
	"claude-3-5-sonnet-latest": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 3.75 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3.5 Sonnet (latest alias) on AWS Bedrock. Bedrock end-of-life: 2026-11-13 in its remaining listed Regions (ap-northeast-2, ap-south-1, ap-southeast-1); requests there fail. Kept only for other Regions or private access. Bedrock sells 5-minute cache writes only; a 1-hour write bills at the 5-minute write SKU.",
	},
	"claude-3-5-sonnet-20240620": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3.5 Sonnet (June 2024 release) on AWS Bedrock. Bedrock end-of-life: 2026-11-13 in its remaining listed Regions (ap-northeast-2, ap-south-1, ap-southeast-1); requests there fail. Kept only for other Regions or private access. No Bedrock prompt-cache SKU, so cache tokens bill as input.",
	},
	"claude-3-5-sonnet-20241022": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 3.75 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3.5 Sonnet v2 (October 2024 release) on AWS Bedrock. Bedrock end-of-life: 2026-11-13 in its remaining listed Regions (ap-northeast-2, ap-south-1, ap-southeast-1); requests there fail. Kept only for other Regions or private access. Bedrock sells 5-minute cache writes only; a 1-hour write bills at the 5-minute write SKU.",
	},
	"claude-3-7-sonnet-latest": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 3.75 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 60000,
		Description:        "Claude 3.7 Sonnet (latest alias) on AWS Bedrock with extended thinking. Bedrock end-of-life: 2026-11-13 in its remaining listed Regions (ap-south-1, eu-west-2); requests there fail. Kept only for other Regions or private access. Bedrock sells 5-minute cache writes only; a 1-hour write bills at the 5-minute write SKU.",
	},
	"claude-3-7-sonnet-20250219": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 3.75 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 60000,
		Description:        "Claude 3.7 Sonnet (February 2025 release) on AWS Bedrock with extended thinking. Bedrock end-of-life: 2026-11-13 in its remaining listed Regions (ap-south-1, eu-west-2); requests there fail. Kept only for other Regions or private access. Bedrock sells 5-minute cache writes only; a 1-hour write bills at the 5-minute write SKU.",
	},
	"claude-sonnet-4-0": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 3.75 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 60000,
		Description:        "Claude Sonnet 4 (alias) on AWS Bedrock. Bedrock end-of-life: 2026-10-14 in us-east-1, us-east-2, us-west-1, us-west-2, ap-east-2, ap-northeast-1/2/3, ap-south-1/2, ap-southeast-1/2/3/4/5/7, eu-central-1, eu-north-1, eu-south-1/2, eu-west-1/3 and il-central-1; requests there fail. Kept only for other Regions or private access. Bedrock sells 5-minute cache writes only; a 1-hour write bills at the 5-minute write SKU.",
	},
	"claude-sonnet-4-20250514": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 3.75 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 60000,
		Description:        "Claude Sonnet 4 (May 2025 release) on AWS Bedrock. Bedrock end-of-life: 2026-10-14 in us-east-1, us-east-2, us-west-1, us-west-2, ap-east-2, ap-northeast-1/2/3, ap-south-1/2, ap-southeast-1/2/3/4/5/7, eu-central-1, eu-north-1, eu-south-1/2, eu-west-1/3 and il-central-1; requests there fail. Kept only for other Regions or private access. Bedrock sells 5-minute cache writes only; a 1-hour write bills at the 5-minute write SKU.",
	},
	"claude-sonnet-4-5": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 60000,
		Description:        "Claude Sonnet 4.5 (alias) on AWS Bedrock. Bedrock Legacy since 2026-10-08 with end-of-life scheduled 2027-04-08 in every listed Region; migrate before then.",
	},
	"claude-sonnet-4-5-20250929": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 60000,
		Description:        "Claude Sonnet 4.5 (September 2025 release) on AWS Bedrock. Bedrock Legacy since 2026-10-08 with end-of-life scheduled 2027-04-08 in every listed Region; migrate before then.",
	},
	"claude-sonnet-4-6": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 120000,
		Description:        "Claude Sonnet 4.6 on AWS Bedrock with 1M-token context and extended thinking.",
	},
	"claude-sonnet-5": {
		Ratio: 2 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.2 * ratio.MilliTokensUsd, CacheWrite5mRatio: 2.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 4 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeAdaptiveOnlySamplingParams,
		Description: "Claude Sonnet 5 on AWS Bedrock with 1M-token context, adaptive thinking, and permanent $2/$10 per million token pricing.",
	},
	"claude-3-opus-20240229": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		ContextLength: 200000, MaxOutputTokens: 4096,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesNoReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude 3 Opus on AWS Bedrock (premium multimodal model). No Bedrock prompt-cache SKU, so cache tokens bill as input.",
	},
	"claude-opus-4-0": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 18.75 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 32000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 30000,
		Description:        "Claude Opus 4 (alias) on AWS Bedrock. Bedrock sells 5-minute cache writes only; a 1-hour write bills at the 5-minute write SKU.",
	},
	"claude-opus-4-20250514": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 18.75 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 32000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 30000,
		Description:        "Claude Opus 4 (May 2025 release) on AWS Bedrock. Bedrock sells 5-minute cache writes only; a 1-hour write bills at the 5-minute write SKU.",
	},
	"claude-opus-4-1": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 18.75 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 32000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 30000,
		Description:        "Claude Opus 4.1 (alias) on AWS Bedrock. AWS Bedrock model entered Legacy state on 2026-07-08 with EOL scheduled 2027-01-08. Bedrock sells 5-minute cache writes only; a 1-hour write bills at the 5-minute write SKU.",
	},
	"claude-opus-4-1-20250805": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 75.0 / 15,
		CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 18.75 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 32000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 30000,
		Description:        "Claude Opus 4.1 (August 2025 release) on AWS Bedrock with extended thinking. AWS Bedrock model entered Legacy state on 2026-07-08 with EOL scheduled 2027-01-08. Bedrock sells 5-minute cache writes only; a 1-hour write bills at the 5-minute write SKU.",
	},
	"claude-opus-4-5": {
		Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5,
		CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 60000,
		Description:        "Claude Opus 4.5 (alias) on AWS Bedrock.",
	},
	"claude-opus-4-5-20251101": {
		Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5,
		CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 60000,
		Description:        "Claude Opus 4.5 (November 2025 release) on AWS Bedrock.",
	},
	"claude-opus-4-6": {
		Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5,
		CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		MaxReasoningTokens: 120000,
		Description:        "Claude Opus 4.6 on AWS Bedrock with 1M-token context and extended thinking.",
	},
	"claude-opus-4-7": {
		Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5,
		CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Opus 4.7 on AWS Bedrock with 1M-token context and adaptive thinking.",
	},
	"claude-opus-4-8": {
		Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5,
		CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Opus 4.8 on AWS Bedrock with 1M-token context and adaptive thinking; flagship Anthropic model.",
	},
	"claude-opus-5": {
		Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5,
		CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Opus 5 on AWS Bedrock with 1M-token context and adaptive thinking; most capable Anthropic model.",
	},
	"claude-fable-5": {
		Ratio: 10 * ratio.MilliTokensUsd, CompletionRatio: 5,
		CachedInputRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite5mRatio: 12.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 20 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeSamplingParams,
		Description: "Claude Fable 5 on AWS Bedrock with 1M-token context and frontier-level reasoning (adaptive thinking; budget_tokens not supported).",
	},
	"claude-fable-5-1": {
		Ratio: 10 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.25 * ratio.MilliTokensUsd, CacheWrite5mRatio: 12.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 20 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeAdaptiveOnlySamplingParams,
		Description: "Claude Fable 5.1 on AWS Bedrock with 1M-token context and adaptive thinking; prompt-cache reads cost $0.25 per million tokens.",
	},
	"claude-mythos-5-1": {
		Ratio: 10 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.25 * ratio.MilliTokensUsd, CacheWrite5mRatio: 12.5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 20 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures: awsClaudeFeaturesWithReasoning, SupportedSamplingParameters: awsClaudeAdaptiveOnlySamplingParams,
		Description: "Claude Mythos 5.1 invite-only model on AWS Bedrock with 1M-token context and adaptive thinking; prompt-cache reads cost $0.25 per million tokens.",
	},
	"claude-fable-5-2": {
		Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 5,
		CachedInputRatio:  .2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 10 * ratio.MilliTokensUsd, CacheWrite1hRatio: 16 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedSamplingParameters: awsClaudeAdaptiveOnlySamplingParams,
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:      "high",
		Description:                 "Claude Fable 5.2 on Bedrock: 1M context, 128K output, always-on adaptive thinking; standard $8/$40 input/output, $0.20 cache reads and $10/$16 cache writes per million tokens. Requires cross-region profile and AWS data-retention opt-in.",
	},
	"claude-mythos-5-2": {
		Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 5,
		CachedInputRatio:  .2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 10 * ratio.MilliTokensUsd, CacheWrite1hRatio: 16 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: awsClaudeVisionInputs, OutputModalities: awsTextOutputs,
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedSamplingParameters: awsClaudeAdaptiveOnlySamplingParams,
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:      "high",
		Description:                 "Claude Mythos 5.2 on Bedrock: restricted access, 1M context, 128K output, always-on adaptive thinking; standard $8/$40 input/output, $0.20 cache reads and $10/$16 cache writes per million tokens. Requires a supported inference profile.",
	},
	"claude-opus-5-5": {
		Ratio: 4 * ratio.MilliTokensUsd, CompletionRatio: 5,
		CachedInputRatio:  .2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 5 * ratio.MilliTokensUsd, CacheWrite1hRatio: 8 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: []string{"text", "image", "file"}, OutputModalities: []string{"text"},
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedSamplingParameters: []string{"max_tokens", "stop"},
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:      "medium",
		Description:                 "Claude Opus 5.5 on Amazon Bedrock; always-on adaptive thinking, 1M context and 128K output. Standard global inference $4/$20 input/output per million tokens, $0.20 cache reads, $5/$8 5-minute/1-hour cache writes. Regional or contracted prices require operator overrides. Native structured outputs are not supported on this Bedrock model card.",
	},
	"claude-sonnet-5-5": {
		Ratio:                       2 * ratio.MilliTokensUsd,
		CompletionRatio:             5,
		CachedInputRatio:            0.1 * ratio.MilliTokensUsd,
		CacheWrite5mRatio:           2.5 * ratio.MilliTokensUsd,
		CacheWrite1hRatio:           4 * ratio.MilliTokensUsd,
		ContextLength:               1000000,
		MaxOutputTokens:             128000,
		InputModalities:             []string{"text", "image", "file"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "reasoning"},
		SupportedSamplingParameters: []string{"stop", "max_tokens"},
		SupportedReasoningEfforts:   []string{"low", "medium", "high", "xhigh", "max"},
		DefaultReasoningEffort:      "high",
		Description:                 "Claude Sonnet 5.5 on Amazon Bedrock: 1M context, 128K output, adaptive thinking (default high). Commercial-region Invoke calls use a global inference profile. Native structured outputs and Batch are not supported by the launch model card. Default input/output prices are $2/$10 per million tokens, cache read $0.10 and 5m/1h writes $2.50/$4, with the AWS-published October 7 cache-read reduction; other rates retain the owner-approved Claude price-parity policy. Channel price overrides remain supported.",
	},
}

// awsFable52ModelPricing projects the existing public effort-validation subset
// from the complete Claude catalog; it does not define another tariff.
var awsFable52ModelPricing = map[string]adaptor.ModelConfig{
	"claude-fable-5-2":  awsClaudeModelPricing["claude-fable-5-2"],
	"claude-mythos-5-2": awsClaudeModelPricing["claude-mythos-5-2"],
}
