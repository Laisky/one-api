package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// qwenAudioModels returns independently owned qwen audio defaults for ali.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
func qwenAudioModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"qwen-audio-chat": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               8192,
			MaxOutputTokens:             2000,
			InputModalities:             []string{"text", "file"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			Description:                 "Qwen-Audio Chat: legacy closed-weight audio chat model (free trial).",
		},
		"qwen-audio-turbo": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               8192,
			MaxOutputTokens:             2000,
			InputModalities:             []string{"text", "file"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			Description:                 "Qwen-Audio Turbo: closed-weight audio-understanding model (currently free trial).",
		},
		"qwen-omni-turbo": {
			Ratio:                       nativeRate(0.4),
			CompletionRatio:             4,
			ContextLength:               32768,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text", "image", "file"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			Description:                 "Qwen-Omni Turbo: closed-weight multimodal tier (text I/O rate billed here; audio billing is per-modality upstream).",
		},
		"qwen2-audio-instruct": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               8192,
			MaxOutputTokens:             2048,
			InputModalities:             []string{"text", "file"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "bf16",
			HuggingFaceID:               "Qwen/Qwen2-Audio-7B-Instruct",
			Description:                 "Qwen2-Audio Instruct: open-weight audio-understanding model (currently free trial).",
		},
		"qwen3-omni-flash": {
			Ratio:                       nativeRate(1.8),
			CompletionRatio:             3.8333,
			ContextLength:               262144,
			MaxOutputTokens:             16384,
			InputModalities:             []string{"text", "image", "file"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			Description:                 "Qwen3-Omni Flash: closed-weight multimodal (text/image/audio/video) tier (text I/O rate billed here; audio billing is per-modality upstream).",
		},
		"qwen3.5-omni-plus": {
			Ratio:                       nativeRate(7),
			CompletionRatio:             5.714285714285714,
			ContextLength:               262144,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text", "image", "audio", "video"},
			OutputModalities:            []string{"text", "audio"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			Description:                 "Qwen3.5 Omni Plus: closed-weight multimodal model accepting text/image/audio/video inputs with text and audio outputs; 262K context.",
		},
		"qwen3.5-omni-plus-realtime": {
			Ratio:                       0.7142857142857143,
			CompletionRatio:             6,
			ContextLength:               262144,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text", "image", "audio", "video"},
			OutputModalities:            []string{"text", "audio"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			Description:                 "Qwen3.5 Omni Plus Realtime: closed-weight realtime multimodal model accepting text/image/audio/video inputs with text and audio outputs; 262K context (text I/O rate billed here; audio billing is per-modality upstream).",
		},
	}
}
