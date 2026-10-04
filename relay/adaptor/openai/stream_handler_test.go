package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"github.com/Laisky/errors/v2"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/Laisky/one-api/relay/streaming"
)

var initTokenEncodersOnce sync.Once

func ensureTokenEncoders() {
	initTokenEncodersOnce.Do(func() {
		InitTokenEncoders()
	})
}

// setTrackerAbortErr sets the unexported abortErr field on a QuotaTracker
// via reflection so that RecordCompletionTokens immediately returns the error.
func setTrackerAbortErr(tracker *streaming.QuotaTracker, err error) {
	v := reflect.ValueOf(tracker).Elem()
	f := v.FieldByName("abortErr")
	// Use unsafe to write to the unexported field
	ptr := unsafe.Pointer(f.UnsafeAddr())
	*(*error)(ptr) = err
}

// helper: build a gin test context backed by an httptest recorder.
func newTestGinContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// Provide a dummy request so gin doesn't panic on c.Query() etc.
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, w
}

// helper: build an http.Response whose body contains the given SSE text.
func newSSEResponse(sseData string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(sseData)),
	}
}

// helper to build a typical chat-completions SSE chunk line.
func chatChunk(content string, finishReason *string) string {
	fr := "null"
	if finishReason != nil {
		fr = `"` + *finishReason + `"`
	}
	return `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"` + content + `"},"finish_reason":` + fr + `}]}`
}

func shStrPtr(s string) *string { return &s }

// ---------------------------------------------------------------------------
// 1. Normal stream: multiple data chunks -> finish_reason:stop -> [DONE]
// ---------------------------------------------------------------------------
func TestStreamHandler_NormalStream(t *testing.T) {
	c, w := newTestGinContext()

	sseData := strings.Join([]string{
		chatChunk("Hello", nil),
		"",
		chatChunk(" world", nil),
		"",
		chatChunk("", shStrPtr("stop")),
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, usage := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp, "expected no error")
	assert.Equal(t, "Hello world", responseText)
	assert.Nil(t, usage, "no usage chunk was sent")

	body := w.Body.String()
	assert.Contains(t, body, `"content":"Hello"`)
	assert.Contains(t, body, `"content":" world"`)
	assert.Contains(t, body, "[DONE]")
}

// ---------------------------------------------------------------------------
// 2. Upstream drops without [DONE]
// ---------------------------------------------------------------------------
func TestStreamHandler_UpstreamDropsWithoutDone(t *testing.T) {
	c, w := newTestGinContext()

	sseData := strings.Join([]string{
		chatChunk("partial", nil),
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "partial", responseText)

	// When upstream drops without sending [DONE], the handler does NOT fabricate [DONE].
	body := w.Body.String()
	assert.Contains(t, body, `"content":"partial"`)
	// No [DONE] should appear since upstream didn't send it
	assert.NotContains(t, body, "[DONE]", "handler should NOT fabricate [DONE] when upstream drops")
}

// ---------------------------------------------------------------------------
// 3. Empty stream: only [DONE]
// ---------------------------------------------------------------------------
func TestStreamHandler_EmptyStream(t *testing.T) {
	c, w := newTestGinContext()

	sseData := "data: [DONE]\n\n"
	resp := newSSEResponse(sseData)

	errResp, responseText, usage := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "", responseText)
	assert.Nil(t, usage)

	body := w.Body.String()
	assert.Contains(t, body, "[DONE]")
}

// ---------------------------------------------------------------------------
// 4. Stream with reasoning content
// ---------------------------------------------------------------------------
func TestStreamHandler_ReasoningContent(t *testing.T) {
	c, w := newTestGinContext()

	sseData := strings.Join([]string{
		`data: {"id":"chatcmpl-r","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-r1","choices":[{"index":0,"delta":{"reasoning":"Let me think"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-r","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-r1","choices":[{"index":0,"delta":{"reasoning":" about this"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-r","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-r1","choices":[{"index":0,"delta":{"content":"The answer is 42"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-r","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-r1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	// responseText = reasoningText + responseText content
	assert.Equal(t, "Let me think about thisThe answer is 42", responseText)

	// Verify the ConvertedResponse context value
	converted, exists := c.Get(ctxkey.ConvertedResponse)
	require.True(t, exists)
	m := converted.(map[string]any)
	assert.Equal(t, "Let me think about this", m["reasoning"])

	body := w.Body.String()
	assert.Contains(t, body, "[DONE]")
	_ = w
}

// ---------------------------------------------------------------------------
// 5. Stream with tool calls
// ---------------------------------------------------------------------------
func TestStreamHandler_ToolCalls(t *testing.T) {
	c, w := newTestGinContext()

	sseData := strings.Join([]string{
		`data: {"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"NYC\"}"}}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	// No text content was sent, so responseText is empty
	assert.Equal(t, "", responseText)

	body := w.Body.String()
	assert.Contains(t, body, "get_weather")
	assert.Contains(t, body, "[DONE]")
	_ = w
}

// ---------------------------------------------------------------------------
// 6. Stream with usage info in final chunk
// ---------------------------------------------------------------------------
func TestStreamHandler_WithUsage(t *testing.T) {
	c, _ := newTestGinContext()

	sseData := strings.Join([]string{
		chatChunk("hi", nil),
		"",
		`data: {"id":"chatcmpl-u","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, usage := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "hi", responseText)
	require.NotNil(t, usage)
	assert.Equal(t, 10, usage.PromptTokens)
	assert.Equal(t, 5, usage.CompletionTokens)
	assert.Equal(t, 15, usage.TotalTokens)
}

// ---------------------------------------------------------------------------
// 7. Malformed JSON in data lines -> skipped gracefully (forwarded raw)
// ---------------------------------------------------------------------------
func TestStreamHandler_MalformedJSON(t *testing.T) {
	c, w := newTestGinContext()

	sseData := strings.Join([]string{
		`data: {this is not valid json}`,
		"",
		chatChunk("ok", nil),
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "ok", responseText)

	body := w.Body.String()
	// The malformed JSON line is still forwarded raw to the client
	assert.Contains(t, body, "this is not valid json")
	assert.Contains(t, body, `"content":"ok"`)
	assert.Contains(t, body, "[DONE]")
}

// ---------------------------------------------------------------------------
// 8. Scanner error: simulate read error
// ---------------------------------------------------------------------------

type streamErrorReader struct {
	data    string
	pos     int
	errOnce bool
}

func (r *streamErrorReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		if !r.errOnce {
			r.errOnce = true
			return 0, errors.New("simulated read error")
		}
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *streamErrorReader) Close() error { return nil }

func TestStreamHandler_ScannerError(t *testing.T) {
	c, w := newTestGinContext()

	// Provide one valid chunk, then the reader will error before EOF.
	partialData := chatChunk("before_error", nil) + "\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       &streamErrorReader{data: partialData},
	}

	errResp, responseText, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	// StreamHandler does not return the scanner error as an ErrorWithStatusCode;
	// it just logs and continues.
	require.Nil(t, errResp)
	assert.Equal(t, "before_error", responseText)

	body := w.Body.String()
	assert.Contains(t, body, `"content":"before_error"`)
}
