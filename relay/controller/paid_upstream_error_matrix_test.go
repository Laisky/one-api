package controller

import (
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
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// paidMatrixStatuses are the provider error statuses the matrix exercises:
// a request rejection, a rate limit and a server failure.
var paidMatrixStatuses = []int{http.StatusBadRequest, http.StatusTooManyRequests, http.StatusInternalServerError}

// Hand-computed receipt prices with ratio.MilliTokensUsd = 0.5 quota per
// token per $1/M and group ratio 1 (quota = tokens x ratio):
//   - gpt-4o-mini: $0.15/M input (ratio 0.075) and $0.60/M output (ratio 0.3),
//     so 20,000 x 0.075 + 4,000 x 0.3 = 1,500 + 1,200 = 2,700.
//   - gpt-6-cyber: $20/M input (ratio 10) and $100/M output (ratio 50),
//     so 20,000 x 10 + 4,000 x 50 = 200,000 + 200,000 = 400,000.
const (
	paidMatrixMiniPrice  = int64(2_700)
	paidMatrixCyberPrice = int64(400_000)
)

// paidMatrixReceipt is the explicit usage receipt one provider error body
// carries, rendered in the wire shape of the surface that was called.
type paidMatrixReceipt struct {
	// tier is the provider-reported service tier; empty omits the field.
	tier string
	// prompt and completion are the receipt's input and output counters.
	prompt, completion int
	// details is the body of the input (Responses) or prompt (Chat) token
	// details object, without braces; empty omits the object.
	details string
}

// responsesUsage renders the receipt as a Responses usage object. It takes no
// parameters and returns the JSON text.
func (r paidMatrixReceipt) responsesUsage() string {
	details := ""
	if r.details != "" {
		details = `,"input_tokens_details":{` + r.details + `}`
	}
	return fmt.Sprintf(`{"input_tokens":%d,"output_tokens":%d,"total_tokens":%d%s}`, r.prompt, r.completion, r.prompt+r.completion, details)
}

// chatUsage renders the receipt as a Chat Completions usage object. It takes
// no parameters and returns the JSON text.
func (r paidMatrixReceipt) chatUsage() string {
	details := ""
	if r.details != "" {
		details = `,"prompt_tokens_details":{` + r.details + `}`
	}
	return fmt.Sprintf(`{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d%s}`, r.prompt, r.completion, r.prompt+r.completion, details)
}

// tierField renders the optional root service_tier member. It takes no
// parameters and returns the JSON member followed by a comma, or "".
func (r paidMatrixReceipt) tierField() string {
	if r.tier == "" {
		return ""
	}
	return fmt.Sprintf(`"service_tier":%q,`, r.tier)
}

// paidMatrixStandardReceipt returns the matrix's Standard receipt of 20,000
// input and 4,000 output tokens. It takes no parameters.
func paidMatrixStandardReceipt() *paidMatrixReceipt {
	return &paidMatrixReceipt{prompt: 20_000, completion: 4_000}
}

// paidMatrixErrorObject renders the OpenAI error object for status. Parameters:
// status is the provider HTTP status. Returns: the JSON error object.
func paidMatrixErrorObject(status int) string {
	errType, code := "server_error", "server_error"
	switch status {
	case http.StatusBadRequest:
		errType, code = "invalid_request_error", "invalid_request"
	case http.StatusTooManyRequests:
		errType, code = "rate_limit_error", "rate_limit_exceeded"
	}
	return fmt.Sprintf(`{"message":"generation failed","type":%q,"code":%q}`, errType, code)
}

// paidMatrixUpstream returns a TLS fake provider that answers every request
// with status. A nil receipt answers a plain error object; otherwise /responses
// answers a failed Responses envelope and other routes a Chat-style error body,
// each carrying the receipt. Parameters: t owns cleanup, status is the provider
// status, receipt the optional receipt, modelName is echoed, and calls counts
// real HTTP requests and records the last request path. Returns: the server.
func paidMatrixUpstream(t *testing.T, status int, receipt *paidMatrixReceipt, modelName string, calls *atomic.Int32, path *atomic.Value) *httptest.Server {
	t.Helper()
	errObject := paidMatrixErrorObject(status)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		path.Store(r.URL.Path)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		switch {
		case receipt == nil:
			_, _ = fmt.Fprintf(w, `{"error":%s}`, errObject)
		case strings.HasSuffix(r.URL.Path, "/responses"):
			_, _ = fmt.Fprintf(w, `{"id":"resp_paid_matrix","object":"response","status":"failed","model":%q,%s"error":%s,`+
				`"output":[{"type":"message","id":"msg_paid_matrix","role":"assistant","content":[{"type":"output_text","text":"partial"}]}],"usage":%s}`,
				modelName, receipt.tierField(), errObject, receipt.responsesUsage())
		default:
			_, _ = fmt.Fprintf(w, `{"error":%s,%s"usage":%s}`, errObject, receipt.tierField(), receipt.chatUsage())
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// paidMatrixOutcome is what one relay attempt left in the response and ledger.
type paidMatrixOutcome struct {
	apiErr       *relaymodel.ErrorWithStatusCode
	calls        int32
	path         string
	held         int64
	userCharged  int64
	tokenCharged int64
	logQuota     int64
	requestCost  int64
	settled      bool
	retryable    bool
	logs         []model.Log
}

// estimateReason returns the single consume log's billing_estimate_reason, or
// "" when the attempt logged no consume entry. It takes no parameters.
func (o paidMatrixOutcome) estimateReason() string {
	if len(o.logs) != 1 {
		return ""
	}
	reason, _ := o.logs[0].Metadata["billing_estimate_reason"].(string)
	return reason
}

// relayPaidMatrixAttempt drives one request through the production helper for
// protocol against a provider that fails with status. It mirrors
// relayPaidErrorAttempt with a status parameter and the full ledger view.
// Parameters: t owns the fixture, channel and modelName select the route,
// protocol is chat/responses/responses_native/messages, status is the provider
// status and receipt the optional error-body receipt. Returns: the outcome.
func relayPaidMatrixAttempt(t *testing.T, channel int, modelName, protocol string, status int, receipt *paidMatrixReceipt) paidMatrixOutcome {
	t.Helper()
	const balance = int64(10_000_000)
	xaiVideoSetup(t, balance, false)
	var calls atomic.Int32
	var upstreamPath atomic.Value
	upstream := paidMatrixUpstream(t, status, receipt, modelName, &calls, &upstreamPath)
	previous := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = previous })

	// responses_native is the client Responses protocol on the provider's native
	// Responses route; plain responses on a custom base URL uses the Chat fallback.
	clientProtocol := strings.TrimSuffix(protocol, "_native")
	path, body := lyriaProtocolRequest(clientProtocol, false)
	c, _, requestID := protocolContext(t, channel, modelName, path, body, upstream.URL, balance, 1, false, nil)
	if channel == channeltype.Azure {
		c.Set(ctxkey.Config, model.ChannelConfig{APIVersion: "2025-04-01-preview"})
	}
	if protocol == "responses_native" {
		c.Set(ctxkey.BaseURL, "https://api.openai.com")
		c.Set(ctxkey.Config, model.ChannelConfig{EndpointURLs: map[string]string{
			channeltype.RelayModeToEndpointName(relaymode.ResponseAPI): upstream.URL + "/v1/responses",
		}})
	}
	clearNativeSettledLedger(t, requestID)
	if protocol == "responses_native" {
		require.True(t, supportsNativeResponseAPI(metalib.GetByContext(c)), "control: the native Responses route must be selected")
	} else if clientProtocol == "responses" && channel == channeltype.OpenAI {
		require.False(t, supportsNativeResponseAPI(metalib.GetByContext(c)), "control: the custom base URL selects the Chat fallback")
	}

	apiErr := relayLyriaProtocol(c, clientProtocol)
	drainCriticalTasks(t)
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	calledPath, _ := upstreamPath.Load().(string)
	if protocol == "responses_native" {
		require.True(t, strings.HasSuffix(calledPath, "/responses"), "control: the native route calls Responses, got %q", calledPath)
	} else if clientProtocol == "responses" {
		require.False(t, strings.HasSuffix(calledPath, "/responses"), "control: the fallback calls Chat Completions, got %q", calledPath)
	}
	out := paidMatrixOutcome{
		apiErr: apiErr, calls: calls.Load(), path: calledPath, held: c.GetInt64(ctxkey.PreConsumedQuotaAmount),
		userCharged: balance - reloadUserQuota(t), tokenCharged: balance - token.RemainQuota,
		logQuota: consumeLogQuota(t, requestID), requestCost: requestCostQuota(t, requestID),
		settled: c.GetBool(responseSettlementKey), retryable: BillingAllowsRetry(c),
	}
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&out.logs).Error)
	return out
}

