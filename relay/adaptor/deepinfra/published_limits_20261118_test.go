package deepinfra_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/deepinfra"
)

// TestDeepInfraPublishedOutputAndContextLimits pins the per-model limits that
// DeepInfra publishes in its model API (https://api.deepinfra.com/models/<model>,
// fields `max_output_tokens` and `max_tokens`, fetched 2026-11-18T14:25Z). A zero
// maxOutput means the row only corrects the context window; a zero context means
// the existing catalog context already matches the API. Entries the API leaves
// `max_output_tokens: null` are deliberately absent and stay unspecified. It
// accepts t and returns no values.
func TestDeepInfraPublishedOutputAndContextLimits(t *testing.T) {
	t.Parallel()
	catalog := (&deepinfra.Adaptor{}).GetDefaultModelPricing()
	for _, tc := range []struct {
		id        string
		maxOutput int32
		context   int32
	}{
		{"MiniMaxAI/MiniMax-M2.7", 131072, 0},
		{"MiniMaxAI/MiniMax-M2.7-Turbo", 131072, 0},
		{"MiniMaxAI/MiniMax-M3", 131072, 0},
		{"Qwen/Qwen3-Coder-480B-A35B-Instruct-Turbo", 65536, 0},
		{"Qwen/Qwen3.5-122B-A10B", 81920, 0},
		{"Qwen/Qwen3.5-27B", 81920, 0},
		{"Qwen/Qwen3.5-35B-A3B", 81920, 0},
		{"Qwen/Qwen3.5-397B-A17B", 81920, 0},
		{"Qwen/Qwen3.5-9B", 81920, 0},
		{"Qwen/Qwen3.6-27B", 81920, 0},
		{"Qwen/Qwen3.6-35B-A3B", 65536, 0},
		{"Qwen/Qwen3.8-2.4T-A95B", 131072, 0},
		{"Qwen/Qwen3.8-Flash", 131072, 0},
		{"XiaomiMiMo/MiMo-V2.5", 131072, 0},
		{"XiaomiMiMo/MiMo-V2.5-Pro", 131072, 0},
		{"deepseek-ai/DeepSeek-R1-0528", 32768, 0},
		{"deepseek-ai/DeepSeek-V3-0324", 0, 163840},
		{"deepseek-ai/DeepSeek-V3.1", 163840, 0},
		{"deepseek-ai/DeepSeek-V3.2", 163840, 0},
		{"deepseek-ai/DeepSeek-V4-Flash", 393216, 0},
		{"deepseek-ai/DeepSeek-V4-Flash-0731", 384000, 0},
		{"deepseek-ai/DeepSeek-V4-Flash-Vision-Exp", 262144, 0},
		{"deepseek-ai/DeepSeek-V4-Pro", 393216, 0},
		{"deepseek-ai/DeepSeek-V4-Pro-0813", 393216, 0},
		{"deepseek-ai/DeepSeek-V4.1-Flash", 393216, 0},
		{"deepseek-ai/DeepSeek-V4.1-Flash-1017", 393216, 0},
		{"google/gemma-4-31B-it-Ultra", 8192, 0},
		{"google/gemma-4-31B-it-turbo", 16384, 0},
		{"inclusionAI/Ling-3.0-flash", 32768, 0},
		{"inclusionAI/Ling-3.0-flash-VL", 32768, 0},
		{"meta-llama/Llama-3.3-70B-Instruct-Turbo", 16384, 0},
		{"moonshotai/Kimi-K2.5", 64000, 0},
		{"moonshotai/Kimi-K3", 1048576, 0},
		{"moonshotai/Kimi-K3.1-Pro", 943718, 0},
		{"nvidia/NVIDIA-Nemotron-3-Ultra-550B-A55B", 16384, 0},
		{"nvidia/NVIDIA-Nemotron-3.5-Lightning", 32768, 0},
		{"nvidia/Nemotron-3-Nano-30B-A3B", 228000, 0},
		{"openai/gpt-oss-120b", 131072, 0},
		{"openai/gpt-oss-120b-Ultra", 131072, 0},
		{"openai/gpt-oss-20b", 131072, 0},
		{"stepfun-ai/Step-3.7-Flash", 32768, 0},
		{"tencent/Hy3", 131072, 0},
		{"tencent/Hy4-preview", 131072, 0},
		{"thinkingmachines/Inkling", 262144, 524288},
		{"thinkingmachines/Inkling-Small", 262144, 0},
		{"zai-org/GLM-4.6", 131072, 0},
		{"zai-org/GLM-4.7", 131072, 0},
		{"zai-org/GLM-4.7-Flash", 0, 202752},
		{"zai-org/GLM-5", 16384, 202752},
		{"zai-org/GLM-5.1", 65536, 0},
		{"zai-org/GLM-5.2", 163840, 0},
		{"zai-org/GLM-5.3", 131072, 0},
		{"zai-org/GLM-5.3-Flash", 1048576, 0},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, exists := catalog[tc.id]
			require.True(t, exists, "model must stay advertised")
			if tc.maxOutput > 0 {
				require.Equal(t, tc.maxOutput, cfg.MaxOutputTokens)
				require.LessOrEqual(t, cfg.MaxOutputTokens, cfg.ContextLength,
					"published output limit cannot exceed the context window")
			}
			if tc.context > 0 {
				require.Equal(t, tc.context, cfg.ContextLength)
			}
		})
	}
}

// TestDeepInfraDescriptionsMatchPublishedOutputLimits keeps model descriptions
// honest: an entry that carries a published output limit must not claim its
// output limit is unpublished. It accepts t and returns no values.
func TestDeepInfraDescriptionsMatchPublishedOutputLimits(t *testing.T) {
	t.Parallel()
	for id, cfg := range (&deepinfra.Adaptor{}).GetDefaultModelPricing() {
		if cfg.MaxOutputTokens > 0 {
			require.NotContains(t, strings.ToLower(cfg.Description), "unpublished output limit",
				"%s publishes max_output_tokens=%d", id, cfg.MaxOutputTokens)
		}
	}
}
