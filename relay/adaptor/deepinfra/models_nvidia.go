package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// nvidiaModels returns the nvidia model defaults for deepinfra.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func nvidiaModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"nvidia/NVIDIA-Nemotron-3-Super-120B-A12B": {
			Ratio:                       nativeRate(0.085),
			CompletionRatio:             4.705882352941177,
			ContextLength:               262144,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "nvidia/NVIDIA-Nemotron-3-Super-120B-A12B on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"nvidia/NVIDIA-Nemotron-3-Ultra-550B-A55B": {
			Ratio:                       nativeRate(0.5),
			CompletionRatio:             4.4,
			CachedInputRatio:            nativeRate(0.1),
			ContextLength:               262144,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "nvidia/NVIDIA-Nemotron-3-Ultra-550B-A55B on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"nvidia/NVIDIA-Nemotron-3.5-Lightning": {
			Ratio:                       nativeRate(0.08),
			CompletionRatio:             2.5,
			CachedInputRatio:            nativeRate(0.04),
			ContextLength:               262144,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "nvidia/NVIDIA-Nemotron-3.5-Lightning on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"nvidia/Nemotron-3-Embed-1B-BF16": {
			Ratio:           nativeRate(0.015),
			CompletionRatio: 1,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.015),
			},
			ContextLength:    32768,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "NVIDIA multilingual Nemotron 1B BF16 embedding model.",
		},
		"nvidia/Nemotron-3-Embed-1B-NVFP4": {
			Ratio:           nativeRate(0.01),
			CompletionRatio: 1,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.01),
			},
			ContextLength:    32768,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "NVIDIA multilingual Nemotron 1B NVFP4 embedding model.",
		},
		"nvidia/Nemotron-3-Embed-8B": {
			Ratio:           nativeRate(0.035),
			CompletionRatio: 1,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.035),
			},
			ContextLength:    32768,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
			Description:      "NVIDIA multilingual Nemotron 8B embedding model.",
		},
		"nvidia/Nemotron-3-Nano-30B-A3B": {
			Ratio:                       nativeRate(0.05),
			CompletionRatio:             4,
			CachedInputRatio:            nativeRate(0.025),
			ContextLength:               262144,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "nvidia/Nemotron-3-Nano-30B-A3B on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"nvidia/Nemotron-3.5-ASR-Streaming-Multilingual-0.6b": {
			Ratio:           nativeRate(0.3333333333333333),
			CompletionRatio: 1,
			Audio: &adaptor.AudioPricingConfig{
				PromptTokensPerSecond: 10,
				UsdPerSecond:          3.3333333333333333e-06,
			},
			InputModalities:  []string{"audio"},
			OutputModalities: []string{"text"},
			Description:      "Low-latency multilingual Nemotron ASR model.",
		},
		"nvidia/Nemotron-Content-Safety-3.5": {
			Ratio:                       nativeRate(0.2),
			CompletionRatio:             1,
			ContextLength:               131072,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "nvidia/Nemotron-Content-Safety-3.5 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"nvidia/llama-nemotron-embed-vl-1b-v2": {
			Ratio:           nativeRate(0.01),
			CompletionRatio: 0,
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio: nativeRate(0.01),
			},
			ContextLength:    10240,
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"embedding"},
			Description:      "nvidia/llama-nemotron-embed-vl-1b-v2 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
