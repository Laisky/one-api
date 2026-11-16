package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// gpt35ModelRatios captures pricing and metadata for GPT-3.5 chat-completion models.
// Sources: https://platform.openai.com/docs/models/gpt-3-5-turbo and
// https://platform.openai.com/docs/pricing.
var gpt35ModelRatios = map[string]adaptor.ModelConfig{
	"gpt-3.5-turbo": {
		Ratio:                       0.5 * ratio.MilliTokensUsd,
		CompletionRatio:             1.5 / 0.5,
		ContextLength:               16385,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-3.5 Turbo: cost-efficient legacy chat model with 16K context.",
	},
	"gpt-3.5-turbo-0301": {
		Ratio:                       1.5 * ratio.MilliTokensUsd,
		CompletionRatio:             2.0 / 1.5,
		ContextLength:               4096,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-3.5 Turbo snapshot from 2023-03-01.",
	},
	"gpt-3.5-turbo-0613": {
		Ratio:                       1.5 * ratio.MilliTokensUsd,
		CompletionRatio:             2.0 / 1.5,
		ContextLength:               4096,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-3.5 Turbo snapshot from 2023-06-13 with 4K context.",
	},
	"gpt-3.5-turbo-1106": {
		Ratio:                       1.0 * ratio.MilliTokensUsd,
		CompletionRatio:             2.0 / 1.0,
		ContextLength:               16385,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-3.5 Turbo snapshot from 2023-11-06 with 16K context and JSON mode.",
	},
	"gpt-3.5-turbo-0125": {
		Ratio:                       0.5 * ratio.MilliTokensUsd,
		CompletionRatio:             1.5 / 0.5,
		ContextLength:               16385,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-3.5 Turbo snapshot from 2024-01-25 with 16K context.",
	},
	"gpt-3.5-turbo-16k": {
		Ratio:                       3.0 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0 / 3.0,
		ContextLength:               16385,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-3.5 Turbo with 16K context window (legacy alias).",
	},
	"gpt-3.5-turbo-16k-0613": {
		Ratio:                       3.0 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0 / 3.0,
		ContextLength:               16385,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-3.5 Turbo 16K snapshot from 2023-06-13.",
	},
	"gpt-3.5-turbo-instruct": {
		Ratio:                       1.5 * ratio.MilliTokensUsd,
		CompletionRatio:             2.0 / 1.5,
		ContextLength:               4096,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-3.5 Turbo Instruct: legacy completions endpoint successor to text-davinci-003.",
	},
}
