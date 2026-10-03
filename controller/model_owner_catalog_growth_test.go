package controller

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestSharedGLMFallbackProviderPermutations verifies the native-provider and
// reseller fallback rankings are independent of provider enumeration, using t.
func TestSharedGLMFallbackProviderPermutations(t *testing.T) {
	t.Parallel()
	for _, owners := range [][]string{
		{"zhipu", "zai"}, {"zai", "zhipu"},
		{"ali", "zai", "zhipu"}, {"ali", "zhipu", "zai"},
		{"zai", "ali", "zhipu"}, {"zai", "zhipu", "ali"},
		{"zhipu", "ali", "zai"}, {"zhipu", "zai", "ali"},
	} {
		rows := make([]OpenAIModels, 0, len(owners))
		for _, owner := range owners {
			rows = append(rows, OpenAIModels{Id: "glm-4.7", Root: "glm-4.7", OwnedBy: owner})
		}
		got := dedupeStaticModelsByOwner(rows)
		require.Len(t, got, 1)
		want := "ali"
		if len(owners) == 2 {
			want = "zai"
		}
		require.Equal(t, want, got[0].OwnedBy, "provider order: %v", owners)
		require.Equal(t, "glm-4.7", got[0].Root)
	}
}

// TestListModelsSharedGLMWithResellerFollowsPriority exercises the real listing
// handler with all three providers. It uses t and verifies the static fallback
// cannot override the configured priority or channel-ID tie-break.
func TestListModelsSharedGLMWithResellerFollowsPriority(t *testing.T) {
	for _, tc := range []struct {
		name            string
		ali, zai, zhipu int64
		want            string
	}{
		{"reseller priority", 20, 10, 0, "ali"},
		{"zai priority", 0, 20, 10, "zai"},
		{"zhipu priority", 0, 10, 20, "zhipu"},
		{"channel ID beats alphabetic owner", 5, 5, 5, "zhipu"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(setupUserAvailableModelsTestEnvironment(t))
			const id = "glm-4.7"
			group := fmt.Sprintf("reseller-glm-%d", time.Now().UnixNano())
			for _, ch := range []struct {
				id, kind int
				priority int64
			}{
				{5401, channeltype.Zhipu, tc.zhipu},
				{5402, channeltype.Zai, tc.zai},
				{5403, channeltype.Ali, tc.ali},
			} {
				require.NoError(t, model.DB.Create(&model.Channel{
					Id: ch.id, Name: fmt.Sprintf("reseller-channel-%d", ch.id),
					Status: model.ChannelStatusEnabled, Type: ch.kind, Models: id, Group: group,
				}).Error)
				require.NoError(t, model.DB.Create(&model.Ability{
					Group: group, Model: id, ChannelId: ch.id,
					Enabled: true, Priority: ptrInt64(ch.priority),
				}).Error)
			}
			rows := listModelsForGroup(t, group)
			require.Len(t, rows, 1, "shared model must appear once, not once per provider")
			require.Equal(t, id, rows[0].Id)
			require.Equal(t, id, rows[0].Root)
			require.Equal(t, tc.want, rows[0].OwnedBy)
		})
	}
}
