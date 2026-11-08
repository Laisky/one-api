package claudeconformance

import (
	"encoding/json"
	"strings"
)

// knownEventTypes lists the Messages streaming event types the SDKs accept.
var knownEventTypes = map[string]bool{
	"message_start":       true,
	"content_block_start": true,
	"content_block_delta": true,
	"content_block_stop":  true,
	"message_delta":       true,
	"message_stop":        true,
	"ping":                true,
	"error":               true,
}

// deltaTypesByBlock lists the delta types valid for each content block type.
var deltaTypesByBlock = map[string]map[string]bool{
	"text":            {"text_delta": true, "citations_delta": true},
	"thinking":        {"thinking_delta": true, "signature_delta": true},
	"tool_use":        {"input_json_delta": true},
	"server_tool_use": {"input_json_delta": true},
}

// sseEvent is one dispatched server-sent event.
type sseEvent struct {
	name string
	data string
}

// parseSSE splits raw bytes into dispatched events following the SSE framing
// rules: field lines accumulate until a blank line dispatches the event, comment
// lines are ignored, and one leading space after the colon is stripped. It also
// reports whether trailing field lines were left without a dispatching blank line.
func parseSSE(raw string) (events []sseEvent, trailing bool) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	var name string
	var data []string
	hasData, pending := false, false
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			if hasData {
				events = append(events, sseEvent{name: name, data: strings.Join(data, "\n")})
			}
			name, data, hasData, pending = "", nil, false, false
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		pending = true
		switch field {
		case "event":
			name = value
		case "data":
			data = append(data, value)
			hasData = true
		}
	}
	return events, pending
}

// streamBlock tracks one content block while validating a stream.
type streamBlock struct {
	block  Block
	closed bool
	input  strings.Builder
}

// streamState carries ordering state across events.
type streamState struct {
	rep            *reporter
	started        bool
	startCount     int
	blocks         []*streamBlock
	messageDeltas  int
	messageStopped bool
	finalReason    *string
	msg            Message
	events         []string
}

// ValidateStream validates a Messages SSE response body and accumulates the
// final Message from its events the same way the SDK stream accumulators do.
func ValidateStream(body []byte) Report {
	st := &streamState{rep: &reporter{}}
	events, trailing := parseSSE(string(body))
	if trailing {
		st.rep.addf(RuleSSETrailingData, "stream ends with an event that has no terminating blank line")
	}
	for i, ev := range events {
		st.consume(i, ev)
	}
	st.finish()
	return Report{Violations: st.rep.violations, Message: st.msg, Events: st.events}
}

// consume validates one dispatched SSE event.
func (st *streamState) consume(i int, ev sseEvent) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(ev.data), &root); err != nil || root == nil {
		st.rep.addf(RuleSSEDataJSON, "event %d data is not a JSON object: %q", i, truncate(ev.data))
		return
	}
	typ, _ := stringField(root, "type")
	st.events = append(st.events, typ)
	if ev.name == "" {
		st.rep.addf(RuleSSEEventName, "event %d (%s) has no event: line; SDKs drop it", i, typ)
	} else if ev.name != typ {
		st.rep.addf(RuleSSEEventName, "event %d event: %q does not match data.type %q", i, ev.name, typ)
	}
	if !knownEventTypes[typ] {
		st.rep.addf(RuleSSEUnknownType, "event %d has unknown type %q", i, typ)
		return
	}
	if typ == "ping" || typ == "error" {
		return
	}
	if st.messageStopped {
		st.rep.addf(RuleMessageStopLast, "event %d (%s) arrives after message_stop", i, typ)
	}
	if typ != "message_start" && !st.started {
		st.rep.addf(RuleStreamFirstEvent, "event %d (%s) arrives before message_start", i, typ)
	}
	switch typ {
	case "message_start":
		st.onMessageStart(i, root)
	case "content_block_start":
		st.onBlockStart(i, root)
	case "content_block_delta":
		st.onBlockDelta(i, root)
	case "content_block_stop":
		st.onBlockStop(i, root)
	case "message_delta":
		st.onMessageDelta(i, root)
	case "message_stop":
		if st.messageDeltas == 0 {
			st.rep.addf(RuleMessageDeltaPresent, "message_stop at event %d without a preceding message_delta", i)
		}
		st.messageStopped = true
	}
}

