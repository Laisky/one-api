package controller

import (
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
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	quotautil "github.com/Laisky/one-api/relay/quota"
	"github.com/stretchr/testify/require"
)

// nativeResponseTierObservation captures one bounded provider dispatch and its
// durable owner/token holds without assertions in the HTTP goroutine.
type nativeResponseTierObservation struct {
	Path, Model string
	Prompt, Cap int
	User, Token int64
	Err         error
}

// TestSecurityNativeResponseTierAdmission uses an intentionally configured
// API-compatible Responses proxy with explicit tier prices. Parameters: t owns
// disposable SQLite and one loopback provider response per scenario. Returns:
// none; an unaffordable tier receipt must reject before native provider I/O.
func TestSecurityNativeResponseTierAdmission(t *testing.T) {
	const name = "native-tier-proxy-fixture"
	const output = 20
	config := model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, Tiers: []model.ModelRatioTierLocal{{
		InputTokenThreshold: 100, Ratio: 5, CompletionRatio: 100,
	}}}
	for _, scenario := range []string{"owner_low", "token_low", "funded", "flat_control", "retry_refund", "no_usage", "partial_output", "default_configured", "uppercase_cap", "uncapped", "free_group"} {
		t.Run(scenario, func(t *testing.T) {
			userBalance, tokenBalance := int64(10000), int64(10000)
			if scenario == "owner_low" {
				userBalance = 1000
			}
			if scenario == "token_low" {
				tokenBalance = 1000
			}
			xaiVideoSetup(t, userBalance, false)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).Update("remain_quota", tokenBalance).Error)
			observed := make(chan nativeResponseTierObservation, 1)
			var calls atomic.Int32
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				got := nativeResponseTierObservation{Path: r.URL.Path}
				var request openai.ResponseAPIRequest
				got.Err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request)
				if got.Err == nil {
					got.Model = request.Model
					got.Prompt = getResponseAPIPromptTokens(r.Context(), &request)
					if request.MaxOutputTokens != nil {
						got.Cap = *request.MaxOutputTokens
					}
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
				expectedCap := output
				if scenario == "default_configured" || scenario == "uppercase_cap" {
					expectedCap = 0
				}
				if got.Err != nil || got.Model != name || got.Cap != expectedCap {
					http.Error(w, "invalid bounded native fixture", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if scenario == "retry_refund" {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = fmt.Fprint(w, `{"error":{"message":"local explicit rejection","type":"overloaded_error"}}`)
					return
				}
				receiptOutput := output
				if scenario == "partial_output" {
					receiptOutput = 2
				}
				usageJSON := fmt.Sprintf(`{"input_tokens":%d,"output_tokens":%d,"total_tokens":%d}`, got.Prompt, receiptOutput, got.Prompt+receiptOutput)
				if scenario == "no_usage" {
					usageJSON = "null"
				}
				_, err := fmt.Fprintf(w, `{"id":"resp_local_tier","object":"response","status":"completed","model":"%s","output":[{"type":"message","id":"msg_local","status":"completed","role":"assistant","content":[{"type":"output_text","text":"done","annotations":[]}]}],"usage":%s}`, name, usageJSON)
				if err != nil {
					return
				}
			}))
			t.Cleanup(provider.Close)
			old := client.HTTPClient
			client.HTTPClient = provider.Client()
			t.Cleanup(func() { client.HTTPClient = old })
			fields := map[string]any{"model": "alias", "input": strings.Repeat("input ", 200), "max_output_tokens": output, "store": false}
			if scenario == "default_configured" || scenario == "uppercase_cap" || scenario == "uncapped" {
				delete(fields, "max_output_tokens")
			}
			if scenario == "uppercase_cap" {
				fields["MAX_OUTPUT_TOKENS"] = 2
			}
			payload, err := json.Marshal(fields)
			require.NoError(t, err)
			local := config
			if scenario == "flat_control" {
				local.Tiers = nil
			}
			if scenario == "default_configured" || scenario == "uppercase_cap" {
				local.MaxTokens = output
			}
			// ContextLength is a joint input/output limit, not an output cap.
			if scenario == "uncapped" {
				local.ContextLength = 100000
			}
			group := 1.0
			if scenario == "free_group" {
				group = 0
			}
			c, _, requestID := protocolContext(t, channeltype.OpenAICompatible, name, "/v1/responses", string(payload), provider.URL, userBalance, group, false, &local)
			c.Set(ctxkey.Config, model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse})
			c.Set(ctxkey.TokenQuota, tokenBalance)
			meta := metalib.GetByContext(c)
			meta.OriginModelName, meta.ActualModelName = name, name
			require.True(t, supportsNativeResponseAPI(meta))
			unsupported := *meta
			unsupported.ChannelType = channeltype.Anthropic
			require.False(t, supportsNativeResponseAPI(&unsupported), "Anthropic Responses uses Chat fallback, not this native branch")
			apiErr := RelayResponseAPIHelper(c)
			held := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
			if scenario == "retry_refund" {
				// Generic possibly-forwarded HTTP failures retain the hold until
				// the existing retry lifecycle explicitly abandons that attempt.
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
				require.Equal(t, userBalance-held, reloadUserQuota(t))
				ResetPerAttemptBillingForRetry(gmw.Ctx(c), c)
			}
			drainCriticalTasks(t)
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			charged := int64(0)
			if calls.Load() > 0 {
				got := <-observed
				require.NoError(t, got.Err)
				require.Equal(t, "/v1/responses", got.Path)
				require.Equal(t, name, got.Model)
				require.GreaterOrEqual(t, got.Prompt, 100)
				expectedCap := output
				if scenario == "default_configured" || scenario == "uppercase_cap" {
					expectedCap = 0
				}
				require.Equal(t, expectedCap, got.Cap)
				require.Equal(t, held, userBalance-got.User)
				require.Equal(t, held, tokenBalance-got.Token)
				require.False(t, metalib.GetByContext(c).ResponseAPIFallback)
				channelRatio, completionRatio := getChannelRatios(c)
				cfg := getChannelModelConfigs(c)
				receiptOutput := output
				if scenario == "partial_output" {
					receiptOutput = 2
				}
				if scenario == "no_usage" {
					receiptOutput = openai.CountTokenText("done", name)
				}
				charged = quotautil.Compute(quotautil.ComputeInput{
					Usage:     &relaymodel.Usage{PromptTokens: got.Prompt, CompletionTokens: receiptOutput},
					ModelName: name, ModelRatio: channelRatio[name], ChannelModelRatio: channelRatio, GroupRatio: group,
					ChannelModelConfigs: cfg, ChannelCompletionRatio: completionRatio,
					PricingAdaptor: &openai.Adaptor{}, RequestTime: meta.StartTime,
				}).TotalQuota
				if strings.HasSuffix(scenario, "low") {
					require.Less(t, held, int64(1000), "scalar hold fits the limited balance")
					require.Greater(t, charged, int64(1000), "effective settlement tariff exceeds the limited balance")
				}
				if scenario == "retry_refund" {
					charged = 0
				} else {
					require.Equal(t, charged, consumeLogQuota(t, requestID))
				}
				require.Equal(t, charged, requestCostQuota(t, requestID))
				require.Equal(t, userBalance-charged, reloadUserQuota(t))
				require.Equal(t, tokenBalance-charged, token.RemainQuota)
				t.Logf("NATIVE_RESPONSE_TIER scenario=%s path=%s model=%s prompt=%d output=%d calls=%d held=%d user_at_dispatch=%d token_at_dispatch=%d charge=%d final_user=%d final_token=%d api_error=%v", scenario, got.Path, got.Model, got.Prompt, output, calls.Load(), held, got.User, got.Token, charged, reloadUserQuota(t), token.RemainQuota, apiErr)
			}
			if strings.HasSuffix(scenario, "low") || scenario == "uncapped" || scenario == "uppercase_cap" {
				require.Zero(t, calls.Load(), "unsafe tier allowance must reject before provider I/O")
				require.NotNil(t, apiErr)
				expectedStatus := http.StatusForbidden
				if scenario == "uncapped" || scenario == "uppercase_cap" {
					expectedStatus = http.StatusBadRequest
				}
				require.Equal(t, expectedStatus, apiErr.StatusCode)
				require.Zero(t, held)
				require.Equal(t, userBalance, reloadUserQuota(t))
				require.Equal(t, tokenBalance, token.RemainQuota)
				require.Zero(t, token.UsedQuota)
			} else {
				require.EqualValues(t, 1, calls.Load())
				if scenario == "retry_refund" {
					require.NotNil(t, apiErr)
					require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
				} else {
					require.Nil(t, apiErr)
				}
				require.GreaterOrEqual(t, reloadUserQuota(t), int64(0))
				require.GreaterOrEqual(t, token.RemainQuota, int64(0))
				require.Equal(t, charged, token.UsedQuota)
				if scenario == "flat_control" || scenario == "funded" || scenario == "default_configured" || scenario == "uppercase_cap" || scenario == "free_group" {
					require.Equal(t, held, charged)
				}
				if scenario == "partial_output" || scenario == "no_usage" {
					require.Less(t, charged, held, "preserve existing measured-output and terminal text-estimate reconciliation")
				}
			}
		})
	}
}

