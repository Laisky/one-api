package mistral

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// TestReasoningRequestCompatibility verifies Chat and Responses-style controls,
// exact model IDs, tool payloads, replayed thinking, and input non-mutation.
func TestReasoningRequestCompatibility(t *testing.T) {
	t.Parallel()
	for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh"} {
		for _, source := range []string{"chat", "responses"} {
			t.Run(source+"/"+effort, func(t *testing.T) {
				t.Parallel()
				const input = `{"model":"mistral-medium-3-5","seed":42,"max_completion_tokens":777,"messages":[{"role":"assistant","content":"Answer","reasoning_content":"Prior analysis","tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{}"}}]}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],"tool_choice":"auto"}`
				var req model.GeneralOpenAIRequest
				require.NoError(t, json.Unmarshal([]byte(input), &req))
				if source == "chat" {
					req.ReasoningEffort = &effort
				} else {
					req.Reasoning = &model.OpenAIResponseReasoning{Effort: &effort}
				}
				before, err := json.Marshal(req)
				require.NoError(t, err)
				converted, err := (&Adaptor{}).ConvertRequest(nil, 0, &req)
				require.NoError(t, err)
				encoded, err := json.Marshal(converted)
				require.NoError(t, err)
				var got map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(encoded, &got))
				require.JSONEq(t, `"`+effort+`"`, string(got["reasoning_effort"]))
				require.NotContains(t, got, "reasoning")
				require.NotContains(t, got, "seed")
				require.NotContains(t, got, "max_completion_tokens")
				require.JSONEq(t, `42`, string(got["random_seed"]))
				require.JSONEq(t, `777`, string(got["max_tokens"]))
				require.Contains(t, string(got["messages"]), `"thinking"`)
				require.Contains(t, string(got["messages"]), `Prior analysis`)
				require.Contains(t, string(got["messages"]), `call-1`)
				require.Contains(t, string(got["tools"]), `lookup`)
				after, err := json.Marshal(req)
				require.NoError(t, err)
				require.JSONEq(t, string(before), string(after))
			})
		}
	}
	_, err := (&Adaptor{}).ConvertRequest(nil, 0, nil)
	require.Error(t, err)
	low, high := "none", "high"
	req := &model.GeneralOpenAIRequest{ReasoningEffort: &low, Reasoning: &model.OpenAIResponseReasoning{Effort: &high}}
	converted, err := reasoningRequest(req)
	require.NoError(t, err)
	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"reasoning_effort":"none"`)
}

// TestThinkingBodyJSONAndSSE exercises fragmented reads, multiline event data,
// preserved tool/usage fields, EOF termination, and idempotent upstream closure.
func TestThinkingBodyJSONAndSSE(t *testing.T) {
	t.Parallel()
	const payload = `{"id":"reply","choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"thinking","thinking":[{"type":"text","text":"think"}]},{"type":"text","text":"answer"}],"tool_calls":[{"id":"call1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12}}`
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "sse"}[stream], func(t *testing.T) {
			t.Parallel()
			input := payload
			if stream {
				input = ": heartbeat\r\nevent: message\r\nid: 9\r\ndata: " + strings.Replace(payload, `,"choices"`, ",\ndata: \"choices\"", 1) + "\r\n\r\ndata: [DONE]"
			}
			upstream := &countedBody{Reader: &fragmentReader{data: []byte(input)}}
			body := newThinkingBody(upstream, stream)
			empty := make([]byte, 0)
			n, err := body.Read(empty)
			require.Zero(t, n)
			require.NoError(t, err)
			var out bytes.Buffer
			_, err = io.CopyBuffer(&out, body, make([]byte, 7))
			require.NoError(t, err)
			require.Contains(t, out.String(), `"reasoning_content":"think"`)
			require.Contains(t, out.String(), `"content":"answer"`)
			require.Contains(t, out.String(), `"tool_calls"`)
			require.Contains(t, out.String(), `"total_tokens":12`)
			if stream {
				require.Contains(t, out.String(), "id: 9")
				require.Contains(t, out.String(), "data: [DONE]")
			}
			require.NoError(t, body.Close())
			require.NoError(t, body.Close())
			require.Equal(t, 1, upstream.closes)
		})
	}
}

// TestThinkingBodyFailures verifies bounded memory, explicit unknown-chunk
// errors, malformed input, and preservation of ordinary provider error bodies.
func TestThinkingBodyFailures(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{`, `{"choices":[{"delta":{"content":[{"type":"unknown","text":"must not disappear"}]}}]}`, strings.Repeat("x", maxThinkingFrame+1)} {
		upstream := &countedBody{Reader: strings.NewReader(raw)}
		_, err := io.ReadAll(newThinkingBody(upstream, false))
		require.Error(t, err)
		require.Equal(t, 1, upstream.closes)
	}
	const errorBody = `{"error":{"message":"invalid effort","type":"invalid_request_error"}}`
	got, err := normalizeThinking([]byte(errorBody))
	require.NoError(t, err)
	require.JSONEq(t, errorBody, string(got))
}

// TestReasoningResponseIntegration goes through the real adaptor and shared
// response handlers, preserving provider usage without live model calls.
func TestReasoningResponseIntegration(t *testing.T) {
	t.Parallel()
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "sse"}[stream], func(t *testing.T) {
			t.Parallel()
			payload := `{"id":"reply","object":"chat.completion","model":"mistral-small-latest","choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"thinking","thinking":[{"type":"text","text":"think"}]},{"type":"text","text":"answer"}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12}}`
			if stream {
				payload = "data: " + strings.ReplaceAll(payload, `"message":`, `"delta":`) + "\n\ndata: [DONE]\n\n"
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(payload))}
			usage, apiErr := (&Adaptor{}).DoResponse(c, resp, &meta.Meta{ActualModelName: "mistral-small-latest", IsStream: stream})
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			require.Equal(t, 8, usage.PromptTokens)
			require.Equal(t, 4, usage.CompletionTokens)
			require.Contains(t, recorder.Body.String(), "answer")
			require.Contains(t, recorder.Body.String(), "think")
		})
	}
}

// countedBody records transport ownership while delegating reads.
type countedBody struct {
	io.Reader
	closes int
}

// Close records exactly how often the adapter closes its upstream body.
func (b *countedBody) Close() error { b.closes++; return nil }

// fragmentReader simulates arbitrary TCP fragmentation without network calls.
type fragmentReader struct{ data []byte }

// Read returns at most three bytes so parsers cannot assume one-read frames.
func (r *fragmentReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data[:min(3, len(r.data))])
	r.data = r.data[n:]
	return n, nil
}
