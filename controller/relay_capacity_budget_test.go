package controller

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/model"
)

// capacityCooldownColumns lists both persisted cooldown deadlines; the retry
// ceiling must count an ability cooling under either one, and selection must
// keep excluding it under either one.
var capacityCooldownColumns = []string{"suspend_until", "adaptive_suspend_until"}

// setCapacityAbilityCooldown persists column for channelID's retry-order
// ability and rebuilds the routing cache, so both routing paths observe it. A
// zero until clears every cooldown deadline of the ability, the way an expiry
// or an operator action does.
//
// Parameters:
//   - t: the test handle.
//   - channelID: the channel whose ability changes.
//   - column: the cooldown column to set ("suspend_until" or "adaptive_suspend_until").
//   - until: the new deadline; the zero time clears both deadlines.
func setCapacityAbilityCooldown(t *testing.T, channelID int, column string, until time.Time) {
	t.Helper()
	updates := map[string]any{"suspend_until": nil, "adaptive_suspend_until": nil}
	if !until.IsZero() {
		updates = map[string]any{column: until.UTC()}
	}
	require.NoError(t, dbmodel.DB.Model(&dbmodel.Ability{}).
		Where("channel_id = ? AND model = ?", channelID, retryOrderModel).
		Updates(updates).Error)
	dbmodel.InitChannelCache()
}

// capacityCooldownEndsDuringAttempt scripts a walk across a channel that is
// cooling when the request starts: initial answers 413, inFlight answers 500,
// and while inFlight's attempt runs the cooling channel's cooldown ends (when
// recover is true). Every other channel succeeds.
//
// Parameters:
//   - t: the test handle.
//   - initial, inFlight, cooling: the channel IDs playing each role.
//   - recover: whether the cooling channel's cooldown ends mid-walk.
//
// Return values:
//   - retryOrderOutcome: the scripted outcome function.
func capacityCooldownEndsDuringAttempt(t *testing.T, initial, inFlight, cooling int, recover bool) retryOrderOutcome {
	t.Helper()
	return func(channelID int) *model.ErrorWithStatusCode {
		switch channelID {
		case initial:
			return capacityUpstreamFailure(http.StatusRequestEntityTooLarge)
		case inFlight:
			if recover {
				setCapacityAbilityCooldown(t, cooling, "", time.Time{})
			}
			return capacityUpstreamFailure(http.StatusInternalServerError)
		default:
			return nil
		}
	}
}

// TestRelayCapacityBudget_CooldownEndingMidWalkKeepsItsSlot reproduces the
// review's budget finding: the 413 retry ceiling counted only the channels
// routable when the initial failure happened (memory path) or fell back to a
// fixed single retry (database path, which had no count). A channel that was
// merely cooling then consumed no slot, so when its cooldown ended while a later
// attempt was in flight the walk stopped one channel short. The ceiling must
// count every untried channel of the pool while each selection keeps enforcing
// both cooldown deadlines: a channel still cooling is never dispatched, and the
// walk then ends with the last real upstream error.
func TestRelayCapacityBudget_CooldownEndingMidWalkKeepsItsSlot(t *testing.T) {
	for _, path := range routingPaths {
		for _, column := range capacityCooldownColumns {
			for _, tc := range []struct {
				name       string
				recover    bool
				wantOrder  []int
				wantStatus int
			}{
				{name: "recovered", recover: true, wantOrder: []int{1, 3, 2}, wantStatus: http.StatusOK},
				{name: "still-cooling", recover: false, wantOrder: []int{1, 3}, wantStatus: http.StatusInternalServerError},
			} {
				t.Run(path.name+"/"+column+"/"+tc.name, func(t *testing.T) {
					a := retryOrderChannel(1, "A-prio-10", 10)
					b := retryOrderChannel(2, "B-prio-5-cooling", 5)
					c := retryOrderChannel(3, "C-prio-0", 0)
					setupRetryOrderDB(t, []*dbmodel.Channel{a, b, c})
					setCapacityAbilityCooldown(t, 2, column, time.Now().Add(time.Hour))

					got := runRelayRetryScenario(t, a, capacityCooldownEndsDuringAttempt(t, 1, 3, 2, tc.recover), path.memoryCache, 0)

					require.Equal(t, tc.wantOrder, got.order, "a cooling channel keeps its slot but is never dispatched while cooling")
					require.Equal(t, tc.wantStatus, got.status, string(got.body))
				})
			}
		}
	}
}

// TestRelayCapacityBudget_DatabasePathCountsRemainingChannels reproduces the
// database-path half of the budget finding: without a memory cache the 413
// budget could not be counted and fell back to one retry, so the third channel
// was never tried although two untried channels remained after A's 413.
func TestRelayCapacityBudget_DatabasePathCountsRemainingChannels(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			a := retryOrderChannel(1, "A-prio-10", 10)
			b := retryOrderChannel(2, "B-prio-5", 5)
			c := retryOrderChannel(3, "C-prio-0", 0)
			setupRetryOrderDB(t, []*dbmodel.Channel{a, b, c})

			got := runRelayRetryScenario(t, a, capacityScriptedStatuses(map[int]int{
				1: http.StatusRequestEntityTooLarge,
				2: http.StatusRequestEntityTooLarge,
			}), path.memoryCache, 0)

			require.Equal(t, []int{1, 2, 3}, got.order, "every untried channel of the pool gets a 413 retry slot")
			require.Equal(t, http.StatusOK, got.status, string(got.body))
		})
	}
}
