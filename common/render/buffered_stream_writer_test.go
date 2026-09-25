package render

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// flushProbe records each explicit transport flush while retaining Gin's writer behavior.
type flushProbe struct {
	gin.ResponseWriter
	flushes int
}

// Flush records a flush and forwards it to the original writer.
func (p *flushProbe) Flush() { p.flushes++; p.ResponseWriter.Flush() }

// bufferedWriterFixture primes a complete buffered line and returns an armed writer and its transport probe.
func bufferedWriterFixture(t *testing.T) (*BufferedStreamWriter, *flushProbe) {
	t.Helper()
	c, _ := newTestContext()
	p := &flushProbe{ResponseWriter: c.Writer}
	c.Writer = p
	reader := commonsse.NewLineReader(strings.NewReader("data: seed\n\ndata: next\n\n"), 4096)
	_, err := reader.Next()
	require.NoError(t, err)
	w := NewBufferedStreamWriter(c, reader)
	w.AllowBuffering()
	// Freeze the elapsed-time condition; budget-specific tests exercise it independently.
	w.lastFlush = time.Now().Add(time.Hour)
	t.Cleanup(w.Close)
	return w, p
}

// TestBufferedStreamWriterBounds checks first-content delivery, bounded flushes, finalization and writer restoration.
func TestBufferedStreamWriterBounds(t *testing.T) {
	t.Run("first content is immediate", func(t *testing.T) {
		w, p := bufferedWriterFixture(t)
		w.enabled = false
		_, err := w.WriteString("data: first\n\n")
		require.NoError(t, err)
		w.Flush()
		require.Equal(t, 1, p.flushes)
	})
	t.Run("buffered frames coalesce without changing bytes", func(t *testing.T) {
		w, p := bufferedWriterFixture(t)
		original := w.context.Writer
		for range streamFlushFrames - 1 {
			_, err := w.Write([]byte("data: value\n\n"))
			require.NoError(t, err)
			w.Flush()
		}
		require.Zero(t, p.flushes)
		require.Equal(t, (streamFlushFrames-1)*len("data: value\n\n"), w.Size())
		w.Close()
		require.Equal(t, 1, p.flushes)
		require.Same(t, p, w.context.Writer)
		require.NotSame(t, original, w.context.Writer)
		w.Close()
		require.Equal(t, 1, p.flushes)
	})
	t.Run("frame budget", func(t *testing.T) {
		w, p := bufferedWriterFixture(t)
		for range streamFlushFrames {
			_, err := w.WriteString("x")
			require.NoError(t, err)
			w.Flush()
		}
		require.Equal(t, 1, p.flushes)
	})
	t.Run("byte budget", func(t *testing.T) {
		w, p := bufferedWriterFixture(t)
		_, err := w.WriteString(strings.Repeat("x", streamFlushBytes))
		require.NoError(t, err)
		w.Flush()
		require.Equal(t, 1, p.flushes)
	})
	t.Run("elapsed processing budget", func(t *testing.T) {
		w, p := bufferedWriterFixture(t)
		w.lastFlush = time.Now().Add(-streamFlushInterval)
		_, err := w.WriteString("x")
		require.NoError(t, err)
		w.Flush()
		require.Equal(t, 1, p.flushes)
	})
	t.Run("no complete buffered line", func(t *testing.T) {
		w, p := bufferedWriterFixture(t)
		w.reader = commonsse.NewLineReader(strings.NewReader("partial"), 4096)
		_, err := w.WriteString("x")
		require.NoError(t, err)
		w.Flush()
		require.Equal(t, 1, p.flushes)
	})
	t.Run("do not overwrite a newer wrapper", func(t *testing.T) {
		w, _ := bufferedWriterFixture(t)
		newer := &flushProbe{ResponseWriter: w}
		w.context.Writer = newer
		w.Close()
		require.Same(t, newer, w.context.Writer)
	})
}

// notifiedRead signals the first potentially blocking upstream read.
type notifiedRead struct {
	io.Reader
	once    sync.Once
	started chan struct{}
}

// Read signals readiness before delegating to the underlying reader.
func (r *notifiedRead) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.Reader.Read(p)
}

// TestBufferedStreamFlushesBeforeBlockingRead proves a trailing blank line cannot strand a pending content frame.
func TestBufferedStreamFlushesBeforeBlockingRead(t *testing.T) {
	c, _ := newTestContext()
	probe := &flushProbe{ResponseWriter: c.Writer}
	c.Writer = probe
	input, output := io.Pipe()
	defer input.Close()
	defer output.Close()
	blocked := &notifiedRead{Reader: input, started: make(chan struct{})}
	reader := commonsse.NewLineReader(io.MultiReader(strings.NewReader("data: seed\n\n"), blocked), 4096)
	_, err := reader.Next()
	require.NoError(t, err)
	w := NewBufferedStreamWriter(c, reader)
	defer w.Close()
	h := NewHeartbeatLineReader(c, reader, time.Hour)
	defer h.Close()
	probe.flushes = 0
	w.AllowBuffering()
	w.lastFlush = time.Now().Add(time.Hour)
	_, err = w.WriteString("data: pending\n\n")
	require.NoError(t, err)
	w.Flush()
	require.Zero(t, probe.flushes)
	line, err := h.Next()
	require.NoError(t, err)
	require.Equal(t, commonsse.LineKindBlank, line.Kind)
	done := make(chan error, 1)
	go func() { _, err := h.Next(); done <- err }()
	select {
	case <-blocked.started:
	case <-time.After(time.Second):
		t.Fatal("upstream read did not start")
	}
	// The signal establishes a happens-before edge after the forced flush.
	require.Equal(t, 1, probe.flushes)
	require.NoError(t, output.Close())
	select {
	case err := <-done:
		require.ErrorIs(t, err, io.EOF)
	case <-time.After(time.Second):
		t.Fatal("read did not end")
	}
}

// TestBufferedStreamWriterPreservesWriteErrors checks that coalescing does not mask a disconnected downstream.
func TestBufferedStreamWriterPreservesWriteErrors(t *testing.T) {
	w, _ := bufferedWriterFixture(t)
	w.ResponseWriter = &errorWriter{ResponseWriter: w.ResponseWriter, writeErr: io.ErrClosedPipe}
	n, err := w.Write([]byte("data: content\n\n"))
	require.Zero(t, n)
	require.ErrorIs(t, err, io.ErrClosedPipe)
	require.False(t, w.pending)
}
