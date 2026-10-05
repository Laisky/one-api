package anthropic

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// TestClaude431ResponsesControls verifies actual Responses-to-Chat-to-Claude conversion for all five effort levels.
func TestClaude431ResponsesControls(t *testing.T) {
	t.Parallel()
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		t.Run(effort, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			var request openai.ResponseAPIRequest
			raw := `{"model":"claude-sonnet-5-5","input":"Hello","max_output_tokens":64,"reasoning":{"effort":"` + effort + `"},"thinking":{"type":"adaptive"},"temperature":0.4}`
			require.NoError(t, json.Unmarshal([]byte(raw), &request))
			chat, err := openai.ConvertResponseAPIToChatCompletionRequest(&request)
			require.NoError(t, err)
			converted, err := ConvertRequest(c, *chat)
			require.NoError(t, err)
			body, err := json.Marshal(converted)
			require.NoError(t, err)
			require.Contains(t, string(body), `"output_config":{"effort":"`+effort+`"}`)
			require.Contains(t, string(body), `"max_tokens":64`)
			require.NotContains(t, string(body), `"temperature"`)
			require.ElementsMatch(t, []string{"low", "medium", "high", "xhigh", "max"}, ModelRatios["claude-sonnet-5-5"].SupportedReasoningEfforts)
			require.Equal(t, "high", ModelRatios["claude-sonnet-5-5"].DefaultReasoningEffort)
		})
	}
}

// TestClaude431FinalBodyPolicy verifies final wire normalization, exact native fields, and unknown-model isolation.
func TestClaude431FinalBodyPolicy(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"claude-sonnet-5-5", "anthropic/claude-sonnet-5.5", "custom-model"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw := `{"model":"` + name + `","max_tokens":64,"thinking":{"type":"disabled"},"temperature":0.4,"output_config":{"effort":"low","id":9007199254740993},"messages":[{"role":"user","content":"hello <world>&"}]}`
			reader := strings.NewReader(raw)
			prepared, err := PrepareRequestBody(nil, name, reader)
			require.NoError(t, err)
			body, err := io.ReadAll(prepared)
			require.NoError(t, err)
			require.Contains(t, string(body), `"id":9007199254740993`)
			require.Contains(t, string(body), `hello <world>&`)
			if name == "claude-sonnet-5-5" {
				require.Contains(t, string(body), `"type":"between_tools"`)
				require.NotContains(t, string(body), `"temperature"`)
			} else {
				require.Equal(t, raw, string(body))
			}
		})
	}
}

// claudeFailWriter injects downstream failed or short writes without sending data over a network.
type claudeFailWriter struct {
	gin.ResponseWriter
	short bool
}

// Write returns a deliberate downstream delivery failure for receipt-retention tests.
func (w *claudeFailWriter) Write(p []byte) (int, error) {
	if w.short {
		return 0, nil
	}
	return 0, io.ErrClosedPipe
}

// TestClaude431FailedJSONDeliveryRetainsReceipt verifies paid usage survives native and converted write failures.
func TestClaude431FailedJSONDeliveryRetainsReceipt(t *testing.T) {
	t.Parallel()
	for _, native := range []bool{false, true} {
		for _, short := range []bool{false, true} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			c.Writer = &claudeFailWriter{ResponseWriter: c.Writer, short: short}
			raw := `{"id":"msg","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":21,"output_tokens":50}}`
			handler := Handler
			if native {
				handler = ClaudeNativeHandler
			}
			failure, usage := handler(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw))}, 0, "claude-sonnet-5-5")
			require.NotNil(t, failure)
			require.Equal(t, &model.Usage{PromptTokens: 21, CompletionTokens: 50, TotalTokens: 71}, usage)
		}
	}
}

// TestClaude431UsageValidation rejects malformed counters atomically and preserves the prior complete receipt.
func TestClaude431UsageValidation(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{`{"output_tokens":-1}`, `{"output_tokens":1.5}`, `{"input_tokens":"12"}`, `{"output_tokens":9223372036854775807}`, `{"cache_creation_input_tokens":301}`} {
		t.Run(bad, func(t *testing.T) {
			t.Parallel()
			var receipt httpUsageReceipt
			require.NoError(t, receipt.apply(json.RawMessage(`{"input_tokens":21,"output_tokens":50,"cache_creation_input_tokens":300,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":200}}`)))
			before := receipt.snapshot()
			require.Error(t, receipt.apply(json.RawMessage(bad)))
			require.Equal(t, before, receipt.snapshot())
		})
	}
}
