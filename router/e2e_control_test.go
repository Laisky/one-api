package router

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

const (
	// e2eMiniModel is the control model: native OpenAI gpt-4o-mini at $0.15/M
	// input (ratio 0.075), $0.075/M cached input (ratio 0.0375) and $0.60/M
	// output (completion ratio 4, i.e. 0.3 quota per output token).
	e2eMiniModel = "gpt-4o-mini"
	// e2eNativeKey and e2eNativeBackupKey are the credentials of the primary and
	// secondary native OpenAI channels.
	e2eNativeKey       = "e2e-native-primary"
	e2eNativeBackupKey = "e2e-native-secondary"
)

// e2eMiniReceipt is the control receipt: 20,000 input tokens of which 4,000
// are cached, and 4,000 output tokens.
var e2eMiniReceipt = e2eUsage{Input: 20_000, Output: 4_000, Cached: 4_000}

// e2eMiniCharge is e2eMiniReceipt priced by hand from the catalog constants
// (ratio.MilliTokensUsd = 0.5 quota per token per $1/M):
// 16,000 uncached x 0.075 = 1,200; 4,000 cached x 0.0375 = 150;
// 4,000 output x 0.075 x 4 = 1,200. Total 2,550.
const e2eMiniCharge = int64(2_550)

// e2eNativeChannel returns a native OpenAI channel row serving models.
func e2eNativeChannel(id int, key string, priority int64, models, modelConfigs string) e2eChannel {
	return e2eChannel{ID: id, Type: channeltype.OpenAI, Key: key, Priority: priority, BaseURL: e2eNativeOpenAIBase, Models: models, ModelConfigs: modelConfigs}
}

// TestE2EControlMiniSettlesExactly drives gpt-4o-mini on a native OpenAI
// channel through every client protocol and delivery mode. A complete receipt
// must settle exactly on every ledger surface and reach the client in its own
// protocol, so a regression elsewhere in the suite is visible against this
// baseline. Responses clients use the native /v1/responses route.
func TestE2EControlMiniSettlesExactly(t *testing.T) {
	for _, protocol := range e2eProtocols {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", protocol, stream), func(t *testing.T) {
				env := newE2EEnv(t, func(call e2eCall) e2eReply {
					return e2eSuccess(call, e2eMiniModel, "", e2eMiniReceipt)
				}, e2eNativeChannel(939401, e2eNativeKey, 0, e2eMiniModel, ""))
				before := env.ledger(t)
				path, body := e2eClientRequest(protocol, e2eMiniModel, stream, "")

				res := env.post(t, path, body)

				e2eRequireSuccessWire(t, protocol, stream, res)
				if protocol == e2eChat && !stream {
					e2eRequireChatIdentity(t, res, e2eMiniModel)
				}
				calls := env.upstream.callsFor(e2eNativeKey)
				require.Len(t, calls, 1, "exactly one upstream dispatch")
				require.Equal(t, e2eMiniModel, calls[0].Body["model"])
				if protocol == e2eResponses {
					require.Equal(t, "/v1/responses", calls[0].Path, "a Responses client uses the native route")
				}
				require.Less(t, calls[0].UserQuota, before.UserQuota, "admission must reserve before dispatch")
				env.requireSettled(t, res, before, e2eMiniCharge, false)
			})
		}
	}
}
