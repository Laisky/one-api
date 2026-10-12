package nvidia

import "github.com/Laisky/one-api/relay/adaptor"

// currentGLMModels returns separately allocated NVIDIA-hosted GLM 5.3 defaults.
// It accepts no parameters. Free pricing applies only to the published trial
// endpoints, never to a paid Partner Endpoint or an AI Enterprise deployment.
// Sources verified 2026-10-12:
// https://build.nvidia.com/z-ai/glm-5-3
// https://build.nvidia.com/z-ai/glm-5-3/modelcard
// https://build.nvidia.com/z-ai/glm-5-3-flash
// https://build.nvidia.com/z-ai/glm-5-3-flash/modelcard
func currentGLMModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"z-ai/glm-5.3": {
			Ratio: 0, CompletionRatio: 1, ContextLength: 1048576,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
			SupportedReasoningEfforts:   []string{"low", "high", "max"}, DefaultReasoningEffort: "max", Quantization: "fp4",
			Description: "NVIDIA-hosted GLM-5.3, exact dotted API ID, text-only 1M context. This endpoint serves NVFP4 weights, not the creator's FP8 checkpoint. Low/high/max effort, default max. Free trial endpoint; paid partner/deployment pricing requires channel overrides. Native chat_template_kwargs.clear_thinking defaults to false; NVIDIA recommends true for chat scenarios.",
		},
		"z-ai/glm-5.3-flash": {
			Ratio: 0, CompletionRatio: 1, ContextLength: 1048576,
			InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
			SupportedReasoningEfforts:   []string{"low", "high", "max"}, DefaultReasoningEffort: "max", Quantization: "fp8",
			Description: "NVIDIA-hosted GLM-5.3-Flash, exact dotted API ID, native FP8 checkpoint and text/image input (up to eight images upstream). Low/high/max effort, default max; 1M context. Free trial endpoint only, not a paid partner tariff. Native chat_template_kwargs.clear_thinking defaults to false; NVIDIA recommends true for chat scenarios.",
		},
	}
}
