package router

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/relay/channeltype"
)

const (
	// e2eAIProxyKey and e2eAIProxyBackupKey are AIProxy library credentials.
	e2eAIProxyKey       = "e2e-aiproxy-primary"
	e2eAIProxyBackupKey = "e2e-aiproxy-secondary"
	// e2eCyberOperatorTariff is a deliberately non-native operator price for
	// gpt-6-cyber: $4/M input (ratio 2) and $24/M output (completion ratio 6,
	// i.e. 12 quota per output token). It sets no cached_input_ratio, so cached
	// input bills at the entry's own input price. Native OpenAI is $20/$100.
	e2eCyberOperatorTariff = `{"gpt-6-cyber":{"ratio":2,"completion_ratio":6}}`
)

// e2eOperatorReceipt is the Azure chat receipt for gpt-6-cyber.
var e2eOperatorReceipt = e2eUsage{Input: 20_000, Cached: 4_000, Output: 4_000}

// e2eOperatorCharge prices e2eOperatorReceipt at e2eCyberOperatorTariff:
// 20,000 input (cached included, no cache price) x 2 = 40,000 plus 4,000
// output x 12 = 48,000. Total 88,000. Native pricing would be 364,000.
const e2eOperatorCharge = int64(88_000)

// e2eOperatorHost is one host that inherits OpenAI's catalog but has no
// provider tariff for native-only OpenAI models.
type e2eOperatorHost struct {
	name, primary, secondary string
	// channel builds a channel row of this host serving models.
	channel func(id int, key string, priority int64, models, modelConfigs string) e2eChannel
	// path is the upstream path a text request is dispatched to.
	path string
	// receiptUsage reports whether the host's replies carry token usage. The
	// AIProxy library API reports none, so its documented settlement retains
	// the operator-priced reservation.
	receiptUsage bool
}

// e2eOperatorHosts are Azure and the AIProxy library.
var e2eOperatorHosts = []e2eOperatorHost{
	{
		name: "azure", primary: e2eAzureKey, secondary: e2eAzureBackupKey, receiptUsage: true,
		path: "/openai/deployments/" + e2eCyberModel + "/chat/completions",
		channel: func(id int, key string, priority int64, models, modelConfigs string) e2eChannel {
			return e2eChannel{ID: id, Type: channeltype.Azure, Key: key, Priority: priority, BaseURL: e2eAzureBase, Models: models, Config: e2eAzureConfig, ModelConfigs: modelConfigs}
		},
	},
	{
		name: "aiproxy_library", primary: e2eAIProxyKey, secondary: e2eAIProxyBackupKey,
		path: "/api/library/ask",
		channel: func(id int, key string, priority int64, models, modelConfigs string) e2eChannel {
			return e2eChannel{ID: id, Type: channeltype.AIProxyLibrary, Key: key, Priority: priority, BaseURL: e2eAIProxyBase, Models: models, Config: e2eAIProxyConfig, ModelConfigs: modelConfigs}
		},
	},
}

// e2eOperatorChannels returns the channel pair of one operator-tariff case:
// a primary priced by primaryConfigs and, when withSecondary, a lower-priority
// secondary priced by e2eCyberOperatorTariff.
func e2eOperatorChannels(host e2eOperatorHost, primaryConfigs string, withSecondary bool) []e2eChannel {
	channels := []e2eChannel{host.channel(939701, host.primary, 10, e2eCyberModel, primaryConfigs)}
	if withSecondary {
		channels = append(channels, host.channel(939702, host.secondary, 5, e2eCyberModel, e2eCyberOperatorTariff))
	}
	return channels
}

