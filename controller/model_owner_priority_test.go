package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/dto"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestEnabledAbilityBeatsRanksByPriorityThenChannelID pins the ordering rule used
// everywhere a model served by several channels must be attributed to one of
// them: highest priority first, lowest channel id as the tie-break.
func TestEnabledAbilityBeatsRanksByPriorityThenChannelID(t *testing.T) {
	t.Parallel()

	high := dto.EnabledAbility{Model: "m", ChannelId: 9, Priority: 10}
	low := dto.EnabledAbility{Model: "m", ChannelId: 1, Priority: 0}

	require.True(t, high.Beats(low), "higher priority wins even with a larger channel id")
	require.False(t, low.Beats(high))

	// Equal priority falls back to the lower channel id.
	a := dto.EnabledAbility{Model: "m", ChannelId: 3, Priority: 5}
	b := dto.EnabledAbility{Model: "m", ChannelId: 7, Priority: 5}
	require.True(t, a.Beats(b))
	require.False(t, b.Beats(a))

	// Negative priorities still order correctly (GetPriority coerces nil to 0).
	neg := dto.EnabledAbility{Model: "m", ChannelId: 1, Priority: -5}
	zero := dto.EnabledAbility{Model: "m", ChannelId: 99, Priority: 0}
	require.True(t, zero.Beats(neg))
}

// TestBestAbilityPerModelIsOrderIndependent pins that the winner does not depend
// on the order rows come back from the database.
func TestBestAbilityPerModelIsOrderIndependent(t *testing.T) {
	t.Parallel()

	forward := []dto.EnabledAbility{
		{Model: "glm-4.7", ChannelId: 1, ChannelType: channeltype.Zhipu, Priority: 0},
		{Model: "glm-4.7", ChannelId: 9, ChannelType: channeltype.Zai, Priority: 10},
		{Model: "glm-4.7", ChannelId: 4, ChannelType: channeltype.Zhipu, Priority: 10},
	}
	reversed := []dto.EnabledAbility{forward[2], forward[1], forward[0]}

	for name, input := range map[string][]dto.EnabledAbility{"forward": forward, "reversed": reversed} {
		t.Run(name, func(t *testing.T) {
			best := bestAbilityPerModel(input)
			require.Len(t, best, 1)
			// Top priority tier is {9, 4}; the lower channel id wins.
			require.Equal(t, 4, best["glm-4.7"].ChannelId)
			require.Equal(t, channeltype.Zhipu, best["glm-4.7"].ChannelType)
		})
	}
}

// TestResolveUserAvailableModelsOwnerFollowsChannelPriority is the end-to-end
// guard for the Zhipu/Z.ai split. Both brands advertise glm-4.7 and both ship it
// in the compiled-in catalog, so the snapshot's owner is arbitrary. The listing
// must instead name the channel this deployment would actually route to.
func TestResolveUserAvailableModelsOwnerFollowsChannelPriority(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		abilities []dto.EnabledAbility
		want      string
	}{
		"zai has higher priority": {
			abilities: []dto.EnabledAbility{
				{Model: "glm-4.7", ChannelId: 1, ChannelType: channeltype.Zhipu, Priority: 0},
				{Model: "glm-4.7", ChannelId: 2, ChannelType: channeltype.Zai, Priority: 10},
			},
			want: "zai",
		},
		"zhipu has higher priority": {
			abilities: []dto.EnabledAbility{
				{Model: "glm-4.7", ChannelId: 1, ChannelType: channeltype.Zhipu, Priority: 10},
				{Model: "glm-4.7", ChannelId: 2, ChannelType: channeltype.Zai, Priority: 0},
			},
			want: "zhipu",
		},
		"equal priority falls back to the lower channel id": {
			abilities: []dto.EnabledAbility{
				{Model: "glm-4.7", ChannelId: 8, ChannelType: channeltype.Zai, Priority: 5},
				{Model: "glm-4.7", ChannelId: 3, ChannelType: channeltype.Zhipu, Priority: 5},
			},
			want: "zhipu",
		},
		"only zhipu configured": {
			abilities: []dto.EnabledAbility{
				{Model: "glm-4.7", ChannelId: 1, ChannelType: channeltype.Zhipu, Priority: 0},
			},
			want: "zhipu",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := resolveUserAvailableModels(tc.abilities, map[int]*model.Channel{}, nil)
			require.Len(t, got, 1)
			require.Equal(t, "glm-4.7", got[0].Id)
			require.Equal(t, "glm-4.7", got[0].Root)
			require.Equal(t, tc.want, got[0].OwnedBy,
				"owned_by must name the channel that would serve the model")
			require.Equal(t, modelCatalogCreated, got[0].Created,
				"created must be the frozen constant, never a rebuild timestamp")
		})
	}
}

