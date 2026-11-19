package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// capacityRetryChannel builds a retry-order channel whose model config for the
// retry-order model declares maxTokens. A zero maxTokens keeps a model config
// without max_tokens (an undeclared limit), and a negative one omits the model
// config entirely.
//
// Parameters:
//   - id, name, priority: the channel identity and routing tier.
//   - maxTokens: the declared max_tokens, 0 for unset, negative for no config.
//
// Return values:
//   - *dbmodel.Channel: the unsaved channel.
func capacityRetryChannel(id int, name string, priority int64, maxTokens int) *dbmodel.Channel {
	channel := retryOrderChannel(id, name, priority)
	switch {
	case maxTokens > 0:
		channel.ModelConfigs = stringPtr(`{"` + retryOrderModel + `": {"max_tokens": ` + strconv.Itoa(maxTokens) + `}}`)
	case maxTokens == 0:
		channel.ModelConfigs = stringPtr(`{"` + retryOrderModel + `": {"ratio": 1}}`)
	}
	return channel
}

// capacityUpstreamFailure returns a provider-reported failure with status, the
// shape upstream error wrappers produce (FromUpstream set).
//
// Parameters:
//   - status: the upstream HTTP status.
//
// Return values:
//   - *model.ErrorWithStatusCode: the scripted failure.
func capacityUpstreamFailure(status int) *model.ErrorWithStatusCode {
	return &model.ErrorWithStatusCode{
		StatusCode: status,
		Error:      model.Error{Message: fmt.Sprintf("synthetic upstream %d", status), Type: model.ErrorTypeUpstream, FromUpstream: true},
	}
}

// capacityScriptedStatuses scripts each listed channel to fail with its
// upstream status; every other channel succeeds.
//
// Parameters:
//   - statuses: channel ID to upstream HTTP status.
//
// Return values:
//   - retryOrderOutcome: the scripted outcome function.
func capacityScriptedStatuses(statuses map[int]int) retryOrderOutcome {
	return func(channelID int) *model.ErrorWithStatusCode {
		status, failing := statuses[channelID]
		if !failing {
			return nil
		}
		return capacityUpstreamFailure(status)
	}
}

// TestRelayCapacityRetry_Local413IsNotCapacityEvidence reproduces the review
// finding that a 413 produced by one-api itself was treated as evidence about
// the channel's max_tokens. The Responses fallback answers a local 413
// (response_fallback_input_too_large) for an input only some channels cannot
// carry; it says nothing about A's declared 4k limit, so B (next priority, also
// 4k) must still be tried instead of being skipped for the 8k channel C.
func TestRelayCapacityRetry_Local413IsNotCapacityEvidence(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			a := capacityRetryChannel(1, "A-prio-10-4k", 10, 4096)
			b := capacityRetryChannel(2, "B-prio-5-4k", 5, 4096)
			c := capacityRetryChannel(3, "C-prio-0-8k", 0, 8192)
			setupRetryOrderDB(t, []*dbmodel.Channel{a, b, c})
			localTooLarge := func(id int) *model.ErrorWithStatusCode {
				if id != 1 {
					return nil
				}
				return openai.ErrorWrapper(errors.New("response fallback input exceeds the item limit"),
					"response_fallback_input_too_large", http.StatusRequestEntityTooLarge)
			}

			got := runRelayRetryScenario(t, a, localTooLarge, path.memoryCache, 0)

			require.Equal(t, []int{1, 2}, got.order, "a local one-api 413 is not evidence about the channel's capacity")
			require.Equal(t, http.StatusOK, got.status, string(got.body))
		})
	}
}

