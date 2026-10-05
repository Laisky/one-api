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

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// claudeToolPolicyPerSearch is the explicit channel tariff used by the ledger
// fixtures so expected charges are independent of provider list prices.
const claudeToolPolicyPerSearch = int64(17)

// claudeToolPolicyNativeJSON is a native Messages receipt with two billed searches.
const claudeToolPolicyNativeJSON = `{"id":"msg_tool","type":"message","role":"assistant","model":"claude-sonnet-4","content":[` +
	`{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{"query":"a"}},` +
	`{"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[]},` +
	`{"type":"server_tool_use","id":"srvtoolu_2","name":"web_search","input":{"query":"b"}},` +
	`{"type":"web_search_tool_result","tool_use_id":"srvtoolu_2","content":[]},` +
	`{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7%s}}`

// claudeToolPolicySSEPrefix starts a native stream and runs one search.
const claudeToolPolicySSEPrefix = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_tool","type":"message","role":"assistant","model":"claude-sonnet-4","content":[],"stop_reason":null,"usage":{"input_tokens":11,"output_tokens":1,"server_tool_use":{"web_search_requests":0}}}}` + "\n\n" +
	"event: content_block_start\n" +
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{}}}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"a\"}"}}` + "\n\n" +
	"event: content_block_stop\n" +
	`data: {"type":"content_block_stop","index":0}` + "\n\n"

// claudeToolPolicySSEComplete finishes the stream with a second search, an
// intermediate cumulative receipt, the final receipt, and a duplicate final receipt.
const claudeToolPolicySSEComplete = claudeToolPolicySSEPrefix +
	"event: message_delta\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":null},"usage":{"output_tokens":4,"server_tool_use":{"web_search_requests":1}}}` + "\n\n" +
	"event: content_block_start\n" +
	`data: {"type":"content_block_start","index":1,"content_block":{"type":"server_tool_use","id":"srvtoolu_2","name":"web_search","input":{}}}` + "\n\n" +
	"event: content_block_stop\n" +
	`data: {"type":"content_block_stop","index":1}` + "\n\n" +
	"event: content_block_start\n" +
	`data: {"type":"content_block_start","index":2,"content_block":{"type":"text","text":""}}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"ok"}}` + "\n\n" +
	"event: content_block_stop\n" +
	`data: {"type":"content_block_stop","index":2}` + "\n\n" +
	"event: message_delta\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":7,"server_tool_use":{"web_search_requests":2}}}` + "\n\n" +
	"event: message_delta\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":7,"server_tool_use":{"web_search_requests":2}}}` + "\n\n" +
	"event: message_stop\n" +
	`data: {"type":"message_stop"}` + "\n\n"

// claudeToolPolicyChatJSON is an ordinary converted Chat Completions receipt.
const claudeToolPolicyChatJSON = `{"id":"chat_tool","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`

// claudeToolPolicyResponsesJSON is a converted Responses receipt with two searches.
const claudeToolPolicyResponsesJSON = `{"id":"resp_tool","object":"response","status":"completed","model":"claude-sonnet-4","output":[` +
	`{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"a"}},` +
	`{"type":"web_search_call","id":"ws_2","status":"completed","action":{"type":"search","query":"b"}},` +
	`{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],` +
	`"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}`

// claudeToolPolicyResponsesSSE is a converted Responses stream with two searches
// reported both as output items and again in the terminal response.
const claudeToolPolicyResponsesSSE = "event: response.created\n" +
	`data: {"type":"response.created","response":{"id":"resp_tool","object":"response","status":"in_progress","model":"claude-sonnet-4","output":[]}}` + "\n\n" +
	"event: response.output_item.done\n" +
	`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"a"}}}` + "\n\n" +
	"event: response.output_item.done\n" +
	`data: {"type":"response.output_item.done","output_index":1,"item":{"type":"web_search_call","id":"ws_2","status":"completed","action":{"type":"search","query":"b"}}}` + "\n\n" +
	"event: response.output_text.delta\n" +
	`data: {"type":"response.output_text.delta","output_index":2,"content_index":0,"delta":"ok"}` + "\n\n" +
	"event: response.completed\n" +
	`data: {"type":"response.completed","response":` + claudeToolPolicyResponsesJSON + `}` + "\n\n" +
	"data: [DONE]\n\n"

