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
	requests          chan struct{}
	results           chan heartbeatLineResult
	workerStopped     chan struct{}
	timer             *time.Timer
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
		c:             c,
		reader:        reader,
		interval:      interval,
		done:          make(chan struct{}),
		requests:      make(chan struct{}),
		results:       make(chan heartbeatLineResult, 1),
		workerStopped: make(chan struct{}),
		timer:         time.NewTimer(interval),
	}

	h.timer.Stop()
	go readHeartbeatLines(reader, h.requests, h.results, h.done, h.workerStopped)

	if c != nil && c.Writer != nil {
		c.Writer.Flush()
	}

	return h
}

// readHeartbeatLines reads only when requested and never accesses a Gin context.
// A demand handshake is essential: an oversized Line.Large reader shares its
// underlying buffer with LineReader.Next and must be consumed before another read.
func readHeartbeatLines(reader *commonsse.LineReader, requests <-chan struct{}, results chan<- heartbeatLineResult, done <-chan struct{}, stopped chan<- struct{}) {
	defer close(stopped)
	for {
		select {
		case <-done:
			return
		case <-requests:
		}
		line, err := reader.Next()
		select {
		case results <- heartbeatLineResult{line: line, err: err}:
		case <-done:
			return
		}
	}
}

// Next returns the next SSE line and sends heartbeats only while waiting for it.
// Calls must be sequential, as with LineReader.Next. Close may run concurrently;
// the caller still owns closing the upstream body to unblock a pending read.
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
	select {
	case h.requests <- struct{}{}:
	case <-h.done:
		return commonsse.Line{}, io.EOF
	case <-clientCtx.Done():
		return commonsse.Line{}, errors.WithStack(clientCtx.Err())
	}

	// Reuse one timer per stream rather than allocating a timer, goroutine and
	// result channel for every SSE line. Reset keeps the original idle semantics.
	h.timer.Reset(h.interval)
	defer h.timer.Stop()
	for {
		select {
		case result := <-h.results:
			return result.line, result.err
		case <-h.timer.C:
			h.sendHeartbeat()
			h.timer.Reset(h.interval)
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
