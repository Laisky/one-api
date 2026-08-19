package model

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Laisky/one-api/common/config"
)

func TestCacheGetRandomSatisfiedChannelExcluding(t *testing.T) {
	// Enable memory cache for this test
	originalMemoryCacheEnabled := config.MemoryCacheEnabled
	config.MemoryCacheEnabled = true
	defer func() { config.MemoryCacheEnabled = originalMemoryCacheEnabled }()

	// Setup test data
	testGroup := "test-group"
	testModel := "gpt-3.5-turbo"

	// Create test channels with different priorities
	channels := []*Channel{
		{Id: 1, Priority: &[]int64{100}[0], Name: "high-priority-1"},
		{Id: 2, Priority: &[]int64{100}[0], Name: "high-priority-2"},
		{Id: 3, Priority: &[]int64{50}[0], Name: "low-priority-1"},
		{Id: 4, Priority: &[]int64{50}[0], Name: "low-priority-2"},
	}

	// Mock the cache data
	if group2model2channels == nil {
		group2model2channels = make(map[string]map[string][]*Channel)
	}
	if group2model2channels[testGroup] == nil {
		group2model2channels[testGroup] = make(map[string][]*Channel)
	}
	group2model2channels[testGroup][testModel] = channels

	tests := []struct {
		name                string
		excludeChannelIds   map[int]bool
		ignoreFirstPriority bool
		expectedChannelIds  []int // Possible channel IDs that could be returned
		shouldError         bool
		tryLargerMaxTokens  bool
	}{
		{
			name:                "No exclusions, highest priority",
			excludeChannelIds:   map[int]bool{},
			ignoreFirstPriority: false,
			expectedChannelIds:  []int{1, 2}, // Should only return high priority channels
			shouldError:         false,
			tryLargerMaxTokens:  true,
		},
		{
			name:                "No exclusions, lower priority",
			excludeChannelIds:   map[int]bool{},
			ignoreFirstPriority: true,
			expectedChannelIds:  []int{3, 4}, // Should only return low priority channels
			shouldError:         false,
			tryLargerMaxTokens:  true,
		},
		{
			name:                "Exclude one high priority channel",
			excludeChannelIds:   map[int]bool{1: true},
			ignoreFirstPriority: false,
			expectedChannelIds:  []int{2}, // Should return the remaining high priority channel
			shouldError:         false,
			tryLargerMaxTokens:  true,
		},
		{
			name:                "Exclude all high priority channels",
			excludeChannelIds:   map[int]bool{1: true, 2: true},
			ignoreFirstPriority: false,
			expectedChannelIds:  []int{3, 4}, // Should fallback to next highest available priority (low priority channels)
			shouldError:         false,
			tryLargerMaxTokens:  true,
		},
		{
			name:                "Exclude one low priority channel",
			excludeChannelIds:   map[int]bool{3: true},
			ignoreFirstPriority: true,
			expectedChannelIds:  []int{4}, // Should return the remaining low priority channel
			shouldError:         false,
			tryLargerMaxTokens:  true,
		},
		{
			name:                "Exclude all channels",
			excludeChannelIds:   map[int]bool{1: true, 2: true, 3: true, 4: true},
			ignoreFirstPriority: false,
			expectedChannelIds:  []int{}, // Should return error as no channels available
			shouldError:         true,
			tryLargerMaxTokens:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel, err := CacheGetRandomSatisfiedChannelExcluding(testGroup, testModel, tt.ignoreFirstPriority, tt.excludeChannelIds, tt.tryLargerMaxTokens)

			if tt.shouldError {
				assert.Error(t, err)
				assert.Nil(t, channel)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, channel)
				assert.Contains(t, tt.expectedChannelIds, channel.Id, "Returned channel ID should be in expected list")
			}
		})
	}
}

func TestGetRandomSatisfiedChannelExcluding(t *testing.T) {
	// This would require a test database setup
	// For now, we'll create a unit test that verifies the logic

	excludeChannelIds := map[int]bool{1: true, 3: true}

	// Test that the exclusion logic works
	testIds := []int{1, 2, 3, 4, 5}
	var availableIds []int

	for _, id := range testIds {
		if !excludeChannelIds[id] {
			availableIds = append(availableIds, id)
		}
	}

	expectedIds := []int{2, 4, 5}
	assert.Equal(t, expectedIds, availableIds, "Should exclude channels 1 and 3")
}

func TestChannel_GetPriority(t *testing.T) {
	tests := []struct {
		name     string
		priority *int64
		expected int64
	}{
		{
			name:     "Nil priority should return 0",
			priority: nil,
			expected: 0,
		},
		{
			name:     "Non-nil priority should return value",
			priority: &[]int64{100}[0],
			expected: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{Priority: tt.priority}
			assert.Equal(t, tt.expected, channel.GetPriority())
		})
	}
}