// requirePaidMatrixCharge asserts every ledger view of a settled paid error
// equals want and that the attempt can never be replayed. Parameters: t
// asserts, out is the outcome and want the expected quota.
func requirePaidMatrixCharge(t *testing.T, out paidMatrixOutcome, want int64) {
	t.Helper()
	require.Equal(t, want, out.userCharged, "user balance delta")
	require.Equal(t, want, out.tokenCharged, "token remain_quota delta")
	require.Len(t, out.logs, 1, "a paid error settles exactly one consume log")
	require.Equal(t, want, out.logQuota, "consume log quota")
	require.Equal(t, want, out.requestCost, "user_request_costs quota")
	require.True(t, out.settled, "a settled paid attempt must be marked for settlement")
	require.False(t, out.retryable, "a paid attempt must never be replayed on another channel")
}

// paidMatrixRoutes are the OpenAI-wire provider routes whose error receipts
// are trusted, each with its hand-computed Standard receipt price.
var paidMatrixRoutes = []struct {
	channel int
	model   string
	price   int64
}{
	{channeltype.OpenAI, "gpt-4o-mini", paidMatrixMiniPrice},
	{channeltype.OpenAI, "gpt-6-cyber", paidMatrixCyberPrice},
	{channeltype.Azure, "gpt-4o-mini", paidMatrixMiniPrice},
}

