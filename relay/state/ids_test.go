package state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGatewayIDEntropyAndShape verifies the ID scheme meets the configured
// random-bit minimum, has the OpenAI-compatible prefix, and does not collide
// across a large batch (SEC05).
func TestGatewayIDEntropyAndShape(t *testing.T) {
	t.Parallel()

	const n = 10000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id, err := NewResponseID()
		require.NoError(t, err)
		require.True(t, LooksLikeGatewayResponseID(id), "id %q should look like a gateway response id", id)
		require.Len(t, id, len(responseIDPrefix)+gatewayIDEntropyBytes*2)
		_, dup := seen[id]
		require.False(t, dup, "collision on %q", id)
		seen[id] = struct{}{}
	}
}

// TestGatewayIDPrefixesAreDistinct verifies response, conversation, and item IDs
// are distinguishable by shape while still matching the OpenAI wire prefixes.
func TestGatewayIDPrefixesAreDistinct(t *testing.T) {
	t.Parallel()

	resp, err := NewResponseID()
	require.NoError(t, err)
	conv, err := NewConversationID()
	require.NoError(t, err)
	item, err := NewItemID()
	require.NoError(t, err)

	require.True(t, LooksLikeGatewayResponseID(resp))
	require.False(t, LooksLikeGatewayResponseID(conv))
	require.True(t, LooksLikeGatewayConversationID(conv))
	require.False(t, LooksLikeGatewayConversationID(resp))
	require.Contains(t, item, itemIDPrefix)
}

// TestLegacySyntheticIDDetection verifies the hyphenated legacy fallback IDs are
// recognized so they can be rejected once the feature is enabled.
func TestLegacySyntheticIDDetection(t *testing.T) {
	t.Parallel()

	require.True(t, LooksLikeLegacySyntheticID("resp-abc123"))
	require.False(t, LooksLikeLegacySyntheticID("resp_abc123"))
	gid, err := NewResponseID()
	require.NoError(t, err)
	require.False(t, LooksLikeLegacySyntheticID(gid))
}
