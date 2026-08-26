package zhipu

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// flagshipVisionModels enumerates Zhipu's flagship multimodal vision-understanding models
// with tiered pricing. Sources:
//   - https://docs.bigmodel.cn/cn/guide/models/vlm/glm-5v-turbo
//   - https://docs.bigmodel.cn/cn/guide/models/vlm/autoglm-phone
//   - https://docs.bigmodel.cn/cn/guide/models/vlm/glm-4.6v
//   - https://docs.bigmodel.cn/cn/guide/models/free/glm-4.6v-flash
//   - https://docs.bigmodel.cn/cn/guide/models/free/glm-4v-flash
var flagshipVisionModels = map[string]adaptor.ModelConfig{
	// AutoGLM-Phone: vision-language AI phone assistant (free for a limited time).
	"autoglm-phone": {
		Ratio:                       0,
		CompletionRatio:             1,
		CachedInputRatio:            0,
		ContextLength:               20_000,
		MaxOutputTokens:             2_048,
		InputModalities:             textImageInput(),
		OutputModalities:            textOutput(),
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: chatSamplingParameters(),
		Description:                 "AutoGLM-Phone: vision-language AI phone assistant that understands screens and drives Android devices via ADB; 20K context, 2048 max output; free for a limited time.",
	},
	// GLM-5.3-Flash: released 2026-08-25 with a two-week 50% launch promotion
	// (promo ¥0.4/¥0.115/¥1.4). The rates below are the standard list prices —
	// ¥0.8 input / ¥0.23 cached input / ¥2.8 output per 1M tokens — so billing
	// does not silently under-charge once the promotion lapses.
	"glm-5.3-flash": {
		Ratio:                       0.8 * ratio.MilliTokensRmb,
		CompletionRatio:             2.8 / 0.8,
		CachedInputRatio:            0.23 * ratio.MilliTokensRmb,
		ContextLength:               1_000_000,
		MaxOutputTokens:             131_072,
		InputModalities:             textImageVideoFileInput(),
		OutputModalities:            textOutput(),
		SupportedFeatures:           reasoningChatFeatures(),
		SupportedSamplingParameters: chatSamplingParameters(),
		Description:                 "GLM-5.3-Flash: native multimodal Flash sibling of GLM-5.3 with 1M context and 128K max output; list pricing ¥0.8/¥2.8 per 1M input/output, ¥0.23 cached-input (launched 2026-08-25 with a two-week 50% promotion).",
	},
	// GLM-5V-Turbo: input [0,32K) ¥5/¥22, input [32K+) ¥7/¥26 (same as GLM-5-Turbo)
	"glm-5v-turbo": {
		Ratio:            5 * ratio.MilliTokensRmb,
		CompletionRatio:  22.0 / 5.0,
		CachedInputRatio: 1.2 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 7 * ratio.MilliTokensRmb, CompletionRatio: 26.0 / 7.0, CachedInputRatio: 1.8 * ratio.MilliTokensRmb, InputTokenThreshold: 32_000},
		},
		ContextLength:               200_000,
		MaxOutputTokens:             131_072,
		InputModalities:             textImageVideoFileInput(),
		OutputModalities:            textOutput(),
		SupportedFeatures:           reasoningChatFeatures(),
		SupportedSamplingParameters: chatSamplingParameters(),
		HuggingFaceID:               "zai-org/GLM-5V-Turbo",
		Quantization:                "bf16",
		Description:                 "GLM-5V-Turbo: 744B MoE native multimodal agent (released 2026-04-01) for visual programming, GUI automation, and design-to-code; 200K context.",
	},
	// GLM-4.6V: input [0,32K) ¥1/¥3, input [32K,128K) ¥2/¥6
	"glm-4.6v": {
		Ratio:            1 * ratio.MilliTokensRmb,
		CompletionRatio:  3.0 / 1.0,
		CachedInputRatio: 0.2 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 2 * ratio.MilliTokensRmb, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.4 * ratio.MilliTokensRmb, InputTokenThreshold: 32_000},
		},
		ContextLength:               131_072,
		MaxOutputTokens:             32_768,
		InputModalities:             textImageVideoFileInput(),
		OutputModalities:            textOutput(),
		SupportedFeatures:           reasoningChatFeatures(),
		SupportedSamplingParameters: chatSamplingParameters(),
		Quantization:                "bf16",
		Description:                 "GLM-4.6V: 106B/12B-active vision-reasoning MoE with native multimodal tool calling.",
	},
	// GLM-4.6V-FlashX: input [0,32K) ¥0.15/¥1.5, input [32K,128K) ¥0.3/¥3
	"glm-4.6v-flashx": {
		Ratio:            0.15 * ratio.MilliTokensRmb,
		CompletionRatio:  1.5 / 0.15,
		CachedInputRatio: 0.03 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 0.3 * ratio.MilliTokensRmb, CompletionRatio: 3.0 / 0.3, CachedInputRatio: 0.03 * ratio.MilliTokensRmb, InputTokenThreshold: 32_000},
		},
		ContextLength:               131_072,
		MaxOutputTokens:             32_768,
		InputModalities:             textImageVideoFileInput(),
		OutputModalities:            textOutput(),
		SupportedFeatures:           reasoningChatFeatures(),
		SupportedSamplingParameters: chatSamplingParameters(),
		Quantization:                "bf16",
		Description:                 "GLM-4.6V-FlashX: 9B lightweight vision-reasoning sibling of GLM-4.6V.",
	},
	// GLM-4.5V: input [0,32K) ¥2/¥6, input [32K,64K) ¥4/¥12
	"glm-4.5v": {
		Ratio:            2 * ratio.MilliTokensRmb,
		CompletionRatio:  6.0 / 2.0,
		CachedInputRatio: 0.4 * ratio.MilliTokensRmb,
		Tiers: []adaptor.ModelRatioTier{
			{Ratio: 4 * ratio.MilliTokensRmb, CompletionRatio: 12.0 / 4.0, CachedInputRatio: 0.8 * ratio.MilliTokensRmb, InputTokenThreshold: 32_000},
		},
		ContextLength:               65_536,
		MaxOutputTokens:             16_384,
		InputModalities:             textImageInput(),
		OutputModalities:            textOutput(),
		SupportedFeatures:           reasoningChatFeatures(),
		SupportedSamplingParameters: chatSamplingParameters(),
		HuggingFaceID:               "zai-org/GLM-4.5V",
		Quantization:                "bf16",
		Description:                 "GLM-4.5V: open-weight visual reasoning model with 64K context.",
	},
	// GLM-4.6V-Flash (free)
	"glm-4.6v-flash": {
		Ratio:                       0,
		CompletionRatio:             1,
		CachedInputRatio:            0,
		ContextLength:               131_072,
		MaxOutputTokens:             32_768,
		InputModalities:             textImageVideoFileInput(),
		OutputModalities:            textOutput(),
		SupportedFeatures:           reasoningChatFeatures(),
		SupportedSamplingParameters: chatSamplingParameters(),
		Quantization:                "bf16",
		Description:                 "GLM-4.6V-Flash: free vision-reasoning model with 128K context and toggleable thinking.",
	},
	// GLM-4V-Flash (free)
	"glm-4v-flash": {
		Ratio:                       0,
		CompletionRatio:             1,
		CachedInputRatio:            0,
		ContextLength:               16_384,
		MaxOutputTokens:             1_024,
		InputModalities:             textImageInput(),
		OutputModalities:            textOutput(),
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: chatSamplingParameters(),
		Description:                 "GLM-4V-Flash: free legacy image understanding model with multilingual support.",
	},
}

