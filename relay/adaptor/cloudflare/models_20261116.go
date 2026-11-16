package cloudflare

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init registers verified native Workers AI models and annotates announced
// retirements without deleting administrator-configured IDs. It takes no
// parameters and returns no values. Sources verified on 2026-11-16:
// https://developers.cloudflare.com/workers-ai/models/deepseek-v4.1-flash-1017/
// https://developers.cloudflare.com/workers-ai/models/inkling-2-small/
// https://developers.cloudflare.com/workers-ai/models/embeddinggemma-2/
// https://developers.cloudflare.com/changelog/product/workers-ai/
func init() {
	ModelRatios = adaptor.JoinModelCatalogs(ModelRatios, novemberModels())
	for _, id := range []string{
		"@cf/deepseek-ai/deepseek-r1-distill-qwen-32b", "@cf/nvidia/nemotron-3-120b-a12b",
		"@cf/deepseek-ai/deepseek-v4-pro-0813", "@cf/deepseek-ai/deepseek-v4-flash-0731",
		"@cf/qwen/qwq-32b", "@cf/qwen/qwen3-30b-a3b-fp8",
		"@cf/moonshotai/kimi-k2.6", "@cf/moonshotai/kimi-k2.7-code",
		"@cf/zai-org/glm-5.2", "@cf/zai-org/glm-5.3-flash",
		"@cf/meta-llama/llama-2-7b-chat-hf-lora", "@cf/google/gemma-2b-it-lora", "@cf/google/gemma-7b-it-lora",
		"@cf/meta/llama-3.1-8b-instruct-fp8", "@cf/meta/llama-3.2-1b-instruct", "@cf/meta/llama-3.2-3b-instruct",
		"@cf/myshell-ai/melotts", "@cf/stabilityai/stable-diffusion-xl-base-1.0",
		"@cf/runwayml/stable-diffusion-v1-5-inpainting", "@cf/bytedance/stable-diffusion-xl-lightning",
		"@cf/lykon/dreamshaper-8-lcm", "@cf/openai/whisper-tiny-en",
		"@cf/llava-hf/llava-1.5-7b-hf", "@cf/google/embeddinggemma-300m",
	} {
		if cfg, exists := ModelRatios[id]; exists {
			cfg = cfg.Clone()
			cfg.Description += " Cloudflare announced retirement on 2026-11-20 (postponed from November 15), after which requests return HTTP 410, error 5028. Historical ID and tariff retained for operator configurations."
			ModelRatios[id] = cfg
		}
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}

// novemberModels returns independently owned Workers AI defaults with native
// provider tariffs and published limits. It takes no parameters. Account plan
// eligibility is enforced upstream and does not suppress these published IDs.
func novemberModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"@cf/deepseek-ai/deepseek-v4.1-flash-1017": {
			Ratio: 0.30 * ratio.MilliTokensUsd, CachedInputRatio: 0.006 * ratio.MilliTokensUsd, CompletionRatio: 1.20 / 0.30,
			ContextLength:   1_048_576,
			InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedReasoningEfforts:   []string{"none", "low", "high", "max"},
			DefaultReasoningEffort:      "high",
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "tools", "tool_choice", "response_format", "reasoning_effort"},
			Description:                 "Workers AI DeepSeek V4.1 Flash 1017 with vision, tools and a 1,048,576-token context. Native none/low/high/max reasoning, default high; upstream accepts additional compatibility aliases. USD/M input/cache/output: 0.30/0.006/1.20. Maximum output and hosted precision are not specified. Workers Paid or prepaid AI Gateway credits are required upstream.",
		},
		"@cf/thinkingmachines/inkling-2-small": {
			Ratio: 0.30 * ratio.MilliTokensUsd, CachedInputRatio: 0.10 * ratio.MilliTokensUsd, CompletionRatio: 1.00 / 0.30,
			ContextLength:   1_048_576,
			InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedReasoningEfforts:   []string{"none", "minimal", "low", "medium", "high", "max"},
			DefaultReasoningEffort:      "high",
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "tools", "tool_choice", "response_format", "reasoning_effort"},
			Description:                 "Workers AI Inkling 2 Small, released 2026-11-12, with vision, tools and a 1,048,576-token context. Native none/minimal/low/medium/high/max reasoning, default high. USD/M input/cache/output: 0.30/0.10/1.00. Maximum output and hosted precision are not specified. Workers Paid or prepaid AI Gateway credits are required upstream.",
		},
		"@cf/google/embeddinggemma-2": {
			Ratio: 0.0017 * ratio.MilliTokensUsd, CompletionRatio: 1,
			ContextLength:   8192,
			InputModalities: []string{"text"},
			Embedding:       &adaptor.EmbeddingPricingConfig{TextTokenRatio: 0.0017 * ratio.MilliTokensUsd},
			Description:     "Workers AI EmbeddingGemma 2, released 2026-11-12, at $0.0017/M input tokens. Standard text embeddings accept dimensions 128/256/512/768 (default 768) and float/base64 encoding. The provider's separate nested-conversation media input is not advertised through this standard embeddings contract.",
		},
	}
}
