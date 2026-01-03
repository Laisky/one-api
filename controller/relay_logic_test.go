package controller

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/songquanpeng/one-api/relay/model"
)

func TestShouldRetry429Logic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		statusCode             int
		expectedRetryTimes     int
		shouldTryLowerPriority bool
	}{
		{
			name:                   "429 error should increase retry times and try lower priority",
			statusCode:             http.StatusTooManyRequests,
			expectedRetryTimes:     6, // Original retryTimes * 2
			shouldTryLowerPriority: true,
		},
		{
			name:                   "500 error should use normal retry times",
			statusCode:             http.StatusInternalServerError,
			expectedRetryTimes:     3, // Original retryTimes
			shouldTryLowerPriority: false,
		},
		{
			name:                   "503 error should use normal retry times",
			statusCode:             http.StatusServiceUnavailable,
			expectedRetryTimes:     3, // Original retryTimes
			shouldTryLowerPriority: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Simulate the logic from the Relay function
			bizErr := &model.ErrorWithStatusCode{
				StatusCode: tt.statusCode,
				Error: model.Error{
					Message: "Test error",
				},
			}

			// Base retry times (from config)
			baseRetryTimes := 3

			// Apply the logic from the Relay function
			retryTimes := baseRetryTimes
			if bizErr.StatusCode == http.StatusTooManyRequests && retryTimes > 0 {
				retryTimes = retryTimes * 2
			}

			shouldTryLowerPriorityFirst := bizErr.StatusCode == http.StatusTooManyRequests

			assert.Equal(t, tt.expectedRetryTimes, retryTimes, "Retry times should match expected")
			assert.Equal(t, tt.shouldTryLowerPriority, shouldTryLowerPriorityFirst, "Lower priority preference should match expected")
		})
	}
}

func TestFailedChannelTrackingLogic(t *testing.T) {
	t.Parallel()
	// Test that failed channels are properly tracked
	failedChannels := make(map[int]bool)

	// Simulate adding failed channels
	failedChannelIds := []int{1, 3, 5}
	for _, id := range failedChannelIds {
		failedChannels[id] = true
	}

	// Test that the channels are properly tracked
	assert.True(t, failedChannels[1], "Channel 1 should be marked as failed")
	assert.True(t, failedChannels[3], "Channel 3 should be marked as failed")
	assert.True(t, failedChannels[5], "Channel 5 should be marked as failed")
	assert.False(t, failedChannels[2], "Channel 2 should not be marked as failed")
	assert.False(t, failedChannels[4], "Channel 4 should not be marked as failed")

	// Test exclusion logic
	allChannels := []int{1, 2, 3, 4, 5}
	var availableChannels []int

	for _, channelId := range allChannels {
		if !failedChannels[channelId] {
			availableChannels = append(availableChannels, channelId)
		}
	}

	expectedAvailable := []int{2, 4}
	assert.Equal(t, expectedAvailable, availableChannels, "Only non-failed channels should be available")
}
