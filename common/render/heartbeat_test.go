package render

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, w
}

// newScanner creates a bufio.Scanner from a string.
func newScanner(data string) *bufio.Scanner {
	s := bufio.NewScanner(strings.NewReader(data))
	s.Split(bufio.ScanLines)
	return s
}

// TestHeartbeatScanner_ForwardsAllLines verifies that all lines from the
// underlying scanner are forwarded via Scan()/Text().
func TestHeartbeatScanner_ForwardsAllLines(t *testing.T) {
	c, _ := newTestContext()
	input := "line1\nline2\nline3\n"
	scanner := newScanner(input)

	hbs := NewHeartbeatScanner(c, scanner, DefaultHeartbeatInterval)
	defer hbs.Close()

	var lines []string
	for hbs.Scan() {
		lines = append(lines, hbs.Text())
	}
	require.NoError(t, hbs.Err())
	assert.Equal(t, []string{"line1", "line2", "line3"}, lines)
}

// TestHeartbeatScanner_EmptyInput verifies that an empty input ends
// immediately with no error.
func TestHeartbeatScanner_EmptyInput(t *testing.T) {
	c, _ := newTestContext()
	scanner := newScanner("")

	hbs := NewHeartbeatScanner(c, scanner, DefaultHeartbeatInterval)
	defer hbs.Close()

	assert.False(t, hbs.Scan(), "Scan should return false on empty input")
	assert.NoError(t, hbs.Err())
}

// slowReader is an io.Reader that delays before returning data.
type slowReader struct {
	mu       sync.Mutex
	chunks   []string
	delays   []time.Duration
	current  int
	released chan struct{}
}

func (r *slowReader) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	idx := r.current
	r.mu.Unlock()

	if idx >= len(r.chunks) {
		return 0, io.EOF
	}

	// Wait for delay
	if idx < len(r.delays) && r.delays[idx] > 0 {
		select {
		case <-time.After(r.delays[idx]):
		case <-r.released:
			return 0, io.EOF
		}
	}

	r.mu.Lock()
	data := r.chunks[r.current]
	r.current++
	r.mu.Unlock()

	n = copy(p, []byte(data))
	if r.current >= len(r.chunks) {
		return n, io.EOF
	}
	return n, nil
}

// TestHeartbeatScanner_SendsHeartbeatDuringDelay verifies that heartbeat
// comments are written to the client when the upstream is slow.
func TestHeartbeatScanner_SendsHeartbeatDuringDelay(t *testing.T) {
	c, w := newTestContext()

	// Use a slow reader that delays 250ms before the first (and only) line.
	// Set heartbeat interval to 50ms so we get multiple heartbeats.
	reader := &slowReader{
		chunks:   []string{"data: hello\n"},
		delays:   []time.Duration{250 * time.Millisecond},
		released: make(chan struct{}),
	}
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)

	hbs := NewHeartbeatScanner(c, scanner, 50*time.Millisecond)
	defer hbs.Close()

	var lines []string
	for hbs.Scan() {
		lines = append(lines, hbs.Text())
	}

	require.NoError(t, hbs.Err())
	assert.Equal(t, []string{"data: hello"}, lines)

	// Check that heartbeat comments were written to the response
	body := w.Body.String()
	heartbeatCount := strings.Count(body, ":\n")
	assert.GreaterOrEqual(t, heartbeatCount, 2,
		"expected at least 2 heartbeats during 250ms delay with 50ms interval, got %d. body: %q",
		heartbeatCount, body)
}

// TestHeartbeatScanner_NoHeartbeatWhenDataFlows verifies that no heartbeats
// are sent when data arrives faster than the interval.
func TestHeartbeatScanner_NoHeartbeatWhenDataFlows(t *testing.T) {
	c, w := newTestContext()
	// Multiple lines with no delay — data flows immediately.
	input := "line1\nline2\nline3\n"
	scanner := newScanner(input)

	hbs := NewHeartbeatScanner(c, scanner, 1*time.Second)
	defer hbs.Close()

	for hbs.Scan() {
		// consume all
	}
	require.NoError(t, hbs.Err())

	body := w.Body.String()
	heartbeatCount := strings.Count(body, ":\n")
	assert.Equal(t, 0, heartbeatCount,
		"expected no heartbeats when data flows fast, got %d. body: %q",
		heartbeatCount, body)
}

