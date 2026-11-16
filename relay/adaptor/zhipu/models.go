package zhipu

import (
	"maps"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
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
func textImageVideoFileInput() []string { return []string{"text", "image", "video", "file"} }

// ModelRatios contains all supported models and their pricing ratios.
// The model list is derived from the keys of this map, eliminating redundancy.
// Pricing source: https://bigmodel.cn/pricing.
//
// The map is composed from family-specific sub-maps defined in sibling files
// (models_text.go, models_vision.go, models_media.go, models_utility.go,
// models_audio.go) so that each
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
	audioModels,
	realtimeModels,
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

// ZhipuToolingDefaults captures Open BigModel's published CNY search-tool prices.
// Sources checked 2026-09-24: https://docs.bigmodel.cn/cn/guide/start/pricing
// and https://bigmodel.cn/pricing. Z.AI remains a separate USD catalog.
var ZhipuToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"search_std":       searchToolPricing(0.01),
		"search_pro":       searchToolPricing(0.03),
		"search_pro_sogou": searchToolPricing(0.05),
		"search_pro_quark": searchToolPricing(0.05),
	},
}

// searchToolPricing converts one CNY per-call amount to the existing USD quota
// convention. It accepts a published amount and returns its tool configuration.
func searchToolPricing(cny float64) adaptor.ToolPricingConfig {
	return adaptor.ToolPricingConfig{UsdPerCall: cny / float64(ratio.ExchangeRateRmb)}
}
