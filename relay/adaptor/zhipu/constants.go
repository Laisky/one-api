package zhipu

import (
	"maps"

	"github.com/Laisky/one-api/relay/adaptor"
)

// chatSamplingParameters returns the OpenAI-compatible sampling parameters supported
// by Zhipu's chat-completions models. Zhipu accepts the standard OpenAI set plus
// `top_k` and `repetition_penalty`. A new slice is returned on every call so callers
// cannot mutate shared state.
//
// Source: https://docs.bigmodel.cn/cn/guide/start/concept-param
func chatSamplingParameters() []string {
	return []string{
		"temperature",
		"top_p",
		"top_k",
		"frequency_penalty",
		"presence_penalty",
		"repetition_penalty",
		"stop",
		"seed",
		"max_tokens",
		"logit_bias",
	}
}

// reasoningSamplingParameters returns the constrained sampling-parameter set used
// by Zhipu's GLM-Z1 (and other reasoning) models, which do not accept temperature
// or penalty knobs.
func reasoningSamplingParameters() []string {
	return []string{"max_tokens", "stop", "seed"}
}

// commonChatFeatures advertises the feature set that virtually every Zhipu chat
// model supports (function-calling, JSON mode, structured outputs, web search).
func commonChatFeatures() []string {
	return []string{"tools", "json_mode", "structured_outputs", "web_search"}
}

// reasoningChatFeatures appends `reasoning` to commonChatFeatures for thinking-capable models.
func reasoningChatFeatures() []string {
	return []string{"tools", "json_mode", "structured_outputs", "web_search", "reasoning"}
}

// textOutput returns a fresh `["text"]` slice for the OutputModalities field.
func textOutput() []string { return []string{"text"} }

// textInput returns a fresh `["text"]` slice for the InputModalities field.
func textInput() []string { return []string{"text"} }

// textImageInput returns a fresh `["text","image"]` slice for vision models.
func textImageInput() []string { return []string{"text", "image"} }

// textImageVideoFileInput returns the full multimodal input set used by GLM-4.6V.
func textImageVideoFileInput() []string { return []string{"text", "image", "file"} }

// ModelRatios contains all supported models and their pricing ratios.
// The model list is derived from the keys of this map, eliminating redundancy.
// Pricing source: https://open.bigmodel.cn/pricing.
//
// The map is composed from family-specific sub-maps defined in sibling files
// (constants_text.go, constants_vision.go, constants_misc.go) so that each
// file stays small and easy to navigate. Pricing entries are immutable.
//
// Metadata sources:
//   - https://docs.bigmodel.cn/cn/guide/start/model-overview
//   - https://docs.bigmodel.cn/cn/guide/models/text/* and /vlm/*
//   - https://huggingface.co/zai-org for open-weight HuggingFace IDs.
var ModelRatios = mergeModelRatios(
	flagshipTextModels,
	flagshipVisionModels,
	languageModels,
	reasoningModels,
	multimodalModels,
	imageGenerationModels,
	utilityModels,
	embeddingModels,
	ocrModels,
	legacyModels,
)

// mergeModelRatios consolidates the per-family pricing tables into the unified
// ModelRatios map. It panics if any model key is duplicated across families,
// which surfaces accidental overlaps at startup rather than silently masking
// pricing entries.
func mergeModelRatios(tables ...map[string]adaptor.ModelConfig) map[string]adaptor.ModelConfig {
	total := 0
	for _, t := range tables {
		total += len(t)
	}
	merged := make(map[string]adaptor.ModelConfig, total)
	for _, t := range tables {
		for k := range t {
			if _, exists := merged[k]; exists {
				panic("zhipu: duplicate model key in ModelRatios: " + k)
			}
		}
		maps.Copy(merged, t)
	}
	return merged
}

// ZhipuToolingDefaults captures Open BigModel's published search-tool pricing tiers (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://open.bigmodel.cn/pricing
var ZhipuToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"search_std":       {UsdPerCall: 0.01},
		"search_pro":       {UsdPerCall: 0.03},
		"search_pro_sogou": {UsdPerCall: 0.05},
		"search_pro_quark": {UsdPerCall: 0.05},
	},
}
