package controller

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestOpenRouterFlatPromptCacheAdmission verifies that flat channel overrides
// fund automatic writes without restoring provider tiers or changing other
// hosts. Parameter t owns assertions; invalid prepared lifetimes are refused.
func TestOpenRouterFlatPromptCacheAdmission(t *testing.T) {
	const name = "openai/gpt-6.2-astra"
	local := map[string]model.ModelConfigLocal{name: {Ratio: 4, CompletionRatio: 5, CacheWrite5mRatio: 5}}
	request := &relaymodel.GeneralOpenAIRequest{Model: name, MaxTokens: 1,
		Messages: []relaymodel.Message{{Role: "user", Content: "fixture"}}}
	for _, host := range []int{channeltype.OpenRouter, channeltype.OpenAI, channeltype.Azure, channeltype.OpenAICompatible} {
		m := &metalib.Meta{ChannelType: host, APIType: channeltype.ToAPIType(host), ActualModelName: name,
			Mode: relaymode.ChatCompletions, StartTime: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)}
		for _, prompt := range []int{100_000, 272_001} {
			quote, cache, err := estimatePreConsumedQuotaWithCachePolicy(request, &relaymodel.Usage{PromptTokens: prompt},
				4, 5, nil, 1, local, nil, m)
			require.NoError(t, err)
			inputRate := int64(4)
			if host == channeltype.OpenRouter {
				inputRate = 5
			}
			require.Equal(t, int64(prompt)*inputRate+20+config.PreConsumedQuota*4, quote,
				"host %d must retain its effective flat tariff at %d tokens", host, prompt)
			require.Equal(t, host == channeltype.OpenRouter, cache.write5m)
			require.False(t, cache.write1h)
			require.False(t, cache.pinExplicit)
		}
	}

	m := &metalib.Meta{ChannelType: channeltype.OpenRouter, ActualModelName: name}
	for _, raw := range []string{
		`{"prompt_cache_options":{"ttl":"12h"},"messages":[{"role":"user","content":"hi"}]}`,
		`{"messages":[{"role":"user","content":[{"type":"text","text":"hi","prompt_cache_breakpoint":{"ttl":"12h"}}]}]}`,
	} {
		quote, _, err := estimatePreConsumedQuotaWithCachePolicy(request, &relaymodel.Usage{PromptTokens: 100_000},
			4, 5, nil, 1, local, nil, m, json.RawMessage(raw))
		require.Error(t, err)
		require.True(t, adaptor.IsUnbillableRequestControl(err))
		require.EqualValues(t, -1, quote, "a flat override cannot skip prepared cache-control validation")
	}
}

// TestOpenRouterFlatPreparedResponsesAdmission prices the bytes that Responses
// actually forwards. Parameter t owns assertions; historical stripping of root
// prompt_cache_options leaves automatic writes enabled, while block 12h controls
// survive preparation and must still be refused before any reservation.
func TestOpenRouterFlatPreparedResponsesAdmission(t *testing.T) {
	const name = "openai/gpt-6.2-astra"
	local := model.ModelConfigLocal{Ratio: 4, CompletionRatio: 5, CacheWrite5mRatio: 5}
	for _, tc := range []struct {
		name, body string
		refused    bool
	}{
		{name: "automatic", body: `{"model":"openai/gpt-6.2-astra","input":"hello","max_output_tokens":1}`},
		{name: "explicit_options_stripped", body: `{"model":"openai/gpt-6.2-astra","prompt_cache_options":{"mode":"explicit"},"input":"hello","max_output_tokens":1}`},
		{name: "block_12h_refused", body: `{"model":"openai/gpt-6.2-astra","input":[{"role":"user","content":[{"type":"input_text","text":"hello","prompt_cache_breakpoint":{"ttl":"12h"}}]}],"max_output_tokens":1}`, refused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _ := protocolContext(t, channeltype.OpenRouter, name, "/v1/responses", tc.body, "", 1_000_000, 1, false, &local)
			m := metalib.GetByContext(c)
			m.OriginModelName, m.ActualModelName = name, name
			var request openai.ResponseAPIRequest
			require.NoError(t, json.Unmarshal([]byte(tc.body), &request))
			prepared, err := prepareResponseTierBody(c, m, &request, resolvePricingAdaptor(m))
			require.NoError(t, err)
			var wire map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(prepared, &wire))
			require.NotContains(t, wire, "prompt_cache_options", "this change does not enable new Responses root controls")
			quote, applies, err := quoteResponseTierAdmission(c, m, &request, 999, prepared)
			require.True(t, applies, "a qualified flat override must use complete admission")
			if tc.refused {
				require.Error(t, err)
				require.True(t, adaptor.IsUnbillableRequestControl(err))
				require.Zero(t, quote)
				return
			}
			require.NoError(t, err)
			var quoted openai.ResponseAPIRequest
			require.NoError(t, json.Unmarshal(prepared, &quoted))
			prompt := getResponseAPIPromptTokens(gmw.Ctx(c), &quoted)
			require.Positive(t, prompt)
			require.Equal(t, int64(prompt*5+20), quote, "actual prepared input, automatic writes, and flat output determine the hold")
			require.Equal(t, prompt, m.PromptTokens)
		})
	}
}

