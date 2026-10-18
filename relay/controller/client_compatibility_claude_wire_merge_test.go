package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClientCompatibilityClaudeLoopFieldExpansion preserves opaque fields while applying a larger typed view.
// Parameters: t owns independent JSON maps. Returns: none; overlays and removals leave all inputs unchanged.
func TestClientCompatibilityClaudeLoopFieldExpansion(t *testing.T) {
	t.Parallel()
	raw := map[string]json.RawMessage{
		"vendor":      json.RawMessage(`{"exact":9007199254740993}`),
		"model":       json.RawMessage(`"old"`),
		"system":      json.RawMessage(`"unchanged"`),
		"temperature": json.RawMessage(`0.5`),
	}
	previous := map[string]json.RawMessage{
		"model":       json.RawMessage(`"old"`),
		"system":      json.RawMessage(`"unchanged"`),
		"temperature": json.RawMessage(`0.5`),
	}
	current := map[string]json.RawMessage{
		"model":      json.RawMessage(`"new"`),
		"system":     json.RawMessage(`"unchanged"`),
		"max_tokens": json.RawMessage(`64`),
		"tools":      json.RawMessage(`[{"name":"added","input_schema":{"type":"object"}}]`),
		"metadata":   json.RawMessage(`{"user_id":"fixture"}`),
		"thinking":   json.RawMessage(`{"type":"disabled"}`),
	}
	inputs := []map[string]json.RawMessage{raw, previous, current}
	before := make([][]byte, len(inputs))
	for i, input := range inputs {
		encoded, err := json.Marshal(input)
		require.NoError(t, err)
		before[i] = encoded
	}
	merged := mergeClaudeLoopFields(raw, previous, current)
	wire, err := json.Marshal(merged)
	require.NoError(t, err)
	require.JSONEq(t, `{"vendor":{"exact":9007199254740993},"model":"new","system":"unchanged","max_tokens":64,"tools":[{"name":"added","input_schema":{"type":"object"}}],"metadata":{"user_id":"fixture"},"thinking":{"type":"disabled"}}`, string(wire))
	require.Contains(t, string(wire), "9007199254740993")
	require.NotContains(t, merged, "temperature")
	for i, input := range inputs {
		encoded, err := json.Marshal(input)
		require.NoError(t, err)
		require.Equal(t, before[i], encoded)
	}
	fresh := mergeClaudeLoopFields(nil, nil, map[string]json.RawMessage{"model": json.RawMessage(`"fresh"`)})
	require.JSONEq(t, `"fresh"`, string(fresh["model"]), "a nil original snapshot must still accept added fields")
}
