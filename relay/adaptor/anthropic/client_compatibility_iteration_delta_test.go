package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClientCompatibilityIterationRootOnlyDeltas checks cumulative root counters
// after an iteration snapshot. Official root input/output sum non-compaction work;
// replacing that component must retain compaction without billing either twice.
// https://platform.claude.com/docs/en/build-with-claude/compaction-threshold#understanding-usage
func TestClientCompatibilityIterationRootOnlyDeltas(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, initial                     string
		compactionInput, compactionOutput int
	}{
		{name: "message_only", initial: `{"input_tokens":11,"output_tokens":1,"iterations":[{"type":"message","input_tokens":11,"output_tokens":1}]}`},
		{name: "compaction_and_message", initial: `{"input_tokens":11,"output_tokens":1,"iterations":[{"type":"compaction","input_tokens":144,"output_tokens":276},{"type":"message","input_tokens":11,"output_tokens":1}]}`, compactionInput: 144, compactionOutput: 276},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var receipt httpUsageReceipt
			require.NoError(t, receipt.apply(json.RawMessage(tc.initial)))
			for _, update := range []struct {
				raw           string
				input, output int
			}{
				{raw: `{"output_tokens":7}`, input: 11, output: 7},
				{raw: `{"output_tokens":7}`, input: 11, output: 7},
				{raw: `{"input_tokens":23}`, input: 23, output: 7},
				{raw: `{"input_tokens":23,"output_tokens":7}`, input: 23, output: 7},
				{raw: `{"service_tier":"standard"}`, input: 23, output: 7},
			} {
				require.NoError(t, receipt.apply(json.RawMessage(update.raw)))
				usage := receipt.snapshot()
				require.Equal(t, tc.compactionInput+update.input, usage.PromptTokens, update.raw)
				require.Equal(t, tc.compactionOutput+update.output, usage.CompletionTokens, update.raw)
				require.Equal(t, tc.compactionInput+update.input+tc.compactionOutput+update.output, usage.TotalTokens, update.raw)
			}
		})
	}
}

// TestClientCompatibilityIterationPartialSnapshotThenRootDelta verifies missing
// fields preserve earlier indexed counts before a later non-compaction root update.
func TestClientCompatibilityIterationPartialSnapshotThenRootDelta(t *testing.T) {
	t.Parallel()
	var receipt httpUsageReceipt
	require.NoError(t, receipt.apply(json.RawMessage(`{"input_tokens":11,"output_tokens":1,"iterations":[{"type":"compaction","input_tokens":144,"output_tokens":276},{"type":"message","input_tokens":11,"output_tokens":1}]}`)))
	require.NoError(t, receipt.apply(json.RawMessage(`{"output_tokens":7,"iterations":[{"type":"compaction"},{"type":"message","output_tokens":7}]}`)))
	require.Equal(t, 155, receipt.snapshot().PromptTokens)
	require.Equal(t, 283, receipt.snapshot().CompletionTokens)
	require.NoError(t, receipt.apply(json.RawMessage(`{"output_tokens":9}`)))
	require.Equal(t, 155, receipt.snapshot().PromptTokens)
	require.Equal(t, 285, receipt.snapshot().CompletionTokens)
	before := receipt.snapshot()
	require.Error(t, receipt.apply(json.RawMessage(`{"output_tokens":-1}`)))
	require.Equal(t, before, receipt.snapshot(), "invalid root update must be atomic")
}
