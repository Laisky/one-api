package controller

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
)

// tariffDispatchObservation is what the fake provider saw at dispatch time. The
// user balance is read inside the provider handler, after admission and before
// settlement, so balance-minus-observed is exactly the admission reservation.
// Errors are carried back instead of calling testing.Fatal off the test goroutine.
type tariffDispatchObservation struct {
	Path      string
	Body      string
	UserQuota int64
	Err       error
}

// tariffUpstream is a fake TLS provider that counts calls and records each dispatch.
type tariffUpstream struct {
	server       *httptest.Server
	calls        atomic.Int32
	observations chan tariffDispatchObservation
}

// newTariffUpstream starts a fake provider and routes every production HTTP
// client dial to it, whatever host the channel base URL names, so a native
// https://api.openai.com channel keeps its production routing. Parameters: t owns
// cleanup; reply returns the content type and body for a request path. Returns:
// the started upstream, whose URL may also be used as the channel base URL.
func newTariffUpstream(t *testing.T, reply func(path string) (string, string)) *tariffUpstream {
	t.Helper()
	upstream := &tariffUpstream{observations: make(chan tariffDispatchObservation, 4)}
	upstream.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstream.calls.Add(1)
		body, readErr := io.ReadAll(r.Body)
		var quota int64
		quotaErr := model.DB.Model(&model.User{}).Where("id = ?", fallbackUserID).Select("quota").Scan(&quota).Error
		if readErr != nil {
			quotaErr = readErr
		}
		upstream.observations <- tariffDispatchObservation{Path: r.URL.Path, Body: string(body), UserQuota: quota, Err: quotaErr}
		contentType, payload := reply(r.URL.Path)
		w.Header().Set("Content-Type", contentType)
		_, _ = io.WriteString(w, payload)
	}))
	t.Cleanup(upstream.server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(upstream.server.Certificate())
	address := upstream.server.Listener.Addr().String()
	previous := client.HTTPClient
	client.HTTPClient = &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
		// httptest certificates are issued for example.com and loopback only.
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12},
	}}
	t.Cleanup(func() { client.HTTPClient = previous })
	return upstream
}

// next returns the single recorded dispatch. Parameters: t owns assertions.
// Returns: the observation, failing the test when the provider saw nothing.
func (u *tariffUpstream) next(t *testing.T) tariffDispatchObservation {
	t.Helper()
	select {
	case obs := <-u.observations:
		require.NoError(t, obs.Err, "the provider handler must observe the ledger")
		return obs
	default:
		require.FailNow(t, "the provider recorded no dispatch")
		return tariffDispatchObservation{}
	}
}

// useProductionGlobalPricing loads the global pricing layer exactly as main.go
// does, so resolvers see the merged native OpenAI catalog that an operator-only
// model must never borrow; without it the global fallback is empty in this
// package and a leak through it would be invisible. Parameters: t owns cleanup,
// which restores the previous contributing adapters and, when the layer was
// not initialized before, leaves it empty and uninitialized again.
func useProductionGlobalPricing(t *testing.T) {
	t.Helper()
	wasInitialized := pricing.IsGlobalPricingInitialized()
	previousAdapters := pricing.GetContributingAdapters()
	relay.InitializeGlobalPricing()
	pricing.ReloadDefaultConfiguration()
	pricing.ReloadGlobalPricing()
	_, hasNativeCyber := pricing.GetGlobalModelConfig("gpt-6-cyber")
	require.True(t, hasNativeCyber, "control: the global layer must carry the native tariff the gate guards against")
	t.Cleanup(func() {
		pricing.SetContributingAdapters(previousAdapters)
		pricing.ReloadGlobalPricing()
		if !wasInitialized {
			pricing.InitializeGlobalPricingManager(nil)
		}
	})
}

// tariffRequestLedger sums every log row and the request-cost row of one
// request. Parameters: t owns assertions; requestID selects the rows. Returns:
// the quota of all log rows (provisional included) and of user_request_costs,
// treating an absent request-cost row as zero.
func tariffRequestLedger(t *testing.T, requestID string) (int64, int64) {
	t.Helper()
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Find(&logs).Error)
	var logQuota int64
	for _, entry := range logs {
		logQuota += int64(entry.Quota)
	}
	var costs []model.UserRequestCost
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Find(&costs).Error)
	var costQuota int64
	for _, cost := range costs {
		costQuota += cost.Quota
	}
	return logQuota, costQuota
}

