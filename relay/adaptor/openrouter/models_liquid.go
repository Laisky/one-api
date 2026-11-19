package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// liquidModels returns independently owned liquid defaults for openrouter.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://openrouter.ai/api/v1/models
func liquidModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"liquid/lfm-2-24b-a2b": {
			Ratio:                       nativeRate(0.03),
			CompletionRatio:             4,
			ContextLength:               32768,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "max_tokens", "logit_bias"},
			HuggingFaceID:               "LiquidAI/LFM2-24B-A2B",
			Description:                 "LFM2-24B-A2B is the largest model in the LFM2 family of hybrid architectures designed for efficient on-device deployment. Built as a 24B parameter Mixture-of-Experts model with only 2B active parameters per... [Deprecated: no longer listed on OpenRouter's live model catalog as of 2026-07-13]",
		},
		"liquid/lfm-2.5-1.2b-instruct:free": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               32768,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			HuggingFaceID:               "LiquidAI/LFM2.5-1.2B-Instruct",
			Description:                 "LFM2.5-1.2B-Instruct is a compact, high-performance instruction-tuned model built for fast on-device AI. It delivers strong chat quality in a 1.2B parameter footprint, with efficient edge inference and broad runtime support.",
		},
		"liquid/lfm-2.5-1.2b-thinking:free": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               32768,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			HuggingFaceID:               "LiquidAI/LFM2.5-1.2B-Thinking",
			Description:                 "LFM2.5-1.2B-Thinking is a lightweight reasoning-focused model optimized for agentic tasks, data extraction, and RAG—while still running comfortably on edge devices. It supports long context (up to 32K tokens) and is...",
		},
		"liquid/lfm-2.5-2.6b:free": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               65536,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "logprobs", "max_completion_tokens", "max_tokens", "min_p", "presence_penalty", "reasoning", "repetition_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "tool_choice", "tools", "top_k", "top_logprobs", "top_p"},
			HuggingFaceID:               "LiquidAI/LFM2.5-2.6B",
			Description:                 "liquid/lfm-2.5-2.6b:free on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"liquid/lfm-2.5-24b-a2b": {
			Ratio:                       nativeRate(0.05),
			CompletionRatio:             14,
			CachedInputRatio:            nativeRate(0.025),
			ContextLength:               128000,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "logprobs", "max_completion_tokens", "max_tokens", "min_p", "presence_penalty", "reasoning", "repetition_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "tool_choice", "tools", "top_k", "top_logprobs", "top_p"},
			HuggingFaceID:               "LiquidAI/LFM2.5-24B-A2B",
			Description:                 "liquid/lfm-2.5-24b-a2b on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"liquid/lfm-3-1.7b": {
			Ratio:                       nativeRate(0.08),
			CompletionRatio:             2.5,
			CachedInputRatio:            nativeRate(0.02),
			ContextLength:               262144,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "logprobs", "max_completion_tokens", "max_tokens", "min_p", "presence_penalty", "reasoning", "repetition_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "tool_choice", "tools", "top_k", "top_logprobs", "top_p"},
			HuggingFaceID:               "LiquidAI/LFM3-1.7B",
			Description:                 "liquid/lfm-3-1.7b on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"liquid/lfm-3-1.7b:free": {
			Ratio:                       nativeRate(0),
			CompletionRatio:             1,
			ContextLength:               262144,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "logprobs", "max_completion_tokens", "max_tokens", "min_p", "presence_penalty", "reasoning", "repetition_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "tool_choice", "tools", "top_k", "top_logprobs", "top_p"},
			HuggingFaceID:               "LiquidAI/LFM3-1.7B",
			Description:                 "liquid/lfm-3-1.7b:free on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges. The published token tariff is explicitly free; account limits still apply.",
		},
	}
}
