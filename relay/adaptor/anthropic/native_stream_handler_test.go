package anthropic

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestContext creates a gin test context with a recorder and logger.
func newTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	gmw.SetLogger(c, glog.Shared.Named("anthropic-native-stream-test"))
	return c, recorder
}

// makeSSEResponse wraps an SSE string body into an *http.Response.
func makeSSEResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
	}
}

// ---------------------------------------------------------------------------
// Behavior tests: SSE event: type lines
// ---------------------------------------------------------------------------

// TestNativeStream_EventTypeLinesEmitted verifies that each forwarded SSE event
// includes the proper "event: <type>" line derived from the JSON type field.
func TestNativeStream_EventTypeLinesEmitted(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	sse := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":10,"output_tokens":0}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":0,"output_tokens":5}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	errResp, usage := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	body := recorder.Body.String()

	// Every event type must appear as an SSE event: line
	for _, eventType := range []string{
		"message_start",
		"content_block_start",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	} {
		assert.Contains(t, body, "event: "+eventType+"\n", "missing event: line for %s", eventType)
	}

	// No [DONE] marker in Anthropic native stream
	assert.NotContains(t, body, "[DONE]")
}

// TestNativeStream_EventTypeFromJSONFallback verifies that when upstream omits
// the SSE "event:" line, the event type is inferred from the JSON "type" field.
func TestNativeStream_EventTypeFromJSONFallback(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	// No "event:" lines — only bare "data:" lines (some providers strip event lines)
	sse := `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":5,"output_tokens":0}}}` + "\n\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}` + "\n\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":0,"output_tokens":2}}` + "\n\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	errResp, usage := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	body := recorder.Body.String()

	// Even without upstream event: lines, the handler should infer them from JSON
	assert.Contains(t, body, "event: message_start\n")
	assert.Contains(t, body, "event: content_block_delta\n")
	assert.Contains(t, body, "event: message_delta\n")
	assert.Contains(t, body, "event: message_stop\n")
}

// TestNativeStream_SSEEventTypePreferredOverJSON verifies that when both
// an SSE "event:" line and a JSON "type" field are present, the SSE line wins.
func TestNativeStream_SSEEventTypePreferredOverJSON(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	// Contrived: SSE event: says "ping" but JSON type says "message_start"
	sse := "event: ping\n" +
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":1,"output_tokens":0}}}` + "\n\n"

	errResp, _ := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)

	body := recorder.Body.String()
	// SSE event: line should use the value from the event: line, not the JSON type
	assert.Contains(t, body, "event: ping\n")
	assert.NotContains(t, body, "event: message_start\n")
}

// ---------------------------------------------------------------------------
// Behavior tests: [DONE] handling
// ---------------------------------------------------------------------------

// TestNativeStream_DoneMarkerStripped verifies that upstream [DONE] markers
// (an OpenAI convention) are not forwarded to Anthropic native clients.
func TestNativeStream_DoneMarkerStripped(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	sse := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":1,"output_tokens":0}}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n" +
		"data: [DONE]\n\n"

	errResp, _ := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)

	body := recorder.Body.String()
	assert.NotContains(t, body, "[DONE]")
	assert.Contains(t, body, "event: message_stop\n")
}

// TestNativeStream_NoDoneEmittedAtEnd verifies the handler does not append
// a trailing "data: [DONE]" after all events are processed.
func TestNativeStream_NoDoneEmittedAtEnd(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	sse := "event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	errResp, _ := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)

	body := recorder.Body.String()
	assert.NotContains(t, body, "[DONE]")
	// The last event should be message_stop
	assert.True(t, strings.HasSuffix(strings.TrimSpace(body),
		`data: {"type":"message_stop"}`),
		"stream should end with the last real event, got: %s", body)
}

// ---------------------------------------------------------------------------
// Billing / usage correctness tests
// ---------------------------------------------------------------------------

// TestNativeStream_UsageAccumulation verifies that usage tokens are correctly
// accumulated from message_start and message_delta events.
func TestNativeStream_UsageAccumulation(t *testing.T) {
	t.Parallel()
	c, _ := newTestContext(t)

	sse := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":100,"output_tokens":0,"cache_read_input_tokens":50}}}` + "\n\n" +
		"event: message_start\n" +
		`data: {"type":"message_start","usage":{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":25}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello world"}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":42,"cache_read_input_tokens":5}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	errResp, usage := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	// PromptTokens come from message_delta usage.input_tokens
	assert.Equal(t, 10, usage.PromptTokens)
	// CompletionTokens come from message_delta usage.output_tokens
	assert.Equal(t, 42, usage.CompletionTokens)
	// CachedTokens accumulated from message_start (50+25) + message_delta (5)
	require.NotNil(t, usage.PromptTokensDetails)
	assert.Equal(t, 80, usage.PromptTokensDetails.CachedTokens)
}

// TestNativeStream_UsageCacheCreationTokens verifies cache creation token
// accumulation from both the nested CacheCreation object and the flat field.
func TestNativeStream_UsageCacheCreationTokens(t *testing.T) {
	t.Parallel()
	c, _ := newTestContext(t)

	sse := "event: message_start\n" +
		`data: {"type":"message_start","usage":{"input_tokens":0,"output_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":200}}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","usage":{"input_tokens":5,"output_tokens":10,"cache_creation_input_tokens":300}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	errResp, usage := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	assert.Equal(t, 5, usage.PromptTokens)
	assert.Equal(t, 10, usage.CompletionTokens)
	// CacheWrite5mTokens: 100 (from message_start) + 300 (from message_delta flat field)
	assert.Equal(t, 400, usage.CacheWrite5mTokens)
	// CacheWrite1hTokens: 200 (from message_start nested)
	assert.Equal(t, 200, usage.CacheWrite1hTokens)
}