// tokenReservationQuote recomputes, by hand, the admission reservation a text
// controller takes: (buffer + prompt) input tokens plus maxTokens output tokens
// at the effective tariff, truncated to quota units like the production quote.
// Parameters: buffered adds config.PreConsumedQuota (Chat Completions and the
// Responses-to-Chat fallback do; Claude Messages and native Responses do not),
// prompt is the gateway's prompt estimate, maxTokens the client output limit,
// and ratio/completion the effective input ratio and completion multiplier at
// group ratio 1. Returns: the expected reservation in quota units.
func tokenReservationQuote(buffered bool, prompt, maxTokens int, ratio, completion float64) int64 {
	input := int64(prompt)
	if buffered {
		input += config.PreConsumedQuota
	}
	return int64(float64(input)*ratio + float64(maxTokens)*ratio*completion)
}

// lyriaMaxTokens is the output limit every lyriaProtocolRequest body carries.
const lyriaMaxTokens = 128

// azureCyberChatReply is an Azure chat completion receipt of 20 input and 2
// output tokens, the only usage the settlement may bill.
const azureCyberChatReply = `{"id":"chatcmpl-azure","object":"chat.completion","model":"gpt-6-cyber","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ack"}}],"usage":{"prompt_tokens":20,"completion_tokens":2,"total_tokens":22}}`

// aiproxyLibraryReply is an AIProxy library answer. The library API reports no
// token usage, so settlement must retain the operator-priced reservation.
const aiproxyLibraryReply = `{"success":true,"answer":"ack","documents":[]}`

// cyberOperatorTariff is a deliberately non-native operator price for
// gpt-6-cyber: $4/M input (ratio 2) and $24/M output (completion ratio 6).
// OpenAI's native tariff is $20/$100 (ratio 10, completion ratio 5).
var cyberOperatorTariff = model.ModelConfigLocal{Ratio: 4 * billingratio.MilliTokensUsd, CompletionRatio: 6}

