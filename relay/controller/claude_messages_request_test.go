package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func TestRewriteClaudeRequestBody_PreservesThinkingSignatures(t *testing.T) {
	t.Parallel()

	// Simulate a real multi-turn conversation with thinking blocks and signatures
	rawBody := `{"model":"claude-opus-4-6","max_tokens":8192,"messages":[{"role":"user","content":"Hello"},{"role":"assistant","content":[{"type":"thinking","thinking":"Let me think about this...","signature":"ErQCCkYICxgCKkAshzhbP3C4GmNpfhqL2nqYBrLXBBmw4DX5yCj0aeS7pjDz=="},{"type":"text","text":"Hi there!"}]},{"role":"user","content":"Follow up question"}]}`

	request := &ClaudeMessagesRequest{
		Model:     "claude-opus-4-6",
		MaxTokens: 8192,
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	// The signature must be preserved exactly byte-for-byte
	assert.Contains(t, string(result), `"signature":"ErQCCkYICxgCKkAshzhbP3C4GmNpfhqL2nqYBrLXBBmw4DX5yCj0aeS7pjDz=="`)
	// Thinking text must be preserved exactly
	assert.Contains(t, string(result), `"thinking":"Let me think about this..."`)
	// Model should be set
	assert.Contains(t, string(result), `"model":"claude-opus-4-6"`)
}

func TestRewriteClaudeRequestBody_PreservesHTMLCharsInThinking(t *testing.T) {
	t.Parallel()

	// Thinking text contains HTML-sensitive characters that Go's json.Marshal
	// would normally escape as \u003c, \u003e, \u0026
	rawBody := `{"model":"old-model","max_tokens":1024,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"The user wants <html> tags & \"quotes\" in code","signature":"abc123base64sig=="},{"type":"text","text":"response"}]}]}`

	request := &ClaudeMessagesRequest{
		Model:     "new-model",
		MaxTokens: 1024,
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	// HTML characters must NOT be escaped to \u003c, \u003e, \u0026
	assert.Contains(t, string(result), `<html>`)
	assert.Contains(t, string(result), `& \"quotes\"`)
	assert.NotContains(t, string(result), `\u003c`)
	assert.NotContains(t, string(result), `\u003e`)
	assert.NotContains(t, string(result), `\u0026`)
	// Model should be updated
	assert.Contains(t, string(result), `"model":"new-model"`)
}

func TestRewriteClaudeRequestBody_PreservesExactSignatureBytes(t *testing.T) {
	t.Parallel()

	// Use a realistic base64 signature with +, /, = characters
	signature := "ErQCCkYICxgCKkAshzhbP3C4GmNpfhqL2nqYBrLXBBmw4DX5yCj0aeS7pjDzRhCJ3+3ASIb1/4dOZWmMuq80AsY0ZLOV6aeX8XXVEgy5FMRVt5Y7LfUQ4QcaDE510gAKyXhc8a9mYSIw2SC/L0FwLDGScqo+ayNoqVTf/mKcpgNqpihUwh2MhY8e9MAqlw3Nu2OjFLcYw4SeKpsBTttKv4tYJDfMPZ7d5ROy2p38YGsRMvHGhXlcPFWH6datYQf/xCY5jabL+PuV3h9AbOFwAtzua96l+UyPX0EnmQrimsbWZOhFp8TlXJVseY5jneDYb616yeayGDnfVrUdkflcZ7+Q6thc00QCxGvalNttxbAibjBLf1EZTJTzPtXHA7hpjRhNKgi4iBJzN+ZkH1FdJYZtQdAJBzwYAQ=="

	rawBody := `{"model":"claude-opus-4-6","max_tokens":8192,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"some thought","signature":"` + signature + `"}]}]}`

	request := &ClaudeMessagesRequest{
		Model:     "claude-opus-4-6",
		MaxTokens: 8192,
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	// Signature must be preserved exactly, including +, /, = characters
	assert.Contains(t, string(result), signature)
}

func TestRewriteClaudeRequestBody_ModelRewriting(t *testing.T) {
	t.Parallel()

	rawBody := `{"model":"original-model","max_tokens":1024,"messages":[{"role":"user","content":"hello"}]}`

	request := &ClaudeMessagesRequest{
		Model:     "mapped-model",
		MaxTokens: 1024,
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &parsed))

	var model string
	require.NoError(t, json.Unmarshal(parsed["model"], &model))
	assert.Equal(t, "mapped-model", model)
}

func TestRewriteClaudeRequestBody_DeletesExtraBody(t *testing.T) {
	t.Parallel()

	rawBody := `{"model":"claude-3","max_tokens":1024,"extra_body":{"custom":"value"},"messages":[{"role":"user","content":"hi"}]}`

	request := &ClaudeMessagesRequest{
		Model:     "claude-3",
		MaxTokens: 1024,
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	assert.NotContains(t, string(result), `"extra_body"`)
	assert.NotContains(t, string(result), `"custom"`)
}

func TestRewriteClaudeRequestBody_DeletesTopPWhenNil(t *testing.T) {
	t.Parallel()

	rawBody := `{"model":"claude-3","max_tokens":1024,"top_p":0.9,"messages":[{"role":"user","content":"hi"}]}`

	request := &ClaudeMessagesRequest{
		Model:     "claude-3",
		MaxTokens: 1024,
		TopP:      nil, // top_p should be deleted
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	assert.NotContains(t, string(result), `"top_p"`)
}

func TestRewriteClaudeRequestBody_PreservesTopPWhenSet(t *testing.T) {
	t.Parallel()

	rawBody := `{"model":"claude-3","max_tokens":1024,"top_p":0.9,"messages":[{"role":"user","content":"hi"}]}`

	topP := 0.9
	request := &ClaudeMessagesRequest{
		Model:     "claude-3",
		MaxTokens: 1024,
		TopP:      &topP,
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	assert.Contains(t, string(result), `"top_p"`)
}

func TestRewriteClaudeRequestBody_NilAndEmptyInputs(t *testing.T) {
	t.Parallel()

	t.Run("nil request", func(t *testing.T) {
		result, err := rewriteClaudeRequestBody([]byte(`{}`), nil)
		require.NoError(t, err)
		assert.Equal(t, `{}`, string(result))
	})

	t.Run("empty raw body", func(t *testing.T) {
		result, err := rewriteClaudeRequestBody(nil, &ClaudeMessagesRequest{})
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestRewriteClaudeRequestBody_PreservesMultipleSignatures(t *testing.T) {
	t.Parallel()

	// Multi-turn conversation with multiple thinking blocks each having signatures
	rawBody := `{"model":"claude-opus-4-6","max_tokens":8192,"messages":[` +
		`{"role":"user","content":"first question"},` +
		`{"role":"assistant","content":[{"type":"thinking","thinking":"thought 1","signature":"sig1AAAA=="},{"type":"text","text":"answer 1"}]},` +
		`{"role":"user","content":"second question"},` +
		`{"role":"assistant","content":[{"type":"thinking","thinking":"thought 2","signature":"sig2BBBB=="},{"type":"text","text":"answer 2"}]},` +
		`{"role":"user","content":"third question"},` +
		`{"role":"assistant","content":[{"type":"thinking","thinking":"thought 3 with <html> & special chars","signature":"sig3CCCC+/=="},{"type":"text","text":"answer 3"}]},` +
		`{"role":"user","content":"follow up"}` +
		`]}`

	request := &ClaudeMessagesRequest{
		Model:     "claude-opus-4-6",
		MaxTokens: 8192,
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	// All signatures must be preserved
	assert.Contains(t, string(result), `"signature":"sig1AAAA=="`)
	assert.Contains(t, string(result), `"signature":"sig2BBBB=="`)
	assert.Contains(t, string(result), `"signature":"sig3CCCC+/=="`)
	// HTML characters in thinking text must not be escaped
	assert.Contains(t, string(result), `<html>`)
	assert.Contains(t, string(result), `& special chars`)
}

func TestRewriteClaudeRequestBody_PreservesUnicodeInThinking(t *testing.T) {
	t.Parallel()

	rawBody := `{"model":"claude-opus-4-6","max_tokens":1024,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"用户在问关于中文的问题","signature":"sigUnicode=="},{"type":"text","text":"回答"}]}]}`

	request := &ClaudeMessagesRequest{
		Model:     "claude-opus-4-6",
		MaxTokens: 1024,
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	// Unicode text must be preserved as-is (not re-escaped)
	assert.Contains(t, string(result), `用户在问关于中文的问题`)
	assert.Contains(t, string(result), `"signature":"sigUnicode=="`)
}

func TestRewriteClaudeRequestBody_PreservesToolUseBlocks(t *testing.T) {
	t.Parallel()

	rawBody := `{"model":"claude-opus-4-6","max_tokens":8192,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"I need to use a tool","signature":"sigTool=="},{"type":"text","text":"Let me check"},{"type":"tool_use","id":"toolu_123","name":"skill_resource","input":{"action":"read","path":"some/path"}}]}]}`

	request := &ClaudeMessagesRequest{
		Model:     "claude-opus-4-6",
		MaxTokens: 8192,
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	assert.Contains(t, string(result), `"signature":"sigTool=="`)
	assert.Contains(t, string(result), `"id":"toolu_123"`)
	assert.Contains(t, string(result), `"name":"skill_resource"`)
}

func TestRewriteClaudeRequestBody_PreservesRedactedThinking(t *testing.T) {
	t.Parallel()

	// Redacted thinking blocks also have signatures
	rawBody := `{"model":"claude-opus-4-6","max_tokens":1024,"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","data":"encrypted_data_here","signature":"sigRedacted=="},{"type":"text","text":"response"}]}]}`

	request := &ClaudeMessagesRequest{
		Model:     "claude-opus-4-6",
		MaxTokens: 1024,
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	assert.Contains(t, string(result), `"type":"redacted_thinking"`)
	assert.Contains(t, string(result), `"signature":"sigRedacted=="`)
	assert.Contains(t, string(result), `"data":"encrypted_data_here"`)
}

func TestRewriteClaudeRequestBody_OutputIsValidJSON(t *testing.T) {
	t.Parallel()

	rawBody := `{"model":"old","max_tokens":1024,"extra_body":{"x":1},"top_p":0.5,"messages":[{"role":"user","content":"test"},{"role":"assistant","content":[{"type":"thinking","thinking":"<b>bold</b> & stuff","signature":"sig=="},{"type":"text","text":"ok"}]}]}`

	request := &ClaudeMessagesRequest{
		Model:     "new-model",
		MaxTokens: 1024,
		TopP:      nil,
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	// Must be valid JSON
	assert.True(t, json.Valid(result), "output must be valid JSON")

	// Parse and verify structure
	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &parsed))

	// model should be updated
	var model string
	require.NoError(t, json.Unmarshal(parsed["model"], &model))
	assert.Equal(t, "new-model", model)

	// extra_body should be removed
	_, hasExtraBody := parsed["extra_body"]
	assert.False(t, hasExtraBody)

	// top_p should be removed
	_, hasTopP := parsed["top_p"]
	assert.False(t, hasTopP)
}

func TestRewriteClaudeRequestBody_RawMessagesPreservedBytewise(t *testing.T) {
	t.Parallel()

	// The messages array should be preserved byte-for-byte
	messagesJSON := `[{"role":"user","content":"hello <world> & stuff"},{"role":"assistant","content":[{"type":"thinking","thinking":"I see <code> & 'quotes'","signature":"sigABC+/123=="},{"type":"text","text":"reply with <tags>"}]}]`
	rawBody := `{"model":"old","max_tokens":1024,"messages":` + messagesJSON + `}`

	request := &ClaudeMessagesRequest{
		Model:     "new",
		MaxTokens: 1024,
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	// The messages value should be preserved exactly
	assert.Contains(t, string(result), messagesJSON)
}

func TestCountThinkingSignatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "no signatures",
			input:    `{"messages":[{"role":"user","content":"hello"}]}`,
			expected: 0,
		},
		{
			name:     "one signature",
			input:    `{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"...","signature":"abc=="}]}]}`,
			expected: 1,
		},
		{
			name:     "multiple signatures",
			input:    `{"messages":[{"content":[{"signature":"a=="}]},{"content":[{"signature":"b=="}]},{"content":[{"signature":"c=="}]}]}`,
			expected: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			count := countThinkingSignatures([]byte(tc.input))
			assert.Equal(t, tc.expected, count)
		})
	}
}

func TestSanitizeClaudeMessagesRequest(t *testing.T) {
	t.Parallel()

	t.Run("removes top_p when temperature is set", func(t *testing.T) {
		temp := 0.7
		topP := 0.9
		req := &ClaudeMessagesRequest{
			Temperature: &temp,
			TopP:        &topP,
		}
		sanitizeClaudeMessagesRequest(req)
		assert.Nil(t, req.TopP)
		assert.NotNil(t, req.Temperature)
	})

	t.Run("preserves top_p when temperature is nil", func(t *testing.T) {
		topP := 0.9
		req := &ClaudeMessagesRequest{
			TopP: &topP,
		}
		sanitizeClaudeMessagesRequest(req)
		assert.NotNil(t, req.TopP)
	})

	t.Run("nil request does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			sanitizeClaudeMessagesRequest(nil)
		})
	})

	t.Run("Claude Opus 4.7 strips sampling and rewrites legacy thinking", func(t *testing.T) {
		temp := 0.7
		topP := 0.9
		topK := 32
		budgetTokens := 4096
		req := &ClaudeMessagesRequest{
			Model:       "claude-opus-4-7",
			Temperature: &temp,
			TopP:        &topP,
			TopK:        &topK,
			Thinking: &relaymodel.Thinking{
				Type:         "enabled",
				BudgetTokens: &budgetTokens,
			},
		}
		sanitizeClaudeMessagesRequest(req)
		assert.Nil(t, req.Temperature)
		assert.Nil(t, req.TopP)
		assert.Nil(t, req.TopK)
		require.NotNil(t, req.Thinking)
		assert.Equal(t, "adaptive", req.Thinking.Type)
		assert.Nil(t, req.Thinking.BudgetTokens)
	})
}

// TestRewriteClaudeRequestBody_BackwardCompatibility verifies that the rewrite
// function still correctly handles all the operations it did before the fix
// (model mapping, extra_body removal, top_p removal) while now also preserving
// thinking signatures.
func TestRewriteClaudeRequestBody_BackwardCompatibility(t *testing.T) {
	t.Parallel()

	rawBody := `{"model":"input-model","max_tokens":2048,"temperature":0.5,"top_p":0.8,"extra_body":{"key":"val"},"messages":[{"role":"user","content":"test"}],"system":"You are helpful","tools":[{"name":"test_tool","input_schema":{"type":"object"}}]}`

	temp := 0.5
	request := &ClaudeMessagesRequest{
		Model:       "output-model",
		MaxTokens:   2048,
		Temperature: &temp,
		TopP:        nil, // sanitized: should remove top_p
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &parsed))

	// Model mapped
	var model string
	require.NoError(t, json.Unmarshal(parsed["model"], &model))
	assert.Equal(t, "output-model", model)

	// extra_body removed
	_, hasExtraBody := parsed["extra_body"]
	assert.False(t, hasExtraBody)

	// top_p removed
	_, hasTopP := parsed["top_p"]
	assert.False(t, hasTopP)

	// Other fields preserved
	_, hasTemp := parsed["temperature"]
	assert.True(t, hasTemp)
	_, hasMessages := parsed["messages"]
	assert.True(t, hasMessages)
	_, hasSystem := parsed["system"]
	assert.True(t, hasSystem)
	_, hasTools := parsed["tools"]
	assert.True(t, hasTools)
}

func TestRewriteClaudeRequestBody_ClaudeOpus47DeletesUnsupportedFieldsAndRewritesThinking(t *testing.T) {
	t.Parallel()

	rawBody := `{"model":"claude-opus-4-7","max_tokens":2048,"temperature":0.5,"top_p":0.8,"top_k":32,"thinking":{"type":"enabled","budget_tokens":4096},"messages":[{"role":"user","content":"test"}]}`

	request := &ClaudeMessagesRequest{
		Model:     "claude-opus-4-7",
		MaxTokens: 2048,
		Thinking: &relaymodel.Thinking{
			Type: "adaptive",
		},
	}

	result, err := rewriteClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &parsed))
	_, hasTemperature := parsed["temperature"]
	assert.False(t, hasTemperature)
	_, hasTopP := parsed["top_p"]
	assert.False(t, hasTopP)
	_, hasTopK := parsed["top_k"]
	assert.False(t, hasTopK)

	var thinking map[string]any
	require.NoError(t, json.Unmarshal(parsed["thinking"], &thinking))
	assert.Equal(t, "adaptive", thinking["type"])
	_, hasBudgetTokens := thinking["budget_tokens"]
	assert.False(t, hasBudgetTokens)
}

func TestStripClaudeThinkingFromAssistantHistory(t *testing.T) {
	t.Parallel()

	rawBody := `{"model":"claude-opus-4-6","thinking":{"type":"adaptive"},"metadata":{"trace":"abc"},"messages":[{"role":"user","content":"Hello"},{"role":"assistant","content":[{"type":"thinking","thinking":"plan","signature":"sig1=="},{"type":"text","text":"Answer"},{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"q":"x"}}]},{"role":"assistant","content":[{"type":"redacted_thinking","data":"opaque","signature":"sig2=="}]},{"role":"user","content":"Follow up"}]}`

	sanitized, stats, err := stripClaudeThinkingFromAssistantHistory([]byte(rawBody))
	require.NoError(t, err)
	require.True(t, json.Valid(sanitized))
	assert.Equal(t, 2, stats.RemovedThinkingBlocks)
	assert.Equal(t, 1, stats.RemovedAssistantMessages)
	assert.NotContains(t, string(sanitized), `"type":"thinking"`)
	assert.NotContains(t, string(sanitized), `"type":"redacted_thinking"`)
	assert.Contains(t, string(sanitized), `"type":"text","text":"Answer"`)
	assert.Contains(t, string(sanitized), `"type":"tool_use","id":"toolu_1"`)
	assert.Contains(t, string(sanitized), `"metadata":{"trace":"abc"}`)
	assert.Contains(t, string(sanitized), `"thinking":{"type":"adaptive"}`)

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(sanitized, &parsed))
	var messages []map[string]any
	require.NoError(t, json.Unmarshal(parsed["messages"], &messages))
	require.Len(t, messages, 3)
	assert.Equal(t, "assistant", messages[1]["role"])
	content := messages[1]["content"].([]any)
	require.Len(t, content, 2)
	firstBlock := content[0].(map[string]any)
	assert.Equal(t, "text", firstBlock["type"])
}

func TestStripClaudeThinkingFromAssistantHistory_NoThinkingBlocks(t *testing.T) {
	t.Parallel()

	rawBody := `{"model":"claude-opus-4-6","messages":[{"role":"user","content":"Hello"},{"role":"assistant","content":[{"type":"text","text":"Answer"}]}]}`

	sanitized, stats, err := stripClaudeThinkingFromAssistantHistory([]byte(rawBody))
	require.NoError(t, err)
	assert.Equal(t, rawBody, string(sanitized))
	assert.Equal(t, 0, stats.RemovedThinkingBlocks)
	assert.Equal(t, 0, stats.RemovedAssistantMessages)
}

func TestStripClaudeUnsignedThinkingFromAssistantHistory(t *testing.T) {
	t.Parallel()

	rawBody := `{"model":"claude-opus-4-6","messages":[{"role":"user","content":"Hello"},{"role":"assistant","content":[{"type":"thinking","thinking":"signed","signature":"sig1=="},{"type":"text","text":"Answer 1"}]},{"role":"assistant","content":[{"type":"thinking","thinking":"missing signature"},{"type":"text","text":"Answer 2"},{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"q":"x"}}]},{"role":"assistant","content":[{"type":"redacted_thinking","data":"opaque"}]},{"role":"user","content":"Follow up"}]}`

	sanitized, stats, err := stripClaudeUnsignedThinkingFromAssistantHistory([]byte(rawBody))
	require.NoError(t, err)
	require.True(t, json.Valid(sanitized))
	assert.Equal(t, 2, stats.RemovedThinkingBlocks)
	assert.Equal(t, 1, stats.RemovedAssistantMessages)
	assert.Equal(t, []string{"messages[2].content[0]", "messages[3].content[0]"}, stats.Locations)
	assert.Contains(t, string(sanitized), `"signature":"sig1=="`)
	assert.Contains(t, string(sanitized), `"type":"tool_use","id":"toolu_1"`)
	assert.NotContains(t, string(sanitized), `"thinking":"missing signature"`)
	assert.NotContains(t, string(sanitized), `"type":"redacted_thinking","data":"opaque"`)

	var parsed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(sanitized, &parsed))
	var messages []map[string]any
	require.NoError(t, json.Unmarshal(parsed["messages"], &messages))
	require.Len(t, messages, 4)
}