// paidMatrixProtocols returns the client protocols a channel serves. Azure
// gpt-4o-mini has no native Responses route (supportsNativeResponseAPI is
// false for it), so responses_native applies to native OpenAI only.
// Parameters: channel is the channel type. Returns: the protocol names.
func paidMatrixProtocols(channel int) []string {
	if channel == channeltype.OpenAI {
		return []string{"chat", "responses", "responses_native", "messages"}
	}
	return []string{"chat", "responses", "messages"}
}

// TestPaidUpstreamErrorReceiptMatrix drives provider HTTP 400, 429 and 500
// errors, with and without an explicit usage receipt in the body, through every
// client protocol for native OpenAI gpt-4o-mini, native OpenAI gpt-6-cyber and
// Azure gpt-4o-mini. The client must see the provider status after exactly one
// upstream call. A receipt settles exactly its hand-computed price in the user
// balance, token quota, consume log and user_request_costs, and vetoes retry.
// A plain 4xx proves the provider rejected the request before inference, so
// its admission hold is refunded; a plain 5xx keeps only the retained hold.
// Either way it records no consume log and a zero request cost, and stays
// retryable.
func TestPaidUpstreamErrorReceiptMatrix(t *testing.T) {
	for _, status := range paidMatrixStatuses {
		for _, route := range paidMatrixRoutes {
			for _, protocol := range paidMatrixProtocols(route.channel) {
				for _, withReceipt := range []bool{true, false} {
					name := fmt.Sprintf("%d/%s/%s/%s/receipt=%t", status, channeltype.IdToName(route.channel), route.model, protocol, withReceipt)
					t.Run(name, func(t *testing.T) {
						var receipt *paidMatrixReceipt
						if withReceipt {
							receipt = paidMatrixStandardReceipt()
						}
						out := relayPaidMatrixAttempt(t, route.channel, route.model, protocol, status, receipt)
						t.Logf("provider route called: %s", out.path)
						require.NotNil(t, out.apiErr, "the provider error must reach the client")
						require.Equal(t, status, out.apiErr.StatusCode, "the client status must be the provider status")
						require.EqualValues(t, 1, out.calls, "exactly one upstream call")
						if withReceipt {
							requirePaidMatrixCharge(t, out, route.price)
							require.Empty(t, out.estimateReason(), "a Standard receipt settles exactly")
							return
						}
						require.Positive(t, out.held, "control: admission reserved quota")
						require.Less(t, out.held, route.price, "control: the hold is smaller than a receipt")
						if status < http.StatusInternalServerError {
							require.Zero(t, out.userCharged, "a provider 4xx rejection refunds the admission hold")
							require.Zero(t, out.tokenCharged, "the token hold is refunded too")
						} else {
							require.Equal(t, out.held, out.userCharged, "an error without usage keeps only the retained admission hold")
							require.Equal(t, out.held, out.tokenCharged, "the token keeps the same retained hold")
						}
						for _, row := range out.logs {
							// A refunded 4xx reconciles its provisional row to zero.
							require.Zero(t, row.Quota, "an unpaid error never logs a charge: %+v", row)
						}
						if status >= http.StatusInternalServerError {
							require.Empty(t, out.logs, "a retained 5xx hold is published only when the relay stops retrying")
						}
						// Every route, including the Chat fallback
						// (TestResponseChatFallbackUnpaidErrorResetsRequestCost), resets it.
						require.Zero(t, out.requestCost, "an unpaid error records a zero request cost")
						require.False(t, out.settled, "an unpaid provider error must not be marked settled")
						require.True(t, out.retryable, "an unpaid provider error stays retryable")
					})
				}
			}
		}
	}
}

