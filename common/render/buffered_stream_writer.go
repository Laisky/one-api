package render

import (
	"time"

	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/gin-gonic/gin"
)

const (
	streamFlushBytes    = 8 << 10
	streamFlushFrames   = 8
	streamFlushInterval = time.Millisecond
)

// BufferedStreamWriter coalesces flushes only while complete upstream lines are already in memory.
// It is request-local, never starts a timer or goroutine, and never waits for more upstream bytes.
// Call AllowBuffering only after the first content frame has been emitted, and always defer Close.
type BufferedStreamWriter struct {
	gin.ResponseWriter
	context   *gin.Context
	reader    *commonsse.LineReader
	enabled   bool
	pending   bool
	closed    bool
	bytes     int
	frames    int
	lastFlush time.Time
}

// NewBufferedStreamWriter wraps c.Writer without changing its headers, status, or write-error behavior.
// Buffering starts disabled so response headers and the first content frame flush immediately.
func NewBufferedStreamWriter(c *gin.Context, reader *commonsse.LineReader) *BufferedStreamWriter {
	writer := &BufferedStreamWriter{ResponseWriter: c.Writer, context: c, reader: reader, lastFlush: time.Now()}
	c.Writer = writer
	return writer
}

// AllowBuffering enables opportunistic coalescing after the caller has emitted the first content frame.
func (w *BufferedStreamWriter) AllowBuffering() { w.enabled = true }

// Write forwards data and accounts only for bytes actually accepted by the underlying writer.
func (w *BufferedStreamWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	w.pending = w.pending || n > 0
	return n, err
}

// WriteString forwards text while preserving the underlying writer's byte count and error.
func (w *BufferedStreamWriter) WriteString(data string) (int, error) {
	n, err := w.ResponseWriter.WriteString(data)
	w.bytes += n
	w.pending = w.pending || n > 0
	return n, err
}

// Flush sends headers immediately and bounds coalescing by bytes, frames, and elapsed processing time.
// The caller must FlushPending before blocking on upstream I/O, even after a blank or comment line.
func (w *BufferedStreamWriter) Flush() {
	w.frames++
	if !w.enabled || !w.pending || w.bytes >= streamFlushBytes || w.frames >= streamFlushFrames || time.Since(w.lastFlush) >= streamFlushInterval || !w.reader.BufferedLineReady() {
		w.ResponseWriter.Flush()
		w.pending = false
		w.bytes, w.frames = 0, 0
		w.lastFlush = time.Now()
	}
}

// FlushPending publishes pending bytes without consulting upstream readiness or the coalescing budget.
func (w *BufferedStreamWriter) FlushPending() {
	if w.pending {
		w.ResponseWriter.Flush()
		w.pending = false
		w.bytes, w.frames = 0, 0
		w.lastFlush = time.Now()
	}
}

// Close flushes the last frame and restores the original writer without overwriting a newer wrapper.
func (w *BufferedStreamWriter) Close() {
	if w.closed {
		return
	}
	w.closed = true
	w.FlushPending()
	if w.context.Writer == w {
		w.context.Writer = w.ResponseWriter
	}
}
