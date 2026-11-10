package anthropic

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaude52PolicyIdentity pins the exact identity boundary without broadening legacy matching.
func TestClaude52PolicyIdentity(t *testing.T) {
	for _, name := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		require.True(t, IsClaude52(name))
	}
	for _, name := range []string{"", "claude-fable-5-1", "claude-fable-5-2-fast", "claude-fable-5-2-latest", "anthropic.claude-fable-5-2", "CLAUDE-FABLE-5-2", " claude-fable-5-2", "opaque-deployment"} {
		require.False(t, IsClaude52(name))
	}
}
