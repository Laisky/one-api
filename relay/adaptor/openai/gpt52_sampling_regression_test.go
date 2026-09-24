package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGPT52SamplingDefaultsAndSnapshots preserves supported sampling without broadening other model contracts.
func TestGPT52SamplingDefaultsAndSnapshots(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gpt-5.2", "gpt-5.2-2025-12-11"} {
		t.Run(name, func(t *testing.T) {
			config := ModelRatios[name]
			require.Equal(t, "none", config.DefaultReasoningEffort)
			require.Equal(t, []string{"none", "low", "medium", "high", "xhigh"}, config.SupportedReasoningEfforts)
			// Legacy minimal is an input alias, not a different upstream reasoning contract.
			minimal := "minimal"
			require.Equal(t, "none", *normalizeReasoningEffortForModel(name, &minimal))
			require.Equal(t, "none", *normalizeReasoningEffortForModel(name, nil))
			for _, effort := range []string{"", "none", "low", "medium", "high", "xhigh"} {
				root := map[string]json.RawMessage{
					"model": json.RawMessage(`"` + name + `"`),
					"temperature": json.RawMessage(`0`), "top_p": json.RawMessage(`0.9`),
				}
				if effort != "" {
					root["reasoning"] = json.RawMessage(`{"effort":"` + effort + `"}`)
				}
				NormalizeModelRequestParameters(root)
				if effort == "" || effort == "none" {
					require.Equal(t, "0", string(root["temperature"]), effort)
					require.Equal(t, "0.9", string(root["top_p"]), effort)
				} else {
					require.NotContains(t, root, "temperature", effort)
					require.NotContains(t, root, "top_p", effort)
				}
			}
		})
	}
	for _, name := range []string{"gpt-5", "gpt-5.2-pro", "gpt-5.2-pro-2025-12-11", "gpt-5.2-codex"} {
		none := "none"
		require.False(t, modelSupportsSampling(name, &none), name)
	}
}
