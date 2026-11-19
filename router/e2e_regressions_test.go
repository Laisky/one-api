package router

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// This file holds E2E regressions for defects the suite found in the shipped
// gateway (fixed 2026-11-18). Each doc comment states the request, the
// contract, the defect that used to violate it and the responsible code.

// TestE2EStreamedRetryFailsOver pins cross-channel failover for streamed
// requests. Request: stream=true through a failing primary (plain HTTP 429 or
// 500, no receipt) and a healthy secondary with RETRY_TIMES=1. Contract: like
// the non-stream request, the attempt fails over and the secondary settles its
// own receipt exactly. Defect: RelayTextHelper (relay/controller/text.go) and
// the Responses-to-Chat fallback (relay/controller/response_fallback.go)
// replaced c.Request with a cancellable attempt context and deferred its
// cancel, so the retry loop in controller/relay.go read
// c.Request.Context().Err() != nil and stopped as if the client had
// disconnected (retry_outcome="request_canceled"); bindStreamingUpstreamContext
// now restores the client context.
func TestE2EStreamedRetryFailsOver(t *testing.T) {
	for _, host := range e2ePaidHosts {
		for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError} {
			for _, protocol := range e2eProtocols {
				t.Run(fmt.Sprintf("%s/%d/%s", host.name, status, protocol), func(t *testing.T) {
					run := runE2EPaidError(t, host, status, false, protocol, true)
					require.Len(t, run.secondary, 1, "a streamed retryable error must fail over; body=%s", run.res.Body)
					e2eRequireSuccessWire(t, protocol, true, run.res)
					run.env.requireSettled(t, run.res, run.before, host.backupCharge, false)
				})
			}
		}
	}
}

// TestE2ETerminalErrorHoldIsVisible pins ledger consistency for a
// terminal provider failure without a receipt. Request: non-stream, primary
// answers a plain HTTP 400 (terminal by design; the secondary is never tried)
// or, with retries disabled, a plain 500. Expected: whatever the wallet keeps
// (nothing for the 400, which is refunded; the hold for the 500) is visible and
// agrees across surfaces: user used_quota, consume log rows and
// user_request_costs.
// Defect: the conservative no-refund branch for possibly forwarded requests
// (relay/controller/billing_safety.go) kept the hold debited while used_quota
// stayed unchanged, user_request_costs was 0 and the only log row was stranded
// as LogTypeProvisional, which every Logs-page query filters out. The terminal
// relay now publishes the hold (SettleTerminalRetainedReservation).
func TestE2ETerminalErrorHoldIsVisible(t *testing.T) {
	for _, host := range []e2ePaidHost{e2ePaidHosts[0], e2ePaidHosts[3]} {
		for _, protocol := range e2eProtocols {
			t.Run(host.name+"/400/"+protocol, func(t *testing.T) {
				run := runE2EPaidError(t, host, http.StatusBadRequest, false, protocol, false)
				e2eRequireErrorWire(t, protocol, run.res, http.StatusBadRequest)
				e2eRequireConsistentLedger(t, run.env, run.res, run.before)
			})
			t.Run(host.name+"/500/"+protocol, func(t *testing.T) {
				run := runE2ETerminalError(t, host, http.StatusInternalServerError, protocol, false)
				e2eRequireErrorWire(t, protocol, run.res, run.res.Code)
				e2eRequireConsistentLedger(t, run.env, run.res, run.before)
			})
		}
	}
}

// e2eRequireConsistentLedger asserts that the wallet debit of one request is
// visible and identical in used_quota, consume log rows and user_request_costs.
// Parameters: t owns assertions, env the gateway, res the client result and
// before the ledger snapshot taken before the request.
func e2eRequireConsistentLedger(t *testing.T, env *e2eEnv, res e2eResult, before e2eLedger) {
	t.Helper()
	after := env.ledger(t)
	debit := before.UserQuota - after.UserQuota
	var visible int64
	for _, row := range env.logs(t, res.RequestID) {
		if row.Type == model.LogTypeConsume {
			visible += int64(row.Quota)
		}
	}
	cost, _ := env.requestCost(t, res.RequestID)
	require.Equal(t, debit, after.UserUsed-before.UserUsed, "used_quota must account for the wallet debit")
	require.Equal(t, debit, visible, "the wallet debit must be visible in a consume log")
	require.Equal(t, debit, cost, "user_request_costs must match the wallet debit")
}

// TestE2EChatWirePreservesResponseIdentity pins the Chat Completions response
// identity on a Chat-wire route. Request: a non-stream Chat Completions client
// on an Azure gpt-4o-mini channel whose provider answers a complete
// chat.completion with id, object, created and model. Expected: those fields
// reach the client (OpenAI SDKs expose them). Defect (fixed by the embedded
// model.ChatCompletionIdentity): the client body held only choices and usage
// because openai.Handler (relay/adaptor/openai/main.go)
// decodes the provider body into SlimTextResponse (choices, usage, error) and
// re-marshals that struct, dropping every other root field. Pre-existing on
// main; not introduced by PR #624.
func TestE2EChatWirePreservesResponseIdentity(t *testing.T) {
	host := e2ePaidHosts[3]
	env := newE2EEnv(t, func(call e2eCall) e2eReply {
		return e2eSuccess(call, host.model, "", host.backupReceipt)
	}, host.channels[0])
	path, body := e2eClientRequest(e2eChat, host.model, false, "")

	res := env.post(t, path, body)

	e2eRequireSuccessWire(t, e2eChat, false, res)
	e2eRequireChatIdentity(t, res, host.model)
}

// TestE2EFallbackLogsClientAPIFormat pins consume-log attribution for a
// Responses client served through the Chat Completions fallback (native OpenAI
// channel on a custom base URL). Defect: the log recorded user_api_format
// "chat_completions" because the fallback switches meta.Mode before billing.
func TestE2EFallbackLogsClientAPIFormat(t *testing.T) {
	host := e2ePaidHosts[1]
	env := newE2EEnv(t, func(call e2eCall) e2eReply {
		return e2eSuccess(call, host.model, "", host.backupReceipt)
	}, host.channels[0])
	path, body := e2eClientRequest(e2eResponses, host.model, false, "")
	res := env.post(t, path, body)
	e2eRequireSuccessWire(t, e2eResponses, false, res)
	var formats []any
	for _, row := range env.logs(t, res.RequestID) {
		if row.Type == model.LogTypeConsume {
			formats = append(formats, row.Metadata[model.LogMetadataKeyUserAPIFormat])
		}
	}
	require.Equal(t, []any{"response_api"}, formats)
}
