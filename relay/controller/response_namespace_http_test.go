package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

const responseNamespaceTools = `[
    {"type":"namespace","name":"alpha","description":"First local tool group","tools":[
        {"type":"function","name":"shared","description":"Alpha implementation","parameters":{"type":"object","properties":{"alpha":{"type":"string"}}},"strict":false,"defer_loading":false},
        {"type":"custom","name":"apply_patch","format":{"type":"grammar","syntax":"lark","definition":"start: \"alpha\""},"defer_loading":false}
    ]},
    {"type":"namespace","name":"beta","description":"Second local tool group","tools":[
        {"type":"function","name":"shared","description":"Beta implementation","parameters":{"type":"object","properties":{"beta":{"type":"integer"}}},"strict":true,"defer_loading":true},
        {"type":"custom","name":"apply_patch","format":{"type":"text"},"defer_loading":true}
    ]}
]`

// responseNamespaceObservation captures provider wire bytes and quota holds
// without calling testing.Fatal from the loopback HTTP server goroutine.
type responseNamespaceObservation struct {
	Path        string
	Tools       json.RawMessage
	User, Token int64
	Err         error
}

// responseNamespaceAlias collides a gateway MCP alias with the namespace tool
// type; grouped local tools must retain native provider ownership.
func responseNamespaceAlias(t *testing.T, baseURL string) {
	t.Helper()
	server := &model.MCPServer{
		Name: "namespace-http-routing", Status: model.MCPServerStatusEnabled,
		BaseURL: baseURL, ToolWhitelist: model.JSONStringSlice{"namespace"},
	}
	require.NoError(t, model.DB.Create(server).Error)
	t.Cleanup(func() {
		drainCriticalTasks(t)
		require.NoError(t, model.DB.Where("server_id = ?", server.Id).Delete(&model.MCPTool{}).Error)
		require.NoError(t, model.DB.Where("id = ?", server.Id).Delete(&model.MCPServer{}).Error)
	})
	require.NoError(t, model.DB.Create(&model.MCPTool{
		ServerId: server.Id, Name: "namespace", Description: "Gateway alias collision",
		InputSchema: `{"type":"object","properties":{}}`,
	}).Error)
}

// responseNamespaceCleanLedger isolates deterministic request IDs when the
// focused regression is repeated within one SQLite-backed test process.
func responseNamespaceCleanLedger(t *testing.T, requestID string) {
	t.Helper()
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.UserRequestCost{}).Error)
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Delete(&model.Log{}).Error)
}

