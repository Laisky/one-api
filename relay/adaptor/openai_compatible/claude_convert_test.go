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

	relaymodel "github.com/Laisky/one-api/relay/model"
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
		"usage": {
			"prompt_tokens":5,
			"completion_tokens":7,
			"total_tokens":12,
			"prompt_tokens_details":{"cached_tokens":3},
			"cache_write_5m_tokens":11,
			"cache_write_1h_tokens":13
		}
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
	assert.Equal(t, 3, cr.Usage.CacheReadInputTokens)
	assert.Equal(t, 24, cr.Usage.CacheCreationInputTokens)
	require.NotNil(t, cr.Usage.CacheCreation)
	assert.Equal(t, 11, cr.Usage.CacheCreation.Ephemeral5mInputTokens)
	assert.Equal(t, 13, cr.Usage.CacheCreation.Ephemeral1hInputTokens)
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
		"usage":{
			"input_tokens":3,
			"output_tokens":4,
			"total_tokens":7,
			"input_tokens_details":{"cached_tokens":2},
			"cache_write_5m_tokens":5,
			"cache_write_1h_tokens":6
		},
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
	assert.Equal(t, 2, cr.Usage.CacheReadInputTokens)
	assert.Equal(t, 11, cr.Usage.CacheCreationInputTokens)
	require.NotNil(t, cr.Usage.CacheCreation)
	assert.Equal(t, 5, cr.Usage.CacheCreation.Ephemeral5mInputTokens)
	assert.Equal(t, 6, cr.Usage.CacheCreation.Ephemeral1hInputTokens)

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
		`data: {"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":4},"cache_write_5m_tokens":20,"cache_write_1h_tokens":30}}`,
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
	require.NotNil(t, usage.PromptTokensDetails)
	assert.Equal(t, 4, usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 20, usage.CacheWrite5mTokens)
	assert.Equal(t, 30, usage.CacheWrite1hTokens)

	out := w.Body.String()
	// Should include Claude-native SSE events
	assert.Contains(t, out, "\"type\":\"message_start\"")
	assert.Contains(t, out, "\"type\":\"content_block_start\"")
	assert.Contains(t, out, "\"type\":\"content_block_delta\"")
	assert.Contains(t, out, "\"cache_read_input_tokens\":4")
	assert.Contains(t, out, "\"ephemeral_5m_input_tokens\":20")
	assert.Contains(t, out, "\"ephemeral_1h_input_tokens\":30")
	assert.Contains(t, out, "\"type\":\"message_stop\"")
	// Claude format does NOT output [DONE]
	assert.NotContains(t, out, "data: [DONE]")
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
	// Claude format does NOT output [DONE]
	assert.NotContains(t, out, "data: [DONE]")
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

// ---------- Additional coverage tests for ConvertOpenAIStreamToClaudeSSE ----------

// helperParseSSEDataLines extracts all "data: ..." payloads from raw SSE output,
// excluding the literal "[DONE]" sentinel.
func helperParseSSEDataLines(raw string) []map[string]any {
	var results []map[string]any
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err == nil {
			results = append(results, m)
		}
	}
	return results
}