// TestNativeStream_UsageZeroOnEmptyStream verifies that an empty stream
// returns zero usage without errors.
func TestNativeStream_UsageZeroOnEmptyStream(t *testing.T) {
	t.Parallel()
	c, _ := newTestContext(t)

	errResp, usage := ClaudeNativeStreamHandler(c, makeSSEResponse(""))
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	assert.Equal(t, 0, usage.PromptTokens)
	assert.Equal(t, 0, usage.CompletionTokens)
}

// TestNativeStream_UsageNotAffectedByNonUsageEvents verifies that events
// without usage fields (content_block_delta, etc.) don't corrupt usage accounting.
func TestNativeStream_UsageNotAffectedByNonUsageEvents(t *testing.T) {
	t.Parallel()
	c, _ := newTestContext(t)

	sse := "event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":7,"output_tokens":3}}` + "\n\n"

	errResp, usage := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	assert.Equal(t, 7, usage.PromptTokens)
	assert.Equal(t, 3, usage.CompletionTokens)
}

// ---------------------------------------------------------------------------
// Oversized data tests
// ---------------------------------------------------------------------------

// TestNativeStream_OversizedDataWithEventType verifies that oversized data lines
// are forwarded with the correct "event:" line from the preceding SSE event.
func TestNativeStream_OversizedDataWithEventType(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	largeText := strings.Repeat("x", 128*1024)
	sse := "event: message_start\n" +
		`data: {"type":"message_start","usage":{"input_tokens":3,"cache_read_input_tokens":1}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"` + largeText + `"}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","usage":{"input_tokens":3,"output_tokens":4}}` + "\n\n"

	errResp, usage := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	body := recorder.Body.String()

	// The oversized event should have the correct event: type line
	assert.Contains(t, body, "event: content_block_delta\ndata: ")

	// The message_start event should also have event: line
	assert.Contains(t, body, "event: message_start\n")

	// Usage from message_delta should be correctly accumulated
	assert.Equal(t, 3, usage.PromptTokens)
	assert.Equal(t, 4, usage.CompletionTokens)
	require.NotNil(t, usage.PromptTokensDetails)
	assert.Equal(t, 1, usage.PromptTokensDetails.CachedTokens)

	// The large text content must be present
	assert.Contains(t, body, largeText[:1024])
	assert.Contains(t, body, largeText[len(largeText)-1024:])
}

// TestNativeStream_OversizedDataWithoutEventLine verifies oversized data
// is forwarded even when no preceding "event:" line was seen.
func TestNativeStream_OversizedDataWithoutEventLine(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	largeText := strings.Repeat("y", 128*1024)
	// No event: line before the oversized data
	sse := `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"` + largeText + `"}}` + "\n\n"

	errResp, _ := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)

	body := recorder.Body.String()
	// Should still contain the data
	assert.Contains(t, body, largeText[:1024])
	// Should have a "data: " prefix
	assert.Contains(t, body, "data: ")
}

// ---------------------------------------------------------------------------
// Regression tests
// ---------------------------------------------------------------------------

// TestNativeStream_UnparseableEventForwardedAsIs verifies that events that
// fail JSON parsing are still forwarded to the client with the SSE event type
// if available.
func TestNativeStream_UnparseableEventForwardedAsIs(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	sse := "event: weird_event\n" +
		"data: {this is not valid json}\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	errResp, _ := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)

	body := recorder.Body.String()
	// Unparseable event should still be forwarded
	assert.Contains(t, body, "{this is not valid json}")
	// Should use the SSE event type
	assert.Contains(t, body, "event: weird_event\n")
	// Subsequent events should still work
	assert.Contains(t, body, "event: message_stop\n")
}

