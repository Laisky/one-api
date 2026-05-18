package alibailian

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Shared metadata helpers for Alibaba Bailian (Model Studio) hosted Qwen and
// DeepSeek chat models. Reused across ModelRatios entries so the table stays
// compact and consistent.
var (
	// bailianTextInputs lists the input modalities for text-only Bailian chat models.
	bailianTextInputs = []string{"text"}
	// bailianTextOutputs lists the output modalities for Bailian chat completions.
	bailianTextOutputs = []string{"text"}
	// bailianMultimodalInputs adds image/file input for Qwen-VL family entries.
	bailianMultimodalInputs = []string{"text", "image"}

	// bailianChatFeatures advertises the capability set for non-reasoning Qwen
	// chat/coder/translate tiers on Bailian: tool-calling, JSON mode and
	// structured outputs are supported per Model Studio docs.
	bailianChatFeatures = []string{"tools", "json_mode", "structured_outputs"}
	// bailianReasoningFeatures advertises the capability set for Qwen reasoning
	// models (QwQ) and hosted DeepSeek thinking models.
	bailianReasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}

	// bailianSamplingParameters lists the OpenAI-compatible sampling parameters
	// Bailian accepts. Bailian additionally exposes top_k and repetition_penalty
	// alongside the standard OpenAI knobs for Chinese-cloud chat APIs.
	bailianSamplingParameters = []string{
		"temperature",
		"top_p",
		"top_k",
		"frequency_penalty",
		"presence_penalty",
		"repetition_penalty",
		"stop",
		"seed",
		"max_tokens",
	}
	// bailianReasoningSamplingParameters lists the constrained sampling-parameter
	// set supported by Qwen-style reasoning models on Bailian (QwQ etc.), which
	// reject most decoding-tuning knobs and reliably accept only seed + max_tokens.
	bailianReasoningSamplingParameters = []string{"seed", "max_tokens"}
)