// TestConvertOpenAIStreamToClaudeSSE_EventLinesAndNoDONE verifies that every output
// SSE data line is valid JSON with a "type" field, and that the literal "[DONE]" appears
// exactly once at the end (as Claude proxy convention).
func TestConvertOpenAIStreamToClaudeSSE_EventLinesAndNoDONE(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	chunks := []string{
		`data: {"choices":[{"delta":{"content":"Hi"}}]}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n") + "\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	_, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "test-model")
	require.Nil(t, errResp)

	out := w.Body.String()
	events := helperParseSSEDataLines(out)

	// Every parsed event must have a "type" field
	for i, ev := range events {
		_, ok := ev["type"]
		assert.True(t, ok, "event %d missing 'type' field: %v", i, ev)
	}

	// Claude format does NOT output [DONE]
	doneCount := strings.Count(out, "data: [DONE]")
	assert.Equal(t, 0, doneCount, "Claude format should NOT contain [DONE]")
	// The stream should end with message_stop
	assert.Contains(t, out, "\"type\":\"message_stop\"", "stream should end with message_stop")
}

// TestConvertOpenAIStreamToClaudeSSE_UpstreamDropsNoDONE verifies behavior when
// upstream stream ends without sending [DONE].
func TestConvertOpenAIStreamToClaudeSSE_UpstreamDropsNoDONE(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	// Upstream sends content but never sends [DONE]
	chunks := []string{
		`data: {"choices":[{"delta":{"content":"partial"}}]}`,
		`data: {"choices":[{"delta":{"content":" response"}}]}`,
	}
	body := strings.Join(chunks, "\n") + "\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	usage, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "test-model")
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	out := w.Body.String()
	// When upstream drops without [DONE], no message_stop is emitted
	assert.NotContains(t, out, "\"type\":\"message_stop\"")
	// Content should still be present
	assert.Contains(t, out, "partial")
	assert.Contains(t, out, " response")
	// message_start is always emitted
	assert.Contains(t, out, "\"type\":\"message_start\"")
}

// TestConvertOpenAIStreamToClaudeSSE_ThinkingContent verifies that upstream reasoning/thinking
// deltas produce content_block_start with thinking type and thinking_delta events.
func TestConvertOpenAIStreamToClaudeSSE_ThinkingContent(t *testing.T) {
	t.Parallel()

	// Test each of the three reasoning field variants
	reasoningFields := []struct {
		name  string
		field string
	}{
		{"thinking", "thinking"},
		{"reasoning_content", "reasoning_content"},
		{"reasoning", "reasoning"},
	}

	for _, rf := range reasoningFields {
		t.Run(rf.name, func(t *testing.T) {
			t.Parallel()
			c, w := newGinTestContext()

			chunks := []string{
				`data: {"choices":[{"delta":{"` + rf.field + `":"Let me think..."}}]}`,
				`data: {"choices":[{"delta":{"` + rf.field + `":"Step 1: analyze"}}]}`,
				`data: [DONE]`,
			}
			body := strings.Join(chunks, "\n") + "\n"
			resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

			_, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "test-model")
			require.Nil(t, errResp)

			out := w.Body.String()
			events := helperParseSSEDataLines(out)

			// Find content_block_start with thinking type
			foundThinkingStart := false
			foundThinkingDelta := false
			for _, ev := range events {
				if ev["type"] == "content_block_start" {
					if cb, ok := ev["content_block"].(map[string]any); ok {
						if cb["type"] == "thinking" {
							foundThinkingStart = true
						}
					}
				}
				if ev["type"] == "content_block_delta" {
					if d, ok := ev["delta"].(map[string]any); ok {
						if d["type"] == "thinking_delta" {
							foundThinkingDelta = true
						}
					}
				}
			}
			assert.True(t, foundThinkingStart, "expected thinking content_block_start")
			assert.True(t, foundThinkingDelta, "expected thinking_delta event")
		})
	}
}

// TestConvertOpenAIStreamToClaudeSSE_SignatureDelta verifies that signature deltas
// produce signature_delta events attached to the thinking block.
func TestConvertOpenAIStreamToClaudeSSE_SignatureDelta(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	chunks := []string{
		`data: {"choices":[{"delta":{"thinking":"deep thought"}}]}`,
		`data: {"choices":[{"delta":{"signature":"sig_abc123"}}]}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n") + "\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	_, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "test-model")
	require.Nil(t, errResp)

	out := w.Body.String()
	events := helperParseSSEDataLines(out)

	foundSignatureDelta := false
	for _, ev := range events {
		if ev["type"] == "content_block_delta" {
			if d, ok := ev["delta"].(map[string]any); ok {
				if d["type"] == "signature_delta" {
					assert.Equal(t, "sig_abc123", d["signature"])
					foundSignatureDelta = true
				}
			}
		}
	}
	assert.True(t, foundSignatureDelta, "expected signature_delta event")
}

