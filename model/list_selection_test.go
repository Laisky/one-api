package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestListSelectionRejectsImplicitAll ensures empty, malformed, mixed and oversized selections fail closed.
func TestListSelectionRejectsImplicitAll(t *testing.T) {
	for _, selection := range []ListSelection{
		{}, {Mode: "ids"}, {Mode: "all"}, {Mode: "ids", IDs: []string{"1"}},
		{Mode: "ids", IDs: []string{"018fcf6d-c484-7000-8000-000000000101"}, ExcludedIDs: []string{"x"}},
		{Mode: "all_matching", IDs: []string{"x"}}, {Mode: "all_matching", ExcludedIDs: []string{"' OR 1=1 --"}},
		{Mode: "ids", IDs: make([]string, MaxListSelection+1)},
	} {
		_, err := selection.Normalize()
		require.Error(t, err)
	}
	id := "018fcf6d-c484-7000-8000-000000000101"
	normalized, err := (ListSelection{Mode: "ids", IDs: []string{id, strings.ToUpper(id)}}).Normalize()
	require.NoError(t, err)
	require.Equal(t, []string{id}, normalized.IDs)
}

// TestSelectedChannelSnapshotRespectsFilterAndExclusions verifies off-page identities and exact UUID search.
func TestSelectedChannelSnapshotRespectsFilterAndExclusions(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	channels := make([]Channel, 25)
	for i := range channels {
		channels[i] = Channel{Name: fmt.Sprintf("provider-%02d", i), Models: "Foo", Group: "default", Status: ChannelStatusEnabled}
		require.NoError(t, channels[i].Insert())
	}
	other := Channel{Name: "not-matching", Models: "Bar", Group: "default"}
	require.NoError(t, other.Insert())
	targets, err := SelectChannelTargets(context.Background(), ListSelection{Mode: "all_matching", ExcludedIDs: []string{channels[4].UUID}}, "provider-")
	require.NoError(t, err)
	require.Len(t, targets, 24)
	for _, target := range targets {
		require.NotEqual(t, channels[4].UUID, target.UUID)
		require.NotEqual(t, other.UUID, target.UUID)
	}
	targets, err = SelectChannelTargets(context.Background(), ListSelection{Mode: "ids", IDs: []string{channels[0].UUID, other.UUID}}, "provider-")
	require.NoError(t, err)
	require.Len(t, targets, 1)
	targets, err = SelectChannelTargets(context.Background(), ListSelection{Mode: "all_matching"}, channels[20].UUID)
	require.NoError(t, err)
	require.Equal(t, channels[20].UUID, targets[0].UUID)
	var count int64
	require.NoError(t, db.Model(&Channel{}).Count(&count).Error)
	require.EqualValues(t, 26, count, "resolution is read only")
}

// TestSelectedDisabledChannelDeletionRechecksStatusAndRollsBack preserves enabled channels and atomic routing state.
func TestSelectedDisabledChannelDeletionRechecksStatusAndRollsBack(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	channel := Channel{Name: "selected", Models: "Foo", Group: "default", Status: ChannelStatusEnabled}
	require.NoError(t, channel.Insert())
	deleted, err := DeleteSelectedDisabledChannel(context.Background(), channel.Id)
	require.NoError(t, err)
	require.False(t, deleted)
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("status", ChannelStatusManuallyDisabled).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_selected_delete BEFORE DELETE ON channels BEGIN SELECT RAISE(FAIL, 'forced'); END`).Error)
	deleted, err = DeleteSelectedDisabledChannel(context.Background(), channel.Id)
	require.Error(t, err)
	require.False(t, deleted)
	var abilities int64
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&abilities).Error)
	require.EqualValues(t, 1, abilities)
	require.NoError(t, db.Exec("DROP TRIGGER fail_selected_delete").Error)
	deleted, err = DeleteSelectedDisabledChannel(context.Background(), channel.Id)
	require.NoError(t, err)
	require.True(t, deleted)
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&abilities).Error)
	require.Zero(t, abilities)
}

// TestSelectedDisabledChannelMissingIsDistinct verifies a removed ID reports a
// missing-channel signal, not the enabled-channel skip or an operational failure.
func TestSelectedDisabledChannelMissingIsDistinct(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	channel := Channel{Name: "removed-selection", Models: "Foo", Group: "default", Status: ChannelStatusEnabled}
	require.NoError(t, channel.Insert())
	deleted, err := DeleteSelectedDisabledChannel(context.Background(), channel.Id)
	require.NoError(t, err, "enabled channels are a different skip")
	require.False(t, deleted)
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("status", ChannelStatusManuallyDisabled).Error)
	deleted, err = DeleteSelectedDisabledChannel(context.Background(), channel.Id)
	require.NoError(t, err)
	require.True(t, deleted)
	deleted, err = DeleteSelectedDisabledChannel(context.Background(), channel.Id)
	require.ErrorIs(t, err, ErrSelectedChannelMissing)
	require.False(t, deleted)
	var count int64
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
	require.Zero(t, count)
}
