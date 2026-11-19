package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/model"
)

// provisionalUpstreamFailure returns a scripted provider-proven failure with the
// given status, as an adaptor's UpstreamErrorWrapper would build it.
//
// Parameters:
//   - status: the provider HTTP status.
//
// Returns:
//   - retryOrderOutcome: an outcome that fails every attempt with that status.
func provisionalUpstreamFailure(status int) retryOrderOutcome {
	return func(int) *model.ErrorWithStatusCode {
		return &model.ErrorWithStatusCode{
			StatusCode: status,
			Error:      model.Error{Message: "provider failure", Type: model.ErrorTypeUpstream, Code: "provider_failure", FromUpstream: true},
		}
	}
}

// gateProvisionalProcessor runs the REAL asynchronous error processor but parks
// it until the returned release function is called, so a test can observe the
// window between the client's failure response and the cooldown persistence.
//
// Parameters:
//   - t: the test handle; cleanup releases the gate and drains the processor.
//
// Returns:
//   - func(): releases the processor and waits for it to finish.
func gateProvisionalProcessor(t *testing.T) func() {
	t.Helper()
	processChannelRelayErrorForTest = nil
	gate := make(chan struct{})
	processChannelRelayErrorGateForTest = gate
	released := false
	release := func() {
		if !released {
			released = true
			close(gate)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, graceful.Drain(ctx))
	}
	t.Cleanup(func() {
		release()
		processChannelRelayErrorGateForTest = nil
	})
	return release
}

// relayProvisionalOnce drives the real controller.Relay for one request bound to
// initial, scripting every attempt with outcome. The async processor is not
// replaced, so whatever processChannelRelayError does really runs.
//
// Parameters:
//   - t: the test handle.
//   - initial: the channel the distributor selected.
//   - method: the client HTTP method.
//   - outcome: the scripted upstream result of each attempt.
//
// Returns:
//   - int: the HTTP status written to the client.
func relayProvisionalOnce(t *testing.T, initial *dbmodel.Channel, method string, outcome retryOrderOutcome) int {
	t.Helper()
	relayHelperForTest = func(c *gin.Context, _ int) *model.ErrorWithStatusCode {
		c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
		return outcome(c.GetInt(ctxkey.ChannelId))
	}
	t.Cleanup(func() { relayHelperForTest = nil })
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	gmw.SetLogger(c, logger.Logger)
	body, err := json.Marshal(map[string]any{"model": retryOrderModel, "messages": []map[string]string{{"role": "user", "content": "hi"}}})
	require.NoError(t, err)
	c.Request = httptest.NewRequest(method, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 2)
	c.Set(ctxkey.Group, retryOrderGroup)
	c.Set(ctxkey.RequestModel, retryOrderModel)
	middleware.SetupContextForSelectedChannel(c, initial, retryOrderModel)
	Relay(c)
	return rec.Code
}

// setupProvisionalRelay seeds the preferred channel A (id 1) and the fallback B
// (id 2), enables the memory cache and disables retries, so every scenario
// measures what the NEXT request would be routed to.
//
// Parameters:
//   - t: the test handle; cleanup restores the globals.
//
// Returns:
//   - *dbmodel.Channel: channel A, the one each scenario fails.
func setupProvisionalRelay(t *testing.T) *dbmodel.Channel {
	t.Helper()
	gin.SetMode(gin.TestMode)
	a := retryOrderChannel(1, "A-10", 10)
	setupRetryOrderDB(t, []*dbmodel.Channel{a, retryOrderChannel(2, "B-0", 0)})
	oldMemory, oldRetry := config.MemoryCacheEnabled, config.RetryTimes
	config.MemoryCacheEnabled, config.RetryTimes = true, 0
	t.Cleanup(func() { config.MemoryCacheEnabled, config.RetryTimes = oldMemory, oldRetry })
	return a
}

// nextRoutedChannels returns the channels that n consecutive cached selections
// for the scenario's pool pick.
//
// Parameters:
//   - t: the test handle.
//   - n: the number of selections.
//
// Returns:
//   - map[int]int: selection counts by channel id.
func nextRoutedChannels(t *testing.T, n int) map[int]int {
	t.Helper()
	picked := make(map[int]int)
	for range n {
		channel, err := dbmodel.CacheGetRandomSatisfiedChannelWithContext(context.Background(), retryOrderGroup, retryOrderModel, false)
		require.NoError(t, err)
		picked[channel.Id]++
	}
	return picked
}

