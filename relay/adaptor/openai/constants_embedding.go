package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// embeddingModelRatios captures pricing and metadata for OpenAI embedding and
// legacy completion-only base models. Embedding models advertise no sampling
// parameters and only "text" output modality.
// Source: https://platform.openai.com/docs/models/text-embedding-3-large.
var embeddingModelRatios = map[string]adaptor.ModelConfig{
	"text-embedding-ada-002": {
		Ratio:            0.1 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    8191,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "text-embedding-ada-002: legacy 1536-dim embedding model.",
	},
	"text-embedding-3-small": {
		Ratio:            0.02 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    8191,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "text-embedding-3-small: cost-efficient 1536-dim embedding (resizable).",
	},
	"text-embedding-3-large": {
		Ratio:            0.13 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    8191,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "text-embedding-3-large: high-quality 3072-dim embedding (resizable).",
	},

	// Legacy GPT-3 base/instruct models retained for billing compatibility.
	"text-curie-001": {
		Ratio:            2.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    2049,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "text-curie-001: legacy GPT-3 instruct model (deprecated).",
	},
	"text-babbage-001": {
		Ratio:            0.5 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    2049,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "text-babbage-001: legacy GPT-3 instruct model (deprecated).",
	},
	"text-ada-001": {
		Ratio:            0.4 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    2049,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "text-ada-001: legacy GPT-3 instruct model (deprecated).",
	},
	"text-davinci-002": {
		Ratio:            20.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    4097,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "text-davinci-002: legacy GPT-3 instruct model (deprecated).",
	},
	"text-davinci-003": {
		Ratio:            20.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    4097,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "text-davinci-003: legacy GPT-3 instruct model (deprecated).",
	},
	"text-davinci-edit-001": {
		Ratio:            20.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    2049,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "text-davinci-edit-001: legacy edit endpoint model (deprecated).",
	},
	"davinci-002": {
		Ratio:            2.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    16384,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "davinci-002: GPT-3 base model usable for fine-tuning.",
	},
	"babbage-002": {
		Ratio:            0.4 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    16384,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "babbage-002: GPT-3 base model usable for fine-tuning.",
	},
}