// TestPaidUpstreamErrorCyberReceiptDimensions sends gpt-6-cyber error bodies
// whose receipts carry Cyber-specific billing dimensions through every client
// protocol and status. A priority service tier and 12h cache writes are
// unpriced: settlement retains the larger of the reservation and the Standard
// price as a flagged estimate. Short-context 30m cache writes are priced at
// Cyber's $25/M write rate (ratio 12.5) and settle exactly.
func TestPaidUpstreamErrorCyberReceiptDimensions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		receipt paidMatrixReceipt
		// priced is the hand-computed Standard price of the receipt.
		priced int64
		// unpriced reports that the tariff cannot price the receipt.
		unpriced bool
	}{
		// 20,000 x 10 + 4,000 x 50 at Standard rates.
		{name: "priority_tier", receipt: paidMatrixReceipt{tier: "priority", prompt: 20_000, completion: 4_000}, priced: 400_000, unpriced: true},
		// 20 x 10 + 2 x 50 = 300, below the admission reservation.
		{name: "priority_tier_below_reservation", receipt: paidMatrixReceipt{tier: "priority", prompt: 20, completion: 2}, priced: 300, unpriced: true},
		// 17,000 uncached x 10 + 1,000 cached x 1 + 2,000 30m writes x 12.5
		// + 4,000 x 50 = 170,000 + 1,000 + 25,000 + 200,000.
		{name: "cache_write_30m_short_context", priced: 396_000, receipt: paidMatrixReceipt{tier: "default", prompt: 20_000, completion: 4_000,
			details: `"cached_tokens":1000,"cache_write_tokens":2000,"cache_write_tokens_details":{"ttl_30m_tokens":2000}`}},
		// 19,000 x 10 (12h writes have no tariff and stay ordinary input)
		// + 1,000 30m writes x 12.5 + 4,000 x 50 = 190,000 + 12,500 + 200,000.
		{name: "cache_write_12h", priced: 402_500, unpriced: true, receipt: paidMatrixReceipt{tier: "default", prompt: 20_000, completion: 4_000,
			details: `"cache_write_tokens":2000,"cache_write_tokens_details":{"ttl_30m_tokens":1000,"ttl_12h_tokens":1000}`}},
	} {
		for _, status := range paidMatrixStatuses {
			for _, protocol := range paidMatrixProtocols(channeltype.OpenAI) {
				t.Run(fmt.Sprintf("%s/%d/%s", tc.name, status, protocol), func(t *testing.T) {
					receipt := tc.receipt
					out := relayPaidMatrixAttempt(t, channeltype.OpenAI, "gpt-6-cyber", protocol, status, &receipt)
					require.NotNil(t, out.apiErr, "the provider error must reach the client")
					require.Equal(t, status, out.apiErr.StatusCode)
					require.EqualValues(t, 1, out.calls)
					require.Positive(t, out.held)
					if !tc.unpriced {
						requirePaidMatrixCharge(t, out, tc.priced)
						require.Empty(t, out.estimateReason(), "a priced Cyber receipt settles exactly")
						return
					}
					requirePaidMatrixCharge(t, out, max(out.held, tc.priced))
					require.Equal(t, unpricedUsageEstimateReason, out.estimateReason(), "an unpriced receipt must be an auditable estimate")
				})
			}
		}
	}
}

// TestResponseChatFallbackUnpaidErrorResetsRequestCost pins request-cost
// reconciliation on the Responses-to-Chat fallback route (a Responses client on
// a custom-base-URL OpenAI channel or on Azure gpt-4o-mini). When the provider
// answers 400, 429 or 500 without a usage receipt, every helper reconciles the
// provisional user_request_costs row to 0 before returning the retryable
// error. The fallback (response_fallback.go) used to schedule the refund but
// keep the admission estimate as the request cost.
func TestResponseChatFallbackUnpaidErrorResetsRequestCost(t *testing.T) {
	for _, status := range paidMatrixStatuses {
		for _, route := range paidMatrixRoutes {
			t.Run(fmt.Sprintf("%d/%s/%s", status, channeltype.IdToName(route.channel), route.model), func(t *testing.T) {
				out := relayPaidMatrixAttempt(t, route.channel, route.model, "responses", status, nil)
				require.NotNil(t, out.apiErr)
				require.Equal(t, status, out.apiErr.StatusCode)
				require.True(t, out.retryable, "control: the unpaid error stays retryable")
				for _, row := range out.logs {
					require.Zero(t, row.Quota, "control: no consume log carries a charge: %+v", row)
				}
				require.Zero(t, out.requestCost, "an unpaid fallback error must reconcile its provisional request cost to zero")
			})
		}
	}
}
