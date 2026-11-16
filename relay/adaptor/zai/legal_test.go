package zai_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/zai"
	"github.com/Laisky/one-api/relay/adaptor/zhipu"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestLegalModelProviderPricingAndWire verifies legal-model discovery, native
// currency prices and the inherited v4 request path using t; it returns nothing.
func TestLegalModelProviderPricingAndWire(t *testing.T) {
	t.Parallel()
	const id = "glm-5.3-flashx-legal"
	for _, tc := range []struct {
		name, baseURL               string
		a                           adaptor.Adaptor
		unit, input, cached, output float64
	}{
		{"bigmodel", "https://open.bigmodel.cn", &zhipu.Adaptor{}, ratio.MilliTokensRmb, 2, .57, 7},
		{"zai", "https://api.z.ai", &zai.Adaptor{}, ratio.MilliTokensUsd, .37, .075, 1.25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, ok := tc.a.GetDefaultModelPricing()[id]
			require.True(t, ok)
			require.Contains(t, tc.a.GetModelList(), id)
			require.InDelta(t, tc.input, tc.a.GetModelRatio(id)/tc.unit, 1e-12)
			require.InDelta(t, tc.cached, cfg.CachedInputRatio/tc.unit, 1e-12)
			require.InDelta(t, tc.output, tc.a.GetModelRatio(id)*tc.a.GetCompletionRatio(id)/tc.unit, 1e-12)
			require.EqualValues(t, 1_000_000, cfg.ContextLength)
			require.EqualValues(t, 131_072, cfg.MaxOutputTokens)
			require.Empty(t, cfg.TimeWindows, "the Flash launch discount must not leak into Legal")
			require.Empty(t, cfg.Tiers)
			require.NotContains(t, cfg.SupportedFeatures, "structured_outputs")
			require.Equal(t, []string{"low", "high", "max"}, cfg.SupportedReasoningEfforts)
			require.Equal(t, "max", cfg.DefaultReasoningEffort)
			for _, effort := range cfg.SupportedReasoningEfforts {
				body := `{"model":"glm-5.3-flashx-legal","messages":[{"role":"user","content":[{"type":"text","text":"Read this document."},{"type":"image_url","image_url":{"url":"https://example.com/document.png"}}]}]}`
				var req model.GeneralOpenAIRequest
				require.NoError(t, json.Unmarshal([]byte(body), &req))
				req.ReasoningEffort = &effort
				converted, err := tc.a.ConvertRequest(nil, relaymode.ChatCompletions, &req)
				require.NoError(t, err)
				encoded, err := json.Marshal(converted)
				require.NoError(t, err)
				var got model.GeneralOpenAIRequest
				require.NoError(t, json.Unmarshal(encoded, &got))
				require.Equal(t, id, got.Model)
				require.Equal(t, req.Messages, got.Messages)
				require.Equal(t, &effort, got.ReasoningEffort)
			}
			url, err := tc.a.GetRequestURL(&meta.Meta{ActualModelName: id, BaseURL: tc.baseURL, Mode: relaymode.ChatCompletions})
			require.NoError(t, err)
			require.Equal(t, tc.baseURL+"/api/paas/v4/chat/completions", url)
			cloned := cfg.Clone()
			cloned.InputModalities[0] = "changed"
			cloned.SupportedReasoningEfforts[0] = "changed"
			require.Equal(t, "text", tc.a.GetDefaultModelPricing()[id].InputModalities[0])
			require.Equal(t, "low", tc.a.GetDefaultModelPricing()[id].SupportedReasoningEfforts[0])
		})
	}
}
