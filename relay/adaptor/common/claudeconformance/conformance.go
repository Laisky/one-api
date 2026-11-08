// Package claudeconformance validates Anthropic Messages API wire payloads
// (non-streaming Message objects and SSE event streams) against the public
// contract that Claude Code and the official Anthropic SDKs rely on.
//
// The validator is protocol-only: it never inspects provider-specific fields and
// it never treats a field the specification marks optional as a failure. It is
// imported by tests across adaptor and controller packages so every converted
// upstream (OpenAI Chat, OpenAI Responses, Gemini, ...) is held to one contract.
package claudeconformance

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Rule identifiers. Each violation names exactly one rule so regression tests
// and reproduction reports can enumerate which contract clauses failed.
const (
	RuleSSEEventName        = "sse.event_name_matches_type"
	RuleSSEDataJSON         = "sse.data_is_json_object"
	RuleSSEUnknownType      = "sse.known_event_type"
	RuleSSETrailingData     = "sse.no_unterminated_event"
	RuleStreamFirstEvent    = "stream.message_start_first"
	RuleStreamSingleStart   = "stream.single_message_start"
	RuleStartMessageShape   = "stream.message_start_message_shape"
	RuleBlockIndexDense     = "stream.block_index_dense_start_order"
	RuleBlockShape          = "stream.content_block_start_shape"
	RuleDeltaOpenBlock      = "stream.delta_targets_open_block"
	RuleDeltaTypeMatch      = "stream.delta_type_matches_block"
	RuleStopOpenBlock       = "stream.stop_targets_open_block"
	RuleBlocksClosed        = "stream.blocks_closed_before_message_delta"
	RuleContentAfterDelta   = "stream.no_content_after_message_delta"
	RuleMessageDeltaShape   = "stream.message_delta_shape"
	RuleMessageDeltaPresent = "stream.message_delta_present"
	RuleFinalStopReason     = "stream.final_stop_reason_non_null"
	RuleMessageStopLast     = "stream.message_stop_terminates"
	RuleToolInputJSON       = "stream.tool_input_json_object"
	RuleMessageShape        = "message.required_fields"
	RuleStopReasonEnum      = "message.stop_reason_enum"
	RuleStopSequenceKey     = "message.stop_sequence_present"
	RuleUsageShape          = "message.usage_shape"
	RuleContentShape        = "message.content_block_shape"
	RuleToolUseID           = "message.tool_use_id_valid_unique"
	RuleStopReasonToolUse   = "message.stop_reason_consistent_with_tool_use"
)

// toolUseIDPattern is the identifier grammar the Messages API enforces when a
// client echoes a tool_use id back in a later tool_result.
var toolUseIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// stopReasons is the closed stop_reason enumeration the SDKs type-check.
var stopReasons = map[string]bool{
	"end_turn":                      true,
	"max_tokens":                    true,
	"stop_sequence":                 true,
	"tool_use":                      true,
	"pause_turn":                    true,
	"refusal":                       true,
	"model_context_window_exceeded": true,
}

// Violation records one broken contract clause.
type Violation struct {
	Rule   string
	Detail string
}

// String renders the violation as "rule: detail" for test failure output.
func (v Violation) String() string {
	return v.Rule + ": " + v.Detail
}

// Report is the result of validating one payload. Message is the assembled
// Message object (directly decoded for non-streaming payloads, accumulated from
// events for streams) so tests can assert on semantic content as well.
type Report struct {
	Violations []Violation
	Message    Message
	// Events lists SSE event types in wire order (streams only).
	Events []string
}

// Message is the validator's decoded view of a Messages API Message object.
type Message struct {
	ID           string
	Model        string
	StopReason   string
	StopSequence *string
	Usage        map[string]any
	Content      []Block
}

// Block is a decoded content block with its accumulated streaming payload.
type Block struct {
	Type      string
	Text      string
	Thinking  string
	Signature string
	ID        string
	Name      string
	// Input is the raw JSON of the tool input (decoded object or, for streams,
	// the concatenated partial_json; empty concatenation means {}).
	Input string
}

// Rules returns the distinct rule identifiers present in the report, in first
// occurrence order.
func (r Report) Rules() []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range r.Violations {
		if !seen[v.Rule] {
			seen[v.Rule] = true
			out = append(out, v.Rule)
		}
	}
	return out
}

