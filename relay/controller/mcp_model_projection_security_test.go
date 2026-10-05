package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/mcp"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestSecurityMCPModelProjection separates model history from the lossless MCP
// client result. Synthetic values make accidental boundary crossings explicit.
func TestSecurityMCPModelProjection(t *testing.T) {
	for _, raw := range []string{
		`{"resultType":"complete","content":[{"type":"text","text":"public answer"}],"structuredContent":{"answer":42},"isError":true,"_meta":{"credential":"private-meta-fixture"},"requestState":"private-state-fixture","inputRequests":{"form":{"prompt":"private-input-fixture"}},"debug":"private-debug-fixture"}`,
		`{"content":[{"type":"text","text":"public answer"}],"structured_content":{"answer":42},"is_error":true,"_meta":{"credential":"private-meta-fixture"},"request_state":"private-state-fixture","input_requests":{"form":{"prompt":"private-input-fixture"}},"debug":"private-debug-fixture"}`,
	} {
		var result mcp.CallToolResult
		require.NoError(t, json.Unmarshal([]byte(raw), &result))
		wireBefore, err := json.Marshal(result)
		require.NoError(t, err)
		rawBefore := append([]byte(nil), result.Raw...)

		message, err := buildToolResultMessage("call-fixture", &result)
		require.NoError(t, err)
		require.Equal(t, "tool", message.Role)
		require.Equal(t, "call-fixture", message.ToolCallId)
		outbound, err := json.Marshal(relaymodel.GeneralOpenAIRequest{
			Model:    "fixture-model",
			Messages: []relaymodel.Message{message},
		})
		require.NoError(t, err)
		for _, private := range []string{"private-meta-fixture", "private-state-fixture", "private-input-fixture", "private-debug-fixture"} {
			require.NotContains(t, string(outbound), private)
			require.Contains(t, string(wireBefore), private, "direct MCP clients must retain their protocol data")
		}
		var visible map[string]any
		require.NoError(t, json.Unmarshal([]byte(message.StringContent()), &visible))
		require.Equal(t, true, visible["isError"])
		require.Equal(t, map[string]any{"answer": float64(42)}, visible["structuredContent"])
		require.Contains(t, message.StringContent(), "public answer")
		for key := range visible {
			require.Contains(t, []string{"content", "structuredContent", "isError"}, key, "unknown model-visible field")
		}
		wireAfter, err := json.Marshal(result)
		require.NoError(t, err)
		require.JSONEq(t, string(wireBefore), string(wireAfter))
		require.Equal(t, rawBefore, []byte(result.Raw), "projection must not mutate the original raw result")
	}
}

// TestSecurityMCPModelProjectionNil retains the existing empty-result contract.
func TestSecurityMCPModelProjectionNil(t *testing.T) {
	message, err := buildToolResultMessage("empty-fixture", nil)
	require.NoError(t, err)
	require.Equal(t, "tool", message.Role)
	require.Equal(t, "empty-fixture", message.ToolCallId)
	require.Equal(t, "", message.StringContent())
}
