package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/common/claudeconformance"
	"github.com/Laisky/one-api/relay/meta"
)

// geminiClaudeToolTurn is a Gemini function-call turn: Gemini reports STOP and
// a no-argument call carries no args at all.
const geminiClaudeToolTurn = `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"Paris"}}},{"functionCall":{"name":"list_files"}}]},"finishReason":"STOP","index":0}],` +
	`"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3,"totalTokenCount":8}}`

// TestClaudeConformanceGeminiToolTurn verifies both Gemini Claude renderings of
// a function-call turn report stop_reason tool_use, give the argument-less
// call an object input, and satisfy the Messages wire contract.
func TestClaudeConformanceGeminiToolTurn(t *testing.T) {
	t.Parallel()
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonstream", true: "stream"}[stream], func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
			info := &meta.Meta{ActualModelName: "gemini-2.5-flash", PromptTokens: 5}
			var report claudeconformance.Report
			if stream {
				converted, failure := (&Adaptor{}).convertStreamingToClaudeResponse(c, resp, []byte("data: "+geminiClaudeToolTurn+"\r\n\r\n"), info)
				require.Nil(t, failure)
				body, err := io.ReadAll(converted.Body)
				require.NoError(t, err)
				report = claudeconformance.ValidateStream(body)
			} else {
				converted, failure := (&Adaptor{}).convertNonStreamingToClaudeResponse(c, resp, []byte(geminiClaudeToolTurn), info)
				require.Nil(t, failure)
				body, err := io.ReadAll(converted.Body)
				require.NoError(t, err)
				report = claudeconformance.ValidateMessage(body)
			}
			require.Empty(t, report.Violations)
			require.Equal(t, "tool_use", report.Message.StopReason)
			tools := report.Message.ToolUses()
			require.Len(t, tools, 2)
			require.JSONEq(t, `{"city":"Paris"}`, tools[0].Input)
			require.JSONEq(t, `{}`, tools[1].Input)
			require.NotEqual(t, tools[0].ID, tools[1].ID)
		})
	}
}

// TestMapGeminiFinishReasonToClaude pins the finishReason mapping: safety
// blocks are refusals rather than stop_sequence (no sequence matched).
func TestMapGeminiFinishReasonToClaude(t *testing.T) {
	t.Parallel()
	for reason, want := range map[string]string{
		"STOP": "end_turn", "": "end_turn", "MAX_TOKENS": "max_tokens",
		"SAFETY": "refusal", "RECITATION": "refusal", "PROHIBITED_CONTENT": "refusal",
		"MALFORMED_FUNCTION_CALL": "end_turn",
	} {
		require.Equal(t, want, mapGeminiFinishReason(reason), reason)
	}
}
