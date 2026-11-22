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
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// Quota ratios used below (ratio.MilliTokensUsd = 0.5 quota per token per $1/M):
//   - gpt-6-cyber short context: input $20 (10), 30m write $25 (12.5), output $100 (50).
//   - gpt-6-cyber above 272,000 input: input $40 (20), output $150 (75), write unpublished.
//   - gpt-6.1-terra short context: input $1 (0.5), 30m write $1.25 (0.625), output $5 (2.5).
//   - gpt-6.2-astra short context: input $8 (4), 30m write $10 (5), output $40 (20).
//   - gpt-6.2-sol short context: input $2 (1), 30m write $2.5 (1.25), output $10 (5).

// implicitCacheObservation records what the fake OpenAI upstream received and
// the balance it observed while the admission hold was outstanding.
type implicitCacheObservation struct {
	body      map[string]any
	userQuota int64
	err       error
}

// implicitCacheUpstream models OpenAI's GPT-5.6+ prompt-cache contract: caching
// is on by default, so a request writes its prefix unless it selects
// prompt_cache_options.mode=explicit without any explicit breakpoint. It
// answers every request with receipt input/output counters. Parameters: t owns
// cleanup, modelName is echoed, input/output are the receipt counters, and
// calls counts requests. Returns: the server and the channel of observations.
func implicitCacheUpstream(t *testing.T, modelName string, input, output int, calls *atomic.Int32) (*httptest.Server, chan implicitCacheObservation) {
	t.Helper()
	observations := make(chan implicitCacheObservation, 4)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		raw, err := io.ReadAll(r.Body)
		observation := implicitCacheObservation{err: err}
		if err == nil {
			observation.err = json.Unmarshal(raw, &observation.body)
		}
		if observation.err == nil {
			var user model.User
			observation.err = model.DB.First(&user, fallbackUserID).Error
			observation.userQuota = user.Quota
		}
		observations <- observation
		mode := ""
		if options, ok := observation.body["prompt_cache_options"].(map[string]any); ok {
			mode, _ = options["mode"].(string)
		}
		writes := input
		if mode == "explicit" && !strings.Contains(string(raw), `"prompt_cache_breakpoint"`) {
			writes = 0
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"resp_implicit_cache","object":"response","model":%q,"status":"completed","service_tier":"default","output":[{"type":"message","id":"msg_implicit","role":"assistant","content":[{"type":"output_text","text":"ack"}]}],"usage":{"input_tokens":%d,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":%d,"cache_write_tokens_details":{"ttl_30m_tokens":%d,"ttl_12h_tokens":0}},"output_tokens":%d,"total_tokens":%d}}`,
			modelName, input, writes, writes, output, input+output)
	}))
	t.Cleanup(server.Close)
	return server, observations
}

// cyberCacheRequest builds one client request whose user text repeats a word
// count times, so local prompt estimation reaches a chosen tier. Parameters:
// protocol is chat, responses or messages, words sets the prompt size, and
// extra is appended to the Responses input content part (for example a
// prompt_cache_breakpoint). Returns: the path and JSON body.
func cyberCacheRequest(protocol string, words int, extra string) (string, string) {
	text := strings.Repeat(" hello", words)
	switch protocol {
	case "responses":
		return "/v1/responses", `{"model":"alias","max_output_tokens":128,"input":[{"role":"user","content":[{"type":"input_text","text":"` + text + `"` + extra + `}]}]}`
	case "messages":
		return "/v1/messages", `{"model":"alias","max_tokens":128,"messages":[{"role":"user","content":"` + text + `"}]}`
	default:
		return "/v1/chat/completions", `{"model":"alias","max_tokens":128,"messages":[{"role":"user","content":"` + text + `"}]}`
	}
}

// relayCyberCacheResult is the ledger outcome of one native cache request.
type relayCyberCacheResult struct {
	failed         bool
	status         int
	message        string
	promptTokens   int
	charged        int64
	tokenCharged   int64
	logQuota       int64
	requestCost    int64
	estimateReason string
}

