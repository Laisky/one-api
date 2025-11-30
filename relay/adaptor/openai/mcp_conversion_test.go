package openai_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/model"
)

// TestConvertChatCompletionToResponseAPIWithMCP tests that MCP tools are properly converted
// from ChatCompletion format to Response API format, preserving all MCP-specific fields
func TestConvertChatCompletionToResponseAPIWithMCP(t *testing.T) {
	// Create a ChatCompletion request with MCP tool (matches the failing curl example)
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4o",
		Messages: []model.Message{
			{
				Role:    "user",
				Content: "What transport protocols are supported in the 2025-03-26 version of the MCP spec?",
			},
		},
		Tools: []model.Tool{
			{
				Type:            "mcp",
				ServerLabel:     "deepwiki",
				ServerUrl:       "https://mcp.deepwiki.com/mcp",
				RequireApproval: "never",
			},
		},
	}

	// Convert to Response API format
	responseAPI := openai.ConvertChatCompletionToResponseAPI(chatRequest)

	// Verify the conversion succeeded
	require.NotNil(t, responseAPI, "ConvertChatCompletionToResponseAPI returned nil")

	// Verify tools were converted properly
	require.Len(t, responseAPI.Tools, 1, "Expected 1 tool")

	tool := responseAPI.Tools[0]

	// Verify MCP-specific fields are preserved
	require.Equal(t, "mcp", tool.Type, "Expected tool type 'mcp'")
	require.Equal(t, "deepwiki", tool.ServerLabel, "Expected server_label 'deepwiki'")
	require.Equal(t, "https://mcp.deepwiki.com/mcp", tool.ServerUrl, "Expected server_url 'https://mcp.deepwiki.com/mcp'")
	require.Equal(t, "never", tool.RequireApproval, "Expected require_approval 'never'")

	// Verify function-specific fields are empty for MCP tools
	require.Empty(t, tool.Name, "Expected empty name for MCP tool")
	require.Empty(t, tool.Description, "Expected empty description for MCP tool")
	require.Nil(t, tool.Parameters, "Expected nil parameters for MCP tool")

	// Verify the tool can be marshaled to JSON (important for the actual API request)
	jsonData, err := json.Marshal(tool)
	require.NoError(t, err, "Failed to marshal MCP tool to JSON")

	// Verify the JSON contains the required server_label field
	var result map[string]any
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err, "Failed to unmarshal JSON")

	serverLabel, exists := result["server_label"]
	require.True(t, exists, "server_label field is missing from JSON - this would cause the original error")
	require.Equal(t, "deepwiki", serverLabel, "Expected server_label 'deepwiki' in JSON")

	t.Logf("Successfully converted MCP tool to Response API format: %s", string(jsonData))
}

// TestConvertChatCompletionToResponseAPIWithMCPAndFunction tests mixed MCP and function tools
func TestConvertChatCompletionToResponseAPIWithMCPAndFunction(t *testing.T) {
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4o",
		Messages: []model.Message{
			{
				Role:    "user",
				Content: "Test mixed tools",
			},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "get_weather",
					Description: "Get weather information",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type": "string",
							},
						},
					},
				},
			},
			{
				Type:        "mcp",
				ServerLabel: "stripe",
				ServerUrl:   "https://mcp.stripe.com",
				RequireApproval: map[string]any{
					"never": map[string]any{
						"tool_names": []string{"create_payment_link"},
					},
				},
				AllowedTools: []string{"create_payment_link", "get_balance"},
				Headers: map[string]string{
					"Authorization": "Bearer sk_test_123",
				},
			},
		},
	}

	responseAPI := openai.ConvertChatCompletionToResponseAPI(chatRequest)

	require.Len(t, responseAPI.Tools, 2, "Expected 2 tools")

	// Verify function tool
	functionTool := responseAPI.Tools[0]
	require.Equal(t, "function", functionTool.Type, "Expected first tool type 'function'")
	require.Equal(t, "get_weather", functionTool.Name, "Expected function name 'get_weather'")
	require.Empty(t, functionTool.ServerLabel, "Function tool should not have server_label")

	// Verify MCP tool
	mcpTool := responseAPI.Tools[1]
	require.Equal(t, "mcp", mcpTool.Type, "Expected second tool type 'mcp'")
	require.Equal(t, "stripe", mcpTool.ServerLabel, "Expected server_label 'stripe'")
	require.Len(t, mcpTool.AllowedTools, 2, "Expected 2 allowed tools")
	require.Equal(t, "Bearer sk_test_123", mcpTool.Headers["Authorization"], "Expected Authorization header")
	require.Empty(t, mcpTool.Name, "MCP tool should not have function name")
}

// TestMCPToolJSONSerialization tests that the converted MCP tool produces valid JSON
func TestMCPToolJSONSerialization(t *testing.T) {
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4o",
		Messages: []model.Message{
			{Role: "user", Content: "Test"},
		},
		Tools: []model.Tool{
			{
				Type:            "mcp",
				ServerLabel:     "deepwiki",
				ServerUrl:       "https://mcp.deepwiki.com/mcp",
				RequireApproval: "never",
			},
		},
	}

	responseAPI := openai.ConvertChatCompletionToResponseAPI(chatRequest)

	// Marshal the entire request to JSON
	jsonData, err := json.Marshal(responseAPI)
	require.NoError(t, err, "Failed to marshal Response API request")

	// Verify it can be unmarshaled back
	var unmarshaled openai.ResponseAPIRequest
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err, "Failed to unmarshal Response API request")

	// Verify MCP tool fields are preserved
	require.Len(t, unmarshaled.Tools, 1, "Expected 1 tool after round-trip")

	tool := unmarshaled.Tools[0]
	require.Equal(t, "deepwiki", tool.ServerLabel, "server_label lost during JSON round-trip")

	t.Logf("JSON serialization successful: %s", string(jsonData))
}
