package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The Responses wire contract below is the client-facing shape the gateway
// must emit when it synthesizes Responses output from a non-Responses
// upstream. Required fields come from the official OpenAI OpenAPI spec
// (github.com/openai/openai-openapi main @ 9752a7f, info.version 2.3.0); the
// structural rules come from the parsers of mainstream clients that consume
// this output: the openai-python responses.stream() helper (dense output_index,
// content_index into added parts, added/done identity, every added item done
// before completion), the Vercel AI SDK zod schemas (item ids, function_call
// arguments, custom_tool_call status, output_text annotations) and Codex
// (summary_index on reasoning summary events, phase values, usage details).

// responsesEventRequiredFields lists, per stream event type, the fields that
// must be present besides type and sequence_number (present-but-null counts).
var responsesEventRequiredFields = map[string][]string{
	"response.created":                       {"response"},
	"response.in_progress":                   {"response"},
	"response.completed":                     {"response"},
	"response.incomplete":                    {"response"},
	"response.failed":                        {"response"},
	"response.output_item.added":             {"output_index", "item"},
	"response.output_item.done":              {"output_index", "item"},
	"response.content_part.added":            {"item_id", "output_index", "content_index", "part"},
	"response.content_part.done":             {"item_id", "output_index", "content_index", "part"},
	"response.output_text.delta":             {"item_id", "output_index", "content_index", "delta", "logprobs"},
	"response.output_text.done":              {"item_id", "output_index", "content_index", "text", "logprobs"},
	"response.reasoning_summary_part.added":  {"item_id", "output_index", "summary_index", "part"},
	"response.reasoning_summary_part.done":   {"item_id", "output_index", "summary_index", "part"},
	"response.reasoning_summary_text.delta":  {"item_id", "output_index", "summary_index", "delta"},
	"response.reasoning_summary_text.done":   {"item_id", "output_index", "summary_index", "text"},
	"response.reasoning_text.delta":          {"item_id", "output_index", "content_index", "delta"},
	"response.reasoning_text.done":           {"item_id", "output_index", "content_index", "text"},
	"response.function_call_arguments.delta": {"item_id", "output_index", "delta"},
	"response.function_call_arguments.done":  {"item_id", "output_index", "arguments"},
	"response.custom_tool_call_input.delta":  {"item_id", "output_index", "delta"},
	"response.custom_tool_call_input.done":   {"item_id", "output_index", "input"},
	"response.web_search_call.in_progress":   {"item_id", "output_index"},
	"response.web_search_call.searching":     {"item_id", "output_index"},
	"response.web_search_call.completed":     {"item_id", "output_index"},
	"error":                                  {"message", "code", "param"},
}

// responsesStringFields and responsesIndexFields give the JSON type each
// shared event field must have when present.
var (
	responsesStringFields = map[string]bool{"item_id": true, "text": true, "arguments": true, "input": true, "message": true}
	responsesIndexFields  = map[string]bool{"output_index": true, "content_index": true, "summary_index": true, "sequence_number": true}
)

// responsesWireItem tracks one streamed output item for structural checks.
type responsesWireItem struct {
	kind, id, callID string
	done             bool
	parts, summaries int
	// deltas accumulates streamed text per delta channel ("text:0",
	// "summary:0", "arguments", "input") so done events can be compared.
	deltas map[string]*strings.Builder
}

// accumulate appends one delta to channel and returns the builder.
func (i *responsesWireItem) accumulate(channel, delta string) {
	if i.deltas == nil {
		i.deltas = make(map[string]*strings.Builder)
	}
	if i.deltas[channel] == nil {
		i.deltas[channel] = &strings.Builder{}
	}
	i.deltas[channel].WriteString(delta)
}

// streamed returns the text accumulated on channel and whether any delta arrived.
func (i *responsesWireItem) streamed(channel string) (string, bool) {
	if builder := i.deltas[channel]; builder != nil {
		return builder.String(), true
	}
	return "", false
}

// requireDoneMatchesDeltas checks that a done event's full value equals the
// deltas streamed before it on the same channel. Reasoning summaries compare
// trimmed, because a sealed Claude block's exact text is authoritative.
func (i *responsesWireItem) requireDoneMatchesDeltas(t *testing.T, channel, value, where string, trim bool) {
	t.Helper()
	streamed, ok := i.streamed(channel)
	if !ok {
		return
	}
	if trim {
		streamed, value = strings.TrimSpace(streamed), strings.TrimSpace(value)
	}
	require.Equal(t, streamed, value, "%s: the done value must equal the streamed deltas", where)
}

