package openai_compatible

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/streaming"
)

// doneCancellationWriter cancels at the actual rendered protocol boundary.
type doneCancellationWriter struct {
	gin.ResponseWriter
	cancel context.CancelFunc
}

// Write keeps the delivery result and cancels only after the upstream marker is rendered.
func (w *doneCancellationWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if err == nil && strings.Contains(string(p), "[DONE]") {
		w.cancel()
	}
	return n, err
}

// WriteString follows the same byte-oriented renderer contract.
func (w *doneCancellationWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// streamTerminalReadFailure retains a genuine non-EOF transport error for the control.
type streamTerminalReadFailure struct{}

// Read always reports a truncated response, not a clean EOF.
func (streamTerminalReadFailure) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// TestSharedStreamDoneCancellation distinguishes a completed protocol followed by
// clean EOF from pre-terminal cancellation and genuine trailing transport failure.
func TestSharedStreamDoneCancellation(t *testing.T) {
	for _, trailingError := range []bool{false, true} {
		name := "completed_eof"
		if trailingError {
			name = "transport_error_control"
		}
		t.Run(name, func(t *testing.T) {
			c, rec := newBridgeTestCtx(t)
			gmw.SetLogger(c, createTestLogger())
			ctx, cancel := context.WithCancel(c.Request.Context())
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			c.Writer = &doneCancellationWriter{ResponseWriter: c.Writer, cancel: cancel}
			c.Set(ctxkey.RequestModel, "gpt-4")
			tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{Ctx: ctx, ModelName: "gpt-4", PromptTokens: 11, PreConsumedQuota: 10000, TokenCounter: func(s, _ string) int { return len(s) }})
			streaming.StoreTracker(c, tracker)
			wire := "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18}}\n\ndata: [DONE]\n\n"
			var source io.Reader = strings.NewReader(wire)
			if trailingError {
				source = io.MultiReader(source, streamTerminalReadFailure{})
			}
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(source)}
			apiErr, usage := UnifiedStreamProcessing(c, resp, 11, "gpt-4", false)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			require.Equal(t, 1, strings.Count(rec.Body.String(), "[DONE]"))
			require.NotNil(t, usage)
			require.Equal(t, 18, usage.TotalTokens)
			require.Empty(t, usage.BillingEstimateReason)
			if trailingError {
				require.NotNil(t, apiErr)
				require.True(t, errors.Is(apiErr.RawError, io.ErrUnexpectedEOF), "retain the real read failure rather than overwriting it with cancellation")
			} else {
				require.Nil(t, apiErr, "cancellation after the processed upstream terminal marker cannot replace clean EOF")
			}
		})
	}
}
