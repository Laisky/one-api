package openrouter_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/stretchr/testify/require"
)

// TestOctoberOpenRouterPublishedRates checks host-specific token defaults through
// both public channel surfaces using t. It returns nothing and performs no I/O.
func TestOctoberOpenRouterPublishedRates(t *testing.T) {
	t.Parallel()
	a := &openrouter.Adaptor{}
	generic := &openai.Adaptor{ChannelType: channeltype.OpenRouter}
	for _, tc := range []struct {
		id                    string
		input, cached, output float64
	}{
		{"meta/muse-wonder", 4, 0.2, 20},
		{"stepfun/step-5-preview", 1, 0.05, 2.7},
		{"anthropic/claude-haiku-5.5", 0.1, 0.01, 0.5},
		{"mistralai/mistral-large-4-0", 0.68, 0.07, 2.09},
		{"inclusionai/ling-3.1-flash", 0, 0, 0},
		{"apodex/apodex-1.1-mini:free", 0, 0, 0},
		{"unbiased/pareto-26.10-preview", 0.8, 0.03, 3.2},
		{"anthropic/claude-sonnet-5.5", 2, 0.1, 10},
		{"perceptron/perceptron-mk1.5", 0.15, 0, 1.5},
		{"fireworks/ember-1", 3, 0.3, 15},
		{"x-ai/grok-4.7", 2, 0.5, 6},
		{"prism-ml/ternary-bonsai-2-27b", 0.075, 0.0375, 0.5},
		{"~deepseek/deepseek-pro-latest", 0.24948, 0.007938, 0.74844},
		{"~deepseek/deepseek-flash-latest", 0.0336, 0.0336, 1.188},
		{"~openai/gpt-sol-latest", 2, 0.1, 10},
		{"inclusionai/ling-3.0-flash-vl", 0.021, 0.0042, 0.0616},
		{"deepseek/deepseek-v4.1-flash", 0.15, 0.003, 0.6},
		{"inclusionai/ling-3.0-flash-sante", 0.042, 0.0084, 0.1232},
		{"google/gemini-3.8-flash", 0.75, 0.075, 3.75},
		{"tencent/hy4-preview", 0.7506, 0.0378, 2.2509},
		{"inclusionai/ling-3.0-flash-fin", 0.042, 0.0084, 0.1232},
		{"~z-ai/glm-flash-latest", 0.0232, 0.0232, 1.2},
		{"z-ai/glm-5.3-flash", 0.0232, 0.0232, 1.2},
		{"deepseek/deepseek-v4-flash-vision-exp", 0.2156, 0.00686, 0.6468},
		{"~z-ai/glm-latest", 0.055, 0.06, 5},
		{"z-ai/glm-5.3", 0.079, 0.059, 4.8},
		{"qwen/qwen3.8-27b", 0.04, 0.0198, 2.3},
		{"google/gemini-3.7-flash", 0.75, 0.075, 3.75},
		{"deepseek/deepseek-v4-pro-0813", 0.66, 0.022, 1.98},
		{"nvidia/nemotron-3.5-lightning", 0.07, 0.04, 0.2},
		{"~deepseek/deepseek-v4-flash-latest", 0.0128, 0.0128, 0.768},
		{"deepseek/deepseek-v4-flash-0731", 0.016, 0.016, 0.64},
		{"google/gemini-3.6-flash", 0.75, 0.075, 3.75},
		{"google/gemini-3.5-flash-lite", 0.3, 0.03, 2.5},
		{"moonshotai/kimi-k3", 0.73, 0.55, 10},
		{"~x-ai/grok-latest", 2, 0.5, 6},
		{"tencent/hy3", 0.0825, 0.020625, 0.33},
		{"z-ai/glm-5.2", 0.06, 0.059, 7},
		{"moonshotai/kimi-k2.7-code", 0.6712, 0.18, 3.35},
		{"~anthropic/claude-haiku-latest", 0.1, 0.01, 0.5},
		{"~google/gemini-pro-latest", 2, 0.2, 12},
		{"~moonshotai/kimi-latest", 0.72, 0.3, 24},
		{"~google/gemini-flash-latest", 0.75, 0.075, 3.75},
		{"~anthropic/claude-sonnet-latest", 2, 0.1, 10},
		{"qwen/qwen3.6-35b-a3b", 0.1, 0.1, 0.95},
		{"qwen/qwen3.6-27b", 0.32, 0, 3.2},
		{"deepseek/deepseek-v4-pro", 0.41238, 0.034365, 0.82476},
		{"deepseek/deepseek-v4-flash", 0.03, 0.03, 1.28},
		{"moonshotai/kimi-k2.6", 0.65, 0.15, 3.41},
		{"minimax/minimax-m2.7", 0.21, 0.042, 0.84},
		{"qwen/qwen3.5-35b-a3b", 0.08, 0.04, 0.75},
		{"qwen/qwen3.5-27b", 0.26, 0, 2.6},
		{"google/gemini-3.1-pro-preview-customtools", 2, 0.2, 12},
		{"google/gemini-3.1-pro-preview", 2, 0.2, 12},
		{"moonshotai/kimi-k2.5", 0.5, 0.15, 2.5},
		{"z-ai/glm-4.7", 0.6, 0.11, 2.2},
		{"nvidia/nemotron-3-nano-30b-a3b", 0.06, 0, 0.24},
		{"deepseek/deepseek-v3.2", 0.259, 0.14, 0.8},
		{"qwen/qwen3-vl-8b-instruct", 0.25, 0.12, 0.75},
		{"qwen/qwen3-vl-30b-a3b-thinking", 0.29, 0, 1},
		{"qwen/qwen3-vl-30b-a3b-instruct", 0.15, 0, 0.6},
		{"z-ai/glm-4.6", 0.5, 0.1, 2},
		{"openai/gpt-oss-120b", 0.037, 0, 0.17},
		{"openai/gpt-oss-20b", 0.018, 0.009, 0.09},
		{"qwen/qwen3-30b-a3b-instruct-2507", 0.1, 0, 0.3},
		{"qwen/qwen3-235b-a22b-thinking-2507", 0.45, 0, 3.5},
		{"qwen/qwen3-235b-a22b-2507", 0.09, 0, 0.55},
		{"minimax/minimax-m1", 0.55, 0, 2.2},
		{"google/gemini-2.5-pro", 1.25, 0.125, 10},
		{"google/gemini-2.5-pro-preview", 1.25, 0.125, 10},
		{"meta-llama/llama-4-maverick", 0.1875, 0.05, 0.6525},
		{"meta-llama/llama-4-scout", 0.18, 0, 0.59},
		{"deepseek/deepseek-chat-v3-0324", 0.29, 0.11, 1.14},
		{"meta-llama/llama-3.3-70b-instruct", 0.22, 0.11, 0.5},
		{"mistralai/mistral-nemo", 0.029, 0, 0.03},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, exists := a.GetDefaultModelPricing()[tc.id]
			require.True(t, exists)
			require.Contains(t, a.GetModelList(), tc.id)
			require.Contains(t, generic.GetModelList(), tc.id)
			require.Equal(t, cfg, generic.GetDefaultModelPricing()[tc.id])
			require.InDelta(t, tc.input, cfg.Ratio/ratio.MilliTokensUsd, 1e-10)
			require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-10)
			require.InDelta(t, tc.output, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-10)
			require.NotContains(t, a.GetModelList(), tc.id+":batch")
		})
	}
}

