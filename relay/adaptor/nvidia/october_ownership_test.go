package nvidia

import (
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberGLMOwnership checks independent constructor allocations using t;
// it returns nothing and never mutates package-level state.
func TestOctoberGLMOwnership(t *testing.T) {
	t.Parallel()
	first := currentGLMModels()
	second := currentGLMModels()
	cfg := first["z-ai/glm-5.3"]
	cfg.InputModalities[0] = "changed"
	cfg.SupportedReasoningEfforts[0] = "changed"
	require.Equal(t, "text", second["z-ai/glm-5.3"].InputModalities[0])
	require.Equal(t, "low", second["z-ai/glm-5.3"].SupportedReasoningEfforts[0])
	require.Equal(t, "text", first["z-ai/glm-5.3-flash"].InputModalities[0])
}
