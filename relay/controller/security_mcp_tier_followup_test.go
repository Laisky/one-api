package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/mcp"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	quotautil "github.com/Laisky/one-api/relay/quota"
	"github.com/stretchr/testify/require"
)

// mcpTierRoundObservation records durable balances and the provider receipt
// for one bounded local Claude round without assertions in the HTTP goroutine.
type mcpTierRoundObservation struct {
	Round, Prompt, Output int
	User, Token           int64
	Err                   error
}

// TestSecurityMCPFollowupTierReservation isolates follow-up admission from the
// initial issue by reserving the correct first-round tier tariff before two local
// provider rounds and one local MCP call. Parameters: t owns disposable SQLite
// and loopback fixtures. Returns: none; owner and token admission reject an
// unaffordable second round, funded rounds settle once, and explicit errors refund.
func TestSecurityMCPFollowupTierReservation(t *testing.T) {
	for _, protocol := range []string{"messages", "chat"} {
		for _, scenario := range []string{"owner_low", "token_low", "funded", "refund_second", "cross_owner_low", "cross_token_low", "cross_funded", "cross_refund_second", "injected_funded"} {
			t.Run(protocol+"/"+scenario, func(t *testing.T) {
				runMCPTierFollowupFixture(t, protocol, scenario)
			})
		}
	}
}

