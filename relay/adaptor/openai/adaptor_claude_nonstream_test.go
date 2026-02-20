package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

func TestConvertNonStreamingToClaudeResponse_PassThroughInvalidBody(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	body := []byte(`{"error":"bad_request"`)

	converted, errWithStatus := (&Adaptor{}).convertNonStreamingToClaudeResponse(nil, resp, body)
	require.Nil(t, errWithStatus)
	require.NotNil(t, converted)
	require.Equal(t, http.StatusBadRequest, converted.StatusCode)

	convertedBody, err := io.ReadAll(converted.Body)
	require.NoError(t, err)
	require.Equal(t, string(body), string(convertedBody))
}

func TestConvertNonStreamingToClaudeResponse_ConvertsChatCompletion(t *testing.T) {
	t.Parallel()

	upstreamBody := `{
		"id":"chatcmpl-1",
		"model":"gpt-4o-mini",
		"choices":[{
			"finish_reason":"tool_calls",
			"message":{
				"content":"hello",
				"tool_calls":[{
					"id":"tool_1",
					"type":"function",
					"function":{"name":"lookup","arguments":"{\"city\":\"Paris\"}"}
				}]
			}
		}],
		"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}
	}`

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	converted, errWithStatus := (&Adaptor{}).convertNonStreamingToClaudeResponse(nil, resp, []byte(upstreamBody))
	require.Nil(t, errWithStatus)
	require.NotNil(t, converted)
	require.Equal(t, "application/json", converted.Header.Get("Content-Type"))
	require.NotEmpty(t, converted.Header.Get("Content-Length"))

	body, err := io.ReadAll(converted.Body)
	require.NoError(t, err)

	var claudeResp model.ClaudeResponse
	require.NoError(t, json.Unmarshal(body, &claudeResp))
	require.Equal(t, "chatcmpl-1", claudeResp.ID)
	require.Equal(t, "assistant", claudeResp.Role)
	require.Equal(t, "tool_use", claudeResp.StopReason)
	require.Equal(t, 11, claudeResp.Usage.InputTokens)
	require.Equal(t, 7, claudeResp.Usage.OutputTokens)
	require.GreaterOrEqual(t, len(claudeResp.Content), 2)
	require.Equal(t, "text", claudeResp.Content[0].Type)
	require.Equal(t, "hello", claudeResp.Content[0].Text)
	require.Equal(t, "tool_use", claudeResp.Content[1].Type)
	require.Equal(t, "lookup", claudeResp.Content[1].Name)
	require.Equal(t, "tool_1", claudeResp.Content[1].ID)
	require.True(t, strings.Contains(string(claudeResp.Content[1].Input), "Paris"))
}