// claudeToolPolicyCase describes one Claude Messages request through the real controller.
type claudeToolPolicyCase struct {
	channel     int
	actual      string
	payload     string
	contentType string
	response    string
	beta        string
	config      *model.ChannelConfig
	tooling     *model.ChannelToolingConfig
	prepare     func(c *gin.Context)
}

// claudeToolPolicyResult carries the observable effects of one controller run.
type claudeToolPolicyResult struct {
	apiErr  *relaymodel.ErrorWithStatusCode
	calls   int32
	bodies  []map[string]any
	headers []http.Header
	id      string
	c       *gin.Context
	w       *httptest.ResponseRecorder
}

// claudeToolPolicyPayload builds a Claude Messages body with the given tools and extra root fields.
func claudeToolPolicyPayload(t *testing.T, stream bool, tools []any, extra map[string]any) string {
	t.Helper()
	body := map[string]any{
		"model":      "alias",
		"max_tokens": 64,
		"stream":     stream,
		"messages":   []any{map[string]any{"role": "user", "content": "hello"}},
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	for key, value := range extra {
		body[key] = value
	}
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	return string(encoded)
}

// claudeToolPolicyWebSearch returns a versioned Anthropic web search declaration.
func claudeToolPolicyWebSearch() map[string]any {
	return map[string]any{"type": "web_search_20250305", "name": "web_search", "max_uses": 3}
}

// runClaudeToolPolicyCase drives RelayClaudeMessagesHelper against a synthetic
// TLS upstream (or the AWS endpoint override) and drains post-billing work.
func runClaudeToolPolicyCase(t *testing.T, balance int64, tc claudeToolPolicyCase) claudeToolPolicyResult {
	t.Helper()
	xaiVideoSetup(t, balance, false)
	var calls atomic.Int32
	observed := make(chan map[string]any, 8)
	headers := make(chan http.Header, 8)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request map[string]any
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&request); err != nil {
			http.Error(w, "invalid synthetic request", http.StatusBadRequest)
			return
		}
		select {
		case observed <- request:
		default:
		}
		select {
		case headers <- r.Header.Clone():
		default:
		}
		w.Header().Set("Content-Type", tc.contentType)
		_, _ = io.WriteString(w, tc.response)
	})
	var upstream *httptest.Server
	if tc.channel == channeltype.AwsClaude {
		upstream = httptest.NewServer(handler)
		t.Setenv("AWS_ENDPOINT_URL", upstream.URL)
		t.Setenv("AWS_MAX_ATTEMPTS", "1")
	} else {
		upstream = httptest.NewTLSServer(handler)
		previous := client.HTTPClient
		client.HTTPClient = upstream.Client()
		t.Cleanup(func() { client.HTTPClient = previous })
	}
	t.Cleanup(upstream.Close)
	c, w, id := protocolContext(t, tc.channel, tc.actual, "/v1/messages", tc.payload, upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	if tc.config != nil {
		c.Set(ctxkey.Config, *tc.config)
	}
	if tc.beta != "" {
		c.Request.Header.Set("anthropic-beta", tc.beta)
	}
	if tc.tooling != nil {
		value, _ := c.Get(ctxkey.ChannelModel)
		require.NoError(t, value.(*model.Channel).SetToolingConfig(tc.tooling))
	}
	if tc.prepare != nil {
		tc.prepare(c)
	}
	apiErr := RelayClaudeMessagesHelper(c)
	drainCriticalTasks(t)
	result := claudeToolPolicyResult{apiErr: apiErr, calls: calls.Load(), id: id, c: c, w: w}
	for len(observed) > 0 {
		result.bodies = append(result.bodies, <-observed)
	}
	for len(headers) > 0 {
		result.headers = append(result.headers, <-headers)
	}
	return result
}

// requireClaudeToolPolicyLedger asserts one exact owner, token, request-cost and log settlement.
func requireClaudeToolPolicyLedger(t *testing.T, result claudeToolPolicyResult, balance, charged int64) {
	t.Helper()
	require.EqualValues(t, charged, requestCostQuota(t, result.id))
	require.Equal(t, balance-charged, reloadUserQuota(t))
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.Equal(t, balance-charged, token.RemainQuota)
	require.Equal(t, charged, token.UsedQuota)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", result.id, model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1, "exactly one final consume row")
	require.EqualValues(t, charged, logs[0].Quota)
}

