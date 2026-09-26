package observation_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime/trace"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/render"
	"github.com/Laisky/one-api/tests/stream-perf/observation"
)

// wireProbe captures actual write and flush boundaries, including short-write/error propagation.
type wireProbe struct {
	gin.ResponseWriter
	segments []string
	flushes  int
	failAt   int
	short    bool
}

var errProbeWrite = errors.New("diagnostic wire write failure")

// Write captures one transport write and applies the requested downstream failure.
func (w *wireProbe) Write(p []byte) (int, error) {
	w.segments = append(w.segments, string(p))
	if len(w.segments) == w.failAt {
		if w.short {
			return max(0, len(p)-1), nil
		}
		return 0, errProbeWrite
	}
	return w.ResponseWriter.Write(p)
}

// WriteString retains the same error observation for StringWriter and byte writes.
func (w *wireProbe) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// Flush counts each explicit flush before forwarding it.
func (w *wireProbe) Flush() { w.flushes++; w.ResponseWriter.Flush() }

// oldStringData independently preserves the pinned production framing used before adding the overlay.
func oldStringData(c *gin.Context, s string) {
	s = strings.TrimPrefix(s, "data: ")
	s = strings.TrimSuffix(s, "\r")
	c.Render(-1, common.CustomEvent{Data: "data: " + s})
	c.Writer.Flush()
}

type outcome struct {
	body          string
	headers       http.Header
	segments      []string
	flushes       int
	aborted       bool
	errors        []string
	short, failed bool
}

// captureWire records transport-visible and error behavior for the original and overlaid renderer.
func captureWire(fn func(*gin.Context, string), inputs []string, failAt int, short bool) (outcome, int) {
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest("GET", "/", nil).WithContext(context.Background())
	c.Request.Header.Set(observation.Header, "0")
	c.Header("Cache-Control", "private, no-store")
	w := &wireProbe{ResponseWriter: c.Writer, failAt: failAt, short: short}
	c.Writer = w
	for _, s := range inputs {
		fn(c, s)
	}
	o := outcome{body: response.Body.String(), headers: response.Header().Clone(), segments: w.segments, flushes: w.flushes, aborted: c.IsAborted()}
	for _, err := range c.Errors {
		o.errors = append(o.errors, err.Error())
		o.short = o.short || errors.Is(err.Err, io.ErrShortWrite)
		o.failed = o.failed || errors.Is(err.Err, errProbeWrite)
	}
	return o, c.GetInt(observation.StateKey)
}

// TestDiagnosticOverlayWire verifies real writes under a live trace; normal test builds explicitly skip this opt-in check.
func TestDiagnosticOverlayWire(t *testing.T) {
	if os.Getenv("ONEAPI_TEST_CORRELATION_OVERLAY") != "1" {
		t.Skip("requires explicit diagnostic build overlay")
	}
	var log bytes.Buffer
	require.NoError(t, trace.Start(&log))
	t.Cleanup(trace.Stop)
	inputs := []string{"", "data: ", "data:x", "data:  x", "hello\nworld\r", "data: \xff\xfe🙂", "[DONE]"}
	old, _ := captureWire(oldStringData, inputs, 0, false)
	actual, frames := captureWire(render.StringData, inputs, 0, false)
	require.Equal(t, old, actual)
	require.Equal(t, len(inputs), frames)
	require.Equal(t, len(inputs), actual.flushes)
	for _, short := range []bool{false, true} {
		for _, failAt := range []int{1, 2} {
			old, _ := captureWire(oldStringData, []string{"hello\nworld"}, failAt, short)
			actual, _ := captureWire(render.StringData, []string{"hello\nworld"}, failAt, short)
			require.Equal(t, old, actual)
			require.True(t, actual.aborted)
		}
	}
	trace.Stop()
	require.NotZero(t, log.Len())
}
