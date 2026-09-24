package mistral_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/mistral"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
)

// TestCurrentMistralCachePricing verifies published rates through public adaptor
// methods using t; it returns nothing. Ordinary input/output tariffs are pinned
// independently to prevent a cache update from changing uncached billing.
func TestCurrentMistralCachePricing(t *testing.T) {
	t.Parallel()
	for id, prices := range map[string][3]float64{
		"mistral-medium-latest": {1.5, 0.15, 7.5},
		"mistral-medium-2604":   {1.5, 0.15, 7.5},
		"mistral-medium-3-5":    {1.5, 0.15, 7.5},
		"mistral-large-latest":  {0.5, 0.05, 1.5},
		"mistral-large-2512":    {0.5, 0.05, 1.5},
		"mistral-small-latest":  {0.15, 0.015, 0.6},
		"mistral-small-2603":    {0.15, 0.015, 0.6},
		"ministral-14b-2512":    {0.2, 0.02, 0.2},
		"ministral-8b-latest":   {0.15, 0.015, 0.15},
		"ministral-8b-2512":     {0.15, 0.015, 0.15},
		"ministral-3b-latest":   {0.1, 0.01, 0.1},
		"ministral-3b-2512":     {0.1, 0.01, 0.1},
		"codestral-latest":     {0.3, 0.03, 0.9},
		"codestral-2508":       {0.3, 0.03, 0.9},
		"codestral-embed-2505":  {0.15, 0.015, 0.15},
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			a := &mistral.Adaptor{}
			cfg, exists := a.GetDefaultModelPricing()[id]
			require.True(t, exists)
			require.InDelta(t, prices[0], a.GetModelRatio(id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, prices[1], cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, prices[2], a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensUsd, 1e-12)
		})
	}
}

// TestNewMistralChatModels verifies discovery, pricing, and preservation of model
// IDs and tool payloads through conversion using t; it returns nothing.
func TestNewMistralChatModels(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"zai-glm-5-2", "zai-glm-5-3", "labs-leanstral-1-5"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			a := &mistral.Adaptor{}
			require.Contains(t, a.GetModelList(), id)
			require.Contains(t, mistral.ModelList, id, "legacy discovery must also be rebuilt")
			cfg, exists := a.GetDefaultModelPricing()[id]
			require.True(t, exists)
			require.EqualValues(t, 128_000, cfg.MaxOutputTokens)
			require.Empty(t, cfg.SupportedReasoningEfforts)
			require.Equal(t, []string{"text"}, cfg.InputModalities)
			if id == "labs-leanstral-1-5" {
				require.Zero(t, a.GetModelRatio(id))
				require.EqualValues(t, 262_144, cfg.ContextLength)
			} else {
				require.InDelta(t, 1.4, a.GetModelRatio(id)/ratio.MilliTokensUsd, 1e-12)
				require.InDelta(t, 4.4, a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensUsd, 1e-12)
				require.InDelta(t, 0.14, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
				require.EqualValues(t, 1_000_000, cfg.ContextLength)
			}
			const input = `{"messages":[{"role":"user","content":"Return one fact."}],"tools":[{"type":"function","function":{"name":"record_fact","parameters":{"type":"object","properties":{"fact":{"type":"string"}}}}}],"tool_choice":"auto"}`
			var req model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal([]byte(input), &req))
			req.Model = id
			converted, err := a.ConvertRequest(nil, 0, &req)
			require.NoError(t, err)
			encoded, err := json.Marshal(converted)
			require.NoError(t, err)
			var want, got map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(input), &want))
			require.NoError(t, json.Unmarshal(encoded, &got))
			for _, key := range []string{"messages", "tools", "tool_choice"} {
				require.JSONEq(t, string(want[key]), string(got[key]), key)
			}
			var actualID string
			require.NoError(t, json.Unmarshal(got["model"], &actualID))
			require.Equal(t, id, actualID)
		})
	}
}
