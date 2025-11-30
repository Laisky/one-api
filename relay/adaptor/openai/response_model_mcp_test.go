package openai_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestMCPOutputItemSerialization tests that MCP-specific OutputItem fields are properly serialized
func TestMCPOutputItemSerialization(t *testing.T) {
	// Test mcp_list_tools output item
	mcpListTools := openai.OutputItem{
		Type:        "mcp_list_tools",
		Id:          "mcpl_682d4379df088191886b70f4ec39f90403937d5f622d7a90",
		ServerLabel: "deepwiki",
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "read_wiki_structure",
					Description: "Read repository structure",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"repoName": map[string]any{
								"type":        "string",
								"description": "GitHub repository: owner/repo (e.g. \"facebook/react\")",
							},
						},
						"required": []string{"repoName"},
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(mcpListTools)
	require.NoError(t, err, "Failed to marshal mcp_list_tools")

	var result map[string]any
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err, "Failed to unmarshal JSON")

	// Verify MCP-specific fields
	require.Equal(t, "mcp_list_tools", result["type"], "Expected type 'mcp_list_tools'")
	require.Equal(t, "deepwiki", result["server_label"], "Expected server_label 'deepwiki'")
	tools, ok := result["tools"].([]any)
	require.True(t, ok && len(tools) == 1, "Expected tools array with 1 item, got %v", result["tools"])
}

// TestMCPCallOutputItem tests mcp_call output item serialization
func TestMCPCallOutputItem(t *testing.T) {
	// Test successful mcp_call
	mcpCall := openai.OutputItem{
		Type:        "mcp_call",
		Id:          "mcp_682d437d90a88191bf88cd03aae0c3e503937d5f622d7a90",
		ServerLabel: "deepwiki",
		Name:        "ask_question",
		Arguments:   "{\"repoName\":\"modelcontextprotocol/modelcontextprotocol\",\"question\":\"What transport protocols does the 2025-03-26 version of the MCP spec support?\"}",
		Output:      "The 2025-03-26 version of the Model Context Protocol (MCP) specification supports two standard transport mechanisms: `stdio` and `Streamable HTTP`",
	}

	jsonData, err := json.Marshal(mcpCall)
	require.NoError(t, err, "Failed to marshal mcp_call")

	var result map[string]any
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err, "Failed to unmarshal JSON")

	// Verify fields
	require.Equal(t, "mcp_call", result["type"], "Expected type 'mcp_call'")
	require.Equal(t, "ask_question", result["name"], "Expected name 'ask_question'")
	require.Equal(t, "deepwiki", result["server_label"], "Expected server_label 'deepwiki'")
	require.NotEmpty(t, result["output"], "Expected output to be present")
}

// TestMCPCallWithError tests mcp_call output item with error
func TestMCPCallWithError(t *testing.T) {
	errorMsg := "Connection failed"
	mcpCallError := openai.OutputItem{
		Type:        "mcp_call",
		Id:          "mcp_error_123",
		ServerLabel: "stripe",
		Name:        "create_payment_link",
		Arguments:   "{\"amount\":2000}",
		Error:       &errorMsg,
	}

	jsonData, err := json.Marshal(mcpCallError)
	require.NoError(t, err, "Failed to marshal mcp_call with error")

	var result map[string]any
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err, "Failed to unmarshal JSON")

	require.Equal(t, "Connection failed", result["error"], "Expected error 'Connection failed'")
}

// TestMCPApprovalRequest tests mcp_approval_request output item
func TestMCPApprovalRequest(t *testing.T) {
	approvalRequest := openai.OutputItem{
		Type:        "mcp_approval_request",
		Id:          "mcpr_682d498e3bd4819196a0ce1664f8e77b04ad1e533afccbfa",
		ServerLabel: "deepwiki",
		Name:        "ask_question",
		Arguments:   "{\"repoName\":\"modelcontextprotocol/modelcontextprotocol\",\"question\":\"What transport protocols are supported in the 2025-03-26 version of the MCP spec?\"}",
	}

	jsonData, err := json.Marshal(approvalRequest)
	require.NoError(t, err, "Failed to marshal mcp_approval_request")

	var result map[string]any
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err, "Failed to unmarshal JSON")

	// Verify fields
	require.Equal(t, "mcp_approval_request", result["type"], "Expected type 'mcp_approval_request'")
	require.Equal(t, "deepwiki", result["server_label"], "Expected server_label 'deepwiki'")
}

