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
)

// tierScheduleReply answers chat-completions and Responses dispatches with a
// receipt of prompt input and completion output tokens. Parameters: name is
// the model and prompt/completion the reported usage. Returns: the reply func.
func tierScheduleReply(name string, prompt, completion int) func(string) (string, string) {
	return func(path string) (string, string) {
		if strings.HasSuffix(path, "/responses") {
			return "application/json", fmt.Sprintf(`{"id":"resp-tier","object":"response","created_at":1,"status":"completed","model":%q,"output":[{"type":"message","id":"msg-tier","status":"completed","role":"assistant","content":[{"type":"output_text","text":"ack","annotations":[]}]}],"usage":{"input_tokens":%d,"output_tokens":%d,"total_tokens":%d}}`,
				name, prompt, completion, prompt+completion)
		}
		return "application/json", fmt.Sprintf(`{"id":"chatcmpl-tier","object":"chat.completion","model":%q,"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ack"}}],"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d}}`,
			name, prompt, completion, prompt+completion)
	}
}

// TestChannelTierScheduleSettlementHTTP proves, through the production chat
// controller, a fake TLS provider and the SQLite ledger, that a channel's
// model_configs schedule settles at its tier prices: (a) an entry with its own
// ratio and tiers bills the tier input ratio, which a scalar ratio map copied
// from the entry previously suppressed; (b) an entry that inherits the
// provider's base ratio (metadata only) inherits the provider's long-context
// tier. Balance delta == consume log == request cost == the hand-priced charge.
func TestChannelTierScheduleSettlementHTTP(t *testing.T) {
	useProductionGlobalPricing(t)
	m := billingratio.MilliTokensUsd
	for _, tc := range []struct {
		name, model        string
		local              *model.ModelConfigLocal
		prompt, completion int
		// charge is the receipt priced by hand; wrong is the pre-fix charge.
		charge, wrong int64
	}{
		// $0.50/M base, $1/M from 5,000 input tokens: 10,000*0.5 + 2,000*0.5*3.
		{name: "channel_ratio_with_tiers", model: "gpt-4o-mini", prompt: 10_000, completion: 2_000, charge: 8_000, wrong: 4_000,
			local: &model.ModelConfigLocal{Ratio: 0.5 * m, CompletionRatio: 3, Tiers: []model.ModelRatioTierLocal{{InputTokenThreshold: 5_000, Ratio: 1 * m}}}},
		// gpt-5.5-pro above 272K input is $60/$270: 300,000*30 + 1,000*135.
		{name: "metadata_entry_inherits_provider_tiers", model: "gpt-5.5-pro", prompt: 300_000, completion: 1_000, charge: 9_135_000, wrong: 4_590_000,
			local: &model.ModelConfigLocal{MaxTokens: 4096}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(20_000_000)
			xaiVideoSetup(t, balance, false)
			upstream := newTariffUpstream(t, tierScheduleReply(tc.model, tc.prompt, tc.completion))
			path, body := lyriaProtocolRequest("chat", false)
			c, _, requestID := protocolContext(t, channeltype.Azure, tc.model, path, body, upstream.server.URL, balance, 1, false, tc.local)
			c.Set(ctxkey.Config, model.ChannelConfig{APIVersion: "2025-04-01-preview"})
			clearNativeSettledLedger(t, requestID)

			apiErr := relayLyriaProtocol(c, "chat")
			drainCriticalTasks(t)
			require.Nil(t, apiErr)
			require.EqualValues(t, 1, upstream.calls.Load(), "the request is dispatched exactly once")
			upstream.next(t)

			logQuota, costQuota := tariffRequestLedger(t, requestID)
			require.NotEqual(t, tc.wrong, consumeLogQuota(t, requestID), "the tier schedule must not be dropped")
			require.Equal(t, balance-tc.charge, reloadUserQuota(t), "balance delta must equal the tier charge")
			require.Equal(t, tc.charge, consumeLogQuota(t, requestID))
			require.Equal(t, tc.charge, logQuota, "no provisional row may retain an extra charge")
			require.Equal(t, tc.charge, costQuota, "the request cost must reconcile to the settled charge")
		})
	}
}
