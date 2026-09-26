package controller

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

const liveTransportRetryModel = "gemini-3.8-live"

// liveTransportRetryChannel creates one channel in the transport-routing test
// pool. Parameters: id identifies the channel, name labels it, channelType
// selects the adaptor, and priority selects routing order. Returns: a routable
// channel advertising the Live-only model.
func liveTransportRetryChannel(id int, name string, channelType int, priority int64) *dbmodel.Channel {
	return &dbmodel.Channel{
		Id:       id,
		Name:     name,
		Type:     channelType,
		Status:   dbmodel.ChannelStatusEnabled,
		Models:   liveTransportRetryModel,
		Group:    retryOrderGroup,
		Priority: &priority,
	}
}

// TestRelayLiveOnlyRESTMismatchFallsBackToCompatibleBridge reproduces the
// routing failure where a high-priority Google REST channel returned a terminal
// 400 before a lower-priority third-party bridge could be selected. Parameters:
// t is the test handle. Returns: none.
func TestRelayLiveOnlyRESTMismatchFallsBackToCompatibleBridge(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			google := liveTransportRetryChannel(71, "google-rest", channeltype.Gemini, 10)
			bridge := liveTransportRetryChannel(72, "third-party-bridge", channeltype.OpenAICompatible, 5)
			setupRetryOrderDB(t, []*dbmodel.Channel{google, bridge})

			result := runRelayRetryScenarioForModel(t, google, liveTransportRetryModel, func(channelID int) *relaymodel.ErrorWithStatusCode {
				require.Equal(t, bridge.Id, channelID, "the Google REST mismatch must be excluded before adaptor dispatch")
				return nil
			}, path.memoryCache, 0)

			require.Equal(t, http.StatusOK, result.status)
			require.Equal(t, []int{bridge.Id}, result.order, "only the compatible bridge reaches adaptor dispatch")
			require.Equal(t, 1, result.processedErrors, "the mismatched Google channel is recorded once without a health penalty")
		})
	}
}

// TestRelayLiveOnlyRESTMismatchReturns400WhenNoBridgeExists verifies that the
// client still receives the transport guidance when every eligible channel is a
// Google REST mismatch. Parameters: t is the test handle. Returns: none.
func TestRelayLiveOnlyRESTMismatchReturns400WhenNoBridgeExists(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			google := liveTransportRetryChannel(81, "google-rest", channeltype.Gemini, 10)
			vertex := liveTransportRetryChannel(82, "vertex-rest", channeltype.VertextAI, 5)
			setupRetryOrderDB(t, []*dbmodel.Channel{google, vertex})

			result := runRelayRetryScenarioForModel(t, google, liveTransportRetryModel, func(channelID int) *relaymodel.ErrorWithStatusCode {
				t.Fatalf("incompatible channel %d reached adaptor dispatch", channelID)
				return nil
			}, path.memoryCache, 0)

			require.Equal(t, http.StatusBadRequest, result.status)
			require.Empty(t, result.order)
			require.Equal(t, 2, result.processedErrors)
			require.Contains(t, string(result.body), "unsupported_model_transport")
		})
	}
}

// TestRelayDoesNotReplayPossiblyForwardedPaidVideo verifies the actual router
// retry loop makes only one creation attempt when the first channel may already
// have accepted and charged for the POST.
func TestRelayDoesNotReplayPossiblyForwardedPaidVideo(t *testing.T) {
	for _, path := range routingPaths {
		t.Run(path.name, func(t *testing.T) {
			modelName := "veo3-fast"
			priorityA, priorityB := int64(10), int64(5)
			first := &dbmodel.Channel{
				Id: 171, Name: "first-paid-video", Type: channeltype.OpenAI,
				Status: dbmodel.ChannelStatusEnabled, Models: modelName,
				Group: retryOrderGroup, Priority: &priorityA,
			}
			second := &dbmodel.Channel{
				Id: 172, Name: "second-paid-video", Type: channeltype.OpenAI,
				Status: dbmodel.ChannelStatusEnabled, Models: modelName,
				Group: retryOrderGroup, Priority: &priorityB,
			}
			setupRetryOrderDB(t, []*dbmodel.Channel{first, second})

			result := runRelayRetryScenarioForRequest(t, first, modelName, http.MethodPost, "/v1/videos", func(channelID int) *relaymodel.ErrorWithStatusCode {
				require.Equal(t, first.Id, channelID)
				return serverError(channelID)
			}, path.memoryCache, 2)

			require.Equal(t, []int{first.Id}, result.order)
			require.Equal(t, http.StatusInternalServerError, result.status)
		})
	}
}