func TestRewriteAndSanitizeClaudeRequestBody(t *testing.T) {
	t.Parallel()

	// Body with model to rewrite, extra_body to remove, and unsigned thinking to strip
	rawBody := `{"model":"old-model","extra_body":{"foo":"bar"},"messages":[{"role":"user","content":"Hello"},{"role":"assistant","content":[{"type":"thinking","thinking":"signed","signature":"sig1=="},{"type":"text","text":"Answer 1"}]},{"role":"assistant","content":[{"type":"thinking","thinking":"missing signature"},{"type":"text","text":"Answer 2"}]}]}`

	request := &ClaudeMessagesRequest{Model: "new-model"}
	result, stats, err := rewriteAndSanitizeClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)
	require.True(t, json.Valid(result))

	// Verify rewrite: model changed, extra_body removed
	assert.Contains(t, string(result), `"new-model"`)
	assert.NotContains(t, string(result), `"old-model"`)
	assert.NotContains(t, string(result), `"extra_body"`)

	// Verify sanitize: unsigned thinking removed, signed preserved
	assert.Equal(t, 1, stats.RemovedThinkingBlocks)
	assert.Contains(t, string(result), `"signature":"sig1=="`)
	assert.NotContains(t, string(result), `"thinking":"missing signature"`)
}

