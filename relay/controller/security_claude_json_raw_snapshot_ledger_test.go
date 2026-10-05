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

// TestSecurityClaudeJSONRawSnapshotLedger checks delivered raw JSON and durable owner/token/log accounting through local HTTP.
func TestSecurityClaudeJSONRawSnapshotLedger(t *testing.T) {
	payloads := []struct{ name, raw string }{
		{"large_exponent", `{"n":1e400,"answer":"` + strings.Repeat("synthetic raw accounting evidence ", 100) + `"}`},
		{"inner_whitespace", `{"answer":` + strings.Repeat(" ", 2000) + `"synthetic"}`},
		{"quoted_scalar", `"synthetic\nquoted \"answer\""`},
		{"literal_null", "null"},
	}
	shapes := []struct{ name, before, after string }{
		{"root_json", `{"type":"response.output_json.done","output_index":0,"json":`, "}"},
		{"part_json", `{"type":"response.output_json.done","output_index":0,"part":{"type":"output_json","json":`, "}}"},
		{"output_json_block", `{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_json","json":`, "}] }]}}"},
	}
	for _, shape := range shapes {
		for _, payload := range payloads {
			for _, measured := range []bool{false, true} {
				name := shape.name + "/" + payload.name
				if measured {
					name += "/measured"
				}
				t.Run(name, func(t *testing.T) {
					wire := "data: " + shape.before + payload.raw + shape.after + "\n\n"
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
					require.Equal(t, payload.raw, delivered.String())
					cost := requestCostQuota(t, id)
					var logs []model.Log
					require.NoError(t, model.LOG_DB.Where("request_id = ? AND type IN ?", id, []int{model.LogTypeConsume, model.LogTypeProvisional}).Find(&logs).Error)
					require.Len(t, logs, 1)
					expected := openai_compatible.CountTokenText(delivered.String(), "gpt-4")
					if measured {
						expected = 3
						require.EqualValues(t, 10, cost)
					}
					require.Equal(t, expected, logs[0].CompletionTokens)
					require.Equal(t, balance-cost, reloadUserQuota(t))
					var token model.Token
					require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
					require.Equal(t, balance-cost, token.RemainQuota)
					require.Equal(t, cost, token.UsedQuota)
					require.EqualValues(t, cost, logs[0].Quota)
					require.Equal(t, model.LogTypeConsume, logs[0].Type)

				})
			}
		}
	}
}
