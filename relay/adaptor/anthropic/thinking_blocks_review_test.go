package anthropic

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestInsertClaudeThinkingBlocksReturnsDetachedSlice verifies the merged
// content holds every rebuilt block and every replayed block exactly once, and
// that it never shares storage with the caller's slice, even when that slice
// has spare capacity a reuse would silently overwrite.
func TestInsertClaudeThinkingBlocksReturnsDetachedSlice(t *testing.T) {
	t.Parallel()
	contents := make([]Content, 0, 16)
	contents = append(contents, Content{Type: "text", Text: "answer"}, Content{Type: "tool_use", Id: "t1"})
	snapshot := append([]Content(nil), contents...)
	spare := contents[:cap(contents)]
	blocks := []model.ClaudeThinkingBlock{
		{Type: "thinking", Thinking: "plan", Signature: "sig1"},
		{Type: "redacted_thinking", Data: "opaque", AfterText: true},
	}

	out := insertClaudeThinkingBlocks(contents, blocks)
	require.Len(t, out, len(contents)+len(blocks))
	require.Equal(t, "thinking", out[0].Type)
	require.Equal(t, "text", out[1].Type)
	require.Equal(t, "redacted_thinking", out[2].Type)
	require.Equal(t, "tool_use", out[3].Type)

	require.Equal(t, snapshot, contents, "the caller's content must not change")
	require.Equal(t, Content{}, spare[2], "spare capacity of the caller's slice must stay untouched")
	out[1].Text = "mutated"
	require.Equal(t, "answer", contents[0].Text, "the result must not alias the caller's slice")
}
