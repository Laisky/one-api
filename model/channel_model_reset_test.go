package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// resetTestCatalog supplies a deterministic provider catalog for persistence tests.
func resetTestCatalog(int) []string { return []string{"Foo", "Bar"} }

// TestResetChannelModelsToDefaultsRebuildsAbilities verifies the stored model list,
// visibility, routing groups, and channel status change as one coherent operation.
func TestResetChannelModelsToDefaultsRebuildsAbilities(t *testing.T) {
	for _, status := range []int{ChannelStatusEnabled, ChannelStatusManuallyDisabled, ChannelStatusAutoDisabled} {
		t.Run(string(rune('0'+status)), func(t *testing.T) {
			db := useChannelPersistenceTestDB(t)
			priority := int64(-3)
			channel := &Channel{
				Type: channeltype.OpenAI, Name: "reset-test", Key: "never-overwrite-this-key",
				Status: status, Models: "Retired,Foo", Group: "default,vip", Priority: &priority,
				HiddenModels: resetTestString(`["Foo"]`), TestingModel: resetTestString(ChannelTestingModelSkip),
				ModelConfigs: resetTestString(`{"Foo":{"ratio":2}}`),
			}
			require.NoError(t, channel.Insert())
			for range 2 {
				result, err := ResetChannelModelsToDefaults(context.Background(), channel.Id, resetTestCatalog)
				require.NoError(t, err)
				require.Empty(t, result.Key)
				var stored Channel
				require.NoError(t, db.First(&stored, "id = ?", channel.Id).Error)
				require.Equal(t, "Foo,Bar", stored.Models)
				require.Nil(t, stored.HiddenModels)
				require.Equal(t, channel.UUID, stored.UUID)
				require.Equal(t, channel.Name, stored.Name)
				require.Equal(t, channel.Key, stored.Key)
				require.Equal(t, status, stored.Status)
				require.Equal(t, channel.Priority, stored.Priority)
				require.Equal(t, channel.ModelConfigs, stored.ModelConfigs)
				require.Equal(t, channel.TestingModel, stored.TestingModel)
				var abilities []Ability
				require.NoError(t, db.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
				require.Len(t, abilities, 4)
				for _, ability := range abilities {
					require.Contains(t, []string{"Foo", "Bar"}, ability.Model)
					require.Contains(t, []string{"default", "vip"}, ability.Group)
					require.Equal(t, status == ChannelStatusEnabled, ability.Enabled)
					require.Equal(t, channel.Priority, ability.Priority)
				}
			}
		})
	}
}

// TestResetChannelModelsToDefaultsRollback verifies both preflight refusal and
// an ability-write failure preserve the complete channel and previous abilities.
func TestResetChannelModelsToDefaultsRollback(t *testing.T) {
	for _, failure := range []string{"conflict", "ability-write"} {
		t.Run(failure, func(t *testing.T) {
			db := useChannelPersistenceTestDB(t)
			channel := &Channel{
				Type: channeltype.OpenAI, Name: "rollback", Status: ChannelStatusEnabled,
				Models: "Retired", Group: "default", TestingModel: resetTestString("Retired"),
			}
			if failure == "conflict" {
				channel.ModelMapping = resetTestString(`{"Retired":"Foo"}`)
			}
			require.NoError(t, channel.Insert())
			var before Channel
			require.NoError(t, db.First(&before, "id = ?", channel.Id).Error)
			var beforeAbilities []Ability
			require.NoError(t, db.Where("channel_id = ?", channel.Id).Find(&beforeAbilities).Error)
			if failure == "ability-write" {
				rejectAbilityInserts(t, db)
			}
			_, err := ResetChannelModelsToDefaults(context.Background(), channel.Id, resetTestCatalog)
			require.Error(t, err)
			var after Channel
			require.NoError(t, db.First(&after, "id = ?", channel.Id).Error)
			var afterAbilities []Ability
			require.NoError(t, db.Where("channel_id = ?", channel.Id).Find(&afterAbilities).Error)
			require.Equal(t, before, after)
			require.Equal(t, beforeAbilities, afterAbilities)
		})
	}
}

// TestResetChannelModelsToDefaultsClearsRemovedTestingModel verifies the SQL
// update persists a null testing model rather than GORM silently skipping nil.
func TestResetChannelModelsToDefaultsClearsRemovedTestingModel(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	channel := &Channel{Type: channeltype.OpenAI, Name: "old-test", Models: "Retired", Group: "default", TestingModel: resetTestString("Retired")}
	require.NoError(t, channel.Insert())
	_, err := ResetChannelModelsToDefaults(context.Background(), channel.Id, resetTestCatalog)
	require.NoError(t, err)
	var stored Channel
	require.NoError(t, db.First(&stored, "id = ?", channel.Id).Error)
	require.Nil(t, stored.TestingModel)
}

// TestResetChannelModelsToDefaultsCancellation verifies a canceled request does
// not apply changes and a nonexistent channel cannot be reported as reset.
func TestResetChannelModelsToDefaultsCancellation(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	channel := &Channel{Type: channeltype.OpenAI, Name: "canceled", Models: "Retired", Group: "default"}
	require.NoError(t, channel.Insert())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ResetChannelModelsToDefaults(ctx, channel.Id, resetTestCatalog)
	require.Error(t, err)
	var stored Channel
	require.NoError(t, db.First(&stored, "id = ?", channel.Id).Error)
	require.Equal(t, "Retired", stored.Models)
	_, err = ResetChannelModelsToDefaults(context.Background(), channel.Id+1000, resetTestCatalog)
	require.Error(t, err)
}
