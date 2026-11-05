package openai

import (
	stderrors "errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// failingDeliveryWriter commits headers and then fails every body write, like a
// downstream connection that broke after the stream started.
type failingDeliveryWriter struct {
	gin.ResponseWriter
	cause error
}

// Write commits the status line and reports the configured delivery failure.
func (w failingDeliveryWriter) Write([]byte) (int, error) {
	w.WriteHeaderNow()
	return 0, w.cause
}

// WriteString follows the same failed delivery boundary as Write.
func (w failingDeliveryWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// TestResponseStreamDeliveryFailureCode verifies that both Responses stream
// handlers name a failed downstream write as a delivery failure, keep the cause
// and the provider receipt, and still name a provider read failure as such.
func TestResponseStreamDeliveryFailureCode(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode int
		call func(*gin.Context, *http.Response, int) (*model.ErrorWithStatusCode, string, *model.Usage)
	}{
		{"direct", relaymode.ResponseAPI, ResponseAPIDirectStreamHandler},
		{"converted", relaymode.ChatCompletions, ResponseAPIStreamHandler},
	} {
		t.Run(tc.name+"/delivery", func(t *testing.T) {
			c, _ := newDirectStreamContext()
			cause := stderrors.New("synthetic client connection reset")
			c.Writer = failingDeliveryWriter{ResponseWriter: c.Writer, cause: cause}
			event := securityReceiptEvent(t, 4)
			failure, _, usage := tc.call(c, fakeUpstream(event+event), tc.mode)
			require.NotNil(t, failure)
			require.Equal(t, "write_stream_failed", failure.Code)
			require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
			require.Equal(t, model.ErrorTypeOneAPI, failure.Type)
			require.ErrorIs(t, failure.RawError, cause)
			require.NotNil(t, usage, "the provider receipt survives a delivery failure")
			require.Equal(t, 18, usage.TotalTokens)
			require.True(t, c.Writer.Written())
		})
		t.Run(tc.name+"/read", func(t *testing.T) {
			c, rec := newDirectStreamContext()
			body := &securityStreamBody{Reader: strings.NewReader(securityReceiptEvent(t, 4)), readErr: stderrors.New("synthetic provider reset")}
			failure, _, usage := tc.call(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}, tc.mode)
			require.NotNil(t, failure)
			require.Equal(t, "read_stream_failed", failure.Code, "a provider read failure is not a delivery failure")
			require.NotNil(t, usage)
			require.Contains(t, rec.Body.String(), "data:")
		})
	}
}
