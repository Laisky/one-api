package router

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	// e2eCyberModel is native OpenAI gpt-6-cyber. Standard tariff: $20/M input
	// (ratio 10), $2/M cached input (ratio 1), $25/M 30m cache write (ratio
	// 12.5) and $100/M output (completion ratio 5, i.e. 50 quota per output
	// token). Requests whose input exceeds 272,000 tokens bill the long-context
	// tier: $40/M input (ratio 20), $4/M cached (ratio 2) and $150/M output (75
	// quota per output token), with no published cache-write price.
	e2eCyberModel = "gpt-6-cyber"
)

// e2eCyberReceiptCase is one gpt-6-cyber provider receipt and its expected charge.
type e2eCyberReceiptCase struct {
	name  string
	tier  string
	usage e2eUsage
	// charge is the exact charge of a priced receipt, or, for an unknown one,
	// the catalog price of its priced part.
	charge  int64
	unknown bool
}

// e2eCyberReceipts are the success receipts. charge is computed by hand from
// the catalog constants. unknown marks receipts the catalog cannot fully price:
// settlement flags them unpriced_usage_retained_reservation and applies the
// estimate floor of retainUnpricedUsage (relay/controller/billing_usage.go),
// i.e. it charges max(admission reservation, catalog price of the priced part).
var e2eCyberReceipts = []e2eCyberReceiptCase{
	{
		// 14,000 uncached x 10 = 140,000; 4,000 cached x 1 = 4,000;
		// 2,000 30m writes x 12.5 = 25,000; 4,000 output x 50 = 200,000.
		name: "standard", tier: "default",
		usage:  e2eUsage{Input: 20_000, Cached: 4_000, CacheWrite: 2_000, Output: 4_000},
		charge: 369_000,
	},
	{
		// 300,000 input selects the long-context tier: 200,000 uncached x 20 =
		// 4,000,000; 100,000 cached x 2 = 200,000; 1,000 output x 75 = 75,000.
		name: "long_context", tier: "default",
		usage:  e2eUsage{Input: 300_000, Cached: 100_000, Output: 1_000},
		charge: 4_275_000,
	},
	{
		// OpenAI publishes no long-context cache-write price, so the estimate
		// prices the write at OpenAI's documented generic 30m rule (1.25x the
		// $40/M uncached input = $50/M, ratio 25), never at the cheaper
		// inherited short-context rate: 199,000 uncached x 20 = 3,980,000;
		// 100,000 cached x 2 = 200,000; 1,000 writes x 25 = 25,000; 1,000
		// output x 75 = 75,000. It exceeds the reservation.
		name: "long_context_cache_write_unpriced", tier: "default",
		usage:   e2eUsage{Input: 300_000, Cached: 100_000, CacheWrite: 1_000, Output: 1_000},
		charge:  4_280_000,
		unknown: true,
	},
	{
		// The provider reports an actual tier the catalog does not price. At
		// Standard prices: 16,000 x 10 + 4,000 x 1 + 4,000 x 50 = 364,000, above
		// the reservation.
		name: "reported_priority_tier_unpriced", tier: "priority",
		usage:   e2eUsage{Input: 20_000, Cached: 4_000, Output: 4_000},
		charge:  364_000,
		unknown: true,
	},
	{
		name: "reported_flex_tier_unpriced", tier: "flex",
		usage:   e2eUsage{Input: 20_000, Cached: 4_000, Output: 4_000},
		charge:  364_000,
		unknown: true,
	},
	{
		// A small unpriced receipt (20 x 10 + 2 x 50 = 300) stays below the
		// reservation, so the reservation is retained.
		name: "reported_priority_tier_small_unpriced", tier: "priority",
		usage:   e2eUsage{Input: 20, Output: 2},
		charge:  300,
		unknown: true,
	},
}

// TestE2ECyberReceiptSettlement drives gpt-6-cyber on a native OpenAI channel
// through every client protocol and delivery mode. Every entrypoint must reach
// the native /v1/responses route pinned to service_tier "default"; a Standard
// receipt (short and long context) settles exactly, and a receipt the catalog
// cannot price retains the admission reservation as a flagged estimate.
func TestE2ECyberReceiptSettlement(t *testing.T) {
	for _, receipt := range e2eCyberReceipts {
		for _, protocol := range e2eProtocols {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", receipt.name, protocol, stream), func(t *testing.T) {
					env := newE2EEnv(t, func(call e2eCall) e2eReply {
						return e2eSuccess(call, e2eCyberModel, receipt.tier, receipt.usage)
					}, e2eNativeChannel(939501, e2eNativeKey, 0, e2eCyberModel, ""))
					before := env.ledger(t)
					path, body := e2eClientRequest(protocol, e2eCyberModel, stream, "")

					res := env.post(t, path, body)

					e2eRequireSuccessWire(t, protocol, stream, res)
					calls := env.upstream.callsFor(e2eNativeKey)
					require.Len(t, calls, 1, "exactly one upstream dispatch")
					require.Equal(t, "/v1/responses", calls[0].Path, "gpt-6-cyber is served by the native Responses route")
					require.Equal(t, e2eCyberModel, calls[0].Body["model"])
					require.Equal(t, "default", calls[0].Body["service_tier"], "every entrypoint must pin the priced Standard tier")
					reserved := before.UserQuota - calls[0].UserQuota
					require.Positive(t, reserved, "admission must reserve before dispatch")
					charge := receipt.charge
					if receipt.unknown {
						charge = max(reserved, receipt.charge)
					}
					env.requireSettled(t, res, before, charge, receipt.unknown)
				})
			}
		}
	}
}