// e2eRequireOperatorSettled asserts a priced dispatch of gpt-6-cyber on host
// settled at the operator tariff. Parameters: t owns assertions, env/res/before
// describe the request, host the route and call the single dispatch.
func e2eRequireOperatorSettled(t *testing.T, env *e2eEnv, res e2eResult, before e2eLedger, host e2eOperatorHost, call e2eCall) {
	t.Helper()
	require.Equal(t, host.path, call.Path)
	require.Contains(t, string(call.Raw), `"model":"`+e2eCyberModel+`"`, "the provider receives the mapped native-only model")
	reserved := before.UserQuota - call.UserQuota
	require.Positive(t, reserved, "admission must reserve before dispatch")
	// The reservation is quoted at the operator tariff: input (with any
	// PreConsumedQuota buffer) at ratio 2 plus max_tokens at 12 per token. The
	// prompt estimate of the short fixture is far below 1,000 tokens.
	require.LessOrEqual(t, reserved, (config.PreConsumedQuota+1_000)*2+e2eMaxTokens*12, "the reservation must use the operator tariff, never native $20/$100")
	charge, estimated := e2eOperatorCharge, false
	if !host.receiptUsage {
		charge = reserved
		estimated = true
	}
	env.requireSettled(t, res, before, charge, estimated)
}

// TestE2EOperatorTariffNativeOnlyText drives gpt-6-cyber on Azure and the
// AIProxy library, for every text protocol, through the shipped retry loop.
// Without an operator tariff in channel model_configs the request is refused
// before any provider contact and nothing is charged; when no other channel
// can serve it, the client gets 503 model_pricing_not_configured. With a
// complete operator tariff the request settles at the operator price, never at
// the native $20/$100 tariff. A priced lower-priority secondary is reached when
// the unpriced primary refuses, and the refusal consumes no retry slot.
func TestE2EOperatorTariffNativeOnlyText(t *testing.T) {
	for _, host := range e2eOperatorHosts {
		for _, protocol := range e2eProtocols {
			for _, tc := range []struct {
				name           string
				primaryConfigs string
				withSecondary  bool
			}{
				{name: "unpriced"},
				{name: "incomplete_tariff", primaryConfigs: `{"gpt-6-cyber":{"ratio":2}}`},
				{name: "priced", primaryConfigs: e2eCyberOperatorTariff},
				{name: "unpriced_fails_over", withSecondary: true},
			} {
				t.Run(fmt.Sprintf("%s/%s/%s", host.name, protocol, tc.name), func(t *testing.T) {
					env := newE2EEnv(t, func(call e2eCall) e2eReply {
						return e2eSuccess(call, e2eCyberModel, "", e2eOperatorReceipt)
					}, e2eOperatorChannels(host, tc.primaryConfigs, tc.withSecondary)...)
					before := env.ledger(t)
					path, body := e2eClientRequest(protocol, e2eCyberModel, false, "")

					res := env.post(t, path, body)

					switch tc.name {
					case "priced":
						e2eRequireSuccessWire(t, protocol, false, res)
						calls := env.upstream.callsFor(host.primary)
						require.Len(t, calls, 1, "a priced request is dispatched exactly once")
						e2eRequireOperatorSettled(t, env, res, before, host, calls[0])
					case "unpriced_fails_over":
						e2eRequireSuccessWire(t, protocol, false, res)
						require.Empty(t, env.upstream.callsFor(host.primary), "the unpriced primary must never be contacted")
						calls := env.upstream.callsFor(host.secondary)
						require.Len(t, calls, 1, "the priced secondary serves the request exactly once")
						e2eRequireOperatorSettled(t, env, res, before, host, calls[0])
					default:
						errObject := e2eRequireErrorWire(t, protocol, res, http.StatusServiceUnavailable)
						require.Equal(t, "model_pricing_not_configured", errObject["code"], "body=%s", res.Body)
						require.Contains(t, errObject["message"], `model "gpt-6-cyber" requires explicit positive input and output pricing in channel model_configs`)
						require.Zero(t, env.upstream.total(), "no provider may be contacted without an operator tariff")
						env.requireNothingCharged(t, res, before)
					}
				})
			}
		}
	}
}