// TestMCPApprovalResponseInput tests the MCP approval response input structure
func TestMCPApprovalResponseInput(t *testing.T) {
	approvalResponse := openai.MCPApprovalResponseInput{
		Type:              "mcp_approval_response",
		Approve:           true,
		ApprovalRequestId: "mcpr_682d498e3bd4819196a0ce1664f8e77b04ad1e533afccbfa",
	}

	jsonData, err := json.Marshal(approvalResponse)
	require.NoError(t, err, "Failed to marshal MCPApprovalResponseInput")

	var result map[string]any
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err, "Failed to unmarshal JSON")

	// Verify fields
	require.Equal(t, "mcp_approval_response", result["type"], "Expected type 'mcp_approval_response'")
	require.Equal(t, true, result["approve"], "Expected approve true")
	require.Equal(t, "mcpr_682d498e3bd4819196a0ce1664f8e77b04ad1e533afccbfa", result["approval_request_id"], "Expected correct approval_request_id")

	// Test deserialization
	var deserializedResponse openai.MCPApprovalResponseInput
	err = json.Unmarshal(jsonData, &deserializedResponse)
	require.NoError(t, err, "Failed to deserialize MCPApprovalResponseInput")

	require.Equal(t, "mcp_approval_response", deserializedResponse.Type, "Expected type 'mcp_approval_response'")
	require.True(t, deserializedResponse.Approve, "Expected approve to be true")
	require.Equal(t, "mcpr_682d498e3bd4819196a0ce1664f8e77b04ad1e533afccbfa", deserializedResponse.ApprovalRequestId, "Expected correct approval_request_id")
}

// TestResponseAPIResponseWithMCPOutput tests complete ResponseAPIResponse with MCP output items
func TestResponseAPIResponseWithMCPOutput(t *testing.T) {
	response := openai.ResponseAPIResponse{
		Id:        "resp_123",
		Object:    "response",
		Model:     "gpt-4.1",
		Status:    "completed",
		CreatedAt: 1234567890,
		Output: []openai.OutputItem{
			{
				Type:        "mcp_list_tools",
				Id:          "mcpl_456",
				ServerLabel: "deepwiki",
				Tools: []model.Tool{
					{
						Type: "function",
						Function: &model.Function{
							Name:        "ask_question",
							Description: "Ask a question",
						},
					},
				},
			},
			{
				Type:        "mcp_call",
				Id:          "mcp_789",
				ServerLabel: "deepwiki",
				Name:        "ask_question",
				Arguments:   "{\"question\":\"test\"}",
				Output:      "Test response",
			},
		},
	}

	jsonData, err := json.Marshal(response)
	require.NoError(t, err, "Failed to marshal ResponseAPIResponse with MCP output")

	var deserializedResponse openai.ResponseAPIResponse
	err = json.Unmarshal(jsonData, &deserializedResponse)
	require.NoError(t, err, "Failed to deserialize ResponseAPIResponse")

	require.Len(t, deserializedResponse.Output, 2, "Expected 2 output items")

	// Verify first output item (mcp_list_tools)
	firstOutput := deserializedResponse.Output[0]
	require.Equal(t, "mcp_list_tools", firstOutput.Type, "Expected first output type 'mcp_list_tools'")
	require.Len(t, firstOutput.Tools, 1, "Expected 1 tool")

	// Verify second output item (mcp_call)
	secondOutput := deserializedResponse.Output[1]
	require.Equal(t, "mcp_call", secondOutput.Type, "Expected second output type 'mcp_call'")
	require.Equal(t, "Test response", secondOutput.Output, "Expected output 'Test response'")
}

// TestConvertResponseAPIToChatCompletionWithMCP tests MCP output conversion to ChatCompletion
func TestConvertResponseAPIToChatCompletionWithMCP(t *testing.T) {
	responseAPIResp := &openai.ResponseAPIResponse{
		Id:        "resp_mcp_test",
		Model:     "gpt-4.1",
		Status:    "completed",
		CreatedAt: 1234567890,
		Output: []openai.OutputItem{
			{
				Type: "message",
				Role: "assistant",
				Content: []openai.OutputContent{
					{
						Type: "output_text",
						Text: "Hello! I'll help you with that.",
					},
				},
			},
			{
				Type:        "mcp_list_tools",
				ServerLabel: "deepwiki",
				Tools: []model.Tool{
					{Type: "function", Function: &model.Function{Name: "ask_question"}},
				},
			},
			{
				Type:        "mcp_call",
				ServerLabel: "deepwiki",
				Name:        "ask_question",
				Arguments:   "{\"question\":\"test\"}",
				Output:      "The answer is 42",
			},
		},
	}

	chatResponse := openai.ConvertResponseAPIToChatCompletion(responseAPIResp)

	require.NotNil(t, chatResponse, "Expected non-nil chat response")
	require.Equal(t, "resp_mcp_test", chatResponse.Id, "Expected ID 'resp_mcp_test'")
	require.Len(t, chatResponse.Choices, 1, "Expected 1 choice")

	choice := chatResponse.Choices[0]
	content := choice.Message.Content.(string)

	// Verify that MCP content was included in the response text
	require.True(t, containsSubstring(content, "Hello! I'll help you with that."), "Expected original message content to be preserved")
	require.True(t, containsSubstring(content, "MCP Server 'deepwiki' tools imported: 1 tools available"), "Expected MCP list tools info to be included")
	require.True(t, containsSubstring(content, "MCP Tool 'ask_question' result: The answer is 42"), "Expected MCP call result to be included")
}

// Helper function to check if a string contains a substring.
// It simplifies the use of the standard library.
func containsSubstring(s, substr string) bool { return strings.Contains(s, substr) }