// TestSecurityNativeResponseReachability records the configured-native branch
// independently of billing fixtures. Parameters: t owns assertions. Returns:
// none; the native route is enabled only for the declared compatible format.
func TestSecurityNativeResponseReachability(t *testing.T) {
	meta := &metalib.Meta{ChannelType: channeltype.OpenAICompatible, ActualModelName: "native-tier-proxy-fixture", BaseURL: "http://127.0.0.1", Config: model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse}}
	require.True(t, supportsNativeResponseAPI(meta))
	meta.Config.APIFormat = channeltype.OpenAICompatibleAPIFormatChatCompletion
	require.False(t, supportsNativeResponseAPI(meta))
	meta.ChannelType = channeltype.Anthropic
	require.False(t, supportsNativeResponseAPI(meta))
}

// TestSecurityResponseTierPricingControls checks the shared tariff, actual
// prepared cap, metadata fallbacks, and checked arithmetic without provider I/O.
// Parameters: t owns assertions. Returns: none; standalone flat helpers remain
// compatible and invalid or unbounded tier contracts fail closed.
func TestSecurityResponseTierPricingControls(t *testing.T) {
	const name = "native-tier-pricing-fixture"
	config := model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, Tiers: []model.ModelRatioTierLocal{{InputTokenThreshold: 100, Ratio: 5, CompletionRatio: 3}}}
	c, _, _ := protocolContext(t, channeltype.OpenAICompatible, name, "/v1/responses", "{}", "", 10000, 1.5, false, &config)
	meta := metalib.GetByContext(c)
	meta.StartTime = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	channel := c.MustGet(ctxkey.ChannelModel).(*model.Channel)
	config.TimeWindows = []model.TimeWindowLocal{{TimeZone: "UTC", Ranges: []model.ClockRangeLocal{{Start: "11:00", End: "13:00"}}, Overlay: model.ModelConfigLocal{Ratio: 4}}}
	require.NoError(t, channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{name: config}))
	output := 10
	request := &openai.ResponseAPIRequest{Model: name, MaxOutputTokens: &output}
	quote, applies, err := quoteResponseTierAdmission(c, meta, request, 125, nil)
	require.NoError(t, err)
	require.True(t, applies)
	rates, completions := getChannelRatios(c)
	cfg := getChannelModelConfigs(c)
	expected := quotautil.Compute(quotautil.ComputeInput{
		Usage:     &relaymodel.Usage{PromptTokens: 125, CompletionTokens: output},
		ModelName: name, ModelRatio: rates[name], ChannelModelRatio: rates, GroupRatio: 1.5,
		ChannelModelConfigs: cfg, ChannelCompletionRatio: completions, PricingAdaptor: &openai.Adaptor{}, RequestTime: meta.StartTime,
	}).TotalQuota
	require.Equal(t, expected, quote, "channel/time/group precedence must match settlement")

	negative := -1
	request.MaxOutputTokens = &negative
	_, applies, err = quoteResponseTierAdmission(c, meta, request, 125, nil)
	require.True(t, applies)
	require.Error(t, err)
	request.MaxOutputTokens = &output
	_, _, err = quoteResponseTierAdmission(c, meta, request, -1, nil)
	require.Error(t, err)
	overflow := math.MaxInt
	request.MaxOutputTokens = &overflow
	_, _, err = quoteResponseTierAdmission(c, meta, request, 125, nil)
	require.Error(t, err)
	request.MaxOutputTokens = &output
	c.Set(ctxkey.ChannelRatio, math.Inf(1))
	_, _, err = quoteResponseTierAdmission(c, meta, request, 125, nil)
	require.Error(t, err)
	c.Set(ctxkey.ChannelRatio, 0.0)
	request.MaxOutputTokens = nil
	quote, applies, err = quoteResponseTierAdmission(c, meta, request, 125, nil)
	require.NoError(t, err)
	require.True(t, applies)
	require.Zero(t, quote, "authoritative free group requires no output cap")

	c.Set(ctxkey.ChannelRatio, 1.0)
	request.MaxOutputTokens = &output
	_, _, err = quoteResponseTierAdmission(c, meta, request, 125, []byte("{"))
	require.Error(t, err)
	mismatch, err := json.Marshal(map[string]any{"model": "another-model", "input": "hello", "max_output_tokens": 10})
	require.NoError(t, err)
	_, _, err = quoteResponseTierAdmission(c, meta, request, 125, mismatch)
	require.Error(t, err)
	_, _, err = quoteResponseTierAdmission(nil, meta, request, 125, nil)
	require.Error(t, err)
	_, _, err = quoteResponseTierAdmission(c, nil, request, 125, nil)
	require.Error(t, err)
	_, _, err = quoteResponseTierAdmission(c, meta, nil, 125, nil)
	require.Error(t, err)

	// A known catalog output cap remains available when an operator supplies
	// only a tier tariff. ContextLength never substitutes for that output cap.
	const catalogName = "gpt-4o"
	require.NoError(t, channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{catalogName: {Ratio: 1, CompletionRatio: 2, Tiers: config.Tiers}}))
	request = &openai.ResponseAPIRequest{Model: catalogName, Input: openai.ResponseAPIInput{strings.Repeat("input ", 200)}}
	body, err := json.Marshal(request)
	require.NoError(t, err)
	typedCap := 2
	request.MaxOutputTokens = &typedCap
	quote, applies, err = quoteResponseTierAdmission(c, meta, request, 0, body)
	require.NoError(t, err)
	require.True(t, applies)
	provider := resolvePricingAdaptor(meta)
	known, _ := pricing.ResolveModelConfig(catalogName, nil, provider, meta.StartTime)
	require.Positive(t, known.MaxOutputTokens)
	rates, completions = getChannelRatios(c)
	cfg = getChannelModelConfigs(c)
	prompt := getResponseAPIPromptTokens(gmw.Ctx(c), request)
	expected = quotautil.Compute(quotautil.ComputeInput{
		Usage:     &relaymodel.Usage{PromptTokens: prompt, CompletionTokens: int(known.MaxOutputTokens)},
		ModelName: catalogName, ModelRatio: rates[catalogName], ChannelModelRatio: rates, GroupRatio: 1,
		ChannelModelConfigs: cfg, ChannelCompletionRatio: completions, PricingAdaptor: provider, RequestTime: meta.StartTime,
	}).TotalQuota
	require.Equal(t, expected, quote)
	require.Equal(t, prompt, meta.PromptTokens, "prepared input controls the adaptor's fallback usage estimate")
	c.Set(ctxkey.ChannelModel, nil)
	request.Model = "standalone-flat-model"
	_, applies, err = quoteResponseTierAdmission(c, meta, request, 125, nil)
	require.NoError(t, err)
	require.False(t, applies, "missing channel metadata must preserve flat helper compatibility")
}

