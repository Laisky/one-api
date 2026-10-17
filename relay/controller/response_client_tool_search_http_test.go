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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

const responseClientSearchDefinition = `{"type":"tool_search","execution":"client","description":"Find deferred tools","parameters":{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer"}},"required":["query"],"additionalProperties":false}}`
const responseClientSearchCall = `{"type":"tool_search_call","id":"search_item","call_id":"search_1","execution":"client","status":"completed","arguments":{"query":"calendar","limit":2,"large":9007199254740993}}`
const responseClientSearchDiscovered = `[{"type":"namespace","name":"calendar","description":"Deferred client tools","tools":[{"type":"function","name":"lookup","strict":false,"defer_loading":true,"parameters":{"type":"object","properties":{"day":{"type":"string"}}}},{"type":"custom","name":"patch","format":{"type":"grammar","syntax":"lark","definition":"start: WORD"},"defer_loading":true}]},{"type":"custom","name":"apply_patch","format":{"type":"text"},"defer_loading":false}]`

// responseClientSearchObservation records only native protocol fields and quota
// reservations. It carries HTTP fixture errors without testing.Fatal in a goroutine.
type responseClientSearchObservation struct {
	responseNamespaceObservation
	Input json.RawMessage
}

// responseClientSearchAlias installs a colliding gateway MCP alias. Parameters:
// t owns cleanup and base is the loopback URL. Returns: nothing; native client
// search must retain provider ownership despite the enabled gateway alias.
func responseClientSearchAlias(t *testing.T, base string) {
	t.Helper()
	server := &model.MCPServer{Name: "client-search-collision", Status: model.MCPServerStatusEnabled, BaseURL: base, ToolWhitelist: model.JSONStringSlice{"tool_search"}}
	require.NoError(t, model.DB.Create(server).Error)
	t.Cleanup(func() {
		drainCriticalTasks(t)
		require.NoError(t, model.DB.Where("server_id = ?", server.Id).Delete(&model.MCPTool{}).Error)
		require.NoError(t, model.DB.Where("id = ?", server.Id).Delete(&model.MCPServer{}).Error)
	})
	require.NoError(t, model.DB.Create(&model.MCPTool{ServerId: server.Id, Name: "tool_search", Description: "Gateway collision", InputSchema: `{"type":"object"}`}).Error)
}

// responseClientSearchNoEffects asserts pre-dispatch rejection. Parameters: t
// owns assertions, c is the handled request, id identifies isolated receipts,
// balance is the initial quota and calls counts provider dispatches. Returns:
// nothing; rejection cannot reserve quota or create financial records.
func responseClientSearchNoEffects(t *testing.T, c *gin.Context, id string, balance int64, calls *atomic.Int32) {
	t.Helper()
	require.Zero(t, calls.Load())
	require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
	require.Equal(t, balance, reloadUserQuota(t))
	var user model.User
	var token model.Token
	require.NoError(t, model.DB.First(&user, fallbackUserID).Error)
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.Zero(t, user.UsedQuota)
	require.Equal(t, balance, token.RemainQuota)
	require.Zero(t, token.UsedQuota)
	var costs, logs int64
	require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&costs).Error)
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ?", id).Count(&logs).Error)
	require.Zero(t, costs)
	require.Zero(t, logs)
}

