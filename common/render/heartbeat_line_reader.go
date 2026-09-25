package render

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/errors/v2"

	commonsse "github.com/Laisky/one-api/common/sse"
)

const (
	// DefaultHeartbeatInterval is the default interval between SSE heartbeat comments.
	// Cloudflare's 524 timeout is 100 seconds; 5s gives ample margin.
	DefaultHeartbeatInterval = 5 * time.Second

	// heartbeatPayload is a minimal SSE comment line used as a keep-alive.
	// It starts with ':' so compliant SSE parsers ignore it.
	heartbeatPayload = ":\n"
)

// heartbeatLineResult captures a single asynchronous line-reader result.
type heartbeatLineResult struct {
	line commonsse.Line
	err  error
}

// HeartbeatLineReader wraps an SSE line reader and emits heartbeat comments while waiting.
type HeartbeatLineReader struct {
	c                 *gin.Context
	reader            *commonsse.LineReader
	interval          time.Duration
	done              chan struct{}
	closeOnce         sync.Once
	heartbeatsSent    int
	heartbeatWriteErr error
}

// NewHeartbeatLineReader creates a heartbeat wrapper around the provided line reader.
// It flushes response headers immediately so downstream proxies observe the SSE response.
func NewHeartbeatLineReader(c *gin.Context, reader *commonsse.LineReader, interval time.Duration) *HeartbeatLineReader {
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}

	h := &HeartbeatLineReader{
		c:        c,
		reader:   reader,
		interval: interval,
		done:     make(chan struct{}),
	}

	if c != nil && c.Writer != nil {
		c.Writer.Flush()
	}

	return h
}

// Next returns the next SSE line while sending heartbeats during idle periods.
func (h *HeartbeatLineReader) Next() (commonsse.Line, error) {
	clientCtx := context.Background()
	if h.c != nil && h.c.Request != nil {
		clientCtx = h.c.Request.Context()
	}
	select {
	case <-h.done:
		return commonsse.Line{}, io.EOF
	case <-clientCtx.Done():
		return commonsse.Line{}, errors.WithStack(clientCtx.Err())
	default:
	}
	// Buffered complete lines need no asynchronous I/O or heartbeat timer.
	// A partial or oversized line keeps the original blocking/heartbeat path.
	if line, ready, err := h.reader.NextBuffered(); ready {
		return line, err
	}

	// A skipped blank/comment line can leave a previous frame pending. Publish it
	// before waiting on I/O, rather than making delivery depend on the next token.
	if h.c != nil {
		if flusher, ok := h.c.Writer.(interface{ FlushPending() }); ok {
			flusher.FlushPending()
		}
	}

	resultCh := make(chan heartbeatLineResult, 1)
	go func() {
		line, err := h.reader.Next()
		select {
		case resultCh <- heartbeatLineResult{line: line, err: err}:
		case <-h.done:
		}
	}()

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case result := <-resultCh:
			return result.line, result.err
		case <-ticker.C:
			h.sendHeartbeat()
		case <-clientCtx.Done():
			return commonsse.Line{}, errors.WithStack(clientCtx.Err())
		case <-h.done:
			return commonsse.Line{}, io.EOF
		}
	}
}

// Close stops future Next calls and is safe to call multiple times.
func (h *HeartbeatLineReader) Close() {
	h.closeOnce.Do(func() {
		close(h.done)
	})
}

// HeartbeatsSent returns the number of heartbeat comments written so far.
func (h *HeartbeatLineReader) HeartbeatsSent() int {
	return h.heartbeatsSent
}

// HeartbeatWriteErr returns the first heartbeat write error, if any.
func (h *HeartbeatLineReader) HeartbeatWriteErr() error {
	return h.heartbeatWriteErr
}

// sendHeartbeat writes a minimal SSE comment to keep idle connections alive.
func (h *HeartbeatLineReader) sendHeartbeat() {
	if h.c == nil || h.c.Writer == nil {
		return
	}

	_, err := h.c.Writer.Write([]byte(heartbeatPayload))
	if err != nil {
		if h.heartbeatWriteErr == nil {
			h.heartbeatWriteErr = errors.WithStack(err)
		}
		return
	}

	h.c.Writer.Flush()
	h.heartbeatsSent++
}
