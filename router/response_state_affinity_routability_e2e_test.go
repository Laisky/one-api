package router

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestResponseStateAffinityRequiresRoutableAbilityPublicRouter is the end-to-end
// regression for the PR #627 review finding that provider affinity checked only
// the cooldown of the bound ability. A channel that is enabled and still lists
// the model, but whose exact (group, model, channel) ability is missing (the
// model was hidden on the channel, or the row was removed) or disabled, is not
// routable: normal selection never picks it, so a stored response bound to it
// must fall back to normal selection and replay the prior turn canonically
// instead of being pinned to that channel. Every case runs on both the
// database and the memory-cache routing paths.
func TestResponseStateAffinityRequiresRoutableAbilityPublicRouter(t *testing.T) {
	groupColumn := "`group`"
	for _, tc := range []struct {
		name         string
		breakAbility func(t *testing.T, channelID int)
	}{
		{
			// The reviewer's case: an admin hides the model on the bound channel
			// through the real channel update path. buildChannelAbilities omits a
			// hidden model, so its ability row is removed, yet the channel still
			// lists the model and Channel.SupportsModel still accepts it.
			name: "model hidden after binding",
			breakAbility: func(t *testing.T, channelID int) {
				var channel model.Channel
				require.NoError(t, model.DB.Where("id = ?", channelID).Take(&channel).Error)
				hidden := `["` + passthroughKeysAllowedModel + `"]`
				channel.HiddenModels = &hidden
				channel.HiddenModelsProvided = true
				channel.TestingModel = nil
				require.NoError(t, channel.UpdateWithContext(context.Background()))
				var rows int64
				require.NoError(t, model.DB.Model(&model.Ability{}).
					Where(groupColumn+" = ? AND model = ? AND channel_id = ?", "default", passthroughKeysAllowedModel, channelID).
					Count(&rows).Error)
				require.Zero(t, rows, "hiding the model must remove its ability from the routing projection")
			},
		},
		{
			name: "missing ability row",
			breakAbility: func(t *testing.T, channelID int) {
				require.NoError(t, model.DB.Where(groupColumn+" = ? AND model = ? AND channel_id = ?",
					"default", passthroughKeysAllowedModel, channelID).Delete(&model.Ability{}).Error)
			},
		},
		{
			name: "disabled ability row",
			breakAbility: func(t *testing.T, channelID int) {
				require.NoError(t, model.DB.Model(&model.Ability{}).
					Where(groupColumn+" = ? AND model = ? AND channel_id = ?", "default", passthroughKeysAllowedModel, channelID).
					Update("enabled", false).Error)
			},
		},
	} {
		for _, memory := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cache=%t", tc.name, memory), func(t *testing.T) {
				h := newAffinityHarness(t, memory)

				// Turn 1: only channel A exists; the stored response R is bound to A.
				first, callsA, callsB := h.turn(t, `{"model":"`+passthroughKeysAllowedModel+`","stream":false,"store":true,"input":"`+affinityPriorUserText+`"}`)
				require.Equal(t, http.StatusOK, first.status, first.body)
				require.Equal(t, 1, callsA, "turn 1 must be served by channel A")
				require.Zero(t, callsB)

				h.addAlternateChannel(t)

				// Control: A is still routable, so the continuation is pinned to A even
				// though normal selection prefers the higher-priority channel B.
				control, callsA, callsB := h.turn(t, continuation(first.id, "control follow-up"))
				require.Equal(t, http.StatusOK, control.status, control.body)
				require.Equal(t, 1, callsA, "a routable binding must pin the continuation to channel A")
				require.Zero(t, callsB)

				// Break A's exact ability while A stays enabled and keeps listing the model.
				tc.breakAbility(t, h.channelA)
				h.refreshRoutingCache(t)
				var channelA model.Channel
				require.NoError(t, model.DB.Where("id = ?", h.channelA).Take(&channelA).Error)
				require.Equal(t, model.ChannelStatusEnabled, channelA.Status, "the channel itself must stay enabled")
				require.True(t, channelA.SupportsModel(passthroughKeysAllowedModel), "the channel must still list the model")

				const followUp = "unroutable follow-up 6131"
				unpinned, callsA, callsB := h.turn(t, continuation(first.id, followUp))
				require.Equal(t, http.StatusOK, unpinned.status, unpinned.body)
				require.Zero(t, callsA, "a binding to a channel without a routable ability must not pin it")
				require.Equal(t, 1, callsB, "the unpinned continuation must be served by normal selection")
				require.Equal(t, affinityAlternateAssistant, unpinned.text, unpinned.body)
				requireReplayedPriorTurn(t, h.upB.lastChatRequest(t), followUp)
			})
		}
	}
}
