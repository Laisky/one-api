package openai

import (
	"context"
	"fmt"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSecurityResponseExactEventBoundary pins below/at/above the documented
// payload limit without relying on the implementation constant as its oracle.
func TestSecurityResponseExactEventBoundary(t *testing.T) {
	for _, native := range []bool{true, false} {
		for _, size := range []int{(4 << 20) - 1, 4 << 20, (4 << 20) + 1} {
			t.Run(fmt.Sprintf("%v/%d", native, size), func(t *testing.T) {
				c, w := newDirectStreamContext()
				prefix := `{"type":"response.completed","response":{"id":"boundary","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}},"padding":"`
				payload := prefix + strings.Repeat("B", size-len(prefix)-2) + `"}`
				require.Len(t, payload, size)
				body := &securityStreamBody{Reader: strings.NewReader("data: " + payload + "\n\ndata: [DONE]\n\n")}
				resp := &http.Response{StatusCode: 200, Body: body, Header: http.Header{"Content-Type": {"text/event-stream"}}}
				var apiErr *model.ErrorWithStatusCode
				var usage *model.Usage
				if native {
					apiErr, _, usage = ResponseAPIDirectStreamHandler(c, resp, relaymode.ResponseAPI)
				} else {
					apiErr, _, usage = ResponseAPIStreamHandler(c, resp, relaymode.ChatCompletions)
				}
				if size > 4<<20 {
					require.NotNil(t, apiErr)
					require.NotContains(t, w.Body.String(), strings.Repeat("B", 1024))
				} else {
					require.Nil(t, apiErr)
					require.NotNil(t, usage)
					require.Equal(t, 5, usage.TotalTokens)
				}
				require.EqualValues(t, 1, body.closed.Load())
			})
		}
	}
}

// securityStalledResponse models a provider that stalls inside a large payload.
type securityStalledResponse struct {
	io.Reader
	cancel context.CancelFunc
	closed chan struct{}
	once   sync.Once
}

// Read cancels the downstream only after enough bytes reached the real large-line path.
func (b *securityStalledResponse) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		b.cancel()
		<-b.closed
		return n, io.ErrClosedPipe
	}
	return n, err
}

// Close releases the blocked read without touching a pooled request context.
func (b *securityStalledResponse) Close() error { b.once.Do(func() { close(b.closed) }); return nil }

// TestSecurityResponseLargeCancellation requires cancellation itself to unblock
// a provider read; a bounded watchdog fails the baseline instead of hanging CI.
func TestSecurityResponseLargeCancellation(t *testing.T) {
	for _, native := range []bool{true, false} {
		t.Run(fmt.Sprint(native), func(t *testing.T) {
			c, _ := newDirectStreamContext()
			ctx, cancel := context.WithCancel(c.Request.Context())
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			body := &securityStalledResponse{Reader: strings.NewReader(`data: {"type":"response.output_text.delta","delta":"` + strings.Repeat("S", 128<<10)), cancel: cancel, closed: make(chan struct{})}
			var watchdog atomic.Bool
			timer := time.AfterFunc(time.Second, func() { watchdog.Store(true); _ = body.Close() })
			defer timer.Stop()
			resp := &http.Response{StatusCode: 200, Body: body, Header: http.Header{"Content-Type": {"text/event-stream"}}}
			var apiErr *model.ErrorWithStatusCode
			if native {
				apiErr, _, _ = ResponseAPIDirectStreamHandler(c, resp, relaymode.ResponseAPI)
			} else {
				apiErr, _, _ = ResponseAPIStreamHandler(c, resp, relaymode.ChatCompletions)
			}
			require.False(t, watchdog.Load(), "downstream cancellation must close a blocked upstream body")
			require.NotNil(t, apiErr)
		})
	}
}
