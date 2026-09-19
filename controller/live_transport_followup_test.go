package controller

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestRelayRESTMismatchDoesNotConsumeUpstreamRetry verifies that a local Google
// mismatch encountered after a real failure does not hide the next valid bridge.
// Parameters: t owns the test. Returns: none.
func TestRelayRESTMismatchDoesNotConsumeUpstreamRetry(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			first := liveTransportRetryChannel(91, "first-bridge", channeltype.OpenAICompatible, 30)
			google := liveTransportRetryChannel(92, "google-rest", channeltype.Gemini, 20)
			last := liveTransportRetryChannel(93, "last-bridge", channeltype.OpenAICompatible, 10)
			setupRetryOrderDB(t, []*dbmodel.Channel{first, google, last})
			result := runRelayRetryScenarioForModel(t, first, liveTransportRetryModel, failOnly(serverError, first.Id), path.memoryCache, 1)
			require.Equal(t, http.StatusOK, result.status)
			require.Equal(t, []int{first.Id, last.Id}, result.order)
		})
	}
}

// TestRelayRESTMismatchDoesNotExpandPaidRetryBudget verifies that skipping an
// incompatible initial channel still honors zero retries for real provider calls.
// Parameters: t owns the test. Returns: none.
func TestRelayRESTMismatchDoesNotExpandPaidRetryBudget(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			google := liveTransportRetryChannel(101, "google-rest", channeltype.Gemini, 30)
			first := liveTransportRetryChannel(102, "first-bridge", channeltype.OpenAICompatible, 20)
			last := liveTransportRetryChannel(103, "last-bridge", channeltype.OpenAICompatible, 10)
			setupRetryOrderDB(t, []*dbmodel.Channel{google, first, last})
			result := runRelayRetryScenarioForModel(t, google, liveTransportRetryModel, failOnly(serverError, first.Id), path.memoryCache, 0)
			require.Equal(t, http.StatusInternalServerError, result.status)
			require.Equal(t, []int{first.Id}, result.order)
		})
	}
}

// TestRelayRESTMismatchCannotMaskProviderFailure verifies that exhausting local
// incompatible candidates does not turn a real provider fault into a caller error.
// Parameters: t owns the test. Returns: none.
func TestRelayRESTMismatchCannotMaskProviderFailure(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			first := liveTransportRetryChannel(111, "first-bridge", channeltype.OpenAICompatible, 20)
			google := liveTransportRetryChannel(112, "google-rest", channeltype.Gemini, 10)
			setupRetryOrderDB(t, []*dbmodel.Channel{first, google})
			result := runRelayRetryScenarioForModel(t, first, liveTransportRetryModel, serverError, path.memoryCache, 1)
			require.Equal(t, http.StatusInternalServerError, result.status)
			require.Equal(t, []int{first.Id}, result.order)
			require.NotContains(t, string(result.body), "unsupported_model_transport")
		})
	}
}

// TestRelayRESTFallbackStopsOnTerminalProviderError verifies that routing past
// a local mismatch does not authorize replay after a real non-retryable 400.
// Parameters: t owns the test. Returns: none.
func TestRelayRESTFallbackStopsOnTerminalProviderError(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			google := liveTransportRetryChannel(121, "google-rest", channeltype.Gemini, 30)
			first := liveTransportRetryChannel(122, "first-bridge", channeltype.OpenAICompatible, 20)
			last := liveTransportRetryChannel(123, "last-bridge", channeltype.OpenAICompatible, 10)
			setupRetryOrderDB(t, []*dbmodel.Channel{google, first, last})
			result := runRelayRetryScenarioForModel(t, google, liveTransportRetryModel, func(int) *relaymodel.ErrorWithStatusCode {
				return &relaymodel.ErrorWithStatusCode{StatusCode: http.StatusBadRequest, Error: relaymodel.Error{
					Message: "invalid user payload", Type: relaymodel.ErrorTypeInvalidRequest, Code: "invalid_request",
				}}
			}, path.memoryCache, 2)
			require.Equal(t, http.StatusBadRequest, result.status)
			require.Equal(t, []int{first.Id}, result.order)
		})
	}
}
