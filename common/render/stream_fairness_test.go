package render

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	commonsse "github.com/Laisky/one-api/common/sse"
)

// fairnessFlushWitness records each actual caller flush without changing response writes.
type fairnessFlushWitness struct {
	gin.ResponseWriter
	flushes int
}

// Flush records and forwards a flush to the underlying response writer.
func (w *fairnessFlushWitness) Flush() {
	w.flushes++
	w.ResponseWriter.Flush()
}

// fairnessLines returns distinct 128-byte lines, deliberately smaller than either scheduling budget.
func fairnessLines(count int) ([]string, string) {
	lines := make([]string, count)
	for i := range lines {
		prefix := fmt.Sprintf("data: %04d ", i)
		lines[i] = prefix + strings.Repeat("x", 128-len(prefix))
	}
	return lines, strings.Join(lines, "\n") + "\n"
}

// TestBufferedStreamFairnessAcrossSmallLines requires scheduling across calls, after prior caller flushes.
func TestBufferedStreamFairnessAcrossSmallLines(t *testing.T) {
	lines, wire := fairnessLines(100)
	c, recorder := newTestContext()
	witness := &fairnessFlushWitness{ResponseWriter: c.Writer}
	c.Writer = witness
	h := NewHeartbeatLineReader(c, commonsse.NewLineReader(strings.NewReader(wire), commonsse.DefaultLineBufferSize), DefaultHeartbeatInterval)
	defer h.Close()
	seen, yields := 0, 0
	var expected strings.Builder
	h.yield = func() {
		yields++
		require.GreaterOrEqual(t, seen, 33, "the first content must not wait for this scheduling point")
		require.Equal(t, seen+1, witness.flushes, "each prior line must already have been flushed")
		require.Equal(t, expected.String(), recorder.Body.String(), "scheduling must not retain pending caller output")
	}
	for i, want := range lines {
		line, err := h.Next()
		require.NoError(t, err)
		require.Equal(t, want, line.Text())
		if i == 0 {
			require.Zero(t, yields, "first line cannot be delayed by the accumulated budget")
		}
		StringData(c, line.Text())
		expected.WriteString(want + "\n\n")
		seen++
	}
	_, err := h.Next()
	require.Same(t, io.EOF, err)
	require.Equal(t, 3, yields, "99 buffered 128-byte lines must cross three budgets")
	require.Zero(t, h.HeartbeatsSent())
	require.Nil(t, h.HeartbeatWriteErr())
}

// TestBufferedStreamFairnessHonorsCancellation requires cancellation or Close to win before consuming another line.
func TestBufferedStreamFairnessHonorsCancellation(t *testing.T) {
	for _, mode := range []string{"cancel", "close"} {
		t.Run(mode, func(t *testing.T) {
			lines, wire := fairnessLines(100)
			c, _, cancel := newTestContextWithCancel()
			defer cancel()
			reader := commonsse.NewLineReader(strings.NewReader(wire), commonsse.DefaultLineBufferSize)
			h := NewHeartbeatLineReader(c, reader, DefaultHeartbeatInterval)
			defer h.Close()
			calls := 0
			h.yield = func() {
				calls++
				if mode == "cancel" {
					cancel()
				} else {
					h.Close()
				}
			}
			for i := range 33 {
				line, err := h.Next()
				require.NoError(t, err)
				require.Equal(t, lines[i], line.Text())
			}
			_, err := h.Next()
			if mode == "cancel" {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.Same(t, io.EOF, err)
			}
			require.Equal(t, 1, calls)
			line, ready, err := reader.NextBuffered()
			require.NoError(t, err)
			require.True(t, ready)
			require.Equal(t, lines[33], line.Text(), "cancellation must not consume a buffered line")
		})
	}
}

// TestBufferedStreamFairnessResetsOnIO preserves the original asynchronous path without needless extra yields.
func TestBufferedStreamFairnessResetsOnIO(t *testing.T) {
	lines, wire := fairnessLines(100)
	c, _ := newTestContext()
	h := NewHeartbeatLineReader(c, commonsse.NewLineReader(strings.NewReader(wire), 256), DefaultHeartbeatInterval)
	defer h.Close()
	yields := 0
	h.yield = func() { yields++ }
	for _, want := range lines {
		line, err := h.Next()
		require.NoError(t, err)
		require.Equal(t, want, line.Text())
	}
	_, err := h.Next()
	require.Same(t, io.EOF, err)
	require.Zero(t, yields, "the blocking reader path already gives the scheduler control")
}
