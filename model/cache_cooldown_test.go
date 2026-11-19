package model

import (
	"context"
	"testing"
	"time"

	"github.com/Laisky/one-api/common/config"
	"github.com/stretchr/testify/require"
)

// TestCachedAbilityCooldown checks immediate suppression and automatic recovery through public cached selectors.
func TestCachedAbilityCooldown(t *testing.T) {
	for _, refreshed := range []bool{false, true} {
		name := "suspended_after_snapshot"
		if refreshed {
			name = "snapshot_during_suspension"
		}
		t.Run(name, func(t *testing.T) {
			oldDB, oldMemory := DB, config.MemoryCacheEnabled
			DB = setupTestDB(t)
			config.MemoryCacheEnabled = true
			channelSyncLock.Lock()
			oldGroups, oldChannels, oldSuspensions := group2model2channels, channelId2channel, cachedAbilitySuspensions
			channelSyncLock.Unlock()
			t.Cleanup(func() {
				DB = oldDB
				config.MemoryCacheEnabled = oldMemory
				channelSyncLock.Lock()
				group2model2channels, channelId2channel, cachedAbilitySuspensions = oldGroups, oldChannels, oldSuspensions
				channelSyncLock.Unlock()
			})
			high, low := int64(10), int64(5)
			for _, c := range []*Channel{
				{Id: 1, Name: "cooling", Status: ChannelStatusEnabled, Models: "target,unrelated", Group: "default,other", Priority: &high},
				{Id: 2, Name: "healthy", Status: ChannelStatusEnabled, Models: "target", Group: "default", Priority: &low},
			} {
				require.NoError(t, DB.Create(c).Error)
				require.NoError(t, c.AddAbilities())
			}
			require.NoError(t, InitChannelCacheContext(context.Background()))
			c, err := CacheGetRandomSatisfiedChannel("default", "target", false)
			require.NoError(t, err)
			require.Equal(t, 1, c.Id)
			cancelled, cancel := context.WithCancel(context.Background())
			cancel()
			require.Error(t, SuspendAbility(cancelled, "default", "target", 1, time.Hour))
			c, err = CacheGetRandomSatisfiedChannel("default", "target", false)
			require.NoError(t, err)
			require.Equal(t, 1, c.Id, "failed persistence must not publish a cache cooldown")
			require.NoError(t, SuspendAbility(context.Background(), "default", "target", 1, 300*time.Millisecond))
			if refreshed {
				require.NoError(t, InitChannelCacheContext(context.Background()))
			}
			for _, selectChannel := range []func() (*Channel, error){
				func() (*Channel, error) { return CacheGetRandomSatisfiedChannel("default", "target", false) },
				func() (*Channel, error) {
					return CacheGetRandomSatisfiedChannelExcluding("default", "target", false, map[int]bool{}, false)
				},
			} {
				c, err := selectChannel()
				require.NoError(t, err)
				require.Equal(t, 2, c.Id, "suspension must apply without a cache rebuild")
			}
			available, err := GetChannelsFromCache("default", "target")
			require.NoError(t, err)
			require.Len(t, available, 1)
			require.Equal(t, 2, available[0].Id)
			_, err = CacheGetRandomSatisfiedChannelExcluding("default", "target", false, map[int]bool{2: true}, false)
			require.Error(t, err, "excluded alternate must not permit the cooling channel")
			for _, pair := range [][2]string{{"other", "target"}, {"default", "unrelated"}} {
				c, err := CacheGetRandomSatisfiedChannel(pair[0], pair[1], false)
				require.NoError(t, err)
				require.Equal(t, 1, c.Id, "cooldown must remain scoped to exact ability")
			}
			require.Eventually(t, func() bool {
				c, err := CacheGetRandomSatisfiedChannel("default", "target", false)
				return err == nil && c.Id == 1
			}, 2*time.Second, 10*time.Millisecond, "expiry must restore cached routing without rebuild")
			c, err = CacheGetRandomSatisfiedChannelExcluding("default", "target", false, map[int]bool{1: true}, false)
			require.NoError(t, err)
			require.Equal(t, 2, c.Id, "failed-channel exclusions still win after expiry")
		})
	}
}
