package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// responseNamespaceFallbackProvider starts a TLS loopback upstream for the
// fallback route that records the request and the quota held at dispatch and
// answers with a plain text completion in channel's format. Parameters: t owns
// cleanup and channel picks Chat or Claude framing. Returns the server, the
// observation channel and the dispatch counter.
func responseNamespaceFallbackProvider(t *testing.T, channel int) (*httptest.Server, chan codexFallbackObservation, *atomic.Int32) {
	t.Helper()
	observed := make(chan codexFallbackObservation, 1)
	calls := &atomic.Int32{}
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		got := codexFallbackObservation{Path: r.URL.Path}
		got.Body, got.Err = io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
		var user model.User
		var token model.Token
		if got.Err == nil {
			got.Err = model.DB.First(&user, fallbackUserID).Error
		}
		if got.Err == nil {
			got.Err = model.DB.First(&token, fallbackTokenID).Error
		}
		got.User, got.Token = user.Quota, token.RemainQuota
		select {
		case observed <- got:
		default:
		}
		writeFallbackTextReply(w, channel)
	}))
	t.Cleanup(provider.Close)
	previousClient := client.HTTPClient
	client.HTTPClient = provider.Client()
	t.Cleanup(func() { client.HTTPClient = previousClient })
	return provider, observed, calls
}

// TestResponseNamespaceHTTPFallbackMissingParentHasNoEffects proves a
// namespaced fallback request whose previous_response_id does not resolve
// still fails during admission (previous_response_not_found) before any
// provider dispatch, pending state, quota reservation or ledger record.
func TestResponseNamespaceHTTPFallbackMissingParentHasNoEffects(t *testing.T) {
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
			require.Equal(t, "previous_response_not_found", apiErr.Code, "namespaces are representable; the missing parent is the rejection")
			require.Nil(t, pendingCommitFromContext(c))
			responseClientSearchNoEffects(t, c, requestID, balance, &calls)
		})
	}
}

// TestResponseNamespaceHTTPFallbackDispatchesFlattenedTools proves grouped
// local tools now reach Chat and Claude fallback upstreams as one flattened
// catalog: same-leaf children of different namespaces stay distinct
// (alpha__shared vs beta__shared), namespace descriptions prefix each child,
// custom children carry the single-string input schema with their grammar,
// and the request settles ordinary token billing without hosted fees.
func TestResponseNamespaceHTTPFallbackDispatchesFlattenedTools(t *testing.T) {
	for _, channel := range []int{channeltype.OpenAICompatible, channeltype.Anthropic} {
		t.Run(fmt.Sprint(channel), func(t *testing.T) {
			const balance = int64(1_000_000)
			xaiVideoSetup(t, balance, false)
			enableStateForTest(t)
			provider, observed, calls := responseNamespaceFallbackProvider(t, channel)
			payload := `{"model":"alias","input":"continue","max_output_tokens":32,"tools":` + responseNamespaceTools + `}`
			c, recorder, requestID := protocolContext(t, channel, "namespace-http-fixture", "/v1/responses", payload, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
			responseNamespaceCleanLedger(t, requestID)
			require.False(t, supportsNativeResponseAPI(metalib.GetByContext(c)))
			apiErr := RelayResponseAPIHelper(c)
			drainCriticalTasks(t)
			require.Nil(t, apiErr)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.EqualValues(t, 1, calls.Load())
			got := <-observed
			require.NoError(t, got.Err)
			require.Equal(t, fallbackUpstreamPath(channel), got.Path)
			require.True(t, metalib.GetByContext(c).ResponseAPIFallback)

			wire := decodeFallbackWire(t, channel, got.Body)
			require.Equal(t, []string{"alpha__shared", "alpha__apply_patch", "beta__shared", "beta__apply_patch"}, fallbackWireToolNames(wire))
			require.Equal(t, "[First local tool group] Alpha implementation", fallbackWireToolByName(t, wire, "alpha__shared").Description)
			require.Equal(t, "[Second local tool group] Beta implementation", fallbackWireToolByName(t, wire, "beta__shared").Description)
			require.JSONEq(t, `{"type":"object","properties":{"alpha":{"type":"string"}}}`, string(fallbackWireToolByName(t, wire, "alpha__shared").Schema))
			require.JSONEq(t, `{"type":"object","properties":{"beta":{"type":"integer"}}}`, string(fallbackWireToolByName(t, wire, "beta__shared").Schema))
			alphaPatch := fallbackWireToolByName(t, wire, "alpha__apply_patch")
			betaPatch := fallbackWireToolByName(t, wire, "beta__apply_patch")
			require.JSONEq(t, codexFallbackCustomInputSchema, string(alphaPatch.Schema))
			require.JSONEq(t, codexFallbackCustomInputSchema, string(betaPatch.Schema))
			require.True(t, strings.HasPrefix(alphaPatch.Description, "[First local tool group] "), alphaPatch.Description)
			require.Contains(t, alphaPatch.Description, "The input must match this lark grammar exactly:\nstart: \"alpha\"")
			require.True(t, strings.HasPrefix(betaPatch.Description, "[Second local tool group] "), betaPatch.Description)
			require.NotContains(t, betaPatch.Description, "grammar", "text-format custom tools carry no grammar")
			requireFallbackWireTurns(t, []fallbackWireTurn{{Role: "user", Text: "continue"}}, wire.Turns)

			var reply struct {
				Status string `json:"status"`
				Output []struct {
					Type    string `json:"type"`
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"output"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &reply))
			require.Equal(t, "completed", reply.Status)
			require.Len(t, reply.Output, 1)
			require.Equal(t, "message", reply.Output[0].Type)
			require.Equal(t, "done", reply.Output[0].Content[0].Text)
			requireFallbackTokenSettlement(t, c, requestID, balance, got.User, got.Token)
		})
	}
}
