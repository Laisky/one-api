package fireworks_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/fireworks"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestSeptemberCatalog verifies published identifiers, tariffs and serialized
// multimodal tool requests using t. It returns nothing and makes no paid calls.
func TestSeptemberCatalog(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id                    string
		input, cached, output float64
		context               int32
		vision                bool
	}{
		{"accounts/fireworks/models/ember-1", 3, 0.3, 15, 1048576, true},
		{"accounts/fireworks/models/deepseek-v4p1-flash", 0.22, 0.007, 0.66, 1048576, true},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			a := &fireworks.Adaptor{}
			require.Contains(t, a.GetModelList(), tc.id)
			cfg, ok := a.GetDefaultModelPricing()[tc.id]
			require.True(t, ok)
			require.InDelta(t, tc.input, a.GetModelRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.output, a.GetModelRatio(tc.id)*a.GetCompletionRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.Equal(t, tc.context, cfg.ContextLength)
			require.Zero(t, cfg.MaxOutputTokens)
			require.Zero(t, cfg.MaxTokens)
			require.Empty(t, cfg.SupportedReasoningEfforts)
			if tc.vision {
				require.Contains(t, cfg.InputModalities, "image")
			} else {
				require.Equal(t, []string{"text"}, cfg.InputModalities)
			}
			const input = `{"messages":[{"role":"user","content":[{"type":"text","text":"Describe it"},{"type":"image_url","image_url":{"url":"https://example.com/image.png"}}]}],"tools":[{"type":"function","function":{"name":"save","parameters":{"type":"object","properties":{}}}}],"tool_choice":"auto"}`
			var req model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal([]byte(input), &req))
			req.Model = tc.id
			if !tc.vision {
				req.Messages = []model.Message{{Role: "user", Content: "Describe it"}}
			}
			want, err := json.Marshal(req)
			require.NoError(t, err)
			converted, err := a.ConvertRequest(nil, relaymode.ChatCompletions, &req)
			require.NoError(t, err)
			got, err := json.Marshal(converted)
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
		})
	}
}
