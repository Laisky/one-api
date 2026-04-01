package render

import (
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

	commonsse "github.com/songquanpeng/one-api/common/sse"
)

// newTestContext creates a gin test context and recorder for heartbeat tests.
func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, w
}

// newTestContextWithCancel creates a gin test context whose request context can be cancelled.
func newTestContextWithCancel() (*gin.Context, *httptest.ResponseRecorder, context.CancelFunc) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	return c, w, cancel
}

// slowReader delays individual chunks to simulate a slow upstream stream source.
type slowReader struct {
	mu       sync.Mutex
	chunks   []string
	delays   []time.Duration
	current  int
	released chan struct{}
}

// Read copies the next delayed chunk into p and returns io.EOF after the last chunk.
func (r *slowReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	idx := r.current
	r.mu.Unlock()

	if idx >= len(r.chunks) {
		return 0, io.EOF
	}

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

	n := copy(p, []byte(data))
	if r.current >= len(r.chunks) {
		return n, io.EOF
	}

	return n, nil
}

// errorWriter simulates a broken downstream writer after an optional number of successful writes.
type errorWriter struct {
	gin.ResponseWriter
	writeErr     error
	writeCount   int
	mu           sync.Mutex
	successCount int
}

// Write forwards successful writes until successCount is exhausted, then returns writeErr.
func (w *errorWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writeCount++
	if w.successCount > 0 && w.writeCount <= w.successCount {
		return w.ResponseWriter.Write(data)
	}
	return 0, w.writeErr
}

// Flush implements gin.ResponseWriter for the broken writer test double.
func (w *errorWriter) Flush() {
	// Intentionally empty.
}

// TestHeartbeatLineReader_ForwardsAllLines verifies the wrapper returns upstream lines unchanged.
func TestHeartbeatLineReader_ForwardsAllLines(t *testing.T) {
	t.Parallel()

	c, _ := newTestContext()
	reader := commonsse.NewLineReader(strings.NewReader("line1\nline2\nline3\n"), commonsse.DefaultLineBufferSize)
	hbr := NewHeartbeatLineReader(c, reader, DefaultHeartbeatInterval)
	defer hbr.Close()

	var lines []string
	for {
		line, err := hbr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		lines = append(lines, line.Text())
	}

	require.Equal(t, []string{"line1", "line2", "line3"}, lines)
}

// TestHeartbeatLineReader_EmptyInput verifies an empty upstream ends immediately with io.EOF.
func TestHeartbeatLineReader_EmptyInput(t *testing.T) {
	t.Parallel()

	c, _ := newTestContext()
	reader := commonsse.NewLineReader(strings.NewReader(""), commonsse.DefaultLineBufferSize)
	hbr := NewHeartbeatLineReader(c, reader, DefaultHeartbeatInterval)
	defer hbr.Close()

	_, err := hbr.Next()
	require.ErrorIs(t, err, io.EOF)
	assert.Nil(t, hbr.HeartbeatWriteErr())
}

// TestHeartbeatLineReader_SendsHeartbeatDuringDelay verifies idle reads emit heartbeat comments.
func TestHeartbeatLineReader_SendsHeartbeatDuringDelay(t *testing.T) {
	t.Parallel()

	c, w := newTestContext()
	reader := &slowReader{
		chunks:   []string{"data: hello\n"},
		delays:   []time.Duration{250 * time.Millisecond},
		released: make(chan struct{}),
	}
	hbr := NewHeartbeatLineReader(c, commonsse.NewLineReader(reader, commonsse.DefaultLineBufferSize), 50*time.Millisecond)
	defer hbr.Close()

	var lines []string
	for {
		line, err := hbr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		lines = append(lines, line.Text())
	}

	require.Equal(t, []string{"data: hello"}, lines)
	assert.GreaterOrEqual(t, strings.Count(w.Body.String(), ":\n"), 2)
	assert.GreaterOrEqual(t, hbr.HeartbeatsSent(), 2)
}

// TestHeartbeatLineReader_NoHeartbeatWhenDataFlows verifies immediate streams do not emit heartbeats.
func TestHeartbeatLineReader_NoHeartbeatWhenDataFlows(t *testing.T) {
	t.Parallel()

	c, w := newTestContext()
	reader := commonsse.NewLineReader(strings.NewReader("line1\nline2\nline3\n"), commonsse.DefaultLineBufferSize)
	hbr := NewHeartbeatLineReader(c, reader, time.Second)
	defer hbr.Close()

	for {
		_, err := hbr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}

	assert.Equal(t, 0, strings.Count(w.Body.String(), ":\n"))
	assert.Equal(t, 0, hbr.HeartbeatsSent())
}

