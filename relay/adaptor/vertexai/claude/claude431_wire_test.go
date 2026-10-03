package vertexai

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"io"
	"strings"
	"testing"
)

// TestClaude431VertexPreparedWire verifies only transport-owned fields change in native bodies.
func TestClaude431VertexPreparedWire(t *testing.T) {
	t.Parallel()
	raw := `{"model":"claude-sonnet-5-5","stream":true,"max_tokens":64,"thinking":{"type":"between_tools"},"output_config":{"effort":"low","id":9007199254740993},"system":[{"type":"text","text":"<policy>&"}],"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"<signed>&opaque"}]},{"role":"user","content":"Continue"}],"tools":[{"name":"lookup","input_schema":{"type":"object","additionalProperties":false,"properties":{}}}],"future_field":{"exact":9007199254740993}}`
	prepared, err := PrepareRequestBody(strings.NewReader(raw))
	require.NoError(t, err)
	body, err := io.ReadAll(prepared)
	require.NoError(t, err)
	var before, after map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &before))
	require.NoError(t, json.Unmarshal(body, &after))
	for _, key := range []string{"thinking", "output_config", "system", "messages", "tools", "future_field", "max_tokens"} {
		require.Equal(t, before[key], after[key], key)
	}
	require.NotContains(t, after, "model")
	require.NotContains(t, after, "stream")
	require.Equal(t, `"vertex-2023-10-16"`, string(after["anthropic_version"]))
	for _, bad := range []string{"null", "[]", "invalid"} {
		_, err := PrepareRequestBody(strings.NewReader(bad))
		require.Error(t, err)
	}
	_, err = PrepareRequestBody(nil)
	require.Error(t, err)
}
