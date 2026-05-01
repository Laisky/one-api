package novita

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// Reusable metadata helpers for Novita-served models. Novita is an inference
// aggregator hosting open-weight checkpoints, so most rows reuse a small set of
// modality and feature constants.
//
// Sources (retrieved 2026-04-28):
//   - https://novita.ai/llm-api  (per-model context, max output, cache pricing)
//   - https://novita.ai/pricing  (token pricing baseline)
//   - https://docs.litellm.ai/docs/providers/novita  (OpenAI-compatible
//     parameter list including top_k, min_p, repetition_penalty)
//   - https://novita.ai/docs/guides/llm-function-calling  (tool calling)
//   - HuggingFace model cards for upstream weights and quantization defaults.
var (
	// novitaTextOnlyModalities advertises a chat model that consumes and emits text only.
	novitaTextOnlyModalities = []string{"text"}
	// novitaTextImageInModalities advertises text+image input with text output (VL/vision models).
	novitaTextImageInModalities = []string{"text", "image"}

	// novitaChatFeatures advertises the standard non-thinking capability set.
	// Novita exposes OpenAI-compatible tools and JSON mode for these chat models.
	novitaChatFeatures = []string{"tools", "json_mode"}
	// novitaReasoningFeatures advertises tools/json_mode plus reasoning for thinking-mode models.
	novitaReasoningFeatures = []string{"tools", "json_mode", "reasoning"}
	// novitaReasoningOnlyFeatures applies to reasoning models that historically did not
	// expose stable tool calling on Novita (e.g., legacy DeepSeek R1 distills).
	novitaReasoningOnlyFeatures = []string{"reasoning"}
	// novitaTextOnlyFeatures applies to legacy or lightweight chat models without
	// documented tool/json_mode coverage on Novita.
	novitaTextOnlyFeatures []string

	// novitaSamplingParams enumerates the OpenAI-compatible sampling parameters Novita
	// accepts on chat completions, including the vLLM-style top_k, min_p and
	// repetition_penalty extensions documented by LiteLLM.
	novitaSamplingParams = []string{
		"temperature",
		"top_p",
		"top_k",
		"min_p",
		"max_tokens",
		"stop",
		"presence_penalty",
		"frequency_penalty",
		"repetition_penalty",
		"seed",
		"n",
		"logit_bias",
		"logprobs",
		"top_logprobs",
		"response_format",
		"tools",
		"tool_choice",
	}
	// novitaReasoningSamplingParams trims the sampling set for reasoning-mode endpoints
	// where temperature/top_p style knobs are commonly disabled or ignored.
	novitaReasoningSamplingParams = []string{
		"max_tokens",
		"stop",
		"seed",
		"response_format",
		"tools",
		"tool_choice",
	}
)

// ModelRatios contains Novita serverless models with explicit token pricing from the public pricing page.
// The map is assembled from per-family sub-maps in the models_*.go files in this package; the
// derived model list reads from this aggregated map.
// Source: https://novita.ai/pricing and https://novita.ai/llm-api (retrieved 2026-04-28)
// Tiered or omnimodal rows without explicit token pricing on the public page are intentionally excluded.
var ModelRatios = mergeModelRatios(
	baiduErnieModelRatios,
	deepseekModelRatios,
	googleGemmaModelRatios,
	metaLlamaModelRatios,
	qwenModelRatios,
	zaiGLMModelRatios,
	miscModelRatios,
)

// mergeModelRatios merges several Novita sub-family pricing maps into a single map.
// It panics on duplicate keys to prevent silent shadowing during refactors.
func mergeModelRatios(sources ...map[string]adaptor.ModelConfig) map[string]adaptor.ModelConfig {
	total := 0
	for _, src := range sources {
		total += len(src)
	}
	merged := make(map[string]adaptor.ModelConfig, total)
	for _, src := range sources {
		for name, cfg := range src {
			if _, exists := merged[name]; exists {
				panic("novita: duplicate model id in ModelRatios: " + name)
			}
			merged[name] = cfg
		}
	}
	return merged
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// NovitaToolingDefaults notes that Novita's public pricing focuses on model tokens; no tool metering is published (retrieved 2026-04-28).
// Source: https://novita.ai/pricing
var NovitaToolingDefaults = adaptor.ChannelToolConfig{}