// runMCPTierFollowupFixture executes one bounded tier-admission scenario through
// the real MCP loop and durable settlement. Parameters: t owns the fixtures,
// protocol selects the native or Chat loop, and scenario selects balances or an
// explicit second-round provider rejection. Returns: none; assertions inspect
// actual dispatch holds, final balances, and the single consumption ledger row.
func runMCPTierFollowupFixture(t *testing.T, protocol, scenario string) {
	t.Helper()
	const (
		name   = "claude-haiku-5-5"
		output = 1000
	)
	userBalance, tokenBalance := int64(80000), int64(80000)
	if scenario == "owner_low" {
		userBalance = 40000
	}
	if scenario == "token_low" {
		tokenBalance = 40000
	}
	words := 56000
	crossThreshold := strings.HasPrefix(scenario, "cross_")
	if crossThreshold {
		words = 30000
		if scenario == "cross_owner_low" {
			userBalance = 10000
		}
		if scenario == "cross_token_low" {
			tokenBalance = 10000
		}
	}
	xaiVideoSetup(t, userBalance, false)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).Update("remain_quota", tokenBalance).Error)
	priorRounds := config.MCPMaxToolRounds
	config.MCPMaxToolRounds = 2
	t.Cleanup(func() { config.MCPMaxToolRounds = priorRounds })

	var toolCalls atomic.Int32
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rpc struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&rpc); err != nil {
			http.Error(w, "bad fixture request", http.StatusBadRequest)
			return
		}
		if strings.HasPrefix(rpc.Method, "notifications/") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch rpc.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "bounded-tier-probe", "version": "1"}}
		case "tools/call":
			toolCalls.Add(1)
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "local tool answer"}}}
		default:
			http.Error(w, "unexpected fixture method", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": result}); err != nil {
			return
		}
	}))
	t.Cleanup(mcpServer.Close)
	stored := &model.MCPServer{Name: "mcp-tier-followup-fixture", Status: model.MCPServerStatusEnabled, BaseURL: mcpServer.URL}
	require.NoError(t, model.DB.Create(stored).Error)
	t.Cleanup(func() { require.NoError(t, model.DB.Delete(stored).Error) })
	tool := &model.MCPTool{ServerId: stored.Id, Name: "probe", InputSchema: `{"type":"object","properties":{}}`}
	require.NoError(t, model.DB.Create(tool).Error)
	t.Cleanup(func() { require.NoError(t, model.DB.Delete(tool).Error) })
	candidates := map[string][]mcp.ToolCandidate{"probe": {{ResolvedTool: mcp.ResolvedTool{
		Tool: tool, ServerID: stored.Id, ServerLabel: stored.Name, ServerURL: mcpServer.URL,
		Policy: mcp.ToolPolicySnapshot{Allowed: true},
	}}}}

	var providerCalls atomic.Int32
	observed := make(chan mcpTierRoundObservation, 2)
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		round := int(providerCalls.Add(1))
		var request ClaudeMessagesRequest
		got := mcpTierRoundObservation{Round: round, Output: output}
		got.Err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request)
		if got.Err == nil {
			got.Prompt, got.Err = getClaudeMessagesPromptTokens(r.Context(), &request)
		}
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
		if got.Err != nil || round > 2 || request.MaxTokens != output {
			http.Error(w, "invalid bounded fixture request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(scenario, "refund_second") && round == 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"overloaded_error","message":"local explicit rejection"}}`)
			return
		}
		content := `[{"type":"text","text":"done"}]`
		stop := "end_turn"
		if round == 1 {
			content = `[{"type":"tool_use","id":"call_probe","name":"probe","input":{}}]`
			stop = "tool_use"
		}
		if _, err := fmt.Fprintf(w, `{"id":"bounded-turn-%d","type":"message","role":"assistant","model":"%s","content":%s,"stop_reason":"%s","usage":{"input_tokens":%d,"output_tokens":%d}}`, round, name, content, stop, got.Prompt, output); err != nil {
			return
		}
	}))
	t.Cleanup(provider.Close)
	priorClient := client.HTTPClient
	client.HTTPClient = provider.Client()
	t.Cleanup(func() { client.HTTPClient = priorClient })

	claudeRequest := &ClaudeMessagesRequest{Model: name, MaxTokens: output, Messages: []relaymodel.ClaudeMessage{{
		Role: "user", Content: strings.Repeat("test ", words),
	}}}
	chatRequest := &relaymodel.GeneralOpenAIRequest{Model: name, MaxTokens: output, Messages: []relaymodel.Message{{
		Role: "user", Content: strings.Repeat("test ", words),
	}}}
	canonical := any(claudeRequest)
	path := "/v1/messages"
	if protocol == "chat" {
		canonical, path = chatRequest, "/v1/chat/completions"
	}
	body, err := json.Marshal(canonical)
	require.NoError(t, err)
	c, _, requestID := protocolContext(t, channeltype.Anthropic, name, path, string(body), provider.URL, userBalance, 1, false, nil)
	c.Set(ctxkey.TokenQuota, tokenBalance)
	meta := metalib.GetByContext(c)
	meta.OriginModelName, meta.ActualModelName = name, name
	c.Set(ctxkey.Meta, meta)
	providerAdaptor := &anthropic.Adaptor{}
	providerAdaptor.Init(meta)
	firstRequest := claudeRequest
	if protocol == "chat" {
		converted, convertErr := providerAdaptor.ConvertRequest(c, meta.Mode, chatRequest)
		require.NoError(t, convertErr)
		prepared, marshalErr := json.Marshal(converted)
		require.NoError(t, marshalErr)
		firstRequest = &ClaudeMessagesRequest{}
		require.NoError(t, json.Unmarshal(prepared, firstRequest))
	}
	prompt, err := getClaudeMessagesPromptTokens(context.Background(), firstRequest)
	require.NoError(t, err)
	meta.PromptTokens = prompt
	firstQuote := quotautil.Compute(quotautil.ComputeInput{
		Usage:     &relaymodel.Usage{PromptTokens: prompt, CompletionTokens: output},
		ModelName: name, ModelRatio: .05, GroupRatio: 1, PricingAdaptor: providerAdaptor, RequestTime: meta.StartTime,
	}).TotalQuota
	if crossThreshold {
		require.Less(t, prompt, 100000)
		require.Greater(t, prompt*2, 100000)
		require.Less(t, firstQuote, int64(4000))
	} else {
		require.Greater(t, prompt, 100000)
		require.Greater(t, firstQuote, int64(26000))
		require.Less(t, firstQuote, int64(30000))
	}
	// Reserve the complete tier charge directly, independently of the initial
	// admission estimator, so this fixture isolates additional MCP round funding.
	held, admissionErr := reservePaidRequestQuota(c, meta, firstQuote, "mcp_followup_correct_first_tier")
	require.Nil(t, admissionErr)
	require.Equal(t, firstQuote, held)
	markPreConsumed(c, held)
	provisional := recordProvisionalLog(c, meta, name, held)
	c.Set(ctxkey.ProvisionalLogId, provisional)

	if scenario == "injected_funded" {
		// A gateway-selected tool can be attached after initial admission.
		// Only its added prepared input requires a first-round top-up.
		description := strings.Repeat("local injected schema description ", 200)
		schema := map[string]any{"type": "object", "properties": map[string]any{}}
		claudeRequest.Tools = []relaymodel.ClaudeTool{{Name: "probe", Description: description, InputSchema: schema}}
		chatRequest.Tools = []relaymodel.Tool{{Type: "function", Function: &relaymodel.Function{Name: "probe", Description: description, Parameters: schema}}}
	}

	var usage *relaymodel.Usage
	var incremental int64
	var apiErr *relaymodel.ErrorWithStatusCode
	if protocol == "chat" {
		registry := &mcpToolRegistry{candidatesByName: candidates, requestHeaders: map[string]map[string]string{}, selectedIndex: map[string]int{}}
		_, usage, _, incremental, apiErr = executeChatMCPToolLoop(c, meta, chatRequest, registry, held)
	} else {
		registry := &claudeToolSearchMCPRegistry{candidatesByName: candidates, requestHeaders: map[string]map[string]string{}, selectedIndex: map[string]int{}}
		_, usage, _, incremental, apiErr = executeClaudeToolSearchMCPLoop(c, meta, claudeRequest, registry, providerAdaptor, held)
	}
	require.NotNil(t, usage)
	require.EqualValues(t, 1, toolCalls.Load())
	require.LessOrEqual(t, providerCalls.Load(), int32(2))
	rounds := make([]mcpTierRoundObservation, 0, 2)
	for range providerCalls.Load() {
		got := <-observed
		require.NoError(t, got.Err)
		rounds = append(rounds, got)
		t.Logf("MCP_TIER_ROUND protocol=%s round=%d prompt=%d held_user=%d held_token=%d remaining_user=%d remaining_token=%d", protocol, got.Round, got.Prompt, userBalance-got.User, tokenBalance-got.Token, got.User, got.Token)
	}
	var total int64
	if protocol == "chat" {
		total = postConsumeQuota(gmw.Ctx(c), usage, meta, chatRequest, .05, held, incremental, .05, nil, 1, false, nil, nil)
	} else {
		total = postConsumeClaudeMessagesQuotaWithTraceID(gmw.Ctx(c), requestID, "", usage, meta, claudeRequest, .05, held, incremental, .05, nil, 1, nil, nil)
	}
	drainCriticalTasks(t)
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	t.Logf("MCP_TIER_FINAL protocol=%s scenario=%s first_quote=%d incremental_holds=%d provider_calls=%d tool_calls=%d charge=%d user_balance=%d token_balance=%d final_user=%d final_token=%d api_error=%v", protocol, scenario, firstQuote, incremental, providerCalls.Load(), toolCalls.Load(), total, userBalance, tokenBalance, reloadUserQuota(t), token.RemainQuota, apiErr)
	require.Equal(t, userBalance-total, reloadUserQuota(t))
	require.Equal(t, tokenBalance-total, token.RemainQuota)
	require.Equal(t, total, token.UsedQuota)
	require.Equal(t, total, consumeLogQuota(t, requestID))
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.GreaterOrEqual(t, reloadUserQuota(t), int64(0))
	require.GreaterOrEqual(t, token.RemainQuota, int64(0))
	if scenario == "injected_funded" {
		expectedFirst := quotautil.Compute(quotautil.ComputeInput{
			Usage:     &relaymodel.Usage{PromptTokens: rounds[0].Prompt, CompletionTokens: output},
			ModelName: name, ModelRatio: .05, GroupRatio: 1, PricingAdaptor: providerAdaptor, RequestTime: meta.StartTime,
		}).TotalQuota
		require.Greater(t, expectedFirst, firstQuote)
		require.Equal(t, expectedFirst, userBalance-rounds[0].User, "injected tools fund only the first-round difference")
	} else {
		require.Equal(t, firstQuote, userBalance-rounds[0].User, "first round must credit its complete existing hold")
	}
	if strings.HasSuffix(scenario, "low") {
		require.EqualValues(t, 1, providerCalls.Load(), "unfunded second round must reject before provider I/O")
		require.NotNil(t, apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
		require.Zero(t, incremental)
		require.Equal(t, firstQuote, total)
	} else {
		require.EqualValues(t, 2, providerCalls.Load())
		if strings.HasSuffix(scenario, "refund_second") {
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
			require.Zero(t, incremental)
			require.Equal(t, firstQuote, total)
		} else {
			require.Nil(t, apiErr)
			require.Greater(t, incremental, int64(26000))
			if crossThreshold {
				require.Greater(t, total, firstQuote*5, "aggregate tariff must cover crossing the input tier")
			}
			require.LessOrEqual(t, total, held+incremental)
			require.Equal(t, held+incremental, userBalance-rounds[1].User)
		}
	}
}

