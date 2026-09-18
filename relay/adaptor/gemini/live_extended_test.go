package gemini

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGeminiExtendedToolBehavior verifies Google's NON_BLOCKING requirement
// before paid setup. Parameters: t is the test handle. Returns: none. Omitted
// behavior gets the model's required default; explicit BLOCKING is rejected.
func TestGeminiExtendedToolBehavior(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, model, behavior string
		want string
		reject bool
	}{
		{"default_extended", "gemini-3.8-live-extended-thinking", "", "NON_BLOCKING", false},
		{"explicit_extended", "gemini-3.8-live-extended-thinking", "NON_BLOCKING", "NON_BLOCKING", false},
		{"blocking_extended", "gemini-3.8-live-extended-thinking", "BLOCKING", "", true},
		{"ordinary_blocking", "gemini-3.8-live", "BLOCKING", "BLOCKING", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			declaration := map[string]any{"name":"lookup", "parameters":map[string]any{"type":"OBJECT"}}
			if tc.behavior != "" { declaration["behavior"] = tc.behavior }
			raw, err := json.Marshal(map[string]any{"setup": map[string]any{"tools": []any{map[string]any{"functionDeclarations": []any{declaration}}}}})
			require.NoError(t, err)
			prepared, err := prepareLiveSetup(raw, tc.model, "friendly")
			if tc.reject { require.Error(t, err); return }
			require.NoError(t, err)
			var out struct { Setup struct { Tools []struct { Functions []struct { Behavior string `json:"behavior"` } `json:"functionDeclarations"` } `json:"tools"` } `json:"setup"` }
			require.NoError(t, json.Unmarshal(prepared, &out))
			require.Equal(t, tc.want, out.Setup.Tools[0].Functions[0].Behavior)
		})
	}
}