// requireClaudeToolPolicyUntouched asserts a rejection with no dispatch and no net charge.
func requireClaudeToolPolicyUntouched(t *testing.T, result claudeToolPolicyResult, balance int64) {
	t.Helper()
	require.NotNil(t, result.apiErr, "the request must be rejected")
	require.Equal(t, http.StatusBadRequest, result.apiErr.StatusCode, fmt.Sprint(result.apiErr))
	require.Zero(t, result.calls, "a rejected capability must never reach the provider")
	require.Equal(t, balance, reloadUserQuota(t))
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.Equal(t, balance, token.RemainQuota)
	require.Zero(t, token.UsedQuota)
}

// claudeToolPolicyAllowSearch prices web search explicitly and whitelists it.
func claudeToolPolicyAllowSearch() *model.ChannelToolingConfig {
	return &model.ChannelToolingConfig{
		Whitelist: []string{"web_search", "tool_search"},
		Pricing:   map[string]model.ToolPricingLocal{"web_search": {QuotaPerCall: claudeToolPolicyPerSearch}},
	}
}

// TestSecurityClaudeNativeToolPolicyAdmission proves disallowed or unpriceable
// provider capabilities are rejected before reservation and upstream dispatch.
func TestSecurityClaudeNativeToolPolicyAdmission(t *testing.T) {
	const balance = int64(100_000)
	nativeOK := fmt.Sprintf(claudeToolPolicyNativeJSON, "")
	for _, tc := range []struct {
		name string
		run  claudeToolPolicyCase
	}{
		{name: "native_search_not_whitelisted", run: claudeToolPolicyCase{
			channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "application/json", response: nativeOK,
			payload: claudeToolPolicyPayload(t, false, []any{claudeToolPolicyWebSearch()}, nil),
			tooling: &model.ChannelToolingConfig{Whitelist: []string{"tool_search"}, Pricing: map[string]model.ToolPricingLocal{"web_search": {QuotaPerCall: claudeToolPolicyPerSearch}}},
		}},
		{name: "native_stream_search_not_whitelisted", run: claudeToolPolicyCase{
			channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "text/event-stream", response: claudeToolPolicySSEComplete,
			payload: claudeToolPolicyPayload(t, true, []any{claudeToolPolicyWebSearch()}, nil),
			tooling: &model.ChannelToolingConfig{Whitelist: []string{"web_fetch"}, Pricing: map[string]model.ToolPricingLocal{"web_search": {QuotaPerCall: claudeToolPolicyPerSearch}}},
		}},
		{name: "native_unpriced_code_execution", run: claudeToolPolicyCase{
			channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "application/json", response: nativeOK,
			payload: claudeToolPolicyPayload(t, false, []any{map[string]any{"type": "code_execution_20250825", "name": "code_execution"}}, nil),
		}},
		{name: "native_unknown_server_tool", run: claudeToolPolicyCase{
			channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "application/json", response: nativeOK, beta: "advisor-tool-2026-03-01",
			payload: claudeToolPolicyPayload(t, false, []any{map[string]any{"type": "advisor_20260301", "name": "advisor"}}, nil),
		}},
		{name: "native_mcp_connector_beta_opt_in", run: claudeToolPolicyCase{
			channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "application/json", response: nativeOK, beta: "mcp-client-2025-11-20",
			payload: claudeToolPolicyPayload(t, false, []any{map[string]any{"type": "mcp_toolset", "mcp_server_name": "remote"}},
				map[string]any{"mcp_servers": []any{map[string]any{"type": "url", "url": "https://mcp.example.invalid/sse", "name": "remote"}}}),
		}},
		{name: "native_container_without_code_execution", run: claudeToolPolicyCase{
			channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "application/json", response: nativeOK,
			payload: claudeToolPolicyPayload(t, false, nil, map[string]any{"container": "container_synthetic"}),
		}},
		{name: "converted_unpriced_web_fetch", run: claudeToolPolicyCase{
			channel: channeltype.OpenAI, actual: "claude-sonnet-4", contentType: "application/json", response: claudeToolPolicyChatJSON,
			payload: claudeToolPolicyPayload(t, false, []any{map[string]any{"type": "web_fetch_20250910", "name": "web_fetch"}}, nil),
		}},
		{name: "converted_tool_search_becomes_unlisted_search", run: claudeToolPolicyCase{
			channel: channeltype.OpenAI, actual: "claude-sonnet-4", contentType: "application/json", response: claudeToolPolicyChatJSON,
			payload: claudeToolPolicyPayload(t, false, []any{map[string]any{"type": "tool_search_tool_regex_20251119", "name": "tool_search_tool_regex"}}, nil),
			tooling: &model.ChannelToolingConfig{Whitelist: []string{"tool_search"}},
		}},
		{name: "aws_unsupported_web_search", run: claudeToolPolicyCase{
			channel: channeltype.AwsClaude, actual: "claude-sonnet-4-5", contentType: "application/json", response: nativeOK,
			payload: claudeToolPolicyPayload(t, false, []any{claudeToolPolicyWebSearch()}, nil),
			config:  &model.ChannelConfig{Region: "us-east-1", AK: "synthetic-access", SK: "synthetic-secret"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runClaudeToolPolicyCase(t, balance, tc.run)
			if result.calls != 0 {
				t.Logf("REPRODUCED_464_DISALLOWED_TOOL_DISPATCH case=%s calls=%d", tc.name, result.calls)
			}
			requireClaudeToolPolicyUntouched(t, result, balance)
		})
	}
	// Ambiguous spellings: a case-insensitive, last-wins decoder sees the trailing
	// empty/null variant, while an exact-key or first-wins upstream sees the paid
	// capability that passthrough forwards verbatim.
	searchNotAllowed := &model.ChannelToolingConfig{Whitelist: []string{"tool_search"}, Pricing: map[string]model.ToolPricingLocal{"web_search": {QuotaPerCall: claudeToolPolicyPerSearch}}}
	webSearch := `{"type":"web_search_20250305","name":"web_search"}`
	mcpServers := `[{"type":"url","url":"https://mcp.example.invalid/sse","name":"remote"}]`
	for _, tc := range []struct {
		name   string
		fields string
	}{
		{name: "tools_upper_case_variant", fields: `"tools":[` + webSearch + `],"TOOLS":null`},
		{name: "tools_title_case_variant", fields: `"tools":[` + webSearch + `],"Tools":[]`},
		{name: "tools_unicode_fold_variant", fields: `"tools":[` + webSearch + `],"tool\u017f":null`},
		{name: "tools_exact_duplicate", fields: `"tools":[` + webSearch + `],"tools":[]`},
		{name: "mcp_servers_case_variant", fields: `"mcp_servers":` + mcpServers + `,"MCP_SERVERS":null`},
		{name: "mcp_servers_unicode_fold_variant", fields: `"mcp_servers":` + mcpServers + `,"mcp_ſerverſ":[]`},
		{name: "mcp_servers_exact_duplicate", fields: `"mcp_servers":` + mcpServers + `,"mcp_servers":[]`},
		{name: "container_case_variant", fields: `"container":"container_synthetic","Container":null`},
		{name: "container_exact_duplicate", fields: `"container":"container_synthetic","container":null`},
		{name: "tool_type_exact_duplicate", fields: `"tools":[{"type":"web_search_20250305","type":"custom","name":"web_search"}]`},
		{name: "tool_type_case_variant", fields: `"tools":[{"TYPE":"web_search_20250305","name":"web_search"}]`},
	} {
		t.Run("ambiguous_"+tc.name, func(t *testing.T) {
			result := runClaudeToolPolicyCase(t, balance, claudeToolPolicyCase{
				channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "application/json", response: nativeOK,
				payload: `{"model":"alias","max_tokens":64,"messages":[{"role":"user","content":"hello"}],` + tc.fields + `}`,
				tooling: searchNotAllowed,
			})
			if result.calls != 0 {
				t.Logf("REPRODUCED_464_AMBIGUOUS_FIELD_DISPATCH case=%s calls=%d", tc.name, result.calls)
			}
			requireClaudeToolPolicyUntouched(t, result, balance)
		})
	}
}

// TestSecurityClaudeNativeToolPolicyBilling proves allowed server-tool receipts
// settle at the configured tariff exactly once, including missing, partial and
// duplicate receipts and a stale counter left by an abandoned attempt.
func TestSecurityClaudeNativeToolPolicyBilling(t *testing.T) {
	const balance = int64(100_000)
	const tokens = int64(18)
	twoSearches := tokens + 2*claudeToolPolicyPerSearch
	for _, tc := range []struct {
		name   string
		run    claudeToolPolicyCase
		charge int64
	}{
		{name: "native_json_receipt", charge: twoSearches, run: claudeToolPolicyCase{
			channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "application/json",
			response: fmt.Sprintf(claudeToolPolicyNativeJSON, `,"server_tool_use":{"web_search_requests":2}`),
			payload:  claudeToolPolicyPayload(t, false, []any{claudeToolPolicyWebSearch()}, nil), tooling: claudeToolPolicyAllowSearch(),
		}},
		{name: "native_json_missing_receipt_counts_blocks", charge: twoSearches, run: claudeToolPolicyCase{
			channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "application/json",
			response: fmt.Sprintf(claudeToolPolicyNativeJSON, ""),
			payload:  claudeToolPolicyPayload(t, false, []any{claudeToolPolicyWebSearch()}, nil), tooling: claudeToolPolicyAllowSearch(),
		}},
		{name: "native_sse_cumulative_and_duplicate_receipts", charge: twoSearches, run: claudeToolPolicyCase{
			channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "text/event-stream", response: claudeToolPolicySSEComplete,
			payload: claudeToolPolicyPayload(t, true, []any{claudeToolPolicyWebSearch()}, nil), tooling: claudeToolPolicyAllowSearch(),
		}},
		{name: "native_json_stale_attempt_counters", charge: twoSearches, run: claudeToolPolicyCase{
			channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "application/json",
			response: fmt.Sprintf(claudeToolPolicyNativeJSON, `,"server_tool_use":{"web_search_requests":2}`),
			payload:  claudeToolPolicyPayload(t, false, []any{claudeToolPolicyWebSearch()}, nil), tooling: claudeToolPolicyAllowSearch(),
			prepare: func(c *gin.Context) {
				c.Set(ctxkey.ToolInvocationCounts, map[string]int{"web_search": 5})
				c.Set(ctxkey.WebSearchCallCount, 5)
			},
		}},
		{name: "native_provider_default_tariff", charge: tokens + 5000, run: claudeToolPolicyCase{
			channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "application/json",
			response: strings.Replace(fmt.Sprintf(claudeToolPolicyNativeJSON, `,"server_tool_use":{"web_search_requests":1}`), `"name":"web_search","input":{"query":"b"}`, `"name":"web_fetch","input":{"url":"https://example.invalid"}`, 1),
			payload:  claudeToolPolicyPayload(t, false, []any{map[string]any{"type": "web_search_20260209", "name": "web_search", "max_uses": 1}, map[string]any{"type": "web_fetch_20250910", "name": "web_fetch"}, map[string]any{"type": "tool_search_tool_bm25_20251119", "name": "tool_search_tool_bm25"}}, nil),
		}},
		{name: "converted_responses_receipt", charge: twoSearches, run: claudeToolPolicyCase{
			channel: channeltype.OpenAICompatible, actual: "claude-sonnet-4", contentType: "application/json", response: claudeToolPolicyResponsesJSON,
			payload: claudeToolPolicyPayload(t, false, []any{map[string]any{"type": "web_search", "name": "web_search"}}, nil), tooling: claudeToolPolicyAllowSearch(),
			config: &model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse},
		}},
		{name: "converted_responses_sse_receipt", charge: twoSearches, run: claudeToolPolicyCase{
			channel: channeltype.OpenAICompatible, actual: "claude-sonnet-4", contentType: "text/event-stream", response: claudeToolPolicyResponsesSSE,
			payload: claudeToolPolicyPayload(t, true, []any{map[string]any{"type": "web_search", "name": "web_search"}}, nil), tooling: claudeToolPolicyAllowSearch(),
			config: &model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runClaudeToolPolicyCase(t, balance, tc.run)
			require.Nil(t, result.apiErr, fmt.Sprint(result.apiErr))
			require.EqualValues(t, 1, result.calls)
			if actual := requestCostQuota(t, result.id); actual != tc.charge {
				t.Logf("REPRODUCED_464_TOOL_RECEIPT_LEDGER case=%s actual=%d expected=%d", tc.name, actual, tc.charge)
			}
			requireClaudeToolPolicyLedger(t, result, balance, tc.charge)
		})
	}
}

