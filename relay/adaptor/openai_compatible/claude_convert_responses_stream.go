package openai_compatible

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/common/toolnamesafe"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// claudeResponsesTool tracks one Responses function_call output item.
type claudeResponsesTool struct {
	callID    string
	name      string
	index     int
	started   bool
	done      bool
	delivered bool
	pending   strings.Builder
}

// claudeResponsesStreamTranslator converts OpenAI Responses stream events into
// Claude content block events on a shared emitter.
//
// Responses streams describe each output item several times: incremental
// deltas, then *.done snapshots (output_text.done, content_part.done,
// output_item.done) and finally the whole output again inside the terminal
// response.completed / response.incomplete event. The translator renders each
// output part exactly once: deltas stream through, and a snapshot is used only
// for a part that produced no deltas (upstreams that skip incremental events).
// Function calls map to tool_use blocks that open at output_item.added and
// close at output_item.done; the terminal event decides the stop_reason.
type claudeResponsesStreamTranslator struct {
	c           *gin.Context
	em          *ClaudeStreamEmitter
	ids         *ClaudeToolIDAllocator
	current     int
	currentKind string
	currentKey  string
	currentPart string
	emitted     map[string]bool
	indexByItem map[string]string
	rawItemKeys map[string]bool
	tools       map[string]*claudeResponsesTool
	toolOrder   []*claudeResponsesTool
	stopReason  string
	refusal     bool
	terminal    bool
	failed      bool
	// receipt records a terminal response event (completed, incomplete or
	// failed) that carried its final usage: the provider's authoritative bill,
	// exactly what the native Responses relay settles from.
	receipt bool
}

// newClaudeResponsesStreamTranslator returns a translator writing to em.
func newClaudeResponsesStreamTranslator(c *gin.Context, em *ClaudeStreamEmitter, ids *ClaudeToolIDAllocator) *claudeResponsesStreamTranslator {
	return &claudeResponsesStreamTranslator{
		c: c, em: em, ids: ids, current: -1,
		emitted: map[string]bool{}, indexByItem: map[string]string{}, rawItemKeys: map[string]bool{},
		tools: map[string]*claudeResponsesTool{},
	}
}

// itemKey identifies an output item. The output index is the canonical key
// because every OpenAI event carries it and it equals the item's position in
// the terminal response; an item id seen together with an index is remembered
// so id-only events resolve to the same key, and an id never seen with an index
// is used as is. Events locating nothing address the first item.
func (t *claudeResponsesStreamTranslator) itemKey(itemID string, outputIndex *int) string {
	if outputIndex != nil {
		key := "#" + strconv.Itoa(*outputIndex)
		if itemID != "" {
			t.indexByItem[itemID] = key
		}
		return key
	}
	if itemID != "" {
		if key, ok := t.indexByItem[itemID]; ok {
			return key
		}
		t.rawItemKeys[itemID] = true
		return itemID
	}
	return "#0"
}

// terminalItemKey resolves the key of the item at position in the terminal
// response output, reusing the key its streamed events were recorded under.
func (t *claudeResponsesStreamTranslator) terminalItemKey(item *responseAPIOutput, position int) string {
	if item.Id != "" {
		if key, ok := t.indexByItem[item.Id]; ok {
			return key
		}
		if t.rawItemKeys[item.Id] {
			return item.Id
		}
	}
	return "#" + strconv.Itoa(position)
}

// partIndex renders an optional part index, defaulting to the first part.
func partIndex(index *int) string {
	if index == nil {
		return "0"
	}
	return strconv.Itoa(*index)
}

// closeCurrent stops the open text/thinking block, if any.
func (t *claudeResponsesStreamTranslator) closeCurrent() {
	if t.current >= 0 && t.em.IsOpen(t.current) {
		t.em.StopBlock(t.current)
	}
	t.current, t.currentKind, t.currentKey, t.currentPart = -1, "", "", ""
}

// closeItem stops the open text/thinking block owned by item key.
func (t *claudeResponsesStreamTranslator) closeItem(key string) {
	if t.current >= 0 && t.currentKey == key {
		t.closeCurrent()
	}
}

// emitPart renders one text or thinking part of an output item. A snapshot
// (isDelta false) is skipped when the part already produced output; a new part
// of the same thinking block is separated by a blank line.
func (t *claudeResponsesStreamTranslator) emitPart(kind, key, part, text string, isDelta bool) {
	partKey := key + "|" + kind + "|" + part
	if text == "" || (!isDelta && t.emitted[partKey]) {
		return
	}
	t.emitted[partKey] = true
	if t.current < 0 || t.currentKind != kind || t.currentKey != key || !t.em.IsOpen(t.current) {
		t.closeCurrent()
		t.current = t.em.StartBlock(relaymodel.ClaudeContent{Type: kind})
		t.currentKind, t.currentKey = kind, key
	} else if kind == claudeBlockThinking && t.currentPart != part {
		text = "\n\n" + text
	}
	t.currentPart = part
	if kind == claudeBlockThinking {
		t.em.ThinkingDelta(t.current, text)
		return
	}
	t.em.TextDelta(t.current, text)
}

