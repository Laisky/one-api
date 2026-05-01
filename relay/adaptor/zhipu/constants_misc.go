package zhipu

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// imageGenerationModels enumerates Zhipu's image and video generation models.
// Pricing entries approximate per-request costs in token units.
var imageGenerationModels = map[string]adaptor.ModelConfig{
	"cogview-4": {
		Ratio:            0.06 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		InputModalities:  textInput(),
		OutputModalities: []string{"image"},
		Description:      "CogView-4: high-quality text-to-image generation model with multi-resolution support.",
	},
	"cogview-3-plus": {
		Ratio:            0.08 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		InputModalities:  textInput(),
		OutputModalities: []string{"image"},
		Description:      "CogView-3-Plus: enhanced CogView-3 image generator.",
	},
	"cogview-3": {
		Ratio:            0.04 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		InputModalities:  textInput(),
		OutputModalities: []string{"image"},
		Description:      "CogView-3: text-to-image generation model.",
	},
	"cogview-3-flash": {
		Ratio:            0,
		CompletionRatio:  1,
		CachedInputRatio: 0,
		InputModalities:  textInput(),
		OutputModalities: []string{"image"},
		Description:      "CogView-3-Flash: free fast text-to-image generation model.",
	},
	"cogviewx": {
		Ratio:           0.04 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		InputModalities: []string{"text", "image"},
		Description:     "CogVideoX: text-and-image-to-video generation model.",
	},
	"cogviewx-flash": {
		Ratio:           0.008 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		InputModalities: []string{"text", "image"},
		Description:     "CogVideoX-Flash: free fast text-to-video generator with 4K and 60fps support.",
	},
}

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
		InputModalities:  []string{"image", "file"},
		OutputModalities: textOutput(),
		HuggingFaceID:    "zai-org/GLM-OCR",
		Description:      "GLM-OCR: layout-aware OCR for images and PDFs (single image <=10MB, PDF <=50MB, 100 pages).",
	},
}
