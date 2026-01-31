package openai_compatible

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// flushRecorder wraps httptest.ResponseRecorder and implements http.Flusher
type flushRecorder struct{ *httptest.ResponseRecorder }

func (f *flushRecorder) Flush() {}

func newGinTestContext() (*gin.Context, *flushRecorder) {
	gin.SetMode(gin.TestMode)
	w := &flushRecorder{httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func TestConvertOpenAIResponseToClaudeResponse_ChatCompletions(t *testing.T) {
	t.Parallel()
	// Build a minimal OpenAI chat completion style response JSON
	body := `{
        "id":"chatcmpl-1",
        "model":"gpt-test",
        "object":"chat.completion",
        "created": 1,
        "choices":[{
            "index":0,
            "message":{
                "role":"assistant",
                "content":"Hello",
                "tool_calls":[{
                    "id":"call_1",
                    "type":"function",
                    "function": {"name":"get_weather","arguments":"{\"city\":\"SF\"}"}
                }]
            },
            "finish_reason":"tool_calls"
        }],
        "usage": {"prompt_tokens":5, "completion_tokens":7, "total_tokens":12}
    }`

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	got, errResp := ConvertOpenAIResponseToClaudeResponse(nil, resp)
	require.Nil(t, errResp)
	require.NotNil(t, got)

	outBody, rerr := io.ReadAll(got.Body)
	require.NoError(t, rerr)

	var cr relaymodel.ClaudeResponse
	require.NoError(t, json.Unmarshal(outBody, &cr))

	assert.Equal(t, "gpt-test", cr.Model)
	assert.Equal(t, 5, cr.Usage.InputTokens)
	assert.Equal(t, 7, cr.Usage.OutputTokens)
	assert.Equal(t, "tool_use", cr.StopReason)
	// Expect text and tool_use blocks
	hasText := false
	hasTool := false
	for _, c := range cr.Content {
		if c.Type == "text" && c.Text == "Hello" {
			hasText = true
		}
		if c.Type == "tool_use" && c.ID == "call_1" && c.Name == "get_weather" {
			hasTool = true
			assert.Contains(t, string(c.Input), "\"city\":\"SF\"")
		}
	}
	assert.True(t, hasText)
	assert.True(t, hasTool)
}

func TestConvertOpenAIResponseToClaudeResponse_ResponseAPI(t *testing.T) {
	t.Parallel()
	body := `{
        "id":"resp_1",
        "object":"response",
        "model":"gpt-resp",
        "output":[
            {"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi"}]},
            {"type":"reasoning","summary":[{"type":"summary_text","text":"think"}]},
            {"type":"function_call","call_id":"call_2","name":"foo","arguments":"{\"x\":1}"}
        ],
        "usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7},
        "created_at": 1,
        "status":"completed"
    }`

	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	got, errResp := ConvertOpenAIResponseToClaudeResponse(nil, resp)
	require.Nil(t, errResp)
	outBody, rerr := io.ReadAll(got.Body)
	require.NoError(t, rerr)

	var cr relaymodel.ClaudeResponse
	require.NoError(t, json.Unmarshal(outBody, &cr))
	assert.Equal(t, "gpt-resp", cr.Model)
	assert.Equal(t, 3, cr.Usage.InputTokens)
	assert.Equal(t, 4, cr.Usage.OutputTokens)

	// Expect text, thinking, and tool_use blocks
	types := make(map[string]int)
	for _, c := range cr.Content {
		types[c.Type]++
	}
	assert.GreaterOrEqual(t, types["text"], 1)
	assert.GreaterOrEqual(t, types["thinking"], 1)
	assert.GreaterOrEqual(t, types["tool_use"], 1)
}

func TestConvertOpenAIResponseToClaudeResponse_ResponseAPIOutputJSON(t *testing.T) {
	t.Parallel()
	body := `{
	    "id":"resp_json",
	    "object":"response",
	    "model":"gpt-resp",
	    "output":[
	        {"type":"message","role":"assistant","content":[{"type":"output_json","json":{"topic":"AI","confidence":0.92}}]}
	    ],
	    "usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5},
	    "created_at": 1,
	    "status":"completed"
	}`

	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	got, errResp := ConvertOpenAIResponseToClaudeResponse(nil, resp)
	require.Nil(t, errResp)
	bytes, readErr := io.ReadAll(got.Body)
	require.NoError(t, readErr)

	var cr relaymodel.ClaudeResponse
	require.NoError(t, json.Unmarshal(bytes, &cr))
	require.NotEmpty(t, cr.Content)
	found := false
	for _, block := range cr.Content {
		if block.Type == "text" {
			found = true
			assert.Equal(t, "{\"topic\":\"AI\",\"confidence\":0.92}", block.Text)
		}
	}
	assert.True(t, found, "expected JSON text block")
}

func TestConvertOpenAIStreamToClaudeSSE_BasicsAndUsage(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	// Build a minimal OpenAI-style SSE stream with text, tool delta, and usage
	chunks := []string{
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\\"city\\":\\"SF\\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		`data: {"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n\n") + "\n\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	usage, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 10, "test-model")
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	// Upstream provided usage; prompt tokens should be from param when missing, but we included usage
	assert.Equal(t, 12, usage.PromptTokens)
	assert.Equal(t, 3, usage.CompletionTokens)
	assert.Equal(t, 15, usage.TotalTokens)

	out := w.Body.String()
	// Should include Claude-native SSE events
	assert.Contains(t, out, "\"type\":\"message_start\"")
	assert.Contains(t, out, "\"type\":\"content_block_start\"")
	assert.Contains(t, out, "\"type\":\"content_block_delta\"")
	assert.Contains(t, out, "\"type\":\"message_stop\"")
	assert.Contains(t, out, "data: [DONE]")
}

// TestConvertOpenAIStreamToClaudeSSE_LargePayload verifies large SSE lines are handled without scanner errors.
func TestConvertOpenAIStreamToClaudeSSE_LargePayload(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	largeContent := strings.Repeat("a", 2*1024*1024)
	chunks := []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"" + largeContent + "\"}}]}",
		"data: [DONE]",
	}
	body := strings.Join(chunks, "\n\n") + "\n\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	usage, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 10, "test-model")
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	out := w.Body.String()
	assert.Contains(t, out, "\"type\":\"content_block_delta\"")
	assert.Contains(t, out, largeContent[:1024])
}

func TestConvertOpenAIStreamToClaudeSSE_NoUpstreamUsage_Computed(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	// No usage event; should compute from accumulated text + tool args
	chunks := []string{
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\\"city\\":\\"SF\\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n\n") + "\n\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	usage, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 10, "test-model")
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	// Expect computed usage with simple estimator: "Hello" (5/4=1); tool args may not accumulate in minimal delta => total=11
	assert.Equal(t, 10, usage.PromptTokens)
	assert.Equal(t, 1, usage.CompletionTokens)
	assert.Equal(t, 11, usage.TotalTokens)

	out := w.Body.String()
	assert.Contains(t, out, "\"type\":\"message_start\"")
	assert.Contains(t, out, "data: [DONE]")
}

func TestConvertOpenAIStreamToClaudeSSE_ResponseAPIToolCall(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	chunks := []string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-4o-mini","status":"completed","output":[{"type":"function_call","call_id":"call_123","name":"get_weather","arguments":"{\"location\":\"SF\"}"}],"usage":{"input_tokens":21,"output_tokens":5,"total_tokens":26}}}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n\n") + "\n\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	usage, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 10, "gpt-4o-mini")
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	assert.Equal(t, 21, usage.PromptTokens)
	assert.Equal(t, 5, usage.CompletionTokens)
	assert.Equal(t, 26, usage.TotalTokens)

	out := w.Body.String()
	assert.Contains(t, out, `"content_block":{"id":"call_123","input":{},"name":"get_weather","type":"tool_use"}`)
	assert.Contains(t, out, `"delta":{"partial_json":"{\"location\":\"SF\"}","type":"input_json_delta"}`)
	assert.Contains(t, out, `"type":"message_stop"`)
}

func TestConvertOpenAIStreamToClaudeSSE_ResponseAPIStructuredJSON(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	chunks := []string{
		`data: {"type":"response.output_json.delta","output_index":0,"delta":{"partial_json":"{\"topic\":\"AI\""}}`,
		`data: {"type":"response.output_json.delta","output_index":0,"delta":{"partial_json":",\"confidence\":0.9}"}}`,
		`data: {"type":"response.output_json.done","output_index":0,"output":{"json":"{\"topic\":\"AI\",\"confidence\":0.9}"}}`,
		`data: {"type":"response.completed","response":{"id":"resp_json","object":"response","model":"gpt-5-mini","status":"completed","usage":{"input_tokens":5,"output_tokens":5,"total_tokens":10}}}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n\n") + "\n\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	usage, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 4, "gpt-5-mini")
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.GreaterOrEqual(t, usage.PromptTokens, 0)
	require.Greater(t, usage.CompletionTokens, 0)

	out := w.Body.String()
	require.NotEmpty(t, out)
	assert.Contains(t, out, "\"content_block_start\"")
	assert.Contains(t, out, "topic")
	assert.Contains(t, out, "confidence")
	assert.Contains(t, out, "\"message_stop\"")
}