// relayNativeCacheRequest dispatches one request for modelName on a native
// OpenAI channel through the production helper. Parameters: t owns fixtures,
// protocol selects the client format, path/body are the client request,
// upstream is the fake provider, balance the funded wallet, and local an
// optional channel tariff override. Returns: the ledger outcome.
func relayNativeCacheRequest(t *testing.T, modelName, protocol, path, body string, upstream *httptest.Server, balance int64, local *model.ModelConfigLocal) *relayCyberCacheResult {
	t.Helper()
	previous := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = previous })
	c, _, requestID := protocolContext(t, channeltype.OpenAI, modelName, path, body, upstream.URL, balance, 1, false, local)
	clearNativeSettledLedger(t, requestID)
	if protocol == "responses_fallback" {
		// A custom base URL routes Responses clients through the Chat fallback.
		require.False(t, supportsNativeResponseAPI(metalib.GetByContext(c)))
		protocol = "responses"
	} else if protocol == "responses" {
		c.Set(ctxkey.BaseURL, "https://api.openai.com")
		c.Set(ctxkey.Config, model.ChannelConfig{EndpointURLs: map[string]string{
			channeltype.RelayModeToEndpointName(relaymode.ResponseAPI): upstream.URL + "/v1/responses",
		}})
		require.True(t, supportsNativeResponseAPI(metalib.GetByContext(c)))
	}
	apiErr := relayLyriaProtocol(c, protocol)
	drainCriticalTasks(t)
	result := &relayCyberCacheResult{
		failed:       apiErr != nil,
		promptTokens: metalib.GetByContext(c).PromptTokens,
		charged:      balance - reloadUserQuota(t),
	}
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	result.tokenCharged = balance - token.RemainQuota
	if apiErr != nil {
		result.status = apiErr.StatusCode
		result.message = apiErr.Message
		return result
	}
	result.requestCost = requestCostQuota(t, requestID)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	result.logQuota = int64(logs[0].Quota)
	result.estimateReason, _ = logs[0].Metadata["billing_estimate_reason"].(string)
	return result
}

// requireExactSettlement asserts that every ledger agrees on one exact charge
// that is not flagged as an estimate. Parameters: t owns assertions, result is
// the outcome and exact the hand-computed charge.
func requireExactSettlement(t *testing.T, result *relayCyberCacheResult, exact int64) {
	t.Helper()
	require.False(t, result.failed, result.message)
	require.Equal(t, exact, result.charged, "user quota")
	require.Equal(t, exact, result.tokenCharged, "token remain_quota")
	require.Equal(t, exact, result.logQuota, "consume log")
	require.Equal(t, exact, result.requestCost, "user_request_costs")
	require.Empty(t, result.estimateReason, "an exact receipt must not be flagged as estimated")
}

// TestNativePromptCacheImplicitWriteReservation proves the admission hold
// covers OpenAI's implicit 30m cache writes. OpenAI caches GPT-5.6+ prompts by
// default and bills the written prefix at 1.25x input; the previous hold priced
// the prompt at the plain input rate, so it did not cover the highest charge
// the admitted request could incur. The receipt with a full write still
// settles exactly.
func TestNativePromptCacheImplicitWriteReservation(t *testing.T) {
	for _, tc := range []struct {
		model             string
		writeRate, output float64
		exact             int64
	}{
		{model: "gpt-6-cyber", writeRate: 12.5, output: 50, exact: 31_400},
		{model: "gpt-6.1-terra", writeRate: 0.625, output: 2.5},
		{model: "gpt-6.2-astra", writeRate: 5, output: 20, exact: 12_560},
		{model: "gpt-6.2-sol", writeRate: 1.25, output: 5, exact: 3_140},
	} {
		for _, protocol := range []string{"chat", "responses", "messages"} {
			t.Run(tc.model+"/"+protocol, func(t *testing.T) {
				const balance = int64(100_000_000)
				xaiVideoSetup(t, balance, false)
				var calls atomic.Int32
				upstream, observations := implicitCacheUpstream(t, tc.model, 2_000, 128, &calls)
				path, body := cyberCacheRequest(protocol, 2_000, "")
				result := relayNativeCacheRequest(t, tc.model, protocol, path, body, upstream, balance, nil)
				require.False(t, result.failed, result.message)
				require.EqualValues(t, 1, calls.Load())
				observation := <-observations
				require.NoError(t, observation.err)
				_, pinned := observation.body["prompt_cache_options"]
				require.False(t, pinned, "short-context writes are priced, so caching must stay enabled")
				reserved := balance - observation.userQuota
				highest := int64(float64(result.promptTokens)*tc.writeRate + 128*tc.output)
				require.GreaterOrEqual(t, reserved, highest,
					"the hold must cover implicit 30m cache writes of the whole admitted prompt")
				if tc.exact > 0 {
					requireExactSettlement(t, result, tc.exact)
				}
			})
		}
	}
}