// TestSecurityResponseTierMissingUsageHold verifies the existing settlement
// helper's nil/zero-usage contract for the new full tier hold. Parameters: t owns
// disposable ledgers. Returns: none; unmeasured helper input retains its hold.
func TestSecurityResponseTierMissingUsageHold(t *testing.T) {
	for _, usage := range []*relaymodel.Usage{nil, {}} {
		t.Run(fmt.Sprintf("nil=%v", usage == nil), func(t *testing.T) {
			const name = "native-tier-missing-usage-fixture"
			const balance int64 = 10000
			xaiVideoSetup(t, balance, false)
			output := 20
			local := model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, Tiers: []model.ModelRatioTierLocal{{InputTokenThreshold: 100, CompletionRatio: 100}}}
			c, _, id := protocolContext(t, channeltype.OpenAICompatible, name, "/v1/responses", "{}", "", balance, 1, false, &local)
			meta := metalib.GetByContext(c)
			meta.OriginModelName, meta.ActualModelName = name, name
			request := &openai.ResponseAPIRequest{Model: name, MaxOutputTokens: &output}
			held, apiErr := preConsumeResponseAPIQuota(c, request, 456, 1, 2, false, meta)
			require.Nil(t, apiErr)
			require.EqualValues(t, 2456, held)
			markPreConsumed(c, held)
			c.Set(ctxkey.ProvisionalLogId, recordProvisionalLog(c, meta, name, held))
			rates, completion := getChannelRatios(c)
			total := postConsumeResponseAPIQuota(gmw.Ctx(c), usage, meta, request, held, 1, rates, 1, getChannelModelConfigs(c), completion)
			drainCriticalTasks(t)
			require.Equal(t, held, total)
			require.Equal(t, balance-total, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-total, token.RemainQuota)
			require.Equal(t, total, consumeLogQuota(t, id))
		})
	}
}