// TestNativeStream_EventTypeResetAfterUse verifies that the captured event type
// does not leak from one event to the next.
func TestNativeStream_EventTypeResetAfterUse(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	// First event has SSE event: line, second does not — the second should
	// use JSON type fallback, not the first event's type.
	sse := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":1,"output_tokens":0}}}` + "\n\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}` + "\n\n"

	errResp, _ := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)

	body := recorder.Body.String()
	// First event uses SSE event: type
	assert.Contains(t, body, "event: message_start\n")
	// Second event should use JSON type fallback, NOT "message_start" leaking
	assert.Contains(t, body, "event: content_block_delta\n")
	// "message_start" should appear only once as an event: line
	assert.Equal(t, 1, strings.Count(body, "event: message_start\n"))
}

// TestNativeStream_PingEventsForwarded verifies that ping events are forwarded
// with the correct event type.
func TestNativeStream_PingEventsForwarded(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	sse := "event: ping\n" +
		`data: {"type":"ping"}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	errResp, _ := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)

	body := recorder.Body.String()
	assert.Contains(t, body, "event: ping\ndata: {\"type\":\"ping\"}\n\n")
}

// TestNativeStream_ErrorEventForwarded verifies that error events from
// upstream are correctly forwarded with their event type.
func TestNativeStream_ErrorEventForwarded(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	sse := "event: error\n" +
		`data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}` + "\n\n"

	errResp, _ := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)

	body := recorder.Body.String()
	assert.Contains(t, body, "event: error\n")
	assert.Contains(t, body, "overloaded_error")
}

// TestNativeStream_CommentLinesSkipped verifies SSE comment lines (starting
// with ':') are not forwarded to the client.
func TestNativeStream_CommentLinesSkipped(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	sse := ": this is a comment\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	errResp, _ := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)

	body := recorder.Body.String()
	assert.NotContains(t, body, "this is a comment")
	assert.Contains(t, body, "event: message_stop\n")
}

// TestNativeStream_EmptyTypeFieldNoEventLine verifies that when the JSON
// "type" field is empty and no SSE event: line was seen, no event: line is emitted.
func TestNativeStream_EmptyTypeFieldNoEventLine(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	sse := `data: {"some":"data"}` + "\n\n"

	errResp, _ := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)

	body := recorder.Body.String()
	// Should have data: but no event: line
	assert.Contains(t, body, `data: {"some":"data"}`)
	assert.NotContains(t, body, "event:")
}

// ---------------------------------------------------------------------------
// Full end-to-end realistic stream test
// ---------------------------------------------------------------------------

// TestNativeStream_RealisticConversation simulates a complete realistic
// Anthropic streaming conversation and verifies output format and billing.
func TestNativeStream_RealisticConversation(t *testing.T) {
	t.Parallel()
	c, recorder := newTestContext(t)

	sse := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_abc","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":25,"output_tokens":0,"cache_read_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":30}}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: ping\n" +
		`data: {"type":"ping"}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":", world!"}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":0,"output_tokens":12,"cache_read_input_tokens":10}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	errResp, usage := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	body := recorder.Body.String()

	// Verify all event types present with proper event: lines
	expectedEvents := []string{
		"message_start",
		"content_block_start",
		"ping",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}
	for _, et := range expectedEvents {
		assert.Contains(t, body, "event: "+et+"\n", "missing event: %s", et)
	}

	// Verify no [DONE] marker
	assert.NotContains(t, body, "[DONE]")

	// Verify billing
	assert.Equal(t, 0, usage.PromptTokens, "prompt tokens from message_delta.input_tokens")
	assert.Equal(t, 12, usage.CompletionTokens, "completion tokens from message_delta.output_tokens")

	// Cache read: 100 (message_start nested in message) + 10 (message_delta)
	require.NotNil(t, usage.PromptTokensDetails)
	assert.Equal(t, 110, usage.PromptTokensDetails.CachedTokens)

	// Cache creation from message_start's nested cache_creation
	assert.Equal(t, 50, usage.CacheWrite5mTokens)
	assert.Equal(t, 30, usage.CacheWrite1hTokens)

	// Verify content is forwarded
	assert.Contains(t, body, `"text":"Hello"`)
	assert.Contains(t, body, `"text":", world!"`)

	// Verify the output is well-formed SSE: each event block should have
	// an "event:" line followed by a "data:" line
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "event: ") {
			// Next line should be a data: line
			if i+1 < len(lines) {
				assert.True(t, strings.HasPrefix(lines[i+1], "data: "),
					"event: line at position %d not followed by data: line, got: %q", i, lines[i+1])
			}
		}
	}
}

// TestNativeStream_MultipleMessageDeltaUsageAccumulates verifies that multiple
// message_delta events correctly accumulate usage (edge case for long conversations).
func TestNativeStream_MultipleMessageDeltaUsageAccumulates(t *testing.T) {
	t.Parallel()
	c, _ := newTestContext(t)

	sse := "event: message_delta\n" +
		`data: {"type":"message_delta","usage":{"input_tokens":5,"output_tokens":10}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","usage":{"input_tokens":3,"output_tokens":7}}` + "\n\n"

	errResp, usage := ClaudeNativeStreamHandler(c, makeSSEResponse(sse))
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	assert.Equal(t, 8, usage.PromptTokens, "should accumulate across multiple message_delta events")
	assert.Equal(t, 17, usage.CompletionTokens)
}
