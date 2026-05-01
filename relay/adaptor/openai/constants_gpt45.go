package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// gpt45ModelRatios captures pricing and metadata for GPT-4.5 preview models.
// Source: https://platform.openai.com/docs/models/gpt-4.5-preview.
var gpt45ModelRatios = map[string]adaptor.ModelConfig{
	"gpt-4.5-preview": {
		Ratio:                       75.0 * ratio.MilliTokensUsd,
		CompletionRatio:             2.0,
		ContextLength:               128000,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4.5 preview: research preview emphasizing creative writing and broad knowledge.",
	},
	"gpt-4.5-preview-2025-02-27": {
		Ratio:                       75.0 * ratio.MilliTokensUsd,
		CompletionRatio:             2.0,
		ContextLength:               128000,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4.5 preview snapshot from 2025-02-27.",
	},
}
