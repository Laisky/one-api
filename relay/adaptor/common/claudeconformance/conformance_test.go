package claudeconformance

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// canonicalStream mirrors a real Anthropic Messages stream with thinking, text,
// and two tool_use blocks, including a ping and nullable/optional usage fields.
const canonicalStream = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","model":"claude-x","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1,"cache_creation_input_tokens":null,"cache_read_input_tokens":0}}}` + "\n\n" +
	"event: ping\ndata: {\"type\":\"ping\"}\n\n" +
	"event: content_block_start\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}` + "\n\n" +
	"event: content_block_delta\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hmm"}}` + "\n\n" +
	"event: content_block_delta\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig"}}` + "\n\n" +
	"event: content_block_stop\n" + `data: {"type":"content_block_stop","index":0}` + "\n\n" +
	"event: content_block_start\n" + `data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}` + "\n\n" +
	"event: content_block_delta\n" + `data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Hi"}}` + "\n\n" +
	"event: content_block_stop\n" + `data: {"type":"content_block_stop","index":1}` + "\n\n" +
	"event: content_block_start\n" + `data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_a","name":"a","input":{}}}` + "\n\n" +
	"event: content_block_delta\n" + `data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"x\":"}}` + "\n\n" +
	"event: content_block_delta\n" + `data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"1}"}}` + "\n\n" +
	"event: content_block_stop\n" + `data: {"type":"content_block_stop","index":2}` + "\n\n" +
	"event: content_block_start\n" + `data: {"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"toolu_b","name":"b","input":{}}}` + "\n\n" +
	"event: content_block_stop\n" + `data: {"type":"content_block_stop","index":3}` + "\n\n" +
	"event: message_delta\n" + `data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":42}}` + "\n\n" +
	"event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n"

// canonicalMessage mirrors a real non-streaming Anthropic Message.
const canonicalMessage = `{"id":"msg_01","type":"message","role":"assistant","model":"claude-x",` +
	`"content":[{"type":"thinking","thinking":"hmm","signature":"sig"},{"type":"text","text":""},{"type":"tool_use","id":"toolu_a","name":"a","input":{}}],` +
	`"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":3,"output_tokens":4,"cache_read_input_tokens":null}}`

// TestValidateStreamCanonicalPasses guards against false positives on a fully
// conformant stream and checks the SDK-style accumulation.
func TestValidateStreamCanonicalPasses(t *testing.T) {
	t.Parallel()
	report := ValidateStream([]byte(canonicalStream))
	require.Empty(t, report.Violations)
	require.Equal(t, "Hi", report.Message.Text())
	require.Equal(t, "tool_use", report.Message.StopReason)
	tools := report.Message.ToolUses()
	require.Len(t, tools, 2)
	require.JSONEq(t, `{"x":1}`, tools[0].Input)
	require.JSONEq(t, `{}`, tools[1].Input)
	require.Equal(t, "sig", report.Message.Content[0].Signature)
}

// TestValidateMessageCanonicalPasses guards against false positives on a
// conformant Message, including null optional usage counters.
func TestValidateMessageCanonicalPasses(t *testing.T) {
	t.Parallel()
	report := ValidateMessage([]byte(canonicalMessage))
	require.Empty(t, report.Violations)
	require.Equal(t, "tool_use", report.Message.StopReason)
}