// abilityAdaptiveDeadline reads channel A's persisted adaptive deadline.
//
// Parameters:
//   - t: the test handle.
//   - channelID: the ability's channel.
//
// Returns:
//   - *time.Time: the persisted adaptive deadline, nil when absent.
func abilityAdaptiveDeadline(t *testing.T, channelID int) *time.Time {
	t.Helper()
	var row dbmodel.Ability
	require.NoError(t, dbmodel.DB.Where("channel_id = ?", channelID).Take(&row).Error)
	return row.AdaptiveSuspendUntil
}

// TestRelayProvisionalCooldownClosesPublicationWindow reproduces the same-replica
// window: after a provider-proven 429 or 5xx, the next requests must stop
// selecting the failed ability at once, not only after the asynchronous
// processor persisted and published the adaptive cooldown.
func TestRelayProvisionalCooldownClosesPublicationWindow(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			a := setupProvisionalRelay(t)
			release := gateProvisionalProcessor(t)

			require.Equal(t, status, relayProvisionalOnce(t, a, http.MethodPost, provisionalUpstreamFailure(status)))
			require.Nil(t, abilityAdaptiveDeadline(t, 1), "control: the processor is parked, nothing is persisted yet")
			require.Equal(t, map[int]int{2: 50}, nextRoutedChannels(t, 50),
				"the next requests must not be routed to the ability that just failed")
			cooling, err := dbmodel.AbilityCoolingDown(context.Background(), retryOrderGroup, retryOrderModel, 1)
			require.NoError(t, err)
			require.True(t, cooling, "provider affinity must see the same cooldown")

			release()
			require.NotNil(t, abilityAdaptiveDeadline(t, 1), "the processor persists the adaptive rung")
			require.Equal(t, map[int]int{2: 50}, nextRoutedChannels(t, 50), "the persisted cooldown takes over")
		})
	}
}

// TestRelayProvisionalCooldownClearedWhenOutcomeDiscarded verifies that the
// provisional cooldown is replaced by the persisted outcome even when that
// outcome is discarded: an attempt routed with a replaced upstream revision is
// fenced by the writer, so once the processor finished the ability must route
// again rather than stay hidden for the provisional rung.
func TestRelayProvisionalCooldownClearedWhenOutcomeDiscarded(t *testing.T) {
	a := setupProvisionalRelay(t)
	release := gateProvisionalProcessor(t)

	require.Equal(t, http.StatusTooManyRequests, relayProvisionalOnce(t, a, http.MethodPost, provisionalUpstreamFailure(http.StatusTooManyRequests)))
	require.Equal(t, map[int]int{2: 20}, nextRoutedChannels(t, 20))
	// The channel's upstream changed after the attempt was routed: the writer
	// discards the stale outcome.
	require.NoError(t, dbmodel.DB.Model(&dbmodel.Channel{}).Where("id = ?", 1).UpdateColumn("upstream_revision", 7).Error)

	release()
	require.Nil(t, abilityAdaptiveDeadline(t, 1), "control: the fenced outcome was discarded")
	require.Equal(t, map[int]int{1: 20}, nextRoutedChannels(t, 20),
		"a discarded outcome must clear the provisional cooldown instead of hiding the ability for a full rung")
}

// TestRelayProvisionalCooldownScopedToAdaptivePath verifies that failures the
// processor handles with the legacy fixed policy never get a provisional
// adaptive rung: an operator override of the cooldown length, a read-only
// provider call, and a 429 without provider provenance.
func TestRelayProvisionalCooldownScopedToAdaptivePath(t *testing.T) {
	unproven := func(int) *model.ErrorWithStatusCode { return rateLimited(1) }
	for _, tc := range []struct {
		name     string
		method   string
		override string
		outcome  retryOrderOutcome
	}{
		{name: "operator-override", method: http.MethodPost, override: "0", outcome: provisionalUpstreamFailure(http.StatusTooManyRequests)},
		{name: "read-only", method: http.MethodGet, outcome: provisionalUpstreamFailure(http.StatusTooManyRequests)},
		{name: "unproven-provenance", method: http.MethodPost, outcome: unproven},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.override != "" {
				t.Setenv("CHANNEL_SUSPEND_SECONDS_FOR_429", tc.override)
				oldFixed := config.ChannelSuspendSecondsFor429
				config.ChannelSuspendSecondsFor429 = 0
				t.Cleanup(func() { config.ChannelSuspendSecondsFor429 = oldFixed })
			}
			a := setupProvisionalRelay(t)
			release := gateProvisionalProcessor(t)
			require.Equal(t, http.StatusTooManyRequests, relayProvisionalOnce(t, a, tc.method, tc.outcome))
			require.Equal(t, map[int]int{1: 20}, nextRoutedChannels(t, 20),
				"no provisional adaptive rung for a failure the legacy policy handles")
			release()
			require.Nil(t, abilityAdaptiveDeadline(t, 1), "control: the legacy policy wrote no adaptive rung")
		})
	}
}

