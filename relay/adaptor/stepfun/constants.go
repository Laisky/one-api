package stepfun

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// StepFun (阶跃星辰) hosts the Step-1 / Step-2 / Step-3 chat model families.
// Reference docs:
//   - https://platform.stepfun.com/docs (model overview)
//   - https://platform.stepfun.com/docs/zh/guides/pricing/details.md (rates, verified 2026-06-13)
//
// Most Step-* SKUs are closed-weight; HuggingFaceID and Quantization stay empty.
// Exception: step-3.5-flash is open-weight (Apache 2.0, stepfun-ai/Step-3.5-Flash).
// step-1v-* and step-1o-* are vision-capable; step-1x-medium is an image
// generation SKU billed per image (kept out of this token-pricing map).
// step-3 / step-r1-v-mini are reasoning-capable models.

// stepfunTextInputs is the input modality set for text-only Step-* models.
var stepfunTextInputs = []string{"text"}

// stepfunVisionInputs is the modality set for vision-capable Step-1V / 1O / 3 models.
var stepfunVisionInputs = []string{"text", "image"}

// stepfunVideoVisionInputs is the modality set for Step models that natively
// support image and video understanding (e.g. step-3.7-flash).
var stepfunVideoVisionInputs = []string{"text", "image", "video"}

// stepfunTextOutputs is the text-only output modality used by Step models.
var stepfunTextOutputs = []string{"text"}

// stepfunChatFeatures captures tool / JSON capabilities offered by the
// OpenAI-compatible StepFun chat API.
var stepfunChatFeatures = []string{"tools", "json_mode"}

// stepfunReasoningFeatures adds reasoning to the chat feature set for Step-3
// and Step-R1 series.
var stepfunReasoningFeatures = []string{"tools", "json_mode", "reasoning"}

// stepfunSamplingParams lists sampling parameters accepted by StepFun chat
// completion endpoints.
var stepfunSamplingParams = []string{
	"temperature",
	"top_p",
	"max_tokens",
	"stop",
	"frequency_penalty",
	"presence_penalty",
	"seed",
}