// Text returns the concatenation of every text block in the message.
func (m Message) Text() string {
	var sb strings.Builder
	for _, b := range m.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

// ToolUses returns the tool_use blocks of the message in content order.
func (m Message) ToolUses() []Block {
	var out []Block
	for _, b := range m.Content {
		if b.Type == "tool_use" {
			out = append(out, b)
		}
	}
	return out
}

// reporter accumulates violations for one validation run.
type reporter struct {
	violations []Violation
}

// addf records a violation for rule with a formatted detail message.
func (r *reporter) addf(rule, format string, args ...any) {
	r.violations = append(r.violations, Violation{Rule: rule, Detail: fmt.Sprintf(format, args...)})
}

// ValidateMessage validates a non-streaming Messages API response body.
// It returns a report whose Message is populated when the body decodes.
func ValidateMessage(body []byte) Report {
	rep := &reporter{}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		rep.addf(RuleMessageShape, "body is not a JSON object: %v", err)
		return Report{Violations: rep.violations}
	}
	msg := validateMessageObject(rep, root, false)
	validateToolUseIDs(rep, msg.Content)
	validateStopReasonToolConsistency(rep, msg)
	return Report{Violations: rep.violations, Message: msg}
}

// validateMessageObject checks the Message envelope. inStart selects the
// message_start variant (empty content, null stop_reason) instead of the final
// object (non-null stop_reason, decoded content blocks).
func validateMessageObject(rep *reporter, root map[string]json.RawMessage, inStart bool) Message {
	rule := RuleMessageShape
	if inStart {
		rule = RuleStartMessageShape
	}
	msg := Message{}
	if id, ok := stringField(root, "id"); !ok || id == "" {
		rep.addf(rule, "id must be a non-empty string")
	} else {
		msg.ID = id
	}
	if typ, ok := stringField(root, "type"); !ok || typ != "message" {
		rep.addf(rule, `type must be "message", got %q`, typ)
	}
	if role, ok := stringField(root, "role"); !ok || role != "assistant" {
		rep.addf(rule, `role must be "assistant", got %q`, role)
	}
	if model, ok := stringField(root, "model"); !ok {
		rep.addf(rule, "model must be a string")
	} else {
		msg.Model = model
	}

	rawContent, ok := root["content"]
	var blocks []json.RawMessage
	if !ok || json.Unmarshal(rawContent, &blocks) != nil || blocks == nil {
		rep.addf(rule, "content must be an array")
	}
	if inStart && len(blocks) != 0 {
		rep.addf(rule, "message_start content must be empty, got %d blocks", len(blocks))
	}

	rawStop, hasStop := root["stop_reason"]
	switch {
	case !hasStop:
		rep.addf(rule, "stop_reason key must be present")
	case inStart:
		if !isNull(rawStop) {
			rep.addf(rule, "message_start stop_reason must be null, got %s", string(rawStop))
		}
	default:
		var reason string
		if isNull(rawStop) || json.Unmarshal(rawStop, &reason) != nil {
			rep.addf(RuleStopReasonEnum, "stop_reason must be a non-null string, got %s", string(rawStop))
		} else if !stopReasons[reason] {
			rep.addf(RuleStopReasonEnum, "stop_reason %q is not in the enum", reason)
		}
		msg.StopReason = reason
	}

	stopSeqRule := RuleStopSequenceKey
	if inStart {
		stopSeqRule = rule
	}
	if rawSeq, ok := root["stop_sequence"]; !ok {
		rep.addf(stopSeqRule, "stop_sequence key must be present (string or null)")
	} else if !isNull(rawSeq) {
		var seq string
		if json.Unmarshal(rawSeq, &seq) != nil {
			rep.addf(stopSeqRule, "stop_sequence must be a string or null, got %s", string(rawSeq))
		} else {
			msg.StopSequence = &seq
		}
	}

	msg.Usage = validateUsage(rep, root["usage"], inStart, true)

	if !inStart {
		for i, raw := range blocks {
			msg.Content = append(msg.Content, validateContentBlock(rep, RuleContentShape, i, raw, false))
		}
	}
	return msg
}

// validateUsage checks a usage object. requireInput selects the Message usage
// variant (input_tokens and output_tokens required); the message_delta variant
// only requires output_tokens. Optional cache counters may be absent or null but
// must be non-negative integers when present.
func validateUsage(rep *reporter, raw json.RawMessage, inStart, requireInput bool) map[string]any {
	rule := RuleUsageShape
	if inStart {
		rule = RuleStartMessageShape
	}
	if len(raw) == 0 || isNull(raw) {
		rep.addf(rule, "usage object must be present")
		return nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		rep.addf(rule, "usage must be an object, got %s", string(raw))
		return nil
	}
	required := []string{"output_tokens"}
	if requireInput {
		required = []string{"input_tokens", "output_tokens"}
	}
	for _, key := range required {
		if !isNonNegativeInt(fields[key]) {
			rep.addf(rule, "usage.%s must be a non-negative integer, got %s", key, string(fields[key]))
		}
	}
	for _, key := range []string{"input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens"} {
		value, ok := fields[key]
		if !ok || isNull(value) {
			continue
		}
		if !isNonNegativeInt(value) {
			rep.addf(rule, "usage.%s must be a non-negative integer or null, got %s", key, string(value))
		}
	}
	decoded := map[string]any{}
	_ = json.Unmarshal(raw, &decoded)
	return decoded
}