// e2eCyberTierRequest sends one gpt-6-cyber request asking for tier through
// a gateway with two native channels, so a retry would be visible. Parameters:
// t owns the fixture, protocol/stream describe the request and tier is the
// client service_tier. Returns: the env, the client result and the ledger
// snapshot taken before the request.
func e2eCyberTierRequest(t *testing.T, protocol string, stream bool, tier string) (*e2eEnv, e2eResult, e2eLedger) {
	t.Helper()
	env := newE2EEnv(t, func(call e2eCall) e2eReply {
		return e2eSuccess(call, e2eCyberModel, "default", e2eCyberReceipts[0].usage)
	}, e2eNativeChannel(939511, e2eNativeKey, 10, e2eCyberModel, ""), e2eNativeChannel(939512, e2eNativeBackupKey, 5, e2eCyberModel, ""))
	before := env.ledger(t)
	path, body := e2eClientRequest(protocol, e2eCyberModel, stream, `,"service_tier":"`+tier+`"`)
	return env, env.post(t, path, body), before
}

// TestE2ECyberClientServiceTierRefused proves that a client asking gpt-6-cyber
// for an unpriced service tier is refused with HTTP 400 (the value is the
// caller's) before any provider contact on any channel and is charged nothing,
// on the protocols that carry an OpenAI service_tier field (Chat Completions
// and Responses). An explicit "default" is accepted as the control.
func TestE2ECyberClientServiceTierRefused(t *testing.T) {
	for _, protocol := range []string{e2eChat, e2eResponses} {
		for _, stream := range []bool{false, true} {
			for _, tier := range []string{"priority", "flex", "auto", "default"} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", protocol, stream, tier), func(t *testing.T) {
					env, res, before := e2eCyberTierRequest(t, protocol, stream, tier)
					if tier == "default" {
						e2eRequireSuccessWire(t, protocol, stream, res)
						calls := env.upstream.callsFor(e2eNativeKey)
						require.Len(t, calls, 1)
						require.Equal(t, "default", calls[0].Body["service_tier"])
						require.Empty(t, env.upstream.callsFor(e2eNativeBackupKey))
						env.requireSettled(t, res, before, e2eCyberReceipts[0].charge, false)
						return
					}
					errObject := e2eRequireErrorWire(t, protocol, res, http.StatusBadRequest)
					require.Contains(t, errObject["message"], "service_tier", "the refusal must name the unsupported field")
					require.Contains(t, errObject["message"], tier)
					require.Zero(t, env.upstream.total(), "an unpriced tier must never reach any provider")
					env.requireNothingCharged(t, res, before)
				})
			}
		}
	}
}

// TestE2ECyberClaudeServiceTierPinned proves a Claude Messages client's
// service_tier (an Anthropic field whose values differ from OpenAI's) is never
// translated into an unpriced OpenAI tier: the upstream request is pinned to
// "default" and the Standard receipt settles exactly.
func TestE2ECyberClaudeServiceTierPinned(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tier := range []string{"priority", "auto", "standard_only"} {
			t.Run(fmt.Sprintf("stream=%t/%s", stream, tier), func(t *testing.T) {
				env, res, before := e2eCyberTierRequest(t, e2eMessages, stream, tier)
				e2eRequireSuccessWire(t, e2eMessages, stream, res)
				calls := env.upstream.callsFor(e2eNativeKey)
				require.Len(t, calls, 1)
				require.Equal(t, "default", calls[0].Body["service_tier"])
				require.Empty(t, env.upstream.callsFor(e2eNativeBackupKey))
				env.requireSettled(t, res, before, e2eCyberReceipts[0].charge, false)
			})
		}
	}
}

// TestE2ECyberWebSocketRefused proves the Responses WebSocket handshake for
// gpt-6-cyber is refused before any reservation or provider contact, because
// frame-level billing for the HTTP-only policy is not qualified.
func TestE2ECyberWebSocketRefused(t *testing.T) {
	env := newE2EEnv(t, func(call e2eCall) e2eReply {
		return e2eSuccess(call, e2eCyberModel, "default", e2eCyberReceipts[0].usage)
	}, e2eNativeChannel(939521, e2eNativeKey, 0, e2eCyberModel, ""))
	before := env.ledger(t)
	header := http.Header{}
	header.Set("Connection", "Upgrade")
	header.Set("Upgrade", "websocket")
	header.Set("Sec-WebSocket-Version", "13")
	header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	res := env.do(t, http.MethodGet, "/v1/responses?model="+e2eCyberModel, "", header)

	errObject := e2eRequireErrorWire(t, e2eResponses, res, http.StatusBadRequest)
	require.Equal(t, "response_websocket_model_billing_not_supported", errObject["code"], "body=%s", res.Body)
	require.Zero(t, env.upstream.total(), "the refused handshake must not reach the provider")
	env.requireNothingCharged(t, res, before)
}
