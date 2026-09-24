package model

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSetChannelStatusPreservesConfiguration checks enable/disable transitions,
// routing groups, unrelated channels, and repeated requests using an isolated DB.
// The test handle receives any persistence or field-preservation failure.
func TestSetChannelStatusPreservesConfiguration(t *testing.T) {
	for _, initial := range []int{ChannelStatusEnabled, ChannelStatusManuallyDisabled, ChannelStatusAutoDisabled} {
		t.Run(strconv.Itoa(initial), func(t *testing.T) {
			db := useChannelPersistenceTestDB(t)
			priority := int64(-3)
			channel := &Channel{
				Name: "configured", Key: "private-key", Status: initial, Models: "Foo,Bar",
				Group: "default,vip", Priority: &priority, Config: `{"custom":true}`,
				ModelMapping: resetTestString(`{"Foo":"Bar"}`), ModelConfigs: resetTestString(`{"Foo":{"ratio":2}}`),
				HiddenModels: resetTestString(`["Bar"]`), TestingModel: resetTestString(ChannelTestingModelSkip),
			}
			require.NoError(t, channel.Insert())
			other := &Channel{Name: "not-selected", Models: "Foo", Group: "default", Status: ChannelStatusEnabled}
			require.NoError(t, other.Insert())
			var before Channel
			require.NoError(t, db.First(&before, channel.Id).Error)
			var original []Ability
			require.NoError(t, db.Where("channel_id = ?", channel.Id).Order("model").Find(&original).Error)
			require.NotEmpty(t, original)
			for _, status := range []int{ChannelStatusEnabled, ChannelStatusEnabled, ChannelStatusManuallyDisabled, ChannelStatusManuallyDisabled} {
				require.NoError(t, SetChannelStatusWithContext(context.Background(), channel.Id, status))
				var after Channel
				require.NoError(t, db.First(&after, channel.Id).Error)
				require.Equal(t, status, after.Status)
				after.Status, after.UpdatedAt = before.Status, before.UpdatedAt
				require.Equal(t, before, after, "only status and update timestamp may change")
				var abilities []Ability
				require.NoError(t, db.Where("channel_id = ?", channel.Id).Order("model").Find(&abilities).Error)
				require.Len(t, abilities, len(original))
				for i := range abilities {
					require.Equal(t, status == ChannelStatusEnabled, abilities[i].Enabled)
					abilities[i].Enabled = original[i].Enabled
					abilities[i].UpdatedAt = original[i].UpdatedAt
				}
				require.Equal(t, original, abilities)
				var untouched Channel
				require.NoError(t, db.First(&untouched, other.Id).Error)
				require.Equal(t, ChannelStatusEnabled, untouched.Status)
			}
		})
	}
}

// TestSetChannelStatusRollback checks injected row and ability update failures
// leave both database tables unchanged and return errors to the test caller.
func TestSetChannelStatusRollback(t *testing.T) {
	for _, failure := range []string{"channel", "ability"} {
		t.Run(failure, func(t *testing.T) {
			db := useChannelPersistenceTestDB(t)
			channel := &Channel{Name: "rollback", Models: "Foo", Group: "default", Status: ChannelStatusEnabled}
			require.NoError(t, channel.Insert())
			var before Channel
			require.NoError(t, db.First(&before, channel.Id).Error)
			var oldAbilities []Ability
			require.NoError(t, db.Where("channel_id = ?", channel.Id).Find(&oldAbilities).Error)
			statement := `CREATE TRIGGER fail_status BEFORE UPDATE OF status ON channels WHEN NEW.status != OLD.status BEGIN SELECT RAISE(FAIL, 'injected failure'); END`
			if failure == "ability" {
				statement = `CREATE TRIGGER fail_status BEFORE UPDATE OF enabled ON abilities BEGIN SELECT RAISE(FAIL, 'injected failure'); END`
			}
			require.NoError(t, db.Exec(statement).Error)
			require.Error(t, SetChannelStatusWithContext(context.Background(), channel.Id, ChannelStatusManuallyDisabled))
			var after Channel
			require.NoError(t, db.First(&after, channel.Id).Error)
			var newAbilities []Ability
			require.NoError(t, db.Where("channel_id = ?", channel.Id).Find(&newAbilities).Error)
			require.Equal(t, before, after)
			require.Equal(t, oldAbilities, newAbilities)
		})
	}
}

// TestSetChannelStatusRejectsInvalidOrCanceledRequests checks invalid status,
// absent channels, and canceled contexts never report a successful state change.
func TestSetChannelStatusRejectsInvalidOrCanceledRequests(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	channel := &Channel{Name: "untouched", Models: "Foo", Group: "default", Status: ChannelStatusEnabled}
	require.NoError(t, channel.Insert())
	for _, status := range []int{-1, 0, 4} {
		require.Error(t, SetChannelStatusWithContext(context.Background(), channel.Id, status))
	}
	require.Error(t, SetChannelStatusWithContext(context.Background(), channel.Id+1000, ChannelStatusManuallyDisabled))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, SetChannelStatusWithContext(ctx, channel.Id, ChannelStatusManuallyDisabled))
	var stored Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	require.Equal(t, ChannelStatusEnabled, stored.Status)
}