// TestResponseClientToolSearchHTTPNativeWireAndBilling checks actual TLS native
// JSON/SSE forwarding for initial discovery and client-supplied continuation.
// Parameters: t owns isolated fixtures. Returns: nothing; tools, structured
// arguments, dynamic definitions and token-only settlement must survive intact.
func TestResponseClientToolSearchHTTPNativeWireAndBilling(t *testing.T) {
	const balance = int64(1_000_000)
	response := `{"id":"resp_client_search","object":"response","status":"completed","model":"client-search-fixture","output":[` + responseClientSearchCall + `],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}`
	for _, stream := range []bool{false, true} {
		for _, phase := range []string{"continuation=false", "continuation=true", "additional_tools=true"} {
			t.Run(fmt.Sprintf("stream=%t/%s", stream, phase), func(t *testing.T) {
				xaiVideoSetup(t, balance, false)
				observed := make(chan responseClientSearchObservation, 1)
				var calls atomic.Int32
				provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					got := responseClientSearchObservation{responseNamespaceObservation: responseNamespaceObservation{Path: r.URL.Path}}
					var wire struct {
						Tools json.RawMessage `json:"tools"`
						Input json.RawMessage `json:"input"`
					}
					got.Err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&wire)
					got.Tools, got.Input = wire.Tools, wire.Input
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
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = fmt.Fprintf(w, "event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":%s}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":%s}\n\n", responseClientSearchCall, response)
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, response)
					}
				}))
				t.Cleanup(provider.Close)
				oldClient := client.HTTPClient
				client.HTTPClient = provider.Client()
				t.Cleanup(func() { client.HTTPClient = oldClient })
				responseClientSearchAlias(t, provider.URL)
				tools := `[` + responseClientSearchDefinition + `]`
				input := `"Find a calendar tool"`
				if phase == "continuation=true" {
					input = `[` + responseClientSearchCall + `,{"type":"tool_search_output","call_id":"search_1","execution":"client","status":"completed","tools":` + responseClientSearchDiscovered + `}]`
				}
				toolField := `,"tools":` + tools
				if phase == "additional_tools=true" {
					// Lite builder catalogs carry the client search declaration
					// and deferred definitions inside input, with no outer Tools.
					input = `[{"type":"additional_tools","role":"developer","tools":[` + responseClientSearchDefinition + `,` + responseClientSearchDiscovered[1:] + `},{"role":"user","content":"hello"}]`
					toolField = ""
				}
				payload := fmt.Sprintf(`{"model":"alias","input":%s,"stream":%t,"max_output_tokens":32%s}`, input, stream, toolField)
				c, recorder, id := protocolContext(t, channeltype.OpenAICompatible, "client-search-fixture", "/v1/responses", payload, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
				c.Set(ctxkey.Config, model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse})
				responseNamespaceCleanLedger(t, id)
				require.True(t, supportsNativeResponseAPI(metalib.GetByContext(c)))
				apiErr := RelayResponseAPIHelper(c)
				drainCriticalTasks(t)
				require.Nil(t, apiErr)
				require.Equal(t, http.StatusOK, recorder.Code)
				require.EqualValues(t, 1, calls.Load())
				got := <-observed
				require.NoError(t, got.Err)
				require.Equal(t, "/v1/responses", got.Path)
				if phase == "additional_tools=true" {
					require.Empty(t, got.Tools, "input-only catalogs must not synthesize outer tools")
					require.JSONEq(t, input, string(got.Input))
				} else {
					require.JSONEq(t, tools, string(got.Tools))
				}
				if phase == "continuation=true" {
					require.JSONEq(t, input, string(got.Input))
					require.Contains(t, string(got.Input), "9007199254740993")
				}
				require.False(t, metalib.GetByContext(c).ResponseAPIFallback, "MCP alias cannot steal client tool_search")
				held := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
				require.Positive(t, held)
				require.Equal(t, balance-held, got.User)
				require.Equal(t, balance-held, got.Token)
				var reply struct {
					Output json.RawMessage `json:"output"`
					Usage  struct {
						Input  int `json:"input_tokens"`
						Output int `json:"output_tokens"`
						Total  int `json:"total_tokens"`
					} `json:"usage"`
				}
				if stream {
					require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
					done, completed := 0, 0
					for _, event := range parseSSEEvents(recorder.Body.String()) {
						if event.event == "response.output_item.done" {
							var item struct {
								Item json.RawMessage `json:"item"`
							}
							require.NoError(t, json.Unmarshal([]byte(event.data), &item))
							require.JSONEq(t, responseClientSearchCall, string(item.Item))
							require.Contains(t, string(item.Item), "9007199254740993")
							done++
						}
						if event.event == "response.completed" {
							var envelope struct {
								Response json.RawMessage `json:"response"`
							}
							require.NoError(t, json.Unmarshal([]byte(event.data), &envelope))
							require.NoError(t, json.Unmarshal(envelope.Response, &reply))
							completed++
						}
					}
					require.Equal(t, 1, done)
					require.Equal(t, 1, completed)
				} else {
					require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &reply))
				}
				require.JSONEq(t, `[`+responseClientSearchCall+`]`, string(reply.Output))
				require.Contains(t, string(reply.Output), "9007199254740993")
				require.Equal(t, 11, reply.Usage.Input)
				require.Equal(t, 7, reply.Usage.Output)
				require.Equal(t, 18, reply.Usage.Total)
				require.EqualValues(t, 25, requestCostQuota(t, id), "11 input + 7 output*2, no hosted-search fee")
				require.EqualValues(t, 25, consumeLogQuota(t, id))
				require.Equal(t, balance-25, reloadUserQuota(t))
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				require.Equal(t, balance-25, token.RemainQuota)
				require.EqualValues(t, 25, token.UsedQuota)
			})
		}
	}
}