// TestNativePromptCacheLongContextPinsExplicitMode proves a Cyber request
// above 272,000 input tokens is sent with prompt_cache_options.mode=explicit.
// OpenAI publishes no long-context Cyber cache-write price, and its implicit
// caching wrote such prompts on every request, so each receipt settled the
// whole reservation as an unpriced estimate (2.2x the exact charge in this
// fixture). Pinned, the receipt has no write and settles exactly.
func TestNativePromptCacheLongContextPinsExplicitMode(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "responses_fallback", "messages"} {
		t.Run(protocol, func(t *testing.T) {
			const balance = int64(100_000_000)
			const receiptInput, receiptOutput = 290_000, 10
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			upstream, observations := implicitCacheUpstream(t, "gpt-6-cyber", receiptInput, receiptOutput, &calls)
			path, body := cyberCacheRequest(strings.TrimSuffix(protocol, "_fallback"), 285_000, "")
			result := relayNativeCacheRequest(t, "gpt-6-cyber", protocol, path, body, upstream, balance, nil)
			require.False(t, result.failed, result.message)
			require.Greater(t, result.promptTokens, 272_000, "fixture must reach the long-context tier")
			observation := <-observations
			require.NoError(t, observation.err)
			options, _ := observation.body["prompt_cache_options"].(map[string]any)
			require.Equal(t, map[string]any{"mode": "explicit"}, options, "unpriced long-context writes must be disabled upstream")
			require.Equal(t, "default", observation.body["service_tier"])
			requireExactSettlement(t, result, receiptInput*20+receiptOutput*75)
		})
	}
}

// TestNativePromptCacheLongContextClientControls proves explicit client cache
// writes in the unpriced tier are refused before dispatch, while a client that
// already disables caching is admitted unchanged and billed exactly.
func TestNativePromptCacheLongContextClientControls(t *testing.T) {
	for _, tc := range []struct {
		name, root, part string
		refused          bool
	}{
		{name: "explicit_breakpoint", part: `,"prompt_cache_breakpoint":{"mode":"explicit"}`, refused: true},
		{name: "explicit_implicit_mode", root: `"prompt_cache_options":{"mode":"implicit"},`, refused: true},
		{name: "prewarm", root: `"prompt_cache_options":{"mode":"explicit","prewarm":true},`, refused: true},
		{name: "client_disables_caching", root: `"prompt_cache_options":{"mode":"explicit","ttl":"30m"},`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(100_000_000)
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			upstream, observations := implicitCacheUpstream(t, "gpt-6-cyber", 290_000, 10, &calls)
			_, body := cyberCacheRequest("responses", 285_000, tc.part)
			body = strings.Replace(body, `{"model":"alias",`, `{"model":"alias",`+tc.root, 1)
			result := relayNativeCacheRequest(t, "gpt-6-cyber", "responses", "/v1/responses", body, upstream, balance, nil)
			if tc.refused {
				require.True(t, result.failed, "an explicit unpriced cache write must be refused before dispatch")
				require.Equal(t, http.StatusBadRequest, result.status)
				require.Contains(t, result.message, "no cache-write price")
				require.Zero(t, calls.Load())
				require.Zero(t, result.charged)
				require.Zero(t, result.tokenCharged)
				return
			}
			observation := <-observations
			require.NoError(t, observation.err)
			require.Equal(t, map[string]any{"mode": "explicit", "ttl": "30m"}, observation.body["prompt_cache_options"])
			requireExactSettlement(t, result, 290_000*20+10*75)
		})
	}
}