// tool returns the tracked function_call item for key, creating it on demand.
func (t *claudeResponsesStreamTranslator) tool(key string) *claudeResponsesTool {
	tool, ok := t.tools[key]
	if !ok {
		tool = &claudeResponsesTool{index: -1}
		t.tools[key] = tool
		t.toolOrder = append(t.toolOrder, tool)
	}
	return tool
}

// describeTool records the call id and name of a function_call item and opens
// its block once the name is known.
func (t *claudeResponsesStreamTranslator) describeTool(tool *claudeResponsesTool, callID, name string) {
	if tool.callID == "" {
		tool.callID = callID
	}
	if tool.name == "" && name != "" {
		tool.name = toolnamesafe.RestoreToolName(t.c, name)
	}
	if !tool.started && tool.name != "" {
		t.startTool(tool)
	}
}

// startTool opens a tool_use block and flushes buffered argument fragments.
func (t *claudeResponsesStreamTranslator) startTool(tool *claudeResponsesTool) {
	t.closeCurrent()
	tool.started = true
	tool.index = t.em.StartBlock(relaymodel.ClaudeContent{Type: claudeBlockToolUse, ID: t.ids.Allocate(tool.callID), Name: tool.name})
	if tool.pending.Len() > 0 {
		t.em.InputJSONDelta(tool.index, tool.pending.String())
		tool.pending.Reset()
	}
}

// toolArguments delivers argument text; a snapshot (isDelta false) is used
// only when no fragment was delivered before.
func (t *claudeResponsesStreamTranslator) toolArguments(tool *claudeResponsesTool, args string, isDelta bool) {
	if args == "" || (!isDelta && tool.delivered) {
		return
	}
	tool.delivered = true
	if tool.started {
		t.em.InputJSONDelta(tool.index, args)
		return
	}
	tool.pending.WriteString(args)
}

// finishTool completes a function_call item from its final snapshot.
func (t *claudeResponsesStreamTranslator) finishTool(key string, item *responseAPIOutput) {
	tool := t.tool(key)
	if tool.done {
		return
	}
	t.toolArguments(tool, item.Arguments, false)
	t.describeTool(tool, item.CallId, item.Name)
	if !tool.started {
		t.startTool(tool)
	}
	tool.done = true
	t.em.StopBlock(tool.index)
}

// renderItemSnapshot renders the not-yet-streamed parts of a completed output
// item and closes its block.
func (t *claudeResponsesStreamTranslator) renderItemSnapshot(key string, item *responseAPIOutput) {
	switch item.Type {
	case "message":
		for i, part := range item.Content {
			t.emitPart(claudeBlockText, key, strconv.Itoa(i), responseContentSnapshotText(&part), false)
		}
	case "reasoning":
		for i, part := range item.Summary {
			t.emitPart(claudeBlockThinking, key, strconv.Itoa(i), part.Text, false)
		}
		for i, part := range item.Content {
			if part.Type == "reasoning_text" {
				t.emitPart(claudeBlockThinking, key, "content"+strconv.Itoa(i), part.Text, false)
			}
		}
	case "function_call":
		t.finishTool(key, item)
		return
	}
	t.closeItem(key)
}

// responseContentSnapshotText returns the client-visible text of a message
// content part (output text, refusal text, or structured JSON).
func responseContentSnapshotText(part *responseAPIContent) string {
	switch part.Type {
	case "output_text", "text", "refusal":
		return part.Text
	case "output_json":
		if len(part.JSON) > 0 {
			return string(part.JSON)
		}
		return part.Text
	default:
		return ""
	}
}