// TestResponseNamespaceHTTPNativePreservesWireAndBilling proves grouped local
// tools survive actual native stream/nonstream forwarding without hosted fees.
func TestResponseNamespaceHTTPNativePreservesWireAndBilling(t *testing.T) {
	const balance = int64(1_000_000)
	const response = `{"id":"resp_namespace","object":"response","status":"completed","model":"namespace-http-fixture","output":[{"type":"message","id":"msg_namespace","role":"assistant","content":[{"type":"output_text","text":"done"}]}],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}`
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			for _, grouped := range []bool{false, true} {
				t.Run(fmt.Sprintf("namespace=%t", grouped), func(t *testing.T) {
					xaiVideoSetup(t, balance, false)
					observed := make(chan responseNamespaceObservation, 1)
					var calls atomic.Int32
					provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						got := responseNamespaceObservation{Path: r.URL.Path}
						var wire struct {
							Tools json.RawMessage `json:"tools"`
						}
						got.Err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&wire)
						got.Tools = wire.Tools
						var user model.User
						var token model.Token
						if got.Err == nil {
							got.Err = model.DB.First(&user, fallbackUserID).Error
						}
						if got.Err == nil {
							got.Err = model.DB.First(&token, fallbackTokenID).Error
						}
						got.User, got.Token = user.Quota, token.RemainQuota
						observed <- got
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							_, _ = fmt.Fprintf(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":%s}\n\n", response)
						} else {
							w.Header().Set("Content-Type", "application/json")
							_, _ = io.WriteString(w, response)
						}
					}))
					t.Cleanup(provider.Close)
					previousClient := client.HTTPClient
					client.HTTPClient = provider.Client()
					t.Cleanup(func() { client.HTTPClient = previousClient })
					responseNamespaceAlias(t, provider.URL)
					tools := `[{"type":"function","name":"shared","parameters":{"type":"object"}},{"type":"custom","name":"apply_patch","format":{"type":"text"}}]`
					if grouped {
						tools = responseNamespaceTools
					}
					payload := fmt.Sprintf(`{"model":"alias","input":"hello","stream":%t,"max_output_tokens":32,"tools":%s}`, stream, tools)
					c, recorder, requestID := protocolContext(t, channeltype.OpenAICompatible, "namespace-http-fixture", "/v1/responses", payload, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
					c.Set(ctxkey.Config, model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse})
					responseNamespaceCleanLedger(t, requestID)
					require.True(t, supportsNativeResponseAPI(metalib.GetByContext(c)))
					apiErr := RelayResponseAPIHelper(c)
					drainCriticalTasks(t)
					require.Nil(t, apiErr)
					require.Equal(t, http.StatusOK, recorder.Code)
					require.EqualValues(t, 1, calls.Load())
					got := <-observed
					require.NoError(t, got.Err)
					require.Equal(t, "/v1/responses", got.Path)
					require.JSONEq(t, tools, string(got.Tools), "retain groups, same child names across namespaces, grammar and explicit false")
					require.False(t, metalib.GetByContext(c).ResponseAPIFallback, "gateway alias must not steal provider-owned namespace tools")
					held := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
					require.Positive(t, held, "retain ordinary token reservation")
					require.Equal(t, balance-held, got.User)
					require.Equal(t, balance-held, got.Token)
					var reply openai.ResponseAPIResponse
					if stream {
						require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
						var completed int
						for _, event := range parseSSEEvents(recorder.Body.String()) {
							if event.event == "response.completed" {
								var envelope struct {
									Response openai.ResponseAPIResponse `json:"response"`
								}
								require.NoError(t, json.Unmarshal([]byte(event.data), &envelope))
								reply = envelope.Response
								completed++
							}
						}
						require.Equal(t, 1, completed)
					} else {
						require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &reply))
					}
					require.NotNil(t, reply.Usage)
					require.Equal(t, 11, reply.Usage.InputTokens)
					require.Equal(t, 7, reply.Usage.OutputTokens)
					require.Equal(t, 18, reply.Usage.TotalTokens)
					// Explicit tariff: 11 input + 7 output * 2, with no hosted fee.
					require.EqualValues(t, 25, requestCostQuota(t, requestID))
					require.EqualValues(t, 25, consumeLogQuota(t, requestID))
					require.Equal(t, balance-25, reloadUserQuota(t))
					var token model.Token
					require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
					require.Equal(t, balance-25, token.RemainQuota)
					require.EqualValues(t, 25, token.UsedQuota)
				})
			}
		})
	}
}

