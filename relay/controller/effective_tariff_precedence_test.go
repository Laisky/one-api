package controller

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// precedenceChatReceipt and precedenceResponsesReceipt are provider receipts of
// 10,000 input and 2,000 output tokens, large enough that every tariff below
// settles to an exact integer quota without rounding ambiguity.
const (
	precedenceChatReceipt      = `{"id":"chatcmpl-precedence","object":"chat.completion","model":"gpt-4o-mini","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ack"}}],"usage":{"prompt_tokens":10000,"completion_tokens":2000,"total_tokens":12000}}`
	precedenceResponsesReceipt = `{"id":"resp-precedence","object":"response","created_at":1,"status":"completed","model":"gpt-4o-mini","output":[{"type":"message","id":"msg-precedence","status":"completed","role":"assistant","content":[{"type":"output_text","text":"ack","annotations":[]}]}],"usage":{"input_tokens":10000,"output_tokens":2000,"total_tokens":12000}}`
)

// precedenceReply answers chat-completions and Responses dispatches with the
// matching receipt. Parameters: path is the provider request path. Returns: the
// content type and body.
func precedenceReply(path string) (string, string) {
	if strings.HasSuffix(path, "/responses") {
		return "application/json", precedenceResponsesReceipt
	}
	return "application/json", precedenceChatReceipt
}

// gpt4oMiniDefaultRatio and gpt4oMiniDefaultCompletion are OpenAI's current
// gpt-4o-mini tariff ($0.15/M input, $0.60/M output), which Azure inherits.
const (
	gpt4oMiniDefaultRatio      = 0.15 * billingratio.MilliTokensUsd
	gpt4oMiniDefaultCompletion = 4.0
)

// TestEffectiveTariffPrecedenceHTTP proves, through the production controllers,
// a fake TLS provider and the SQLite ledger, which tariff an ordinary inherited
// model (gpt-4o-mini) is billed at on Azure and on a native OpenAI channel for
// every client protocol: (a) without channel model_configs, the provider
// default; (b) with a channel override carrying a historical price, the
// override, although it differs from the current provider default; (c) with a
// ratio-only override, the override ratio plus the provider's completion ratio
// for the omitted field. The admission reservation observed at dispatch is
// priced from the same effective tariff as the final settlement, so balance
// delta == consume log == request cost == the hand-computed receipt charge.
func TestEffectiveTariffPrecedenceHTTP(t *testing.T) {
	useProductionGlobalPricing(t)
	for _, host := range []struct {
		name, base string
		channel    int
		config     model.ChannelConfig
		// nativeResponses reports whether Responses clients reach the provider's
		// native Responses API (no reservation buffer) instead of the Chat fallback.
		nativeResponses bool
	}{
		{name: "azure", channel: channeltype.Azure, config: model.ChannelConfig{APIVersion: "2025-04-01-preview"}},
		// The production base URL keeps native Responses routing for Responses clients.
		{name: "openai", base: "https://api.openai.com", channel: channeltype.OpenAI, config: model.ChannelConfig{}, nativeResponses: true},
	} {
		for _, protocol := range []string{"chat", "messages", "responses"} {
			for _, tariff := range []struct {
				name              string
				local             *model.ModelConfigLocal
				ratio, completion float64
				// charge is the receipt (10,000 input, 2,000 output) priced by hand.
				charge int64
			}{
				// 10,000*0.075 + 2,000*0.075*4 = 750 + 600.
				{name: "provider_default", ratio: gpt4oMiniDefaultRatio, completion: gpt4oMiniDefaultCompletion, charge: 1350},
				// Historical $0.50/M input, $1.50/M output: 10,000*0.25 + 2,000*0.75.
				{name: "historical_override", local: &model.ModelConfigLocal{Ratio: 0.5 * billingratio.MilliTokensUsd, CompletionRatio: 3}, ratio: 0.25, completion: 3, charge: 4000},
				// Ratio-only override inherits the provider completion ratio 4: 2,500 + 2,000*0.25*4.
				{name: "ratio_only_override", local: &model.ModelConfigLocal{Ratio: 0.5 * billingratio.MilliTokensUsd}, ratio: 0.25, completion: gpt4oMiniDefaultCompletion, charge: 4500},
			} {
				t.Run(fmt.Sprintf("%s/%s/%s", host.name, protocol, tariff.name), func(t *testing.T) {
					const balance = int64(100_000)
					xaiVideoSetup(t, balance, false)
					upstream := newTariffUpstream(t, precedenceReply)
					base := host.base
					if base == "" {
						base = upstream.server.URL
					}
					path, body := lyriaProtocolRequest(protocol, false)
					c, _, requestID := protocolContext(t, host.channel, "gpt-4o-mini", path, body, base, balance, 1, false, tariff.local)
					c.Set(ctxkey.Config, host.config)
					clearNativeSettledLedger(t, requestID)

					apiErr := relayLyriaProtocol(c, protocol)
					drainCriticalTasks(t)
					require.Nil(t, apiErr)
					require.EqualValues(t, 1, upstream.calls.Load(), "the request is dispatched exactly once")
					obs := upstream.next(t)

					prompt := metalib.GetByContext(c).PromptTokens
					require.Positive(t, prompt, "admission must estimate the prompt")
					buffered := protocol == "chat" || (protocol == "responses" && !host.nativeResponses)
					wantReservation := tokenReservationQuote(buffered, prompt, lyriaMaxTokens, tariff.ratio, tariff.completion)
					require.Equal(t, wantReservation, balance-obs.UserQuota, "admission must reserve at the effective tariff")
					if tariff.local != nil {
						defaultReservation := tokenReservationQuote(buffered, prompt, lyriaMaxTokens, gpt4oMiniDefaultRatio, gpt4oMiniDefaultCompletion)
						require.NotEqual(t, defaultReservation, wantReservation, "an override must not be quoted at the provider default")
					}

					logQuota, costQuota := tariffRequestLedger(t, requestID)
					require.Equal(t, balance-tariff.charge, reloadUserQuota(t), "balance delta must equal the effective-tariff charge")
					require.Equal(t, tariff.charge, consumeLogQuota(t, requestID))
					require.Equal(t, tariff.charge, logQuota, "no provisional row may retain an extra charge")
					require.Equal(t, tariff.charge, costQuota, "the request cost must reconcile to the settled charge")
				})
			}
		}
	}
}

