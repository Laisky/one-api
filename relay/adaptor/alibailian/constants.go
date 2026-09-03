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
		CachedInputRatio:            0.2 * (0.00031 * 1000 * ratio.MilliTokensRmb),
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
		Ratio:                       0.00015 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.00015 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             10, // 1.5 / 0.15
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
		Ratio:                       0.0008 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.0008 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             2.5, // 2.0 / 0.8
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
		CachedInputRatio:            0.2 * (0.0005 * 1000 * ratio.MilliTokensRmb),
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
		Ratio:                       0.0024 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.0024 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             4, // 9.6 / 2.4
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen Max on Bailian: most capable Qwen flagship chat tier (32K context, 2.4/9.6 CNY/1M).",
	},

	// ----- Qwen3 / Qwen3.5 / Qwen3.6 closed (Bailian) --------------------------
	// Aliyun pricing (verified 2026-05-19, Beijing CNY/1M):
	//   qwen3-max:           0-32K 2.5/10, tiered to 128K-256K 7/28
	//   qwen3.5-plus:        0-128K 0.8/4.8, tiered to 256K-1M 4/24
	//   qwen3.5-flash:       0-128K 0.2/2, tiered to 256K-1M 1.2/12
	//   qwen3.6-plus:        0-256K 2/12, 256K-1M 8/48 (released 2026-04-02)
	//   qwen3.6-flash:       0-256K 1.2/7.2, 256K-1M 4.8/28.8 (released 2026-04-16)
	//   qwen3.6-max-preview: 0-128K 9/54, 128K-256K 15/90 (released 2026-04-20)
	"qwen3-max": {
		Ratio:                       0.0025 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.0025 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             4, // 10 / 2.5
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen3 Max on Bailian: closed-weight flagship (256K context, 0-32K base tier billed here).",
	},
	"qwen3.5-plus": {
		Ratio:                       0.0008 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.0008 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             6, // 4.8 / 0.8
		ContextLength:               1000000,
		MaxOutputTokens:             32768,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen3.5 Plus on Bailian: balanced 1M-context tier (0-128K base tier billed here).",
	},
	"qwen3.5-flash": {
		Ratio:                       0.0002 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.0002 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             10, // 2 / 0.2
		ContextLength:               1000000,
		MaxOutputTokens:             32768,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen3.5 Flash on Bailian: cost-optimized 1M-context tier (0-128K base tier billed here).",
	},
	"qwen3.6-plus": {
		Ratio:                       0.002 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.002 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             6, // 12 / 2
		ContextLength:               1000000,
		MaxOutputTokens:             65536,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen3.6 Plus on Bailian: closed-weight flagship released 2026-04-02 (1M context, 0-256K base tier billed here).",
	},
	"qwen3.6-flash": {
		Ratio:                       0.0012 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.0012 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             6, // 7.2 / 1.2
		ContextLength:               1000000,
		MaxOutputTokens:             65536,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen3.6 Flash on Bailian: cost-optimized tier released 2026-04-16 (1M context, 0-256K base tier billed here).",
	},
	"qwen3.6-max-preview": {
		Ratio:                       0.009 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.009 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             6, // 54 / 9
		ContextLength:               262144,
		MaxOutputTokens:             65536,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianReasoningFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		MaxReasoningTokens:          65536,
		Quantization:                "bf16",
		Description:                 "Qwen3.6 Max Preview on Bailian: closed-weight reasoning flagship released 2026-04-20 (256K context, 0-128K base tier billed here).",
	},

	// ----- Qwen3.7 (Bailian) ---------------------------------------------------
	// Aliyun pricing (Beijing CNY/1M): qwen3.7-max 12/36 (cache 1.2, text-only
	// reasoning); qwen3.7-plus 2/8 (cache 0.4, vision+GUI). Values mirror
	// relay/adaptor/ali/constants_qwen_closed.go.
	"qwen3.7-max": {
		Ratio:                       12 * ratio.MilliTokensRmb,
		CachedInputRatio:            1.2 * ratio.MilliTokensRmb,
		CompletionRatio:             36.0 / 12.0,
		ContextLength:               1000000,
		MaxOutputTokens:             65536,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianReasoningFeatures,
		SupportedSamplingParameters: bailianReasoningSamplingParameters,
		MaxReasoningTokens:          65536,
		Description:                 "Qwen3.7 Max on Bailian: closed-weight reasoning flagship (2026-05-20) with 1M context and native extended-thinking; text-only.",
	},
	"qwen3.7-plus": {
		Ratio:                       2 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * 2 * ratio.MilliTokensRmb,
		CompletionRatio:             4, // 8 / 2 (0-256K base tier)
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Description:                 "Qwen3.7 Plus on Bailian: closed-weight multimodal vision+GUI agent (GA 2026-06-01) with 1M context; supports image input and computer-use tasks.",
	},

	// ----- Qwen Coder (Bailian) ------------------------------------------------
	"qwen-coder-plus": {
		Ratio:                       0.0036 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.0036 * 1000 * ratio.MilliTokensRmb),
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
		CachedInputRatio:            0.2 * (0.0036 * 1000 * ratio.MilliTokensRmb),
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
		CachedInputRatio:            0.2 * (0.00206 * 1000 * ratio.MilliTokensRmb),
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
		CachedInputRatio:            0.2 * (0.00206 * 1000 * ratio.MilliTokensRmb),
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
	// Aliyun pricing (verified 2026-05-19, Beijing CNY/1M, 0-32K base tier):
	//   qwen3-coder-plus:   4 / 16
	//   qwen3-coder-flash:  1 / 4
	//   qwen3-coder-next:   1 / 4  (256K context)
	"qwen3-coder-plus": {
		Ratio:                       0.004 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.004 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             4, // 16 / 4
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
		Ratio:                       0.001 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.001 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             4, // 4 / 1
		ContextLength:               1000000,
		MaxOutputTokens:             65536,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen3 Coder Flash on Bailian: cost-optimized code tier (0-32K base tier billed here).",
	},
	"qwen3-coder-next": {
		Ratio:                       0.001 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.001 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             4, // 4 / 1
		ContextLength:               262144,
		MaxOutputTokens:             65536,
		InputModalities:             bailianTextInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen3-Coder-Next",
		Quantization:                "bf16",
		Description:                 "Qwen3 Coder Next on Bailian: open-weight cost-optimized coder successor (256K context, 0-32K base tier billed here).",
	},

	// ----- Qwen VL (Bailian) ---------------------------------------------------
	"qwen-vl-max": {
		Ratio:                       0.0016 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.0016 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             2.5, // 4 / 1.6
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
		CachedInputRatio:            0.2 * (0.00082 * 1000 * ratio.MilliTokensRmb),
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
		CachedInputRatio:            0.2 * (0.00515 * 1000 * ratio.MilliTokensRmb),
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
		Ratio:                       0.001 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.001 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             10, // 10 / 1.0
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
		Ratio:                       0.00015 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.00015 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             10, // 1.5 / 0.15
		ContextLength:               262144,
		MaxOutputTokens:             16384,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen3-VL Flash on Bailian: cost-optimized multimodal tier (0-32K base tier billed here).",
	},

	// ----- Qwen3 VL open-weight tiers (Bailian) --------------------------------
	// Aliyun pricing (verified 2026-05-19, Beijing CNY/1M, flat):
	//   235B-A22B Instruct: 2 / 8;   235B-A22B Thinking: 2 / 20
	//    32B Instruct:      2 / 8;    32B Thinking:      2 / 20
	//    30B-A3B Instruct:  0.75 / 3; 30B-A3B Thinking:  0.75 / 7.5
	//     8B Instruct:      0.5  / 2;  8B Thinking:      0.5  / 5
	"qwen3-vl-235b-a22b-instruct": {
		Ratio:                       0.002 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.002 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             4, // 8 / 2
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen3-VL-235B-A22B-Instruct",
		Quantization:                "bf16",
		Description:                 "Qwen3-VL 235B A22B Instruct on Bailian: open-weight multimodal MoE flagship (22B active).",
	},
	"qwen3-vl-235b-a22b-thinking": {
		Ratio:                       0.002 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.002 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             10, // 20 / 2
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianReasoningFeatures,
		SupportedSamplingParameters: bailianReasoningSamplingParameters,
		MaxReasoningTokens:          38912,
		HuggingFaceID:               "Qwen/Qwen3-VL-235B-A22B-Thinking",
		Quantization:                "bf16",
		Description:                 "Qwen3-VL 235B A22B Thinking on Bailian: open-weight multimodal reasoning MoE.",
	},
	"qwen3-vl-32b-instruct": {
		Ratio:                       0.002 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.002 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             4, // 8 / 2
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen3-VL-32B-Instruct",
		Quantization:                "bf16",
		Description:                 "Qwen3-VL 32B Instruct on Bailian: open-weight dense multimodal model.",
	},
	"qwen3-vl-32b-thinking": {
		Ratio:                       0.002 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.002 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             10, // 20 / 2
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianReasoningFeatures,
		SupportedSamplingParameters: bailianReasoningSamplingParameters,
		MaxReasoningTokens:          38912,
		HuggingFaceID:               "Qwen/Qwen3-VL-32B-Thinking",
		Quantization:                "bf16",
		Description:                 "Qwen3-VL 32B Thinking on Bailian: open-weight dense multimodal reasoning model.",
	},
	"qwen3-vl-30b-a3b-instruct": {
		Ratio:                       0.00075 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.00075 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             4, // 3 / 0.75
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen3-VL-30B-A3B-Instruct",
		Quantization:                "bf16",
		Description:                 "Qwen3-VL 30B A3B Instruct on Bailian: open-weight multimodal MoE (3B active).",
	},
	"qwen3-vl-30b-a3b-thinking": {
		Ratio:                       0.00075 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.00075 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             10, // 7.5 / 0.75
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianReasoningFeatures,
		SupportedSamplingParameters: bailianReasoningSamplingParameters,
		MaxReasoningTokens:          38912,
		HuggingFaceID:               "Qwen/Qwen3-VL-30B-A3B-Thinking",
		Quantization:                "bf16",
		Description:                 "Qwen3-VL 30B A3B Thinking on Bailian: open-weight multimodal MoE reasoning model (3B active).",
	},
	"qwen3-vl-8b-instruct": {
		Ratio:                       0.0005 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.0005 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             4, // 2 / 0.5
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		HuggingFaceID:               "Qwen/Qwen3-VL-8B-Instruct",
		Quantization:                "bf16",
		Description:                 "Qwen3-VL 8B Instruct on Bailian: open-weight compact multimodal model.",
	},
	"qwen3-vl-8b-thinking": {
		Ratio:                       0.0005 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.0005 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             10, // 5 / 0.5
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianReasoningFeatures,
		SupportedSamplingParameters: bailianReasoningSamplingParameters,
		MaxReasoningTokens:          38912,
		HuggingFaceID:               "Qwen/Qwen3-VL-8B-Thinking",
		Quantization:                "bf16",
		Description:                 "Qwen3-VL 8B Thinking on Bailian: open-weight compact multimodal reasoning model.",
	},

	// ----- Qwen Omni (Bailian, multimodal) -------------------------------------
	// Text I/O rate billed here; audio billing is per-modality upstream.
	"qwen3-omni-flash": {
		Ratio:                       0.0018 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.0018 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             3.8333, // text output 6.9 / text input 1.8
		ContextLength:               262144,
		MaxOutputTokens:             16384,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen3-Omni Flash on Bailian: closed-weight multimodal tier (text I/O rate billed here).",
	},
	"qwen-omni-turbo": {
		Ratio:                       0.0004 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.0004 * 1000 * ratio.MilliTokensRmb),
		CompletionRatio:             4.0, // text output 1.6 / text input 0.4
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             bailianMultimodalInputs,
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianChatFeatures,
		SupportedSamplingParameters: bailianSamplingParameters,
		Quantization:                "bf16",
		Description:                 "Qwen-Omni Turbo on Bailian: closed-weight multimodal tier (text I/O rate billed here).",
	},

	// ----- Qwen MT (Bailian) ---------------------------------------------------
	"qwen-mt-plus": {
		Ratio:                       0.00186 * 1000 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (0.00186 * 1000 * ratio.MilliTokensRmb),
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
		CachedInputRatio:            0.2 * (0.00072 * 1000 * ratio.MilliTokensRmb),
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
		CachedInputRatio:            0.2 * (0.00206 * 1000 * ratio.MilliTokensRmb),
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
		CachedInputRatio:            0.2 * (0.00165 * 1000 * ratio.MilliTokensRmb),
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
		CachedInputRatio:            0.2 * (0.00824 * 1000 * ratio.MilliTokensRmb),
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
		// Kept in lockstep with ali/constants_deepseek.go: DashScope and Bailian are
		// the same Alibaba platform, so the same SKU must not carry two prices. This
		// entry used to read 1.0 with CompletionRatio 1 against DashScope's 4 / 4,
		// a 4x input and 16x output under-charge.
		Ratio:                       4 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (4 * ratio.MilliTokensRmb),
		CompletionRatio:             4,
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
		// Kept in lockstep with ali/constants_deepseek.go (see deepseek-r1 above).
		// This entry used to read 0.07 with CompletionRatio 1 against DashScope's
		// 2 / 4 — a 28x input and 114x output under-charge on the same SKU.
		Ratio:                       2 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * (2 * ratio.MilliTokensRmb),
		CompletionRatio:             4,
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

	// ----- Kimi (月之暗面直供 / supplied directly by Moonshot) ---------------------
	// Bailian resells Moonshot's own K3 endpoint in the Beijing region at
	// 20元 in / 100元 out per 1M tokens. Cache hits bill at 10% of input here —
	// NOT the 20% that Alibaba's own models use — per
	// https://help.aliyun.com/zh/model-studio/kimi-api-by-moonshot-ai:
	// "kimi/kimi-k3命中缓存的输入Token按输入价格的10%计费". Verified 2026-07-20 against
	// https://help.aliyun.com/zh/model-studio/model-pricing.
	//
	// Thinking is always on (仅思考模型) and, unlike Moonshot direct, Bailian
	// accepts only reasoning_effort="max". Chain-of-thought tokens bill at the
	// output rate.
	"kimi/kimi-k3": {
		Ratio:                       20.0 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.1 * (20.0 * ratio.MilliTokensRmb),
		CompletionRatio:             100.0 / 20.0,
		ContextLength:               1048576,
		MaxOutputTokens:             1048576,
		InputModalities:             []string{"text", "image", "video"},
		OutputModalities:            bailianTextOutputs,
		SupportedFeatures:           bailianReasoningFeatures,
		SupportedSamplingParameters: []string{"max_tokens", "stop"},
		SupportedReasoningEfforts:   []string{"max"},
		DefaultReasoningEffort:      "max",
		Description:                 "Moonshot Kimi K3 on Bailian: 1M-context multimodal flagship with always-on thinking, served directly by Moonshot.",
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
