package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestGemini4ClaudeNativeFormatForcedToolTransport preserves both explicit constraints through real Messages ingress and compatible HTTP dispatch.
func TestGemini4ClaudeNativeFormatForcedToolTransport(t *testing.T) {
	const balance int64 = 1_000_000
	xaiVideoSetup(t, balance, false)
	var calls atomic.Int32
	observed := make(chan struct {
		body []byte
		path string
		err  error
	}, 1)
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) > 1 {
			http.Error(w, "unexpected local provider retry", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		observed <- struct {
			body []byte
			path string
			err  error
		}{body: body, path: r.URL.Path, err: err}
		if err != nil {
			http.Error(w, "invalid local provider body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"id":"local-format-tool","object":"chat.completion","model":"gemini-4-hydrogen","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_topic","type":"function","function":{"name":"topic_classifier","arguments":"{\"topic\":\"analysis\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`); err != nil {
			return
		}
	}))
	t.Cleanup(provider.Close)
	previousClient := client.HTTPClient
	client.HTTPClient = provider.Client()
	t.Cleanup(func() { client.HTTPClient = previousClient })

	const nativeConfig = `{"effort":"high","format":{"type":"json_schema","name":"native_output","schema":{"type":"object","properties":{"count":{"type":"integer","minimum":1,"maximum":7}},"required":["count"],"additionalProperties":false}}}`
	body := `{"model":"alias","max_tokens":128,"messages":[{"role":"user","content":"Return structured topic and confidence JSON."}],"tools":[{"name":"topic_classifier","description":"Return structured JSON data","input_schema":{"type":"object","properties":{"topic":{"type":"string"}},"required":["topic"],"additionalProperties":false}}],"tool_choice":{"type":"tool","name":"topic_classifier"},"output_config":` + nativeConfig + `}`
	c, recorder, requestID := protocolContext(t, channeltype.GeminiOpenAICompatible, "gemini-4-hydrogen", "/v1/messages", body, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	responseNamespaceCleanLedger(t, requestID)
	apiErr := RelayClaudeMessagesHelper(c)
	drainCriticalTasks(t)
	require.Nil(t, apiErr)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.EqualValues(t, 1, calls.Load(), "one incoming Messages request must dispatch once")
	got := <-observed
	require.NoError(t, got.err)
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(got.body, &payload))
	require.JSONEq(t, `"gemini-4-hydrogen"`, string(payload["model"]))
	require.JSONEq(t, `"high"`, string(payload["reasoning_effort"]))
	require.NotContains(t, payload, "output_config")
	require.JSONEq(t, `{"type":"function","function":{"name":"topic_classifier"}}`, string(payload["tool_choice"]))
	var tools []relaymodel.Tool
	require.NoError(t, json.Unmarshal(payload["tools"], &tools))
	require.Len(t, tools, 1)
	require.Equal(t, "function", tools[0].Type)
	require.NotNil(t, tools[0].Function)
	require.Equal(t, "topic_classifier", tools[0].Function.Name)
	require.Equal(t, "Return structured JSON data", tools[0].Function.Description)
	var format struct {
		Type   string `json:"type"`
		Schema struct {
			Name   string          `json:"name"`
			Strict *bool           `json:"strict"`
			Schema json.RawMessage `json:"schema"`
		} `json:"json_schema"`
	}
	require.NoError(t, json.Unmarshal(payload["response_format"], &format))
	require.Equal(t, "json_schema", format.Type)
	require.Equal(t, "native_output", format.Schema.Name)
	require.NotNil(t, format.Schema.Strict)
	require.True(t, *format.Schema.Strict)
	require.JSONEq(t, `{"type":"object","properties":{"count":{"type":"integer","minimum":1,"maximum":7}},"required":["count"],"additionalProperties":false}`, string(format.Schema.Schema))
	original, exists := c.Get(ctxkey.OriginalClaudeRequest)
	require.True(t, exists)
	source, ok := original.(*relaymodel.ClaudeRequest)
	require.True(t, ok)
	require.Equal(t, nativeConfig, string(source.OutputConfig), "conversion must retain the native source controls")
	require.Len(t, source.Tools, 1)
	require.Equal(t, map[string]any{"type": "tool", "name": "topic_classifier"}, source.ToolChoice)
	raw, err := common.GetRequestBody(c)
	require.NoError(t, err)
	require.JSONEq(t, body, string(raw), "the incoming raw payload must remain unchanged")
	var downstream struct {
		Content []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &downstream))
	require.Len(t, downstream.Content, 1)
	require.Equal(t, "tool_use", downstream.Content[0].Type)
	require.Equal(t, "topic_classifier", downstream.Content[0].Name)
	require.EqualValues(t, 18, requestCostQuota(t, requestID))
	require.Equal(t, balance-18, reloadUserQuota(t))
	require.Equal(t, "/chat/completions", got.path)
}
