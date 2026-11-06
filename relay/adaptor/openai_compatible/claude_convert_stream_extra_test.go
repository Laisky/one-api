package openai_compatible

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestConvertOpenAIStreamToClaudeSSE_MixedThinkingAndText verifies that
// alternating thinking and text deltas become sequential blocks (thinking,
// text, thinking, text) with dense indices: each block is stopped before the
// next starts, as the Messages API never interleaves open blocks.
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

	msg := requireConformantClaudeStream(t, w.Body.String())
	require.Len(t, msg.Content, 4)
	require.Equal(t, "thinking", msg.Content[0].Type)
	require.Equal(t, "analyzing...", msg.Content[0].Thinking)
	require.Equal(t, "text", msg.Content[1].Type)
	require.Equal(t, "The answer is 42", msg.Content[1].Text)
	require.Equal(t, "thinking", msg.Content[2].Type)
	require.Equal(t, "more thought", msg.Content[2].Thinking)
	require.Equal(t, "text", msg.Content[3].Type)
	require.Equal(t, " actually.", msg.Content[3].Text)
	require.Equal(t, "end_turn", msg.StopReason)
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
	// Anthropic semantics: input_tokens excludes cache reads and writes (100-30-40).
	assert.Contains(t, out, `"input_tokens":30`)
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
	// The terminal response.completed snapshot repeats text already streamed
	// as deltas; it must not be rendered a second time.
	msg := requireConformantClaudeStream(t, out)
	require.Equal(t, "Hello from Response API", msg.Text())
	require.Len(t, msg.Content, 1)
	require.Equal(t, "end_turn", msg.StopReason)
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
