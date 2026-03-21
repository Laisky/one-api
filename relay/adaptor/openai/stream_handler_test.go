package openai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
	"github.com/songquanpeng/one-api/relay/streaming"
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

// ---------------------------------------------------------------------------
// 9. StreamRewriter integration
// ---------------------------------------------------------------------------

type mockStreamRewriter struct {
	chunks       int
	upstreamDone bool
	doneCalled   bool
	finalUsage   *model.Usage
}

func (m *mockStreamRewriter) HandleChunk(c *gin.Context, chunk *openai_compatible.ChatCompletionsStreamResponse) (bool, bool) {
	m.chunks++
	// Let the default handler forward the first chunk, but handle the rest
	if m.chunks > 1 {
		return true, false
	}
	return false, false
}

func (m *mockStreamRewriter) HandleUpstreamDone(c *gin.Context) (bool, bool) {
	m.upstreamDone = true
	return true, true // Handle it and mark done
}

func (m *mockStreamRewriter) HandleDone(c *gin.Context) (bool, bool) {
	m.doneCalled = true
	return true, true
}

func (m *mockStreamRewriter) FinalizeUsage(usage *model.Usage) {
	m.finalUsage = usage
}

func TestStreamHandler_StreamRewriter(t *testing.T) {
	c, w := newTestGinContext()

	rewriter := &mockStreamRewriter{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rewriter)

	sseData := strings.Join([]string{
		chatChunk("first", nil),
		"",
		chatChunk("second", nil),
		"",
		`data: {"id":"chatcmpl-u","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, usage := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "first" + "second", responseText)

	// Verify rewriter was called
	assert.Equal(t, 3, rewriter.chunks, "HandleChunk called for each parsed chunk")
	assert.True(t, rewriter.upstreamDone, "HandleUpstreamDone called")
	assert.True(t, rewriter.doneCalled, "HandleDone called")
	assert.NotNil(t, rewriter.finalUsage)
	assert.Equal(t, 5, rewriter.finalUsage.PromptTokens)

	require.NotNil(t, usage)
	assert.Equal(t, 8, usage.TotalTokens)

	body := w.Body.String()
	// Only the first chunk should be forwarded (rewriter handled the rest)
	assert.Contains(t, body, `"content":"first"`)
}

// ---------------------------------------------------------------------------
// 10. Non-data lines are skipped
// ---------------------------------------------------------------------------
func TestStreamHandler_NonDataLinesSkipped(t *testing.T) {
	c, w := newTestGinContext()

	sseData := strings.Join([]string{
		": this is a comment",
		"event: some_event",
		"id: 123",
		"retry: 5000",
		chatChunk("real_data", nil),
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "real_data", responseText)

	body := w.Body.String()
	assert.Contains(t, body, `"content":"real_data"`)
	// The non-data lines should not appear in output
	assert.NotContains(t, body, "this is a comment")
	assert.NotContains(t, body, "some_event")
	assert.Contains(t, body, "[DONE]")
}

// ---------------------------------------------------------------------------
// 11. Empty choices are skipped (Azure behavior)
// ---------------------------------------------------------------------------
func TestStreamHandler_EmptyChoicesSkipped(t *testing.T) {
	c, _ := newTestGinContext()

	sseData := strings.Join([]string{
		// Azure sends an initial chunk with empty choices
		`data: {"id":"chatcmpl-az","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4","choices":[]}`,
		"",
		chatChunk("content", nil),
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "content", responseText)
}

// ---------------------------------------------------------------------------
// 12. Usage-only chunk (no choices)
// ---------------------------------------------------------------------------
func TestStreamHandler_UsageOnlyChunk(t *testing.T) {
	c, _ := newTestGinContext()

	sseData := strings.Join([]string{
		chatChunk("hi", nil),
		"",
		// A chunk with usage but no choices (OpenAI stream_options: include_usage)
		`data: {"id":"chatcmpl-u2","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4","choices":[],"usage":{"prompt_tokens":20,"completion_tokens":10,"total_tokens":30}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, _, usage := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	require.NotNil(t, usage)
	assert.Equal(t, 30, usage.TotalTokens)
}

// ---------------------------------------------------------------------------
// 13. reasoning_content field (deepseek format)
// ---------------------------------------------------------------------------
func TestStreamHandler_ReasoningContentField(t *testing.T) {
	c, _ := newTestGinContext()

	sseData := strings.Join([]string{
		`data: {"id":"chatcmpl-rc","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-r1","choices":[{"index":0,"delta":{"reasoning_content":"Step 1: "},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-rc","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-r1","choices":[{"index":0,"delta":{"reasoning_content":"done."},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-rc","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-r1","choices":[{"index":0,"delta":{"content":"Result"},"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "Step 1: done.Result", responseText)

	converted, exists := c.Get(ctxkey.ConvertedResponse)
	require.True(t, exists)
	m := converted.(map[string]any)
	assert.Equal(t, "Step 1: done.", m["reasoning"])
}

// ---------------------------------------------------------------------------
// 14. data: prefix without space (NormalizeDataLine handles this)
// ---------------------------------------------------------------------------
func TestStreamHandler_DataPrefixNoSpace(t *testing.T) {
	c, _ := newTestGinContext()

	// Some providers send "data:{json}" without a space
	sseData := strings.Join([]string{
		`data:{"id":"chatcmpl-ns","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"no_space"},"finish_reason":null}]}`,
		"",
		"data:[DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "no_space", responseText)
}

// ---------------------------------------------------------------------------
// 15. StreamRewriter HandleDone fallback (rewriter returns handled=false)
// ---------------------------------------------------------------------------
func TestStreamHandler_StreamRewriterFallback(t *testing.T) {
	c, w := newTestGinContext()

	rewriter := &fallbackRewriter{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rewriter)

	sseData := strings.Join([]string{
		chatChunk("data", nil),
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, _, _ := StreamHandler(c, resp, relaymode.ChatCompletions)
	require.Nil(t, errResp)

	body := w.Body.String()
	// [DONE] should be present from the upstream forwarding (HandleUpstreamDone returns false)
	assert.Contains(t, body, "[DONE]")
	assert.True(t, rewriter.finalizeCalled)
}

type fallbackRewriter struct {
	finalizeCalled bool
}

func (f *fallbackRewriter) HandleChunk(_ *gin.Context, _ *openai_compatible.ChatCompletionsStreamResponse) (bool, bool) {
	return false, false
}
func (f *fallbackRewriter) HandleUpstreamDone(_ *gin.Context) (bool, bool) {
	return false, false
}
func (f *fallbackRewriter) HandleDone(_ *gin.Context) (bool, bool) {
	return false, false // Not handled -> fallback renders Done
}
func (f *fallbackRewriter) FinalizeUsage(usage *model.Usage) {
	f.finalizeCalled = true
}

// ---------------------------------------------------------------------------
// 16. Multiple choices in a single chunk
// ---------------------------------------------------------------------------
func TestStreamHandler_MultipleChoices(t *testing.T) {
	c, _ := newTestGinContext()

	sseData := strings.Join([]string{
		`data: {"id":"chatcmpl-mc","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"A"},"finish_reason":null},{"index":1,"delta":{"content":"B"},"finish_reason":null}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "AB", responseText)
}

// ---------------------------------------------------------------------------
// 17. Body close error
// ---------------------------------------------------------------------------
type failCloseReader struct {
	io.Reader
}

func (f *failCloseReader) Close() error {
	return errors.New("close failed")
}

func TestStreamHandler_BodyCloseError(t *testing.T) {
	c, _ := newTestGinContext()

	sseData := "data: [DONE]\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       &failCloseReader{Reader: strings.NewReader(sseData)},
	}

	errResp, _, _ := StreamHandler(c, resp, relaymode.ChatCompletions)
	require.NotNil(t, errResp)
	assert.Equal(t, http.StatusInternalServerError, errResp.StatusCode)
	assert.Contains(t, errResp.Error.Message, "close failed")
}

// ---------------------------------------------------------------------------
// 18. Completions relay mode
// ---------------------------------------------------------------------------
func TestStreamHandler_CompletionsMode(t *testing.T) {
	c, w := newTestGinContext()

	sseData := strings.Join([]string{
		`data: {"id":"cmpl-1","object":"text_completion","created":1700000000,"model":"gpt-3.5-turbo-instruct","choices":[{"text":"Hello ","index":0,"finish_reason":null}]}`,
		"",
		`data: {"id":"cmpl-1","object":"text_completion","created":1700000000,"model":"gpt-3.5-turbo-instruct","choices":[{"text":"world!","index":0,"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, _ := StreamHandler(c, resp, relaymode.Completions)

	require.Nil(t, errResp)
	assert.Equal(t, "Hello world!", responseText)

	body := w.Body.String()
	assert.Contains(t, body, `"text":"Hello "`)
	assert.Contains(t, body, `"text":"world!"`)
	assert.Contains(t, body, "[DONE]")
}

// ---------------------------------------------------------------------------
// 19. Completions mode with malformed JSON
// ---------------------------------------------------------------------------
func TestStreamHandler_CompletionsMalformed(t *testing.T) {
	c, w := newTestGinContext()

	sseData := strings.Join([]string{
		`data: {bad json}`,
		"",
		`data: {"id":"cmpl-2","object":"text_completion","created":1700000000,"model":"gpt-3.5-turbo-instruct","choices":[{"text":"ok","index":0,"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, _ := StreamHandler(c, resp, relaymode.Completions)

	require.Nil(t, errResp)
	assert.Equal(t, "ok", responseText)

	body := w.Body.String()
	// Both lines are forwarded in Completions mode (StringData before parse)
	assert.Contains(t, body, "bad json")
	assert.Contains(t, body, "[DONE]")
}

// ---------------------------------------------------------------------------
// 20. Tracker with metaInfo: exercises the tracker code path
// ---------------------------------------------------------------------------
func TestStreamHandler_WithTracker(t *testing.T) {
	ensureTokenEncoders()
	c, w := newTestGinContext()

	// Set up meta so the tracker code path is reached
	meta := &metalib.Meta{
		ActualModelName: "gpt-4",
	}
	c.Set(ctxkey.Meta, meta)

	// Create a tracker with a very long flush interval so it doesn't hit DB
	tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{
		FlushInterval: 1 * time.Hour,
		Ctx:           context.Background(),
	})
	streaming.StoreTracker(c, tracker)

	sseData := strings.Join([]string{
		chatChunk("Hello", nil),
		"",
		chatChunk(" world", nil),
		"",
		`data: {"id":"chatcmpl-t","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, usage := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "Hello world", responseText)
	require.NotNil(t, usage)
	assert.Equal(t, 15, usage.TotalTokens)

	body := w.Body.String()
	assert.Contains(t, body, `"content":"Hello"`)
	assert.Contains(t, body, "[DONE]")
}

// ---------------------------------------------------------------------------
// 21. Tracker with reasoning content: exercises tracker delta tokens path
//     for both content and reasoning
// ---------------------------------------------------------------------------
func TestStreamHandler_TrackerWithReasoning(t *testing.T) {
	ensureTokenEncoders()
	c, _ := newTestGinContext()

	meta := &metalib.Meta{
		ActualModelName: "deepseek-r1",
	}
	c.Set(ctxkey.Meta, meta)

	tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{
		FlushInterval: 1 * time.Hour,
		Ctx:           context.Background(),
	})
	streaming.StoreTracker(c, tracker)

	sseData := strings.Join([]string{
		`data: {"id":"chatcmpl-tr","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-r1","choices":[{"index":0,"delta":{"reasoning":"think"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-tr","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-r1","choices":[{"index":0,"delta":{"content":"answer"},"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, "thinkanswer", responseText)
}

// ---------------------------------------------------------------------------
// 22. Completions mode with tracker
// ---------------------------------------------------------------------------
func TestStreamHandler_CompletionsModeWithTracker(t *testing.T) {
	ensureTokenEncoders()
	c, _ := newTestGinContext()

	meta := &metalib.Meta{
		ActualModelName: "gpt-3.5-turbo-instruct",
	}
	c.Set(ctxkey.Meta, meta)

	tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{
		FlushInterval: 1 * time.Hour,
		Ctx:           context.Background(),
	})
	streaming.StoreTracker(c, tracker)

	sseData := strings.Join([]string{
		`data: {"id":"cmpl-tr","object":"text_completion","created":1700000000,"model":"gpt-3.5-turbo-instruct","choices":[{"text":"tracked","index":0,"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, responseText, _ := StreamHandler(c, resp, relaymode.Completions)

	require.Nil(t, errResp)
	assert.Equal(t, "tracked", responseText)
}

// ---------------------------------------------------------------------------
// 23. StreamRewriter HandleChunk returns doneRendered=true
// ---------------------------------------------------------------------------

type doneRenderingRewriter struct {
	chunkCount int
	finalUsage *model.Usage
}

func (d *doneRenderingRewriter) HandleChunk(_ *gin.Context, _ *openai_compatible.ChatCompletionsStreamResponse) (bool, bool) {
	d.chunkCount++
	// On second chunk, mark doneRendered
	if d.chunkCount == 2 {
		return true, true // handled + doneRendered
	}
	return true, false
}
func (d *doneRenderingRewriter) HandleUpstreamDone(_ *gin.Context) (bool, bool) {
	return true, false
}
func (d *doneRenderingRewriter) HandleDone(_ *gin.Context) (bool, bool) {
	return true, true
}
func (d *doneRenderingRewriter) FinalizeUsage(usage *model.Usage) {
	d.finalUsage = usage
}

func TestStreamHandler_RewriterChunkDoneRendered(t *testing.T) {
	c, _ := newTestGinContext()

	rewriter := &doneRenderingRewriter{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rewriter)

	sseData := strings.Join([]string{
		chatChunk("a", nil),
		"",
		chatChunk("b", nil),
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, _, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.Equal(t, 2, rewriter.chunkCount)
}

// ---------------------------------------------------------------------------
// 24. Rewriter HandleDone not handled, doneRendered already true
//     (covers line 304-307 else-if branch)
// ---------------------------------------------------------------------------

type doneAlreadyRewriter struct {
	finalizeCalled bool
}

func (d *doneAlreadyRewriter) HandleChunk(_ *gin.Context, _ *openai_compatible.ChatCompletionsStreamResponse) (bool, bool) {
	return false, false
}
func (d *doneAlreadyRewriter) HandleUpstreamDone(_ *gin.Context) (bool, bool) {
	return false, false // Let default handle [DONE] -> doneRendered=true
}
func (d *doneAlreadyRewriter) HandleDone(_ *gin.Context) (bool, bool) {
	return false, false // Not handled, but doneRendered is already true from [DONE]
}
func (d *doneAlreadyRewriter) FinalizeUsage(usage *model.Usage) {
	d.finalizeCalled = true
}

func TestStreamHandler_RewriterHandleDoneNotHandledAlreadyDone(t *testing.T) {
	c, w := newTestGinContext()

	rewriter := &doneAlreadyRewriter{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rewriter)

	sseData := strings.Join([]string{
		chatChunk("x", nil),
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, _, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.True(t, rewriter.finalizeCalled)

	body := w.Body.String()
	// [DONE] from upstream forwarding, and no additional [DONE] from HandleDone fallback
	assert.Contains(t, body, "[DONE]")
}

// ---------------------------------------------------------------------------
// 25. Rewriter HandleDone not handled + upstream dropped without [DONE]
//     (covers line 304-307: fallback render.Done with rewriter present)
// ---------------------------------------------------------------------------

type noHandleDoneRewriter struct {
	finalizeCalled bool
}

func (r *noHandleDoneRewriter) HandleChunk(_ *gin.Context, _ *openai_compatible.ChatCompletionsStreamResponse) (bool, bool) {
	return false, false
}
func (r *noHandleDoneRewriter) HandleUpstreamDone(_ *gin.Context) (bool, bool) {
	return false, false // Not relevant since no [DONE] in stream
}
func (r *noHandleDoneRewriter) HandleDone(_ *gin.Context) (bool, bool) {
	return false, false // Not handled -> fallback render.Done
}
func (r *noHandleDoneRewriter) FinalizeUsage(usage *model.Usage) {
	r.finalizeCalled = true
}

func TestStreamHandler_RewriterNotHandledNoDone(t *testing.T) {
	c, w := newTestGinContext()

	rewriter := &noHandleDoneRewriter{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rewriter)

	// Stream without [DONE]
	sseData := strings.Join([]string{
		chatChunk("y", nil),
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, _, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.Nil(t, errResp)
	assert.True(t, rewriter.finalizeCalled)

	body := w.Body.String()
	// When upstream drops without [DONE], the handler does NOT fabricate [DONE]
	assert.NotContains(t, body, "[DONE]", "handler should NOT fabricate [DONE] when upstream drops")
}

// ---------------------------------------------------------------------------
// 26. Tracker quota exceeded in ChatCompletions mode
//     Pre-set abortErr on the tracker so RecordCompletionTokens fails immediately.
// ---------------------------------------------------------------------------
func TestStreamHandler_TrackerQuotaExceeded(t *testing.T) {
	ensureTokenEncoders()
	c, w := newTestGinContext()

	meta := &metalib.Meta{
		ActualModelName: "gpt-4",
	}
	c.Set(ctxkey.Meta, meta)

	tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{
		FlushInterval: 1 * time.Hour,
		Ctx:           context.Background(),
	})
	setTrackerAbortErr(tracker, streaming.ErrQuotaExceeded)
	streaming.StoreTracker(c, tracker)

	sseData := strings.Join([]string{
		chatChunk("Hello", nil),
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, _, usage := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.NotNil(t, errResp)
	assert.Equal(t, http.StatusForbidden, errResp.StatusCode)

	body := w.Body.String()
	assert.Contains(t, body, "insufficient_user_quota")
	_ = usage
}

// ---------------------------------------------------------------------------
// 27. Tracker generic error (non-quota) in ChatCompletions mode
// ---------------------------------------------------------------------------
func TestStreamHandler_TrackerGenericError(t *testing.T) {
	ensureTokenEncoders()
	c, w := newTestGinContext()

	meta := &metalib.Meta{
		ActualModelName: "gpt-4",
	}
	c.Set(ctxkey.Meta, meta)

	tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{
		FlushInterval: 1 * time.Hour,
		Ctx:           context.Background(),
	})
	setTrackerAbortErr(tracker, errors.New("billing backend down"))
	streaming.StoreTracker(c, tracker)

	sseData := strings.Join([]string{
		chatChunk("Hi", nil),
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, _, _ := StreamHandler(c, resp, relaymode.ChatCompletions)

	require.NotNil(t, errResp)
	assert.Equal(t, http.StatusInternalServerError, errResp.StatusCode)

	body := w.Body.String()
	assert.Contains(t, body, "streaming_billing_failed")
}

// ---------------------------------------------------------------------------
// 28. Tracker quota exceeded in Completions mode
// ---------------------------------------------------------------------------
func TestStreamHandler_CompletionsTrackerQuotaExceeded(t *testing.T) {
	ensureTokenEncoders()
	c, w := newTestGinContext()

	meta := &metalib.Meta{
		ActualModelName: "gpt-3.5-turbo-instruct",
	}
	c.Set(ctxkey.Meta, meta)

	tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{
		FlushInterval: 1 * time.Hour,
		Ctx:           context.Background(),
	})
	setTrackerAbortErr(tracker, streaming.ErrQuotaExceeded)
	streaming.StoreTracker(c, tracker)

	sseData := strings.Join([]string{
		`data: {"id":"cmpl-e","object":"text_completion","created":1700000000,"model":"gpt-3.5-turbo-instruct","choices":[{"text":"hello","index":0,"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, _, _ := StreamHandler(c, resp, relaymode.Completions)

	require.NotNil(t, errResp)
	assert.Equal(t, http.StatusForbidden, errResp.StatusCode)

	body := w.Body.String()
	assert.Contains(t, body, "insufficient_user_quota")
}

// ---------------------------------------------------------------------------
// 29. Tracker generic error in Completions mode
// ---------------------------------------------------------------------------
func TestStreamHandler_CompletionsTrackerGenericError(t *testing.T) {
	ensureTokenEncoders()
	c, w := newTestGinContext()

	meta := &metalib.Meta{
		ActualModelName: "gpt-3.5-turbo-instruct",
	}
	c.Set(ctxkey.Meta, meta)

	tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{
		FlushInterval: 1 * time.Hour,
		Ctx:           context.Background(),
	})
	setTrackerAbortErr(tracker, errors.New("billing down"))
	streaming.StoreTracker(c, tracker)

	sseData := strings.Join([]string{
		`data: {"id":"cmpl-e2","object":"text_completion","created":1700000000,"model":"gpt-3.5-turbo-instruct","choices":[{"text":"world","index":0,"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")

	resp := newSSEResponse(sseData)
	errResp, _, _ := StreamHandler(c, resp, relaymode.Completions)

	require.NotNil(t, errResp)
	assert.Equal(t, http.StatusInternalServerError, errResp.StatusCode)

	body := w.Body.String()
	assert.Contains(t, body, "streaming_billing_failed")
}

