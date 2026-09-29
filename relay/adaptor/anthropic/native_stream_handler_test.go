package anthropic

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newTestContext creates a gin test context with a recorder and logger.
func newTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	gmw.SetLogger(c, glog.Shared.Named("anthropic-native-stream-test"))
	return c, recorder
}

// makeSSEResponse wraps an SSE string body into an *http.Response.
func makeSSEResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
	}
}

// nativeFramedFixture supplies documented start/content/final/stop framing with cumulative usage.
func nativeFramedFixture(t *testing.T, text string, named bool, extras ...string) string {
	t.Helper()
	encoded, err := json.Marshal(text)
	require.NoError(t, err)
	events := []string{
		`{"type":"message_start","message":{"id":"msg_native","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[],"usage":{"input_tokens":21,"output_tokens":1,"cache_read_input_tokens":1000,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":200}}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":` + string(encoded) + `}}`,
		`{"type":"content_block_stop","index":0}`,
	}
	events = append(events, extras...)
	events = append(events, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":50,"cache_creation_input_tokens":300}}`, `{"type":"message_stop"}`)
	var out strings.Builder
	for _, event := range events {
		if named {
			var envelope struct {
				Type string `json:"type"`
			}
			require.NoError(t, json.Unmarshal([]byte(event), &envelope))
			out.WriteString("event: " + envelope.Type + "\n")
		}
		out.WriteString("data: " + event + "\n\n")
	}
	return out.String()
}

// TestNativeStreamFraming covers named and inferred events, large payloads, comments, pings and extensions.
func TestNativeStreamFraming(t *testing.T) {
	t.Parallel()
	for _, named := range []bool{true, false} {
		for _, large := range []bool{true, false} {
			name := "inferred"
			if named {
				name = "named"
			}
			if large {
				name += "-large"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				text := "Hello <native> & exact"
				if large {
					text = strings.Repeat("z", 128*1024)
				}
				raw := nativeFramedFixture(t, text, named, `{"type":"ping"}`, `{"type":"future_event","opaque":9007199254740993}`)
				raw = ": keepalive\n\n" + raw + "data: [DONE]\n\n"
				c, rec := newTestContext(t)
				failure, usage := ClaudeNativeStreamHandler(c, makeSSEResponse(raw))
				require.Nil(t, failure)
				require.NotNil(t, usage)
				require.Equal(t, 21, usage.PromptTokens)
				require.Equal(t, 50, usage.CompletionTokens)
				require.Equal(t, 1000, usage.PromptTokensDetails.CachedTokens)
				require.Equal(t, 100, usage.CacheWrite5mTokens)
				require.Equal(t, 200, usage.CacheWrite1hTokens)
				for _, event := range []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop", "ping", "future_event"} {
					require.Contains(t, rec.Body.String(), "event: "+event+"\n")
				}
				require.Contains(t, rec.Body.String(), `"opaque":9007199254740993`)
				require.NotContains(t, rec.Body.String(), "[DONE]")
				require.NotContains(t, rec.Body.String(), "keepalive")
				if large {
					require.Contains(t, rec.Body.String(), text)
				}
				require.True(t, strings.HasSuffix(strings.TrimSpace(rec.Body.String()), `data: {"type":"message_stop"}`))
			})
		}
	}
}

// TestNativeStreamRejectsIncompleteProtocol replaces old false-success expectations for malformed streams.
func TestNativeStreamRejectsIncompleteProtocol(t *testing.T) {
	t.Parallel()
	complete := nativeFramedFixture(t, "Hello", true)
	for name, raw := range map[string]string{
		"empty":           "",
		"missing-start":   `data: {"type":"message_stop"}` + "\n\n",
		"missing-stop":    strings.ReplaceAll(complete, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", ""),
		"empty-type":      `data: {"hello":"world"}` + "\n\n",
		"mismatched-type": strings.Replace(complete, "event: message_start", "event: ping", 1),
		"malformed":       strings.Replace(complete, `"text":"Hello"`, `"text": broken`, 1),
		"late-error":      complete + "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"busy\"}}\n\n",
		"duplicate-start": complete[:strings.Index(complete, "event: content_block_start")] + complete,
		"open-block":      strings.ReplaceAll(complete, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n", ""),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c, rec := newTestContext(t)
			failure, usage := ClaudeNativeStreamHandler(c, makeSSEResponse(raw))
			require.NotNil(t, failure)
			require.NotNil(t, usage)
			require.NotContains(t, rec.Body.String(), "event: message_stop")
			require.NotContains(t, rec.Body.String(), "[DONE]")
		})
	}
}

// TestNativeStreamErrorEventForwarded preserves an upstream error without a false completion marker.
func TestNativeStreamErrorEventForwarded(t *testing.T) {
	t.Parallel()
	c, rec := newTestContext(t)
	raw := `{"type":"error","error":{"type":"overloaded_error","message":"busy"},"request_id":"req_test"}`
	failure, usage := ClaudeNativeStreamHandler(c, makeSSEResponse("event: error\ndata: "+raw+"\n\n"))
	require.NotNil(t, failure)
	require.NotNil(t, usage)
	require.NotEmpty(t, usage.BillingEstimateReason)
	require.Contains(t, rec.Body.String(), raw)
	require.NotContains(t, rec.Body.String(), "message_stop")
}