// TestOctoberOpenRouterHaikuTier pins every Haiku tariff on either side of the
// inclusive host threshold. It accepts t and returns no values.
func TestOctoberOpenRouterHaikuTier(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"anthropic/claude-haiku-5.5", "~anthropic/claude-haiku-latest"} {
		for _, tokens := range []int{0, 99999, 100000, 100001, 500000} {
			got := pricing.ResolveEffectivePricingForUsage(id, tokens, 1, &openrouter.Adaptor{})
			input, cached, output, write5m, write1h := 0.1, 0.01, 0.5, 0.125, 0.2
			threshold := 0
			if tokens >= 100000 {
				input, cached, output, write5m, write1h, threshold = 0.5, 0.05, 2.5, 0.625, 1, 100000
			}
			require.Equal(t, threshold, got.AppliedTierThreshold)
			require.InDelta(t, input, got.InputRatio/ratio.MilliTokensUsd, 1e-10)
			require.InDelta(t, cached, got.CachedInputRatio/ratio.MilliTokensUsd, 1e-10)
			require.InDelta(t, output, got.OutputRatio/ratio.MilliTokensUsd, 1e-10)
			require.InDelta(t, write5m, got.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-10)
			require.InDelta(t, write1h, got.CacheWrite1hRatio/ratio.MilliTokensUsd, 1e-10)
		}
	}
}

