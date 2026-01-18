package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCallToolResult_UnmarshalJSON_PreservesRaw verifies that raw payloads are preserved.
func TestCallToolResult_UnmarshalJSON_PreservesRaw(t *testing.T) {
	raw := `{"content":[{"type":"text","text":"hello"}],"is_error":false,"extra":{"results":[1,2]}}`
	var result CallToolResult
	err := json.Unmarshal([]byte(raw), &result)
	require.NoError(t, err)
	require.Equal(t, raw, string(result.Raw))
	require.NotNil(t, result.Content)
	require.False(t, result.IsError)
}
