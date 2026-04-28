package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

func setupTestDB(t *testing.T) *gorm.DB {
	// Create in-memory SQLite database for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto-migrate the tables
	err = db.AutoMigrate(&Channel{}, &Ability{})
	require.NoError(t, err)

	return db
}

func TestGetRandomSatisfiedChannelExcluding_PriorityLogic(t *testing.T) {
	// Setup test database
	testDB := setupTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	// Set SQLite flag for proper query handling
	originalUsingSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	defer func() { common.UsingSQLite.Store(originalUsingSQLite) }()

	// Create test channels with different priorities
	channels := []Channel{
		{Id: 1, Name: "high-priority-1", Status: ChannelStatusEnabled, Models: "gpt-3.5-turbo", Group: "default", Priority: &[]int64{100}[0]},
		{Id: 2, Name: "high-priority-2", Status: ChannelStatusEnabled, Models: "gpt-3.5-turbo", Group: "default", Priority: &[]int64{100}[0]},
		{Id: 3, Name: "medium-priority-1", Status: ChannelStatusEnabled, Models: "gpt-3.5-turbo", Group: "default", Priority: &[]int64{50}[0]},
		{Id: 4, Name: "medium-priority-2", Status: ChannelStatusEnabled, Models: "gpt-3.5-turbo", Group: "default", Priority: &[]int64{50}[0]},
		{Id: 5, Name: "low-priority-1", Status: ChannelStatusEnabled, Models: "gpt-3.5-turbo", Group: "default", Priority: &[]int64{10}[0]},
		{Id: 6, Name: "low-priority-2", Status: ChannelStatusEnabled, Models: "gpt-3.5-turbo", Group: "default", Priority: &[]int64{10}[0]},
	}

	// Insert channels
	for _, channel := range channels {
		err := DB.Create(&channel).Error
		require.NoError(t, err)
		err = channel.AddAbilities()
		require.NoError(t, err)
	}

	testGroup := "default"
	testModel := "gpt-3.5-turbo"

	tests := []struct {
		name                string
		excludeChannelIds   map[int]bool
		ignoreFirstPriority bool
		expectedPriorities  []int64 // Expected priority levels that could be returned
		shouldError         bool
		description         string
	}{
		{
			name:                "No exclusions, highest priority",
			excludeChannelIds:   map[int]bool{},
			ignoreFirstPriority: false,
			expectedPriorities:  []int64{100}, // Should only return highest priority (100)
			shouldError:         false,
			description:         "Should select from highest priority channels when ignoreFirstPriority=false",
		},
		{
			name:                "No exclusions, lower priority",
			excludeChannelIds:   map[int]bool{},
			ignoreFirstPriority: true,
			expectedPriorities:  []int64{50, 10}, // Should return medium or low priority (not 100)
			shouldError:         false,
			description:         "Should select from lower priority channels when ignoreFirstPriority=true",
		},
		{
			name:                "Exclude one high priority channel",
			excludeChannelIds:   map[int]bool{1: true},
			ignoreFirstPriority: false,
			expectedPriorities:  []int64{100}, // Should still return high priority (remaining channel 2)
			shouldError:         false,
			description:         "Should select from remaining highest priority channels",
		},
		{
			name:                "Exclude all high priority channels, request highest",
			excludeChannelIds:   map[int]bool{1: true, 2: true},
			ignoreFirstPriority: false,
			expectedPriorities:  []int64{50}, // Should return next highest priority (50)
			shouldError:         false,
			description:         "Should fallback to next highest priority when highest are excluded",
		},
		{
			name:                "Exclude all high priority channels, request lower",
			excludeChannelIds:   map[int]bool{1: true, 2: true},
			ignoreFirstPriority: true,
			expectedPriorities:  []int64{10}, // Should return priority lower than 50 (which is 10)
			shouldError:         false,
			description:         "Should select from priorities lower than the new maximum",
		},
		{
			name:                "Exclude high and medium priority channels",
			excludeChannelIds:   map[int]bool{1: true, 2: true, 3: true, 4: true},
			ignoreFirstPriority: false,
			expectedPriorities:  []int64{10}, // Should return low priority (10)
			shouldError:         false,
			description:         "Should fallback to lowest priority when higher ones are excluded",
		},
		{
			name:                "Exclude high and medium, request lower priority",
			excludeChannelIds:   map[int]bool{1: true, 2: true, 3: true, 4: true},
			ignoreFirstPriority: true,
			expectedPriorities:  []int64{}, // No channels with priority < 10
			shouldError:         true,
			description:         "Should error when no lower priority channels exist",
		},
		{
			name:                "Exclude all channels",
			excludeChannelIds:   map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true},
			ignoreFirstPriority: false,
			expectedPriorities:  []int64{},
			shouldError:         true,
			description:         "Should error when all channels are excluded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel, err := GetRandomSatisfiedChannelExcluding(testGroup, testModel, tt.ignoreFirstPriority, tt.excludeChannelIds)

			if tt.shouldError {
				assert.Error(t, err, tt.description)
				assert.Nil(t, channel)
			} else {
				assert.NoError(t, err, tt.description)
				assert.NotNil(t, channel)

				// Verify the returned channel has the expected priority
				channelPriority := channel.GetPriority()
				assert.Contains(t, tt.expectedPriorities, channelPriority,
					"Returned channel priority %d should be in expected priorities %v. %s",
					channelPriority, tt.expectedPriorities, tt.description)

				// Verify the channel is not in the excluded list
				assert.False(t, tt.excludeChannelIds[channel.Id],
					"Returned channel %d should not be in excluded list", channel.Id)
			}
		})
	}
}

