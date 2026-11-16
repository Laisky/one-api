package zhipu

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// utilityModels enumerates character, code, and rerank utility models.
var utilityModels = map[string]adaptor.ModelConfig{
	"charglm-4": {
		Ratio:                       1 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8_192,
		MaxOutputTokens:             4_096,
		InputModalities:             textInput(),
		OutputModalities:            textOutput(),
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: chatSamplingParameters(),
		Description:                 "CharGLM-4: anthropomorphic role-play and emotional companionship model.",
	},
	"emohaa": {
		Ratio:                       15 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8_192,
		MaxOutputTokens:             4_096,
		InputModalities:             textInput(),
		OutputModalities:            textOutput(),
		SupportedSamplingParameters: chatSamplingParameters(),
		Description:                 "Emohaa: psychological-counseling-tuned model for emotional support.",
	},
	"codegeex-4": {
		Ratio:                       0.1 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131_072,
		MaxOutputTokens:             32_768,
		InputModalities:             textInput(),
		OutputModalities:            textOutput(),
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: chatSamplingParameters(),
		HuggingFaceID:               "THUDM/codegeex4-all-9b",
		Quantization:                "bf16",
		Description:                 "CodeGeeX-4: code-completion-tuned model with open weights.",
	},
	"rerank": {
		Ratio:            0.8 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    4_096,
		InputModalities:  textInput(),
		OutputModalities: textOutput(),
		Description:      "Rerank: reorder candidate documents by relevance for retrieval pipelines.",
	},
}

// embeddingModels enumerates Zhipu's text embedding models.
var embeddingModels = map[string]adaptor.ModelConfig{
	"embedding-3": {
		Ratio:           0.5 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		ContextLength:   8_192,
		InputModalities: textInput(),
		Description:     "Embedding-3: V3 text embedding model with 8K context.",
	},
	"embedding-2": {
		Ratio:           0.5 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		ContextLength:   8_192,
		InputModalities: textInput(),
		Description:     "Embedding-2: V2 text embedding model with 8K context.",
	},
}

// ocrModels enumerates Zhipu's layout-aware OCR/document parsing models.
var ocrModels = map[string]adaptor.ModelConfig{
	"glm-ocr": {
		Ratio:            0.2 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		MaxOutputTokens:  4096,
		InputModalities:  []string{"image", "file"},
		OutputModalities: textOutput(),
		HuggingFaceID:    "zai-org/GLM-OCR",
		Description:      "GLM-OCR: layout-aware OCR for images and PDFs (single image <=10MB, PDF <=50MB, 100 pages).",
	},
}