// TestEffectiveTariffOverrideCacheRatePrecedenceHTTP pins how a channel override
// interacts with the provider's cached-input discount on Azure gpt-4o-mini, for
// a receipt of 10,000 input tokens (4,000 cached) and 2,000 output tokens. Ratio
// and completion ratio fall back field by field, but cached_input_ratio does
// not: an override that omits it bills cached tokens at the override's full
// input price (never below it), and a cache-only override keeps the provider's
// input and output prices.
func TestEffectiveTariffOverrideCacheRatePrecedenceHTTP(t *testing.T) {
	useProductionGlobalPricing(t)
	const receipt = `{"id":"chatcmpl-precedence","object":"chat.completion","model":"gpt-4o-mini","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ack"}}],"usage":{"prompt_tokens":10000,"completion_tokens":2000,"total_tokens":12000,"prompt_tokens_details":{"cached_tokens":4000}}}`
	for _, tc := range []struct {
		name   string
		local  *model.ModelConfigLocal
		charge int64
	}{
		// Provider default: 6,000*0.075 + 4,000*0.0375 (cached $0.075/M) + 2,000*0.3.
		{name: "provider_default", charge: 1200},
		// Override without a cache price: all 10,000 input tokens at 0.25, plus 2,000*0.75.
		{name: "override_without_cache_rate", local: &model.ModelConfigLocal{Ratio: 0.5 * billingratio.MilliTokensUsd, CompletionRatio: 3}, charge: 4000},
		// Override with its own cache price ($0.25/M): 6,000*0.25 + 4,000*0.125 + 1,500.
		{name: "override_with_cache_rate", local: &model.ModelConfigLocal{Ratio: 0.5 * billingratio.MilliTokensUsd, CompletionRatio: 3, CachedInputRatio: 0.25 * billingratio.MilliTokensUsd}, charge: 3500},
		// Cache-only override ($0.025/M): 6,000*0.075 + 4,000*0.0125 + 600.
		{name: "cache_rate_only_override", local: &model.ModelConfigLocal{CachedInputRatio: 0.025 * billingratio.MilliTokensUsd}, charge: 1100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(100_000)
			xaiVideoSetup(t, balance, false)
			upstream := newTariffUpstream(t, func(string) (string, string) { return "application/json", receipt })
			path, body := lyriaProtocolRequest("chat", false)
			c, _, requestID := protocolContext(t, channeltype.Azure, "gpt-4o-mini", path, body, upstream.server.URL, balance, 1, false, tc.local)
			c.Set(ctxkey.Config, model.ChannelConfig{APIVersion: "2025-04-01-preview"})
			clearNativeSettledLedger(t, requestID)

			apiErr := relayLyriaProtocol(c, "chat")
			drainCriticalTasks(t)
			require.Nil(t, apiErr)
			require.EqualValues(t, 1, upstream.calls.Load())
			upstream.next(t)
			logQuota, costQuota := tariffRequestLedger(t, requestID)
			require.Equal(t, balance-tc.charge, reloadUserQuota(t))
			require.Equal(t, tc.charge, consumeLogQuota(t, requestID))
			require.Equal(t, tc.charge, logQuota)
			require.Equal(t, tc.charge, costQuota)
		})
	}
}
