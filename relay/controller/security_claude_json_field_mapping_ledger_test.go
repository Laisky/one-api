package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestSecurityClaudeJSONFieldMappingLedger checks delivered raw JSON and durable owner/token/log accounting through local HTTP.
func TestSecurityClaudeJSONFieldMappingLedger(t *testing.T) {
	payload := `{"answer":"` + strings.Repeat(`synthetic \"quoted\" accounting evidence `, 100) + `"}`
	quote := func(s string) string { b, err := json.Marshal(s); require.NoError(t, err); return string(b) }
	q := quote(payload)
	cases := []struct{ name, fields, expected string }{
		{"delta_json_object_normalization", `"delta":{"json":{"answer":   "synthetic"}}`, `{"answer":"synthetic"}`},
		{"delta_json_exponent_raw_fallback", `"delta":{"json":{"n":1e400, "answer":"synthetic"}}`, `{"json":{"n":1e400, "answer":"synthetic"}}`},
		{"delta_plain_object_raw_fallback", `"delta":{"answer":   "synthetic"}`, `{"answer":   "synthetic"}`},
		{"delta_key_precedence", `"delta":{"json":` + q + `,"partial_json":"ignored","text":"ignored"}`, payload},
		{"output_json_null_object_fallback", `"output":{"json":null}`, `{"json":null}`},
		{"output_array_first_extractable", `"output":[null,{"json":` + q + `},{"text":"ignored"}]`, payload},
		{"output_array_raw_fallback", `"output":[null,1,false]`, `[null,1,false]`},
		{"root_json_null_precedence", `"json":null,"part":{"json":"ignored"},"delta":"ignored"`, "null"},
		{"part_json_quoted_scalar_precedence", `"part":{"json":"synthetic","text":"ignored"},"output":"ignored"`, `"synthetic"`},

		{"delta_json_string", `"delta":{"json":` + q + "}", payload},
		{"delta_partial_json_string", `"delta":{"partial_json":` + q + "}", payload},
		{"delta_text_string", `"delta":{"text":` + q + "}", payload},
		{"delta_string", `"delta":` + q, payload},
		{"root_text", `"text":` + q, payload},
		{"part_text", `"part":{"type":"output_json","text":` + q + "}", payload},
		{"output_string", `"output":` + q, payload},
		{"output_json_string_wrapper", `"output":{"json":` + q + "}", payload},
		{"output_text_string_wrapper", `"output":{"text":` + q + "}", payload},
		{"output_content_array_wrapper", `"output":{"content":[{"text":` + q + "}]}", payload},
		{"output_object_normalization", `"output":{"answer":   "synthetic"}`, `{"answer":"synthetic"}`},
		{"output_exponent_raw_fallback", `"output":{"n":1e400,  "answer":"synthetic"}`, `{"n":1e400,  "answer":"synthetic"}`},
		{"root_json_precedence", `"json":` + payload + `,"part":{"json":null},"delta":{"json":"ignored"}`, payload},
		{"part_json_precedence", `"part":{"json":` + payload + `},"output":"ignored","delta":{"json":"ignored"}`, payload},
		{"part_text_precedence", `"part":{"text":` + q + `},"output":"ignored","text":"ignored"`, payload},
		{"output_precedence", `"output":` + q + `,"text":"ignored","delta":"ignored"`, payload},
		{"root_text_precedence", `"text":` + q + `,"delta":"ignored"`, payload},
		{"protocol_quoted_scalar", `"text":` + quote(`"synthetic\nquoted \"answer\""`), "synthetic\nquoted \"answer\""},
		{"delta_null_empty_control", `"delta":null`, ""},
		{"output_null_raw_fallback", `"output":null`, "null"},
		{"delta_json_null_control", `"delta":{"json":null}`, "null"},
	}
	for _, tc := range cases {
		for _, measured := range []bool{false, true} {
			name := tc.name
			if measured {
				name += "/measured"
			}
			t.Run(name, func(t *testing.T) {
				wire := "data: " + `{"type":"response.output_json.done","output_index":0,` + tc.fields + "}\n\n"
				if measured {
					wire += `data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3}}` + "\n\n"
				}
				const balance int64 = 10_000_000
				xaiVideoSetup(t, balance, false)
				var calls atomic.Int32
				upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, wire+"data: [DONE]\n\n")
				}))
				t.Cleanup(upstream.Close)
				old := client.HTTPClient
				client.HTTPClient = upstream.Client()
				t.Cleanup(func() { client.HTTPClient = old })
				c, w, id := protocolContext(t, channeltype.OpenAI, "gpt-4", "/v1/messages", `{"model":"alias","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hello"}]}`, upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
				require.Nil(t, RelayClaudeMessagesHelper(c))
				drainCriticalTasks(t)
				require.EqualValues(t, 1, calls.Load())
				var delivered strings.Builder
				for _, line := range strings.Split(w.Body.String(), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var e struct {
						Type  string `json:"type"`
						Delta struct {
							Text string `json:"text"`
						} `json:"delta"`
					}
					require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &e))
					if e.Type == "content_block_delta" {
						delivered.WriteString(e.Delta.Text)
					}
				}
				require.Equal(t, tc.expected, delivered.String())
				cost := requestCostQuota(t, id)
				var logs []model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ? AND type IN ?", id, []int{model.LogTypeConsume, model.LogTypeProvisional}).Find(&logs).Error)
				require.Len(t, logs, 1)
				require.Equal(t, balance-cost, reloadUserQuota(t))
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				require.Equal(t, balance-cost, token.RemainQuota)
				require.Equal(t, cost, token.UsedQuota)
				require.EqualValues(t, cost, logs[0].Quota)
				require.Equal(t, model.LogTypeConsume, logs[0].Type)

				expected := openai_compatible.CountTokenText(delivered.String(), "gpt-4")
				if measured {
					expected = 3
					require.EqualValues(t, 10, cost)
				}
				if !measured && expected > 32 {
					require.EqualValues(t, logs[0].PromptTokens+expected, cost)
				}
				require.Equal(t, expected, logs[0].CompletionTokens)
			})
		}
	}
}
