package maas

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
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestMaaSToolSchemaAndHistoryRoundTrip checks the actual HTTP wire, not just metadata.
func TestMaaSToolSchemaAndHistoryRoundTrip(t *testing.T) {
	for _, name := range ModelList {
		t.Run(name, func(t *testing.T) {
			raw := `{"model":"configured-alias","messages":[{"role":"user","content":[{"type":"text","text":"note"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]},{"role":"assistant","tool_calls":[{"id":"b","type":"function","function":{"name":"save","arguments":"{\"kind\":\"note\"}"}},{"id":"a","type":"function","function":{"name":"save","arguments":"{\"kind\":\"memo\"}"}}]},{"role":"tool","tool_call_id":"a","content":"first result"},{"role":"tool","tool_call_id":"b","content":"second result"}],"max_completion_tokens":128,"tools":[{"type":"function","function":{"name":"save","parameters":{"type":"object","properties":{"kind":{"const":"note","anyOf":[{"type":"string"},{"type":"null"}]}}}}}],"tool_choice":"required","response_format":{"type":"json_schema","json_schema":{"name":"result","schema":{"anyOf":[{"const":"note"},{"enum":["memo"]}]}}},"extra_body":{"chat_template_kwargs":{"enable_thinking":false}}}`
			var req model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal([]byte(raw), &req))
			before, err := json.Marshal(req)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			meta.Set2Context(c, &meta.Meta{ActualModelName: name})
			converted, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, &req)
			require.NoError(t, err)
			wire, err := json.Marshal(converted)
			require.NoError(t, err)
			var sent map[string]any
			require.NoError(t, json.Unmarshal(wire, &sent))
			require.Equal(t, name, sent["model"])
			require.Equal(t, float64(128), sent["max_tokens"])
			require.NotContains(t, sent, "max_completion_tokens")
			var original map[string]any
			require.NoError(t, json.Unmarshal(before, &original))
			for _, key := range []string{"messages", "tools", "tool_choice", "response_format", "extra_body"} {
				require.Equal(t, original[key], sent[key], key)
			}
			after, err := json.Marshal(req)
			require.NoError(t, err)
			require.Equal(t, string(before), string(after))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, e := io.ReadAll(r.Body)
				if e != nil {
					t.Error(e)
				}
				require.JSONEq(t, string(wire), string(body))
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"id":"offline","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"plan","tool_calls":[{"id":"result","type":"function","function":{"name":"save","arguments":"{\"kind\":\"note\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,"prompt_tokens_details":{"cached_tokens":40}}}`)
			}))
			defer server.Close()
			resp, err := server.Client().Post(server.URL, "application/json", bytes.NewReader(wire))
			require.NoError(t, err)
			usage, upErr := (&Adaptor{}).DoResponse(c, resp, &meta.Meta{ActualModelName: name, PromptTokens: 100})
			require.Nil(t, upErr)
			require.NotNil(t, usage)
			require.Equal(t, 100, usage.PromptTokens)
			require.Equal(t, 10, usage.CompletionTokens)
			require.Equal(t, 40, usage.PromptTokensDetails.CachedTokens)
			require.Contains(t, w.Body.String(), `"finish_reason":"tool_calls"`)
			require.Contains(t, w.Body.String(), `"reasoning":"plan"`)
			require.Contains(t, w.Body.String(), `"id":"result"`)
		})
	}
}

// TestMaaSStreamToolUsage checks split arguments, completion state and trailing usage.
func TestMaaSStreamToolUsage(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	sse := strings.Join([]string{
		`data: {"id":"offline","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"plan","tool_calls":[{"index":0,"id":"call-a","type":"function","function":{"name":"save","arguments":"{\"kind\":"}}]}}]}`,
		`data: {"id":"offline","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"note\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: {"id":"offline","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":12,"total_tokens":112,"prompt_tokens_details":{"cached_tokens":40}}}`,
		`data: [DONE]`, ""}, "\n\n")
	for _, name := range ModelList {
		t.Run(name, func(t *testing.T) {
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(sse))}
			usage, upErr := (&Adaptor{}).DoResponse(c, resp, &meta.Meta{ActualModelName: name, IsStream: true, PromptTokens: 100})
			require.Nil(t, upErr)
			require.NotNil(t, usage)
			require.Equal(t, 12, usage.CompletionTokens)
			require.Equal(t, 40, usage.PromptTokensDetails.CachedTokens)
		})
	}
	text := w.Body.String()
	require.Contains(t, text, "call-a")
	require.Contains(t, text, "tool_calls")
	require.Contains(t, text, "reasoning_content")
	require.Contains(t, text, "[DONE]")
}

// TestMaaSUnsupportedInputControls guards against provider mixing and nil panics.
func TestMaaSUnsupportedInputControls(t *testing.T) {
	a := &Adaptor{}
	_, err := a.ConvertRequest(nil, relaymode.ChatCompletions, nil)
	require.Error(t, err)
	for _, req := range []*model.GeneralOpenAIRequest{
		{Model: "unverified"}, {Model: ModelList[0], Thinking: &model.Thinking{Type: "enabled"}},
	} {
		_, err = a.ConvertRequest(nil, relaymode.ChatCompletions, req)
		require.Error(t, err)
	}
	_, err = a.ConvertRequest(nil, relaymode.Embeddings, &model.GeneralOpenAIRequest{Model: ModelList[0]})
	require.Error(t, err)
	req := &model.GeneralOpenAIRequest{Model: ModelList[0], MaxTokens: 32, MaxCompletionTokens: model.IntPtr(128)}
	converted, err := a.ConvertRequest(nil, relaymode.ChatCompletions, req)
	require.NoError(t, err)
	require.Equal(t, 32, converted.(*model.GeneralOpenAIRequest).MaxTokens)
}