// responsesWireStream is the decoded state of one Responses SSE body.
type responsesWireStream struct {
	items      []*responsesWireItem
	responseID string
	terminal   string
	completed  map[string]json.RawMessage
}

// requireResponsesStreamConformance validates a Responses SSE body against the
// required-field contract and the structural rules clients rely on. Parameters:
// t owns assertions and body is the raw SSE text. It returns the terminal
// response object so callers can make scenario-specific assertions.
func requireResponsesStreamConformance(t *testing.T, body string) map[string]json.RawMessage {
	t.Helper()
	stream := &responsesWireStream{}
	sequence := 0
	for position, frame := range parseSSEEvents(body) {
		if frame.data == "[DONE]" {
			continue
		}
		var event map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(frame.data), &event), "event %d is not a JSON object: %s", position, frame.data)
		kind := responsesWireString(t, event, "type", frame.data)
		where := fmt.Sprintf("event %d %s", position, kind)
		require.Equal(t, kind, frame.event, "%s: the SSE event name must equal data.type", where)
		require.Empty(t, stream.terminal, "%s: no event may follow %s", where, stream.terminal)
		requireWireKey(t, event, "sequence_number", "%s: sequence_number is required on every event", where)
		require.Equal(t, sequence, responsesWireIndex(t, event, "sequence_number", where), "%s: sequence_number must start at 0 and increase by 1", where)
		sequence++
		required, known := responsesEventRequiredFields[kind]
		if !known {
			continue
		}
		for _, field := range required {
			requireWireKey(t, event, field, "%s: required field %q is missing: %s", where, field, frame.data)
		}
		for field := range event {
			if responsesStringFields[field] && kind != "error" {
				responsesWireString(t, event, field, where)
			}
			if responsesIndexFields[field] {
				responsesWireIndex(t, event, field, where)
			}
		}
		if logprobs, ok := event["logprobs"]; ok {
			require.True(t, strings.HasPrefix(strings.TrimSpace(string(logprobs)), "["), "%s: logprobs must be an array", where)
		}
		stream.apply(t, kind, event, where)
	}
	require.Contains(t, []string{"response.completed", "response.incomplete", "response.failed"}, stream.terminal, "the stream must end with exactly one terminal response event")
	return stream.completed
}

