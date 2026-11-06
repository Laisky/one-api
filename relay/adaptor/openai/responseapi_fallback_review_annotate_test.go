package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAnnotateResponseAssistantMessageShapes proves the web-search note is
// prefixed to string content, prepended as an output_text part to array
// content, and becomes the whole content otherwise, without mutating the
// caller's item or sharing its content array.
func TestAnnotateResponseAssistantMessageShapes(t *testing.T) {
	t.Parallel()
	const note = "[Web search: go release]"

	got := annotateResponseAssistantMessage(map[string]any{"type": "message", "role": "assistant", "content": "Go 1.27 is out."}, note)
	require.Equal(t, map[string]any{"type": "message", "role": "assistant", "content": note + "\nGo 1.27 is out."}, got)

	for name, content := range map[string]any{"missing": nil, "number": 3.0, "object": map[string]any{"text": "x"}} {
		item := map[string]any{"type": "message", "role": "assistant"}
		if content != nil {
			item["content"] = content
		}
		require.Equal(t, note, annotateResponseAssistantMessage(item, note)["content"], name)
	}

	parts := make([]any, 0, 8)
	parts = append(parts, map[string]any{"type": "output_text", "text": "one"}, map[string]any{"type": "output_text", "text": "two"})
	spare := parts[:cap(parts)]
	item := map[string]any{"type": "message", "role": "assistant", "content": parts}
	got = annotateResponseAssistantMessage(item, note)
	require.Equal(t, []any{
		map[string]any{"type": "output_text", "text": note},
		map[string]any{"type": "output_text", "text": "one"},
		map[string]any{"type": "output_text", "text": "two"},
	}, got["content"])
	require.Len(t, item["content"], 2, "the caller's item keeps its content")
	require.Nil(t, spare[2], "spare capacity of the caller's content must stay untouched")
	annotated := got["content"].([]any)
	annotated[1] = "mutated"
	require.Equal(t, map[string]any{"type": "output_text", "text": "one"}, parts[0], "the result must not alias the caller's content")
}
