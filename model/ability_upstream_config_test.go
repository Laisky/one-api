package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestChannelConfigEditLiftsOnlyForUpstreamFields verifies that saving a channel
// whose config changes only gateway-side settings (tooling, MCP tool blacklist,
// supported endpoints) keeps the abilities' cooldown streak and upstream
// revision, while a change to an upstream-identifying config field (region,
// credentials, endpoint URLs) restarts the streak and advances the revision.
func TestChannelConfigEditLiftsOnlyForUpstreamFields(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config string
		lifts  bool
	}{
		{name: "tooling_only", config: `{"region":"us-east-1","tooling":{"whitelist":["web_search"]}}`},
		{name: "mcp_blacklist_only", config: `{"region":"us-east-1","mcp_tool_blacklist":["x"]}`},
		{name: "supported_endpoints_only", config: `{"region":"us-east-1","supported_endpoints":["chat_completions"]}`},
		{name: "reordered_keys_only", config: `{ "region" : "us-east-1" }`},
		{name: "region_changes", config: `{"region":"eu-west-1"}`, lifts: true},
		{name: "endpoint_url_changes", config: `{"region":"us-east-1","endpoint_urls":{"chat_completions":"https://example.invalid/v1"}}`, lifts: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := useChannelPersistenceTestDB(t)
			ctx := context.Background()
			channel := &Channel{Name: "config", Key: "k", Status: ChannelStatusEnabled, Models: "route", Group: "test", Config: `{"region":"us-east-1"}`}
			require.NoError(t, channel.Insert())
			key := cachedAbilityKey{"test", "route", channel.Id}
			var row Ability
			require.NoError(t, exactCooldownAbility(db, key).Take(&row).Error)
			at := time.Unix(0, row.CooldownConfigChangedAt).UTC().Add(time.Second)
			_, err := RecordAbilityCooldownFailure(ctx, key.group, key.model, key.channelID, at, at)
			require.NoError(t, err)

			edited, err := GetChannelById(channel.Id, true)
			require.NoError(t, err)
			edited.TestingModel = nil
			edited.Config = tc.config
			require.NoError(t, edited.Update())

			reloaded, err := GetChannelById(channel.Id, true)
			require.NoError(t, err)
			require.NoError(t, exactCooldownAbility(db, key).Take(&row).Error)
			if tc.lifts {
				require.EqualValues(t, 1, reloaded.UpstreamRevision)
				require.Zero(t, row.CooldownFailures)
				return
			}
			require.Zero(t, reloaded.UpstreamRevision, "a gateway-side config edit is not an upstream change")
			require.Equal(t, 1, row.CooldownFailures, "a gateway-side config edit keeps the streak")
		})
	}
}
