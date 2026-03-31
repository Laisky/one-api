package render

import (
	"bufio"
	"context"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// DefaultHeartbeatInterval is the default interval between SSE heartbeat comments.
	// Cloudflare's 524 timeout is 100 seconds; 5s gives ample margin.
	DefaultHeartbeatInterval = 5 * time.Second

	// heartbeatPayload is a standard SSE comment line used as a keep-alive.
	//
	// Per the WHATWG HTML Living Standard (Server-sent events, §9.2):
	//   "If the line starts with a U+003A COLON character (:) Ignore the line."
	//
	// The spec explicitly recommends this mechanism:
	//   "Legacy proxy servers are known to, in certain cases, drop HTTP
	//    connections after a short timeout. To protect against such proxy
	//    servers, authors can include a comment line (one starting with a
	//    ':' character) every 15 seconds or so."
	//
	// All major AI SDK SSE parsers (OpenAI Python/Node, Anthropic Python/Node)
	// correctly ignore comment lines. We use the minimal form ":\n" (a bare
	// colon followed by a newline) to avoid dispatching empty events — a known
	// edge case in some SSE decoders (cf. openai/openai-go#556).
	heartbeatPayload = ":\n"
)

// HeartbeatScanner wraps a bufio.Scanner and sends SSE heartbeat comments
// to the client when the upstream is idle. This prevents reverse proxies
// (like Cloudflare) from issuing 524 timeout errors.
//
// All writes to c.Writer happen in the caller's goroutine (via Scan()),
// so there are no concurrent write concerns.
//
// Usage mirrors bufio.Scanner:
//
//	hbs := render.NewHeartbeatScanner(c, scanner, render.DefaultHeartbeatInterval)
//	defer hbs.Close()
//	for hbs.Scan() {
//	    line := hbs.Text()
//	    // process line...
//	}
//	if err := hbs.Err(); err != nil { ... }
type HeartbeatScanner struct {
	c        *gin.Context
	lines    chan string
	done     chan struct{}
	interval time.Duration
	text     string
	mu       sync.Mutex
	err      error
	closed   bool

	// heartbeatsSent tracks the number of heartbeat comments sent to the client.
	// Accessed only from the caller's goroutine (via Scan()), so no mutex needed.
	heartbeatsSent int

	// heartbeatWriteErr records the first write error encountered when sending
	// a heartbeat. A non-nil value indicates the client connection is likely dead.
	heartbeatWriteErr error
}

// NewHeartbeatScanner creates a HeartbeatScanner that reads lines from the
// given scanner in a background goroutine. During idle periods (no data from
// upstream), it sends SSE comment heartbeats to the client at the specified
// interval.
//
// The caller must call Close() when done (typically via defer) to stop the
// background reader goroutine.
func NewHeartbeatScanner(c *gin.Context, scanner *bufio.Scanner, interval time.Duration) *HeartbeatScanner {
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}
	h := &HeartbeatScanner{
		c: c,
		// Buffer of 1 so the reader goroutine doesn't block while the
		// caller is processing the previous line.
		lines:    make(chan string, 1),
		done:     make(chan struct{}),
		interval: interval,
	}
	// Flush headers immediately so the reverse proxy sees the 200 + SSE
	// content-type right away, before any upstream data arrives.
	if c != nil && c.Writer != nil {
		c.Writer.Flush()
	}

	go h.readLoop(scanner)
	return h
}

// readLoop reads lines from the scanner and sends them on the lines channel.
// It runs in a separate goroutine and closes the lines channel when done.
func (h *HeartbeatScanner) readLoop(scanner *bufio.Scanner) {
	defer close(h.lines)
	for scanner.Scan() {
		select {
		case h.lines <- scanner.Text():
		case <-h.done:
			return
		}
	}
	// Set err before closing the channel so the caller sees it after
	// the channel-close receive.
	h.mu.Lock()
	h.err = scanner.Err()
	h.mu.Unlock()
}

// Scan advances to the next line from the upstream scanner. While waiting
// for data, it sends SSE heartbeat comments to the client at the configured
// interval. Returns true if a line is available via Text(), false when the
// stream has ended or the client has disconnected.
func (h *HeartbeatScanner) Scan() bool {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	clientCtx := context.Background()
	if h.c != nil && h.c.Request != nil {
		clientCtx = h.c.Request.Context()
	}

	for {
		select {
		case line, ok := <-h.lines:
			if !ok {
				return false
			}
			h.text = line
			return true
		case <-ticker.C:
			h.sendHeartbeat()
		case <-clientCtx.Done():
			// Client disconnected (e.g. Cloudflare 524, browser closed).
			// Stop processing to avoid wasting resources on a dead connection.
			h.mu.Lock()
			if h.err == nil {
				h.err = clientCtx.Err()
			}
			h.mu.Unlock()
			return false
		}
	}
}

// Text returns the text of the current line (set by the most recent Scan call).
func (h *HeartbeatScanner) Text() string {
	return h.text
}

// Err returns the first non-EOF error encountered by the underlying scanner.
func (h *HeartbeatScanner) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.err
}

// HeartbeatsSent returns the total number of heartbeat comments sent during
// this scanner's lifetime. Useful for diagnostics and logging.
func (h *HeartbeatScanner) HeartbeatsSent() int {
	return h.heartbeatsSent
}

// HeartbeatWriteErr returns the first write error encountered when sending
// a heartbeat, or nil if all heartbeats were written successfully.
func (h *HeartbeatScanner) HeartbeatWriteErr() error {
	return h.heartbeatWriteErr
}

// Close signals the background reader goroutine to stop. It is safe to call
// multiple times. After Close, the caller should also close the upstream
// resp.Body to unblock any in-progress scanner.Scan() call in the goroutine.
func (h *HeartbeatScanner) Close() {
	if !h.closed {
		h.closed = true
		close(h.done)
	}
}

func (h *HeartbeatScanner) sendHeartbeat() {
	if h.c == nil || h.c.Writer == nil {
		return
	}
	_, err := h.c.Writer.Write([]byte(heartbeatPayload))
	if err != nil {
		if h.heartbeatWriteErr == nil {
			h.heartbeatWriteErr = err
		}
		return
	}
	h.c.Writer.Flush()
	h.heartbeatsSent++
}
