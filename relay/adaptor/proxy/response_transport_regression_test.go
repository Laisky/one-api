package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
)

// proxyObservedBody records closure and can assert behavior before a later read.
type proxyObservedBody struct {
	reader     io.Reader
	closed     bool
	reads      int
	beforeRead func(int)
}

// Read delegates a network-sized read after checking the preceding write.
func (b *proxyObservedBody) Read(p []byte) (int, error) {
	b.reads++
	if b.beforeRead != nil {
		b.beforeRead(b.reads)
	}
	return b.reader.Read(p)
}

// Close records upstream body ownership without performing network I/O.
func (b *proxyObservedBody) Close() error {
	b.closed = true
	return nil
}

// proxyBrokenWriter simulates a disconnected client or a short successful write.
type proxyBrokenWriter struct {
	gin.ResponseWriter
	short bool
}

// Write deterministically fails without retaining response bytes.
func (w proxyBrokenWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}
	return 0, io.ErrClosedPipe
}

// proxyErrorReader injects an upstream interruption after the preceding reader.
type proxyErrorReader struct{}

// Read returns the transport error, never a clean EOF.
func (proxyErrorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// TestProxyForwardingRetainsUsageOnClientFailure proves observed work survives
// both explicit write failures and short writes for reservation-aware settlement.
func TestProxyForwardingRetainsUsageOnClientFailure(t *testing.T) {
	for _, short := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		c.Writer = proxyBrokenWriter{ResponseWriter: c.Writer, short: short}
		body := &proxyObservedBody{reader: strings.NewReader(`{"usage":{"prompt_tokens":73,"completion_tokens":19,"total_tokens":92}}`)}
		response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: body}
		usage, err := (&Adaptor{}).DoResponse(c, response, &meta.Meta{PromptTokens: 2})
		require.NotNil(t, err)
		require.NotNil(t, usage)
		require.Equal(t, 73, usage.PromptTokens)
		require.Equal(t, 19, usage.CompletionTokens)
		require.NotEmpty(t, usage.BillingEstimateReason)
		require.True(t, body.closed)
	}
}

// TestProxyForwardingFlushesBeforeEOF verifies interactive delivery, byte/status
// preservation, repeated headers, and closure without a timing-sensitive sleep.
func TestProxyForwardingFlushesBeforeEOF(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	payload := "data: {\"usage\":{\"prompt_tokens\":73,\"completion_tokens\":19}}\n\n"
	body := &proxyObservedBody{reader: strings.NewReader(payload)}
	body.beforeRead = func(read int) {
		if read > 1 {
			require.True(t, recorder.Flushed, "SSE must be flushed before the next upstream read")
		}
	}
	response := &http.Response{StatusCode: http.StatusAccepted, Header: http.Header{"Content-Type": {"text/event-stream"}, "Set-Cookie": {"first=1", "second=2"}}, Body: body}
	usage, err := (&Adaptor{}).DoResponse(c, response, nil)
	require.Nil(t, err)
	require.Equal(t, 73, usage.PromptTokens)
	require.Equal(t, 19, usage.CompletionTokens)
	require.Equal(t, payload, recorder.Body.String())
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.Equal(t, []string{"first=1", "second=2"}, recorder.Header().Values("Set-Cookie"))
	require.True(t, body.closed)
}

// TestProxyForwardingPreservesPartialUpstreamUsage verifies read errors do not
// discard input receipts or observed output and cannot trigger a full refund.
func TestProxyForwardingPreservesPartialUpstreamUsage(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	payload := "data: {\"usage\":{\"prompt_tokens\":73},\"choices\":[{\"delta\":{\"content\":\"generated output before interruption\"}}]}\n\n"
	body := &proxyObservedBody{reader: io.MultiReader(strings.NewReader(payload), proxyErrorReader{})}
	response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}
	usage, err := (&Adaptor{}).DoResponse(c, response, &meta.Meta{PromptTokens: 2})
	require.NotNil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 73, usage.PromptTokens)
	require.Positive(t, usage.CompletionTokens)
	require.NotEmpty(t, usage.BillingEstimateReason)
	require.True(t, body.closed)
}