// apply checks one known event against the stream's structural state.
func (s *responsesWireStream) apply(t *testing.T, kind string, event map[string]json.RawMessage, where string) {
	t.Helper()
	switch kind {
	case "response.created", "response.in_progress", "response.completed", "response.incomplete", "response.failed":
		response := requireResponsesObjectConformance(t, event["response"], where)
		id := responsesWireString(t, response, "id", where)
		if kind == "response.created" {
			require.Empty(t, s.responseID, "%s: response.created must be the first lifecycle event", where)
			s.responseID = id
		}
		require.NotEmpty(t, s.responseID, "%s: response.created must come first", where)
		require.Equal(t, s.responseID, id, "%s: lifecycle events must keep the same response id", where)
		if kind != "response.created" && kind != "response.in_progress" {
			s.terminal, s.completed = kind, response
			s.requireTerminalOutput(t, response, kind, where)
		}
	case "response.output_item.added":
		index := responsesWireIndex(t, event, "output_index", where)
		require.Equal(t, len(s.items), index, "%s: output_index must be dense and in the order items are added", where)
		item := requireResponsesItemConformance(t, event["item"], false, where)
		s.items = append(s.items, &responsesWireItem{kind: responsesWireString(t, item, "type", where), id: responsesWireString(t, item, "id", where), callID: responsesWireOptionalString(item, "call_id")})
	case "response.output_item.done":
		tracked := s.open(t, event, "", where)
		item := requireResponsesItemConformance(t, event["item"], true, where)
		require.Equal(t, tracked.kind, responsesWireString(t, item, "type", where), "%s: done must keep the added item type", where)
		require.Equal(t, tracked.id, responsesWireString(t, item, "id", where), "%s: done must keep the added item id", where)
		require.Equal(t, tracked.callID, responsesWireOptionalString(item, "call_id"), "%s: done must keep the added call_id", where)
		tracked.done = true
	case "response.content_part.added":
		tracked := s.openByItemID(t, event, "message", where)
		require.Equal(t, tracked.parts, responsesWireIndex(t, event, "content_index", where), "%s: content_index must address the next part", where)
		requireResponsesContentConformance(t, event["part"], where)
		tracked.parts++
	case "response.content_part.done", "response.output_text.delta", "response.output_text.done":
		tracked := s.openByItemID(t, event, "message", where)
		index := responsesWireIndex(t, event, "content_index", where)
		require.Less(t, index, tracked.parts, "%s: content_index must address an added part", where)
		channel := fmt.Sprintf("text:%d", index)
		switch kind {
		case "response.content_part.done":
			requireResponsesContentConformance(t, event["part"], where)
		case "response.output_text.delta":
			tracked.accumulate(channel, responsesWireString(t, event, "delta", where))
		default:
			tracked.requireDoneMatchesDeltas(t, channel, responsesWireString(t, event, "text", where), where, false)
		}
	case "response.reasoning_summary_part.added":
		tracked := s.openByItemID(t, event, "reasoning", where)
		require.Equal(t, tracked.summaries, responsesWireIndex(t, event, "summary_index", where), "%s: summary_index must address the next summary part", where)
		requireResponsesSummaryPart(t, event["part"], where)
		tracked.summaries++
	case "response.reasoning_summary_part.done", "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done":
		tracked := s.openByItemID(t, event, "reasoning", where)
		index := responsesWireIndex(t, event, "summary_index", where)
		require.Less(t, index, tracked.summaries, "%s: summary_index must address an added summary part", where)
		channel := fmt.Sprintf("summary:%d", index)
		switch kind {
		case "response.reasoning_summary_text.delta":
			tracked.accumulate(channel, responsesWireString(t, event, "delta", where))
		case "response.reasoning_summary_text.done":
			tracked.requireDoneMatchesDeltas(t, channel, responsesWireString(t, event, "text", where), where, true)
		}
	case "response.function_call_arguments.delta", "response.function_call_arguments.done":
		tracked := s.openByItemID(t, event, "function_call", where)
		if kind == "response.function_call_arguments.delta" {
			tracked.accumulate("arguments", responsesWireString(t, event, "delta", where))
		} else {
			tracked.requireDoneMatchesDeltas(t, "arguments", responsesWireString(t, event, "arguments", where), where, false)
		}
	case "response.custom_tool_call_input.delta", "response.custom_tool_call_input.done":
		tracked := s.openByItemID(t, event, "custom_tool_call", where)
		if kind == "response.custom_tool_call_input.delta" {
			tracked.accumulate("input", responsesWireString(t, event, "delta", where))
		} else {
			tracked.requireDoneMatchesDeltas(t, "input", responsesWireString(t, event, "input", where), where, false)
		}
	case "response.web_search_call.in_progress", "response.web_search_call.searching", "response.web_search_call.completed":
		s.openByItemID(t, event, "web_search_call", where)
	}
}

// open returns the not-yet-done item at the event's output_index, optionally
// requiring its type. Parameters: kind is the required item type or "".
func (s *responsesWireStream) open(t *testing.T, event map[string]json.RawMessage, kind, where string) *responsesWireItem {
	t.Helper()
	index := responsesWireIndex(t, event, "output_index", where)
	require.Less(t, index, len(s.items), "%s: output_index %d was never added", where, index)
	tracked := s.items[index]
	require.False(t, tracked.done, "%s: item %d is already done", where, index)
	if kind != "" {
		require.Equal(t, kind, tracked.kind, "%s: event does not match the item type", where)
	}
	return tracked
}

// openByItemID is open plus the rule that item_id names the same item.
func (s *responsesWireStream) openByItemID(t *testing.T, event map[string]json.RawMessage, kind, where string) *responsesWireItem {
	t.Helper()
	tracked := s.open(t, event, kind, where)
	require.Equal(t, tracked.id, responsesWireString(t, event, "item_id", where), "%s: item_id must name the item at output_index", where)
	return tracked
}

