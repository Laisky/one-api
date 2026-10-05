package channeltype

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/relaymode"
)

// TestMuAPINativeVideoCapabilityIsIsolated verifies native capability never
// claims legacy wire compatibility. The synchronous bridge is explicit.
func TestMuAPINativeVideoCapabilityIsIsolated(t *testing.T) {
	endpoints := DefaultEndpointsForChannelType(MuAPI)
	require.False(t, IsEndpointSupported(relaymode.Videos, endpoints), "native submit/poll must not advertise legacy EndpointVideos")
	require.Contains(t, endpoints, MuAsyncEndpointVideos, "native capability is explicit")
	require.True(t, IsEndpointSupported(relaymode.Videos, DefaultEndpointsForChannelType(OpenAI)), "legacy capability is preserved")
}