// onMessageStart validates the message_start envelope.
func (st *streamState) onMessageStart(i int, root map[string]json.RawMessage) {
	if st.started {
		st.rep.addf(RuleStreamSingleStart, "duplicate message_start at event %d", i)
		return
	}
	st.started = true
	var message map[string]json.RawMessage
	if json.Unmarshal(root["message"], &message) != nil || message == nil {
		st.rep.addf(RuleStartMessageShape, "message_start.message must be an object")
		return
	}
	st.msg = validateMessageObject(st.rep, message, true)
}

// blockIndex decodes the index of a content block event.
func (st *streamState) blockIndex(i int, root map[string]json.RawMessage, rule string) (int, bool) {
	var index int
	raw, ok := root["index"]
	if !ok || !isNonNegativeInt(raw) || json.Unmarshal(raw, &index) != nil {
		st.rep.addf(rule, "event %d index must be a non-negative integer, got %s", i, string(raw))
		return 0, false
	}
	return index, true
}

// onBlockStart validates content_block_start ordering and shape.
func (st *streamState) onBlockStart(i int, root map[string]json.RawMessage) {
	if st.messageDeltas > 0 {
		st.rep.addf(RuleContentAfterDelta, "content_block_start at event %d after message_delta", i)
	}
	index, ok := st.blockIndex(i, root, RuleBlockIndexDense)
	if ok && index != st.startCount {
		st.rep.addf(RuleBlockIndexDense, "content_block_start at event %d has index %d, SDK appends at %d", i, index, st.startCount)
	}
	st.startCount++
	block := validateContentBlock(st.rep, RuleBlockShape, st.startCount-1, root["content_block"], true)
	st.blocks = append(st.blocks, &streamBlock{block: block})
}

// openBlock returns the open block addressed by an event, recording a
// violation when it is unknown or already stopped.
func (st *streamState) openBlock(i int, root map[string]json.RawMessage, rule, kind string) *streamBlock {
	index, ok := st.blockIndex(i, root, rule)
	if !ok {
		return nil
	}
	if index >= len(st.blocks) {
		st.rep.addf(rule, "%s at event %d targets unknown block %d", kind, i, index)
		return nil
	}
	block := st.blocks[index]
	if block.closed {
		st.rep.addf(rule, "%s at event %d targets already-stopped block %d", kind, i, index)
		return nil
	}
	return block
}

// onBlockDelta validates content_block_delta targeting and payload.
func (st *streamState) onBlockDelta(i int, root map[string]json.RawMessage) {
	if st.messageDeltas > 0 {
		st.rep.addf(RuleContentAfterDelta, "content_block_delta at event %d after message_delta", i)
	}
	block := st.openBlock(i, root, RuleDeltaOpenBlock, "content_block_delta")
	if block == nil {
		return
	}
	var delta map[string]json.RawMessage
	if json.Unmarshal(root["delta"], &delta) != nil || delta == nil {
		st.rep.addf(RuleDeltaTypeMatch, "content_block_delta at event %d has no delta object", i)
		return
	}
	deltaType, _ := stringField(delta, "type")
	if allowed, known := deltaTypesByBlock[block.block.Type]; known && !allowed[deltaType] {
		st.rep.addf(RuleDeltaTypeMatch, "event %d delta %q does not fit %s block", i, deltaType, block.block.Type)
		return
	}
	field := map[string]string{
		"text_delta":       "text",
		"thinking_delta":   "thinking",
		"signature_delta":  "signature",
		"input_json_delta": "partial_json",
	}[deltaType]
	if field == "" {
		return
	}
	value, ok := stringField(delta, field)
	if !ok {
		st.rep.addf(RuleDeltaTypeMatch, "event %d %s must carry a string %s", i, deltaType, field)
		return
	}
	switch deltaType {
	case "text_delta":
		block.block.Text += value
	case "thinking_delta":
		block.block.Thinking += value
	case "signature_delta":
		block.block.Signature += value
	case "input_json_delta":
		block.input.WriteString(value)
	}
}

