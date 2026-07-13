package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// mistralModels covers Mistral, Mixtral, and Databricks DBRX served by Fireworks.
// All entries use the tiered $0.20/$0.50/$1.20 per 1M-token pricing buckets.
var mistralModels = map[string]adaptor.ModelConfig{
	// Fireworks' own model card lists the API resource ID as
	// "mistral-7b-instruct-v3" (no literal dot) even though the underlying
	// weights are Mistral-7B-Instruct-v0.3; corrected 2026-07-13 per
	// https://fireworks.ai/models/fireworks/mistral-7b-instruct-v3.
	"accounts/fireworks/models/mistral-7b-instruct-v3": {
		Ratio:                       0.20 * ratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               32768,
		MaxOutputTokens:             32768,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwChatFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "mistralai/Mistral-7B-Instruct-v0.3",
		Description:                 "Mistral 7B Instruct v0.3 dense baseline with 32K context, function-calling tokenizer, and instruction tuning. Retired from Fireworks serverless (confirmed via model card, 2026-07-13); on-demand/dedicated only.",
	},
	"accounts/fireworks/models/mixtral-8x7b-instruct": {
		Ratio:                       0.50 * ratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               32768,
		MaxOutputTokens:             32768,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwChatFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "mistralai/Mixtral-8x7B-Instruct-v0.1",
		Description:                 "Mistral Mixtral 8x7B Instruct sparse MoE (46.7B total / ~12.9B active) with 32K context and strong multilingual chat. Retired from Fireworks serverless (confirmed via model card, 2026-07-13); on-demand/dedicated only.",
	},
	"accounts/fireworks/models/mixtral-8x22b-instruct": {
		Ratio:                       1.20 * ratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               65536,
		MaxOutputTokens:             65536,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwChatFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "mistralai/Mixtral-8x22B-Instruct-v0.1",
		Description:                 "Mistral Mixtral 8x22B Instruct sparse MoE (141B total / ~39B active) with 64K context for advanced reasoning and coding. Retired from Fireworks serverless (confirmed via model card, 2026-07-13); on-demand/dedicated only.",
	},
	"accounts/fireworks/models/dbrx-instruct": {
		Ratio:                       1.20 * ratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               32768,
		MaxOutputTokens:             32768,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwChatFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "databricks/dbrx-instruct",
		Description:                 "Databricks DBRX Instruct sparse MoE (132B total / 36B active) chat model with 32K context. Retired from Fireworks serverless (confirmed via model card, 2026-07-13); on-demand/dedicated only.",
	},
}