func TestGetRandomSatisfiedChannelExcluding_SuspendedChannels(t *testing.T) {
	// Setup test database
	testDB := setupTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	// Set SQLite flag for proper query handling
	originalUsingSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	defer func() { common.UsingSQLite.Store(originalUsingSQLite) }()

	// Create test channels
	channels := []Channel{
		{Id: 1, Name: "active-channel", Status: ChannelStatusEnabled, Models: "gpt-3.5-turbo", Group: "default", Priority: &[]int64{100}[0]},
		{Id: 2, Name: "suspended-channel", Status: ChannelStatusEnabled, Models: "gpt-3.5-turbo", Group: "default", Priority: &[]int64{100}[0]},
	}

	// Insert channels and abilities
	for _, channel := range channels {
		err := DB.Create(&channel).Error
		require.NoError(t, err)
		err = channel.AddAbilities()
		require.NoError(t, err)
	}

	// Suspend one channel's ability
	futureTime := time.Now().Add(1 * time.Hour)
	err := DB.Model(&Ability{}).Where("channel_id = ? AND model = ? AND `group` = ?", 2, "gpt-3.5-turbo", "default").
		Update("suspend_until", futureTime).Error
	require.NoError(t, err)

	// Test that suspended channels are not selected
	channel, err := GetRandomSatisfiedChannelExcluding("default", "gpt-3.5-turbo", false, map[int]bool{})
	assert.NoError(t, err)
	assert.NotNil(t, channel)
	assert.Equal(t, 1, channel.Id, "Should only return the non-suspended channel")
}

func TestChannelHiddenModelsHelpers(t *testing.T) {
	testDB := setupTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	hidden := `[" ModelA ","modela","Other",""]`
	channel := &Channel{
		Id:           1,
		Name:         "hidden-helper",
		Status:       ChannelStatusEnabled,
		Models:       "ModelA,Public",
		Group:        "default",
		HiddenModels: &hidden,
	}
	require.NoError(t, channel.NormalizeHiddenModels())
	require.NotNil(t, channel.HiddenModels)
	require.JSONEq(t, `["ModelA","Other"]`, *channel.HiddenModels)
	require.True(t, channel.IsModelHidden("modela"))
	require.True(t, channel.IsModelHidden("MODELA"))
	require.False(t, channel.IsModelHidden("Other"), "hidden models outside Models should be treated as no-ops")

	hiddenSet := channel.GetHiddenModels()
	_, ok := hiddenSet["modela"]
	require.True(t, ok)
	_, ok = hiddenSet["other"]
	require.False(t, ok)
}

func TestChannelUpdateInvalidatesHiddenModelCache(t *testing.T) {
	testDB := setupTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	originalUsingSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	defer func() { common.UsingSQLite.Store(originalUsingSQLite) }()

	group := fmt.Sprintf("group-%d", time.Now().UnixNano())
	channel := &Channel{
		Name:   "hidden-cache",
		Type:   1,
		Status: ChannelStatusEnabled,
		Models: "hidden-alpha,public-alias",
		Group:  group,
	}
	require.NoError(t, channel.Insert())

	abilities, err := GetGroupModelsV2(context.Background(), group)
	require.NoError(t, err)
	require.Len(t, abilities, 2)

	hidden := `["hidden-alpha"]`
	channel.HiddenModels = &hidden
	channel.HiddenModelsProvided = true
	require.NoError(t, channel.Update())

	abilities, err = GetGroupModelsV2(context.Background(), group)
	require.NoError(t, err)
	require.Len(t, abilities, 1)
	require.Equal(t, "public-alias", abilities[0].Model)

	var dbAbilities []Ability
	require.NoError(t, DB.Order("model").Find(&dbAbilities).Error)
	require.Len(t, dbAbilities, 1)
	require.Equal(t, "public-alias", dbAbilities[0].Model)
}