// TestResponseClientToolSearchHTTPRejectsBeforeEffects exercises malformed and
// unsupported search execution and discovered tools through native admission.
// Parameters: t owns isolated HTTP/ledger fixtures. Returns: nothing; no invalid
// request may reach provider work, quota reservation, or financial persistence.
func TestResponseClientToolSearchHTTPRejectsBeforeEffects(t *testing.T) {
	type rejection struct{ name, tools, input, code string }
	cases := []rejection{
		{name: "server_definition", tools: `[{"type":"tool_search","execution":"server"}]`},
		{name: "missing_definition_execution", tools: `[{"type":"tool_search"}]`},
		{name: "ambiguous_definition_execution", tools: `[{"type":"tool_search","execution":"server","Execution":"client"}]`, code: "invalid_response_api_request"},
		{name: "duplicate_definition_execution", tools: `[{"type":"tool_search","execution":"server","execution":"client"}]`, code: "invalid_response_api_request"},
		{name: "escaped_definition_execution", tools: `[{"type":"tool_search","execution":"server","execu\u0074ion":"client"}]`, code: "invalid_response_api_request"},
		{name: "server_call", input: `[{"type":"tool_search_call","call_id":"search_1","execution":"server","arguments":{"query":"x"}}]`},
		{name: "missing_call_execution", input: `[{"type":"tool_search_call","call_id":"search_1","arguments":{"query":"x"}}]`},
		{name: "ambiguous_call_execution", input: `[{"type":"tool_search_call","call_id":"search_1","execution":"server","Execution":"client","arguments":{"query":"x"}}]`, code: "invalid_response_api_request"},
		{name: "server_output", input: `[{"type":"tool_search_output","call_id":"search_1","execution":"server","tools":[]}]`},
		{name: "missing_output_execution", input: `[{"type":"tool_search_output","call_id":"search_1","tools":[]}]`},
		{name: "ambiguous_output_execution", input: `[{"type":"tool_search_output","call_id":"search_1","execution":"server","Execution":"client","tools":[]}]`, code: "invalid_response_api_request"},
	}
	for _, tool := range []string{`{"type":"web_search"}`, `{"type":"code_interpreter","container":{"type":"auto"}}`, `{"type":"image_generation"}`, `{"type":"file_search","vector_store_ids":["vs_fixture"]}`, `{"type":"mcp","server_label":"fixture"}`, `{"type":"future_paid_tool"}`, `{"type":"namespace","name":"outer","tools":[{"type":"namespace","name":"inner","tools":[{"type":"function","name":"lookup"}]}]}`} {
		code := ""
		if strings.HasPrefix(tool, `{"type":"namespace"`) {
			// Nested namespaces are structurally invalid in the typed decoder.
			code = "invalid_response_api_request"
		}
		cases = append(cases, rejection{name: "discovered_" + tool, input: `[{"type":"tool_search_output","call_id":"search_1","execution":"client","tools":[` + tool + `]}]`, code: code})
	}
	for name, tool := range map[string]string{
		"discovered_duplicate_type":            `{"type":"code_interpreter","type":"function","name":"lookup"}`,
		"discovered_casefold_type":             `{"type":"code_interpreter","Type":"function","name":"lookup"}`,
		"discovered_namespace_ambiguous_tools": `{"type":"namespace","name":"calendar","tools":[{"type":"code_interpreter"}],"Tools":[{"type":"function","name":"lookup"}]}`,
	} {
		cases = append(cases, rejection{name: name, input: `[{"type":"tool_search_output","call_id":"search_1","execution":"client","tools":[` + tool + `]}]`, code: "invalid_response_api_request"})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(1_000_000)
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				http.Error(w, "unexpected dispatch", http.StatusInternalServerError)
			}))
			t.Cleanup(provider.Close)
			oldClient := client.HTTPClient
			client.HTTPClient = provider.Client()
			t.Cleanup(func() { client.HTTPClient = oldClient })
			tools, input := tc.tools, tc.input
			if tools == "" {
				tools = `[` + responseClientSearchDefinition + `]`
			}
			if input == "" {
				input = `"hello"`
			}
			payload := `{"model":"alias","input":` + input + `,"max_output_tokens":32,"tools":` + tools + `}`
			c, _, id := protocolContext(t, channeltype.OpenAICompatible, "client-search-fixture", "/v1/responses", payload, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
			c.Set(ctxkey.Config, model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse})
			responseNamespaceCleanLedger(t, id)
			apiErr := RelayResponseAPIHelper(c)
			drainCriticalTasks(t)
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			code := tc.code
			if code == "" {
				code = "tool_billing_not_supported"
			}
			require.Equal(t, code, apiErr.Code)
			responseClientSearchNoEffects(t, c, id, balance, &calls)
		})
	}
}