// multimodalModels enumerates older multimodal vision and voice models with flat per-token pricing.
var multimodalModels = map[string]adaptor.ModelConfig{
	"glm-4v-plus-0111": {
		Ratio:                       4 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8_192,
		MaxOutputTokens:             8_192,
		InputModalities:             textImageInput(),
		OutputModalities:            textOutput(),
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: chatSamplingParameters(),
		Description:                 "GLM-4V-Plus-0111: 2025-01-11 vision-understanding snapshot.",
	},
	"glm-4v-plus": {
		Ratio:                       4 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8_192,
		MaxOutputTokens:             1_024,
		InputModalities:             textImageInput(),
		OutputModalities:            textOutput(),
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: chatSamplingParameters(),
		Description:                 "GLM-4V-Plus: legacy vision-understanding model (predecessor of glm-4v-plus-0111).",
	},
	"glm-4v": {
		Ratio:                       50 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8_192,
		MaxOutputTokens:             1_024,
		InputModalities:             textImageInput(),
		OutputModalities:            textOutput(),
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: chatSamplingParameters(),
		Description:                 "GLM-4V: original vision-understanding model with limited 2K context.",
	},
	"glm-4-voice": {
		Ratio:                       80 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8_192,
		InputModalities:             textInput(),
		OutputModalities:            textOutput(),
		SupportedSamplingParameters: chatSamplingParameters(),
		HuggingFaceID:               "THUDM/glm-4-voice-9b",
		Quantization:                "bf16",
		Description:                 "GLM-4-Voice: speech-capable model for real-time voice dialog (text relay billing).",
	},
}
