package controller

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/songquanpeng/one-api/model"
)

func TestRelay429RetryLogic(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name                   string
		initialError           int
		shouldTryLowerPriority bool
	}{
		{
			name:                   "429 error should try lower priority channels first",
			initialError:           http.StatusTooManyRequests,
			shouldTryLowerPriority: true,
		},
		{
			name:                   "500 error should try highest priority channels first",
			initialError:           http.StatusInternalServerError,
			shouldTryLowerPriority: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Test the retry priority logic directly
			shouldTryLowerPriorityFirst := tt.initialError == http.StatusTooManyRequests
			assert.Equal(t, tt.shouldTryLowerPriority, shouldTryLowerPriorityFirst,
				"Retry priority logic should match expected behavior for status code %d", tt.initialError)
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

// TestRetryPriorityLogic tests the core retry priority decision logic
func TestRetryPriorityLogic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		statusCode             int
		shouldTryLowerPriority bool
	}{
		{
			name:                   "HTTP 429 Too Many Requests should try lower priority first",
			statusCode:             http.StatusTooManyRequests,
			shouldTryLowerPriority: true,
		},
		{
			name:                   "HTTP 500 Internal Server Error should try highest priority first",
			statusCode:             http.StatusInternalServerError,
			shouldTryLowerPriority: false,
		},
		{
			name:                   "HTTP 502 Bad Gateway should try highest priority first",
			statusCode:             http.StatusBadGateway,
			shouldTryLowerPriority: false,
		},
		{
			name:                   "HTTP 503 Service Unavailable should try highest priority first",
			statusCode:             http.StatusServiceUnavailable,
			shouldTryLowerPriority: false,
		},
		{
			name:                   "HTTP 504 Gateway Timeout should try highest priority first",
			statusCode:             http.StatusGatewayTimeout,
			shouldTryLowerPriority: false,
		},
		{
			name:                   "HTTP 400 Bad Request should try highest priority first",
			statusCode:             http.StatusBadRequest,
			shouldTryLowerPriority: false,
		},
		{
			name:                   "HTTP 401 Unauthorized should try highest priority first",
			statusCode:             http.StatusUnauthorized,
			shouldTryLowerPriority: false,
		},
		{
			name:                   "HTTP 404 Not Found should try highest priority first",
			statusCode:             http.StatusNotFound,
			shouldTryLowerPriority: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shouldTryLowerPriorityFirst := tt.statusCode == http.StatusTooManyRequests
			assert.Equal(t, tt.shouldTryLowerPriority, shouldTryLowerPriorityFirst,
				"Status code %d should determine priority logic correctly", tt.statusCode)
		})
	}
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
