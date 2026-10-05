package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// securityReceiptEvent returns a real typed Responses event with one paid action.
func securityReceiptEvent(t *testing.T, padding int) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp-synthetic", "status": "completed", "output": []any{
		map[string]any{"id": "ws-synthetic", "type": "web_search_call", "status": "completed", "action": map[string]any{"type": "search", "query": "synthetic"}},
		map[string]any{"id": "msg-synthetic", "type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": strings.Repeat("x", padding)}}},
	}, "usage": map[string]any{"input_tokens": 11, "output_tokens": 7, "total_tokens": 18}}})
	require.NoError(t, err)
	return "event: response.completed\ndata: " + string(body) + "\n\n"
}

// securityStreamBody provides deterministic read/cancel/close failures after its prefix.
type securityStreamBody struct {
	io.Reader
	cancel                context.CancelFunc
	readErr, errorOnClose error
	closed                atomic.Int32
}

// Read fails only after previously supplied complete events have been processed.
func (b *securityStreamBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		if b.cancel != nil {
			b.cancel()
		}
		if b.readErr != nil {
			return n, b.readErr
		}
	}
	return n, err
}

// Close observes exactly-once transport cleanup.
func (b *securityStreamBody) Close() error { b.closed.Add(1); return b.errorOnClose }

// securityStreamFailWriter fails only a data delivery, not initial SSE header flushing.
type securityStreamFailWriter struct {
	gin.ResponseWriter
	short bool
}

// Write injects a real downstream error or a short write into the production handler.
func (w *securityStreamFailWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "data:") {
		if w.short {
			return len(p) - 1, nil
		}
		return 0, io.ErrClosedPipe
	}
	return w.ResponseWriter.Write(p)
}

// TestSecurityResponseStreamExit retains deduplicated paid actions and measured
// usage on every terminal transport path in both native and converted handlers.
func TestSecurityResponseStreamExit(t *testing.T) {
	for _, native := range []bool{true, false} {
		for _, kind := range []string{"read", "cancel", "write", "short", "close"} {
			t.Run(fmt.Sprintf("native=%v/%s", native, kind), func(t *testing.T) {
				c, _ := newDirectStreamContext()
				ctx, cancel := context.WithCancel(c.Request.Context())
				defer cancel()
				c.Request = c.Request.WithContext(ctx)
				payload := securityReceiptEvent(t, 4)
				body := &securityStreamBody{Reader: strings.NewReader(payload + payload)}
				switch kind {
				case "read":
					body.readErr = io.ErrUnexpectedEOF
				case "cancel":
					body.cancel = cancel
					body.readErr = context.Canceled
				case "write", "short":
					c.Writer = &securityStreamFailWriter{ResponseWriter: c.Writer, short: kind == "short"}
				case "close":
					body.errorOnClose = io.ErrUnexpectedEOF
				}
				resp := &http.Response{StatusCode: 200, Body: body, Header: http.Header{"Content-Type": {"text/event-stream"}}}
				var apiErr *model.ErrorWithStatusCode
				var usage *model.Usage
				if native {
					apiErr, _, usage = ResponseAPIDirectStreamHandler(c, resp, relaymode.ResponseAPI)
				} else {
					apiErr, _, usage = ResponseAPIStreamHandler(c, resp, relaymode.ChatCompletions)
				}
				require.NotNil(t, apiErr)
				if c.GetInt(ctxkey.WebSearchCallCount) != 1 {
					t.Log("REPRODUCED_488_DROPPED_TOOL_RECEIPT")
				}
				require.Equal(t, 1, c.GetInt(ctxkey.WebSearchCallCount), "an observed paid action survives failure and duplicate events")
				require.NotNil(t, usage)
				require.Equal(t, 18, usage.TotalTokens)
				require.EqualValues(t, 1, body.closed.Load(), "the upstream body must be closed on every path")
			})
		}
	}
}

// TestSecurityResponseLargeReceipt runs the actual handlers, not a standalone parser.
func TestSecurityResponseLargeReceipt(t *testing.T) {
	for _, native := range []bool{true, false} {
		t.Run(fmt.Sprint(native), func(t *testing.T) {
			c, w := newDirectStreamContext()
			event := securityReceiptEvent(t, 128<<10)
			body := &securityStreamBody{Reader: strings.NewReader(event + event + "data: [DONE]\n\n")}
			resp := &http.Response{StatusCode: 200, Body: body, Header: http.Header{"Content-Type": {"text/event-stream"}}}
			var apiErr *model.ErrorWithStatusCode
			var text string
			var usage *model.Usage
			if native {
				apiErr, text, usage = ResponseAPIDirectStreamHandler(c, resp, relaymode.ResponseAPI)
			} else {
				apiErr, text, usage = ResponseAPIStreamHandler(c, resp, relaymode.ChatCompletions)
			}
			require.Nil(t, apiErr)
			if usage == nil {
				t.Log("REPRODUCED_489_OVERSIZED_RECEIPT_BYPASS")
			}
			require.NotNil(t, usage)
			require.Equal(t, 18, usage.TotalTokens)
			require.Equal(t, 11, usage.PromptTokens)
			require.Equal(t, 7, usage.CompletionTokens)
			require.Equal(t, 1, c.GetInt(ctxkey.WebSearchCallCount))
			require.EqualValues(t, 1, body.closed.Load())
			if native {
				require.Equal(t, event+event+"data: [DONE]\n\n", w.Body.String())
				require.Equal(t, strings.Repeat("x", 128<<10), text)
			}
		})
	}
}

// TestSecurityResponseEventBound rejects unsupported and malformed large work
// before delivery, while preserving the already observed receipt on rejection.
func TestSecurityResponseEventBound(t *testing.T) {
	for _, native := range []bool{true, false} {
		for _, kind := range []string{"too_large", "malformed"} {
			t.Run(fmt.Sprintf("%v/%s", native, kind), func(t *testing.T) {
				c, w := newDirectStreamContext()
				payload := `{"type":"response.output_text.delta","delta":"` + strings.Repeat("L", (4<<20)+1) + `"}`
				if kind == "malformed" {
					payload = `{"delta":"` + strings.Repeat("M", 128<<10)
				}
				body := &securityStreamBody{Reader: strings.NewReader(securityReceiptEvent(t, 4) + "data: " + payload + "\n\ndata: [DONE]\n\n")}
				resp := &http.Response{StatusCode: 200, Body: body, Header: http.Header{"Content-Type": {"text/event-stream"}}}
				var apiErr *model.ErrorWithStatusCode
				var usage *model.Usage
				if native {
					apiErr, _, usage = ResponseAPIDirectStreamHandler(c, resp, relaymode.ResponseAPI)
				} else {
					apiErr, _, usage = ResponseAPIStreamHandler(c, resp, relaymode.ChatCompletions)
				}
				if apiErr == nil {
					t.Log("REPRODUCED_489_UNBOUNDED_EVENT_FORWARDED")
				}
				require.NotNil(t, apiErr)
				require.NotContains(t, w.Body.String(), strings.Repeat("L", 1024))
				require.NotContains(t, w.Body.String(), strings.Repeat("M", 1024))
				require.NotContains(t, w.Body.String(), "[DONE]")
				require.NotNil(t, usage)
				require.Equal(t, 18, usage.TotalTokens)
				require.Equal(t, 1, c.GetInt(ctxkey.WebSearchCallCount))
				require.EqualValues(t, 1, body.closed.Load())
			})
		}
	}
}
