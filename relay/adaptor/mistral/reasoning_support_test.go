package mistral

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestReasoningEffortFollowsCatalog verifies that caller efforts reach the wire
// only when the target model's catalog entry lists them: values are trimmed and
// lowercased, unsupported values (including none on Mistral Large 4) and every
// value for unsupported or unknown models are omitted so the provider default
// applies. It uses t, asserts serialized payloads, and never mutates catalogs.
func TestReasoningEffortFollowsCatalog(t *testing.T) {
	t.Parallel()
	// Pinned anchors guard against catalog-driven expectations silently drifting:
	// "" means the field must be omitted.
	anchors := map[string]map[string]string{
		"mistral-large-latest":   {"none": "", "minimal": "", "low": "low", "medium": "medium", "HIGH": "high", "xhigh": ""},
		"mistral-large-4":        {"none": "", "low": "low", " High ": "high", "max": ""},
		"mistral-small-latest":   {"none": "none", "NONE": "none", "high": "high", "low": "", "medium": ""},
		"mistral-medium-3-5":     {"none": "none", "high": "high", "medium": ""},
		"magistral-small-latest": {"none": "", "low": "low", "medium": "medium", "high": "high"},
		"zai-glm-5-3":            {"none": "", "low": "low", "max": "max", "medium": ""},
		"codestral-latest":       {"none": "", "high": ""},
		"ministral-8b-latest":    {"none": "", "high": ""},
		"custom-deployment":      {"none": "", "high": ""},
		"":                       {"none": "", "high": ""},
	}
	for id, cfg := range ModelRatios {
		if _, pinned := anchors[id]; pinned {
			continue
		}
		expected := map[string]string{"none": "", "minimal": "", "low": "", "medium": "", "high": "", "xhigh": "", "max": ""}
		for _, effort := range cfg.SupportedReasoningEfforts {
			expected[effort] = effort
		}
		anchors[id] = expected
	}
	for id, efforts := range anchors {
		for effort, want := range efforts {
			for _, source := range []string{"chat", "responses", "both", "omitted"} {
				t.Run(id+"/"+effort+"/"+source, func(t *testing.T) {
					t.Parallel()
					ignored := "ignored-responses-effort"
					req := &model.GeneralOpenAIRequest{
						Model: id, Messages: []model.Message{{Role: "user", Content: "Say hello."}},
					}
					switch source {
					case "chat":
						req.ReasoningEffort = &effort
					case "responses":
						req.Reasoning = &model.OpenAIResponseReasoning{Effort: &effort}
					case "both":
						// The Chat field takes precedence over the Responses fallback.
						req.ReasoningEffort = &effort
						req.Reasoning = &model.OpenAIResponseReasoning{Effort: &ignored}
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
					if want != "" && source != "omitted" {
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
}