// TestSecurityClaudeNativeToolPolicyMissingStreamReceipt proves an accepted
// stream that ends before any final receipt keeps the paid-tool allowance.
func TestSecurityClaudeNativeToolPolicyMissingStreamReceipt(t *testing.T) {
	const balance = int64(100_000)
	result := runClaudeToolPolicyCase(t, balance, claudeToolPolicyCase{
		channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "text/event-stream", response: claudeToolPolicySSEPrefix,
		payload: claudeToolPolicyPayload(t, true, []any{claudeToolPolicyWebSearch()}, nil), tooling: claudeToolPolicyAllowSearch(),
	})
	require.NotNil(t, result.apiErr, "a truncated stream is an upstream failure")
	require.EqualValues(t, 1, result.calls)
	meta := metalib.GetByContext(result.c)
	require.NotNil(t, meta)
	// Prompt quote + max_tokens + max_uses(3) x 17 for the admitted paid search.
	expected := int64(meta.PromptTokens) + 64 + 3*claudeToolPolicyPerSearch
	if actual := requestCostQuota(t, result.id); actual != expected {
		t.Logf("REPRODUCED_464_MISSING_TOOL_RECEIPT actual=%d expected=%d", actual, expected)
	}
	requireClaudeToolPolicyLedger(t, result, balance, expected)
}

