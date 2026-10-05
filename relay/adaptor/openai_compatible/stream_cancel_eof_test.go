package openai_compatible

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/streaming"
)

// canceledEOFWriter makes cancellation occur after actual delta delivery. Its
// upstream uses ordinary EOF, a legitimate result when a transport is closed.
type canceledEOFWriter struct {
	gin.ResponseWriter
	cancel context.CancelFunc
}

// Write cancels only after the selected later output reaches the recorder.
func (w *canceledEOFWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if err == nil && strings.Contains(string(p), "later output") {
		w.cancel()
	}
	return n, err
}

// WriteString routes the actual renderer through the same cancellation boundary.
func (w *canceledEOFWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// TestSharedStreamCanceledEOF preserves observed receipt chronology when a
// canceled upstream ends with EOF rather than a transport-specific error.
func TestSharedStreamCanceledEOF(t *testing.T) {
	for _, cancelAfterDelta := range []bool{false, true} {
		name := "measured_control"
		if cancelAfterDelta {
			name = "cancelled_eof"
		}
		t.Run(name, func(t *testing.T) {
			c, rec := newBridgeTestCtx(t)
			gmw.SetLogger(c, createTestLogger())
			ctx, cancel := context.WithCancel(c.Request.Context())
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			c.Set(ctxkey.RequestModel, "gpt-4")
			tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{Ctx: ctx, ModelName: "gpt-4", PromptTokens: 11, PreConsumedQuota: 10000, TokenCounter: func(s, _ string) int { return len(s) }})
			streaming.StoreTracker(c, tracker)
			receipt := "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18}}\n\n"
			wire := receipt + "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"later output\"}}]}\n\n"
			if cancelAfterDelta {
				c.Writer = &canceledEOFWriter{ResponseWriter: c.Writer, cancel: cancel}
			} else {
				wire += receipt + "data: [DONE]\n\n"
			}
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}
			apiErr, usage := UnifiedStreamProcessing(c, resp, 11, "gpt-4", false)
			require.Contains(t, rec.Body.String(), "later output")
			require.NotNil(t, usage)
			if !cancelAfterDelta {
				require.Nil(t, apiErr)
				require.Equal(t, 18, usage.TotalTokens)
				require.Empty(t, usage.BillingEstimateReason)
				return
			}
			require.Error(t, ctx.Err())
			require.NotNil(t, apiErr, "EOF caused by cancellation must not synthesize successful completion")
			require.Equal(t, 11, usage.PromptTokens)
			require.Equal(t, 7+len("later output"), usage.CompletionTokens)
			require.Equal(t, "stream_output_after_last_receipt", usage.BillingEstimateReason)
			require.NotContains(t, rec.Body.String(), "data: [DONE]")
		})
	}
}
