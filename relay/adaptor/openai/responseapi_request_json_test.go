package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestResponseAPIRequestMarshalJSONNormalizesReasoningEffort verifies native Responses payloads use canonical GPT-5.6 efforts.
func TestResponseAPIRequestMarshalJSONNormalizesReasoningEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		model          string
		effort         string
		expectedEffort string
	}{
		{name: "legacy GPT-5.6 effort", model: " GPT-5.6 ", effort: "minimal", expectedEffort: "none"},
		{name: "legacy Daybreak effort", model: "gpt-daybreak-blue-latest", effort: "minimal", expectedEffort: "none"},
		{name: "canonical max effort", model: "gpt-5.6", effort: "max", expectedEffort: "max"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			effort := test.effort
			request := ResponseAPIRequest{
				Model: test.model,
				Reasoning: &relaymodel.OpenAIResponseReasoning{
					Effort: &effort,
				},
			}

			payload, err := json.Marshal(request)
			require.NoError(t, err)

			var encoded struct {
				Reasoning *struct {
					Effort *string `json:"effort"`
				} `json:"reasoning"`
			}
			require.NoError(t, json.Unmarshal(payload, &encoded))
			require.NotNil(t, encoded.Reasoning)
			require.NotNil(t, encoded.Reasoning.Effort)
			require.Equal(t, test.expectedEffort, *encoded.Reasoning.Effort)
			require.Equal(t, test.effort, effort, "MarshalJSON must not mutate the request")
		})
	}
}