// requireTerminalOutput checks that a completed stream finished every added
// item and that response.output lists the same items in output_index order.
func (s *responsesWireStream) requireTerminalOutput(t *testing.T, response map[string]json.RawMessage, kind, where string) {
	t.Helper()
	if kind != "response.completed" {
		return
	}
	var output []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(response["output"], &output), "%s: output", where)
	ids := make([]string, 0, len(output))
	for _, item := range output {
		ids = append(ids, responsesWireString(t, item, "id", where))
	}
	streamed := make([]string, 0, len(s.items))
	for index, tracked := range s.items {
		require.True(t, tracked.done, "%s: item %d (%s) was added but never done", where, index, tracked.kind)
		streamed = append(streamed, tracked.id)
	}
	require.Equal(t, streamed, ids, "%s: response.output must list the streamed items in output_index order", where)
}

// requireResponsesJSONConformance validates a non-streaming Responses body.
// It returns the decoded response object for scenario-specific assertions.
func requireResponsesJSONConformance(t *testing.T, body []byte) map[string]json.RawMessage {
	t.Helper()
	return requireResponsesObjectConformance(t, body, "response body")
}

// requireResponsesObjectConformance validates a Response object and its output
// items. Parameters: raw is the encoded object and where labels failures.
func requireResponsesObjectConformance(t *testing.T, raw json.RawMessage, where string) map[string]json.RawMessage {
	t.Helper()
	var response map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &response), "%s: response must be an object", where)
	for _, field := range []string{"id", "object", "created_at", "model", "output", "parallel_tool_calls", "tools", "tool_choice",
		"error", "incomplete_details", "instructions", "metadata", "temperature", "top_p"} {
		requireWireKey(t, response, field, "%s: response.%s is required (present, possibly null)", where, field)
	}
	require.Equal(t, `"response"`, string(response["object"]), "%s: object", where)
	require.NotEqual(t, "null", string(response["tool_choice"]), "%s: tool_choice must not be null", where)
	var tools []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(response["tools"], &tools), "%s: tools must be an array", where)
	require.NotNil(t, tools, "%s: tools must be an array, not null", where)
	for i, tool := range tools {
		if responsesWireOptionalString(tool, "type") != "function" {
			continue
		}
		// The spec FunctionTool: flat name, plus parameters and strict
		// (present, possibly null); a Chat-shaped {"function":{...}} fails.
		require.NotEmpty(t, responsesWireString(t, tool, "name", where), "%s: tools[%d].name", where, i)
		requireWireKey(t, tool, "parameters", "%s: tools[%d].parameters is required", where, i)
		requireWireKey(t, tool, "strict", "%s: tools[%d].strict is required", where, i)
	}
	var output []json.RawMessage
	require.NoError(t, json.Unmarshal(response["output"], &output), "%s: output must be an array", where)
	require.NotNil(t, output, "%s: output must be an array, not null", where)
	status := responsesWireOptionalString(response, "status")
	for i, item := range output {
		requireResponsesItemConformance(t, item, status != "in_progress", fmt.Sprintf("%s output[%d]", where, i))
	}
	if usage, ok := response["usage"]; ok && string(usage) != "null" {
		requireResponsesUsageConformance(t, usage, where)
	}
	return response
}

// requireResponsesUsageConformance checks the usage block Codex and the SDKs
// decode: the three totals plus both detail objects with their counters.
func requireResponsesUsageConformance(t *testing.T, raw json.RawMessage, where string) {
	t.Helper()
	var usage map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &usage), "%s: usage", where)
	for _, field := range []string{"input_tokens", "output_tokens", "total_tokens"} {
		responsesWireIndex(t, usage, field, where+" usage")
	}
	for detail, counter := range map[string]string{"input_tokens_details": "cached_tokens", "output_tokens_details": "reasoning_tokens"} {
		requireWireKey(t, usage, detail, "%s: usage.%s is required", where, detail)
		var details map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(usage[detail], &details), "%s: usage.%s", where, detail)
		responsesWireIndex(t, details, counter, where+" usage."+detail)
	}
}

