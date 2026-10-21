package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEnforceResponseCreateModelPrecision verifies that model pinning preserves
// exact numeric values in provider extensions, tool schemas, and tool arguments.
func TestEnforceResponseCreateModelPrecision(t *testing.T) {
	t.Parallel()
	const fields = `"vendor_extension":{"id":9007199254740993,"largest":18446744073709551615,"decimal":0.12345678901234567890123456789},"tools":[{"type":"function","name":"exact","parameters":{"type":"object","properties":{"value":{"const":9007199254740993,"default":1.234567890123456789e+42}}}}],"input":[{"type":"tool_search_call","execution":"client","call_id":"call_exact","arguments":{"cursor":9007199254740993,"threshold":0.000000000000000000123456789}}]`
	for _, tc := range []struct {
		name, model string
		unchanged   bool
	}{
		{name: "missing"},
		{name: "empty", model: `,"model":""`},
		{name: "alias", model: `,"model":"client-alias"`},
		{name: "already_bound", model: `,"model":"deepseek-v4-pro"`, unchanged: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frame := []byte(`{"type":"response.create"` + tc.model + `,` + fields + `}`)
			out, err := enforceResponseCreateModel(frame, "client-alias", "deepseek-v4-pro")
			require.NoError(t, err)
			var before, after map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(frame, &before))
			require.NoError(t, json.Unmarshal(out, &after))
			require.Equal(t, json.RawMessage(`"deepseek-v4-pro"`), after["model"])
			delete(before, "model")
			delete(after, "model")
			// Raw field comparisons cannot hide precision loss through float decoding.
			require.Equal(t, before, after, "model pinning must leave every other field intact")
			if tc.unchanged {
				require.Equal(t, frame, out)
			}
		})
	}
}

// TestEnforceResponseCreateModelLegacyValues preserves existing behavior for
// non-string models, non-create events, and malformed frames during pinning.
func TestEnforceResponseCreateModelLegacyValues(t *testing.T) {
	t.Parallel()
	for _, model := range []string{`null`, `true`, `17`, `{}`, `[]`} {
		t.Run("non_string_model_"+model, func(t *testing.T) {
			frame := []byte(`{"type":"response.create","model":` + model + `,"input":[]}`)
			out, err := enforceResponseCreateModel(frame, "client-alias", "deepseek-v4-pro")
			require.NoError(t, err)
			var payload map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(out, &payload))
			require.Equal(t, json.RawMessage(`"deepseek-v4-pro"`), payload["model"])
			require.Equal(t, json.RawMessage(`[]`), payload["input"])
		})
	}
	for _, frame := range []string{
		`{"type":"response.cancel","model":"other","vendor_extension":{"id":9007199254740993}}`,
		`{"type":17,"model":"other"}`,
		`{"model":"other"}`,
		`null`,
		`{"type":"response.create",`,
	} {
		t.Run("passthrough_"+frame, func(t *testing.T) {
			out, err := enforceResponseCreateModel([]byte(frame), "client-alias", "deepseek-v4-pro")
			require.NoError(t, err)
			require.Equal(t, []byte(frame), out)
		})
	}
}
