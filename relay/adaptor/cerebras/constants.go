package cerebras

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Reusable modality / feature / sampling fragments for Cerebras-hosted models.
//
// Sources (retrieved 2026-06-27):
//   - https://inference-docs.cerebras.ai/models/overview      (public catalog: production vs preview)
//   - https://inference-docs.cerebras.ai/models/openai-oss    (gpt-oss-120b card: context, pricing, reasoning, tools)
//   - https://inference-docs.cerebras.ai/models/zai-glm-47    (zai-glm-4.7 card: context, pricing, reasoning, tools)
//   - https://inference-docs.cerebras.ai/api-reference/chat-completions
//   - https://inference-docs.cerebras.ai/resources/openai     (OpenAI compatibility & supported parameters)
var (
	// textInputs advertises a chat model that consumes text only. Both GA-track
	// Cerebras models are text-only; vision input is a Preview-only capability.
	textInputs = []string{"text"}
	// textOutputs advertises text-only output.
	textOutputs = []string{"text"}

	// reasoningFeatures is the capability set Cerebras advertises for its
	// reasoning chat models. Cerebras' chat completions endpoint exposes tool /
	// function calling (including parallel tool calls), JSON mode, and
	// structured outputs (json_schema), and both bundled models are reasoning
	// models tuned via reasoning_effort.
	reasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}

	// samplingParams enumerates the OpenAI-compatible sampling parameters
	// Cerebras accepts on chat completions. Unlike some inference providers,
	// Cerebras explicitly supports frequency_penalty, presence_penalty,
	// logit_bias, and logprobs in addition to the common knobs, and accepts
	// reasoning_effort as a standard top-level parameter.
	// Source: https://inference-docs.cerebras.ai/resources/openai
	samplingParams = []string{
		"temperature",
		"top_p",
		"max_tokens",
		"max_completion_tokens",
		"stop",
		"presence_penalty",
		"frequency_penalty",
		"seed",
		"logit_bias",
		"logprobs",
		"top_logprobs",
		"response_format",
		"tools",
		"tool_choice",
		"parallel_tool_calls",
		"reasoning_effort",
		"user",
	}

	// reasoningEfforts lists the reasoning_effort levels Cerebras documents for
	// gpt-oss-120b ("none" additionally disables reasoning, but is not part of
	// the project's reasoning-effort vocabulary). Source:
	// https://inference-docs.cerebras.ai/models/openai-oss
	reasoningEfforts = []string{"low", "medium", "high"}
)

// ModelRatios contains the Cerebras Inference models exposed by this adaptor.
//
// Only models that are live on the shared public API (api.cerebras.ai) AND carry
// a publicly published per-token price are registered. As of 2026-06-27 that is:
//
//   - gpt-oss-120b   — Production / GA       ($0.35 in / $0.75 out per 1M tokens)
//   - zai-glm-4.7    — Preview (still live)  ($2.25 in / $2.75 out per 1M tokens)
//
// gemma-4-31b is listed as "coming soon" (not yet live, no published price), so
// it is intentionally omitted rather than registered with a fabricated rate.
//
// Pricing is encoded as Ratio = <USD per 1M input tokens> * ratio.MilliTokensUsd
// and CompletionRatio = <USD per 1M output> / <USD per 1M input>, following the
// project convention. Per-token rates are taken from each model's official doc
// card and corroborated by the public-models API
// (https://inference-docs.cerebras.ai/api-reference/models/public-models).
//
// Context length is 131,072 tokens on the paid tier for both models; the
// documented paid-tier max completion length is ~40k tokens.
var ModelRatios = map[string]adaptor.ModelConfig{
	// ---- OpenAI gpt-oss (Production / GA) ----
	"gpt-oss-120b": {
		Ratio:                       0.35 * ratio.MilliTokensUsd,
		CompletionRatio:             0.75 / 0.35,
		ContextLength:               131072,
		MaxOutputTokens:             40000,
		InputModalities:             textInputs,
		OutputModalities:            textOutputs,
		SupportedFeatures:           reasoningFeatures,
		SupportedSamplingParameters: samplingParams,
		SupportedReasoningEfforts:   reasoningEfforts,
		DefaultReasoningEffort:      "medium",
		HuggingFaceID:               "openai/gpt-oss-120b",
		Description:                 "OpenAI gpt-oss-120b open-weight MoE reasoning model served on Cerebras wafer-scale hardware for ultra-low-latency inference; text-only, tools and structured outputs, 131K context.",
	},

	// ---- Z.ai GLM (Preview — live on the public API with published pricing) ----
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
		Description:                 "Z.ai GLM-4.7 (355B) reasoning/agent model on Cerebras; reasoning is on by default. Marked Preview by Cerebras (evaluation only, may change on short notice). Text-only, tools and structured outputs, 131K context.",
	},
}

// ModelList is derived from ModelRatios for backward compatibility.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// CerebrasToolingDefaults declares no built-in tool metering: Cerebras does not
// publish provider-level per-tool pricing (retrieved 2026-06-27).
var CerebrasToolingDefaults = adaptor.ChannelToolConfig{}