// TestSecurityClaudeNativeToolPolicyCompatibility proves ordinary tools, client
// tools, free capabilities and caller beta/version headers keep working.
func TestSecurityClaudeNativeToolPolicyCompatibility(t *testing.T) {
	const balance = int64(100_000)
	ordinary := []any{
		map[string]any{"name": "lookup", "description": "Look up a record.", "input_schema": map[string]any{"type": "object"}},
		map[string]any{"type": "custom", "name": "lookup_custom", "input_schema": map[string]any{"type": "object"}},
		map[string]any{"type": "bash_20250124", "name": "bash"},
		map[string]any{"type": "text_editor_20250728", "name": "str_replace_based_edit_tool"},
		map[string]any{"type": "memory_20250818", "name": "memory"},
		map[string]any{"type": "computer_20251124", "name": "computer", "display_width_px": 1024, "display_height_px": 768},
	}
	t.Run("native_client_and_custom_tools", func(t *testing.T) {
		result := runClaudeToolPolicyCase(t, balance, claudeToolPolicyCase{
			channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "application/json",
			response: `{"id":"msg_plain","type":"message","role":"assistant","model":"claude-sonnet-4","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`,
			payload:  claudeToolPolicyPayload(t, false, ordinary, nil), beta: "computer-use-2025-11-24,context-management-2025-06-27",
			prepare: func(c *gin.Context) { c.Request.Header.Set("anthropic-version", "2023-06-01") },
		})
		require.Nil(t, result.apiErr, fmt.Sprint(result.apiErr))
		require.EqualValues(t, 1, result.calls)
		require.Len(t, result.bodies, 1)
		tools, ok := result.bodies[0]["tools"].([]any)
		require.True(t, ok)
		require.Len(t, tools, len(ordinary), "client and custom tools are forwarded verbatim")
		require.Len(t, result.headers, 1)
		require.Contains(t, result.headers[0].Get("anthropic-beta"), "computer-use-2025-11-24")
		require.Contains(t, result.headers[0].Get("anthropic-beta"), "context-management-2025-06-27")
		require.Equal(t, "2023-06-01", result.headers[0].Get("anthropic-version"))
		requireClaudeToolPolicyLedger(t, result, balance, 18)
	})
	t.Run("converted_function_tool", func(t *testing.T) {
		result := runClaudeToolPolicyCase(t, balance, claudeToolPolicyCase{
			channel: channeltype.OpenAI, actual: "claude-sonnet-4", contentType: "application/json", response: claudeToolPolicyChatJSON,
			payload: claudeToolPolicyPayload(t, false, ordinary[:1], nil),
		})
		require.Nil(t, result.apiErr, fmt.Sprint(result.apiErr))
		require.EqualValues(t, 1, result.calls)
		requireClaudeToolPolicyLedger(t, result, balance, 18)
	})
	t.Run("native_mcp_connector_explicitly_allowed", func(t *testing.T) {
		result := runClaudeToolPolicyCase(t, balance, claudeToolPolicyCase{
			channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "application/json", beta: "mcp-client-2025-11-20",
			response: `{"id":"msg_mcp","type":"message","role":"assistant","model":"claude-sonnet-4","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`,
			payload: claudeToolPolicyPayload(t, false, []any{map[string]any{"type": "mcp_toolset", "mcp_server_name": "remote"}},
				map[string]any{"mcp_servers": []any{map[string]any{"type": "url", "url": "https://mcp.example.invalid/sse", "name": "remote"}}}),
			tooling: &model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{"mcp_connector": {}}},
		})
		require.Nil(t, result.apiErr, fmt.Sprint(result.apiErr))
		require.EqualValues(t, 1, result.calls)
		require.Contains(t, result.bodies[0], "mcp_servers", "an operator-approved connector is forwarded unchanged")
		requireClaudeToolPolicyLedger(t, result, balance, 18)
	})
}