// TestHeartbeatScanner_CloseStopsGoroutine verifies that Close() terminates
// the background reader goroutine promptly.
func TestHeartbeatScanner_CloseStopsGoroutine(t *testing.T) {
	c, _ := newTestContext()

	// Create a reader that blocks indefinitely (simulates upstream hang)
	pr, pw := io.Pipe()
	defer pw.Close()

	scanner := bufio.NewScanner(pr)
	scanner.Split(bufio.ScanLines)

	hbs := NewHeartbeatScanner(c, scanner, 100*time.Millisecond)

	// Close immediately — this should not hang
	hbs.Close()

	// Close the pipe to unblock the scanner goroutine
	pw.Close()

	// Verify Scan returns false quickly
	done := make(chan bool, 1)
	go func() {
		result := hbs.Scan()
		done <- result
	}()

	select {
	case result := <-done:
		assert.False(t, result, "Scan should return false after Close")
	case <-time.After(2 * time.Second):
		t.Fatal("Scan did not return within 2 seconds after Close")
	}
}

// TestHeartbeatScanner_CloseIsIdempotent verifies calling Close() multiple
// times does not panic.
func TestHeartbeatScanner_CloseIsIdempotent(t *testing.T) {
	c, _ := newTestContext()
	scanner := newScanner("line\n")

	hbs := NewHeartbeatScanner(c, scanner, DefaultHeartbeatInterval)

	// Should not panic
	hbs.Close()
	hbs.Close()
	hbs.Close()
}

// TestHeartbeatScanner_HeartbeatIsValidSSEComment verifies the heartbeat
// payload is a valid SSE comment that clients will ignore.
func TestHeartbeatScanner_HeartbeatIsValidSSEComment(t *testing.T) {
	// Per SSE spec, lines starting with ':' are comments.
	assert.True(t, strings.HasPrefix(heartbeatPayload, ":"),
		"heartbeat payload must start with ':' to be a valid SSE comment")
	assert.True(t, strings.HasSuffix(heartbeatPayload, "\n"),
		"heartbeat payload must end with newline")
	// Must NOT end with double newline to avoid dispatching empty events
	// (cf. openai/openai-go#556)
	assert.False(t, strings.HasSuffix(heartbeatPayload, "\n\n"),
		"heartbeat payload must NOT end with double newline")
}

// TestHeartbeatScanner_FlushesHeadersImmediately verifies that the constructor
// flushes response headers before any upstream data arrives.
func TestHeartbeatScanner_FlushesHeadersImmediately(t *testing.T) {
	c, w := newTestContext()

	// Set SSE headers before creating the scanner
	c.Writer.Header().Set("Content-Type", "text/event-stream")

	// Create a reader that blocks indefinitely
	pr, pw := io.Pipe()

	scanner := bufio.NewScanner(pr)
	scanner.Split(bufio.ScanLines)

	hbs := NewHeartbeatScanner(c, scanner, DefaultHeartbeatInterval)

	// The recorder should have been flushed (status code written)
	assert.True(t, w.Flushed, "expected headers to be flushed on construction")

	hbs.Close()
	pw.Close()
}

// TestHeartbeatScanner_InterleavesHeartbeatsWithData tests a realistic
// scenario where upstream sends data with pauses in between.
func TestHeartbeatScanner_InterleavesHeartbeatsWithData(t *testing.T) {
	c, w := newTestContext()

	pr, pw := io.Pipe()

	scanner := bufio.NewScanner(pr)
	scanner.Split(bufio.ScanLines)

	hbs := NewHeartbeatScanner(c, scanner, 30*time.Millisecond)
	defer hbs.Close()

	// Write data in a separate goroutine with delays
	go func() {
		defer pw.Close()
		pw.Write([]byte("data: chunk1\n"))
		time.Sleep(100 * time.Millisecond) // Should trigger heartbeats
		pw.Write([]byte("data: chunk2\n"))
		time.Sleep(100 * time.Millisecond) // Should trigger heartbeats
		pw.Write([]byte("data: [DONE]\n"))
	}()

	var lines []string
	for hbs.Scan() {
		lines = append(lines, hbs.Text())
	}

	require.NoError(t, hbs.Err())
	assert.Equal(t, []string{"data: chunk1", "data: chunk2", "data: [DONE]"}, lines)

	// Verify heartbeats were sent during the pauses
	body := w.Body.String()
	heartbeatCount := strings.Count(body, ":\n")
	assert.GreaterOrEqual(t, heartbeatCount, 2,
		"expected heartbeats during pauses between chunks, got %d", heartbeatCount)
}

