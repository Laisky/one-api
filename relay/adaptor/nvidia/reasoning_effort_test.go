package nvidia

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// globalReasoningEfforts is the union vocabulary the shared request binding
// accepts; any member a hosted model does not list must not reach NVIDIA.
var globalReasoningEfforts = []string{"none", "default", "minimal", "low", "medium", "high", "xhigh", "max", "ultra", "extreme"}

// TestConvertRequestReasoningEffortFollowsCatalog walks every NVIDIA catalog
// model that publishes an effort vocabulary and asserts the serialized wire:
// listed values (including case variants) survive in canonical form, every
// other value is omitted, and a Responses-style reasoning object is lowered
// into reasoning_effort and never forwarded. It accepts t and returns no values.
func TestConvertRequestReasoningEffortFollowsCatalog(t *testing.T) {
	t.Parallel()
	checked := 0
	for id, cfg := range ModelRatios {
		if len(cfg.SupportedReasoningEfforts) == 0 {
			continue
		}
		checked++
		for _, effort := range globalReasoningEfforts {
			for _, variant := range []string{effort, " " + strings.ToUpper(effort) + " "} {
				for _, viaObject := range []bool{false, true} {
					value := variant
					request := &model.GeneralOpenAIRequest{Model: id, Messages: []model.Message{{Role: "user", Content: "hello"}}}
					if viaObject {
						request.Reasoning = &model.OpenAIResponseReasoning{Effort: &value}
					} else {
						request.ReasoningEffort = &value
					}
					converted, err := (&Adaptor{}).ConvertRequest(nil, 0, request)
					require.NoError(t, err)
					encoded, err := json.Marshal(converted)
					require.NoError(t, err)
					var wire map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(encoded, &wire))
					require.NotContains(t, wire, "reasoning", "%s: reasoning object must not reach NVIDIA", id)
					if slices.Contains(cfg.SupportedReasoningEfforts, effort) {
						require.JSONEq(t, `"`+effort+`"`, string(wire["reasoning_effort"]), "%s %q", id, variant)
					} else {
						require.NotContains(t, wire, "reasoning_effort", "%s %q must be omitted", id, variant)
					}
				}
			}
		}
	}
	require.Positive(t, checked, "the catalog must publish at least one effort vocabulary")
}
