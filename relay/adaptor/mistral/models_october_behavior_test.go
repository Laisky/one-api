package mistral_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/mistral"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestMistralOctoberReasoningWire verifies model-specific effort controls reach
// the wire through Chat and Responses conversion without mutating caller input.
// It accepts the test handle t and returns no values.
func TestMistralOctoberReasoningWire(t *testing.T) {
	t.Parallel()
	for id, efforts := range map[string][]string{
		"zai-glm-5-3":       {"low", "high", "max"},
		"mistral-large-4":   {"none", "high"},
		"mistral-large-4-0": {"none", "high"},
	} {
		for _, effort := range efforts {
			for _, source := range []string{"chat", "responses"} {
				t.Run(id+"/"+effort+"/"+source, func(t *testing.T) {
					t.Parallel()
					a := &mistral.Adaptor{}
					cfg, ok := a.GetDefaultModelPricing()[id]
					require.True(t, ok)
					require.Equal(t, efforts, cfg.SupportedReasoningEfforts)
					require.Contains(t, cfg.SupportedFeatures, "reasoning")
					require.Contains(t, a.GetModelList(), id)
					require.Contains(t, mistral.ModelList, id)
					const input = `{"messages":[{"role":"user","content":"Return one fact."}],"tools":[{"type":"function","function":{"name":"record","parameters":{"type":"object"}}}],"tool_choice":"auto","seed":42,"max_completion_tokens":777}`
					var req model.GeneralOpenAIRequest
					require.NoError(t, json.Unmarshal([]byte(input), &req))
					req.Model = id
					if source == "chat" {
						req.ReasoningEffort = &effort
					} else {
						req.Reasoning = &model.OpenAIResponseReasoning{Effort: &effort}
					}
					before, err := json.Marshal(req)
					require.NoError(t, err)
					converted, err := a.ConvertRequest(nil, 0, &req)
					require.NoError(t, err)
					encoded, err := json.Marshal(converted)
					require.NoError(t, err)
					var wire map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(encoded, &wire))
					require.JSONEq(t, `"`+effort+`"`, string(wire["reasoning_effort"]))
					require.JSONEq(t, `"`+id+`"`, string(wire["model"]))
					require.JSONEq(t, `777`, string(wire["max_tokens"]))
					require.JSONEq(t, `42`, string(wire["random_seed"]))
					require.Contains(t, string(wire["tools"]), "record")
					require.NotContains(t, wire, "reasoning")
					after, err := json.Marshal(req)
					require.NoError(t, err)
					require.JSONEq(t, string(before), string(after))
				})
			}
		}
	}
}

// TestMistralLarge4LaunchWindow verifies absolute prices at both UTC boundaries
// and alias ownership through the production pricing resolver. It returns nothing.
func TestMistralLarge4LaunchWindow(t *testing.T) {
	t.Parallel()
	a := &mistral.Adaptor{}
	for _, id := range []string{"mistral-large-4", "mistral-large-4-0"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			cfg, ok := a.GetDefaultModelPricing()[id]
			require.True(t, ok)
			require.EqualValues(t, 1_000_000, cfg.ContextLength)
			require.Zero(t, cfg.MaxOutputTokens, "do not invent a maximum output limit")
			require.Zero(t, cfg.MaxTokens, "catalogue metadata is not a request admission cap")
			require.Equal(t, []string{"text", "image"}, cfg.InputModalities)
			require.Empty(t, cfg.HuggingFaceID, "announced weights are not released weights")
			for _, tc := range []struct {
				at                    string
				input, cached, output float64
			}{
				{"2026-10-05T23:59:59.999999999Z", 1.36, 0.14, 4.18},
				{"2026-10-06T00:00:00Z", 0.68, 0.07, 2.09},
				{"2026-10-12T12:00:00Z", 0.68, 0.07, 2.09},
				{"2026-10-19T23:59:59.999999999Z", 0.68, 0.07, 2.09},
				{"2026-10-20T00:00:00Z", 1.36, 0.14, 4.18},
				{"2026-10-19T20:00:00-04:00", 1.36, 0.14, 4.18},
			} {
				at, err := time.Parse(time.RFC3339Nano, tc.at)
				require.NoError(t, err)
				resolved := pricing.ApplyTimeWindow(cfg, at)
				require.InDelta(t, tc.input, resolved.Ratio/ratio.MilliTokensUsd, 1e-12)
				require.InDelta(t, tc.cached, resolved.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
				require.InDelta(t, tc.output, resolved.Ratio*resolved.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
			}
		})
	}
	one, two := a.GetDefaultModelPricing()["mistral-large-4"], a.GetDefaultModelPricing()["mistral-large-4-0"]
	require.NotEmpty(t, one.TimeWindows)
	require.NotEmpty(t, two.TimeWindows)
	require.NotSame(t, &one.TimeWindows[0], &two.TimeWindows[0])
	require.NotSame(t, &one.InputModalities[0], &two.InputModalities[0])
	legacy := a.GetDefaultModelPricing()["mistral-large-latest"]
	require.InDelta(t, 0.5, legacy.Ratio/ratio.MilliTokensUsd, 1e-12, "do not infer an undocumented latest-alias migration")
}