// TestHeartbeatLineReader_CloseStopsGoroutine verifies Close unblocks Next once the upstream reader is released.
func TestHeartbeatLineReader_CloseStopsGoroutine(t *testing.T) {
	t.Parallel()

	c, _ := newTestContext()
	pr, pw := io.Pipe()
	hbr := NewHeartbeatLineReader(c, commonsse.NewLineReader(pr, commonsse.DefaultLineBufferSize), 100*time.Millisecond)

	hbr.Close()
	_ = pw.Close()

	done := make(chan error, 1)
	go func() {
		_, err := hbr.Next()
		done <- err
	}()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, io.EOF)
	case <-time.After(2 * time.Second):
		t.Fatal("Next did not return within 2 seconds after Close")
	}
}

// TestHeartbeatLineReader_CloseIsIdempotent verifies repeated Close calls do not panic.
func TestHeartbeatLineReader_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	c, _ := newTestContext()
	reader := commonsse.NewLineReader(strings.NewReader("line\n"), commonsse.DefaultLineBufferSize)
	hbr := NewHeartbeatLineReader(c, reader, DefaultHeartbeatInterval)

	hbr.Close()
	hbr.Close()
	hbr.Close()
}

// TestHeartbeatLineReader_HeartbeatIsValidSSEComment verifies the heartbeat payload remains a valid SSE comment.
func TestHeartbeatLineReader_HeartbeatIsValidSSEComment(t *testing.T) {
	t.Parallel()

	assert.True(t, strings.HasPrefix(heartbeatPayload, ":"))
	assert.True(t, strings.HasSuffix(heartbeatPayload, "\n"))
	assert.False(t, strings.HasSuffix(heartbeatPayload, "\n\n"))
}

// TestHeartbeatLineReader_FlushesHeadersImmediately verifies headers are flushed on construction.
func TestHeartbeatLineReader_FlushesHeadersImmediately(t *testing.T) {
	t.Parallel()

	c, w := newTestContext()
	c.Writer.Header().Set("Content-Type", "text/event-stream")

	pr, pw := io.Pipe()
	hbr := NewHeartbeatLineReader(c, commonsse.NewLineReader(pr, commonsse.DefaultLineBufferSize), DefaultHeartbeatInterval)
	defer hbr.Close()
	defer pw.Close()

	assert.True(t, w.Flushed)
}

// TestHeartbeatLineReader_InterleavesHeartbeatsWithData verifies heartbeats fill pauses between upstream chunks.
func TestHeartbeatLineReader_InterleavesHeartbeatsWithData(t *testing.T) {
	t.Parallel()

	c, w := newTestContext()
	pr, pw := io.Pipe()
	hbr := NewHeartbeatLineReader(c, commonsse.NewLineReader(pr, commonsse.DefaultLineBufferSize), 30*time.Millisecond)
	defer hbr.Close()

	go func() {
		defer pw.Close()
		_, _ = pw.Write([]byte("data: chunk1\n"))
		time.Sleep(100 * time.Millisecond)
		_, _ = pw.Write([]byte("data: chunk2\n"))
		time.Sleep(100 * time.Millisecond)
		_, _ = pw.Write([]byte("data: [DONE]\n"))
	}()

	var lines []string
	for {
		line, err := hbr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		lines = append(lines, line.Text())
	}

	assert.Equal(t, []string{"data: chunk1", "data: chunk2", "data: [DONE]"}, lines)
	assert.GreaterOrEqual(t, strings.Count(w.Body.String(), ":\n"), 2)
}

// TestHeartbeatLineReader_ClientDisconnect verifies downstream cancellation stops waiting reads.
func TestHeartbeatLineReader_ClientDisconnect(t *testing.T) {
	t.Parallel()

	c, _, cancel := newTestContextWithCancel()
	pr, pw := io.Pipe()
	defer pw.Close()

	hbr := NewHeartbeatLineReader(c, commonsse.NewLineReader(pr, commonsse.DefaultLineBufferSize), 50*time.Millisecond)
	defer hbr.Close()

	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	go func() {
		_, err := hbr.Next()
		done <- err
	}()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("Next did not return within 2 seconds after client disconnect")
	}
}

// TestHeartbeatLineReader_ClientDisconnectDuringData verifies reads stop after cancellation between chunks.
func TestHeartbeatLineReader_ClientDisconnectDuringData(t *testing.T) {
	t.Parallel()

	c, _, cancel := newTestContextWithCancel()
	pr, pw := io.Pipe()
	hbr := NewHeartbeatLineReader(c, commonsse.NewLineReader(pr, commonsse.DefaultLineBufferSize), 50*time.Millisecond)
	defer hbr.Close()

	go func() {
		defer pw.Close()
		_, _ = pw.Write([]byte("data: chunk1\n"))
		time.Sleep(50 * time.Millisecond)
		cancel()
		time.Sleep(200 * time.Millisecond)
		_, _ = pw.Write([]byte("data: chunk2\n"))
	}()

	line, err := hbr.Next()
	require.NoError(t, err)
	assert.Equal(t, "data: chunk1", line.Text())

	_, err = hbr.Next()
	assert.ErrorIs(t, err, context.Canceled)
}

