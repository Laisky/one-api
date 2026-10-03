package controller

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Laisky/one-api/relay/mcp"
	"github.com/stretchr/testify/require"
)

// TestMCP433ReviewGatewayMetadataCopy verifies bounded-size copies, capability filtering and caller immutability.
// Nil metadata produces an empty capability object without a panic; custom fields retain exact JSON values.
func TestMCP433ReviewGatewayMetadataCopy(t *testing.T) {
	for _, count := range []int{-1, 0, 1, 4096} {
		t.Run(fmt.Sprintf("entries-%d", count), func(t *testing.T) {
			var meta map[string]any
			if count >= 0 {
				meta = make(map[string]any)
				for index := 0; index < count; index++ {
					meta[fmt.Sprintf("com.example/field-%d", index)] = json.Number("9007199254740993")
				}
				meta["io.modelcontextprotocol/logLevel"] = "debug"
				meta[mcp.MetaClientCapabilitiesKey] = map[string]any{
					"elicitation": map[string]any{"form": map[string]any{}},
					"sampling":    map[string]any{}, "roots": map[string]any{},
					"tasks": map[string]any{}, "subscriptions": map[string]any{},
				}
			}
			before, err := json.Marshal(meta)
			require.NoError(t, err)
			forwarded := forwardedModernMCPToolMeta(meta)
			for index := 0; index < count; index++ {
				require.Equal(t, json.Number("9007199254740993"), forwarded[fmt.Sprintf("com.example/field-%d", index)])
			}
			require.NotContains(t, forwarded, "io.modelcontextprotocol/logLevel")
			capabilities, ok := forwarded[mcp.MetaClientCapabilitiesKey].(map[string]any)
			require.True(t, ok)
			require.NotNil(t, capabilities)
			if count >= 0 {
				require.Len(t, capabilities, 3)
				require.Contains(t, capabilities, "elicitation")
				require.Contains(t, capabilities, "sampling")
				require.Contains(t, capabilities, "roots")
			} else {
				require.Empty(t, capabilities)
			}
			// Mutating the returned envelope and capability set must not affect the caller's maps.
			forwarded["com.example/new"] = true
			delete(capabilities, "sampling")
			after, err := json.Marshal(meta)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