// TestAbilityOwnerFromCacheDoesNoDatabaseWork pins that the listing hot path
// stays I/O-free: it runs once per advertised model, so a per-model channel
// lookup would turn /v1/models into an N+1. model.DB is nil here, so any query
// would panic.
func TestAbilityOwnerFromCacheDoesNoDatabaseWork(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		owner := abilityOwnerFromCache(4242, channeltype.Zai, map[int]*model.Channel{})
		require.Equal(t, "zai", owner)
	})

	// A cached channel overrides the ability's denormalized type.
	cache := map[int]*model.Channel{7: {Id: 7, Type: channeltype.Zhipu}}
	require.Equal(t, "zhipu", abilityOwnerFromCache(7, channeltype.Zai, cache))

	// An unnameable type degrades to a label that still identifies the channel.
	require.Equal(t, "channel-11", abilityOwnerFromCache(11, 9999, map[int]*model.Channel{}))
}

// TestListAndRetrieveAgreeOnOwner pins that GET /v1/models and
// GET /v1/models/:model attribute the same model to the same channel. They reach
// the answer by different routes -- bestAbilityPerModel for the list,
// matchVisibleAbilityByModelID for the single lookup -- so the rankings must stay
// in sync or the two endpoints contradict each other.
func TestListAndRetrieveAgreeOnOwner(t *testing.T) {
	t.Parallel()

	abilities := []dto.EnabledAbility{
		{Model: "glm-4.7", ChannelId: 2, ChannelType: channeltype.Zai, Priority: 0},
		{Model: "glm-4.7", ChannelId: 5, ChannelType: channeltype.Zhipu, Priority: 7},
		{Model: "glm-4.7", ChannelId: 1, ChannelType: channeltype.Zai, Priority: 7},
	}
	listed := resolveUserAvailableModels(abilities, map[int]*model.Channel{}, nil)
	require.Len(t, listed, 1)

	matched, ok := matchVisibleAbilityByModelID(abilities, "glm-4.7")
	require.True(t, ok)
	retrieved, ok := buildModelEntryFromAbility(matched.Model, matched.ChannelId, matched.ChannelType, map[int]*model.Channel{})
	require.True(t, ok)

	// Top tier is priority 7 = {channel 5 zhipu, channel 1 zai}; lowest id wins.
	require.Equal(t, "zai", listed[0].OwnedBy)
	require.Equal(t, 1, matched.ChannelId)

	// The two endpoints must be byte-identical, not merely agree on the owner:
	// RetrieveModel now renders the same struct from the same ability.
	require.Equal(t, listed[0], retrieved,
		"GET /v1/models and GET /v1/models/:model must return the identical entry")
}

// TestLoadChannelCachedNegativelyCachesMisses pins that an orphaned ability (a
// channel row deleted without its abilities -- Channel.Delete is not
// transactional) costs at most one lookup, not one per model it used to serve.
func TestLoadChannelCachedNegativelyCachesMisses(t *testing.T) {
	t.Parallel()

	// A nil entry is the memoized miss. model.DB is nil in this package's unit
	// tests, so a second lookup attempt would panic instead of returning an
	// error; not panicking proves the cached miss short-circuits.
	cache := map[int]*model.Channel{9999: nil}
	require.NotPanics(t, func() {
		got, err := loadChannelCached(9999, cache)
		require.Error(t, err)
		require.Nil(t, got)
	})

	// A negatively cached channel still yields a usable owner label from the
	// ability's own channel_type, which GetGroupModelsV2 read via its JOIN.
	require.Equal(t, "zai", abilityOwnerFromCache(9999, channeltype.Zai, cache))
}
