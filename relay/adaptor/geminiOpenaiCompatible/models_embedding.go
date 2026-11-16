package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// embeddingModelRatios contains complete embedding catalog entries.
var embeddingModelRatios = map[string]adaptor.ModelConfig{
	"gemini-embedding-001": {
		Ratio:            geminiEmbedding001TextPrice * ratio.MilliTokensUsd,
		CompletionRatio:  1,
		ContextLength:    2048,
		InputModalities:  append([]string(nil), geminiInputTextOnly...),
		OutputModalities: append([]string(nil), geminiOutputText...),
		Description:      "Gemini Embedding text model; scheduled shutdown May 14, 2028. Use gemini-embedding-2 for new workloads.",
	},
	"gemini-embedding-2-preview": {
		Ratio:            geminiEmbedding2PreviewTextPrice * ratio.MilliTokensUsd,
		CompletionRatio:  1,
		Embedding:        geminiEmbedding2PreviewConfig(),
		ContextLength:    8192,
		InputModalities:  append([]string(nil), geminiInputMultimodal...),
		OutputModalities: append([]string(nil), geminiOutputText...),
		Description:      "Gemini Embedding 2 preview is deprecated. Since November 6, 2026, Gemini Developer API requests route to gemini-embedding-2 with the same schema, embedding space and pricing. Existing vectors do not require reindexing; use the stable ID for new workloads.",
	},
	"gemini-embedding-2": {
		Ratio:            geminiEmbedding2PreviewTextPrice * ratio.MilliTokensUsd,
		CompletionRatio:  1,
		Embedding:        geminiEmbedding2PreviewConfig(),
		ContextLength:    8192,
		InputModalities:  append([]string(nil), geminiInputMultimodal...),
		OutputModalities: append([]string(nil), geminiOutputText...),
		Description:      "Gemini Embedding 2 multimodal GA model accepting text, image, audio, and video inputs.",
	},
	"aqa": {
		Ratio:                       1,
		CompletionRatio:             1,
		ContextLength:               7168,
		MaxOutputTokens:             1024,
		InputModalities:             append([]string(nil), geminiInputTextOnly...),
		OutputModalities:            append([]string(nil), geminiOutputText...),
		SupportedSamplingParameters: append([]string(nil), []string{"temperature", "top_p", "top_k", "max_tokens"}...),
		Description:                 "Attributed Question Answering (AQA) grounding model.",
	},
}