// requireResponsesItemConformance validates one output item. Parameters: done
// selects the stricter rules for completed items. Returns the decoded item.
func requireResponsesItemConformance(t *testing.T, raw json.RawMessage, done bool, where string) map[string]json.RawMessage {
	t.Helper()
	var item map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &item), "%s: item must be an object", where)
	kind := responsesWireString(t, item, "type", where)
	where = where + " " + kind
	require.NotEmpty(t, responsesWireString(t, item, "id", where), "%s: every item needs a non-empty id", where)
	status := responsesWireOptionalString(item, "status")
	switch kind {
	case "message":
		require.Equal(t, "assistant", responsesWireString(t, item, "role", where), "%s: role", where)
		require.Contains(t, []string{"in_progress", "completed", "incomplete"}, status, "%s: status", where)
		if phase, ok := item["phase"]; ok && string(phase) != "null" {
			require.Contains(t, []string{`"commentary"`, `"final_answer"`}, string(phase), "%s: phase", where)
		}
		var content []json.RawMessage
		require.NoError(t, json.Unmarshal(item["content"], &content), "%s: content must be an array", where)
		require.NotNil(t, content, "%s: content must be an array", where)
		for _, part := range content {
			requireResponsesContentConformance(t, part, where)
		}
	case "reasoning":
		var summary []json.RawMessage
		require.NoError(t, json.Unmarshal(item["summary"], &summary), "%s: summary must be an array", where)
		require.NotNil(t, summary, "%s: summary must be an array", where)
		for _, part := range summary {
			requireResponsesSummaryPart(t, part, where)
		}
		if content, ok := item["content"]; ok && string(content) != "null" {
			var parts []map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(content, &parts), "%s: content", where)
			for _, part := range parts {
				require.Equal(t, "reasoning_text", responsesWireString(t, part, "type", where), "%s: reasoning content parts are reasoning_text", where)
				responsesWireString(t, part, "text", where)
			}
		}
	case "function_call", "custom_tool_call":
		require.NotEmpty(t, responsesWireString(t, item, "call_id", where), "%s: call_id", where)
		require.NotEmpty(t, responsesWireString(t, item, "name", where), "%s: name", where)
		field := map[string]string{"function_call": "arguments", "custom_tool_call": "input"}[kind]
		responsesWireString(t, item, field, where)
		if done {
			allowed := []string{"in_progress", "completed", "incomplete"}
			if kind == "custom_tool_call" {
				allowed = []string{"completed", "incomplete"}
			}
			require.Contains(t, allowed, status, "%s: status of a finished call", where)
		}
	case "web_search_call":
		require.Contains(t, []string{"in_progress", "searching", "completed", "failed", "incomplete"}, status, "%s: status", where)
		if done {
			requireWireKey(t, item, "action", "%s: finished web_search_call needs action", where)
		}
	case "tool_search_call":
		requireWireKey(t, item, "call_id", "%s: call_id is required (nullable)", where)
		requireWireKey(t, item, "arguments", "%s: arguments is required", where)
		require.Contains(t, []string{`"client"`, `"server"`}, string(item["execution"]), "%s: execution", where)
		require.Contains(t, []string{"in_progress", "completed", "incomplete"}, status, "%s: status", where)
	}
	return item
}

// requireResponsesContentConformance validates one message content part.
func requireResponsesContentConformance(t *testing.T, raw json.RawMessage, where string) {
	t.Helper()
	var part map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &part), "%s: content part", where)
	switch kind := responsesWireString(t, part, "type", where); kind {
	case "output_text":
		responsesWireString(t, part, "text", where+" output_text")
		for _, field := range []string{"annotations", "logprobs"} {
			requireWireKey(t, part, field, "%s: output_text.%s is required", where, field)
			require.True(t, strings.HasPrefix(strings.TrimSpace(string(part[field])), "["), "%s: output_text.%s must be an array", where, field)
		}
	case "refusal":
		responsesWireString(t, part, "refusal", where+" refusal")
	default:
		require.Failf(t, "unexpected content part", "%s: message content part type %q", where, kind)
	}
}

// requireResponsesSummaryPart validates one reasoning summary part.
func requireResponsesSummaryPart(t *testing.T, raw json.RawMessage, where string) {
	t.Helper()
	var part map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &part), "%s: summary part", where)
	require.Equal(t, "summary_text", responsesWireString(t, part, "type", where), "%s: summary part type", where)
	responsesWireString(t, part, "text", where+" summary_text")
}

// responsesWireString returns a required string field, failing when it is
// absent or not a string.
func responsesWireString(t *testing.T, object map[string]json.RawMessage, field, where string) string {
	t.Helper()
	raw, ok := object[field]
	require.True(t, ok, "%s: %q is required", where, field)
	var value string
	require.NoError(t, json.Unmarshal(raw, &value), "%s: %q must be a string, got %s", where, field, raw)
	return value
}