// TestSecurityClaudeNativeToolPolicyTrustedToolSearchLoop proves trusted MCP
// Tool Search injection still works and its native rounds bill search receipts.
func TestSecurityClaudeNativeToolPolicyTrustedToolSearchLoop(t *testing.T) {
	const balance = int64(100_000)
	result := runClaudeToolPolicyCase(t, balance, claudeToolPolicyCase{
		channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "application/json",
		response: `{"id":"msg_loop","type":"message","role":"assistant","model":"claude-sonnet-4","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7,"server_tool_use":{"web_search_requests":2}}}`,
		payload: claudeToolPolicyPayload(t, false, []any{
			map[string]any{"type": "tool_search_tool_regex_20251119", "name": "tool_search_tool_regex"},
			claudeToolPolicyWebSearch(),
		}, nil),
		tooling: claudeToolPolicyAllowSearch(),
		prepare: func(c *gin.Context) {
			stored := &model.MCPServer{Name: "tool-policy-loop", Status: model.MCPServerStatusEnabled, BaseURL: "https://example.com/synthetic-unused-mcp"}
			require.NoError(t, model.DB.Create(stored).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Delete(stored).Error) })
			storedTool := &model.MCPTool{ServerId: stored.Id, Name: "lookup_record", Description: "Catalog lookup", InputSchema: `{"type":"object","properties":{"query":{"type":"string"}}}`}
			require.NoError(t, model.DB.Create(storedTool).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Delete(storedTool).Error) })
		},
	})
	require.Nil(t, result.apiErr, fmt.Sprint(result.apiErr))
	require.EqualValues(t, 1, result.calls)
	require.Len(t, result.bodies, 1)
	encoded, err := json.Marshal(result.bodies[0]["tools"])
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"defer_loading":true`, "trusted catalog tools are still injected as deferred tools")
	require.Contains(t, string(encoded), "web_search_20250305")
	charge := int64(18) + 2*claudeToolPolicyPerSearch
	if actual := requestCostQuota(t, result.id); actual != charge {
		t.Logf("REPRODUCED_464_TOOL_SEARCH_LOOP_RECEIPT actual=%d expected=%d", actual, charge)
	}
	requireClaudeToolPolicyLedger(t, result, balance, charge)
}

// TestSecurityClaudeNativeToolPolicyRetryReset proves a cross-channel retry
// cannot bill the abandoned attempt's built-in tool receipts a second time.
func TestSecurityClaudeNativeToolPolicyRetryReset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}"))
	gmw.SetLogger(c, logger.Logger)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.PreConsumedQuotaAmount, int64(0))
	c.Set(ctxkey.ToolInvocationCounts, map[string]int{"web_search": 3})
	c.Set(ctxkey.WebSearchCallCount, 3)
	c.Set(ctxkey.ToolInvocationSummary, &model.ToolUsageSummary{Counts: map[string]int{"web_search": 3}})
	ResetPerAttemptBillingForRetry(gmw.Ctx(c), c)
	drainCriticalTasks(t)
	counts, ok := c.Get(ctxkey.ToolInvocationCounts)
	require.True(t, ok)
	require.Empty(t, counts)
	require.Zero(t, c.GetInt(ctxkey.WebSearchCallCount))
	summary, ok := c.Get(ctxkey.ToolInvocationSummary)
	require.True(t, ok)
	require.Nil(t, summary.(*model.ToolUsageSummary))
}
