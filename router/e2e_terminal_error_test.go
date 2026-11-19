package router

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// e2eTerminalErrorRun is one request whose only channel answers a plain
// provider error (no usage receipt) with retries disabled, so the failure is
// terminal.
type e2eTerminalErrorRun struct {
	env    *e2eEnv
	res    e2eResult
	before e2eLedger
	calls  []e2eCall
}

// runE2ETerminalError sends one request through host's primary channel only,
// with RETRY_TIMES=0, while the provider answers status without a receipt.
// Parameters: t owns the fixture; host, status, protocol and stream select the
// case. Returns: the observed run.
func runE2ETerminalError(t *testing.T, host e2ePaidHost, status int, protocol string, stream bool) e2eTerminalErrorRun {
	t.Helper()
	env := newE2EEnv(t, func(call e2eCall) e2eReply {
		return e2eProviderError(call, status, host.model, nil)
	}, host.channels[0])
	config.RetryTimes = 0 // newE2EEnv's cleanup restores the previous value.
	before := env.ledger(t)
	path, body := e2eClientRequest(protocol, host.model, stream, "")
	res := env.post(t, path, body)
	run := e2eTerminalErrorRun{env: env, res: res, before: before, calls: env.upstream.callsFor(host.primary)}
	require.Len(t, run.calls, 1, "the request reaches the provider exactly once; body=%s", res.Body)
	return run
}

// TestE2ETerminalProviderClientErrorRefundsHold pins the refund contract for
// an explicit provider 4xx answer without a usage receipt. A 4xx status means
// the provider rejected the request before inference (invalid body, auth,
// permission, unknown model, conflict, payload too large, unprocessable, rate
// limit, request timeout), so it did no billable work: the admission hold is
// refunded and every ledger is left unchanged, whether or not the relay
// retries. Previously a terminal 4xx kept the whole hold, which can include the
// model's maximum output (about $12.80 for a GPT-6 Cyber request), and with
// the default RETRY_TIMES=0 every rate-limited request paid it.
func TestE2ETerminalProviderClientErrorRefundsHold(t *testing.T) {
	statuses := []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound,
		http.StatusRequestTimeout, http.StatusConflict, http.StatusRequestEntityTooLarge,
		http.StatusUnprocessableEntity, http.StatusTooManyRequests}
	for _, host := range []e2ePaidHost{e2ePaidHosts[0], e2ePaidHosts[2], e2ePaidHosts[3]} {
		for _, status := range statuses {
			for _, protocol := range e2eProtocols {
				for _, stream := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%d/%s/stream=%t", host.name, status, protocol, stream), func(t *testing.T) {
						run := runE2ETerminalError(t, host, status, protocol, stream)
						require.Equal(t, status, run.res.Code, "the provider status reaches the client; body=%s", run.res.Body)
						hold := run.before.UserQuota - run.calls[0].UserQuota
						require.Positive(t, hold, "control: admission reserved quota before dispatch")
						run.env.requireNothingCharged(t, run.res, run.before)
					})
				}
			}
		}
	}
}

// TestE2ETerminalProviderServerErrorKeepsVisibleHold pins the retained-hold
// contract for a terminal provider 5xx answer without a receipt: the provider
// may have done work before failing, so the hold is kept, and it is visible and
// consistent across the wallet, used quota, a consume log flagged as a
// retained-reservation estimate, and user_request_costs.
func TestE2ETerminalProviderServerErrorKeepsVisibleHold(t *testing.T) {
	statuses := []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable}
	for _, host := range []e2ePaidHost{e2ePaidHosts[0], e2ePaidHosts[3]} {
		for _, status := range statuses {
			for _, protocol := range e2eProtocols {
				t.Run(fmt.Sprintf("%s/%d/%s", host.name, status, protocol), func(t *testing.T) {
					run := runE2ETerminalError(t, host, status, protocol, false)
					require.GreaterOrEqual(t, run.res.Code, 500, "body=%s", run.res.Body)
					hold := run.before.UserQuota - run.calls[0].UserQuota
					require.Positive(t, hold)
					after := run.env.ledger(t)
					require.Equal(t, hold, run.before.UserQuota-after.UserQuota, "a terminal 5xx keeps exactly the hold")
					require.Equal(t, hold, after.UserUsed-run.before.UserUsed, "used_quota accounts for the kept hold")
					var visible int64
					var reasons []string
					for _, row := range run.env.logs(t, run.res.RequestID) {
						if row.Type == model.LogTypeConsume {
							visible += int64(row.Quota)
							reasons = append(reasons, e2eEstimateReason(row))
						}
					}
					require.Equal(t, hold, visible, "the kept hold is visible in a consume log")
					require.Len(t, reasons, 1)
					require.True(t, strings.HasPrefix(reasons[0], "retained_reservation_"), "reason=%q", reasons[0])
					cost, _ := run.env.requestCost(t, run.res.RequestID)
					require.Equal(t, hold, cost, "user_request_costs records the kept hold")
				})
			}
		}
	}
}