// TestNativeOnlyCyberOperatorTariffMatrixHTTP drives gpt-6-cyber through the
// production Chat Completions, Claude Messages and Responses controllers on both
// inheriting hosts (Azure, AIProxy library) against a fake TLS provider and the
// SQLite quota ledger. Unpriced channels must be refused before any provider
// contact, with any reservation refunded and nothing logged. Priced channels must
// dispatch exactly once, reserve at the operator tariff, and settle the
// hand-computed operator charge with balance delta == consume-log quota ==
// request cost == expected. Azure bills the provider receipt; the AIProxy
// library returns no usage, so its documented settlement retains the
// operator-priced reservation rather than refunding it or billing natively.
func TestNativeOnlyCyberOperatorTariffMatrixHTTP(t *testing.T) {
	useProductionGlobalPricing(t)
	aiproxyEndpoints := channeltype.DefaultEndpointsForChannelType(channeltype.AIProxyLibrary)
	for _, mode := range []int{relaymode.ChatCompletions, relaymode.ClaudeMessages, relaymode.ResponseAPI} {
		require.True(t, channeltype.IsEndpointSupported(mode, aiproxyEndpoints), "AIProxy library routes every text protocol by default")
	}
	const (
		inputRatio      = 4 * billingratio.MilliTokensUsd
		completionRatio = 6.0
		// 20 input tokens at $4/M plus 2 output tokens at $24/M: 20*2 + 2*2*6.
		azureReceiptCharge = int64(64)
		// Unpriced requests run on a balance below any quote at OpenAI's native
		// $20/$100 tariff, so an admission quote borrowed from the global layer
		// would surface as insufficient quota instead of the operator-tariff refusal.
		unpricedBalance = int64(5_000)
	)
	require.Greater(t, tokenReservationQuote(true, 0, lyriaMaxTokens, 20*billingratio.MilliTokensUsd, 5), unpricedBalance)
	for _, host := range []struct {
		name, path, reply string
		channel           int
		config            model.ChannelConfig
		receiptUsage      bool
	}{
		{name: "azure", channel: channeltype.Azure, config: model.ChannelConfig{APIVersion: "2025-04-01-preview"}, path: "/openai/deployments/gpt-6-cyber/chat/completions", reply: azureCyberChatReply, receiptUsage: true},
		{name: "aiproxy", channel: channeltype.AIProxyLibrary, config: model.ChannelConfig{LibraryID: "library-fixture"}, path: "/api/library/ask", reply: aiproxyLibraryReply},
	} {
		for _, protocol := range []string{"chat", "messages", "responses"} {
			for _, priced := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/priced=%t", host.name, protocol, priced), func(t *testing.T) {
					balance := unpricedBalance
					if priced {
						balance = 100_000
					}
					xaiVideoSetup(t, balance, false)
					upstream := newTariffUpstream(t, func(string) (string, string) { return "application/json", host.reply })

					var local *model.ModelConfigLocal
					if priced {
						tariff := cyberOperatorTariff
						local = &tariff
					}
					path, body := lyriaProtocolRequest(protocol, false)
					c, writer, requestID := protocolContext(t, host.channel, "gpt-6-cyber", path, body, upstream.server.URL, balance, 1, false, local)
					c.Set(ctxkey.Config, host.config)
					clearNativeSettledLedger(t, requestID)

					apiErr := relayLyriaProtocol(c, protocol)
					drainCriticalTasks(t)
					logQuota, costQuota := tariffRequestLedger(t, requestID)
					if !priced {
						require.NotNil(t, apiErr, "an unpriced native-only model must be rejected")
						require.Contains(t, apiErr.Error.Message, `model "gpt-6-cyber" requires explicit positive input and output pricing in channel model_configs`)
						require.True(t, adaptor.IsRequestPricingNotConfigured(apiErr.RawError),
							"every protocol shares the billing-configuration refusal contract")
						if protocol == "messages" {
							// Claude Messages validates the tariff before any reservation and
							// answers the retryable billing-configuration error directly.
							require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
							require.Equal(t, "model_pricing_not_configured", apiErr.Error.Code)
						} else {
							// Text controllers reserve, then DoRequestHelper refuses dispatch;
							// the relay loop maps the final refusal to 503 (controller package).
							require.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
							require.Equal(t, "do_request_failed", apiErr.Error.Code)
						}
						require.Zero(t, upstream.calls.Load(), "the provider must never be contacted without an operator tariff")
						require.Equal(t, balance, reloadUserQuota(t), "any admission reservation must be refunded in full")
						require.Zero(t, consumeLogQuota(t, requestID), "no native OpenAI tariff may be charged")
						require.Zero(t, logQuota, "no log row may retain a charge")
						require.Zero(t, costQuota, "the request cost must reconcile to zero")
						return
					}
					require.Nil(t, apiErr)
					require.EqualValues(t, 1, upstream.calls.Load(), "a priced request is dispatched exactly once")
					obs := upstream.next(t)
					require.Equal(t, host.path, obs.Path)
					require.Contains(t, obs.Body, `"model":"gpt-6-cyber"`, "the mapped native-only model is what the provider receives")
					require.Contains(t, writer.Body.String(), "ack")

					// The prompt estimate is the gateway's own (11 tokens for this fixture);
					// the tariff applied to it is what this test pins.
					prompt := metalib.GetByContext(c).PromptTokens
					require.Positive(t, prompt, "admission must estimate the prompt")
					wantReservation := tokenReservationQuote(protocol != "messages", prompt, lyriaMaxTokens, inputRatio, completionRatio)
					require.Equal(t, wantReservation, balance-obs.UserQuota, "admission must reserve at the operator tariff")
					// The same request at OpenAI's native tariff would reserve far more.
					require.Less(t, wantReservation, tokenReservationQuote(protocol != "messages", prompt, lyriaMaxTokens, 20*billingratio.MilliTokensUsd, 5))

					want := azureReceiptCharge
					if !host.receiptUsage {
						want = wantReservation
					}
					require.Equal(t, balance-want, reloadUserQuota(t), "balance delta must equal the operator charge")
					require.Equal(t, want, consumeLogQuota(t, requestID), "the consume log must record the operator charge")
					require.Equal(t, want, logQuota, "no provisional row may retain an extra charge")
					require.Equal(t, want, costQuota, "the request cost must reconcile to the operator charge")
				})
			}
		}
	}
}
