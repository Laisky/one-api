package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// ModelRatios contains all supported Alibaba Cloud (DashScope / Model Studio) models
// and their pricing/configuration metadata. The map is assembled from family-specific
// submaps (constants_<family>.go) so the per-family files stay focused and readable.
// The model list is derived from the keys of this map.
//
// All chat/coder/math token prices are expressed in MilliTokensRmb (quota per
// milli-token, RMB pricing):
//
//	1 RMB per 1,000 tokens = 1 * 1000 * ratio.MilliTokensRmb
//	Example: 0.002 RMB/1k tokens = 0.002 * 1000 * ratio.MilliTokensRmb
//
// Pricing/metadata sources (verified 2026-05-01):
//   - https://help.aliyun.com/zh/model-studio/getting-started/models
//   - https://www.alibabacloud.com/help/en/model-studio/
//   - https://huggingface.co/Qwen
var ModelRatios = mergeModelRatios(
	qwenClosedModelRatios,
	qwenOpenModelRatios,
	deepseekModelRatios,
	embeddingModelRatios,
	wanxModelRatios,
)

// AliToolingDefaults notes that Alibaba Model Studio does not expose public built-in tool pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://help.aliyun.com/en/model-studio/developer-reference/tools-reference (requires authentication)
var AliToolingDefaults = adaptor.ChannelToolConfig{}

// mergeModelRatios concatenates per-family pricing maps into a single ModelRatios map.
// It returns a fresh map and panics if duplicate keys are detected so misconfiguration
// surfaces at process start rather than silently overwriting entries.
func mergeModelRatios(maps ...map[string]adaptor.ModelConfig) map[string]adaptor.ModelConfig {
	total := 0
	for _, m := range maps {
		total += len(m)
	}
	out := make(map[string]adaptor.ModelConfig, total)
	for _, m := range maps {
		for name, cfg := range m {
			if _, dup := out[name]; dup {
				panic("ali.mergeModelRatios: duplicate model entry: " + name)
			}
			out[name] = cfg
		}
	}
	return out
}

// qwenStandardSamplingParameters returns a fresh slice of OpenAI-compatible sampling
// parameters supported by standard (non-reasoning) Qwen chat-completions models.
// Alibaba Model Studio natively exposes top_k and repetition_penalty in addition to
// the OpenAI-compatible set. A new slice is returned on every call to keep callers
// from mutating shared state.
func qwenStandardSamplingParameters() []string {
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
	}
}

// qwenReasoningSamplingParameters returns the constrained sampling-parameter set
// supported by Qwen reasoning models (QwQ, qwen3-thinking, qwen3-coder-thinking, etc.).
// Reasoning variants reject most decoding-tuning parameters and accept only seed
// plus max_tokens reliably.
func qwenReasoningSamplingParameters() []string {
	return []string{"seed", "max_tokens"}
}

// qwenChatFeatures returns the default capability set advertised by Qwen
// chat/instruct models on Model Studio: tool-calling, JSON mode, and structured
// outputs. A fresh slice is returned on each call.
func qwenChatFeatures() []string {
	return []string{"tools", "json_mode", "structured_outputs"}
}

// qwenReasoningFeatures returns the capability set for Qwen reasoning models,
// which expose a chain-of-thought channel in addition to the standard chat
// features. A fresh slice is returned on each call.
func qwenReasoningFeatures() []string {
	return []string{"tools", "json_mode", "structured_outputs", "reasoning"}
}
