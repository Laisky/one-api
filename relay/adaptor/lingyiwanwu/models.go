package lingyiwanwu

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// LingYi WanWu (01.AI) previously hosted the Yi family of chat models.
// Its 2026-08-03 notice, checked 2026-11-16, ended public API service at
// the end of 2026-09-03. Retained entries preserve historical configuration.
// Sources: https://platform.lingyiwanwu.com/ (service adjustment notice) and
// https://platform.lingyiwanwu.com/docs (historical API and pricing).
//
// 01.AI has pivoted toward enterprise deployments; only yi-lightning and
// yi-vision-v2 were the last documented consumer platform entries.
// yi-large / yi-medium / yi-spark are no longer publicly listed and are
// omitted from this map. yi-lightning is documented as an intelligent-routing
// model that internally dispatches to DeepSeek-V3 / Qwen3-30B-A3B / Yi-Lightning.
// The smaller Yi-6B / Yi-9B / Yi-34B base models are open on HuggingFace under
// 01-ai/Yi-*; the production yi-lightning and yi-vision-v2 SKUs served by the
// platform are tuned closed-weight derivatives, so HuggingFaceID is left empty.

// lingyiwanwuTextInputs is the input modality set for text-only Yi chat models.
var lingyiwanwuTextInputs = []string{"text"}

// lingyiwanwuVisionInputs is the modality set for yi-vision* multimodal models.
var lingyiwanwuVisionInputs = []string{"text", "image"}

// lingyiwanwuTextOutputs declares the text-only output modality used by Yi.
var lingyiwanwuTextOutputs = []string{"text"}

// lingyiwanwuChatFeatures advertises tool calling and JSON mode support
// exposed by the OpenAI-compatible Yi chat API.
var lingyiwanwuChatFeatures = []string{"tools", "json_mode"}

// lingyiwanwuSamplingParams enumerates sampling parameters accepted by the
// Yi chat completion API.
var lingyiwanwuSamplingParams = []string{
	"temperature",
	"top_p",
	"max_tokens",
	"stop",
	"frequency_penalty",
	"presence_penalty",
	"seed",
}

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on LingYi WanWu pricing: https://platform.lingyiwanwu.com/docs#%E6%A8%A1%E5%9E%8B%E4%B8%8E%E8%AE%A1%E8%B4%B9
var ModelRatios = map[string]adaptor.ModelConfig{
	// LingYi WanWu Models - Based on https://platform.lingyiwanwu.com/docs
	"yi-lightning": {
		Ratio:                       0.99 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               16384,
		MaxOutputTokens:             4096,
		InputModalities:             lingyiwanwuTextInputs,
		OutputModalities:            lingyiwanwuTextOutputs,
		SupportedFeatures:           lingyiwanwuChatFeatures,
		SupportedSamplingParameters: lingyiwanwuSamplingParams,
		Description:                 "Historical 01.AI Yi-Lightning chat model. The official platform notice ended public API service at the end of 2026-09-03; the refund application window closes at the end of 2026-12-03. This entry and its historical tariff remain for operator compatibility, not as a claim of live public API availability.",
	},
	"yi-vision-v2": {
		Ratio:                       6 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               16384,
		MaxOutputTokens:             4096,
		InputModalities:             lingyiwanwuVisionInputs,
		OutputModalities:            lingyiwanwuTextOutputs,
		SupportedFeatures:           lingyiwanwuChatFeatures,
		SupportedSamplingParameters: lingyiwanwuSamplingParams,
		Description:                 "Historical 01.AI Yi-Vision v2 multimodal chat model. The official platform notice ended public API service at the end of 2026-09-03; the refund application window closes at the end of 2026-12-03. This entry and its historical tariff remain for operator compatibility, not as a claim of live public API availability.",
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// LingyiWanwuToolingDefaults notes that LingYi WanWu's pricing docs list model rates only (no tool metering) as of 2026-05-18.
// Source: https://r.jina.ai/https://platform.lingyiwanwu.com/docs#%E6%A8%A1%E5%9E%8B%E4%B8%8E%E8%AE%A1%E8%B4%B9
var LingyiWanwuToolingDefaults = adaptor.ChannelToolConfig{}
