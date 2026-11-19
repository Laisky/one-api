package router

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// e2eOpenRouterModel is an OpenRouter catalog model whose listed (top
// provider) price is far below other providers' prices for the same model.
const e2eOpenRouterModel = "z-ai/glm-5.2"

// e2eOpenRouterChannel is an OpenRouter channel; the fake upstream answers its
// real base URL.
var e2eOpenRouterChannel = e2eChannel{ID: 939701, Type: channeltype.OpenRouter, Key: "sk-or-e2e-fixture",
	Priority: 10, BaseURL: "https://openrouter.ai/api", Models: e2eOpenRouterModel}

// e2eOpenRouterCostJSON renders an OpenRouter usage cost fragment. Parameters:
// costUSD is the reported charge (negative omits the field). Returns: the
// fragment to splice after the token counters, starting with a comma.
func e2eOpenRouterCostJSON(costUSD float64) string {
	if costUSD < 0 {
		return ""
	}
	return fmt.Sprintf(`,"cost":%g,"is_byok":false,"cost_details":{"upstream_inference_cost":null}`, costUSD)
}

// e2eOpenRouterReply answers a Chat Completions call like OpenRouter: the usage
// carries the actually charged cost (JSON body, or the usage chunk before
// [DONE] when streaming). Parameters: call is the gateway request, u the token
// receipt and costUSD the reported cost (negative omits it). Returns: the reply.
func e2eOpenRouterReply(call e2eCall, u e2eUsage, costUSD float64) e2eReply {
	usage := strings.TrimSuffix(e2eChatUsageJSON(u), "}") + e2eOpenRouterCostJSON(costUSD) + "}"
	head := `{"id":"gen-e2e","object":"chat.completion.chunk","created":1767225600,"model":"` + e2eOpenRouterModel + `",`
	if call.Stream() {
		return e2eReply{ContentType: "text/event-stream", Body: "data: " + head + `"choices":[{"index":0,"delta":{"role":"assistant","content":"ack"},"finish_reason":null}]}` + "\n\n" +
			"data: " + head + `"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n" +
			"data: " + head + `"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":` + usage + "}\n\n" +
			"data: [DONE]\n\n"}
	}
	return e2eReply{Body: `{"id":"gen-e2e","object":"chat.completion","created":1767225600,"model":"` + e2eOpenRouterModel +
		`","choices":[{"index":0,"message":{"role":"assistant","content":"ack"},"finish_reason":"stop"}],"usage":` + usage + `}`}
}

// runE2EOpenRouter sends one request to the OpenRouter channel whose provider
// reports costUSD. Parameters: t owns the fixture, protocol and stream select
// the client format, u is the token receipt and costUSD the reported cost.
// Returns: the gateway, the client result and the ledger before the request.
func runE2EOpenRouter(t *testing.T, protocol string, stream bool, u e2eUsage, costUSD float64) (*e2eEnv, e2eResult, e2eLedger) {
	t.Helper()
	env := newE2EEnv(t, func(call e2eCall) e2eReply {
		return e2eOpenRouterReply(call, u, costUSD)
	}, e2eOpenRouterChannel)
	before := env.ledger(t)
	path, body := e2eClientRequest(protocol, e2eOpenRouterModel, stream, "")
	res := env.post(t, path, body)
	return env, res, before
}

// TestE2EOpenRouterReportedCostIsSettlementFloor proves OpenRouter requests
// never settle below the cost OpenRouter actually charged. OpenRouter lists
// each model at its top provider's price but load-balances across providers
// whose prices can be 10-200x higher; usage.cost reports the real charge. The
// settlement is max(catalog price, cost x group ratio): a cost above the
// catalog price is billed at cost, a cheaper one keeps the catalog price, and
// the operator's cost is never echoed to the client.
func TestE2EOpenRouterReportedCostIsSettlementFloor(t *testing.T) {
	receipt := e2eUsage{Input: 100_000, Output: 2_000}
	for _, protocol := range e2eProtocols {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", protocol, stream), func(t *testing.T) {
				// Control: the catalog price of the same receipt without a cost.
				env, res, before := runE2EOpenRouter(t, protocol, stream, receipt, -1)
				e2eRequireSuccessWire(t, protocol, stream, res)
				catalog := before.UserQuota - env.ledger(t).UserQuota
				require.Positive(t, catalog)

				// $10 is far above the catalog price: settle the real cost.
				env, res, before = runE2EOpenRouter(t, protocol, stream, receipt, 10)
				e2eRequireSuccessWire(t, protocol, stream, res)
				const costQuota = int64(10 * 500_000) // QuotaPerUsd = 500,000.
				require.Greater(t, costQuota, catalog, "control: the reported cost exceeds the catalog price")
				row := env.requireSettled(t, res, before, costQuota, false)
				require.Equal(t, "provider_reported_cost", row.Metadata["billing_source"], "the consume log must show the reported-cost floor applied")
				require.EqualValues(t, 10, row.Metadata["upstream_cost_usd"])
				require.NotContains(t, res.Body, `"cost"`, "the operator's provider cost must not reach the client")

				// A reported cost below the catalog price keeps the catalog price.
				env, res, before = runE2EOpenRouter(t, protocol, stream, receipt, 0.000001)
				e2eRequireSuccessWire(t, protocol, stream, res)
				row = env.requireSettled(t, res, before, catalog, false)
				require.NotContains(t, row.Metadata, "billing_source", "a catalog-priced charge carries no floor marker")
			})
		}
	}
}