// TestConvertOpenAIStreamToClaudeSSE_SignatureDeltaWithoutPriorThinking verifies that
// a signature delta creates a thinking block even when no thinking content preceded it.
func TestConvertOpenAIStreamToClaudeSSE_SignatureDeltaWithoutPriorThinking(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	chunks := []string{
		`data: {"choices":[{"delta":{"signature":"sig_xyz"}}]}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n") + "\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	_, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "test-model")
	require.Nil(t, errResp)

	out := w.Body.String()
	events := helperParseSSEDataLines(out)

	// Should have created a thinking block for the signature
	foundThinkingStart := false
	foundSignature := false
	for _, ev := range events {
		if ev["type"] == "content_block_start" {
			if cb, ok := ev["content_block"].(map[string]any); ok {
				if cb["type"] == "thinking" {
					foundThinkingStart = true
				}
			}
		}
		if ev["type"] == "content_block_delta" {
			if d, ok := ev["delta"].(map[string]any); ok {
				if d["type"] == "signature_delta" {
					foundSignature = true
				}
			}
		}
	}
	assert.True(t, foundThinkingStart, "signature should trigger thinking block creation")
	assert.True(t, foundSignature, "expected signature_delta")
}

// TestConvertOpenAIStreamToClaudeSSE_MixedThinkingAndText verifies correct block indices
// when both thinking and text content are present.
func TestConvertOpenAIStreamToClaudeSSE_MixedThinkingAndText(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	chunks := []string{
		`data: {"choices":[{"delta":{"thinking":"analyzing..."}}]}`,
		`data: {"choices":[{"delta":{"content":"The answer is 42"}}]}`,
		`data: {"choices":[{"delta":{"thinking":"more thought"}}]}`,
		`data: {"choices":[{"delta":{"content":" actually."}}]}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n") + "\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	_, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "test-model")
	require.Nil(t, errResp)

	out := w.Body.String()
	events := helperParseSSEDataLines(out)

	// Thinking block should be index 0, text block should be index 1
	thinkingIdx := -1.0
	textIdx := -1.0
	for _, ev := range events {
		if ev["type"] == "content_block_start" {
			if cb, ok := ev["content_block"].(map[string]any); ok {
				if cb["type"] == "thinking" {
					thinkingIdx = ev["index"].(float64)
				}
				if cb["type"] == "text" {
					textIdx = ev["index"].(float64)
				}
			}
		}
	}
	assert.Equal(t, 0.0, thinkingIdx, "thinking block should be index 0")
	assert.Equal(t, 1.0, textIdx, "text block should be index 1")

	// Verify content_block_stop for both
	stopIndices := map[float64]bool{}
	for _, ev := range events {
		if ev["type"] == "content_block_stop" {
			stopIndices[ev["index"].(float64)] = true
		}
	}
	assert.True(t, stopIndices[0.0], "thinking block should have stop event")
	assert.True(t, stopIndices[1.0], "text block should have stop event")

	// Verify thinking deltas reference correct index
	for _, ev := range events {
		if ev["type"] == "content_block_delta" {
			if d, ok := ev["delta"].(map[string]any); ok {
				if d["type"] == "thinking_delta" {
					assert.Equal(t, 0.0, ev["index"].(float64), "thinking delta should reference index 0")
				}
				if d["type"] == "text_delta" {
					assert.Equal(t, 1.0, ev["index"].(float64), "text delta should reference index 1")
				}
			}
		}
	}
}

