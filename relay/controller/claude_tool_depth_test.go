package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// claudeToolDepthFixture creates a small accepted malformed tree with one text sibling per nested result.
func claudeToolDepthFixture(depth int) []any {
	blocks := []any{map[string]any{"type": "text", "text": "leaf"}}
	for range depth {
		blocks = []any{map[string]any{"type": "tool_result", "content": blocks}, map[string]any{"type": "text", "text": "sibling"}}
	}
	return blocks
}

// TestClaudeToolTokenProjectionBoundedDepth verifies small accepted trees preserve text without allocating one flattened subtree at every depth.
func TestClaudeToolTokenProjectionBoundedDepth(t *testing.T) {
	measure := func(depth int) float64 {
		blocks := claudeToolDepthFixture(depth)
		var parts []relaymodel.MessageContent
		allocations := testing.AllocsPerRun(3, func() { parts = claudeContentTokenParts(blocks) })
		require.Len(t, parts, depth+1)
		require.Equal(t, "leaf", *parts[0].Text)
		for _, part := range parts[1:] {
			require.Equal(t, "sibling", *part.Text)
		}
		return allocations
	}
	shallow, deep := measure(8), measure(32)
	t.Logf("BOUNDED_PROJECTION_ALLOCATIONS depth8=%.0f depth32=%.0f", shallow, deep)
	require.LessOrEqual(t, deep, shallow+(32-8)+8, "a shared accumulator should grow geometrically instead of flattening each nested subtree")
}
