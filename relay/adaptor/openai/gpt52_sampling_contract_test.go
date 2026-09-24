package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	relaymeta "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGPT52ReasoningCatalogContract checks the documented alias and snapshot
// contract without broadening the independent Pro, Codex, or GPT-5 contracts.
func TestGPT52ReasoningCatalogContract(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gpt-5.2", "gpt-5.2-2025-12-11"} {
		t.Run(name, func(t *testing.T) {
			cfg, ok := ModelRatios[name]
			require.True(t, ok)
			require.Equal(t, []string{"none", "low", "medium", "high", "xhigh"}, cfg.SupportedReasoningEfforts)
			require.Equal(t, "none", cfg.DefaultReasoningEffort)
			require.Equal(t, "none", *normalizeReasoningEffortForModel(name, nil))
		})
	}
	for _, name := range []string{"gpt-5.2-pro", "gpt-5.2-pro-2025-12-11", "gpt-5.2-codex", "gpt-5"} {
		none := "none"
		require.False(t, modelSupportsSampling(name, &none), name)
	}
}

// TestGPT52MappedSamplingContract verifies real Chat transformations and typed
// Responses serialization using the final upstream alias or snapshot. Explicit
// zero sampling survives default/none/legacy-minimal effort, but not reasoning.
func TestGPT52MappedSamplingContract(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gpt-5.2", "gpt-5.2-2025-12-11"} {
		for _, effort := range []string{"", "none", "minimal", "low", "medium", "high", "xhigh"} {
			t.Run(name+"/"+effort, func(t *testing.T) {
				temperature, topP := 0.0, 0.8
				request := &model.GeneralOpenAIRequest{
					Model: "tenant-alias", Temperature: &temperature, TopP: &topP,
					Messages: []model.Message{{Role: "user", Content: "hello"}},
				}
				if effort != "" {
					requested := effort
					request.ReasoningEffort = &requested
				}
				info := &relaymeta.Meta{ChannelType: channeltype.OpenAI, Mode: relaymode.ChatCompletions, ActualModelName: name}
				a := &Adaptor{ChannelType: channeltype.OpenAI}
				require.NoError(t, a.applyRequestTransformations(info, request))
				allowSampling := effort == "" || effort == "none" || effort == "minimal"
				if allowSampling {
					require.Equal(t, "none", *request.ReasoningEffort)
					require.NotNil(t, request.Temperature)
					require.Equal(t, 0.0, *request.Temperature)
					require.NotNil(t, request.TopP)
				} else {
					require.Nil(t, request.Temperature)
					require.Nil(t, request.TopP)
					require.Equal(t, effort, *request.ReasoningEffort)
				}
				var responseRequest ResponseAPIRequest
				require.NoError(t, json.Unmarshal([]byte(`{"model":"`+name+`","input":"hello","temperature":0,"top_p":0.8}`), &responseRequest))
				if effort != "" {
					require.NoError(t, json.Unmarshal([]byte(`{"reasoning":{"effort":"`+effort+`"}}`), &responseRequest))
				}
				wire, err := json.Marshal(responseRequest)
				require.NoError(t, err)
				var root map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(wire, &root))
				if allowSampling {
					require.Equal(t, "0", string(root["temperature"]))
					require.Equal(t, "0.8", string(root["top_p"]))
				} else {
					require.NotContains(t, root, "temperature")
					require.NotContains(t, root, "top_p")
				}
			})
		}
	}
}
