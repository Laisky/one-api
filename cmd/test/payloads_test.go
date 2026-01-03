package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseAPIToolHistoryPayload verifies that responseAPIPayload renders
// multi-turn tool history using the documented function_call and
// function_call_output items that the upstream Response API expects.
func TestResponseAPIToolHistoryPayload(t *testing.T) {
	t.Parallel()
	payloadAny := responseAPIPayload("gpt-4o-mini", false, expectationToolHistory)
	payload, ok := payloadAny.(map[string]any)
	require.True(t, ok, "payload should be a map")

	inputRaw, ok := payload["input"].([]any)
	require.True(t, ok, "input should be a slice")
	require.Len(t, inputRaw, 4)

	first, ok := inputRaw[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "user", first["role"])
	content, ok := first["content"].([]map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, content)
	require.Equal(t, "input_text", content[0]["type"])

	second, ok := inputRaw[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function_call", second["type"])
	id, ok := second["id"].(string)
	require.True(t, ok)
	require.True(t, strings.HasPrefix(id, "fc_"))
	callID, ok := second["call_id"].(string)
	require.True(t, ok)
	require.True(t, strings.HasPrefix(callID, "call_"))
	_, ok = second["arguments"].(string)
	require.True(t, ok)

	third, ok := inputRaw[2].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function_call_output", third["type"])
	callID, ok = third["call_id"].(string)
	require.True(t, ok)
	require.True(t, strings.HasPrefix(callID, "call_"))
	_, exists := third["id"]
	require.False(t, exists)
	_, ok = third["output"].(string)
	require.True(t, ok)

	fourth, ok := inputRaw[3].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "user", fourth["role"])
}

func TestChatCompletionToolHistoryPayload(t *testing.T) {
	t.Parallel()
	payloadAny := chatCompletionPayload("gpt-4o-mini", false, expectationToolHistory)
	payload, ok := payloadAny.(map[string]any)
	require.True(t, ok)

	messages, ok := payload["messages"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, messages, 5)

	assistant := messages[2]
	toolCalls, ok := assistant["tool_calls"].([]map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, toolCalls)
	call := toolCalls[0]
	id, ok := call["id"].(string)
	require.True(t, ok)
	require.True(t, strings.HasPrefix(id, "call_"))
	function, ok := call["function"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "get_weather", function["name"])
	require.NotEmpty(t, function["arguments"])

	toolMsg := messages[3]
	require.Equal(t, "tool", toolMsg["role"])
	require.Equal(t, id, toolMsg["tool_call_id"])
	require.NotEmpty(t, toolMsg["content"])

	followup := messages[4]
	require.Equal(t, "user", followup["role"])

	tools, ok := payload["tools"].([]map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, tools)
	require.Equal(t, "function", tools[0]["type"])
}

func TestClaudeMessagesToolHistoryPayload(t *testing.T) {
	t.Parallel()
	payloadAny := claudeMessagesPayload("claude-3", false, expectationToolHistory)
	payload, ok := payloadAny.(map[string]any)
	require.True(t, ok)

	messages, ok := payload["messages"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, messages, 3)

	assistant := messages[1]
	content, ok := assistant["content"].([]map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, content)
	require.Equal(t, "tool_use", content[0]["type"])
	require.Equal(t, "get_weather", content[0]["name"])

	user := messages[2]
	userContent, ok := user["content"].([]map[string]any)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(userContent), 2)
	require.Equal(t, "tool_result", userContent[0]["type"])
	require.Equal(t, content[0]["id"], userContent[0]["tool_use_id"])

	tools, ok := payload["tools"].([]map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, tools)
	require.Equal(t, "get_weather", tools[0]["name"])
}
