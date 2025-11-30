package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestToolIndexField tests that the Index field is properly serialized in streaming tool calls
func TestToolIndexField(t *testing.T) {
	// Test streaming tool call with Index field set
	index := 0
	streamingTool := Tool{
		Id:   "call_123",
		Type: "function",
		Function: &Function{
			Name:      "get_weather",
			Arguments: `{"location": "Paris"}`,
		},
		Index: &index,
	}

	// Serialize to JSON
	jsonData, err := json.Marshal(streamingTool)
	require.NoError(t, err, "Failed to marshal streaming tool")

	// Verify that the index field is present in JSON
	var result map[string]any
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err, "Failed to unmarshal JSON")

	// Check that index field exists and has correct value
	indexValue, exists := result["index"]
	require.True(t, exists, "Index field is missing from JSON output")
	require.Equal(t, float64(0), indexValue, "Expected index to be 0")

	// Test non-streaming tool call without Index field
	nonStreamingTool := Tool{
		Id:   "call_456",
		Type: "function",
		Function: &Function{
			Name:      "send_email",
			Arguments: `{"to": "test@example.com"}`,
		},
		// Index is nil for non-streaming responses
	}

	// Serialize to JSON
	jsonData2, err := json.Marshal(nonStreamingTool)
	require.NoError(t, err, "Failed to marshal non-streaming tool")

	// Verify that the index field is omitted in JSON (due to omitempty)
	var result2 map[string]any
	err = json.Unmarshal(jsonData2, &result2)
	require.NoError(t, err, "Failed to unmarshal JSON")

	// Check that index field does not exist
	_, exists2 := result2["index"]
	require.False(t, exists2, "Index field should be omitted for non-streaming tool calls")
}

// TestStreamingToolCallAccumulation tests the complete streaming tool call accumulation workflow
func TestStreamingToolCallAccumulation(t *testing.T) {
	// Simulate streaming tool call deltas as they would come from the API
	streamingDeltas := []Tool{
		{
			Id:    "call_123",
			Type:  "function",
			Index: intPtr(0),
			Function: &Function{
				Name:      "get_weather",
				Arguments: "",
			},
		},
		{
			Index: intPtr(0),
			Function: &Function{
				Arguments: `{"location":`,
			},
		},
		{
			Index: intPtr(0),
			Function: &Function{
				Arguments: ` "Paris"}`,
			},
		},
	}

	// Accumulate the deltas (simulating client-side accumulation)
	finalToolCalls := make(map[int]Tool)

	for _, delta := range streamingDeltas {
		require.NotNil(t, delta.Index, "Index field should be present in streaming tool call deltas")

		index := *delta.Index

		if _, exists := finalToolCalls[index]; !exists {
			// First delta for this tool call
			finalToolCalls[index] = delta
		} else {
			// Subsequent delta - accumulate arguments
			existing := finalToolCalls[index]
			existingArgs, _ := existing.Function.Arguments.(string)
			deltaArgs, _ := delta.Function.Arguments.(string)
			existing.Function.Arguments = existingArgs + deltaArgs
			finalToolCalls[index] = existing
		}
	}

	// Verify the final accumulated tool call
	require.Len(t, finalToolCalls, 1, "Expected 1 final tool call")

	finalTool := finalToolCalls[0]
	expectedArgs := `{"location": "Paris"}`
	actualArgs, _ := finalTool.Function.Arguments.(string)
	require.Equal(t, expectedArgs, actualArgs, "Accumulated arguments mismatch")
	require.Equal(t, "call_123", finalTool.Id, "Tool call id mismatch")
	require.NotNil(t, finalTool.Function, "Function should not be nil")
	require.Equal(t, "get_weather", finalTool.Function.Name, "Function name mismatch")
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}

// TestToolIndexFieldDeserialization tests that the Index field can be properly deserialized
func TestToolIndexFieldDeserialization(t *testing.T) {
	// JSON with index field (streaming response)
	streamingJSON := `{
		"id": "call_789",
		"type": "function",
		"function": {
			"name": "calculate",
			"arguments": "{\"x\": 5, \"y\": 3}"
		},
		"index": 1
	}`

	var streamingTool Tool
	err := json.Unmarshal([]byte(streamingJSON), &streamingTool)
	require.NoError(t, err, "Failed to unmarshal streaming tool JSON")

	// Verify index field is properly set
	require.NotNil(t, streamingTool.Index, "Index field should not be nil for streaming tool")
	require.Equal(t, 1, *streamingTool.Index, "Expected index to be 1")

	// JSON without index field (non-streaming response)
	nonStreamingJSON := `{
		"id": "call_101",
		"type": "function",
		"function": {
			"name": "search",
			"arguments": "{\"query\": \"test\"}"
		}
	}`

	var nonStreamingTool Tool
	err = json.Unmarshal([]byte(nonStreamingJSON), &nonStreamingTool)
	require.NoError(t, err, "Failed to unmarshal non-streaming tool JSON")

	// Verify index field is nil
	require.Nil(t, nonStreamingTool.Index, "Index field should be nil for non-streaming tool")
}

