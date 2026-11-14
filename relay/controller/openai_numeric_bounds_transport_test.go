package controller

import (
	"encoding/json"
	"github.com/Laisky/errors/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// openAIStandardBoundsSchema exercises modern numeric bounds without losing decimal or integer precision.
const openAIStandardBoundsSchema = `{"type":"object","properties":{"count":{"type":"integer","minimum":9007199254740993,"maximum":9007199254740997},"minimum":{"type":"string","enum":["minimum","maximum"]},"nested":{"type":"array","items":{"type":"object","properties":{"ratio":{"type":"number","exclusiveMinimum":0.123456789012345678901234567890,"exclusiveMaximum":0.987654321098765432109876543210}},"required":["ratio"],"additionalProperties":false}}},"required":["count","minimum","nested"],"additionalProperties":false}`

// openAINumericBoundsTransport keeps the logical provider identity while sending every test request to loopback.
type openAINumericBoundsTransport struct {
	transport http.RoundTripper
	target    *url.URL
}

// RoundTrip rejects unexpected destinations and redirects the logical OpenAI request to the local fixture.
func (t openAINumericBoundsTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Host != "api.openai.com" {
		return nil, errors.Errorf("unexpected local OpenAI fixture request host %q", request.URL.Host)
	}
	local := request.Clone(request.Context())
	target := *request.URL
	target.Scheme, target.Host = t.target.Scheme, t.target.Host
	local.URL = &target
	return t.transport.RoundTrip(local)
}

// TestOpenAIStandardNumericBoundsTransport verifies Chat-to-Responses lowering and native Responses retain the supported schema.
func TestOpenAIStandardNumericBoundsTransport(t *testing.T) {
	for _, surface := range []string{"chat-response-format", "native-responses"} {
		t.Run(surface, func(t *testing.T) {
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
				response := map[string]any{
					"id": "resp_openai_standard_bounds", "object": "response", "created_at": 1, "status": "completed", "model": "gpt-6.1-sol",
					"output": []any{map[string]any{"id": "msg_bounds", "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": `{"count":9007199254740995,"minimum":"minimum","nested":[{"ratio":0.5}]}`, "annotations": []any{}}}}},
					"usage":  map[string]any{"input_tokens": 3, "output_tokens": 2, "total_tokens": 5},
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					return
				}
			}))
			t.Cleanup(provider.Close)
			previousClient := client.HTTPClient
			localClient := provider.Client()
			target, err := url.Parse(provider.URL)
			require.NoError(t, err)
			localClient.Transport = openAINumericBoundsTransport{transport: localClient.Transport, target: target}
			client.HTTPClient = localClient
			t.Cleanup(func() { client.HTTPClient = previousClient })
			path := "/v1/chat/completions"
			body := `{"model":"alias","max_tokens":32,"messages":[{"role":"user","content":"Extract bounded values."}],"response_format":{"type":"json_schema","json_schema":{"name":"bounded","strict":true,"schema":` + openAIStandardBoundsSchema + `}}}`
			relay := RelayTextHelper
			if surface == "native-responses" {
				path, relay = "/v1/responses", RelayResponseAPIHelper
				body = `{"model":"alias","store":false,"max_output_tokens":32,"input":"Extract bounded values.","text":{"format":{"type":"json_schema","name":"bounded","strict":true,"schema":` + openAIStandardBoundsSchema + `}}}`
			}
			c, recorder, id := protocolContext(t, channeltype.OpenAI, "gpt-6.1-sol", path, body, "https://api.openai.com", balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			responseNamespaceCleanLedger(t, id)
			apiErr := relay(c)
			drainCriticalTasks(t)
			require.Nil(t, apiErr)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.EqualValues(t, 1, calls.Load())
			got := <-observed
			require.NoError(t, got.err)
			require.Equal(t, "/v1/responses", got.path, "the actual OpenAI adaptor must select the native Responses transport")
			source, err := common.GetRequestBody(c)
			require.NoError(t, err)
			require.Equal(t, body, string(source), "raw caller bytes and bound precision remain owned by the source")
			require.EqualValues(t, 5, requestCostQuota(t, id))
			require.Equal(t, balance-5, reloadUserQuota(t))
			var payload struct {
				Model string `json:"model"`
				Text  struct {
					Format struct {
						Type   string          `json:"type"`
						Schema json.RawMessage `json:"schema"`
					} `json:"format"`
				} `json:"text"`
			}
			require.NoError(t, json.Unmarshal(got.body, &payload))
			require.Equal(t, "gpt-6.1-sol", payload.Model, "mapped models must reach the upstream")
			require.Equal(t, "json_schema", payload.Text.Format.Type)
			raw := string(payload.Text.Format.Schema)
			for _, token := range []string{`"minimum":9007199254740993`, `"maximum":9007199254740997`, `"exclusiveMinimum":0.123456789012345678901234567890`, `"exclusiveMaximum":0.987654321098765432109876543210`} {
				require.Contains(t, raw, token, "supported bound tokens must remain exact")
			}
			var schema struct {
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
			}
			require.NoError(t, json.Unmarshal(payload.Text.Format.Schema, &schema))
			require.Contains(t, schema.Properties, "minimum", "property names are caller data, not schema keywords")
			require.Contains(t, schema.Required, "minimum")
			require.JSONEq(t, `{"type":"string","enum":["minimum","maximum"]}`, string(schema.Properties["minimum"]))
		})
	}
}
