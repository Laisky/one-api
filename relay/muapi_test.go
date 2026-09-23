package relay

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/muapi"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestMuAPIRegistration checks the append-only provider registration and native video routing.
func TestMuAPIRegistration(t *testing.T) {
	t.Parallel()

	require.Equal(t, 60, channeltype.TypeSafe)
	require.Equal(t, 61, channeltype.MuAPI)
	require.Equal(t, apitype.MuAPI, channeltype.ToAPIType(channeltype.MuAPI))
	require.IsType(t, &muapi.Adaptor{}, GetAdaptor(apitype.MuAPI))
	require.Equal(t, "muapi", channeltype.IdToName(channeltype.MuAPI))
	require.Equal(t, "muapi", apitype.String(apitype.MuAPI))
	require.Len(t, channeltype.ChannelBaseURLConfigs, channeltype.Dummy)
	require.Equal(t, "https://api.muapi.ai", channeltype.ChannelBaseURLs[channeltype.MuAPI])
	require.Equal(t, []string{"videos"}, channeltype.DefaultEndpointNamesForChannelType(channeltype.MuAPI))
	require.Equal(t, relaymode.Videos, relaymode.GetByPath("/v1/videos"))
	require.NotContains(t, channeltype.DefaultEndpointsForChannelType(channeltype.MuAPI), channeltype.EndpointImagesGenerations)
}
