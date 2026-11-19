package router

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

const (
	// e2eAzureKey and e2eAzureBackupKey are the primary and secondary Azure credentials.
	e2eAzureKey       = "e2e-azure-primary"
	e2eAzureBackupKey = "e2e-azure-secondary"
	// e2eOpenAIProxyBase is a non-api.openai.com base URL on a native OpenAI
	// channel type. Chat and Claude clients are still forced onto /v1/responses
	// (shouldForceResponseAPI), while a Responses client falls back to Chat
	// (supportsNativeResponseAPI), so this route exercises the Chat wire.
	e2eOpenAIProxyBase = "https://e2e-openai-proxy.example"
	// e2eAzureMiniPath is the Azure deployment Chat Completions path for gpt-4o-mini.
	e2eAzureMiniPath = "/openai/deployments/" + e2eMiniModel + "/chat/completions"
)

// e2ePaidHost is one channel pair (failing primary, healthy lower-priority
// secondary) serving the same model, with the receipts each one reports.
type e2ePaidHost struct {
	name      string
	model     string
	channels  []e2eChannel
	primary   string
	secondary string
	// errorReceipt is what the primary's error body reports when it carries
	// usage, and errorCharge its exact price.
	errorReceipt e2eUsage
	errorCharge  int64
	// backupReceipt is the secondary's success receipt and backupCharge its price.
	backupReceipt e2eUsage
	backupCharge  int64
	// tier is the service_tier the secondary's Responses receipt reports.
	tier string
	// paths maps each client protocol to the upstream path it is dispatched to.
	paths map[string]string
}

// e2eAllProtocolsTo maps every client protocol to one upstream path.
func e2eAllProtocolsTo(path string) map[string]string {
	return map[string]string{e2eChat: path, e2eResponses: path, e2eMessages: path}
}

// e2ePaidHosts are the provider routes under test. Charges are computed by
// hand from the catalog constants (see e2eMiniCharge and e2eCyberModel):
//   - gpt-4o-mini error receipt: e2eMiniReceipt = 2,550. Backup receipt
//     10,000 input and 2,000 output: 10,000 x 0.075 + 2,000 x 0.3 = 1,350.
//   - gpt-6-cyber error receipt: the Standard receipt of e2eCyberReceipts =
//     369,000. Backup receipt 10,000 input and 2,000 output: 10,000 x 10 +
//     2,000 x 50 = 200,000.
//
// Azure inherits OpenAI's gpt-4o-mini catalog entry, so its prices match.
var e2ePaidHosts = []e2ePaidHost{
	{
		name: "openai_native", model: e2eMiniModel, primary: e2eNativeKey, secondary: e2eNativeBackupKey,
		channels: []e2eChannel{
			e2eNativeChannel(939601, e2eNativeKey, 10, e2eMiniModel, ""),
			e2eNativeChannel(939602, e2eNativeBackupKey, 5, e2eMiniModel, ""),
		},
		errorReceipt: e2eMiniReceipt, errorCharge: e2eMiniCharge,
		backupReceipt: e2eUsage{Input: 10_000, Output: 2_000}, backupCharge: 1_350,
		paths: e2eAllProtocolsTo("/v1/responses"),
	},
	{
		name: "openai_custom_base", model: e2eMiniModel, primary: e2eNativeKey, secondary: e2eNativeBackupKey,
		channels: []e2eChannel{
			{ID: 939611, Type: channeltype.OpenAI, Key: e2eNativeKey, Priority: 10, BaseURL: e2eOpenAIProxyBase, Models: e2eMiniModel},
			{ID: 939612, Type: channeltype.OpenAI, Key: e2eNativeBackupKey, Priority: 5, BaseURL: e2eOpenAIProxyBase, Models: e2eMiniModel},
		},
		errorReceipt: e2eMiniReceipt, errorCharge: e2eMiniCharge,
		backupReceipt: e2eUsage{Input: 10_000, Output: 2_000}, backupCharge: 1_350,
		paths: map[string]string{e2eChat: "/v1/responses", e2eResponses: "/v1/chat/completions", e2eMessages: "/v1/responses"},
	},
	{
		name: "openai_native_cyber", model: e2eCyberModel, primary: e2eNativeKey, secondary: e2eNativeBackupKey,
		channels: []e2eChannel{
			e2eNativeChannel(939621, e2eNativeKey, 10, e2eCyberModel, ""),
			e2eNativeChannel(939622, e2eNativeBackupKey, 5, e2eCyberModel, ""),
		},
		errorReceipt: e2eCyberReceipts[0].usage, errorCharge: e2eCyberReceipts[0].charge,
		backupReceipt: e2eUsage{Input: 10_000, Output: 2_000}, backupCharge: 200_000,
		tier: "default", paths: e2eAllProtocolsTo("/v1/responses"),
	},
	{
		name: "azure", model: e2eMiniModel, primary: e2eAzureKey, secondary: e2eAzureBackupKey,
		channels: []e2eChannel{
			{ID: 939631, Type: channeltype.Azure, Key: e2eAzureKey, Priority: 10, BaseURL: e2eAzureBase, Models: e2eMiniModel, Config: e2eAzureConfig},
			{ID: 939632, Type: channeltype.Azure, Key: e2eAzureBackupKey, Priority: 5, BaseURL: e2eAzureBase, Models: e2eMiniModel, Config: e2eAzureConfig},
		},
		errorReceipt: e2eMiniReceipt, errorCharge: e2eMiniCharge,
		backupReceipt: e2eUsage{Input: 10_000, Output: 2_000}, backupCharge: 1_350,
		paths: e2eAllProtocolsTo(e2eAzureMiniPath),
	},
}