// validateContentBlock checks one content block. streamStart selects the
// content_block_start variant, whose payload fields must be empty placeholders.
func validateContentBlock(rep *reporter, rule string, index int, raw json.RawMessage, streamStart bool) Block {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		rep.addf(rule, "block %d must be an object", index)
		return Block{}
	}
	typ, _ := stringField(fields, "type")
	block := Block{Type: typ}
	switch typ {
	case "text":
		text, ok := stringField(fields, "text")
		if !ok {
			rep.addf(rule, "text block %d must carry a string text field", index)
		}
		if streamStart && text != "" {
			rep.addf(rule, "text block %d start must carry empty text, got %q", index, text)
		}
		block.Text = text
	case "thinking":
		thinking, ok := stringField(fields, "thinking")
		if !ok {
			rep.addf(rule, "thinking block %d must carry a string thinking field", index)
		}
		signature, ok := stringField(fields, "signature")
		if !ok {
			rep.addf(rule, "thinking block %d must carry a string signature field", index)
		}
		block.Thinking, block.Signature = thinking, signature
	case "redacted_thinking":
		if _, ok := stringField(fields, "data"); !ok {
			rep.addf(rule, "redacted_thinking block %d must carry a string data field", index)
		}
	case "tool_use", "server_tool_use":
		block.ID, _ = stringField(fields, "id")
		name, ok := stringField(fields, "name")
		if !ok || name == "" {
			rep.addf(rule, "%s block %d must carry a non-empty name", typ, index)
		}
		block.Name = name
		input, ok := fields["input"]
		var obj map[string]any
		if !ok || isNull(input) || json.Unmarshal(input, &obj) != nil || obj == nil {
			rep.addf(rule, "%s block %d input must be a JSON object, got %q", typ, index, string(input))
		} else if streamStart && len(obj) != 0 {
			rep.addf(rule, "%s block %d start input must be {}, got %s", typ, index, string(input))
		}
		if !streamStart {
			block.Input = string(input)
		}
	case "":
		rep.addf(rule, "block %d is missing its type", index)
	}
	return block
}

// validateToolUseIDs enforces the tool_use id grammar and uniqueness.
func validateToolUseIDs(rep *reporter, blocks []Block) {
	seen := map[string]bool{}
	for i, b := range blocks {
		if b.Type != "tool_use" {
			continue
		}
		if !toolUseIDPattern.MatchString(b.ID) {
			rep.addf(RuleToolUseID, "tool_use block %d id %q must match ^[a-zA-Z0-9_-]+$", i, b.ID)
			continue
		}
		if seen[b.ID] {
			rep.addf(RuleToolUseID, "tool_use block %d id %q is duplicated", i, b.ID)
		}
		seen[b.ID] = true
	}
}

// validateStopReasonToolConsistency requires stop_reason tool_use exactly when
// the turn ends by requesting tools: agent loops and the SDK tool runners key
// on stop_reason == "tool_use". A max_tokens truncation may still carry blocks.
func validateStopReasonToolConsistency(rep *reporter, msg Message) {
	tools := len(msg.ToolUses())
	switch {
	case msg.StopReason == "tool_use" && tools == 0:
		rep.addf(RuleStopReasonToolUse, "stop_reason tool_use without any tool_use block")
	case tools > 0 && msg.StopReason != "tool_use" && msg.StopReason != "max_tokens" && msg.StopReason != "pause_turn":
		rep.addf(RuleStopReasonToolUse, "%d tool_use block(s) but stop_reason %q", tools, msg.StopReason)
	}
}

// stringField decodes a string field, reporting whether it was a JSON string.
func stringField(fields map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := fields[key]
	if !ok {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// isNull reports whether raw is the JSON null literal.
func isNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

// isNonNegativeInt reports whether raw is a JSON integer >= 0.
func isNonNegativeInt(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var decoded any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if dec.Decode(&decoded) != nil {
		return false
	}
	n, ok := decoded.(json.Number)
	if !ok {
		return false
	}
	v, err := n.Int64()
	return err == nil && v >= 0
}