// TestRelayCapacityRetry_Upstream413SkipsEqualCapacity is the positive control
// for the local-413 case: an upstream 413 from the 4k channel A does prove that
// the 4k channel B cannot serve the request, so the walk goes to the 8k C.
func TestRelayCapacityRetry_Upstream413SkipsEqualCapacity(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			a := capacityRetryChannel(1, "A-prio-10-4k", 10, 4096)
			b := capacityRetryChannel(2, "B-prio-5-4k", 5, 4096)
			c := capacityRetryChannel(3, "C-prio-0-8k", 0, 8192)
			setupRetryOrderDB(t, []*dbmodel.Channel{a, b, c})

			got := runRelayRetryScenario(t, a, capacityScriptedStatuses(map[int]int{1: http.StatusRequestEntityTooLarge}), path.memoryCache, 0)

			require.Equal(t, []int{1, 3}, got.order, "an upstream 413 at 4k rules out the other 4k channel")
			require.Equal(t, http.StatusOK, got.status, string(got.body))
		})
	}
}

// TestRelayCapacityRetry_Upstream413SkipsSmallerCapacity reproduces the review
// finding that the 413 filter skipped only channels whose max_tokens EQUALS a
// rejected one: after the 8k channel A answered 413, the 4k channel B was still
// tried. A request too large for 8k is too large for 4k, so eligible channels
// must declare strictly more than 8k (or nothing), and the walk goes to 16k.
func TestRelayCapacityRetry_Upstream413SkipsSmallerCapacity(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			a := capacityRetryChannel(1, "A-prio-10-8k", 10, 8192)
			b := capacityRetryChannel(2, "B-prio-5-4k", 5, 4096)
			c := capacityRetryChannel(3, "C-prio-0-16k", 0, 16384)
			setupRetryOrderDB(t, []*dbmodel.Channel{a, b, c})

			got := runRelayRetryScenario(t, a, capacityScriptedStatuses(map[int]int{
				1: http.StatusRequestEntityTooLarge,
				2: http.StatusRequestEntityTooLarge,
			}), path.memoryCache, 0)

			require.Equal(t, []int{1, 3}, got.order, "a channel no larger than a rejecting one must be skipped")
			require.Equal(t, http.StatusOK, got.status, string(got.body))
		})
	}
}

// TestRelayCapacityRetry_413AfterInitial429RaisesFloor reproduces the review
// finding that the 413 policy was fixed by the initial failure: after A's 429
// the walk stayed a plain priority walk, so B's upstream 413 at 4k did not stop
// the 4k channel C from being tried. Capacity evidence must constrain the rest
// of the walk from the first upstream 413 on, whatever the initial failure was.
func TestRelayCapacityRetry_413AfterInitial429RaisesFloor(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			a := capacityRetryChannel(1, "A-prio-10-16k", 10, 16384)
			b := capacityRetryChannel(2, "B-prio-5-4k", 5, 4096)
			c := capacityRetryChannel(3, "C-prio-3-4k", 3, 4096)
			d := capacityRetryChannel(4, "D-prio-0-8k", 0, 8192)
			setupRetryOrderDB(t, []*dbmodel.Channel{a, b, c, d})

			got := runRelayRetryScenario(t, a, capacityScriptedStatuses(map[int]int{
				1: http.StatusTooManyRequests,
				2: http.StatusRequestEntityTooLarge,
				3: http.StatusRequestEntityTooLarge,
			}), path.memoryCache, 3)

			require.Equal(t, []int{1, 2, 4}, got.order, "after B's upstream 413 at 4k, the 4k channel C cannot help")
			require.Equal(t, http.StatusOK, got.status, string(got.body))
		})
	}
}