// TestOctoberOpenRouterClockRates checks UTC boundaries and timezone invariance
// for the advertised DeepSeek and Tencent schedules using t. It returns nothing.
func TestOctoberOpenRouterClockRates(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id, at                string
		input, cached, output float64
	}{
		{"deepseek/deepseek-v4.1-flash", "2026-10-12T00:59:59Z", 0.15, 0.003, 0.6},
		{"deepseek/deepseek-v4.1-flash", "2026-10-12T01:00:00Z", 0.30, 0.006, 1.2},
		{"deepseek/deepseek-v4.1-flash", "2026-10-12T04:00:00Z", 0.15, 0.003, 0.6},
		{"deepseek/deepseek-v4-pro-0813", "2026-10-12T06:00:00Z", 1.32, 0.044, 3.96},
		{"deepseek/deepseek-v4-pro-0813", "2026-10-12T10:00:00Z", 0.66, 0.022, 1.98},
		{"deepseek/deepseek-v4-pro-0813", "2026-10-11T06:00:00Z", 0.66, 0.022, 1.98},
		{"tencent/hy3", "2026-10-12T15:59:59Z", 0.132, 0.033, 0.528},
		{"tencent/hy3", "2026-10-12T16:00:00Z", 0.0825, 0.020625, 0.33},
		{"tencent/hy4-preview", "2026-10-12T15:59:59Z", 0.834, 0.042, 2.501},
		{"tencent/hy4-preview", "2026-10-12T16:00:00Z", 0.7506, 0.0378, 2.2509},
	} {
		at, err := time.Parse(time.RFC3339, tc.at)
		require.NoError(t, err)
		base := openrouter.ModelRatios[tc.id]
		got := pricing.ApplyTimeWindow(base, at)
		require.Equal(t, got, pricing.ApplyTimeWindow(base, at.In(time.FixedZone("offset", -14400))))
		require.InDelta(t, tc.input, got.Ratio/ratio.MilliTokensUsd, 1e-10)
		require.InDelta(t, tc.cached, got.CachedInputRatio/ratio.MilliTokensUsd, 1e-10)
		require.InDelta(t, tc.output, got.Ratio*got.CompletionRatio/ratio.MilliTokensUsd, 1e-10)
	}
}

// TestOctoberOpenRouterRequestPreservation verifies new IDs, text, tools and
// reasoning survive conversion. It accepts t and returns nothing without I/O.
func TestOctoberOpenRouterRequestPreservation(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"meta/muse-wonder", "stepfun/step-5-preview", "anthropic/claude-haiku-5.5", "mistralai/mistral-large-4-0"} {
		var request model.GeneralOpenAIRequest
		body := `{"model":"` + id + `","messages":[{"role":"user","content":"Check availability."}],"reasoning_effort":"high","tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}],"tool_choice":"auto"}`
		require.NoError(t, json.Unmarshal([]byte(body), &request))
		before, err := json.Marshal(&request)
		require.NoError(t, err)
		converted, err := (&openrouter.Adaptor{}).ConvertRequest(nil, 0, &request)
		require.NoError(t, err)
		after, err := json.Marshal(converted)
		require.NoError(t, err)
		require.JSONEq(t, string(before), string(after))
	}
	cfg := openrouter.ModelRatios["google/gemini-3.8-flash"]
	require.InDelta(t, 0.75+0.0416666666666667, cfg.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-12)
	require.NotNil(t, cfg.Audio)
	require.InDelta(t, 0.75, cfg.Ratio*cfg.Audio.PromptRatio/ratio.MilliTokensUsd, 1e-12)
}

// TestOctoberOpenRouterGoogleWriteTier verifies explicit cache storage is added
// to each tier's input price. It accepts t and returns no values.
func TestOctoberOpenRouterGoogleWriteTier(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"google/gemini-2.5-pro", "google/gemini-2.5-pro-preview"} {
		for _, tokens := range []int{199999, 200000, 200001} {
			got := pricing.ResolveEffectivePricingForUsage(id, tokens, 1, &openrouter.Adaptor{})
			want := 1.25 + 0.375
			if tokens >= 200000 {
				want = 2.5 + 0.375
			}
			require.InDelta(t, want, got.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-12)
		}
	}
}
