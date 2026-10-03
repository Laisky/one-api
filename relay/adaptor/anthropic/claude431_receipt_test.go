package anthropic

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// claude431Recorder supplies the close notification used by streaming responses in tests.
type claude431Recorder struct {
	*httptest.ResponseRecorder
	closed chan bool
}

// CloseNotify returns a request-local notification channel without a shared goroutine.
func (r *claude431Recorder) CloseNotify() <-chan bool { return r.closed }

// claude431Stream builds cumulative provider receipts without external inference calls.
func claude431Stream(complete bool) string {
	events := []string{
		`{"type":"message_start","message":{"id":"msg_receipt","type":"message","model":"claude-sonnet-5-5","role":"assistant","content":[],"usage":{"input_tokens":21,"output_tokens":1,"cache_read_input_tokens":1000,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":200}}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":21,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":300}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":21,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":300}}`,
	}
	if complete {
		events = append(events, `{"type":"message_stop"}`)
	}
	return "data: " + strings.Join(events, "\n\ndata: ") + "\n\n"
}

// TestClaude431CumulativeReceipts requires both native and converted streams to settle each bucket once.
func TestClaude431CumulativeReceipts(t *testing.T) {
	t.Parallel()
	for _, native := range []bool{false, true} {
		for _, complete := range []bool{false, true} {
			name := "converted"
			if native {
				name = "native"
			}
			if !complete {
				name += "-truncated"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				rec := &claude431Recorder{ResponseRecorder: httptest.NewRecorder(), closed: make(chan bool)}
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
				response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(claude431Stream(complete)))}
				handler := StreamHandler
				if native {
					handler = ClaudeNativeStreamHandler
				}
				failure, usage := handler(c, response)
				if complete {
					require.Nil(t, failure)
				} else {
					require.NotNil(t, failure, "premature EOF is not successful completion")
				}
				require.NotNil(t, usage)
				require.Equal(t, 21, usage.PromptTokens)
				require.Equal(t, 50, usage.CompletionTokens)
				require.Equal(t, 71, usage.TotalTokens)
				require.NotNil(t, usage.PromptTokensDetails)
				require.Equal(t, 1000, usage.PromptTokensDetails.CachedTokens)
				require.Equal(t, 100, usage.CacheWrite5mTokens)
				require.Equal(t, 200, usage.CacheWrite1hTokens)
				if !complete {
					require.NotContains(t, rec.Body.String(), "[DONE]")
				}
			})
		}
	}
}

// TestClaude431NativeJSONPreservesOpaqueFields verifies native forwarding retains signed and future payloads.
func TestClaude431NativeJSONPreservesOpaqueFields(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	raw := `{"id":"msg_exact","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[{"type":"thinking","thinking":"","signature":"<sig>&","future_number":9007199254740993},{"type":"text","text":"Hello"}],"stop_reason":"end_turn","stop_details":{"type":"future"},"future_root":9007199254740993,"usage":{"input_tokens":21,"output_tokens":50}}`
	response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(raw))}
	failure, usage := ClaudeNativeHandler(c, response, 0, "claude-sonnet-5-5")
	require.Nil(t, failure)
	require.NotNil(t, usage)
	require.Contains(t, rec.Body.String(), `"future_number":9007199254740993`)
	require.Contains(t, rec.Body.String(), `"future_root":9007199254740993`)
	require.Contains(t, rec.Body.String(), `"stop_details":{"type":"future"}`)
	require.Equal(t, &model.Usage{PromptTokens: 21, CompletionTokens: 50, TotalTokens: 71}, usage)
}
