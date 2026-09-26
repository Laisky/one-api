package channeltype

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/apitype"
)

// TestMuAPIRegistration verifies the stable channel id, base URL and native video capability.
func TestMuAPIRegistration(t *testing.T) {
	t.Parallel()

	require.Equal(t, 60, TypeSafe)
	require.Equal(t, 61, MuAPI)
	require.Greater(t, Dummy, MuAPI)
	require.Equal(t, apitype.MuAPI, ToAPIType(MuAPI))
	require.Equal(t, "muapi", IdToName(MuAPI))
	require.Equal(t, "https://api.muapi.ai", GetChannelBaseURLConfig(MuAPI).URL)
	require.True(t, GetChannelBaseURLConfig(MuAPI).Editable)
	require.Equal(t, []Endpoint{EndpointVideos}, DefaultEndpointsForChannelType(MuAPI))
}
