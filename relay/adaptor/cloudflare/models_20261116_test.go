package cloudflare_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/cloudflare"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestNovemberChatCatalog verifies exact native Workers AI tariffs and reasoning
// controls survive the supported wire contract. It accepts t and returns nothing.
func TestNovemberChatCatalog(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id             string
		cached, output float64
		efforts        []string
	}{
		{"@cf/deepseek-ai/deepseek-v4.1-flash-1017", .006, 1.20, []string{"none", "low", "high", "max"}},
		{"@cf/thinkingmachines/inkling-2-small", .10, 1.00, []string{"none", "minimal", "low", "medium", "high", "max"}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			a := &cloudflare.Adaptor{}
			cfg := a.GetDefaultModelPricing()[tc.id]
			require.Contains(t, a.GetModelList(), tc.id)
			require.InDelta(t, .30, a.GetModelRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.output, a.GetModelRatio(tc.id)*a.GetCompletionRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.EqualValues(t, 1_048_576, cfg.ContextLength)
			require.Zero(t, cfg.MaxOutputTokens)
			require.Equal(t, tc.efforts, cfg.SupportedReasoningEfforts)
			require.Equal(t, "high", cfg.DefaultReasoningEffort)
			for _, effort := range tc.efforts {
				converted, err := a.ConvertRequest(nil, relaymode.ChatCompletions, &model.GeneralOpenAIRequest{
					Model: tc.id, ReasoningEffort: &effort,
					Messages: []model.Message{{Role: "user", Content: "Hello"}},
				})
				require.NoError(t, err)
				encoded, err := json.Marshal(converted)
				require.NoError(t, err)
				var wire map[string]any
				require.NoError(t, json.Unmarshal(encoded, &wire))
				require.Equal(t, effort, wire["reasoning_effort"])
				require.Equal(t, tc.id, wire["model"])
			}
		})
	}
}

// TestEmbeddingGemma2AndRetirementMetadata verifies text embedding parameters
// pass through unchanged and upcoming retirements do not erase historical rates.
// It takes t and returns no values without contacting an upstream account.
func TestEmbeddingGemma2AndRetirementMetadata(t *testing.T) {
	t.Parallel()
	a := &cloudflare.Adaptor{}
	const id = "@cf/google/embeddinggemma-2"
	cfg := a.GetDefaultModelPricing()[id]
	require.Contains(t, a.GetModelList(), id)
	require.EqualValues(t, 8192, cfg.ContextLength)
	require.Equal(t, []string{"text"}, cfg.InputModalities)
	require.NotNil(t, cfg.Embedding)
	require.InDelta(t, .0017, cfg.Embedding.TextTokenRatio/ratio.MilliTokensUsd, 1e-12)
	converted, err := a.ConvertRequest(nil, relaymode.Embeddings, &model.GeneralOpenAIRequest{Model: id, Input: []string{"query"}, Dimensions: 256, EncodingFormat: "base64"})
	require.NoError(t, err)
	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))
	require.Equal(t, id, wire["model"])
	require.Equal(t, float64(256), wire["dimensions"])
	require.Equal(t, "base64", wire["encoding_format"])
	for retired, price := range map[string]float64{
		"@cf/deepseek-ai/deepseek-v4-flash-0731": .44,
		"@cf/deepseek-ai/deepseek-v4-pro-0813":   1.32,
		"@cf/zai-org/glm-5.3-flash":              .15,
	} {
		require.Contains(t, a.GetModelList(), retired)
		require.InDelta(t, price, a.GetModelRatio(retired)/ratio.MilliTokensUsd, 1e-12)
		require.Contains(t, a.GetDefaultModelPricing()[retired].Description, "2026-11-20")
	}
}
