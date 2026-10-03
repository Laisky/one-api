package controller

import (
	"fmt"
	"strings"
	"testing"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
	"net/http"
)

// TestAsyncVideo413CannotReopenPaidReplay drives the outer capacity-budget
// expansion and real channel selection after the paid-submission veto.
func TestAsyncVideo413CannotReopenPaidReplay(t *testing.T) {
	for _, memory := range []bool{false, true} {
		for _, budget := range []int{0, 3} {
			for _, path := range []string{"/v1/videos", "/v1/videos/generations", "/v1/async/videos"} {
				t.Run(fmt.Sprintf("memory=%t/retries=%d/endpoint=%s", memory, budget, strings.ReplaceAll(path, "/", "_")), func(t *testing.T) {
					first, second := retryOrderChannel(1, "paid-first", 100), retryOrderChannel(2, "larger-second", 90)
					first.ModelConfigs = stringPtr(`{"gpt-4o-mini":{"max_tokens":4096}}`)
					second.ModelConfigs = stringPtr(`{"gpt-4o-mini":{"max_tokens":8192}}`)
					if path == "/v1/async/videos" {
						first.Type, second.Type = channeltype.MuAPI, channeltype.MuAPI
					}
					setupRetryOrderDB(t, []*dbmodel.Channel{first, second})
					result := runRelayRetryScenarioForRequest(t, first, retryOrderModel, http.MethodPost, path,
						func(id int) *relaymodel.ErrorWithStatusCode {
							if id != first.Id {
								return nil
							}
							return &relaymodel.ErrorWithStatusCode{StatusCode: http.StatusRequestEntityTooLarge,
								Error: relaymodel.Error{Type: relaymodel.ErrorTypeUpstream, Message: "request too large"}}
						}, memory, budget)
					require.Equal(t, []int{first.Id}, result.order, "413 budget growth must not undo the paid-submission veto")
					require.Equal(t, http.StatusRequestEntityTooLarge, result.status)
				})
			}
		}
	}
}

// TestAsyncVideo413PreservesTextCapacityRecovery guards the non-paid positive
// control: a zero configured retry budget may still use a larger text channel.
func TestAsyncVideo413PreservesTextCapacityRecovery(t *testing.T) {
	first, second := retryOrderChannel(1, "text-first", 100), retryOrderChannel(2, "larger-text", 90)
	first.ModelConfigs = stringPtr(`{"gpt-4o-mini":{"max_tokens":4096}}`)
	second.ModelConfigs = stringPtr(`{"gpt-4o-mini":{"max_tokens":8192}}`)
	setupRetryOrderDB(t, []*dbmodel.Channel{first, second})
	result := runRelayRetryScenario(t, first, func(id int) *relaymodel.ErrorWithStatusCode {
		if id != first.Id {
			return nil
		}
		return &relaymodel.ErrorWithStatusCode{StatusCode: http.StatusRequestEntityTooLarge,
			Error: relaymodel.Error{Type: relaymodel.ErrorTypeUpstream, Message: "request too large"}}
	}, true, 0)
	require.Equal(t, []int{first.Id, second.Id}, result.order)
	require.Equal(t, http.StatusOK, result.status)
}
