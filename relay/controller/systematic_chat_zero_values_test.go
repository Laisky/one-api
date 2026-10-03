package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSystematicChatPassthroughPreservesExplicitZeroValues distinguishes actual
// converter deletions from unchanged scalar zero values omitted by JSON tags.
// Parameters: t controls assertions. Returns: none; explicit seed zero survives.
func TestSystematicChatPassthroughPreservesExplicitZeroValues(t *testing.T) {
	original := []byte(`{"model":"custom","seed":0,"stream":false,"max_tokens":0,"messages":[],"top_k":0}`)
	updated := []byte(`{"model":"custom"}`)
	got, _, _, err := mergeControlledPassthroughJSON(original, updated, true)
	require.NoError(t, err)
	// top_k is a pointer field: an explicitly supplied zero is non-nil and
	// would serialize. Its disappearance therefore represents conversion,
	// unlike the zero-valued scalar/slice fields retained below.
	require.JSONEq(t, `{"model":"custom","seed":0,"stream":false,"max_tokens":0,"messages":[]}`, string(got))
	again, _, changed, err := mergeControlledPassthroughJSON(got, got, true)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, got, again)
}
