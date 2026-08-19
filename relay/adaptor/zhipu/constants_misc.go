package zhipu

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// imageGenerationModels enumerates Zhipu's image and video generation models.
// Legacy image entries approximate per-request costs in token units; the Vidu
// entries below encode the published per-call price directly as quota
// (Ratio = price in RMB * ratio.QuotaPerRMB) and also surface it through
// PerCall.UsdPerThousandCalls for the display layer.
var imageGenerationModels = map[string]adaptor.ModelConfig{
	"cogview-4": {
		Ratio:            0.06 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		InputModalities:  textInput(),
		OutputModalities: []string{"image"},
		Description:      "CogView-4: high-quality text-to-image generation model with multi-resolution support.",
	},
	"glm-image": {
		Ratio:            0.1 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		InputModalities:  textInput(),
		OutputModalities: []string{"image"},
		Description:      "GLM-Image: flagship text-to-image generation model (max 1000-char prompt), multi-resolution 512px-2048px in multiples of 32.",
	},
	"cogview-3-plus": {
		Ratio:            0.08 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		InputModalities:  textInput(),
		OutputModalities: []string{"image"},
		Description:      "CogView-3-Plus: enhanced CogView-3 image generator. (retired/superseded; no longer listed among current image-generation models, which now comprise GLM-Image, CogView-4, and CogView-3-Flash)",
	},
	"cogview-3": {
		Ratio:            0.04 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		InputModalities:  textInput(),
		OutputModalities: []string{"image"},
		Description:      "CogView-3: text-to-image generation model. (retired/superseded; no longer listed among current image-generation models, which now comprise GLM-Image, CogView-4, and CogView-3-Flash)",
	},
	"cogview-3-flash": {
		Ratio:            0,
		CompletionRatio:  1,
		CachedInputRatio: 0,
		InputModalities:  textInput(),
		OutputModalities: []string{"image"},
		Description:      "CogView-3-Flash: free fast text-to-image generation model.",
	},
	"cogvideox-3": {
		Ratio:            1 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		Description:      "CogVideoX-3: flagship text/image-to-video generation model supporting first/last-frame control, up to 4K resolution (supersedes CogVideoX/CogVideoX-2).",
	},
	"cogviewx": {
		Ratio:            0.04 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		Description:      "CogVideoX: text-and-image-to-video generation model. (legacy id; no live Zhipu model currently uses this id -- current video-generation ids are cogvideox-3 and cogvideox-flash)",
	},
	"cogviewx-flash": {
		Ratio:            0,
		CompletionRatio:  1,
		CachedInputRatio: 0,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		Description:      "CogVideoX-Flash: free fast text-to-video generator with 4K and 60fps support.",
	},
	// Vidu Q1: high-quality 5s / 1080P video generation, ¥2.5 per call.
	// Source: https://docs.bigmodel.cn/cn/guide/models/video-generation/viduq1
	"viduq1-image": {
		Ratio:            2.5 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 2.5 / 7 * 1000},
		Description:      "Vidu Q1 (image-to-video): fixed 5s 1080P clips at ¥2.5/call; high-fidelity video generation for cinematic scenes.",
	},
	"viduq1-start-end": {
		Ratio:            2.5 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 2.5 / 7 * 1000},
		Description:      "Vidu Q1 (first/last frame): generates 5s 1080P video from start and end frames at ¥2.5/call.",
	},
	"viduq1-text": {
		Ratio:            2.5 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 2.5 / 7 * 1000},
		Description:      "Vidu Q1 (text-to-video): fixed 5s 1080P clips from text prompts at ¥2.5/call.",
	},
	// Vidu 2: fast 4s / 720P video generation, ¥1.25 per call (reference mode ¥2.5).
	// Source: https://docs.bigmodel.cn/cn/guide/models/video-generation/vidu2
	"vidu2-image": {
		Ratio:            1.25 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 1.25 / 7 * 1000},
		Description:      "Vidu 2 (image-to-video): fast 4s 720P clips at ¥1.25/call; stable and color-accurate for e-commerce scenes.",
	},
	"vidu2-start-end": {
		Ratio:            1.25 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 1.25 / 7 * 1000},
		Description:      "Vidu 2 (first/last frame): fast 4s 720P video from start and end frames at ¥1.25/call.",
	},
	"vidu2-reference": {
		Ratio:            2.5 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 2.5 / 7 * 1000},
		Description:      "Vidu 2 (reference-to-video): 4s 720P clips from reference images of people or objects at ¥2.5/call.",
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
		ContextLength:    8192,
		MaxOutputTokens:  4096,
		InputModalities:  []string{"image", "file"},
		OutputModalities: textOutput(),
		HuggingFaceID:    "zai-org/GLM-OCR",
		Description:      "GLM-OCR: layout-aware OCR for images and PDFs (single image <=10MB, PDF <=50MB, 100 pages).",
	},
}