// e2ePaidErrorStatuses are the provider error statuses under test.
var e2ePaidErrorStatuses = []int{http.StatusBadRequest, http.StatusTooManyRequests, http.StatusInternalServerError}

// e2ePaidErrorRun is one paid-error request and what it observed.
type e2ePaidErrorRun struct {
	env       *e2eEnv
	res       e2eResult
	before    e2eLedger
	primary   []e2eCall
	secondary []e2eCall
}

// runE2EPaidError sends one request through host whose primary answers status,
// with an error-body receipt when withReceipt, and whose secondary succeeds.
// Parameters: t owns the fixture; host, status, withReceipt, protocol and
// stream select the case. Returns: the observed run.
func runE2EPaidError(t *testing.T, host e2ePaidHost, status int, withReceipt bool, protocol string, stream bool) e2ePaidErrorRun {
	t.Helper()
	env := newE2EEnv(t, func(call e2eCall) e2eReply {
		if call.Credential == host.primary {
			var receipt *e2eUsage
			if withReceipt {
				usage := host.errorReceipt
				receipt = &usage
			}
			return e2eProviderError(call, status, host.model, receipt)
		}
		return e2eSuccess(call, host.model, host.tier, host.backupReceipt)
	}, host.channels...)
	before := env.ledger(t)
	path, body := e2eClientRequest(protocol, host.model, stream, "")
	res := env.post(t, path, body)
	run := e2ePaidErrorRun{env: env, res: res, before: before,
		primary: env.upstream.callsFor(host.primary), secondary: env.upstream.callsFor(host.secondary)}
	require.Len(t, run.primary, 1, "the higher-priority channel is tried exactly once; body=%s", res.Body)
	require.Equal(t, host.paths[protocol], run.primary[0].Path)
	require.Equal(t, host.model, run.primary[0].Body["model"])
	for _, call := range run.secondary {
		require.Equal(t, host.paths[protocol], call.Path)
	}
	return run
}

// TestE2EPaidUpstreamErrorReceipt drives HTTP 400, 429 and 500 provider errors
// through the shipped retry loop with RETRY_TIMES=1 and a healthy secondary.
// An error body carrying a usage receipt is settled exactly at the receipt
// price, reaches the client as the provider's failure, and is never replayed
// on the secondary. Without a receipt, 429 and 500 fail over: the primary's
// hold is refunded and its provisional row voided, and the secondary settles
// its own receipt exactly. A 400 without a receipt is a terminal client error
// by design (shouldRetry in controller/relay_retry.go): it is not retried, and
// because the provider rejected the request before inference its hold is
// refunded (TestE2ETerminalProviderClientErrorRefundsHold); a terminal 5xx
// keeps a visible hold (TestE2ETerminalProviderServerErrorKeepsVisibleHold). Streamed requests fail over exactly like
// non-stream ones (TestE2EStreamedRetryFailsOver).
func TestE2EPaidUpstreamErrorReceipt(t *testing.T) {
	for _, host := range e2ePaidHosts {
		for _, status := range e2ePaidErrorStatuses {
			for _, withReceipt := range []bool{true, false} {
				for _, protocol := range e2eProtocols {
					for _, stream := range []bool{false, true} {
						failover := !withReceipt && status != http.StatusBadRequest
						name := fmt.Sprintf("%s/%d/receipt=%t/%s/stream=%t", host.name, status, withReceipt, protocol, stream)
						t.Run(name, func(t *testing.T) {
							run := runE2EPaidError(t, host, status, withReceipt, protocol, stream)
							switch {
							case withReceipt:
								e2eRequirePaidErrorWire(t, protocol, stream, run.res, status)
								require.Empty(t, run.secondary, "a settled paid attempt must never be replayed")
								row := run.env.requireSettled(t, run.res, run.before, host.errorCharge, false)
								require.Equal(t, host.channels[0].ID, row.ChannelId, "the paid attempt is billed to the channel that did the work")
							case !failover:
								e2eRequireErrorWire(t, protocol, run.res, status)
								require.Empty(t, run.secondary, "a provider 400 is a terminal client error")
								hold := run.before.UserQuota - run.primary[0].UserQuota
								require.Positive(t, hold, "control: admission reserved quota before dispatch")
								// The provider rejected the request before inference: the
								// hold is refunded and nothing is charged.
								run.env.requireNothingCharged(t, run.res, run.before)
							default:
								e2eRequireSuccessWire(t, protocol, stream, run.res)
								require.Len(t, run.secondary, 1, "an unpaid retryable error fails over to the secondary")
								require.Equal(t, host.model, run.secondary[0].Body["model"])
								row := run.env.requireSettled(t, run.res, run.before, host.backupCharge, false)
								require.Equal(t, host.channels[1].ID, row.ChannelId)
							}
						})
					}
				}
			}
		}
	}
}
