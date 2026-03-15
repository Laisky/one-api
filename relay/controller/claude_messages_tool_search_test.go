package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/adaptor/anthropic"
	"github.com/songquanpeng/one-api/relay/mcp"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func TestHasToolSearchInClaudeRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tools  []relaymodel.ClaudeTool
		expect bool
	}{
		{
			name:   "nil request",
			tools:  nil,
			expect: false,
		},
		{
			name:   "no tools",
			tools:  []relaymodel.ClaudeTool{},
			expect: false,
		},
		{
			name: "only function tools",
			tools: []relaymodel.ClaudeTool{
				{Name: "get_weather", Description: "Get weather"},
			},
			expect: false,
		},
		{
			name: "regex tool search",
			tools: []relaymodel.ClaudeTool{
				{Type: "tool_search_tool_regex_20251119", Name: "tool_search_tool_regex"},
			},
			expect: true,
		},
		{
			name: "bm25 tool search",
			tools: []relaymodel.ClaudeTool{
				{Type: "tool_search_tool_bm25_20251119", Name: "tool_search_tool_bm25"},
			},
			expect: true,
		},
		{
			name: "unversioned regex",
			tools: []relaymodel.ClaudeTool{
				{Type: "tool_search_tool_regex", Name: "tool_search_tool_regex"},
			},
			expect: true,
		},
		{
			name: "unversioned bm25",
			tools: []relaymodel.ClaudeTool{
				{Type: "tool_search_tool_bm25", Name: "tool_search_tool_bm25"},
			},
			expect: true,
		},
		{
			name: "mixed tools with tool search",
			tools: []relaymodel.ClaudeTool{
				{Name: "get_weather", Description: "Get weather", InputSchema: map[string]any{"type": "object"}},
				{Type: "tool_search_tool_regex_20251119", Name: "tool_search_tool_regex"},
			},
			expect: true,
		},
		{
			name: "web_search only",
			tools: []relaymodel.ClaudeTool{
				{Type: "web_search", Name: "web_search"},
			},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &ClaudeMessagesRequest{
				Model:     "claude-sonnet-4-6",
				MaxTokens: 1024,
				Messages:  []relaymodel.ClaudeMessage{{Role: "user", Content: "test"}},
				Tools:     tt.tools,
			}
			if tt.tools == nil {
				result := hasToolSearchInClaudeRequest(nil)
				assert.Equal(t, tt.expect, result)
				return
			}
			result := hasToolSearchInClaudeRequest(request)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestIsToolSearchType(t *testing.T) {
	t.Parallel()

	assert.True(t, isToolSearchType(anthropic.ToolTypeToolSearchRegex))
	assert.True(t, isToolSearchType(anthropic.ToolTypeToolSearchBM25))
	assert.True(t, isToolSearchType("tool_search_tool_regex_20251119"))
	assert.True(t, isToolSearchType("tool_search_tool_bm25_20251119"))
	assert.False(t, isToolSearchType("web_search"))
	assert.False(t, isToolSearchType("function"))
	assert.False(t, isToolSearchType(""))
}

func TestExtractMCPToolCalls(t *testing.T) {
	t.Parallel()

	registry := &claudeToolSearchMCPRegistry{
		candidatesByName: map[string][]mcp.ToolCandidate{
			"get_weather": {{ResolvedTool: mcp.ResolvedTool{ServerID: 1}}},
			"search_db":   {{ResolvedTool: mcp.ResolvedTool{ServerID: 2}}},
		},
		selectedIndex: map[string]int{},
	}

	t.Run("nil response", func(t *testing.T) {
		calls := extractMCPToolCalls(nil, registry)
		assert.Nil(t, calls)
	})

	t.Run("non-tool_use stop reason", func(t *testing.T) {
		stopReason := "end_turn"
		resp := &anthropic.Response{
			StopReason: &stopReason,
			Content: []anthropic.Content{
				{Type: "text", Text: "Hello"},
			},
		}
		calls := extractMCPToolCalls(resp, registry)
		assert.Nil(t, calls)
	})

	t.Run("tool_use with MCP tools", func(t *testing.T) {
		stopReason := "tool_use"
		resp := &anthropic.Response{
			StopReason: &stopReason,
			Content: []anthropic.Content{
				{Type: "text", Text: "Let me check the weather"},
				{Type: "tool_use", Id: "toolu_123", Name: "get_weather", Input: map[string]any{"location": "SF"}},
				{Type: "tool_use", Id: "toolu_456", Name: "search_db", Input: map[string]any{"query": "test"}},
			},
		}
		calls := extractMCPToolCalls(resp, registry)
		require.Len(t, calls, 2)
		assert.Equal(t, "toolu_123", calls[0].ID)
		assert.Equal(t, "get_weather", calls[0].Name)
		assert.Equal(t, "SF", calls[0].Input["location"])
		assert.Equal(t, "toolu_456", calls[1].ID)
		assert.Equal(t, "search_db", calls[1].Name)
	})

	t.Run("tool_use with non-MCP tools only", func(t *testing.T) {
		stopReason := "tool_use"
		resp := &anthropic.Response{
			StopReason: &stopReason,
			Content: []anthropic.Content{
				{Type: "tool_use", Id: "toolu_789", Name: "unknown_tool", Input: map[string]any{}},
			},
		}
		calls := extractMCPToolCalls(resp, registry)
		assert.Nil(t, calls)
	})

	t.Run("mixed MCP and non-MCP tool calls", func(t *testing.T) {
		stopReason := "tool_use"
		resp := &anthropic.Response{
			StopReason: &stopReason,
			Content: []anthropic.Content{
				{Type: "tool_use", Id: "toolu_1", Name: "get_weather", Input: map[string]any{"location": "NYC"}},
				{Type: "tool_use", Id: "toolu_2", Name: "unknown_tool", Input: map[string]any{}},
			},
		}
		calls := extractMCPToolCalls(resp, registry)
		require.Len(t, calls, 1)
		assert.Equal(t, "get_weather", calls[0].Name)
	})
}

func TestBuildClaudeAssistantMessage(t *testing.T) {
	t.Parallel()

	stopReason := "tool_use"
	resp := &anthropic.Response{
		StopReason: &stopReason,
		Content: []anthropic.Content{
			{Type: "text", Text: "I'll check the weather for you."},
			{Type: "tool_use", Id: "toolu_123", Name: "get_weather", Input: map[string]any{"location": "SF"}},
		},
	}

	msg := buildClaudeAssistantMessage(resp)
	assert.Equal(t, "assistant", msg.Role)

	blocks, ok := msg.Content.([]any)
	require.True(t, ok)
	require.Len(t, blocks, 2)

	// Check text block
	textBlock, ok := blocks[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "text", textBlock["type"])
	assert.Equal(t, "I'll check the weather for you.", textBlock["text"])

	// Check tool_use block
	toolBlock, ok := blocks[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "tool_use", toolBlock["type"])
	assert.Equal(t, "toolu_123", toolBlock["id"])
	assert.Equal(t, "get_weather", toolBlock["name"])
}

func TestBuildClaudeToolResultMessage(t *testing.T) {
	t.Parallel()

	stopReason := "tool_use"
	resp := &anthropic.Response{
		StopReason: &stopReason,
		Content: []anthropic.Content{
			{Type: "text", Text: "Checking..."},
			{Type: "tool_use", Id: "toolu_123", Name: "get_weather", Input: map[string]any{}},
			{Type: "tool_use", Id: "toolu_456", Name: "search_db", Input: map[string]any{}},
		},
	}

	results := map[string]string{
		"toolu_123": `{"temperature": 72}`,
		"toolu_456": `{"results": []}`,
	}

	messages := buildClaudeToolResultMessage(resp, results)
	require.Len(t, messages, 1)
	assert.Equal(t, "user", messages[0].Role)

	blocks, ok := messages[0].Content.([]any)
	require.True(t, ok)
	require.Len(t, blocks, 2)

	block0, ok := blocks[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "tool_result", block0["type"])
	assert.Equal(t, "toolu_123", block0["tool_use_id"])
	assert.Equal(t, `{"temperature": 72}`, block0["content"])

	block1, ok := blocks[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "tool_result", block1["type"])
	assert.Equal(t, "toolu_456", block1["tool_use_id"])
}

func TestBuildClaudeToolResultMessage_Empty(t *testing.T) {
	t.Parallel()

	resp := &anthropic.Response{Content: []anthropic.Content{}}
	messages := buildClaudeToolResultMessage(resp, map[string]string{})
	assert.Nil(t, messages)
}

func TestClaudeToolSearchMCPRegistry(t *testing.T) {
	t.Parallel()

	t.Run("isMCPTool", func(t *testing.T) {
		registry := &claudeToolSearchMCPRegistry{
			candidatesByName: map[string][]mcp.ToolCandidate{
				"get_weather": {{ResolvedTool: mcp.ResolvedTool{ServerID: 1}}},
			},
		}
		assert.True(t, registry.isMCPTool("get_weather"))
		assert.True(t, registry.isMCPTool("GET_WEATHER"))
		assert.True(t, registry.isMCPTool(" get_weather "))
		assert.False(t, registry.isMCPTool("unknown"))
		assert.False(t, registry.isMCPTool(""))

		var nilReg *claudeToolSearchMCPRegistry
		assert.False(t, nilReg.isMCPTool("get_weather"))
	})

	t.Run("selectedCandidateIndex", func(t *testing.T) {
		registry := &claudeToolSearchMCPRegistry{
			selectedIndex: map[string]int{"tool_a": 2},
		}
		assert.Equal(t, 2, registry.selectedCandidateIndex("tool_a"))
		assert.Equal(t, 0, registry.selectedCandidateIndex("tool_b"))
		assert.Equal(t, 0, registry.selectedCandidateIndex(""))

		var nilReg *claudeToolSearchMCPRegistry
		assert.Equal(t, 0, nilReg.selectedCandidateIndex("tool_a"))
	})
}

func TestDeferLoadingFieldSerialization(t *testing.T) {
	t.Parallel()

	deferTrue := true
	tool := relaymodel.ClaudeTool{
		Name:         "get_weather",
		Description:  "Get weather",
		InputSchema:  map[string]any{"type": "object"},
		DeferLoading: &deferTrue,
	}

	// Verify it serializes correctly
	data, err := json.Marshal(tool)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, true, parsed["defer_loading"])

	// Verify deserialization
	var deserialized relaymodel.ClaudeTool
	require.NoError(t, json.Unmarshal(data, &deserialized))
	require.NotNil(t, deserialized.DeferLoading)
	assert.True(t, *deserialized.DeferLoading)

	// Without defer_loading
	toolNoDeferr := relaymodel.ClaudeTool{
		Name: "regular_tool",
	}
	data2, err := json.Marshal(toolNoDeferr)
	require.NoError(t, err)
	assert.NotContains(t, string(data2), "defer_loading")
}

func TestStringPtrValue(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", stringPtrValue(nil))
	s := "test"
	assert.Equal(t, "test", stringPtrValue(&s))
}

func TestMarshalStringJSON(t *testing.T) {
	t.Parallel()

	assert.Equal(t, `"hello"`, marshalStringJSON("hello"))
	assert.Equal(t, `"with \"quotes\""`, marshalStringJSON(`with "quotes"`))
}