// TestMCPToolSerialization tests that MCP tools are properly serialized with all MCP fields
func TestMCPToolSerialization(t *testing.T) {
	// Test MCP tool with all fields populated
	mcpTool := Tool{
		Id:              "mcp_001",
		Type:            "mcp",
		ServerLabel:     "deepwiki",
		ServerUrl:       "https://mcp.deepwiki.com/mcp",
		RequireApproval: "never",
		AllowedTools:    []string{"ask_question", "read_wiki_structure"},
		Headers: map[string]string{
			"Authorization":   "Bearer token123",
			"X-Custom-Header": "custom_value",
		},
	}

	// Serialize to JSON
	jsonData, err := json.Marshal(mcpTool)
	require.NoError(t, err, "Failed to marshal MCP tool")

	// Verify all MCP fields are present
	var result map[string]any
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err, "Failed to unmarshal JSON")

	// Check all MCP-specific fields
	require.Equal(t, "deepwiki", result["server_label"], "server_label mismatch")
	require.Equal(t, "https://mcp.deepwiki.com/mcp", result["server_url"], "server_url mismatch")
	require.Equal(t, "never", result["require_approval"], "require_approval mismatch")

	// Verify function field is NOT present for MCP tools (since Function is a pointer and nil)
	_, exists := result["function"]
	require.False(t, exists, "Function field should not be present for MCP tools")
}

// TestMCPToolDeserialization tests that MCP tools can be properly deserialized
func TestMCPToolDeserialization(t *testing.T) {
	// JSON for MCP tool
	mcpJSON := `{
		"id": "mcp_002",
		"type": "mcp",
		"server_label": "stripe",
		"server_url": "https://mcp.stripe.com",
		"require_approval": {
			"never": {
				"tool_names": ["create_payment_link", "get_balance"]
			}
		},
		"allowed_tools": ["create_payment_link", "get_balance", "list_customers"],
		"headers": {
			"Authorization": "Bearer sk_test_123",
			"Content-Type": "application/json"
		}
	}`

	var mcpTool Tool
	err := json.Unmarshal([]byte(mcpJSON), &mcpTool)
	require.NoError(t, err, "Failed to unmarshal MCP tool JSON")

	// Verify all fields are properly set
	require.Equal(t, "mcp_002", mcpTool.Id, "id mismatch")
	require.Equal(t, "mcp", mcpTool.Type, "type mismatch")
	require.Equal(t, "stripe", mcpTool.ServerLabel, "server_label mismatch")
	require.Equal(t, "https://mcp.stripe.com", mcpTool.ServerUrl, "server_url mismatch")

	// Check allowed_tools slice
	expectedTools := []string{"create_payment_link", "get_balance", "list_customers"}
	require.Len(t, mcpTool.AllowedTools, len(expectedTools), "allowed_tools length mismatch")

	// Check headers map
	require.Equal(t, "Bearer sk_test_123", mcpTool.Headers["Authorization"], "Authorization header mismatch")
}

