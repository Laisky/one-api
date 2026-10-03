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
)

// TestClaude431ReviewNonterminalFinishReason keeps incremental chunks from looking terminal.
// It checks the shared converter used by HTTP and cloud transports without network calls.
func TestClaude431ReviewNonterminalFinishReason(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool_1","name":"lookup","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
		`{"type":"message_delta","delta":{},"usage":{"output_tokens":2}}`,
		`{"type":"ping"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			var event StreamResponse
			require.NoError(t, json.Unmarshal([]byte(raw), &event))
			chunk, _ := StreamResponseClaude2OpenAI(c, &event)
			require.NotNil(t, chunk)
			require.Len(t, chunk.Choices, 1)
			require.Nil(t, chunk.Choices[0].FinishReason)
			encoded, err := json.Marshal(chunk)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), `"finish_reason":""`)
		})
	}
	t.Run("terminal", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		var event StreamResponse
		require.NoError(t, json.Unmarshal([]byte(`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}`), &event))
		chunk, _ := StreamResponseClaude2OpenAI(c, &event)
		require.NotNil(t, chunk.Choices[0].FinishReason)
		require.Equal(t, "tool_calls", *chunk.Choices[0].FinishReason)
	})
}

// TestClaude431ReviewJSONErrorAccounting distinguishes explicit admission rejection from unknown paid work.
// Complete JSON errors can be classified without treating arbitrary 200 or 5xx responses as free.
func TestClaude431ReviewJSONErrorAccounting(t *testing.T) {
	t.Parallel()
	for _, native := range []bool{false, true} {
		for _, tc := range []struct {
			name, raw string
			status    int
			rejected  bool
			observed  bool
		}{
			{"invalid_request", `{"type":"error","error":{"type":"invalid_request_error","message":"invalid input"}}`, 400, true, false},
			{"authentication", `{"type":"error","error":{"type":"authentication_error","message":"bad credential"}}`, 401, true, false},
			{"billing", `{"type":"error","error":{"type":"billing_error","message":"account billing"}}`, 402, true, false},
			{"permission", `{"type":"error","error":{"type":"permission_error","message":"denied"}}`, 403, true, false},
			{"not_found", `{"type":"error","error":{"type":"not_found_error","message":"not found"}}`, 404, true, false},
			{"too_large", `{"type":"error","error":{"type":"request_too_large","message":"too large"}}`, 413, true, false},
			{"rate_limit", `{"type":"error","error":{"type":"rate_limit_error","message":"rate limited"}}`, 429, true, false},
			{"internal", `{"type":"error","error":{"type":"api_error","message":"internal failure"}}`, 502, false, false},
			{"overload", `{"type":"error","error":{"type":"overloaded_error","message":"overload"}}`, 502, false, false},
			{"timeout", `{"type":"error","error":{"type":"timeout_error","message":"timeout"}}`, 502, false, false},
			{"unknown", `{"type":"error","error":{"type":"future_error","message":"future"}}`, 502, false, false},
			{"contradictory_receipt", `{"type":"error","error":{"type":"invalid_request_error","message":"invalid"},"usage":{"input_tokens":3,"output_tokens":7}}`, 502, false, true},
			{"unknown_usage", `{"type":"error","error":{"type":"invalid_request_error","message":"invalid"},"usage":{"future_tokens":7}}`, 502, false, false},
			{"message_not_error", `{"type":"message","error":{"type":"invalid_request_error","message":"invalid"}}`, 502, false, false},
			{"truncated_json", `{"type":"error","error":{"type":"invalid_request_error","message":"invalid"}`, 502, false, false},
		} {
			name := tc.name + "/converted"
			if native {
				name = tc.name + "/native"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.raw))}
				handler := Handler
				if native {
					handler = ClaudeNativeHandler
				}
				failure, usage := handler(c, response, 4784, "claude-sonnet-5-5")
				require.NotNil(t, failure)
				require.Equal(t, tc.status, failure.StatusCode)
				require.Empty(t, w.Body.String(), "a failed JSON response must not commit a success body")
				if tc.rejected {
					require.Nil(t, usage, "a documented admission rejection is eligible for the existing refund path")
				} else {
					require.NotNil(t, usage, "uncertain or observed paid work must not use the nil-usage refund path")
					if tc.observed {
						require.Equal(t, 3, usage.PromptTokens)
						require.Equal(t, 7, usage.CompletionTokens)
						require.Empty(t, usage.BillingEstimateReason)
					} else {
						require.NotEmpty(t, usage.BillingEstimateReason)
					}
				}
			})
		}
	}
}

// claude431ReviewBody injects transport failures after a complete-looking error payload.
type claude431ReviewBody struct {
	io.Reader
	readErr, closeErr error
}

// Read returns the configured error on EOF, preserving all earlier payload bytes.
func (b *claude431ReviewBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF && b.readErr != nil {
		return n, b.readErr
	}
	return n, err
}

// Close returns the configured transport-close result without discarding it.
func (b *claude431ReviewBody) Close() error { return b.closeErr }

// TestClaude431ReviewAdmissionProof requires complete transport and unambiguous admission evidence.
// No partial receipt, future usage field, conflicting status, or transport error can authorize a refund.
func TestClaude431ReviewAdmissionProof(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, usage       string
		status            int
		readErr, closeErr error
		admission         bool
	}{
		{name: "absent", status: 200, admission: true},
		{name: "null", usage: `,"usage":null`, status: 200, admission: true},
		{name: "real_status", status: 400, admission: true},
		{name: "conflicting_status", status: 500},
		{name: "empty_usage", usage: `,"usage":{}`, status: 200},
		{name: "explicit_zero", usage: `,"usage":{"input_tokens":0,"output_tokens":0}`, status: 200},
		{name: "cache_only", usage: `,"usage":{"cache_read_input_tokens":100}`, status: 200},
		{name: "invalid_receipt", usage: `,"usage":{"input_tokens":-1}`, status: 200},
		{name: "read_failure", status: 200, readErr: io.ErrUnexpectedEOF},
		{name: "close_failure", status: 200, closeErr: io.ErrClosedPipe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			body := `{"type":"error","error":{"type":"invalid_request_error","message":"rejected"}` + tc.usage + `}`
			failure, usage := Handler(c, &http.Response{StatusCode: tc.status, Body: &claude431ReviewBody{Reader: strings.NewReader(body), readErr: tc.readErr, closeErr: tc.closeErr}}, 0, "claude-sonnet-5-5")
			require.NotNil(t, failure)
			require.Equal(t, tc.admission, IsAdmissionRejection(failure))
			require.Equal(t, tc.admission, usage == nil)
		})
	}
	require.False(t, IsAdmissionRejection(nil))
}