// ModelRatios contains all supported models and their pricing ratios.
// Model list is derived from the keys of this map, eliminating redundancy.
// Based on StepFun pricing: https://platform.stepfun.com/docs/zh/guides/pricing/details.md
//
// Prompt cache: StepFun reports cache-hit tokens via a TOP-LEVEL usage.cached_tokens
// field (promoted into prompt_tokens_details.cached_tokens by the shared
// openai_compatible handler). Cache-hit input is billed at 20% of the input price
// and is only supported on a subset of models (step-3.x-flash and
// step-1o-turbo-vision); other models are left without a CachedInputRatio per
// https://platform.stepfun.com/docs/zh/guides/developer/prompt-cache.
var ModelRatios = map[string]adaptor.ModelConfig{
	// Step-2 family (current generation).
	"step-2-mini": {
		Ratio:                       1.0 * ratio.MilliTokensRmb,
		CompletionRatio:             2.0 / 1.0,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             stepfunTextInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-2 Mini cost-optimized chat model.",
	},
	"step-2-16k": {
		Ratio:                       38.0 * ratio.MilliTokensRmb,
		CompletionRatio:             120.0 / 38.0,
		ContextLength:               16384,
		MaxOutputTokens:             8192,
		InputModalities:             stepfunTextInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-2 second-generation closed-weight chat model with 16k context.",
	},

	// Step-3 reasoning family. Context-dependent pricing; we encode the
	// base tier (lowest context) and rely on Tiers for upper context bands.
	"step-3": {
		Ratio:                       1.5 * ratio.MilliTokensRmb,
		CompletionRatio:             4.0 / 1.5,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             stepfunVisionInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunReasoningFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Tiers: []adaptor.ModelRatioTier{
			{
				Ratio:               4.0 * ratio.MilliTokensRmb,
				CompletionRatio:     10.0 / 4.0,
				InputTokenThreshold: 32000,
			},
		},
		Description: "StepFun Step-3 multimodal reasoning model with tiered pricing (¥1.5-¥4 input / ¥4-¥10 output per 1M tokens).",
	},
	"step-3.5-flash": {
		Ratio:            0.7 * ratio.MilliTokensRmb,
		CachedInputRatio: 0.2 * 0.7 * ratio.MilliTokensRmb, // Prompt Cache: cached input billed at 20% of input price.
		CompletionRatio:  2.1 / 0.7,
		// Note: corrected context length 65536→262144 per official docs 2026-06-13
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             stepfunTextInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunReasoningFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		// StepFun docs (platform.stepfun.ai/docs/llm/reasoning) document
		// reasoning_effort low/medium/high on the Step-3.5-Flash family with
		// model-default "medium". No tunable thinking_budget is published.
		SupportedReasoningEfforts: []string{"low", "medium", "high"},
		DefaultReasoningEffort:    "medium",
		HuggingFaceID:             "stepfun-ai/Step-3.5-Flash",
		Description:               "StepFun Step-3.5 Flash low-latency reasoning model (open-weight, Apache 2.0).",
	},
	"step-3.7-flash": {
		Ratio:                       1.35 * ratio.MilliTokensRmb,
		CompletionRatio:             8.1 / 1.35,                  // ¥8.1/1M output
		CachedInputRatio:            0.27 * ratio.MilliTokensRmb, // ¥0.27/1M cached input
		ContextLength:               262144,
		MaxOutputTokens:             262144,
		InputModalities:             stepfunVideoVisionInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunReasoningFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-3.7 Flash multimodal reasoning model (released 2026-05-29); 198B sparse MoE with 1.8B vision encoder; text+image+video input; selectable reasoning levels low/medium/high; 256K context and output.",
	},
	"step-r1-v-mini": {
		Ratio:                       2.5 * ratio.MilliTokensRmb,
		CompletionRatio:             8.0 / 2.5,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             stepfunVisionInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunReasoningFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-R1-V Mini vision-capable reasoning model.",
	},

	// Step-1 family (legacy general-purpose chat). step-1-128k / -256k are no
	// longer on the official pricing page but retained for channel compatibility
	// using historical rates (¥4/¥20 and ¥8/¥40 per 1M tokens).
	"step-1-8k": {
		Ratio:                       5.0 * ratio.MilliTokensRmb,
		CompletionRatio:             20.0 / 5.0,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunTextInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1 closed-weight chat model with 8k context.",
	},
	"step-1-32k": {
		Ratio:                       15.0 * ratio.MilliTokensRmb,
		CompletionRatio:             70.0 / 15.0,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunTextInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1 closed-weight chat model with 32k context.",
	},
	"step-1-128k": {
		Ratio:                       40.0 * ratio.MilliTokensRmb,
		CompletionRatio:             200.0 / 40.0,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunTextInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1 long-context closed-weight chat model with 128k context (legacy estimated pricing).",
	},
	"step-1-256k": {
		Ratio:                       95.0 * ratio.MilliTokensRmb,
		CompletionRatio:             300.0 / 95.0,
		ContextLength:               262144,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunTextInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1 long-context closed-weight chat model with 256k context (legacy estimated pricing).",
	},
	"step-1-flash": {
		Ratio:                       0.5 * ratio.MilliTokensRmb,
		CompletionRatio:             1.5 / 0.5,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunTextInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1 Flash low-latency closed-weight chat model (legacy estimated pricing).",
	},

	// Step-1V / Step-1O vision-capable chat.
	"step-1v-8k": {
		Ratio:                       5.0 * ratio.MilliTokensRmb,
		CompletionRatio:             20.0 / 5.0,
		ContextLength:               8192,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunVisionInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1V vision-capable chat model with 8k context.",
	},
	"step-1v-32k": {
		Ratio:                       15.0 * ratio.MilliTokensRmb,
		CompletionRatio:             70.0 / 15.0,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunVisionInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1V vision-capable chat model with 32k context.",
	},
	"step-1o-turbo-vision": {
		Ratio:                       2.5 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * 2.5 * ratio.MilliTokensRmb, // Prompt Cache: cached input billed at 20% of input price.
		CompletionRatio:             8.0 / 2.5,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunVisionInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1O Turbo vision-capable low-latency multimodal chat model.",
	},
	"step-1o-vision-32k": {
		Ratio:                       15.0 * ratio.MilliTokensRmb,
		CompletionRatio:             70.0 / 15.0,
		ContextLength:               32768,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunVisionInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1O vision-capable chat model with 32k context.",
	},

	// step-1x-medium retained as a multimodal chat SKU (image generation
	// per-image pricing is handled outside the token rate sheet).
	"step-1x-medium": {
		Ratio:                       2.0 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               16384,
		MaxOutputTokens:             4096,
		InputModalities:             stepfunVisionInputs,
		OutputModalities:            stepfunTextOutputs,
		SupportedFeatures:           stepfunChatFeatures,
		SupportedSamplingParameters: stepfunSamplingParams,
		Description:                 "StepFun Step-1X mid-tier multimodal chat model (legacy estimated pricing; image generation billed per image).",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// StepFunToolingDefaults notes that StepFun publishes model pricing only; no tool-specific fees are documented (verified 2026-06-13).
// Source: https://platform.stepfun.com/docs/zh/guides/pricing/details.md
var StepFunToolingDefaults = adaptor.ChannelToolConfig{}