// responsesWireOptionalString returns a string field or "" when absent/null.
func responsesWireOptionalString(object map[string]json.RawMessage, field string) string {
	var value string
	if raw, ok := object[field]; ok {
		_ = json.Unmarshal(raw, &value)
	}
	return value
}

// responsesWireIndex returns a required non-negative integer field.
func responsesWireIndex(t *testing.T, object map[string]json.RawMessage, field, where string) int {
	t.Helper()
	raw, ok := object[field]
	require.True(t, ok, "%s: %q is required", where, field)
	var value int
	require.NoError(t, json.Unmarshal(raw, &value), "%s: %q must be an integer, got %s", where, field, raw)
	require.GreaterOrEqual(t, value, 0, "%s: %q must be non-negative", where, field)
	return value
}

// requireWireKey fails unless object has key (a present null counts). It keeps
// failure output short instead of printing the whole decoded object.
func requireWireKey(t *testing.T, object map[string]json.RawMessage, key string, msgAndArgs ...any) {
	t.Helper()
	if _, ok := object[key]; !ok {
		require.Fail(t, fmt.Sprintf("missing required key %q", key), msgAndArgs...)
	}
}

// requireResponsesRecorderConformance validates a successful Responses reply
// recorded by a fallback harness: SSE bodies against the stream contract and
// JSON bodies against the object contract. Error replies are left to the
// caller's own assertions. Parameters: t owns assertions, recorder the reply.
func requireResponsesRecorderConformance(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder == nil || recorder.Code != http.StatusOK {
		return
	}
	// A declared length must describe the body actually written; a stale
	// upstream Content-Length makes real HTTP servers abort the response.
	if declared := recorder.Header().Get("Content-Length"); declared != "" {
		require.Equal(t, strconv.Itoa(recorder.Body.Len()), declared, "Content-Length must match the rendered Responses body")
	}
	if strings.Contains(recorder.Header().Get("Content-Type"), "text/event-stream") {
		requireResponsesStreamConformance(t, recorder.Body.String())
		requireCodexActiveItemStreaming(t, recorder.Body.String())
		return
	}
	requireResponsesJSONConformance(t, recorder.Body.Bytes())
}

// requireCodexActiveItemStreaming replays a Responses SSE body the way Codex
// consumes it: output_item.added makes an item active, output_item.done clears
// it, and text or reasoning summary deltas are only shown while an item of the
// matching type is active (Codex logs "OutputTextDelta without active item"
// and drops the delta otherwise). It fails when any streamed text or summary
// delta would be dropped, when a new item starts while a message or reasoning
// item is still open (Codex would record them out of order), and when a
// completed message item carries no text (Codex would show an empty message).
func requireCodexActiveItemStreaming(t *testing.T, body string) {
	t.Helper()
	active := ""
	// openTextual holds message and reasoning items announced but not done;
	// a new item must not start while one is open, or Codex records the
	// items out of the order the model produced them.
	openTextual := map[string]bool{}
	for position, frame := range parseSSEEvents(body) {
		if frame.data == "[DONE]" {
			continue
		}
		var event struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
			Item  struct {
				ID      string `json:"id"`
				Type    string `json:"type"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"item"`
		}
		require.NoError(t, json.Unmarshal([]byte(frame.data), &event))
		switch event.Type {
		case "response.output_item.added":
			require.Empty(t, openTextual, "event %d: %s starts while a message or reasoning item is still open", position, event.Item.Type)
			if event.Item.Type == "message" || event.Item.Type == "reasoning" {
				openTextual[event.Item.ID] = true
			}
			active = event.Item.Type
		case "response.output_item.done":
			delete(openTextual, event.Item.ID)
			if event.Item.Type == "message" {
				text := ""
				for _, part := range event.Item.Content {
					text += part.Text
				}
				require.NotEmpty(t, text, "event %d: a completed message item must carry text", position)
			}
			active = ""
		case "response.output_text.delta":
			require.Equal(t, "message", active, "event %d: Codex drops a text delta unless a message item is active", position)
		case "response.reasoning_summary_text.delta":
			require.Equal(t, "reasoning", active, "event %d: Codex drops a summary delta unless a reasoning item is active", position)
		}
	}
}
