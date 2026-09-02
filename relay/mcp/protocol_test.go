package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWithModernMetaCopiesParametersAndSupportsNil verifies metadata injection is allocation-safe and non-mutating.
//
// Parameters:
//   - t: The test owns map-copy and nil-input assertions.
//
// Return values: none; failures are reported through t.
func TestWithModernMetaCopiesParametersAndSupportsNil(t *testing.T) {
	input := map[string]any{"cursor": "page-2"}
	output := WithModernMeta(input)
	require.Equal(t, "page-2", output["cursor"])
	require.NotNil(t, output["_meta"])
	output["cursor"] = "changed"
	require.Equal(t, "page-2", input["cursor"])

	nilOutput := WithModernMeta(nil)
	require.Len(t, nilOutput, 1)
	require.NotNil(t, nilOutput["_meta"])
}