// TestValidateStreamMutationsAreDetected proves each stream rule fires on the
// corresponding contract break (mutation check for the validator itself).
func TestValidateStreamMutationsAreDetected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, old, new, rule string
	}{
		{"missing_event_line", "event: ping\n", "", RuleSSEEventName},
		{"start_without_usage", `,"usage":{"input_tokens":10,"output_tokens":1,"cache_creation_input_tokens":null,"cache_read_input_tokens":0}}}`, `}}`, RuleStartMessageShape},
		{"start_without_id", `"id":"msg_01",`, "", RuleStartMessageShape},
		{"start_without_stop_reason", `"stop_reason":null,"stop_sequence":null,"usage"`, `"usage"`, RuleStartMessageShape},
		{"sparse_index", `"index":3,"content_block"`, `"index":7,"content_block"`, RuleBlockIndexDense},
		{"thinking_start_no_signature", `"thinking":"","signature":""}`, `"thinking":""}`, RuleBlockShape},
		{"text_start_no_text", `{"type":"text","text":""}`, `{"type":"text"}`, RuleBlockShape},
		{"delta_type_mismatch", `"delta":{"type":"text_delta","text":"Hi"}`, `"delta":{"type":"input_json_delta","partial_json":"Hi"}`, RuleDeltaTypeMatch},
		{"message_delta_without_delta", `"delta":{"stop_reason":"tool_use","stop_sequence":null},`, "", RuleMessageDeltaShape},
		{"null_final_stop_reason", `"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":42}`, `"stop_reason":null,"stop_sequence":null},"usage":{"output_tokens":42}`, RuleFinalStopReason},
		{"bad_stop_reason", `"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":42}`, `"stop_reason":"stop","stop_sequence":null},"usage":{"output_tokens":42}`, RuleStopReasonEnum},
		{"delta_usage_missing_output", `"usage":{"output_tokens":42}`, `"usage":{"input_tokens":42}`, RuleUsageShape},
		{"no_message_stop", "event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n", "", RuleMessageStopLast},
		{"trailing_done", "event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n", "event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\ndata: [DONE]\n\n", RuleSSEDataJSON},
		{"broken_tool_json", `"partial_json":"1}"`, `"partial_json":"1"`, RuleToolInputJSON},
		{"duplicate_tool_id", `"id":"toolu_b"`, `"id":"toolu_a"`, RuleToolUseID},
		{"invalid_tool_id", `"id":"toolu_b"`, `"id":"tool b"`, RuleToolUseID},
		{"end_turn_with_tools", `"stop_reason":"tool_use","stop_sequence":null},"usage"`, `"stop_reason":"end_turn","stop_sequence":null},"usage"`, RuleStopReasonToolUse},
		{"unterminated", "event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n", "event: message_stop\n" + `data: {"type":"message_stop"}`, RuleSSETrailingData},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Contains(t, canonicalStream, tc.old)
			mutated := strings.Replace(canonicalStream, tc.old, tc.new, 1)
			report := ValidateStream([]byte(mutated))
			require.Contains(t, report.Rules(), tc.rule, "violations: %v", report.Violations)
		})
	}
}

// TestValidateStreamClosedBlockAndLateContent proves events for stopped blocks
// and content after message_delta are reported.
func TestValidateStreamClosedBlockAndLateContent(t *testing.T) {
	t.Parallel()
	lateDelta := "event: content_block_delta\n" + `data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"late"}}` + "\n\n"
	anchor := "event: content_block_start\n" + `data: {"type":"content_block_start","index":2`
	mutated := strings.Replace(canonicalStream, anchor, lateDelta+anchor, 1)
	require.Contains(t, ValidateStream([]byte(mutated)).Rules(), RuleDeltaOpenBlock)

	stopAnchor := "event: message_stop\n"
	unclosed := strings.Replace(canonicalStream, "event: content_block_stop\n"+`data: {"type":"content_block_stop","index":3}`+"\n\n", "", 1)
	unclosed = strings.Replace(unclosed, stopAnchor, "event: content_block_stop\n"+`data: {"type":"content_block_stop","index":3}`+"\n\n"+stopAnchor, 1)
	rules := ValidateStream([]byte(unclosed)).Rules()
	require.Contains(t, rules, RuleBlocksClosed)
	require.Contains(t, rules, RuleContentAfterDelta)
}

// TestValidateMessageMutationsAreDetected proves each Message rule fires.
func TestValidateMessageMutationsAreDetected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, old, new, rule string
	}{
		{"omitted_stop_sequence", `"stop_sequence":null,`, "", RuleStopSequenceKey},
		{"empty_text_omitted", `{"type":"text","text":""}`, `{"type":"text"}`, RuleContentShape},
		{"tool_input_omitted", `"name":"a","input":{}}`, `"name":"a"}`, RuleContentShape},
		{"tool_input_string", `"name":"a","input":{}}`, `"name":"a","input":"{}"}`, RuleContentShape},
		{"thinking_no_signature", `"thinking":"hmm","signature":"sig"}`, `"thinking":"hmm"}`, RuleContentShape},
		{"stop_reason_not_enum", `"stop_reason":"tool_use"`, `"stop_reason":"stop"`, RuleStopReasonEnum},
		{"usage_missing_output", `"output_tokens":4,`, "", RuleUsageShape},
		{"missing_id", `"id":"msg_01",`, `"id":"",`, RuleMessageShape},
		{"tool_use_reason_without_tools", `,{"type":"tool_use","id":"toolu_a","name":"a","input":{}}`, "", RuleStopReasonToolUse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Contains(t, canonicalMessage, tc.old)
			mutated := strings.Replace(canonicalMessage, tc.old, tc.new, 1)
			report := ValidateMessage([]byte(mutated))
			require.Contains(t, report.Rules(), tc.rule, "violations: %v", report.Violations)
		})
	}
}