// onBlockStop validates content_block_stop targeting and finalizes tool input.
func (st *streamState) onBlockStop(i int, root map[string]json.RawMessage) {
	if st.messageDeltas > 0 {
		st.rep.addf(RuleContentAfterDelta, "content_block_stop at event %d after message_delta", i)
	}
	block := st.openBlock(i, root, RuleStopOpenBlock, "content_block_stop")
	if block == nil {
		return
	}
	block.closed = true
	if block.block.Type == "tool_use" || block.block.Type == "server_tool_use" {
		input := strings.TrimSpace(block.input.String())
		if input == "" {
			input = "{}"
		}
		var obj map[string]any
		if json.Unmarshal([]byte(input), &obj) != nil || obj == nil {
			st.rep.addf(RuleToolInputJSON, "tool block at event %d accumulated input is not a JSON object: %q", i, truncate(input))
		}
		block.block.Input = input
	}
}

// onMessageDelta validates message_delta shape and block closure.
func (st *streamState) onMessageDelta(i int, root map[string]json.RawMessage) {
	if st.messageDeltas == 0 {
		for idx, block := range st.blocks {
			if !block.closed {
				st.rep.addf(RuleBlocksClosed, "block %d still open at first message_delta (event %d)", idx, i)
			}
		}
	}
	st.messageDeltas++
	var delta map[string]json.RawMessage
	if json.Unmarshal(root["delta"], &delta) != nil || delta == nil {
		st.rep.addf(RuleMessageDeltaShape, "message_delta at event %d must carry a delta object", i)
	} else {
		rawReason, ok := delta["stop_reason"]
		if !ok {
			st.rep.addf(RuleMessageDeltaShape, "message_delta at event %d delta.stop_reason key missing", i)
		}
		if _, ok := delta["stop_sequence"]; !ok {
			st.rep.addf(RuleMessageDeltaShape, "message_delta at event %d delta.stop_sequence key missing", i)
		}
		var reason string
		if ok && !isNull(rawReason) {
			if json.Unmarshal(rawReason, &reason) != nil || !stopReasons[reason] {
				st.rep.addf(RuleStopReasonEnum, "message_delta at event %d stop_reason %s is not in the enum", i, string(rawReason))
			}
			st.finalReason = &reason
		} else {
			st.finalReason = nil
		}
		if rawSeq, ok := delta["stop_sequence"]; ok && !isNull(rawSeq) {
			var seq string
			if json.Unmarshal(rawSeq, &seq) == nil {
				st.msg.StopSequence = &seq
			}
		}
	}
	usage := validateUsage(st.rep, root["usage"], false, false)
	if usage != nil {
		if st.msg.Usage == nil {
			st.msg.Usage = map[string]any{}
		}
		for k, v := range usage {
			st.msg.Usage[k] = v
		}
	}
}

// finish applies end-of-stream rules and assembles the final message.
func (st *streamState) finish() {
	if !st.started {
		st.rep.addf(RuleStreamFirstEvent, "stream never emitted message_start")
	}
	if st.messageDeltas == 0 {
		st.rep.addf(RuleMessageDeltaPresent, "stream never emitted message_delta")
	}
	if st.finalReason == nil {
		st.rep.addf(RuleFinalStopReason, "final message_delta stop_reason is missing or null")
	} else {
		st.msg.StopReason = *st.finalReason
	}
	if !st.messageStopped {
		st.rep.addf(RuleMessageStopLast, "stream never emitted message_stop")
	}
	for idx, block := range st.blocks {
		if !block.closed {
			st.rep.addf(RuleStopOpenBlock, "block %d was never stopped", idx)
			if block.block.Type == "tool_use" {
				block.block.Input = block.input.String()
			}
		}
		st.msg.Content = append(st.msg.Content, block.block)
	}
	validateToolUseIDs(st.rep, st.msg.Content)
	if st.finalReason != nil {
		validateStopReasonToolConsistency(st.rep, st.msg)
	}
}

// truncate shortens long payloads in violation details.
func truncate(s string) string {
	const limit = 160
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}
