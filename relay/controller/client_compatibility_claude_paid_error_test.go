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
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/mcp"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

type clientCompatibilityClaudePaidErrorResult struct {
	err                                                                       *relaymodel.ErrorWithStatusCode
	providerCalls                                                             int32
	toolNames                                                                 []string
	toolArguments                                                             []json.RawMessage
	bodies                                                                    [][]byte
	holds                                                                     []int64
	cost, debit, tokenDebit, tokenUsed                                        int64
	afterRepeatDebit, afterRepeatTokenDebit, afterRepeatCost, afterRepeatLogs int64
	retryAllowed                                                              bool
	body                                                                      string
	logs                                                                      []model.Log
}

// clientCompatibilityClaudePaidErrorLoop runs native rounds and local MCP transport, returning observed ledger and retry state.
func clientCompatibilityClaudePaidErrorLoop(t *testing.T, turns []string) clientCompatibilityClaudePaidErrorResult {
	t.Helper()
	const balance = int64(1_000_000)
	xaiVideoSetup(t, balance, false)
	toolNames := make(chan string, 16)
	toolArguments := make(chan json.RawMessage, 16)
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rpc struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&rpc); err != nil {
			http.Error(w, "invalid local MCP fixture", http.StatusBadRequest)
			return
		}
		if strings.HasPrefix(rpc.Method, "notifications/") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch rpc.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "client-compatibility-fixture", "version": "1"}}
		case "tools/call":
			select {
			case toolNames <- rpc.Params.Name:
			default:
			}
			select {
			case toolArguments <- append(json.RawMessage(nil), rpc.Params.Arguments...):
			default:
			}
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "result:" + rpc.Params.Name}}}
		default:
			http.Error(w, "unexpected local MCP method", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": result})
	}))
	t.Cleanup(mcpServer.Close)
	stored := &model.MCPServer{Name: "client-compatibility-loop", Status: model.MCPServerStatusEnabled, BaseURL: mcpServer.URL}
	require.NoError(t, model.DB.Create(stored).Error)
	t.Cleanup(func() { require.NoError(t, model.DB.Delete(stored).Error) })
	registry := &claudeToolSearchMCPRegistry{candidatesByName: map[string][]mcp.ToolCandidate{}, requestHeaders: map[string]map[string]string{}, selectedIndex: map[string]int{}}
	for _, name := range []string{"probe_a", "probe_b"} {
		tool := &model.MCPTool{ServerId: stored.Id, Name: name, InputSchema: `{"type":"object","properties":{}}`}
		require.NoError(t, model.DB.Create(tool).Error)
		t.Cleanup(func() { require.NoError(t, model.DB.Delete(tool).Error) })
		registry.candidatesByName[name] = []mcp.ToolCandidate{{ResolvedTool: mcp.ResolvedTool{Tool: tool, ServerID: stored.Id, ServerLabel: stored.Name, ServerURL: mcpServer.URL, Policy: mcp.ToolPolicySnapshot{Allowed: true}}}}
	}
	var providerCalls atomic.Int32
	heldBeforeReply := make(chan int64, 16)
	observationsFailed := make(chan error, 16)
	bodies := make(chan []byte, 16)
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "invalid local provider body", http.StatusBadRequest)
			return
		}
		select {
		case bodies <- body:
		default:
		}
		round := int(providerCalls.Add(1)) - 1
		var observedUser model.User
		if err := model.DB.First(&observedUser, fallbackUserID).Error; err != nil {
			observationsFailed <- err
		} else {
			heldBeforeReply <- balance - observedUser.Quota
		}
		w.Header().Set("Content-Type", "application/json")
		if round >= len(turns) {
			_, _ = io.WriteString(w, `{"id":"unexpected-extra-round","type":"message","role":"assistant","content":[{"type":"text","text":"unexpected extra round"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
			return
		}
		_, _ = io.WriteString(w, turns[round])
	}))
	t.Cleanup(provider.Close)
	previous := client.HTTPClient
	client.HTTPClient = provider.Client()
	t.Cleanup(func() { client.HTTPClient = previous })
	payload := clientCompatibilityClaudePayload
	fullController := true
	if fullController {
		// Caller-owned names deliberately do not collide with the gateway
		// catalog: injection must own probe_a/probe_b for this ledger fixture.
		var root map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(payload), &root))
		root["tools"] = json.RawMessage(`[{"type":"tool_search_tool_regex_20251119","name":"tool_search_tool_regex"}]`)
		encoded, encodeErr := json.Marshal(root)
		require.NoError(t, encodeErr)
		payload = string(encoded)
	}
	c, recorder, requestID := protocolContext(t, channeltype.Anthropic, "claude-sonnet-4", "/v1/messages", payload, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	meta := metalib.GetByContext(c)
	adapter := &anthropic.Adaptor{}
	adapter.Init(meta)
	var request ClaudeMessagesRequest
	require.NoError(t, json.Unmarshal([]byte(payload), &request))
	_, err := adapter.ConvertClaudeRequest(c, &request)
	require.NoError(t, err)
	var response *anthropic.Response
	var apiErr *relaymodel.ErrorWithStatusCode
	if fullController {
		responseNamespaceCleanLedger(t, requestID)
		apiErr = RelayClaudeMessagesHelper(c)
		if apiErr == nil {
			response = &anthropic.Response{}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), response))
		}
	} else {
		response, _, _, _, apiErr = executeClaudeToolSearchMCPLoop(c, meta, &request, registry, adapter, 0)
	}
	drainCriticalTasks(t)
	result := clientCompatibilityClaudePaidErrorResult{err: apiErr, providerCalls: providerCalls.Load(), retryAllowed: BillingAllowsRetry(c), body: recorder.Body.String()}
	for len(toolNames) > 0 {
		result.toolNames = append(result.toolNames, <-toolNames)
	}
	for len(toolArguments) > 0 {
		result.toolArguments = append(result.toolArguments, <-toolArguments)
	}
	if fullController {
		result.cost = requestCostQuota(t, requestID)
	}
	for len(bodies) > 0 {
		result.bodies = append(result.bodies, <-bodies)
	}

	for len(observationsFailed) > 0 {
		require.NoError(t, <-observationsFailed)
	}
	for len(heldBeforeReply) > 0 {
		result.holds = append(result.holds, <-heldBeforeReply)
	}
	var user model.User
	var token model.Token
	require.NoError(t, model.DB.First(&user, fallbackUserID).Error)
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	result.debit = balance - user.Quota
	result.tokenDebit = balance - token.RemainQuota
	result.tokenUsed = token.UsedQuota
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Find(&result.logs).Error)
	// Calling all terminal safeguards twice must not create new credit or ledger rows.
	if apiErr != nil && !anthropic.IsAdmissionRejection(apiErr) {
		for i := 0; i < 2; i++ {
			billingAuditSafetyNet(c)
			ResetPerAttemptBillingForRetry(c.Request.Context(), c)
			require.NoError(t, recordZeroCostAfterFailure(c, fallbackUserID, requestID))
		}
	}
	drainCriticalTasks(t)
	require.NoError(t, model.DB.First(&user, fallbackUserID).Error)
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	result.afterRepeatDebit = balance - user.Quota
	result.afterRepeatTokenDebit = balance - token.RemainQuota
	result.afterRepeatCost = requestCostQuota(t, requestID)
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ?", requestID).Count(&result.afterRepeatLogs).Error)
	return result
}

// TestClientCompatibilityClaudePaidErrorsRetainLedger drives actual provider/MCP
// rounds and SQLite balances. Invalid accepted receipts must not refund paid work.
func TestClientCompatibilityClaudePaidErrorsRetainLedger(t *testing.T) {
	local := `[{"type":"tool_use","id":"call_a","name":"probe_a","input":{}}]`
	first := clientCompatibilityClaudeTurn(local, "tool_use")
	final := clientCompatibilityClaudeTurn(`[{"type":"text","text":"done"}]`, "end_turn")
	invalid := strings.Replace(final, `"input_tokens":11`, `"input_tokens":-1`, 1)
	overflowFirst := strings.Replace(first, `"input_tokens":11`, `"cache_creation_input_tokens":1,"cache_creation":{"ephemeral_1h_input_tokens":1},"input_tokens":11`, 1)
	// Each round is representable independently; only their cache sum overflows.
	overflowLast := strings.Replace(final, `"input_tokens":11,"output_tokens":7`, fmt.Sprintf(`"cache_creation_input_tokens":%d,"cache_creation":{"ephemeral_1h_input_tokens":%d},"input_tokens":0,"output_tokens":0`, int(^uint(0)>>1), int(^uint(0)>>1)), 1)
	for _, body := range []string{overflowFirst, overflowLast} {
		_, receiptErr := anthropic.ParseClaudeMessageUsage([]byte(body))
		require.NoError(t, receiptErr, "merge-overflow fixture must be independently valid")
	}
	for _, tc := range []struct {
		name  string
		turns []string
		code  string
		tools int
	}{
		{"invalid_first", []string{invalid}, "invalid_claude_mcp_usage", 0},
		{"invalid_after_accepted_round", []string{first, invalid}, "invalid_claude_mcp_usage", 1},
		{"merge_overflow_after_accepted_round", []string{overflowFirst, overflowLast}, "invalid_mcp_usage", 1},
		// A status/envelope alone is not the private no-work proof used by admission refunds.
		{"unknown_error_without_no_work_proof", []string{`{"type":"error","error":{"type":"future_error","message":"synthetic uncertain failure"}}`}, "invalid_claude_mcp_usage", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := clientCompatibilityClaudePaidErrorLoop(t, tc.turns)
			require.NotNil(t, result.err)
			require.Equal(t, tc.code, result.err.Error.Code)
			require.EqualValues(t, len(tc.turns), result.providerCalls)
			require.Len(t, result.toolNames, tc.tools)
			require.Len(t, result.holds, len(tc.turns))
			held := int64(0)
			for _, amount := range result.holds {
				held = max(held, amount)
			}
			require.Positive(t, held, "test must exercise actual initial/incremental debits")
			require.Equal(t, held, result.debit, "unknown receipt keeps established holds without inventing usage")
			require.Equal(t, result.debit, result.tokenDebit)
			require.Equal(t, result.debit, result.tokenUsed)
			require.Equal(t, result.debit, result.cost)
			require.False(t, result.retryAllowed, "accepted work must not be replayed after settlement")
			require.Empty(t, result.body, "failed continuation must not render successful content")
			require.Len(t, result.logs, 1, "reconcile one provisional receipt rather than duplicate settlement")
			require.Equal(t, model.LogTypeConsume, result.logs[0].Type)
			require.EqualValues(t, result.cost, result.logs[0].Quota)
			require.Equal(t, true, result.logs[0].Metadata["billing_estimated"])
			require.NotEmpty(t, result.logs[0].Metadata["billing_estimate_reason"])
			require.Equal(t, result.debit, result.afterRepeatDebit)
			require.Equal(t, result.tokenDebit, result.afterRepeatTokenDebit)
			require.Equal(t, result.cost, result.afterRepeatCost)
			require.EqualValues(t, 1, result.afterRepeatLogs)
		})
	}
}

// TestClientCompatibilityClaudePaidErrorMeasuredControl requires a valid two-round
// receipt to retain existing exact settlement, rather than applying an error hold policy.
func TestClientCompatibilityClaudePaidErrorMeasuredControl(t *testing.T) {
	first := clientCompatibilityClaudeTurn(`[{"type":"tool_use","id":"call_a","name":"probe_a","input":{}}]`, "tool_use")
	final := clientCompatibilityClaudeTurn(`[{"type":"text","text":"done"}]`, "end_turn")
	result := clientCompatibilityClaudePaidErrorLoop(t, []string{first, final})
	require.Nil(t, result.err)
	require.EqualValues(t, 2, result.providerCalls)
	require.Equal(t, []string{"probe_a"}, result.toolNames)
	require.EqualValues(t, 36, result.cost)
	require.Equal(t, result.cost, result.debit)
	require.Equal(t, result.cost, result.tokenDebit)
	require.Len(t, result.logs, 1)
	require.NotEqual(t, true, result.logs[0].Metadata["billing_estimated"])
}

// TestClientCompatibilityClaudeMCPNoWorkRefundControl proves a complete native
// admission rejection refunds initial and round holds without invoking a tool.
func TestClientCompatibilityClaudeMCPNoWorkRefundControl(t *testing.T) {
	result := clientCompatibilityClaudePaidErrorLoop(t, []string{`{"type":"error","error":{"type":"invalid_request_error","message":"synthetic pre-inference rejection"}}`})
	require.NotNil(t, result.err)
	require.True(t, anthropic.IsAdmissionRejection(result.err))
	require.Equal(t, http.StatusBadRequest, result.err.StatusCode)
	require.EqualValues(t, 1, result.providerCalls)
	require.Empty(t, result.toolNames)
	require.Positive(t, result.holds[0], "must exercise actual reserved quota")
	require.Zero(t, result.debit)
	require.Zero(t, result.tokenDebit)
	require.Zero(t, result.tokenUsed)
	require.Zero(t, result.cost)
	require.True(t, result.retryAllowed, "private no-work proof remains eligible for safe retry")
	require.Len(t, result.logs, 1)
	require.Zero(t, result.logs[0].Quota)
}