// TestHeartbeatLineReader_HeartbeatsSentCounter verifies the heartbeat counter tracks successful writes.
func TestHeartbeatLineReader_HeartbeatsSentCounter(t *testing.T) {
	t.Parallel()

	c, _ := newTestContext()
	reader := &slowReader{
		chunks:   []string{"data: hello\n"},
		delays:   []time.Duration{250 * time.Millisecond},
		released: make(chan struct{}),
	}
	hbr := NewHeartbeatLineReader(c, commonsse.NewLineReader(reader, commonsse.DefaultLineBufferSize), 50*time.Millisecond)
	defer hbr.Close()

	for {
		_, err := hbr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}

	assert.GreaterOrEqual(t, hbr.HeartbeatsSent(), 2)
	assert.Nil(t, hbr.HeartbeatWriteErr())
}

// TestHeartbeatLineReader_HeartbeatsSentZeroWhenFast verifies no heartbeats are counted when data flows faster than the interval.
func TestHeartbeatLineReader_HeartbeatsSentZeroWhenFast(t *testing.T) {
	t.Parallel()

	c, _ := newTestContext()
	reader := commonsse.NewLineReader(strings.NewReader("line1\nline2\nline3\n"), commonsse.DefaultLineBufferSize)
	hbr := NewHeartbeatLineReader(c, reader, time.Second)
	defer hbr.Close()

	for {
		_, err := hbr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}

	assert.Equal(t, 0, hbr.HeartbeatsSent())
}

// TestHeartbeatLineReader_HeartbeatWriteError verifies heartbeat write errors are captured.
func TestHeartbeatLineReader_HeartbeatWriteError(t *testing.T) {
	t.Parallel()

	c, w := newTestContext()
	c.Writer = &errorWriter{
		ResponseWriter: c.Writer,
		writeErr:       io.ErrClosedPipe,
	}

	reader := &slowReader{
		chunks:   []string{"data: hello\n"},
		delays:   []time.Duration{200 * time.Millisecond},
		released: make(chan struct{}),
	}
	hbr := NewHeartbeatLineReader(c, commonsse.NewLineReader(reader, commonsse.DefaultLineBufferSize), 50*time.Millisecond)
	defer hbr.Close()

	for {
		_, err := hbr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}

	assert.Equal(t, 0, hbr.HeartbeatsSent())
	assert.ErrorIs(t, hbr.HeartbeatWriteErr(), io.ErrClosedPipe)
	assert.Empty(t, w.Body.String())
}

// TestHeartbeatLineReader_KeepAliveForwardingSimulation verifies heartbeats fill realistic upstream keepalive gaps.
func TestHeartbeatLineReader_KeepAliveForwardingSimulation(t *testing.T) {
	t.Parallel()

	c, w := newTestContext()
	pr, pw := io.Pipe()
	hbr := NewHeartbeatLineReader(c, commonsse.NewLineReader(pr, commonsse.DefaultLineBufferSize), 30*time.Millisecond)
	defer hbr.Close()

	go func() {
		defer pw.Close()
		_, _ = pw.Write([]byte("event: response.created\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"response.created\"}\n"))
		_, _ = pw.Write([]byte("\n"))
		time.Sleep(150 * time.Millisecond)
		_, _ = pw.Write([]byte("event: keepalive\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"keepalive\",\"sequence_number\":2}\n"))
		_, _ = pw.Write([]byte("\n"))
		time.Sleep(150 * time.Millisecond)
		_, _ = pw.Write([]byte("data: [DONE]\n"))
	}()

	var lines []string
	for {
		line, err := hbr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		lines = append(lines, line.Text())
	}

	assert.Contains(t, lines, "event: response.created")
	assert.Contains(t, lines, "data: {\"type\":\"response.created\"}")
	assert.Contains(t, lines, "event: keepalive")
	assert.Contains(t, lines, "data: {\"type\":\"keepalive\",\"sequence_number\":2}")
	assert.Contains(t, lines, "data: [DONE]")

	heartbeatCount := strings.Count(w.Body.String(), ":\n")
	assert.GreaterOrEqual(t, heartbeatCount, 4)
	assert.Equal(t, heartbeatCount, hbr.HeartbeatsSent())
}
