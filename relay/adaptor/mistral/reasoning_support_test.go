package mistral

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestReasoningEffortFollowsCatalog verifies compatibility controls are omitted
// for unsupported and unknown models and preserved for every advertised model.
// It uses t and asserts serialized payloads without modifying shared catalogs.
func TestReasoningEffortFollowsCatalog(t *testing.T) {
	t.Parallel()
	models := map[string]bool{
		"codestral-latest": false, "ministral-8b-latest": false,
		"mistral-large-latest": false, "custom-deployment": false, "": false,
	}
	for id, cfg := range ModelRatios {
		if len(cfg.SupportedReasoningEfforts) > 0 {
			models[id] = true
		}
	}
	require.True(t, models["magistral-small-latest"], "cover support outside the five hybrid aliases")
	require.True(t, models["mistral-medium-3-5"])
	for id, supported := range models {
		for _, source := range []string{"chat", "responses", "both", "omitted"} {
			t.Run(id+"/"+source, func(t *testing.T) {
				t.Parallel()
				high, none := "high", "none"
				req := &model.GeneralOpenAIRequest{
					Model: id, Messages: []model.Message{{Role: "user", Content: "Say hello."}},
				}
				if source == "chat" || source == "both" {
					req.ReasoningEffort = &high
				}
				if source == "responses" || source == "both" {
					req.Reasoning = &model.OpenAIResponseReasoning{Effort: &none}
				}
				before, err := json.Marshal(req)
				require.NoError(t, err)
				converted, err := (&Adaptor{}).ConvertRequest(nil, 0, req)
				require.NoError(t, err)
				encoded, err := json.Marshal(converted)
				require.NoError(t, err)
				var wire map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(encoded, &wire))
				require.NotContains(t, wire, "reasoning")
				if supported && source != "omitted" {
					want := high
					if source == "responses" {
						want = none
					}
					require.JSONEq(t, `"`+want+`"`, string(wire["reasoning_effort"]))
				} else {
					require.NotContains(t, wire, "reasoning_effort", "unsupported compatibility effort must not reach upstream")
				}
				if id != "" {
					require.JSONEq(t, `"`+id+`"`, string(wire["model"]), "model IDs remain administrator-controlled")
				}
				require.JSONEq(t, `[{"role":"user","content":"Say hello."}]`, string(wire["messages"]))
				after, err := json.Marshal(req)
				require.NoError(t, err)
				require.JSONEq(t, string(before), string(after), "conversion must not mutate the caller's request")
			})
		}
	}
}
