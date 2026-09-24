package cohere_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/cohere"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// TestNorthCatalogAndChat verifies discovery, free API rates, and native v1 chat
// conversion with t; it returns nothing and makes no live provider requests.
func TestNorthCatalogAndChat(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id              string
		context, output int32
	}{
		{"north-mini-code-1-0", 256000, 64000},
		{"north-small-translate-1-0", 16000, 16000},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			a := &cohere.Adaptor{}
			require.Contains(t, a.GetModelList(), tc.id)
			cfg, exists := a.GetDefaultModelPricing()[tc.id]
			require.True(t, exists)
			require.Zero(t, a.GetModelRatio(tc.id))
			require.Equal(t, tc.context, cfg.ContextLength)
			require.Equal(t, tc.output, cfg.MaxOutputTokens)
			require.Equal(t, []string{"text"}, cfg.InputModalities)
			require.Empty(t, cfg.SupportedFeatures, "do not advertise controls dropped by the native converter")
			require.Empty(t, cfg.SupportedReasoningEfforts)
			require.Contains(t, cfg.Description, "rate limits")
			converted, err := a.ConvertRequest(nil, 0, &model.GeneralOpenAIRequest{
				Model: tc.id, MaxTokens: 123,
				Messages: []model.Message{{Role: "user", Content: "Translate hello to French."}},
			})
			require.NoError(t, err)
			encoded, err := json.Marshal(converted)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(encoded, &payload))
			require.Equal(t, tc.id, payload["model"])
			require.Equal(t, "Translate hello to French.", payload["message"])
			require.Equal(t, float64(123), payload["max_tokens"])
			url, err := a.GetRequestURL(&meta.Meta{BaseURL: "https://api.cohere.com", ActualModelName: tc.id})
			require.NoError(t, err)
			require.Equal(t, "https://api.cohere.com/v1/chat", url)
		})
	}
}
