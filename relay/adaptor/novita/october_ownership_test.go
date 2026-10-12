package novita

import (
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberNovitaOwnership proves aliases and constructor calls own their
// mutable metadata independently. It accepts t and returns no values.
func TestOctoberNovitaOwnership(t *testing.T) {
	t.Parallel()
	first, second := octoberChatModels(), octoberChatModels()
	first["deepseek/deepseek-v4.1-flash-p"].InputModalities[0] = "changed"
	first["xiaomimimo/mimo-v2.6-pro"].SupportedFeatures[0] = "changed"
	require.Equal(t, "text", second["deepseek/deepseek-v4.1-flash-p"].InputModalities[0])
	require.Equal(t, "text", first["deepseek/deepseek-v4.1-flash-dst"].InputModalities[0])
	require.Equal(t, "tools", second["xiaomimimo/mimo-v2.6-pro"].SupportedFeatures[0])
}