// TestOpenRouterFlatMessagesReservation checks the durable Messages allowance
// and prepared-control refusal. Parameter t owns the SQLite fixture and verifies
// that both owner and token balances hold the complete flat cache-write quote.
func TestOpenRouterFlatMessagesReservation(t *testing.T) {
	const name = "openai/gpt-6.2-astra"
	const balance = int64(1_000_000)
	local := model.ModelConfigLocal{Ratio: 4, CompletionRatio: 5, CacheWrite5mRatio: 5}
	for _, refused := range []bool{false, true} {
		securityAdmissionSetup(t, balance)
		c, _, _ := protocolContext(t, channeltype.OpenRouter, name, "/v1/messages", "{}", "", balance, 1, false, &local)
		m := metalib.GetByContext(c)
		m.OriginModelName, m.ActualModelName = name, name
		prepared := `{"model":"openai/gpt-6.2-astra","messages":[{"role":"user","content":"hello"}],"max_tokens":1}`
		if refused {
			prepared = `{"model":"openai/gpt-6.2-astra","messages":[{"role":"user","content":[{"type":"text","text":"hello","prompt_cache_breakpoint":{"ttl":"12h"}}]}],"max_tokens":1}`
		}
		c.Set(tierAdmissionProviderBodyKey, []byte(prepared))
		held, apiErr := preConsumeClaudeMessagesQuota(c, &ClaudeMessagesRequest{Model: name, MaxTokens: 1}, 100_000, 4, 5, m)
		if refused {
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.Zero(t, held)
		} else {
			require.Nil(t, apiErr)
			require.EqualValues(t, 500_020, held)
		}
		require.Equal(t, balance-held, reloadUserQuota(t))
		var token model.Token
		require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
		require.Equal(t, balance-held, token.RemainQuota)
	}
}

// TestOpenRouterFlatMCPAndWebSocketAdmission checks follow-up quote consumers.
// Parameter t owns assertions; prepared round/frame writes remain funded with
// flat overrides and unsupported lifetimes cannot mutate the WebSocket hold.
func TestOpenRouterFlatMCPAndWebSocketAdmission(t *testing.T) {
	const name = "openai/gpt-6.2-astra"
	local := model.ModelConfigLocal{Ratio: 4, CompletionRatio: 5, CacheWrite5mRatio: 5}
	c, _, _ := protocolContext(t, channeltype.OpenRouter, name, "/v1/responses", "{}", "", 1_000_000, 1, false, &local)
	m := metalib.GetByContext(c)
	m.OriginModelName, m.ActualModelName = name, name
	round, aggregate, err := quoteMCPTierRoundQuota(c, m, name, 100_000, 1, 1, []byte(`{"messages":[{"role":"user","content":"hello"}]}`),
		&relaymodel.Usage{PromptTokens: 25, CompletionTokens: 7, ToolsCost: 11})
	require.NoError(t, err)
	require.EqualValues(t, 500_020, round)
	require.EqualValues(t, 100_025*5+8*20+11, aggregate)
	_, _, err = quoteMCPTierRoundQuota(c, m, name, 100_000, 1, 1,
		[]byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello","prompt_cache_breakpoint":{"ttl":"12h"}}]}]}`), nil)
	require.True(t, adaptor.IsUnbillableRequestControl(err))

	funding := newResponseWSTierFunding(c, m, 0)
	require.True(t, funding.enabled)
	frame := []byte(`{"type":"response.create","model":"openai/gpt-6.2-astra","input":"hello","max_output_tokens":1}`)
	added, quoted, err := funding.additionalQuote(gmw.Ctx(c), frame)
	require.NoError(t, err)
	var request openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(frame, &request))
	prompt := getResponseAPIPromptTokens(gmw.Ctx(c), &request)
	require.Equal(t, int64(prompt*5+20), quoted)
	require.Equal(t, quoted, added)
	_, _, err = funding.additionalQuote(gmw.Ctx(c), []byte(`{"type":"response.create","model":"openai/gpt-6.2-astra","input":[{"role":"user","content":[{"type":"input_text","text":"hello","prompt_cache_breakpoint":{"ttl":"12h"}}]}],"max_output_tokens":1}`))
	require.True(t, adaptor.IsUnbillableRequestControl(err))
	require.Zero(t, funding.quoted)
	require.Zero(t, funding.reserved)

	c.Set(ctxkey.Channel, channeltype.OpenAICompatible)
	m.ChannelType = channeltype.OpenAICompatible
	require.False(t, newResponseWSTierFunding(c, m, 0).enabled, "other hosts retain flat admission behavior")
}
