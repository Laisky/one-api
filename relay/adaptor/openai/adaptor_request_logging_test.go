package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCollectResponseAPIInputDiagnostics verifies malformed tool history diagnostics
// are captured without logging request payload contents.
// Parameters: t is the test context.
// Returns: no return value.
func TestCollectResponseAPIInputDiagnostics(t *testing.T) {
	diag := collectResponseAPIInputDiagnostics(ResponseAPIInput{
		map[string]any{
			"type":    "function_call_output",
			"call_id": "call_ok",
			"output":  "ok",
		},
		map[string]any{
			"type":    "function_call_output",
			"call_id": "call_missing_output",
		},
		map[string]any{
			"type":   "function_call_output",
			"output": "exists",
		},
		map[string]any{
			"type": "function_call",
		},
		123,
	})

	require.Equal(t, 1, diag.FunctionCallOutputMissingOutput)
	require.Equal(t, 1, diag.FunctionCallOutputMissingCallID)
	require.Equal(t, 1, diag.FunctionCallMissingName)
	require.Equal(t, 1, diag.FunctionCallMissingArguments)
	require.Equal(t, 1, diag.InvalidInputItemType)
	require.Equal(t, []int{1}, diag.MissingOutputSampleIndices)
}

// TestCollectResponseAPIInputDiagnostics_IgnoresStringItems verifies that string inputs
// are treated as valid Response API items and do not increment invalid counters.
// Parameters: t is the test context.
// Returns: no return value.
func TestCollectResponseAPIInputDiagnostics_IgnoresStringItems(t *testing.T) {
	diag := collectResponseAPIInputDiagnostics(ResponseAPIInput{"hello", "world"})

	require.Zero(t, diag.FunctionCallOutputMissingOutput)
	require.Zero(t, diag.FunctionCallOutputMissingCallID)
	require.Zero(t, diag.FunctionCallMissingName)
	require.Zero(t, diag.FunctionCallMissingArguments)
	require.Zero(t, diag.InvalidInputItemType)
	require.Empty(t, diag.MissingOutputSampleIndices)
}
