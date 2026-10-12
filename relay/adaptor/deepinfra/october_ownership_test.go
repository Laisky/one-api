package deepinfra

import (
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberDeepInfraOwnership checks independent mutable metadata in new
// constructors using t. It returns no values and does not mutate global defaults.
func TestOctoberDeepInfraOwnership(t *testing.T) {
	t.Parallel()
	first, second := octoberChatModels(), octoberChatModels()
	first["XiaomiMiMo/MiMo-V2.6-Pro"].InputModalities[0] = "changed"
	first["anthropic/claude-sonnet-5-5"].SupportedFeatures[0] = "changed"
	require.Equal(t, "text", second["XiaomiMiMo/MiMo-V2.6-Pro"].InputModalities[0])
	require.Equal(t, "text", first["XiaomiMiMo/MiMo-V2.6-Flash"].InputModalities[0])
	require.Equal(t, "tools", second["anthropic/claude-sonnet-5-5"].SupportedFeatures[0])
}
