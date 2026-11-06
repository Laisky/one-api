package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// claudeEnvelopeForTest encodes block or fails the test.
func claudeEnvelopeForTest(t *testing.T, block model.ClaudeThinkingBlock) string {
	t.Helper()
	envelope, err := model.EncodeClaudeThinkingEnvelope(block)
	require.NoError(t, err)
	return envelope
}

// decodeResponseInputForTest decodes a Responses input array as the HTTP layer does.
func decodeResponseInputForTest(t *testing.T, raw string) ResponseAPIInput {
	t.Helper()
	var input ResponseAPIInput
	require.NoError(t, json.Unmarshal([]byte(raw), &input))
	return input
}

// TestConvertResponseAPIReplaysClaudeEnvelopesInCodexDoneOrder verifies the
// item order Codex records (reasoning, function_call, message, outputs) restores
// each turn's verbatim blocks onto that turn's assistant message only.
func TestConvertResponseAPIReplaysClaudeEnvelopesInCodexDoneOrder(t *testing.T) {
	t.Parallel()
	first := claudeEnvelopeForTest(t, model.ClaudeThinkingBlock{Type: "thinking", Signature: "sig1"})
	second := claudeEnvelopeForTest(t, model.ClaudeThinkingBlock{Type: "redacted_thinking", Data: "opaque", ToolCallsBefore: 1})
	third := claudeEnvelopeForTest(t, model.ClaudeThinkingBlock{Type: "thinking", Thinking: "next", Signature: "sig3"})
	input := decodeResponseInputForTest(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"go"}]},
		{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"`+first+`"},
		{"type":"reasoning","id":"rs_2","summary":[],"encrypted_content":"`+second+`"},
		{"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":"{}"},
		{"type":"function_call","id":"fc_2","call_id":"call_2","name":"shell","arguments":"{}"},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Running."}]},
		{"type":"function_call_output","call_id":"call_1","output":"a"},
		{"type":"function_call_output","call_id":"call_2","output":"b"},
		{"type":"reasoning","id":"rs_3","summary":[{"type":"summary_text","text":"next"}],"encrypted_content":"`+third+`"},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Done."}]}
	]`)

	chat, err := ConvertResponseAPIToChatCompletionRequest(&ResponseAPIRequest{Model: "claude-opus-5-5", Input: input})
	require.NoError(t, err)
	require.Len(t, chat.Messages, 5)
	turn := chat.Messages[1]
	require.Equal(t, "assistant", turn.Role)
	require.Equal(t, "Running.", turn.StringContent())
	require.Len(t, turn.ToolCalls, 2)
	require.Equal(t, []model.ClaudeThinkingBlock{
		{Type: "thinking", Signature: "sig1"},
		{Type: "redacted_thinking", Data: "opaque", ToolCallsBefore: 1},
	}, turn.ClaudeThinkingBlocks)
	require.Equal(t, "tool", chat.Messages[2].Role)
	require.Equal(t, "tool", chat.Messages[3].Role)
	final := chat.Messages[4]
	require.Equal(t, "Done.", final.StringContent())
	require.Equal(t, []model.ClaudeThinkingBlock{{Type: "thinking", Thinking: "next", Signature: "sig3"}}, final.ClaudeThinkingBlocks)
	// Non-Claude upstreams still see the summary through reasoning_content.
	require.NotNil(t, final.ReasoningContent)
	require.Equal(t, "next", *final.ReasoningContent)
}

// TestConvertResponseAPIReplaysClaudeEnvelopesInOutputIndexOrder verifies the
// response.output order (message, reasoning, function_call) restores blocks too.
func TestConvertResponseAPIReplaysClaudeEnvelopesInOutputIndexOrder(t *testing.T) {
	t.Parallel()
	envelope := claudeEnvelopeForTest(t, model.ClaudeThinkingBlock{Type: "thinking", Signature: "sig1"})
	input := decodeResponseInputForTest(t, `[
		{"type":"message","role":"user","content":"go"},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":""}]},
		{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"`+envelope+`"},
		{"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_1","output":"a"}
	]`)
	chat, err := ConvertResponseAPIToChatCompletionRequest(&ResponseAPIRequest{Model: "claude-opus-5-5", Input: input})
	require.NoError(t, err)
	require.Len(t, chat.Messages, 3)
	require.Len(t, chat.Messages[1].ToolCalls, 1)
	require.Equal(t, []model.ClaudeThinkingBlock{{Type: "thinking", Signature: "sig1"}}, chat.Messages[1].ClaudeThinkingBlocks)
	require.Equal(t, "tool", chat.Messages[2].Role)
}

// TestConvertResponseAPIIgnoresForeignAndMalformedEnvelopes verifies foreign
// provider state, malformed, oversized and tampered envelopes never produce
// blocks and keep the established summary-text replay.
func TestConvertResponseAPIIgnoresForeignAndMalformedEnvelopes(t *testing.T) {
	t.Parallel()
	for name, envelope := range map[string]string{
		"openai":    "gAAAAABn-openai-encrypted",
		"malformed": model.ClaudeThinkingEnvelopePrefix + "%%%",
		"oversized": model.ClaudeThinkingEnvelopePrefix + strings.Repeat("A", model.MaxClaudeThinkingEnvelopeBytes+1),
		"no_state":  model.ClaudeThinkingEnvelopePrefix + "eyJ0IjoidGV4dCJ9", // {"t":"text"}
	} {
		t.Run(name, func(t *testing.T) {
			input := ResponseAPIInput{
				map[string]any{"type": "message", "role": "user", "content": "q"},
				map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": "visible"}}, "encrypted_content": envelope},
				map[string]any{"type": "message", "role": "assistant", "content": "a"},
			}
			chat, err := ConvertResponseAPIToChatCompletionRequest(&ResponseAPIRequest{Model: "claude-opus-5-5", Input: input})
			require.NoError(t, err)
			require.Len(t, chat.Messages, 2)
			require.Empty(t, chat.Messages[1].ClaudeThinkingBlocks)
			require.NotNil(t, chat.Messages[1].ReasoningContent)
			require.Equal(t, "visible", *chat.Messages[1].ReasoningContent)

			stats := InspectResponseClaudeThinkingEnvelopes(input)
			require.Equal(t, 0, stats.Valid)
			if model.IsClaudeThinkingEnvelope(envelope) {
				require.Equal(t, 1, stats.Ignored)
			} else {
				require.Equal(t, 0, stats.Ignored)
			}
		})
	}
}

// TestConvertResponseAPIDropsStagedClaudeBlocksAtUserBoundary verifies a block
// staged before a user message never attaches to a later assistant turn.
func TestConvertResponseAPIDropsStagedClaudeBlocksAtUserBoundary(t *testing.T) {
	t.Parallel()
	envelope := claudeEnvelopeForTest(t, model.ClaudeThinkingBlock{Type: "thinking", Signature: "sig1"})
	input := ResponseAPIInput{
		map[string]any{"type": "reasoning", "summary": []any{}, "encrypted_content": envelope},
		"a user turn",
		map[string]any{"type": "message", "role": "assistant", "content": "later"},
	}
	chat, err := ConvertResponseAPIToChatCompletionRequest(&ResponseAPIRequest{Model: "claude-opus-5-5", Input: input})
	require.NoError(t, err)
	for _, message := range chat.Messages {
		require.Empty(t, message.ClaudeThinkingBlocks)
	}
	require.Equal(t, ResponseClaudeThinkingEnvelopeStats{Valid: 1}, InspectResponseClaudeThinkingEnvelopes(input))
}

// TestOutputItemReasoningAlwaysCarriesSummary verifies an omitted-display
// reasoning item still serializes the schema-required summary array.
func TestOutputItemReasoningAlwaysCarriesSummary(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(OutputItem{Type: "reasoning", Id: "rs_1", EncryptedContent: "oneapi.claude.v1.e30"})
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"oneapi.claude.v1.e30"}`, string(encoded))

	encoded, err = json.Marshal(OutputItem{Type: "reasoning", Summary: []OutputContent{{Type: "summary_text", Text: "s"}}})
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"reasoning","summary":[{"type":"summary_text","text":"s"}]}`, string(encoded))
}
