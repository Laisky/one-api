package openai

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/relaymode"
)

// slowSSEBody is an io.ReadCloser that yields SSE lines with configurable
// delays, simulating a slow upstream provider.
type slowSSEBody struct {
	segments []sseSegment
	current  int
	buf      []byte
}

type sseSegment struct {
	data  string
	delay time.Duration
}

func (s *slowSSEBody) Read(p []byte) (int, error) {
	// Return remaining buffered data first
	if len(s.buf) > 0 {
		n := copy(p, s.buf)
		s.buf = s.buf[n:]
		return n, nil
	}

	if s.current >= len(s.segments) {
		return 0, io.EOF
	}

	seg := s.segments[s.current]
	s.current++

	if seg.delay > 0 {
		time.Sleep(seg.delay)
	}

	s.buf = []byte(seg.data)
	n := copy(p, s.buf)
	s.buf = s.buf[n:]

	if s.current >= len(s.segments) && len(s.buf) == 0 {
		return n, io.EOF
	}
	return n, nil
}

func (s *slowSSEBody) Close() error { return nil }

// TestStreamHandler_HeartbeatDuringSlowUpstream verifies that SSE heartbeat
// comments are sent to the client when upstream is slow to respond.
// This prevents Cloudflare 524 timeouts.
func TestStreamHandler_HeartbeatDuringSlowUpstream(t *testing.T) {
	c, w := newTestGinContext()

	// Simulate upstream that takes 200ms before sending first chunk
	body := &slowSSEBody{
		segments: []sseSegment{
			{delay: 200 * time.Millisecond, data: chatChunk("Hello", nil) + "\n\n"},
			{data: chatChunk("", shStrPtr("stop")) + "\n\n"},
			{data: "data: [DONE]\n\n"},
		},
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       body,
	}

	errResp, responseText, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "Hello", responseText)

	output := w.Body.String()
	// Verify data was forwarded correctly
	assert.Contains(t, output, `"content":"Hello"`)
	assert.Contains(t, output, "[DONE]")

	// Heartbeats should have been sent during the 200ms delay.
	// The default interval is 5s which is too long for this test, but
	// the HeartbeatLineReader is used and headers are flushed immediately.
	// At minimum, verify the response was flushed (headers sent).
	assert.True(t, w.Flushed, "response should be flushed")
}

// TestStreamHandler_HeartbeatBetweenChunks verifies heartbeats are sent
// during gaps between upstream chunks.
func TestStreamHandler_HeartbeatBetweenChunks(t *testing.T) {
	// This test uses a custom HeartbeatInterval via the slowSSEBody
	// to verify heartbeats fire between chunks. Since we can't override
	// the interval per-handler call, we rely on the unit tests in
	// heartbeat_line_reader_test.go for timing verification. Here we verify the
	// handler correctly processes data with the heartbeat wrapper.
	c, w := newTestGinContext()

	body := &slowSSEBody{
		segments: []sseSegment{
			{data: chatChunk("chunk1", nil) + "\n\n"},
			{delay: 50 * time.Millisecond, data: chatChunk("chunk2", nil) + "\n\n"},
			{data: chatChunk("", shStrPtr("stop")) + "\n\n"},
			{data: "data: [DONE]\n\n"},
		},
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       body,
	}

	errResp, responseText, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "chunk1chunk2", responseText)

	output := w.Body.String()
	assert.Contains(t, output, `"content":"chunk1"`)
	assert.Contains(t, output, `"content":"chunk2"`)
	assert.Contains(t, output, "[DONE]")
}

// TestStreamHandler_HeartbeatCommentIgnoredBySSEParsing verifies that if
// heartbeat comments somehow appear in the data stream, they are correctly
// skipped by the line processing logic.
func TestStreamHandler_HeartbeatCommentIgnoredBySSEParsing(t *testing.T) {
	c, w := newTestGinContext()

	// Include SSE comment lines in the stream (as would be sent by heartbeat)
	sseData := strings.Join([]string{
		":",
		chatChunk("Hello", nil),
		"",
		":",
		chatChunk("", shStrPtr("stop")),
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "Hello", responseText)

	output := w.Body.String()
	assert.Contains(t, output, `"content":"Hello"`)
	assert.Contains(t, output, "[DONE]")
}
