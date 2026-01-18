package openai_compatible

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func TestConvertOpenAIResponseToClaudeResponse_ChatCompletionsWithThinking(t *testing.T) {
	t.Parallel()
	// Build a chat completion response with thinking content
	body := `{
		"id":"chatcmpl-thinking-test",
		"model":"claude-3.5-sonnet",
		"object":"chat.completion",
		"created": 1234567890,
		"choices":[{
			"index":0,
			"message":{
				"role":"assistant",
				"content":"This is the main response",
				"thinking": "This is the thinking content that should be extracted"
			},
			"finish_reason":"stop"
		}],
		"usage": {"prompt_tokens":10, "completion_tokens":15, "total_tokens":25}
	}`

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	got, errResp := ConvertOpenAIResponseToClaudeResponse(nil, resp)
	require.Nil(t, errResp)
	require.NotNil(t, got)

	outBody, rerr := io.ReadAll(got.Body)
	require.NoError(t, rerr)

	var cr relaymodel.ClaudeResponse
	require.NoError(t, json.Unmarshal(outBody, &cr))

	assert.Equal(t, "claude-3.5-sonnet", cr.Model)
	assert.Equal(t, 10, cr.Usage.InputTokens)
	assert.Equal(t, 15, cr.Usage.OutputTokens)
	assert.Equal(t, "end_turn", cr.StopReason)

	// Verify thinking content is extracted
	assert.Len(t, cr.Content, 2) // Should have thinking and text blocks

	hasThinking := false
	hasText := false
	for _, c := range cr.Content {
		if c.Type == "thinking" && c.Thinking == "This is the thinking content that should be extracted" {
			hasThinking = true
		}
		if c.Type == "text" && c.Text == "This is the main response" {
			hasText = true
		}
	}

	assert.True(t, hasThinking, "Should have thinking content block")
	assert.True(t, hasText, "Should have text content block")
}

func TestConvertOpenAIResponseToClaudeResponse_ChatCompletionsWithReasoningContent(t *testing.T) {
	t.Parallel()
	// Build a chat completion response with reasoning_content (fallback case)
	body := `{
		"id":"chatcmpl-reasoning-test",
		"model":"deepseek-reasoner",
		"object":"chat.completion",
		"created": 1234567890,
		"choices":[{
			"index":0,
			"message":{
				"role":"assistant",
				"content":"This is the main response",
				"reasoning_content": "This is reasoning content that should be converted to thinking"
			},
			"finish_reason":"stop"
		}],
		"usage": {"prompt_tokens":8, "completion_tokens":12, "total_tokens":20}
	}`

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	got, errResp := ConvertOpenAIResponseToClaudeResponse(nil, resp)
	require.Nil(t, errResp)
	require.NotNil(t, got)

	outBody, rerr := io.ReadAll(got.Body)
	require.NoError(t, rerr)

	var cr relaymodel.ClaudeResponse
	require.NoError(t, json.Unmarshal(outBody, &cr))

	assert.Equal(t, "deepseek-reasoner", cr.Model)
	assert.Equal(t, 8, cr.Usage.InputTokens)
	assert.Equal(t, 12, cr.Usage.OutputTokens)
	assert.Equal(t, "end_turn", cr.StopReason)

	// Verify reasoning_content is converted to thinking
	assert.Len(t, cr.Content, 2) // Should have thinking and text blocks

	hasThinking := false
	hasText := false
	for _, c := range cr.Content {
		if c.Type == "thinking" && c.Thinking == "This is reasoning content that should be converted to thinking" {
			hasThinking = true
		}
		if c.Type == "text" && c.Text == "This is the main response" {
			hasText = true
		}
	}

	assert.True(t, hasThinking, "Should have thinking content block converted from reasoning_content")
	assert.True(t, hasText, "Should have text content block")
}
