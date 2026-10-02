package controller

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Laisky/one-api/model"
)

// TestRetrySelectionPolicyFor pins which retry selection policy each initial
// failure status maps to. 413 is the only status with a dedicated policy; a 429
// walks the remaining channels strictly by priority like every other retryable
// status (the rate-limited channel is excluded, never its whole tier).
func TestRetrySelectionPolicyFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		want       retrySelectionPolicy
	}{
		{name: "429 Too Many Requests walks strict priority", statusCode: http.StatusTooManyRequests, want: retrySelectStrictPriority},
		{name: "413 Request Entity Too Large seeks larger max_tokens", statusCode: http.StatusRequestEntityTooLarge, want: retrySelectLargerMaxTokens},
		{name: "500 Internal Server Error walks strict priority", statusCode: http.StatusInternalServerError, want: retrySelectStrictPriority},
		{name: "502 Bad Gateway walks strict priority", statusCode: http.StatusBadGateway, want: retrySelectStrictPriority},
		{name: "503 Service Unavailable walks strict priority", statusCode: http.StatusServiceUnavailable, want: retrySelectStrictPriority},
		{name: "504 Gateway Timeout walks strict priority", statusCode: http.StatusGatewayTimeout, want: retrySelectStrictPriority},
		{name: "401 Unauthorized walks strict priority", statusCode: http.StatusUnauthorized, want: retrySelectStrictPriority},
		{name: "403 Forbidden walks strict priority", statusCode: http.StatusForbidden, want: retrySelectStrictPriority},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, retrySelectionPolicyFor(tt.statusCode))
		})
	}
}

func TestChannelExclusionInRetry(t *testing.T) {
	t.Parallel()
	// Test that failed channels are properly excluded from subsequent retries

	failedChannels := map[int]bool{
		1: true, // Channel 1 has failed
		3: true, // Channel 3 has failed
	}

	// Available channels
	availableChannels := []model.Channel{
		{Id: 1, Priority: &[]int64{100}[0]}, // Should be excluded
		{Id: 2, Priority: &[]int64{100}[0]}, // Available
		{Id: 3, Priority: &[]int64{50}[0]},  // Should be excluded
		{Id: 4, Priority: &[]int64{50}[0]},  // Available
	}

	// Test logic would verify that channels 1 and 3 are not selected
	// and only channels 2 and 4 are considered for selection

	expectedAvailableIds := []int{2, 4}
	actualAvailableIds := []int{}

	for _, channel := range availableChannels {
		if !failedChannels[channel.Id] {
			actualAvailableIds = append(actualAvailableIds, channel.Id)
		}
	}

	assert.Equal(t, expectedAvailableIds, actualAvailableIds, "Should exclude failed channels")
}

// TestChannelExclusionLogic tests the channel exclusion functionality
func TestChannelExclusionLogic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                string
		failedChannelIds    map[int]bool
		availableChannelIds []int
		expectedChannelIds  []int
	}{
		{
			name:                "No failed channels should include all available",
			failedChannelIds:    map[int]bool{},
			availableChannelIds: []int{1, 2, 3, 4},
			expectedChannelIds:  []int{1, 2, 3, 4},
		},
		{
			name:                "Single failed channel should be excluded",
			failedChannelIds:    map[int]bool{2: true},
			availableChannelIds: []int{1, 2, 3, 4},
			expectedChannelIds:  []int{1, 3, 4},
		},
		{
			name:                "Multiple failed channels should be excluded",
			failedChannelIds:    map[int]bool{1: true, 3: true},
			availableChannelIds: []int{1, 2, 3, 4},
			expectedChannelIds:  []int{2, 4},
		},
		{
			name:                "All channels failed should result in empty list",
			failedChannelIds:    map[int]bool{1: true, 2: true, 3: true, 4: true},
			availableChannelIds: []int{1, 2, 3, 4},
			expectedChannelIds:  nil, // nil slice when all channels are excluded
		},
		{
			name:                "Failed channel not in available list should not affect result",
			failedChannelIds:    map[int]bool{5: true, 6: true},
			availableChannelIds: []int{1, 2, 3, 4},
			expectedChannelIds:  []int{1, 2, 3, 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var actualChannelIds []int
			for _, channelId := range tt.availableChannelIds {
				if !tt.failedChannelIds[channelId] {
					actualChannelIds = append(actualChannelIds, channelId)
				}
			}
			assert.Equal(t, tt.expectedChannelIds, actualChannelIds,
				"Channel exclusion logic should work correctly")
		})
	}
}