// newTestContextWithCancel creates a test gin.Context whose request context
// can be cancelled to simulate client disconnect.
func newTestContextWithCancel() (*gin.Context, *httptest.ResponseRecorder, context.CancelFunc) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	return c, w, cancel
}

// TestHeartbeatScanner_ClientDisconnect verifies that Scan returns false
// when the client context is cancelled (simulating Cloudflare 524 / browser close).
func TestHeartbeatScanner_ClientDisconnect(t *testing.T) {
	t.Parallel()
	c, _, cancel := newTestContextWithCancel()

	// Create a reader that blocks indefinitely (upstream never finishes)
	pr, pw := io.Pipe()
	defer pw.Close()

	scanner := bufio.NewScanner(pr)
	scanner.Split(bufio.ScanLines)

	hbs := NewHeartbeatScanner(c, scanner, 50*time.Millisecond)
	defer hbs.Close()

	// Cancel the client context after a short delay
	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()

	// Scan should return false because the client disconnected
	done := make(chan bool, 1)
	go func() {
		result := hbs.Scan()
		done <- result
	}()

	select {
	case result := <-done:
		assert.False(t, result, "Scan should return false when client disconnects")
	case <-time.After(2 * time.Second):
		t.Fatal("Scan did not return within 2 seconds after client disconnect")
	}

	// The error should reflect the context cancellation
	err := hbs.Err()
	assert.ErrorIs(t, err, context.Canceled)
}

// TestHeartbeatScanner_ClientDisconnectDuringData verifies that client
// disconnect is detected even when upstream data is flowing (between lines).
func TestHeartbeatScanner_ClientDisconnectDuringData(t *testing.T) {
	t.Parallel()
	c, _, cancel := newTestContextWithCancel()

	pr, pw := io.Pipe()

	scanner := bufio.NewScanner(pr)
	scanner.Split(bufio.ScanLines)

	hbs := NewHeartbeatScanner(c, scanner, 50*time.Millisecond)
	defer hbs.Close()

	// Write first line, then cancel context, then write more
	go func() {
		defer pw.Close()
		pw.Write([]byte("data: chunk1\n"))
		time.Sleep(50 * time.Millisecond)
		cancel()
		time.Sleep(200 * time.Millisecond)
		pw.Write([]byte("data: chunk2\n")) // should not be received
	}()

	var lines []string
	for hbs.Scan() {
		lines = append(lines, hbs.Text())
	}

	// Should have received the first line but not the second
	assert.Contains(t, lines, "data: chunk1")
	assert.NotContains(t, lines, "data: chunk2",
		"should not receive data after client disconnect")
}

// TestHeartbeatScanner_HeartbeatsSentCounter verifies the HeartbeatsSent()
// counter tracks heartbeats accurately.
func TestHeartbeatScanner_HeartbeatsSentCounter(t *testing.T) {
	t.Parallel()
	c, _ := newTestContext()

	reader := &slowReader{
		chunks:   []string{"data: hello\n"},
		delays:   []time.Duration{250 * time.Millisecond},
		released: make(chan struct{}),
	}
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)

	hbs := NewHeartbeatScanner(c, scanner, 50*time.Millisecond)
	defer hbs.Close()

	for hbs.Scan() {
		// consume
	}

	require.NoError(t, hbs.Err())
	sent := hbs.HeartbeatsSent()
	assert.GreaterOrEqual(t, sent, 2,
		"expected at least 2 heartbeats during 250ms delay with 50ms interval, got %d", sent)
	assert.Nil(t, hbs.HeartbeatWriteErr(),
		"expected no heartbeat write error with healthy connection")
}

// TestHeartbeatScanner_HeartbeatsSentZeroWhenFast verifies no heartbeats
// are counted when data flows faster than the interval.
func TestHeartbeatScanner_HeartbeatsSentZeroWhenFast(t *testing.T) {
	t.Parallel()
	c, _ := newTestContext()
	scanner := newScanner("line1\nline2\nline3\n")

	hbs := NewHeartbeatScanner(c, scanner, 1*time.Second)
	defer hbs.Close()

	for hbs.Scan() {
		// consume
	}
	require.NoError(t, hbs.Err())
	assert.Equal(t, 0, hbs.HeartbeatsSent(),
		"expected 0 heartbeats when data flows fast")
}