// ModelRatios contains all supported models and their pricing/configuration metadata.
// Model list is derived from the keys of this map, eliminating redundancy.
//
// Pricing is encoded via ratio.MilliTokensRmb (project rate: 8 RMB per USD).
// CNY prices from Aliyun's official Bailian model-pricing tables map to
//
//	(CNY per 1k tokens) * 1000 * ratio.MilliTokensRmb
//	= CNY per 1M tokens * ratio.MilliTokensRmb
//
// Bailian and DashScope publish identical CNY pricing for the same model name,
// so the values below should match relay/adaptor/ali/constants_*.go. Where they
// diverge (e.g., qwen-long context tier), it is documented inline.
//
// Sources verified 2026-05-18:
//   - https://www.alibabacloud.com/help/en/model-studio/model-pricing
//   - https://help.aliyun.com/zh/model-studio/model-pricing
//   - https://help.aliyun.com/zh/model-studio/getting-started/models
//   - https://huggingface.co/Qwen
//   - https://huggingface.co/deepseek-ai
var ModelRatios = map[string]adaptor.ModelConfig{
	// ----- Qwen closed tiers (Bailian) -----------------------------------------
	// qwen-turbo: 0.31 / 0.62 CNY per 1M (deprecated; use qwen-flash).
	"qwen-turbo": {
		Ratio:                       0.00031 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2,
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen2.5-Turbo",
		Quantization:                "bf16",
		Description:                 "Qwen Turbo on Bailian: cost-efficient chat tier with up to 1M context (deprecated; use qwen-flash).",
	},
	"qwen-flash": {
		Ratio:                       0.00016 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             9.6875, // 1.55 / 0.16
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen Flash on Bailian: closed-weight cost-optimized successor to qwen-turbo (0-128K base tier billed here).",
	},
	"qwen-plus": {
		Ratio:                       0.00082 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.51, // 2.06 / 0.82
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen Plus on Bailian: balanced flagship chat tier with tiered 1M context (0-128K base tier billed here).",
	},
	"qwen-long": {
		Ratio:                       0.0005 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             4, // 2.0 / 0.5
		ContextLength:               10000000,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen Long on Bailian: longest-context chat tier (up to 10M tokens) for document QA.",
	},
	"qwen-max": {
		Ratio:                       0.00247 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             4, // 9.88 / 2.47
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen Max on Bailian: most capable Qwen flagship chat tier (32K context, 2.47/9.88 CNY/1M).",
	},

	// ----- Qwen3 closed (Bailian) ----------------------------------------------
	"qwen3-max": {
		Ratio:                       0.00257 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             4, // 10.30 / 2.57
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen3 Max on Bailian: closed-weight flagship (256K context, 0-32K base tier billed here).",
	},

	// ----- Qwen Coder (Bailian) ------------------------------------------------
	"qwen-coder-plus": {
		Ratio:                       0.0036 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2, // 7.21 / 3.60
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen2.5-Coder-32B-Instruct",
		Quantization:                "bf16",
		Description:                 "Qwen Coder Plus on Bailian: code-specialized chat tier with 128K context.",
	},
	"qwen-coder-plus-latest": {
		Ratio:                       0.0036 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen2.5-Coder-32B-Instruct",
		Quantization:                "bf16",
		Description:                 "Qwen Coder Plus (latest alias) on Bailian.",
	},
	"qwen-coder-turbo": {
		Ratio:                       0.00206 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.995, // 6.17 / 2.06
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen2.5-Coder-7B-Instruct",
		Quantization:                "bf16",
		Description:                 "Qwen Coder Turbo on Bailian: cost-efficient code chat tier.",
	},
	"qwen-coder-turbo-latest": {
		Ratio:                       0.00206 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.995,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen2.5-Coder-7B-Instruct",
		Quantization:                "bf16",
		Description:                 "Qwen Coder Turbo (latest alias) on Bailian.",
	},

	// ----- Qwen3 Coder (Bailian, tiered) ---------------------------------------
	"qwen3-coder-plus": {
		Ratio:                       0.00411 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             4, // 16.45 / 4.11
		ContextLength:               1000000,
		MaxOutputTokens:             65536,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen3 Coder Plus on Bailian: closed-weight code flagship (0-32K base tier billed here).",
	},
	"qwen3-coder-flash": {
		Ratio:                       0.00103 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             4, // 4.11 / 1.03
		ContextLength:               1000000,
		MaxOutputTokens:             65536,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen3 Coder Flash on Bailian: cost-optimized code tier (0-32K base tier billed here).",
	},

	// ----- Qwen VL (Bailian) ---------------------------------------------------
	"qwen-vl-max": {
		Ratio:                       0.00165 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.49, // 4.11 / 1.65
		ContextLength:               32000,
		MaxOutputTokens:             2000,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen-VL Max on Bailian: closed-weight high-end multimodal tier.",
	},
	"qwen-vl-plus": {
		Ratio:                       0.00082 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.51, // 2.06 / 0.82
		ContextLength:               32000,
		MaxOutputTokens:             2000,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen-VL Plus on Bailian: closed-weight balanced multimodal tier.",
	},
	"qwen-vl-ocr": {
		Ratio:                       0.00515 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               34096,
		MaxOutputTokens:             4096,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen-VL OCR on Bailian: OCR-tuned multimodal tier (5.15 CNY/1M).",
	},
	"qwen3-vl-plus": {
		Ratio:                       0.00103 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             10, // 10.30 / 1.03
		ContextLength:               262144,
		MaxOutputTokens:             16384,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen3-VL Plus on Bailian: multimodal tier with tiered 256K context (0-32K base tier billed here).",
	},
	"qwen3-vl-flash": {
		Ratio:                       0.00016 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             9.6875,
		ContextLength:               262144,
		MaxOutputTokens:             16384,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen3-VL Flash on Bailian: cost-optimized multimodal tier.",
	},

	// ----- Qwen MT (Bailian) ---------------------------------------------------
	"qwen-mt-plus": {
		Ratio:                       0.00186 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.995, // 5.57 / 1.86
		ContextLength:               4096,
		MaxOutputTokens:             2048,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen MT Plus on Bailian: flagship machine-translation model.",
	},
	"qwen-mt-turbo": {
		Ratio:                       0.00072 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.79, // 2.01 / 0.72
		ContextLength:               4096,
		MaxOutputTokens:             2048,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen MT Turbo on Bailian: cost-efficient machine-translation model.",
	},

	// ----- Reasoning models (Bailian) ------------------------------------------
	"qwq-32b-preview": {
		Ratio:                       0.00206 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.995, // 6.17 / 2.06
		ContextLength:               32768,
		MaxOutputTokens:             16384,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianReasoningFeatures,
		SupportedSamplingParameters: bailianReasoningSamplingParameters,
		MaxReasoningTokens:          38912,
		HuggingFaceID:               "Qwen/QwQ-32B-Preview",
		Quantization:                "bf16",
		Description:                 "QwQ-32B-Preview on Bailian: experimental open-weight reasoning model.",
	},
	"qwq-plus": {
		Ratio:                       0.00165 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.49, // 4.11 / 1.65
		ContextLength:               131072,
		MaxOutputTokens:             16384,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianReasoningFeatures,
		SupportedSamplingParameters: bailianReasoningSamplingParameters,
		MaxReasoningTokens:          38912,
		HuggingFaceID:               "Qwen/QwQ-32B",
		Quantization:                "bf16",
		Description:                 "QwQ Plus on Bailian: managed reasoning tier.",
	},
	"qvq-max": {
		Ratio:                       0.00824 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             4, // 32.96 / 8.24
		ContextLength:               131072,
		MaxOutputTokens:             16384,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianReasoningFeatures,
		SupportedSamplingParameters: bailianReasoningSamplingParameters,
		MaxReasoningTokens:          38912,
		HuggingFaceID:               "Qwen/QVQ-72B-Preview",
		Quantization:                "bf16",
		Description:                 "QVQ Max on Bailian: managed multimodal reasoning tier.",
	},

	// ----- DeepSeek (hosted on Bailian) ----------------------------------------
	// Note: parallel deepseek pricing is also maintained in
	// relay/adaptor/ali/constants_deepseek.go for the DashScope channel.
	"deepseek-r1": {
		Ratio:                       1.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianReasoningFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		MaxReasoningTokens:          32768,
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1",
		Quantization:                "fp8",
		Description:                 "DeepSeek R1 hosted on Bailian: open-weight reasoning chat model (thinking mode).",
	},
	"deepseek-v3": {
		Ratio:                       0.07 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "deepseek-ai/DeepSeek-V3",
		Quantization:                "fp8",
		Description:                 "DeepSeek V3 hosted on Bailian: open-weight MoE chat model (non-thinking mode).",
	},

	// ----- Embeddings (Bailian) ------------------------------------------------
	// 0.5 CNY / 1M tokens per Aliyun model-pricing (verified 2026-05-18).
	"text-embedding-v3": {
		Ratio:            0.0005 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		InputModalities:  bailianTextInputs,
		OutputModalities: []string{},
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.0005 * 1000 * ratio.MilliTokensRmb,
		},
		Description: "Bailian text-embedding-v3: 1024-dim embedding endpoint, 8192-token input.",
	},
	"text-embedding-v4": {
		Ratio:            0.0005 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:  1,
		ContextLength:    8192,
		InputModalities:  bailianTextInputs,
		OutputModalities: []string{},
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio: 0.0005 * 1000 * ratio.MilliTokensRmb,
		},
		Description: "Bailian text-embedding-v4 (Qwen3-Embedding): flexible 64-2048 dim output, 8192-token input.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// AlibailianToolingDefaults reflects that Bailian's public docs do not disclose per-tool pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://www.alibabacloud.com/help/en/model-studio/latest/billing (returns 404 for unauthenticated access)
var AlibailianToolingDefaults = adaptor.ChannelToolConfig{}
