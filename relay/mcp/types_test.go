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

// TestToolDescriptor_MarshalJSON_UsesCamelCase verifies MCP-spec compliant
// camelCase serialization. The MCP spec (2025-03-26 / 2025-06-18 / draft)
// mandates `inputSchema`; the legacy snake_case form would silently drop the
// schema for strict consumers.
func TestToolDescriptor_MarshalJSON_UsesCamelCase(t *testing.T) {
	descriptor := ToolDescriptor{
		Name:        "web_search",
		Description: "Search",
		InputSchema: map[string]any{
			"type": "object",
		},
	}
	encoded, err := json.Marshal(descriptor)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"inputSchema"`)
	require.NotContains(t, string(encoded), `"input_schema"`)

	emptyDescriptor := ToolDescriptor{Name: "web_search"}
	emptyEncoded, err := json.Marshal(emptyDescriptor)
	require.NoError(t, err)
	require.NotContains(t, string(emptyEncoded), `"inputSchema"`)
	require.NotContains(t, string(emptyEncoded), `"input_schema"`)
}

// TestToolDescriptor_UnmarshalJSON_HandlesSchemaFields verifies schema field naming variants are supported.
func TestToolDescriptor_UnmarshalJSON_HandlesSchemaFields(t *testing.T) {
	inputSchemaPayload := `{"name":"web_fetch","description":"Fetch","inputSchema":{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}}`
	var descriptor ToolDescriptor
	err := json.Unmarshal([]byte(inputSchemaPayload), &descriptor)
	require.NoError(t, err)
	require.Equal(t, "web_fetch", descriptor.Name)
	require.NotNil(t, descriptor.InputSchema)
	require.Equal(t, "object", descriptor.InputSchema["type"])

	underscorePayload := `{"name":"web_search","description":"Search","input_schema":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}`
	var underscore ToolDescriptor
	err = json.Unmarshal([]byte(underscorePayload), &underscore)
	require.NoError(t, err)
	require.Equal(t, "web_search", underscore.Name)
	require.NotNil(t, underscore.InputSchema)
	require.Equal(t, "object", underscore.InputSchema["type"])
}