func TestRewriteAndSanitizeClaudeRequestBody_NoThinkingToStrip(t *testing.T) {
	t.Parallel()

	rawBody := `{"model":"old-model","messages":[{"role":"user","content":"Hello"},{"role":"assistant","content":[{"type":"thinking","thinking":"signed","signature":"sig1=="},{"type":"text","text":"Answer"}]}]}`

	request := &ClaudeMessagesRequest{Model: "new-model"}
	result, stats, err := rewriteAndSanitizeClaudeRequestBody([]byte(rawBody), request)
	require.NoError(t, err)
	require.True(t, json.Valid(result))

	assert.Contains(t, string(result), `"new-model"`)
	assert.Equal(t, 0, stats.RemovedThinkingBlocks)
	assert.Contains(t, string(result), `"signature":"sig1=="`)
}

func TestStripClaudeUnsignedThinkingFromAssistantHistory_NoUnsignedThinking(t *testing.T) {
	t.Parallel()

	rawBody := `{"model":"claude-opus-4-6","messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"signed","signature":"sig1=="},{"type":"text","text":"Answer"}]}]}`

	sanitized, stats, err := stripClaudeUnsignedThinkingFromAssistantHistory([]byte(rawBody))
	require.NoError(t, err)
	assert.Equal(t, rawBody, string(sanitized))
	assert.Equal(t, 0, stats.RemovedThinkingBlocks)
	assert.Equal(t, 0, stats.RemovedAssistantMessages)
	assert.Empty(t, stats.Locations)
}
