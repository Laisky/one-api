package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestCalculateTestCostResolvesAdaptorByAPIType guards a type confusion that grows
// more dangerous with every new apitype: relay.GetAdaptor takes an API type, but
// calculateTestCost used to pass it a CHANNEL type. The two enums are independent
// and collide numerically -- channeltype.LingYiWanWu and apitype.Zai are both 31 --
// so the channel-test cost readout for LingYiWanWu would have been priced off
// Z.AI's GLM table the moment apitype.Zai was added.
func TestCalculateTestCostResolvesAdaptorByAPIType(t *testing.T) {
	t.Parallel()

	// The collision this test exists to catch.
	require.Equal(t, 31, channeltype.LingYiWanWu)
	require.Equal(t, 31, int(apitype.Zai))

	// The buggy form resolves the wrong provider entirely.
	wrong := relay.GetAdaptor(channeltype.LingYiWanWu)
	require.NotNil(t, wrong)
	require.Equal(t, "zai", wrong.GetChannelName(),
		"sanity: passing a channel type where an api type is expected picks up Z.AI")

	// The corrected form used by calculateTestCost.
	right := relay.GetAdaptor(channeltype.ToAPIType(channeltype.LingYiWanWu))
	require.NotNil(t, right)
	require.NotEqual(t, "zai", right.GetChannelName(),
		"LingYiWanWu must never be priced off the Z.AI table")

	// And the Zai channel itself still resolves to its own adaptor.
	zai := relay.GetAdaptor(channeltype.ToAPIType(channeltype.Zai))
	require.NotNil(t, zai)
	require.Equal(t, "zai", zai.GetChannelName())
}
