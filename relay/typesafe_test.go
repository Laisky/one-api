package relay

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/typesafe"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestTypeSafeRegistration checks the factory, append-only IDs and native routing.
func TestTypeSafeRegistration(t *testing.T) {
	require.Equal(t, 59, channeltype.Jina)
	require.Equal(t, 60, channeltype.TypeSafe)
	require.Equal(t, apitype.TypeSafe, channeltype.ToAPIType(channeltype.TypeSafe))
	require.IsType(t, &typesafe.Adaptor{}, GetAdaptor(apitype.TypeSafe))
	require.Equal(t, "typesafe", channeltype.IdToName(channeltype.TypeSafe))
	require.Equal(t, "typesafe", apitype.String(apitype.TypeSafe))
	require.Len(t, channeltype.ChannelBaseURLConfigs, channeltype.Dummy)
	require.Equal(t, typesafe.DefaultBaseURL, channeltype.ChannelBaseURLs[channeltype.TypeSafe])
	require.Equal(t, relaymode.SystemOne, relaymode.GetByPath("/v1/systemone"))
	require.Equal(t, relaymode.Unknown, relaymode.GetByPath("/v1/systemone-unknown"))
	require.Equal(t, "systemone", relaymode.String(relaymode.SystemOne))
	require.Equal(t, []string{"systemone"}, channeltype.DefaultEndpointNamesForChannelType(channeltype.TypeSafe))
	require.Equal(t, "systemone", channeltype.RelayModeToEndpointName(relaymode.SystemOne))
	require.NotContains(t, channeltype.DefaultEndpointsForChannelType(channeltype.OpenAI), channeltype.EndpointSystemOne)
}
