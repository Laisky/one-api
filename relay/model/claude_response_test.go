package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaudeContentMarshalTypeAware pins the per-type wire shape of content
// blocks: required fields survive even when empty, and unrelated fields of
// other block types never leak in.
func TestClaudeContentMarshalTypeAware(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		block ClaudeContent
		want  string
	}{
		"empty_text_keeps_text":         {ClaudeContent{Type: "text"}, `{"type":"text","text":""}`},
		"thinking_has_empty_signature":  {ClaudeContent{Type: "thinking", Thinking: "hmm"}, `{"type":"thinking","thinking":"hmm","signature":""}`},
		"thinking_keeps_signature":      {ClaudeContent{Type: "thinking", Thinking: "hmm", Signature: "sig"}, `{"type":"thinking","thinking":"hmm","signature":"sig"}`},
		"tool_use_without_input":        {ClaudeContent{Type: "tool_use", ID: "toolu_1", Name: "f"}, `{"type":"tool_use","id":"toolu_1","name":"f","input":{}}`},
		"tool_use_invalid_input":        {ClaudeContent{Type: "tool_use", ID: "toolu_1", Name: "f", Input: json.RawMessage(`{"a":`)}, `{"type":"tool_use","id":"toolu_1","name":"f","input":{}}`},
		"tool_use_null_input":           {ClaudeContent{Type: "tool_use", ID: "toolu_1", Name: "f", Input: json.RawMessage(`null`)}, `{"type":"tool_use","id":"toolu_1","name":"f","input":{}}`},
		"tool_use_keeps_input":          {ClaudeContent{Type: "tool_use", ID: "toolu_1", Name: "f", Input: json.RawMessage(` {"a":1} `)}, `{"type":"tool_use","id":"toolu_1","name":"f","input":{"a":1}}`},
		"text_drops_foreign_fields":     {ClaudeContent{Type: "text", Text: "hi", Name: "x"}, `{"type":"text","text":"hi"}`},
		"other_type_generic_omitempty":  {ClaudeContent{Type: "redacted_thinking", Text: "d"}, `{"type":"redacted_thinking","text":"d"}`},
		"other_type_generic_empty_text": {ClaudeContent{Type: "image"}, `{"type":"image"}`},
	} {
		got, err := json.Marshal(tc.block)
		require.NoError(t, err, name)
		require.JSONEq(t, tc.want, string(got), name)
	}
}

// TestClaudeResponseMarshalNullStopSequence verifies stop_sequence is always
// present (null when unset) and that decoding still round-trips every field.
func TestClaudeResponseMarshalNullStopSequence(t *testing.T) {
	t.Parallel()
	resp := ClaudeResponse{
		ID: "msg_1", Type: "message", Role: "assistant", Model: "m", StopReason: "end_turn",
		Content: []ClaudeContent{{Type: "text"}, {Type: "tool_use", ID: "toolu_1", Name: "f"}},
		Usage:   ClaudeUsage{InputTokens: 1, OutputTokens: 2},
	}
	body, err := json.Marshal(resp)
	require.NoError(t, err)
	require.Contains(t, string(body), `"stop_sequence":null`)

	var decoded ClaudeResponse
	require.NoError(t, json.Unmarshal(body, &decoded))
	require.Nil(t, decoded.StopSequence)
	require.Len(t, decoded.Content, 2)
	require.Equal(t, "tool_use", decoded.Content[1].Type)
	require.JSONEq(t, `{}`, string(decoded.Content[1].Input))
	require.Equal(t, 2, decoded.Usage.OutputTokens)
}