// TestConvertOpenAIStreamToClaudeSSE_MultipleToolCalls verifies that multiple tool_calls
// produce separate tool_use content blocks with correct indices.
func TestConvertOpenAIStreamToClaudeSSE_MultipleToolCalls(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	chunks := []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"id":"call_a","type":"function","function":{"name":"get_weather","arguments":"{\"city\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"id":"call_a","type":"function","function":{"arguments":"\"NYC\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"id":"call_b","type":"function","function":{"name":"get_time","arguments":"{\"tz\":\"UTC\"}"}}]}}]}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n") + "\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	_, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "test-model")
	require.Nil(t, errResp)

	out := w.Body.String()
	events := helperParseSSEDataLines(out)

	// Collect tool_use content_block_start events
	toolBlocks := map[string]float64{} // id -> index
	for _, ev := range events {
		if ev["type"] == "content_block_start" {
			if cb, ok := ev["content_block"].(map[string]any); ok {
				if cb["type"] == "tool_use" {
					id := cb["id"].(string)
					toolBlocks[id] = ev["index"].(float64)
				}
			}
		}
	}
	assert.Contains(t, toolBlocks, "call_a")
	assert.Contains(t, toolBlocks, "call_b")
	assert.NotEqual(t, toolBlocks["call_a"], toolBlocks["call_b"], "tool blocks should have different indices")

	// Verify input_json_delta events
	jsonDeltas := 0
	for _, ev := range events {
		if ev["type"] == "content_block_delta" {
			if d, ok := ev["delta"].(map[string]any); ok {
				if d["type"] == "input_json_delta" {
					jsonDeltas++
					assert.NotEmpty(t, d["partial_json"])
				}
			}
		}
	}
	assert.Equal(t, 3, jsonDeltas, "expected 3 input_json_delta events (2 for call_a, 1 for call_b)")

	// Verify tool name is set on content_block_start
	assert.Contains(t, out, `"name":"get_weather"`)
	assert.Contains(t, out, `"name":"get_time"`)
}

// TestConvertOpenAIStreamToClaudeSSE_UsageWithCacheInfo verifies that usage with
// cache read/write tokens is properly mapped to Claude format.
func TestConvertOpenAIStreamToClaudeSSE_UsageWithCacheInfo(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	chunks := []string{
		`data: {"choices":[{"delta":{"content":"ok"}}]}`,
		`data: {"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150,"prompt_tokens_details":{"cached_tokens":30},"cache_write_5m_tokens":15,"cache_write_1h_tokens":25}}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n") + "\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	usage, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 10, "test-model")
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	// Verify returned usage object
	assert.Equal(t, 100, usage.PromptTokens)
	assert.Equal(t, 50, usage.CompletionTokens)
	assert.Equal(t, 150, usage.TotalTokens)
	require.NotNil(t, usage.PromptTokensDetails)
	assert.Equal(t, 30, usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 15, usage.CacheWrite5mTokens)
	assert.Equal(t, 25, usage.CacheWrite1hTokens)

	out := w.Body.String()
	// Verify Claude-format SSE output has cache fields in message_delta
	assert.Contains(t, out, `"cache_read_input_tokens":30`)
	assert.Contains(t, out, `"cache_creation_input_tokens":40`) // 15+25
	assert.Contains(t, out, `"ephemeral_5m_input_tokens":15`)
	assert.Contains(t, out, `"ephemeral_1h_input_tokens":25`)
	assert.Contains(t, out, `"input_tokens":100`)
	assert.Contains(t, out, `"output_tokens":50`)
}

// TestConvertOpenAIStreamToClaudeSSE_EmptyStreamOnlyDONE verifies that when upstream
// sends only [DONE] with no content, we get message_start and message_stop but no content blocks.
func TestConvertOpenAIStreamToClaudeSSE_EmptyStreamOnlyDONE(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	body := "data: [DONE]\n\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	usage, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "test-model")
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	out := w.Body.String()
	events := helperParseSSEDataLines(out)

	// Should have message_start and message_stop
	types := make([]string, 0)
	for _, ev := range events {
		if t, ok := ev["type"].(string); ok {
			types = append(types, t)
		}
	}
	assert.Contains(t, types, "message_start")
	assert.Contains(t, types, "message_stop")

	// Should NOT have any content_block_start or content_block_delta
	for _, ev := range events {
		assert.NotEqual(t, "content_block_start", ev["type"], "empty stream should have no content blocks")
		assert.NotEqual(t, "content_block_delta", ev["type"], "empty stream should have no content deltas")
	}

	// Usage should be computed with zero completion tokens
	assert.Equal(t, 5, usage.PromptTokens)
	assert.Equal(t, 0, usage.CompletionTokens)
}

// TestConvertOpenAIStreamToClaudeSSE_ResponseAPIParsed verifies that Response API
// format events are properly parsed via parseStreamChunk and converted.
func TestConvertOpenAIStreamToClaudeSSE_ResponseAPIParsed(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	// Response API "response.output_text.delta" event
	chunks := []string{
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"Hello from "}`,
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"Response API"}`,
		`data: {"type":"response.completed","response":{"id":"resp_test","object":"response","model":"gpt-4o","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello from Response API"}]}],"usage":{"input_tokens":10,"output_tokens":8,"total_tokens":18}}}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n") + "\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	usage, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 10, "gpt-4o")
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	assert.Equal(t, 10, usage.PromptTokens)
	assert.Equal(t, 8, usage.CompletionTokens)

	out := w.Body.String()
	assert.Contains(t, out, "\"type\":\"message_start\"")
	assert.Contains(t, out, "\"type\":\"message_stop\"")
	// The response.completed event with output text should produce text content
	assert.Contains(t, out, "Hello from Response API")
}