// TestMCPRequireApprovalVariations tests different RequireApproval configurations
func TestMCPRequireApprovalVariations(t *testing.T) {
	testCases := []struct {
		name     string
		approval any
		jsonStr  string
	}{
		{
			name:     "String never",
			approval: "never",
			jsonStr:  `"never"`,
		},
		{
			name: "Object with tool names",
			approval: map[string]any{
				"never": map[string]any{
					"tool_names": []string{"tool1", "tool2"},
				},
			},
			jsonStr: `{"never":{"tool_names":["tool1","tool2"]}}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mcpTool := Tool{
				Type:            "mcp",
				ServerLabel:     "test",
				RequireApproval: tc.approval,
			}

			jsonData, err := json.Marshal(mcpTool)
			require.NoError(t, err, "Failed to marshal MCP tool")

			// Verify the require_approval field is serialized correctly
			var result map[string]any
			err = json.Unmarshal(jsonData, &result)
			require.NoError(t, err, "Failed to unmarshal JSON")

			// Convert require_approval back to JSON to compare
			approvalBytes, err := json.Marshal(result["require_approval"])
			require.NoError(t, err, "Failed to marshal require_approval")
			require.Equal(t, tc.jsonStr, string(approvalBytes), "require_approval JSON mismatch")
		})
	}
}

// TestMixedToolArray tests arrays containing both function and MCP tools
func TestMixedToolArray(t *testing.T) {
	tools := []Tool{
		{
			Id:   "func_001",
			Type: "function",
			Function: &Function{
				Name:        "get_weather",
				Description: "Get weather information",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "The location to get weather for",
						},
					},
					"required": []string{"location"},
				},
			},
		},
		{
			Id:              "mcp_001",
			Type:            "mcp",
			ServerLabel:     "deepwiki",
			ServerUrl:       "https://mcp.deepwiki.com/mcp",
			RequireApproval: "never",
			AllowedTools:    []string{"ask_question"},
			Headers: map[string]string{
				"Authorization": "Bearer token123",
			},
		},
	}

	// Serialize the mixed array
	jsonData, err := json.Marshal(tools)
	require.NoError(t, err, "Failed to marshal mixed tool array")

	// Deserialize back
	var deserializedTools []Tool
	err = json.Unmarshal(jsonData, &deserializedTools)
	require.NoError(t, err, "Failed to unmarshal mixed tool array")

	// Verify we have 2 tools
	require.Len(t, deserializedTools, 2, "Expected 2 tools")

	// Verify function tool
	funcTool := deserializedTools[0]
	require.Equal(t, "function", funcTool.Type, "First tool type mismatch")
	require.Equal(t, "get_weather", funcTool.Function.Name, "Function name mismatch")
	require.Empty(t, funcTool.ServerLabel, "Function tool should not have server_label")

	// Verify MCP tool
	mcpTool := deserializedTools[1]
	require.Equal(t, "mcp", mcpTool.Type, "Second tool type mismatch")
	require.Equal(t, "deepwiki", mcpTool.ServerLabel, "server_label mismatch")
	require.Nil(t, mcpTool.Function, "MCP tool should not have function definition")
}

// TestMCPToolEdgeCases tests edge cases and validation scenarios for MCP tools
func TestMCPToolEdgeCases(t *testing.T) {
	// Test MCP tool with minimal fields
	minimalMCP := Tool{
		Type:        "mcp",
		ServerLabel: "minimal",
		ServerUrl:   "https://minimal.example.com",
	}

	jsonData, err := json.Marshal(minimalMCP)
	require.NoError(t, err, "Failed to marshal minimal MCP tool")

	var result map[string]any
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err, "Failed to unmarshal JSON")

	// Check that optional fields are omitted (except function which is always present as empty object)
	optionalFields := []string{"require_approval", "allowed_tools", "headers", "id"}
	for _, field := range optionalFields {
		_, exists := result[field]
		require.False(t, exists, "Field '%s' should be omitted for minimal MCP tool", field)
	}

	// Function field should NOT be present for MCP tools
	_, exists := result["function"]
	require.False(t, exists, "Function field should not be present for MCP tools")

	// Test empty headers map is omitted
	emptyHeadersMCP := Tool{
		Type:        "mcp",
		ServerLabel: "test",
		Headers:     map[string]string{},
	}

	jsonData, err = json.Marshal(emptyHeadersMCP)
	require.NoError(t, err, "Failed to marshal MCP tool with empty headers")

	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err, "Failed to unmarshal JSON")

	_, exists = result["headers"]
	require.False(t, exists, "Empty headers map should be omitted")

	// Test empty allowed_tools slice is omitted
	emptyToolsMCP := Tool{
		Type:         "mcp",
		ServerLabel:  "test",
		AllowedTools: []string{},
	}

	jsonData, err = json.Marshal(emptyToolsMCP)
	require.NoError(t, err, "Failed to marshal MCP tool with empty allowed_tools")

	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err, "Failed to unmarshal JSON")

	_, exists = result["allowed_tools"]
	require.False(t, exists, "Empty allowed_tools slice should be omitted")
}

func TestToolUnmarshalFlattenedFunction(t *testing.T) {
	jsonStr := `{
		"type": "function",
		"name": "get_weather",
		"description": "Get current temperature for a given location.",
		"parameters": {
			"type": "object",
			"properties": {
				"location": {
					"type": "string"
				}
			},
			"required": ["location"],
			"additionalProperties": false
		},
		"strict": true
	}`

	var tool Tool
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &tool))
	require.NotNil(t, tool.Function)
	require.Equal(t, "function", tool.Type)
	require.Equal(t, "get_weather", tool.Function.Name)
	require.Equal(t, "Get current temperature for a given location.", tool.Function.Description)
	require.NotNil(t, tool.Function.Strict)
	require.True(t, *tool.Function.Strict)
	require.NotNil(t, tool.Function.Parameters)

	encoded, err := json.Marshal(tool)
	require.NoError(t, err)

	var serialized map[string]any
	require.NoError(t, json.Unmarshal(encoded, &serialized))
	require.Equal(t, "function", serialized["type"])

	fn, ok := serialized["function"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "get_weather", fn["name"])
	require.Equal(t, true, fn["strict"])

	_, hasName := serialized["name"]
	require.False(t, hasName)
}
