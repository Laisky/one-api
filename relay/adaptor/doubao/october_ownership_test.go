package doubao

import (
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberArkOwnership verifies independent constructor and sibling slices
// using t. It returns no values and does not mutate global catalogue state.
func TestOctoberArkOwnership(t *testing.T) {
	t.Parallel()
	a := currentArkModels()
	b := currentArkModels()
	cfg := a["doubao-seed-evolving"]
	cfg.InputModalities[0] = "changed"
	cfg.SupportedFeatures[0] = "changed"
	require.Equal(t, "text", b["doubao-seed-evolving"].InputModalities[0])
	require.Equal(t, "tools", a["doubao-seed-2-1-pro-260915"].SupportedFeatures[0])
}
