package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/idresolve"
)

// TestOptionalRefsTreatLegacyZeroAsUnset locks in the compatibility rule that
// pre-UUID clients (and the bundled admin CLI) may still send "0" to mean
// "no filter" on optional identifier query params.
func TestOptionalRefsTreatLegacyZeroAsUnset(t *testing.T) {
	t.Parallel()

	for _, ref := range []string{"", " ", "0", " 0 "} {
		require.True(t, isUnsetOptionalRef(ref), "ref %q should be unset", ref)

		id, err := resolveOptionalUserRef(ref)
		require.NoError(t, err, "user ref %q", ref)
		require.Zero(t, id)

		id, err = resolveOptionalChannelRef(ref)
		require.NoError(t, err, "channel ref %q", ref)
		require.Zero(t, id)

		id, err = resolveOptionalMCPServerRef(ref)
		require.NoError(t, err, "mcp server ref %q", ref)
		require.Zero(t, id)
	}

	// Any other integer-looking value is still a strict-in violation.
	for _, ref := range []string{"1", "42", "00"} {
		require.False(t, isUnsetOptionalRef(ref))
		_, err := resolveOptionalUserRef(ref)
		require.ErrorIs(t, err, idresolve.ErrInvalidRef, "user ref %q", ref)
		_, err = resolveOptionalChannelRef(ref)
		require.ErrorIs(t, err, idresolve.ErrInvalidRef, "channel ref %q", ref)
	}
}
