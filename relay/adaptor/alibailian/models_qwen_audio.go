package alibailian

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// qwenAudioModels returns independently owned qwen audio defaults for alibailian.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
func qwenAudioModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"qwen-omni-turbo": {
			Ratio:                       nativeRate(0.4),
			CompletionRatio:             4,
			CachedInputRatio:            nativeRate(0.08),
			ContextLength:               32768,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "bf16",
			Description:                 "Qwen-Omni Turbo on Bailian: closed-weight multimodal tier (text I/O rate billed here).",
		},
		"qwen3-omni-flash": {
			Ratio:                       nativeRate(1.8),
			CompletionRatio:             3.8333,
			CachedInputRatio:            nativeRate(0.36),
			ContextLength:               262144,
			MaxOutputTokens:             16384,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "bf16",
			Description:                 "Qwen3-Omni Flash on Bailian: closed-weight multimodal tier (text I/O rate billed here).",
		},
	}
}
