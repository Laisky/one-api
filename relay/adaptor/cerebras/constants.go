package cerebras

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Sources verified 2026-09-24:
//   - https://api.cerebras.ai/public/v1/models (live prices, limits, capabilities)
//   - https://inference-docs.cerebras.ai/models/overview
//   - https://inference-docs.cerebras.ai/capabilities/reasoning
//   - https://inference-docs.cerebras.ai/dedicated/overview
//
// Discovery is not an account-permission check. Dedicated/legacy entries remain
// configurable; their historical shared-tier prices are not enterprise quotes.
var (
	textInputs        = []string{"text"}
	textOutputs       = []string{"text"}
	visionInputs      = []string{"text", "image"}
	reasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}
	// samplingParams retains the legacy models' parameter metadata. Current
	// public models declare their own supported set below.
	samplingParams = []string{
		"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop",
		"presence_penalty", "frequency_penalty", "seed", "logit_bias",
		"logprobs", "top_logprobs", "response_format", "tools", "tool_choice",
		"parallel_tool_calls", "reasoning_effort", "user",
	}
	reasoningEfforts = []string{"low", "medium", "high"}
)

// ModelRatios contains current public models plus dedicated/legacy compatibility
// entries. Prices are USD per million tokens; mixed precision is not represented
// as a single standard quantization label.
var ModelRatios = map[string]adaptor.ModelConfig{
	"gpt-oss-120b": {
		Ratio:                     0.35 * ratio.MilliTokensUsd,
		CompletionRatio:           0.75 / 0.35,
		ContextLength:             131072,
		MaxOutputTokens:           40960,
		InputModalities:           textInputs,
		OutputModalities:          textOutputs,
		SupportedFeatures:         reasoningFeatures,
		SupportedReasoningEfforts: reasoningEfforts,
		DefaultReasoningEffort:    "medium",
		SupportedSamplingParameters: []string{
			"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop",
			"presence_penalty", "frequency_penalty", "seed", "logit_bias",
			"response_format", "tools", "tool_choice", "reasoning_effort", "user",
		},
		HuggingFaceID: "openai/gpt-oss-120b",
		Description:   "OpenAI GPT-OSS 120B on Cerebras. Public model with text input, tools, structured outputs, and low/medium/high reasoning (default medium). The live public catalog does not advertise logprobs or parallel tool calls.",
	},
	"qwen-3.8-27b": {
		Ratio:                       0.99 * ratio.MilliTokensUsd,
		CompletionRatio:             1.49 / 0.99,
		ContextLength:               65536,
		MaxOutputTokens:             32768,
		InputModalities:             visionInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "logprobs"},
		SupportedSamplingParameters: samplingParams,
		SupportedReasoningEfforts:   []string{"none", "low", "medium", "high"},
		DefaultReasoningEffort:      "high",
		HuggingFaceID:               "Qwen/Qwen3.8-27B",
		// The prose overview advertises 128K paid context, but the live public
		// models API returns 65536/32768. Record exact machine-readable limits
		// rather than guessing a paid-tier completion limit. No request denylist
		// or MaxTokens cap is introduced; operators can override metadata.
		Description: "Qwen 3.8 27B on Cerebras with image input, tools, structured outputs, logprobs, and reasoning (default high; none disables it). Limits reflect the live public models API; the overview separately advertises 128K paid context. Cerebras does not accept Groq's default effort value.",
	},
	"zai-glm-4.7": {
		Ratio:                       2.25 * ratio.MilliTokensUsd,
		CompletionRatio:             2.75 / 2.25,
		ContextLength:               131072,
		MaxOutputTokens:             40000,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		HuggingFaceID:               "zai-org/GLM-4.7",
		Description:                 "GLM-4.7 compatibility entry. No longer in Cerebras' public shared catalog; the model family is offered on dedicated endpoints. Historical shared-tier pricing and limits are retained, not a current enterprise quote. Configure contracted rates and deployment limits before use.",
	},
	"gemma-4-31b": {
		Ratio:                       0.99 * ratio.MilliTokensUsd,
		CompletionRatio:             1.49 / 0.99,
		ContextLength:               131072,
		MaxOutputTokens:             40000,
		InputModalities:             visionInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		SupportedReasoningEfforts:   []string{"none", "low", "medium", "high"},
		DefaultReasoningEffort:      "none",
		HuggingFaceID:               "google/gemma-4-31B-it",
		Description:                 "Gemma 4 31B on Cerebras dedicated endpoints; no longer in the public shared catalog. Reasoning defaults to none; low/medium/high are equivalent enable switches. Historical shared-tier pricing and limits are retained, not a current enterprise quote. Configure contracted rates and deployment limits before use.",
	},
}

// ModelList includes configurable compatibility entries as well as public models.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// CerebrasToolingDefaults does not invent a provider-level per-tool tariff.
var CerebrasToolingDefaults = adaptor.ChannelToolConfig{}