// TestRelayCapacityRetry_NonCapacityFailureSizesStayEligible reproduces the
// review finding that rejected sizes were read from EVERY excluded channel: the
// 16k channel B failed with a 5xx and was treated as having rejected 16k, so the
// healthy 16k channel C was skipped. Only an upstream 413 is capacity evidence.
func TestRelayCapacityRetry_NonCapacityFailureSizesStayEligible(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			a := capacityRetryChannel(1, "A-prio-10-8k", 10, 8192)
			b := capacityRetryChannel(2, "B-prio-5-16k", 5, 16384)
			c := capacityRetryChannel(3, "C-prio-0-16k", 0, 16384)
			setupRetryOrderDB(t, []*dbmodel.Channel{a, b, c})

			got := runRelayRetryScenario(t, a, capacityScriptedStatuses(map[int]int{
				1: http.StatusRequestEntityTooLarge,
				2: http.StatusInternalServerError,
			}), path.memoryCache, 0)

			require.Equal(t, []int{1, 2, 3}, got.order, "a 5xx says nothing about capacity")
			require.Equal(t, http.StatusOK, got.status, string(got.body))
		})
	}
}

// TestRelayCapacityRetry_UntriedChannelSizesStayEligible covers the same defect
// for a channel excluded without being tried because it does not serve the
// endpoint: its 16k size never rejected anything, so the 16k channel C stays
// eligible after the 8k channel A answered 413.
func TestRelayCapacityRetry_UntriedChannelSizesStayEligible(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			a := capacityRetryChannel(1, "A-prio-10-8k", 10, 8192)
			b := capacityRetryChannel(2, "B-prio-5-16k-embeddings-only", 5, 16384)
			b.Config = `{"supported_endpoints":["embeddings"]}`
			c := capacityRetryChannel(3, "C-prio-0-16k", 0, 16384)
			setupRetryOrderDB(t, []*dbmodel.Channel{a, b, c})

			got := runRelayRetryScenario(t, a, capacityScriptedStatuses(map[int]int{1: http.StatusRequestEntityTooLarge}), path.memoryCache, 0)

			require.Equal(t, []int{1, 3}, got.order, "an endpoint-incompatible channel is skipped without trying it")
			require.Equal(t, http.StatusOK, got.status, string(got.body))
		})
	}
}

// TestRelayCapacityRetry_UndeclaredCapacityStaysEligible pins how undeclared
// limits behave: a rejection by a channel without a declared max_tokens proves
// nothing about other channels, and a candidate without a declared limit is
// never proven too small, whether it has a model config without max_tokens or
// no model config at all.
func TestRelayCapacityRetry_UndeclaredCapacityStaysEligible(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name+"/undeclared-rejections", func(t *testing.T) {
			a := capacityRetryChannel(1, "A-prio-10-undeclared", 10, 0)
			b := capacityRetryChannel(2, "B-prio-5-undeclared", 5, 0)
			c := capacityRetryChannel(3, "C-prio-0-no-config", 0, -1)
			setupRetryOrderDB(t, []*dbmodel.Channel{a, b, c})

			got := runRelayRetryScenario(t, a, capacityScriptedStatuses(map[int]int{
				1: http.StatusRequestEntityTooLarge,
				2: http.StatusRequestEntityTooLarge,
			}), path.memoryCache, 0)

			require.Equal(t, []int{1, 2, 3}, got.order, "undeclared capacities are never proven too small")
			require.Equal(t, http.StatusOK, got.status, string(got.body))
		})
		t.Run(path.name+"/undeclared-candidate", func(t *testing.T) {
			a := capacityRetryChannel(1, "A-prio-10-8k", 10, 8192)
			b := capacityRetryChannel(2, "B-prio-5-no-config", 5, -1)
			c := capacityRetryChannel(3, "C-prio-0-4k", 0, 4096)
			setupRetryOrderDB(t, []*dbmodel.Channel{a, b, c})

			got := runRelayRetryScenario(t, a, capacityScriptedStatuses(map[int]int{
				1: http.StatusRequestEntityTooLarge,
				2: http.StatusRequestEntityTooLarge,
			}), path.memoryCache, 0)

			require.Equal(t, []int{1, 2}, got.order, "the undeclared channel is tried; the 4k channel is skipped")
			require.Equal(t, http.StatusRequestEntityTooLarge, got.status, string(got.body))
		})
	}
}