// TestSecurityMCPTierCumulativeQuote bounds measured cache/output/tool usage
// without carrying previous unused output allowances into the next dispatch.
// Parameters: t owns assertions. Returns: none; actual payload markers, channel
// windows, group rates, free groups, and invalid receipts retain safe quotes.
func TestSecurityMCPTierCumulativeQuote(t *testing.T) {
	const name = "claude-haiku-5-5"
	c, _, _ := protocolContext(t, channeltype.Anthropic, name, "/v1/messages", "{}", "", 100000, 1, false, nil)
	meta := metalib.GetByContext(c)
	meta.OriginModelName, meta.ActualModelName = name, name
	base := &relaymodel.Usage{
		PromptTokens: 2, CompletionTokens: 17, ToolsCost: 23,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: 30000},
		CacheWrite5mTokens:  30000, CacheWrite1hTokens: 39997,
	}
	round, aggregate, err := quoteMCPTierRoundQuota(c, meta, name, 5, 10, 1, []byte("{}"), base)
	require.NoError(t, err)
	require.EqualValues(t, 3, round)
	require.EqualValues(t, 50059, aggregate, "full prompt is 100004, output is measured 17 plus current cap 10, known 1h writes remain eligible")
	require.Equal(t, 2, base.PromptTokens, "pricing must not mutate measured usage")
	require.Equal(t, 17, base.CompletionTokens)

	round, _, err = quoteMCPTierRoundQuota(c, meta, name, 100001, 10, 1, []byte("{\"cache_control\":{\"type\":\"ephemeral\",\"ttl\":\"1h\"}}"), nil)
	require.NoError(t, err)
	require.EqualValues(t, 50013, round, "only actual prepared payload markers authorize write pricing")
	_, _, err = quoteMCPTierRoundQuota(c, meta, name, 1, 10, 1, []byte("{\"cache_control\":{\"type\":\"ephemeral\",\"ttl\":\"bad\"}}"), nil)
	require.Error(t, err)

	cfg := model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, Tiers: []model.ModelRatioTierLocal{{InputTokenThreshold: 100, Ratio: 5, CompletionRatio: 3}}}
	cfg.TimeWindows = []model.TimeWindowLocal{{TimeZone: "UTC", Ranges: []model.ClockRangeLocal{{Start: "11:00", End: "13:00"}}, Overlay: model.ModelConfigLocal{Ratio: 4}}}
	channel := c.MustGet(ctxkey.ChannelModel).(*model.Channel)
	require.NoError(t, channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{name: cfg}))
	meta.StartTime = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	c.Set(ctxkey.ChannelRatio, 1.5)
	channelRatio, completionRatio := getChannelRatios(c)
	configs := getChannelModelConfigs(c)
	round, aggregate, err = quoteMCPTierRoundQuota(c, meta, name, 100, 10, 1, []byte("{}"), &relaymodel.Usage{PromptTokens: 25, CompletionTokens: 7, ToolsCost: 11})
	require.NoError(t, err)
	expected := quotautil.Compute(quotautil.ComputeInput{
		Usage: &relaymodel.Usage{PromptTokens: 125, CompletionTokens: 17, ToolsCost: 11}, ModelName: name,
		ModelRatio: channelRatio[name], ChannelModelRatio: channelRatio, GroupRatio: 1.5,
		ChannelModelConfigs: configs, ChannelCompletionRatio: completionRatio,
		PricingAdaptor: &anthropic.Adaptor{}, RequestTime: meta.StartTime,
	}).TotalQuota
	require.Equal(t, expected, aggregate, "the same request-time window and channel completion tariff settle the cumulative receipt")
	require.Positive(t, round)
	c.Set(ctxkey.ChannelRatio, 0.0)
	round, aggregate, err = quoteMCPTierRoundQuota(c, meta, name, 100, 0, 1, []byte("{}"), &relaymodel.Usage{PromptTokens: 25, CompletionTokens: 7, ToolsCost: 11})
	require.NoError(t, err)
	require.Zero(t, round)
	require.EqualValues(t, 11, aggregate, "token pricing is free while measured tool charges retain settlement semantics")

	c.Set(ctxkey.ChannelRatio, 1.0)
	for _, usage := range []*relaymodel.Usage{
		{PromptTokens: math.MaxInt},
		{CompletionTokens: math.MaxInt},
		{PromptTokens: 1, CacheWrite1hTokens: math.MaxInt},
		{ToolsCost: math.MaxInt64},
		{PromptTokens: -1},
		{CompletionTokens: -1},
		{CacheWrite5mTokens: -1},
		{ToolsCost: -1},
	} {
		_, _, err = quoteMCPTierRoundQuota(c, meta, name, 100, 10, 1, []byte("{}"), usage)
		require.Error(t, err)
	}
	_, _, err = quoteMCPTierRoundQuota(nil, meta, name, 1, 1, 1, nil, nil)
	require.Error(t, err)
	_, _, err = quoteMCPTierRoundQuota(c, nil, name, 1, 1, 1, nil, nil)
	require.Error(t, err)
	_, apiErr := reserveMCPTierRoundQuota(nil, nil, 1, 1, math.MaxInt64, 1, 0)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
}