// TestConvertOpenAIStreamToClaudeSSE_ToolCallWithoutID verifies that tool calls
// without an explicit ID get an auto-generated one.
func TestConvertOpenAIStreamToClaudeSSE_ToolCallWithoutID(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	chunks := []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"type":"function","function":{"name":"my_func","arguments":"{\"key\":\"val\"}"}}]}}]}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n") + "\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	_, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "test-model")
	require.Nil(t, errResp)

	out := w.Body.String()
	// Should have auto-generated tool ID
	assert.Contains(t, out, `"type":"tool_use"`)
	assert.Contains(t, out, `"name":"my_func"`)
	assert.Contains(t, out, `"type":"input_json_delta"`)
}

// TestConvertOpenAIStreamToClaudeSSE_MessageStartContainsModel verifies that the
// message_start event includes the correct model name.
func TestConvertOpenAIStreamToClaudeSSE_MessageStartContainsModel(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	body := "data: [DONE]\n\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	_, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "claude-3-opus-20240229")
	require.Nil(t, errResp)

	out := w.Body.String()
	events := helperParseSSEDataLines(out)

	for _, ev := range events {
		if ev["type"] == "message_start" {
			msg, ok := ev["message"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "claude-3-opus-20240229", msg["model"])
			assert.Equal(t, "assistant", msg["role"])
			assert.Equal(t, "message", msg["type"])
		}
	}
}

// TestConvertOpenAIStreamToClaudeSSE_NonDataLinesIgnored verifies that non-data lines
// (comments, event lines, empty lines) in the upstream SSE are ignored.
func TestConvertOpenAIStreamToClaudeSSE_NonDataLinesIgnored(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	chunks := []string{
		`: this is a comment`,
		`event: message`,
		``,
		`data: {"choices":[{"delta":{"content":"works"}}]}`,
		``,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n") + "\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	_, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "test-model")
	require.Nil(t, errResp)

	out := w.Body.String()
	assert.Contains(t, out, "works")
	assert.Contains(t, out, "\"type\":\"text_delta\"")
}

// TestConvertOpenAIStreamToClaudeSSE_UsageZeroTotal verifies that when usage has
// zero total_tokens, it gets computed as prompt + completion.
func TestConvertOpenAIStreamToClaudeSSE_UsageZeroTotal(t *testing.T) {
	t.Parallel()
	c, _ := newGinTestContext()

	chunks := []string{
		`data: {"choices":[{"delta":{"content":"x"}}]}`,
		`data: {"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":0}}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n") + "\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	usage, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 10, "test-model")
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	assert.Equal(t, 15, usage.TotalTokens, "total should be computed as prompt + completion when zero")
}

// TestConvertOpenAIStreamToClaudeSSE_InvalidJSONSkipped verifies that malformed JSON
// payloads in the stream are silently skipped.
func TestConvertOpenAIStreamToClaudeSSE_InvalidJSONSkipped(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	chunks := []string{
		`data: {invalid json}`,
		`data: {"choices":[{"delta":{"content":"after invalid"}}]}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n") + "\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	_, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "test-model")
	require.Nil(t, errResp)

	out := w.Body.String()
	assert.Contains(t, out, "after invalid")
}

// TestConvertOpenAIStreamToClaudeSSE_ToolCallNoFunction verifies that a tool_call
// delta without a function field (nil function) still produces a tool_use block.
func TestConvertOpenAIStreamToClaudeSSE_ToolCallNoFunction(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	chunks := []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"id":"call_nofn","type":"function"}]}}]}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n") + "\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	_, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "test-model")
	require.Nil(t, errResp)

	out := w.Body.String()
	// Should have a tool_use block with empty name
	assert.Contains(t, out, `"type":"tool_use"`)
	assert.Contains(t, out, `"id":"call_nofn"`)
}

