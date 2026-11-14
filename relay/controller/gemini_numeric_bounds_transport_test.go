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
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// geminiNumericBoundsSchema exercises nested inclusive bounds, exact large integers, and a property named like a keyword.
const geminiNumericBoundsSchema = `{"type":"object","properties":{"count":{"type":"integer","minimum":9007199254740993,"maximum":9007199254740997},"minimum":{"type":"string","enum":["minimum","maximum"]},"nested":{"type":"object","properties":{"ratio":{"type":"number","minimum":0.1,"maximum":0.9}},"required":["ratio"],"additionalProperties":false}},"required":["count","minimum","nested"],"additionalProperties":false}`

// assertGeminiNumericBounds checks actual serialized schema tokens without rounding them through float64 assertions.
func assertGeminiNumericBounds(t *testing.T, raw json.RawMessage) {
	t.Helper()
	require.Contains(t, string(raw), `"minimum":9007199254740993`, "the exact integer lower bound must survive")
	require.Contains(t, string(raw), `"maximum":9007199254740997`, "the exact integer upper bound must survive")
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	require.NoError(t, json.Unmarshal(raw, &schema))
	require.Contains(t, schema.Properties, "minimum", "a property named minimum is caller data")
	require.JSONEq(t, `{"type":"string","enum":["minimum","maximum"]}`, string(schema.Properties["minimum"]))
	require.Contains(t, schema.Required, "minimum")
	var nested struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(schema.Properties["nested"], &nested))
	require.JSONEq(t, `{"type":"number","minimum":0.1,"maximum":0.9}`, string(nested.Properties["ratio"]))
}

// TestGeminiNumericBoundsIngressTransport preserves documented inclusive bounds through the actual Chat, Responses, and Claude controller paths.
func TestGeminiNumericBoundsIngressTransport(t *testing.T) {
	for _, tc := range []struct{ name, mode, strict string }{
		{"chat-response-format", "chat", `,"strict":true`},
		{"responses-strict-omitted", "responses", ""},
		{"responses-strict-false", "responses", `,"strict":false`},
		{"responses-strict-true", "responses", `,"strict":true`},
		{"claude-tool-history", "claude", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
					http.Error(w, "invalid local request", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, `{"id":"local-bounds","object":"chat.completion","model":"gemini-4-hydrogen","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`); err != nil {
					return
				}
			}))
			t.Cleanup(provider.Close)
			previousClient := client.HTTPClient
			client.HTTPClient = provider.Client()
			t.Cleanup(func() { client.HTTPClient = previousClient })
			format := `{"type":"json_schema","name":"bounded"` + tc.strict + `,"schema":` + geminiNumericBoundsSchema + `}`
			path := "/v1/chat/completions"
			body := `{"model":"alias","max_tokens":32,"messages":[{"role":"user","content":"Extract values."}],"response_format":{"type":"json_schema","json_schema":{"name":"bounded","strict":true,"schema":` + geminiNumericBoundsSchema + `}}}`
			relay := RelayTextHelper
			if tc.mode == "responses" {
				path, relay = "/v1/responses", RelayResponseAPIHelper
				body = `{"model":"alias","store":false,"max_output_tokens":32,"input":"Extract values.","text":{"format":` + format + `}}`
			}
			if tc.mode == "claude" {
				path, relay = "/v1/messages", RelayClaudeMessagesHelper
				body = `{"model":"alias","max_tokens":32,"messages":[{"role":"user","content":"Run the tool."},{"role":"assistant","content":[{"type":"tool_use","id":"earlier_probe","name":"probe","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"earlier_probe","content":"Earlier result"},{"type":"text","text":"Run again."}]}],"tools":[{"name":"probe","description":"Extract structured JSON values","input_schema":` + geminiNumericBoundsSchema + `}],"tool_choice":{"type":"tool","name":"probe"}}`
			}
			c, recorder, requestID := protocolContext(t, channeltype.GeminiOpenAICompatible, "gemini-4-hydrogen", path, body, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			responseNamespaceCleanLedger(t, requestID)
			apiErr := relay(c)
			drainCriticalTasks(t)
			require.Nil(t, apiErr)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.EqualValues(t, 1, calls.Load())
			got := <-observed
			require.NoError(t, got.err)
			require.Equal(t, "/chat/completions", got.path)
			raw, err := common.GetRequestBody(c)
			require.NoError(t, err)
			require.JSONEq(t, body, string(raw), "the raw caller request remains unchanged")
			require.EqualValues(t, 5, requestCostQuota(t, requestID))
			require.Equal(t, balance-5, reloadUserQuota(t))
			var payload map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(got.body, &payload))
			var schema json.RawMessage
			if tc.mode == "claude" {
				var tools []struct {
					Function struct {
						Name       string          `json:"name"`
						Parameters json.RawMessage `json:"parameters"`
					} `json:"function"`
				}
				require.NoError(t, json.Unmarshal(payload["tools"], &tools))
				require.Len(t, tools, 1)
				require.Equal(t, "probe", tools[0].Function.Name)
				schema = tools[0].Function.Parameters
				require.NotContains(t, payload, "response_format", "actual tool history suppresses structured promotion")
				require.JSONEq(t, `{"type":"function","function":{"name":"probe"}}`, string(payload["tool_choice"]))
			} else {
				var format struct {
					Type   string `json:"type"`
					Schema struct {
						Schema json.RawMessage `json:"schema"`
					} `json:"json_schema"`
				}
				require.NoError(t, json.Unmarshal(payload["response_format"], &format))
				require.Equal(t, "json_schema", format.Type)
				schema = format.Schema.Schema
			}
			assertGeminiNumericBounds(t, schema)
		})
	}
}