// TestNativePromptCacheLongContextOperatorTier proves an explicit operator
// long-context tier with a cache-write rate qualifies the writes: caching
// stays enabled and the write settles at the operator rate.
func TestNativePromptCacheLongContextOperatorTier(t *testing.T) {
	const balance = int64(100_000_000)
	xaiVideoSetup(t, balance, false)
	var calls atomic.Int32
	upstream, observations := implicitCacheUpstream(t, "gpt-6-cyber", 290_000, 10, &calls)
	_, body := cyberCacheRequest("responses", 285_000, "")
	// A tier-only override: a positive scalar Ratio would be a flat channel
	// override that keeps its input rate across tiers.
	local := &model.ModelConfigLocal{
		Tiers: []model.ModelRatioTierLocal{{Ratio: 20, CompletionRatio: 3.75, CachedInputRatio: 2, CacheWrite5mRatio: 25, InputTokenThreshold: 272_001}}}
	result := relayNativeCacheRequest(t, "gpt-6-cyber", "responses", "/v1/responses", body, upstream, balance, local)
	observation := <-observations
	require.NoError(t, observation.err)
	_, pinned := observation.body["prompt_cache_options"]
	require.False(t, pinned, "a qualified operator write tier must keep caching enabled")
	reserved := balance - observation.userQuota
	require.GreaterOrEqual(t, reserved, int64(result.promptTokens)*25+128*75, "the hold covers operator-priced writes")
	requireExactSettlement(t, result, 290_000*25+10*75)
}

// TestNativePromptCacheOptionsForwarded proves a client's sanitized
// prompt_cache_options reach OpenAI for metered GPT-5.6+ models: explicit-only
// mode was previously stripped, so OpenAI applied implicit writes the client
// had opted out of. Diagnostics are dropped and unmetered lifetimes refused;
// non-metered models keep never forwarding the field.
func TestNativePromptCacheOptionsForwarded(t *testing.T) {
	for _, tc := range []struct {
		name, model, options string
		forwarded            map[string]any
		status               int
		exact                int64
	}{
		{name: "explicit_only", model: "gpt-6-cyber", options: `{"mode":" Explicit ","ttl":"30M","comparison_response_id":"resp_other_tenant"}`,
			forwarded: map[string]any{"mode": "explicit", "ttl": "30m"}, exact: 2_000*10 + 10*50},
		{name: "default_mode_ttl_only", model: "gpt-6-cyber", options: `{"ttl":"30m"}`,
			forwarded: map[string]any{"ttl": "30m"}, exact: 2_000*25/2 + 10*50},
		{name: "diagnostics_only_dropped", model: "gpt-6-cyber", options: `{"comparison_response_id":"resp_x"}`, exact: 2_000*25/2 + 10*50},
		{name: "twelve_hour_refused", model: "gpt-6-cyber", options: `{"ttl":"12h"}`, status: http.StatusBadRequest},
		{name: "unknown_mode_refused", model: "gpt-6-cyber", options: `{"mode":"always"}`, status: http.StatusBadRequest},
		{name: "unmetered_model_stripped", model: "gpt-4o-mini", options: `{"mode":"explicit"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(100_000_000)
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			upstream, observations := implicitCacheUpstream(t, tc.model, 2_000, 10, &calls)
			body := `{"model":"alias","max_output_tokens":128,"prompt_cache_options":` + tc.options + `,"input":"` + strings.Repeat(" hello", 2_000) + `"}`
			result := relayNativeCacheRequest(t, tc.model, "responses", "/v1/responses", body, upstream, balance, nil)
			if tc.status != 0 {
				require.True(t, result.failed)
				require.Equal(t, tc.status, result.status)
				require.Zero(t, calls.Load())
				require.Zero(t, result.charged)
				return
			}
			require.False(t, result.failed, result.message)
			observation := <-observations
			require.NoError(t, observation.err)
			options, present := observation.body["prompt_cache_options"]
			if tc.forwarded == nil {
				require.False(t, present, "nothing forwardable remains: %v", options)
			} else {
				require.Equal(t, tc.forwarded, options)
			}
			if tc.exact != 0 {
				requireExactSettlement(t, result, tc.exact)
			}
		})
	}
}
