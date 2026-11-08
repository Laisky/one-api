package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/common/claudeconformance"
)

// TestClaudeConformanceOpenAINonStreamBuilders drives the OpenAI adaptor's
// non-streaming Claude builders (Chat Completions and Responses) through the
// shared Anthropic Messages validator, covering stop_reason mapping, tool id
// and input normalization, sanitized tool-name restoration, and the always
// present stop_sequence/signature fields.
func TestClaudeConformanceOpenAINonStreamBuilders(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, body, stopReason string
		tools                  []string
	}{
		{
			name:       "chat_length",
			body:       `{"id":"chatcmpl-1","model":"m","choices":[{"finish_reason":"length","message":{"content":"cut"}}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`,
			stopReason: "max_tokens",
		},
		{
			name:       "chat_content_filter",
			body:       `{"id":"chatcmpl-1","model":"m","choices":[{"finish_reason":"content_filter","message":{"content":"no"}}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`,
			stopReason: "refusal",
		},
		{
			name: "chat_stop_with_tools",
			body: `{"id":"chatcmpl-1","model":"m","choices":[{"finish_reason":"stop","message":{"content":"","reasoning_content":"plan","tool_calls":[` +
				`{"id":"call_1","type":"function","function":{"name":"mcp__fs_read","arguments":""}}]}}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`,
			stopReason: "tool_use",
			tools:      []string{"mcp__fs.read"},
		},
		{
			name: "responses_incomplete",
			body: `{"id":"resp_1","object":"response","model":"m","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},` +
				`"output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"plan"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"cut"}]}],"usage":{"input_tokens":1,"output_tokens":2}}`,
			stopReason: "max_tokens",
		},
		{
			name: "responses_tools_invalid_args",
			body: `{"id":"resp_1","object":"response","model":"m","status":"completed","output":[` +
				`{"type":"function_call","id":"fc_1","call_id":"call:1","name":"mcp__fs_read","arguments":"{\"path\":"},` +
				`{"type":"function_call","id":"fc_2","call_id":"call:1","name":"mcp__fs_read","arguments":""}],"usage":{"input_tokens":1,"output_tokens":2}}`,
			stopReason: "tool_use",
			tools:      []string{"mcp__fs.read", "mcp__fs.read"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.ToolNameSanitizeMap, map[string]string{"mcp__fs_read": "mcp__fs.read"})
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}}
			converted, failure := (&Adaptor{}).convertNonStreamingToClaudeResponse(c, resp, []byte(tc.body))
			require.Nil(t, failure)
			body, err := io.ReadAll(converted.Body)
			require.NoError(t, err)
			report := claudeconformance.ValidateMessage(body)
			require.Empty(t, report.Violations, "body: %s", string(body))
			require.Equal(t, tc.stopReason, report.Message.StopReason)
			tools := report.Message.ToolUses()
			require.Len(t, tools, len(tc.tools))
			for i, name := range tc.tools {
				require.Equal(t, name, tools[i].Name)
			}
			require.EqualValues(t, 1, report.Message.Usage["input_tokens"], "billing-visible usage must be unchanged")
			require.EqualValues(t, 2, report.Message.Usage["output_tokens"], "billing-visible usage must be unchanged")
		})
	}
}