// ---------------------------------------------------------------------------
// Gap-fill: Claude ← Response API conversion path tests
// ---------------------------------------------------------------------------

// TestConvertOpenAIStreamToClaudeSSE_ResponseAPIReasoningToThinking verifies
// that Response API reasoning_summary_text.delta events are converted to
// Claude "thinking" content blocks.
func TestConvertOpenAIStreamToClaudeSSE_ResponseAPIReasoningToThinking(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	chunks := []string{
		// Response API reasoning events
		`data: {"type":"response.reasoning_summary_text.delta","item_id":"rs_1","output_index":0,"delta":"Let me think","summary_index":0}`,
		`data: {"type":"response.reasoning_summary_text.delta","item_id":"rs_1","output_index":0,"delta":" about this","summary_index":0}`,
		// Response API text events
		`data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":1,"content_index":0,"delta":"The answer is 42"}`,
		`data: {"type":"response.completed","response":{"id":"resp_r","object":"response","model":"o1","status":"completed","usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n\n") + "\n\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	usage, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 10, "o1")
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	out := w.Body.String()
	// Reasoning should produce thinking content blocks
	assert.Contains(t, out, `"type":"thinking"`, "should have thinking content_block_start")
	assert.Contains(t, out, `"type":"thinking_delta"`, "should have thinking_delta events")
	assert.Contains(t, out, "Let me think")
	assert.Contains(t, out, " about this")
	// Text should also be present
	assert.Contains(t, out, "The answer is 42")
	assert.Contains(t, out, `"type":"text_delta"`)
	// Should have message_stop (upstream sent [DONE])
	assert.Contains(t, out, "event: message_stop")
}

// TestConvertOpenAIStreamToClaudeSSE_ResponseAPIUpstreamDrop verifies that
// when Response API upstream drops without [DONE], no message_stop is emitted
// in the Claude output.
func TestConvertOpenAIStreamToClaudeSSE_ResponseAPIUpstreamDrop(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	// Response API events without [DONE] at end
	chunks := []string{
		`data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"partial"}`,
	}
	body := strings.Join(chunks, "\n\n") + "\n\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	_, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "gpt-4o")
	require.Nil(t, errResp)

	out := w.Body.String()
	assert.Contains(t, out, "partial", "content should still be present")
	assert.NotContains(t, out, "message_stop", "no message_stop when upstream drops")
}

// TestConvertOpenAIStreamToClaudeSSE_ResponseAPIKeepalive verifies that
// Response API keepalive events are silently ignored (not converted to any
// Claude event).
func TestConvertOpenAIStreamToClaudeSSE_ResponseAPIKeepalive(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	chunks := []string{
		`data: {"type":"keepalive","sequence_number":1}`,
		`data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"hello"}`,
		`data: [DONE]`,
	}
	body := strings.Join(chunks, "\n\n") + "\n\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	_, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 5, "gpt-4o")
	require.Nil(t, errResp)

	out := w.Body.String()
	assert.Contains(t, out, "hello")
	assert.NotContains(t, out, "keepalive", "keepalive should not appear in Claude output")
	assert.Contains(t, out, "event: message_stop")
}

// TestConvertOpenAIStreamToClaudeSSE_ResponseAPIEmptyStream verifies that
// a Response API stream with only [DONE] produces message_start + message_stop.
func TestConvertOpenAIStreamToClaudeSSE_ResponseAPIEmptyStream(t *testing.T) {
	t.Parallel()
	c, w := newGinTestContext()

	body := "data: [DONE]\n\n"
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

	_, errResp := ConvertOpenAIStreamToClaudeSSE(c, resp, 0, "gpt-4o")
	require.Nil(t, errResp)

	out := w.Body.String()
	assert.Contains(t, out, "event: message_start")
	assert.Contains(t, out, "event: message_stop")
	// No content blocks should be started
	assert.NotContains(t, out, "content_block_start")
}