// TestResponseClientToolSearchHTTPFallbackRejectsBeforeState checks unsupported
// definitions and history before parent hydration or pending-state snapshots.
// Parameters: t owns Chat/Claude fixtures. Returns: nothing; explicit fallback
// errors must precede all provider, state, quota and financial side effects.
func TestResponseClientToolSearchHTTPFallbackRejectsBeforeState(t *testing.T) {
	for _, channel := range []int{channeltype.OpenAICompatible, channeltype.Anthropic} {
		for _, history := range []string{"definition", "call", "output", "additional_tools"} {
			t.Run(fmt.Sprintf("channel=%d/%s", channel, history), func(t *testing.T) {
				const balance = int64(1_000_000)
				xaiVideoSetup(t, balance, false)
				enableStateForTest(t)
				var calls atomic.Int32
				provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					http.Error(w, "unexpected fallback", http.StatusInternalServerError)
				}))
				t.Cleanup(provider.Close)
				oldClient := client.HTTPClient
				client.HTTPClient = provider.Client()
				t.Cleanup(func() { client.HTTPClient = oldClient })
				input, tools := `"continue"`, `[]`
				switch history {
				case "definition":
					tools = `[` + responseClientSearchDefinition + `]`
				case "call":
					input = `[` + responseClientSearchCall + `]`
				case "output":
					input = `[{"type":"tool_search_output","call_id":"search_1","execution":"client","tools":` + responseClientSearchDiscovered + `}]`
				case "additional_tools":
					input = `[{"type":"additional_tools","role":"developer","tools":[` + responseClientSearchDefinition + `,` + responseClientSearchDiscovered[1:] + `}]`
				}
				toolField := `,"tools":` + tools
				if history == "additional_tools" {
					toolField = ""
				}
				payload := `{"model":"alias","input":` + input + `,"previous_response_id":"resp_ffffffffffffffffffffffffffffffff"` + toolField + `}`
				c, _, id := protocolContext(t, channel, "client-search-fixture", "/v1/responses", payload, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
				responseNamespaceCleanLedger(t, id)
				require.False(t, supportsNativeResponseAPI(metalib.GetByContext(c)))
				apiErr := RelayResponseAPIHelper(c)
				drainCriticalTasks(t)
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				require.Equal(t, "tool_search_not_supported", apiErr.Code, "reject before missing-parent hydration")
				require.Nil(t, pendingCommitFromContext(c))
				responseClientSearchNoEffects(t, c, id, balance, &calls)
			})
		}
	}
}
