package render

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var errSSEEventTypeWrite = errors.New("SSE event type write failed")

// sseEventErrorWriter fails writes containing the SSE event-type line and delegates all others.
type sseEventErrorWriter struct {
	gin.ResponseWriter
}

// Write rejects the event-type line so the test can observe whether SSEEvent stops the frame.
func (w *sseEventErrorWriter) Write(payload []byte) (int, error) {
	if bytes.Contains(payload, []byte("event:")) {
		return 0, errSSEEventTypeWrite
	}
	return w.ResponseWriter.Write(payload)
}

// TestSSEEventStopsAfterEventTypeWriteFailure verifies that a broken frame aborts before data is written.
func TestSSEEventStopsAfterEventTypeWriteFailure(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Writer = &sseEventErrorWriter{ResponseWriter: ctx.Writer}

	SSEEvent(ctx, "response.output_text.delta", `{"delta":"hello"}`)

	require.True(t, ctx.IsAborted())
	require.Len(t, ctx.Errors, 1)
	require.ErrorIs(t, ctx.Errors[0].Err, errSSEEventTypeWrite)
	require.Empty(t, recorder.Body.String(), "a failed event-type line must not be followed by a data line")
}