// TestResponseNamespaceHTTPRejectsUnsafeChildrenBeforeQuota traverses actual
// native HTTP admission instead of testing only the typed validator.
func TestResponseNamespaceHTTPRejectsUnsafeChildrenBeforeQuota(t *testing.T) {
	cases := map[string]string{
		"ambiguous_type":     `{"type":"namespace","name":"local","tools":[{"type":"code_interpreter","Type":"function","name":"run"}]}`,
		"duplicate_type":     `{"type":"namespace","name":"local","tools":[{"type":"code_interpreter","type":"function","name":"run"}]}`,
		"escaped_type":       `{"type":"namespace","name":"local","tools":[{"type":"code_interpreter","t\u0079pe":"function","name":"run"}]}`,
		"ambiguous_tools":    `{"type":"namespace","name":"local","tools":[{"type":"code_interpreter"}],"Tools":[{"type":"function","name":"run"}]}`,
		"unicode_fold_tools": `{"type":"namespace","name":"local","tools":[{"type":"code_interpreter"}],"tool\u017f":[{"type":"function","name":"run"}]}`,

		"hosted_code":      `{"type":"namespace","name":"local","tools":[{"type":"code_interpreter","container":{"type":"auto"}}]}`,
		"hosted_search":    `{"type":"namespace","name":"local","tools":[{"type":"web_search"}]}`,
		"hosted_file":      `{"type":"namespace","name":"local","tools":[{"type":"file_search","vector_store_ids":["vs_fixture"]}]}`,
		"explicit_mcp":     `{"type":"namespace","name":"local","tools":[{"type":"mcp","server_label":"fixture"}]}`,
		"unknown_paid":     `{"type":"namespace","name":"local","tools":[{"type":"future_paid_tool"}]}`,
		"nested_namespace": `{"type":"namespace","name":"local","tools":[{"type":"namespace","name":"nested","tools":[{"type":"function","name":"lookup"}]}]}`,
		"missing_name":     `{"type":"namespace","tools":[{"type":"function","name":"lookup"}]}`,
		"invalid_name":     `{"type":"namespace","name":"not a valid name","tools":[{"type":"function","name":"lookup"}]}`,
		"empty_tools":      `{"type":"namespace","name":"local","tools":[]}`,
		"missing_tools":    `{"type":"namespace","name":"local"}`,
	}
	for name, tool := range cases {
		t.Run(name, func(t *testing.T) {
			const balance = int64(1_000_000)
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				http.Error(w, "unexpected provider dispatch", http.StatusInternalServerError)
			}))
			t.Cleanup(provider.Close)
			previousClient := client.HTTPClient
			client.HTTPClient = provider.Client()
			t.Cleanup(func() { client.HTTPClient = previousClient })
			payload := `{"model":"alias","input":"hello","max_output_tokens":32,"tools":[` + tool + `]}`
			c, _, requestID := protocolContext(t, channeltype.OpenAICompatible, "namespace-http-fixture", "/v1/responses", payload, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
			c.Set(ctxkey.Config, model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse})
			responseNamespaceCleanLedger(t, requestID)
			apiErr := RelayResponseAPIHelper(c)
			drainCriticalTasks(t)
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.Zero(t, calls.Load())
			require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
			require.Equal(t, balance, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance, token.RemainQuota)
			require.Zero(t, token.UsedQuota)
			var costs, logs int64
			require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", requestID).Count(&costs).Error)
			require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ?", requestID).Count(&logs).Error)
			require.Zero(t, costs)
			require.Zero(t, logs)
		})
	}
}

// TestResponseNamespaceHTTPFallbackRejectsBeforeState proves fallback never
// flattens a namespace or hydrates/reserves state before explicit rejection.
func TestResponseNamespaceHTTPFallbackRejectsBeforeState(t *testing.T) {
	for _, channel := range []int{channeltype.OpenAICompatible, channeltype.Anthropic} {
		t.Run(fmt.Sprint(channel), func(t *testing.T) {
			const balance = int64(1_000_000)
			xaiVideoSetup(t, balance, false)
			enableStateForTest(t)
			var calls atomic.Int32
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				http.Error(w, "unexpected fallback dispatch", http.StatusInternalServerError)
			}))
			t.Cleanup(provider.Close)
			previousClient := client.HTTPClient
			client.HTTPClient = provider.Client()
			t.Cleanup(func() { client.HTTPClient = previousClient })
			payload := `{"model":"alias","input":"continue","previous_response_id":"resp_ffffffffffffffffffffffffffffffff","tools":` + responseNamespaceTools + `}`
			c, _, requestID := protocolContext(t, channel, "namespace-http-fixture", "/v1/responses", payload, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
			responseNamespaceCleanLedger(t, requestID)
			require.False(t, supportsNativeResponseAPI(metalib.GetByContext(c)))
			apiErr := RelayResponseAPIHelper(c)
			drainCriticalTasks(t)
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.Equal(t, "tool_namespace_not_supported", apiErr.Code, "reject before missing-parent hydration")
			require.Nil(t, pendingCommitFromContext(c))
			require.Zero(t, calls.Load())
			require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
			require.Equal(t, balance, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance, token.RemainQuota)
			require.Zero(t, token.UsedQuota)
		})
	}
}