// TestRelayProvisionalCooldownEligibilityMatchesProcessor is the drift guard
// between the synchronous publication and the asynchronous processor: for every
// failure shape, the provisional rung is published exactly when the REAL
// processChannelRelayError records the failure on the adaptive ladder.
func TestRelayProvisionalCooldownEligibilityMatchesProcessor(t *testing.T) {
	started := time.Now().UTC().Add(-time.Second)
	completed := started.Add(500 * time.Millisecond)
	proven := func(status int, message string) model.ErrorWithStatusCode {
		return model.ErrorWithStatusCode{StatusCode: status, Error: model.Error{Message: message, Type: model.ErrorTypeUpstream, FromUpstream: true}}
	}
	local := func(status int) model.ErrorWithStatusCode {
		return model.ErrorWithStatusCode{StatusCode: status, Error: model.Error{Message: "local", Type: model.ErrorTypeOneAPI}}
	}
	cancelled := proven(http.StatusBadGateway, "upstream closed")
	cancelled.RawError = errors.WithStack(context.Canceled)
	unproven := proven(http.StatusTooManyRequests, "rate limited")
	unproven.FromUpstream = false

	cases := []struct {
		name     string
		err      model.ErrorWithStatusCode
		readOnly bool
		noTimes  bool
		override string
	}{
		{name: "429", err: proven(http.StatusTooManyRequests, "rate limited")},
		{name: "500", err: proven(http.StatusInternalServerError, "boom")},
		{name: "503", err: proven(http.StatusServiceUnavailable, "down")},
		{name: "599", err: proven(599, "odd")},
		{name: "5xx-suggests-retry", err: proven(http.StatusServiceUnavailable, "server is overloaded, please try again")},
		{name: "5xx-client-cancel", err: cancelled},
		{name: "one-api-503", err: local(http.StatusServiceUnavailable)},
		{name: "429-unproven", err: unproven},
		{name: "429-read-only", err: proven(http.StatusTooManyRequests, "rate limited"), readOnly: true},
		{name: "429-no-timestamps", err: proven(http.StatusTooManyRequests, "rate limited"), noTimes: true},
		{name: "429-override", err: proven(http.StatusTooManyRequests, "rate limited"), override: "CHANNEL_SUSPEND_SECONDS_FOR_429"},
		{name: "5xx-override", err: proven(http.StatusBadGateway, "bad gateway"), override: "CHANNEL_SUSPEND_SECONDS_FOR_5XX"},
		{name: "400", err: proven(http.StatusBadRequest, "bad request")},
		{name: "404", err: proven(http.StatusNotFound, "missing")},
		{name: "408", err: proven(http.StatusRequestTimeout, "timeout")},
		{name: "413", err: proven(http.StatusRequestEntityTooLarge, "too large")},
		{name: "422", err: proven(http.StatusUnprocessableEntity, "unprocessable")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.override != "" {
				t.Setenv(tc.override, "30")
			}
			setupRetryOrderDB(t, []*dbmodel.Channel{retryOrderChannel(1, "A", 0)})
			params := processChannelRelayErrorParams{
				StartedAt: started, CompletedAt: completed, ReadOnly: tc.readOnly,
				Revision: dbmodel.AbilityUpstreamRevisionUnknown, ChannelId: 1, ChannelName: "A",
				Group: retryOrderGroup, OriginalModel: retryOrderModel, ActualModel: retryOrderModel,
				Err: tc.err,
			}
			if tc.noTimes {
				params.StartedAt, params.CompletedAt = time.Time{}, time.Time{}
			}
			predicted := relayFailureUsesAdaptiveCooldown(params)
			processChannelRelayError(context.Background(), params)
			require.Equal(t, abilityAdaptiveDeadline(t, 1) != nil, predicted,
				"the provisional publication must match the processor's adaptive decision")
		})
	}
}
