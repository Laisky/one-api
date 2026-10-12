package cloudflare_test

import (
	"encoding/json"
	"github.com/Laisky/one-api/relay/adaptor/cloudflare"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberCloudflareControls verifies public limits, accepted effort values,
// and independently pinned host prices using t. It returns nothing.
func TestOctoberCloudflareControls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id                    string
		context               int32
		efforts               []string
		defaultEffort         string
		input, cached, output float64
	}{
		{"@cf/zai-org/glm-5.2", 262144, []string{"none", "minimal", "low", "medium", "high", "max", "xhigh"}, "max", 1.4, .26, 4.4},
		{"@cf/zai-org/glm-5.3", 1048576, []string{"low", "high", "max"}, "max", 1.4, .26, 4.4},
		{"@cf/zai-org/glm-5.3-flash", 1048576, []string{"low", "high", "max"}, "max", .15, .03, .5},
		{"@cf/moonshotai/kimi-k2.6", 262144, []string{"none", "high", "low", "medium", "max"}, "high", .95, .16, 4},
		{"@cf/moonshotai/kimi-k2.7-code", 262144, nil, "", .95, .19, 4},
		{"@cf/nvidia/nemotron-3-120b-a12b", 256000, nil, "", .5, 0, 1.5},
		{"@cf/google/gemma-4-26b-a4b-it", 256000, nil, "", .1, 0, .3},
		{"@cf/zai-org/glm-4.7-flash", 131072, nil, "", .060, 0, .4},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			a := &cloudflare.Adaptor{}
			cfg, ok := a.GetDefaultModelPricing()[tc.id]
			require.True(t, ok)
			require.Contains(t, a.GetModelList(), tc.id)
			require.Equal(t, tc.context, cfg.ContextLength)
			require.Equal(t, tc.efforts, cfg.SupportedReasoningEfforts)
			require.Equal(t, tc.defaultEffort, cfg.DefaultReasoningEffort)
			require.Zero(t, cfg.MaxOutputTokens, "undocumented output limits stay unspecified")
			require.Zero(t, cfg.MaxTokens, "metadata is not an admission restriction")
			require.InDelta(t, tc.input, a.GetModelRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.output, a.GetModelRatio(tc.id)*a.GetCompletionRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			for _, effort := range tc.efforts {
				req := &model.GeneralOpenAIRequest{Model: tc.id, Messages: []model.Message{{Role: "user", Content: "Test"}}, ReasoningEffort: &effort}
				before, err := json.Marshal(req)
				require.NoError(t, err)
				converted, err := a.ConvertRequest(nil, relaymode.ChatCompletions, req)
				require.NoError(t, err)
				after, err := json.Marshal(converted)
				require.NoError(t, err)
				require.JSONEq(t, string(before), string(after), "preserve requested effort; upstream handles aliases")
			}
		})
	}
}
