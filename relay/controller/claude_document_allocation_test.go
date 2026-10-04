package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestClaudeDocumentEmptyProjectionAllocation verifies that requests containing
// no document-bearing blocks do not allocate a message-count-sized work stack.
// This is an allocation regression, not a claimed remotely reachable overflow.
func TestClaudeDocumentEmptyProjectionAllocation(t *testing.T) {
	for _, emptyBlocks := range []bool{false, true} {
		name := "text"
		if emptyBlocks {
			name = "empty_blocks"
		}
		t.Run(name, func(t *testing.T) {
			request := &ClaudeMessagesRequest{Messages: make([]relaymodel.ClaudeMessage, 2048)}
			for i := range request.Messages {
				request.Messages[i] = relaymodel.ClaudeMessage{Role: "user", Content: "Synthetic text-only history."}
				if emptyBlocks {
					request.Messages[i].Content = []any{}
				}
			}
			var got int
			var scanErr error
			allocations := testing.AllocsPerRun(5, func() { got, scanErr = countClaudeNativeDocumentAllowance(request) })
			require.NoError(t, scanErr)
			require.Zero(t, got)
			t.Logf("DOCUMENT_EMPTY_SCAN empty_blocks=%v messages=%d allocations=%g", emptyBlocks, len(request.Messages), allocations)
			require.Zero(t, allocations, "empty/text-only histories must not allocate a traversal stack")
		})
	}
}

// TestClaudeDocumentNestedAllowanceTraversal retains additive document counting
// through large flat histories and deep tool results without recursive calls.
func TestClaudeDocumentNestedAllowanceTraversal(t *testing.T) {
	const count = 2048
	document := map[string]any{"type": "document", "source": map[string]any{"type": "file", "file_id": "synthetic-document"}}
	blocks := make([]any, count)
	for i := range blocks {
		blocks[i] = document
	}
	request := &ClaudeMessagesRequest{Messages: []relaymodel.ClaudeMessage{{Role: "user", Content: blocks}}}
	require.Equal(t, count*config.ClaudeNativeDocumentTokenAllowance, requireClaudeDocumentTokens(t, request))
	var nested any = document
	for i := 0; i < count; i++ {
		nested = map[string]any{"type": "tool_result", "tool_use_id": "synthetic-call", "content": []any{nested}}
	}
	request.Messages[0].Content = []any{nested}
	require.Equal(t, config.ClaudeNativeDocumentTokenAllowance, requireClaudeDocumentTokens(t, request))
	request.System = []any{document}
	require.Equal(t, 2*config.ClaudeNativeDocumentTokenAllowance, requireClaudeDocumentTokens(t, request))
	require.Zero(t, requireClaudeDocumentTokens(t, nil))
}
