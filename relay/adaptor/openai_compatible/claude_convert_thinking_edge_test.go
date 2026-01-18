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

func TestThinkingFieldPrecedence_AllFieldsPresent(t *testing.T) {
	t.Parallel()
	body := `{
		"id":"chatcmpl-all-fields-test",
		"model":"claude-3.5-sonnet",
		"object":"chat.completion",
		"created": 1234567890,
		"choices":[{
			"index":0,
			"message":{
				"role":"assistant",
				"content":"Main response",
				"thinking": "Thinking wins",
				"reasoning_content": "ReasoningContent fallback",
				"reasoning": "Reasoning fallback"
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

	hasThinking := false
	for _, c := range cr.Content {
		if c.Type == "thinking" && c.Thinking == "Thinking wins" {
			hasThinking = true
		}
	}
	assert.True(t, hasThinking, "Should prefer 'thinking' field when all are present")
}

func TestNoThinkingBlock_AllFieldsEmpty(t *testing.T) {
	t.Parallel()
	body := `{
		"id":"chatcmpl-empty-fields-test",
		"model":"claude-3.5-sonnet",
		"object":"chat.completion",
		"created": 1234567890,
		"choices":[{
			"index":0,
			"message":{
				"role":"assistant",
				"content":"Main response",
				"thinking": "",
				"reasoning_content": "",
				"reasoning": ""
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

	hasThinking := false
	for _, c := range cr.Content {
		if c.Type == "thinking" && c.Thinking != "" {
			hasThinking = true
		}
	}
	assert.False(t, hasThinking, "Should not add thinking block if all fields are empty")
}

func TestMalformedResponse_MissingFields(t *testing.T) {
	t.Parallel()
	body := `{
		"id":"chatcmpl-malformed-test",
		"model":"claude-3.5-sonnet",
		"object":"chat.completion",
		"created": 1234567890,
		"choices":[{
			"index":0,
			"message":{
				"role":"assistant",
				"content":"Main response"
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

	hasThinking := false
	for _, c := range cr.Content {
		if c.Type == "thinking" && c.Thinking != "" {
			hasThinking = true
		}
	}
	assert.False(t, hasThinking, "Should not add thinking block if fields are missing")
}

func TestReasoningFieldOnly(t *testing.T) {
	t.Parallel()
	body := `{
		"id":"chatcmpl-reasoning-only-test",
		"model":"claude-3.5-sonnet",
		"object":"chat.completion",
		"created": 1234567890,
		"choices":[{
			"index":0,
			"message":{
				"role":"assistant",
				"content":"Main response",
				"reasoning": "Reasoning only content"
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

	hasThinking := false
	for _, c := range cr.Content {
		if c.Type == "thinking" && c.Thinking == "Reasoning only content" {
			hasThinking = true
		}
	}
	assert.True(t, hasThinking, "Should extract thinking block from reasoning field if present")
}
