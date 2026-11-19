package controller

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/model"
)

// tooLargeAfterConcurrentCooldown scripts the 413 race from the PR #627 review:
// while the request is in flight on suspendedID, a concurrent request's 429
// suspends that exact ability, and then this attempt is rejected as too large.
// Every other channel in tooLarge also answers 413; the remaining ones succeed.
func tooLargeAfterConcurrentCooldown(t *testing.T, suspendedID int, tooLarge ...int) retryOrderOutcome {
	t.Helper()
	rejects := make(map[int]bool, len(tooLarge)+1)
	rejects[suspendedID] = true
	for _, id := range tooLarge {
		rejects[id] = true
	}
	return func(channelID int) *model.ErrorWithStatusCode {
		if channelID == suspendedID {
			require.NoError(t, dbmodel.SuspendAbility(context.Background(), retryOrderGroup, retryOrderModel, channelID, time.Hour))
		}
		if !rejects[channelID] {
			return nil
		}
		return &model.ErrorWithStatusCode{
			StatusCode: http.StatusRequestEntityTooLarge,
			Error:      model.Error{Message: "request too large", Type: model.ErrorTypeUpstream},
		}
	}
}

// TestRelayRetry_413BudgetCountsRemainingCandidates reproduces the review's
// budget race: the 413 path used len(available)-1, assuming the failed channel
// was still listed. Once a concurrent cooldown hides it, that formula yields
// zero retries although healthy B is available. The budget must count the
// eligible candidates that remain after excluding the failed channel.
func TestRelayRetry_413BudgetCountsRemainingCandidates(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			channelA := retryOrderChannel(1, "A-prio-10", 10)
			channelB := retryOrderChannel(2, "B-prio-5", 5)
			setupRetryOrderDB(t, []*dbmodel.Channel{channelA, channelB})

			got := runRelayRetryScenario(t, channelA, tooLargeAfterConcurrentCooldown(t, 1), path.memoryCache, 0)

			require.Equal(t, []int{1, 2}, got.order, "a concurrently cooled failed channel must not consume the only 413 retry")
			require.Equal(t, http.StatusOK, got.status, string(got.body))
		})
	}
}

// TestRelayRetry_413KeepsCooledChannelCapacity reproduces the review's capacity
// race: the larger_max_tokens filter read the rejected channel's max_tokens from
// the cooldown-filtered slice. When a concurrent 429 cooled A, its 4k limit was
// forgotten, so B (also 4k) was retried and rejected again. The rejected size
// must come from the routing snapshot, independent of temporary availability.
func TestRelayRetry_413KeepsCooledChannelCapacity(t *testing.T) {
	channelA := retryOrderChannel(1, "A-prio-10-4k", 10)
	channelA.ModelConfigs = stringPtr(`{"gpt-4o-mini": {"max_tokens": 4096}}`)
	channelB := retryOrderChannel(2, "B-prio-5-4k", 5)
	channelB.ModelConfigs = stringPtr(`{"gpt-4o-mini": {"max_tokens": 4096}}`)
	channelC := retryOrderChannel(3, "C-prio-0-8k", 0)
	channelC.ModelConfigs = stringPtr(`{"gpt-4o-mini": {"max_tokens": 8192}}`)
	setupRetryOrderDB(t, []*dbmodel.Channel{channelA, channelB, channelC})

	got := runRelayRetryScenario(t, channelA, tooLargeAfterConcurrentCooldown(t, 1, 2), true, 0)

	require.Equal(t, []int{1, 3}, got.order, "413 retries must skip sizes rejected by a channel that is now cooling")
	require.Equal(t, http.StatusOK, got.status, string(got.body))
}