// errorWriter is a ResponseWriter that returns errors on Write to simulate
// a broken client connection.
type errorWriter struct {
	gin.ResponseWriter
	writeErr     error
	writeCount   int
	mu           sync.Mutex
	successCount int // number of writes to succeed before returning errors
}

func (w *errorWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writeCount++
	if w.successCount > 0 && w.writeCount <= w.successCount {
		return w.ResponseWriter.Write(data)
	}
	return 0, w.writeErr
}

func (w *errorWriter) Flush() {
	// no-op for broken writer
}

// TestHeartbeatScanner_HeartbeatWriteError verifies that write errors during
// heartbeat sending are captured and subsequent heartbeats are skipped.
func TestHeartbeatScanner_HeartbeatWriteError(t *testing.T) {
	t.Parallel()
	c, w := newTestContext()

	// Wrap the writer with one that fails on Write
	brokenWriter := &errorWriter{
		ResponseWriter: c.Writer,
		writeErr:       io.ErrClosedPipe,
	}
	c.Writer = brokenWriter

	reader := &slowReader{
		chunks:   []string{"data: hello\n"},
		delays:   []time.Duration{200 * time.Millisecond},
		released: make(chan struct{}),
	}
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)

	hbs := NewHeartbeatScanner(c, scanner, 50*time.Millisecond)
	defer hbs.Close()

	for hbs.Scan() {
		// consume
	}

	// HeartbeatsSent should be 0 since all writes failed
	assert.Equal(t, 0, hbs.HeartbeatsSent(),
		"no heartbeats should be counted when writes fail")

	// The write error should be captured
	assert.ErrorIs(t, hbs.HeartbeatWriteErr(), io.ErrClosedPipe)

	// The response body should be empty (no successful heartbeat writes)
	assert.Empty(t, w.Body.String(), "nothing should be written to a broken writer")
}

// TestHeartbeatScanner_KeepAliveForwardingSimulation simulates the exact
// scenario from the 524 bug: upstream sends keepalive events every 30s-like
// interval, and the HeartbeatScanner should fill the gaps with heartbeats.
func TestHeartbeatScanner_KeepAliveForwardingSimulation(t *testing.T) {
	t.Parallel()
	c, w := newTestContext()

	pr, pw := io.Pipe()

	scanner := bufio.NewScanner(pr)
	scanner.Split(bufio.ScanLines)

	// Use 30ms heartbeat interval (simulating 5s in production but faster for tests)
	hbs := NewHeartbeatScanner(c, scanner, 30*time.Millisecond)
	defer hbs.Close()

	// Simulate the upstream sending pattern from the bug:
	// response.created immediately, then keepalives with gaps
	go func() {
		defer pw.Close()
		// Initial events arrive immediately
		pw.Write([]byte("event: response.created\n"))
		pw.Write([]byte("data: {\"type\":\"response.created\"}\n"))
		pw.Write([]byte("\n"))

		// 150ms gap (simulating 30s upstream thinking pause)
		time.Sleep(150 * time.Millisecond)

		// Keepalive arrives
		pw.Write([]byte("event: keepalive\n"))
		pw.Write([]byte("data: {\"type\":\"keepalive\",\"sequence_number\":2}\n"))
		pw.Write([]byte("\n"))

		// Another 150ms gap
		time.Sleep(150 * time.Millisecond)

		// Stream ends
		pw.Write([]byte("data: [DONE]\n"))
	}()

	var lines []string
	for hbs.Scan() {
		lines = append(lines, hbs.Text())
	}

	require.NoError(t, hbs.Err())

	// Verify all upstream lines were forwarded
	assert.Contains(t, lines, "event: response.created")
	assert.Contains(t, lines, "data: {\"type\":\"response.created\"}")
	assert.Contains(t, lines, "event: keepalive")
	assert.Contains(t, lines, "data: {\"type\":\"keepalive\",\"sequence_number\":2}")
	assert.Contains(t, lines, "data: [DONE]")

	// Verify heartbeats were sent during the gaps
	body := w.Body.String()
	heartbeatCount := strings.Count(body, ":\n")
	assert.GreaterOrEqual(t, heartbeatCount, 4,
		"expected at least 4 heartbeats during two 150ms gaps with 30ms interval, got %d", heartbeatCount)

	// HeartbeatsSent counter should match
	assert.Equal(t, heartbeatCount, hbs.HeartbeatsSent(),
		"HeartbeatsSent() should match actual heartbeats written")
}
