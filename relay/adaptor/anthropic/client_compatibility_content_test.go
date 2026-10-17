package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClientCompatibilityClaudeContentOpaqueRoundTrip preserves opaque provider blocks without float coercion.
func TestClientCompatibilityClaudeContentOpaqueRoundTrip(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"type":"tool_search_tool_result","tool_use_id":"search_a","content":[{"type":"tool_reference","tool_name":"probe_a"}],"vendor":{"exact":9007199254740993}}`,
		`{"type":"web_search_tool_result","tool_use_id":"search_a","content":{"type":"web_search_tool_result_error","error_code":"fixture"},"vendor":{"exact":9007199254740993}}`,
		`{"type":"redacted_thinking","data":"opaque-redacted-fixture","vendor":{"exact":9007199254740993}}`,
		`{"type":"compaction","content":"opaque-compacted-state","signature":"sig-fixture","vendor":{"exact":9007199254740993}}`,
		`{"type":"thinking","thinking":"visible fixture","signature":"sig-fixture","vendor":{"exact":9007199254740993}}`,
		`{"type":"tool_use","id":"call_a","name":"probe_a","input":{"exact":9007199254740993},"cache_control":{"type":"ephemeral"},"vendor":{"retain":true}}`,
		`{"type":"text","text":"answer","citations":[{"type":"char_location","start_char_index":0,"end_char_index":6}],"vendor":{"exact":9007199254740993}}`,
	} {
		var content Content
		require.NoError(t, json.Unmarshal([]byte(raw), &content))
		for cycle := 0; cycle < 2; cycle++ {
			wire, err := json.Marshal(content)
			require.NoError(t, err)
			require.JSONEq(t, raw, string(wire))
			require.Contains(t, string(wire), "9007199254740993", "cycle %d preserves exact numeric token", cycle)
			require.NoError(t, json.Unmarshal(wire, &content))
		}
	}
}

// TestClientCompatibilityClaudeContentTypedMutationWins prevents retained wire bytes from undoing typed changes.
func TestClientCompatibilityClaudeContentTypedMutationWins(t *testing.T) {
	t.Parallel()
	var content Content
	require.NoError(t, json.Unmarshal([]byte(`{"type":"tool_use","id":"call_a","name":"probe_a","input":{"query":"old"},"vendor":{"exact":9007199254740993}}`), &content))
	content.Name = "renamed"
	content.Input = map[string]any{"query": "new"}
	wire, err := json.Marshal(content)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(wire, &fields))
	require.JSONEq(t, `"renamed"`, string(fields["name"]))
	require.JSONEq(t, `{"query":"new"}`, string(fields["input"]))
	require.JSONEq(t, `{"exact":9007199254740993}`, string(fields["vendor"]), "typed changes must not discard unrelated opaque metadata")
	require.Contains(t, string(wire), "9007199254740993")

	// Clearing a known optional field must remove it rather than resurrect raw data.
	content.Name = ""
	wire, err = json.Marshal(content)
	require.NoError(t, err)
	fields = nil
	require.NoError(t, json.Unmarshal(wire, &fields))
	require.NotContains(t, fields, "name")
}

// TestClientCompatibilityClaudeContentLegacyConstructed keeps ordinary typed construction backward compatible.
func TestClientCompatibilityClaudeContentLegacyConstructed(t *testing.T) {
	t.Parallel()
	for _, content := range []Content{
		{Type: "text", Text: "answer"},
		{Type: "tool_use", Id: "call_a", Name: "probe_a", Input: map[string]any{}},
	} {
		wire, err := json.Marshal(content)
		require.NoError(t, err)
		var decoded Content
		require.NoError(t, json.Unmarshal(wire, &decoded))
		require.Equal(t, content.Type, decoded.Type)
		require.Equal(t, content.Text, decoded.Text)
		require.Equal(t, content.Name, decoded.Name)
		require.Equal(t, content.Id, decoded.Id)
		require.Equal(t, content.Input, decoded.Input)
	}
}
