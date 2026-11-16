package cohere

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestEmbed5DimensionsAndBilling verifies the two published IDs use their own
// tariffs and accept all documented dimensions through the real v2 converter.
// It accepts t and returns no values; no provider requests are made.
func TestEmbed5DimensionsAndBilling(t *testing.T) {
	for id, price := range map[string]float64{"embed-v5.0-pro": 0.12, "embed-v5.0-fast": 0.08} {
		t.Run(id, func(t *testing.T) {
			a := &Adaptor{}
			require.Contains(t, a.GetModelList(), id)
			cfg := a.GetDefaultModelPricing()[id]
			require.InDelta(t, price, a.GetModelRatio(id)/ratio.MilliTokensUsd, 1e-12)
			require.NotNil(t, cfg.Embedding)
			require.InDelta(t, price, cfg.Embedding.TextTokenRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, .40, cfg.Embedding.ImageTokenRatio/ratio.MilliTokensUsd, 1e-12)
			require.EqualValues(t, 128_000, cfg.ContextLength)
			require.Equal(t, []string{"text"}, cfg.InputModalities)
			for _, dimensions := range []int{0, 256, 512, 768, 1024, 1536, 2048} {
				t.Run(strconv.Itoa(dimensions), func(t *testing.T) {
					c, _ := embeddingTestContext(t, `{}`)
					request := &model.GeneralOpenAIRequest{Model: id, Input: []string{"query"}, Dimensions: dimensions}
					converted, err := a.ConvertRequest(c, relaymode.Embeddings, request)
					require.NoError(t, err)
					encoded, err := json.Marshal(converted)
					require.NoError(t, err)
					var wire map[string]any
					require.NoError(t, json.Unmarshal(encoded, &wire))
					require.Equal(t, id, wire["model"])
					require.Equal(t, []any{"query"}, wire["texts"])
					if dimensions == 0 {
						require.NotContains(t, wire, "output_dimension", "retain the provider's default dimension")
					} else {
						require.Equal(t, float64(dimensions), wire["output_dimension"])
					}
					require.Equal(t, dimensions, request.Dimensions)
				})
			}
		})
	}
	for _, tc := range []struct {
		id         string
		dimensions int
	}{
		{"embed-v5.0-pro", 333}, {"embed-v5.0-fast", -1},
		{"embed-v4.0", 768}, {"embed-v4.0", 2048}, {"embed-english-v3.0", 256},
	} {
		_, err := (&Adaptor{}).ConvertRequest(nil, relaymode.Embeddings, &model.GeneralOpenAIRequest{Model: tc.id, Input: "query", Dimensions: tc.dimensions})
		require.Error(t, err, "%s must reject unsupported dimension %d", tc.id, tc.dimensions)
	}
	require.NotSame(t, ModelRatios["embed-v5.0-pro"].Embedding, ModelRatios["embed-v5.0-fast"].Embedding)
}