// consume translates one decoded Responses stream event.
func (t *claudeResponsesStreamTranslator) consume(ev *responseAPIStreamEvent) {
	if t.failed {
		return
	}
	key := ""
	if ev.Item == nil {
		key = t.itemKey(ev.ItemId, ev.OutputIndex)
	}
	switch ev.Type {
	case "response.output_item.added":
		if ev.Item == nil {
			return
		}
		itemKey := t.itemKey(ev.Item.Id, ev.OutputIndex)
		if ev.Item.Type == "function_call" {
			tool := t.tool(itemKey)
			t.toolArguments(tool, ev.Item.Arguments, true)
			t.describeTool(tool, ev.Item.CallId, ev.Item.Name)
		}
	case "response.output_item.done":
		if ev.Item != nil {
			t.renderItemSnapshot(t.itemKey(ev.Item.Id, ev.OutputIndex), ev.Item)
		}
	case "response.output_text.delta", "response.refusal.delta":
		t.refusal = t.refusal || ev.Type == "response.refusal.delta"
		t.emitPart(claudeBlockText, key, partIndex(ev.ContentIndex), extractStringFromRawMessage(ev.Delta, "text", "delta"), true)
	case "response.output_text.done", "response.refusal.done":
		t.emitPart(claudeBlockText, key, partIndex(ev.ContentIndex), ev.Text, false)
	case "response.output_json.delta":
		t.emitPart(claudeBlockText, key, partIndex(ev.ContentIndex), extractStringFromRawMessage(ev.Delta, "partial_json", "json", "text"), true)
	case "response.output_json.done":
		t.emitPart(claudeBlockText, key, partIndex(ev.ContentIndex), string(extractJSONFromStreamEvent(ev)), false)
	case "response.content_part.done":
		if ev.Part != nil {
			t.emitPart(claudeBlockText, key, partIndex(ev.ContentIndex), responseContentSnapshotText(ev.Part), false)
		}
	case "response.reasoning_summary_text.delta":
		t.emitPart(claudeBlockThinking, key, partIndex(ev.SummaryIndex), extractStringFromRawMessage(ev.Delta, "text", "delta"), true)
	case "response.reasoning_summary_text.done":
		t.emitPart(claudeBlockThinking, key, partIndex(ev.SummaryIndex), ev.Text, false)
	case "response.reasoning_summary_part.done":
		if ev.Part != nil {
			t.emitPart(claudeBlockThinking, key, partIndex(ev.SummaryIndex), ev.Part.Text, false)
		}
	case "response.reasoning_text.delta":
		t.emitPart(claudeBlockThinking, key, "content"+partIndex(ev.ContentIndex), extractStringFromRawMessage(ev.Delta, "text", "delta"), true)
	case "response.reasoning_text.done":
		t.emitPart(claudeBlockThinking, key, "content"+partIndex(ev.ContentIndex), ev.Text, false)
	case "response.function_call_arguments.delta":
		t.toolArguments(t.tool(key), extractStringFromRawMessage(ev.Delta, "partial_json", "text", "arguments", "delta"), true)
	case "response.function_call_arguments.done":
		t.toolArguments(t.tool(key), ev.Arguments, false)
	case "response.completed", "response.incomplete", "response.failed", "response.cancelled":
		if ev.Response != nil {
			t.consumeTerminal(ev.Response, ev.Type)
		}
	case "error":
		t.fail(ev.Code, ev.Message)
	}
}

// consumeTerminal renders any output the stream did not deliver incrementally
// and records the stop_reason (or the failure) of the finished response.
func (t *claudeResponsesStreamTranslator) consumeTerminal(resp *responseAPIResponse, eventType string) {
	status := strings.ToLower(strings.TrimSpace(resp.Status))
	if resp.Usage != nil && status != "cancelled" && (status == "" || status == "completed" || status == "incomplete" || status == "failed") {
		t.receipt = true
	}
	if eventType == "response.failed" || status == "failed" || status == "cancelled" {
		code, message := "", "upstream response "+status
		if resp.Error != nil {
			code, message = resp.Error.Code, resp.Error.Message
		}
		t.fail(code, message)
		return
	}
	if status != "completed" && status != "incomplete" {
		return
	}
	for i := range resp.Output {
		item := &resp.Output[i]
		t.renderItemSnapshot(t.terminalItemKey(item, i), item)
	}
	incompleteReason := ""
	if resp.IncompleteDetails != nil {
		incompleteReason = resp.IncompleteDetails.Reason
	}
	t.stopReason = ClaudeStopReasonFromResponseStatus(status, incompleteReason)
	if t.refusal && t.stopReason == ClaudeStopEndTurn {
		t.stopReason = ClaudeStopRefusal
	}
	t.terminal = true
}

// fail marks the upstream response as failed and emits an Anthropic error
// event; nothing further is rendered for this message.
func (t *claudeResponsesStreamTranslator) fail(code, message string) {
	if t.failed {
		return
	}
	t.failed = true
	if strings.TrimSpace(message) == "" {
		message = "upstream response failed"
	}
	if code != "" {
		message = code + ": " + message
	}
	t.em.Fail("api_error", message)
}

// flushPendingTools opens function_call items whose name never arrived so
// buffered arguments are still delivered before the message finishes.
func (t *claudeResponsesStreamTranslator) flushPendingTools() {
	for _, tool := range t.toolOrder {
		if !tool.started && (tool.pending.Len() > 0 || tool.callID != "") {
			t.startTool(tool)
		}
	}
}

// finalStopReason returns the Claude stop_reason for the finished message.
func (t *claudeResponsesStreamTranslator) finalStopReason() string {
	return FinalizeClaudeStopReason(t.stopReason, t.em.HasToolUse())
}
