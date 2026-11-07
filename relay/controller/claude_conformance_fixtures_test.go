package controller

import (
	"encoding/json"
	"strings"
)

// claudeConformanceScenario names one upstream answer shape driven through
// /v1/messages: a plain text answer, two parallel tool calls, a max-token
// truncation, and a reasoning-then-text answer.
type claudeConformanceScenario string

const (
	conformanceText      claudeConformanceScenario = "text"
	conformanceTools     claudeConformanceScenario = "tools"
	conformanceLength    claudeConformanceScenario = "length"
	conformanceReasoning claudeConformanceScenario = "reasoning"
)

// conformanceChatChunk renders one OpenAI Chat Completions stream chunk.
func conformanceChatChunk(choices string) string {
	return `data: {"id":"chatcmpl-conf","object":"chat.completion.chunk","created":1,"model":"upstream-model","choices":[` + choices + `]}` + "\n\n"
}

// conformanceChatStream returns a realistic OpenAI Chat Completions SSE body:
// tool call fragments after the first carry only their index (no id/name), the
// finish_reason chunk precedes a choices-less usage chunk, and [DONE] ends it.
func conformanceChatStream(s claudeConformanceScenario) string {
	var sb strings.Builder
	sb.WriteString(conformanceChatChunk(`{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}`))
	finish := "stop"
	switch s {
	case conformanceText, conformanceLength:
		sb.WriteString(conformanceChatChunk(`{"index":0,"delta":{"content":"Hello"},"finish_reason":null}`))
		sb.WriteString(conformanceChatChunk(`{"index":0,"delta":{"content":" world"},"finish_reason":null}`))
		if s == conformanceLength {
			finish = "length"
		}
	case conformanceReasoning:
		sb.WriteString(conformanceChatChunk(`{"index":0,"delta":{"reasoning_content":"Thinking"},"finish_reason":null}`))
		sb.WriteString(conformanceChatChunk(`{"index":0,"delta":{"reasoning_content":" hard"},"finish_reason":null}`))
		sb.WriteString(conformanceChatChunk(`{"index":0,"delta":{"content":"Hello"},"finish_reason":null}`))
		sb.WriteString(conformanceChatChunk(`{"index":0,"delta":{"content":" world"},"finish_reason":null}`))
	case conformanceTools:
		sb.WriteString(conformanceChatChunk(`{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}`))
		sb.WriteString(conformanceChatChunk(`{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]},"finish_reason":null}`))
		sb.WriteString(conformanceChatChunk(`{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}}]},"finish_reason":null}`))
		sb.WriteString(conformanceChatChunk(`{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"get_time","arguments":""}}]},"finish_reason":null}`))
		sb.WriteString(conformanceChatChunk(`{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"tz\":\"CET\"}"}}]},"finish_reason":null}`))
		finish = "tool_calls"
	}
	sb.WriteString(conformanceChatChunk(`{"index":0,"delta":{},"finish_reason":"` + finish + `"}`))
	sb.WriteString(`data: {"id":"chatcmpl-conf","object":"chat.completion.chunk","created":1,"model":"upstream-model","choices":[],"usage":{"prompt_tokens":9,"completion_tokens":2,"total_tokens":11}}` + "\n\n")
	sb.WriteString("data: [DONE]\n\n")
	return sb.String()
}

// conformanceChatJSON returns a non-streaming Chat Completions body.
func conformanceChatJSON(s claudeConformanceScenario) string {
	message := map[string]any{"role": "assistant", "content": "Hello world"}
	finish := "stop"
	switch s {
	case conformanceLength:
		finish = "length"
	case conformanceReasoning:
		message["reasoning_content"] = "Thinking hard"
	case conformanceTools:
		message["content"] = nil
		message["tool_calls"] = []any{
			map[string]any{"id": "call_a", "type": "function", "function": map[string]any{"name": "get_weather", "arguments": `{"city":"Paris"}`}},
			map[string]any{"id": "call_b", "type": "function", "function": map[string]any{"name": "get_time", "arguments": `{"tz":"CET"}`}},
		}
		finish = "tool_calls"
	}
	body, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-conf", "object": "chat.completion", "created": 1, "model": "upstream-model",
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}},
		"usage":   map[string]any{"prompt_tokens": 9, "completion_tokens": 2, "total_tokens": 11},
	})
	return string(body)
}

// conformanceResponsesEvent renders one Responses API SSE event with the
// upstream's own event: line, as OpenAI emits it.
func conformanceResponsesEvent(eventType, body string) string {
	return "event: " + eventType + "\ndata: {\"type\":\"" + eventType + "\"," + body + "}\n\n"
}

// conformanceResponsesOutput returns the final Responses output items.
func conformanceResponsesOutput(s claudeConformanceScenario) string {
	message := `{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","annotations":[],"text":"Hello world"}]}`
	switch s {
	case conformanceTools:
		return `[{"id":"fc_1","type":"function_call","status":"completed","arguments":"{\"city\":\"Paris\"}","call_id":"call_a","name":"get_weather"},` +
			`{"id":"fc_2","type":"function_call","status":"completed","arguments":"{\"tz\":\"CET\"}","call_id":"call_b","name":"get_time"}]`
	case conformanceReasoning:
		return `[{"id":"rs_1","type":"reasoning","summary":[{"type":"summary_text","text":"Thinking hard"}]},` + message + `]`
	case conformanceLength:
		return `[{"id":"msg_1","type":"message","status":"incomplete","role":"assistant","content":[{"type":"output_text","annotations":[],"text":"Hello world"}]}]`
	default:
		return `[` + message + `]`
	}
}

// conformanceResponsesObject returns a terminal Responses object.
func conformanceResponsesObject(s claudeConformanceScenario) string {
	status, incomplete := "completed", "null"
	if s == conformanceLength {
		status, incomplete = "incomplete", `{"reason":"max_output_tokens"}`
	}
	return `{"id":"resp_conf","object":"response","created_at":1,"status":"` + status + `","incomplete_details":` + incomplete +
		`,"model":"gpt-4o","output":` + conformanceResponsesOutput(s) +
		`,"usage":{"input_tokens":9,"input_tokens_details":{"cached_tokens":0},"output_tokens":2,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":11}}`
}

// conformanceResponsesText appends the full message item event sequence.
func conformanceResponsesText(sb *strings.Builder, outputIndex string) {
	at := `"item_id":"msg_1","output_index":` + outputIndex + `,"content_index":0`
	sb.WriteString(conformanceResponsesEvent("response.output_item.added", `"output_index":`+outputIndex+`,"item":{"id":"msg_1","type":"message","status":"in_progress","role":"assistant","content":[]}`))
	sb.WriteString(conformanceResponsesEvent("response.content_part.added", at+`,"part":{"type":"output_text","annotations":[],"text":""}`))
	sb.WriteString(conformanceResponsesEvent("response.output_text.delta", at+`,"delta":"Hello"`))
	sb.WriteString(conformanceResponsesEvent("response.output_text.delta", at+`,"delta":" world"`))
	sb.WriteString(conformanceResponsesEvent("response.output_text.done", at+`,"text":"Hello world"`))
	sb.WriteString(conformanceResponsesEvent("response.content_part.done", at+`,"part":{"type":"output_text","annotations":[],"text":"Hello world"}`))
	sb.WriteString(conformanceResponsesEvent("response.output_item.done", `"output_index":`+outputIndex+`,"item":{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","annotations":[],"text":"Hello world"}]}`))
}

// conformanceResponsesTool appends the full function_call item event sequence.
func conformanceResponsesTool(sb *strings.Builder, outputIndex, itemID, callID, name string, fragments ...string) {
	at := `"item_id":"` + itemID + `","output_index":` + outputIndex
	sb.WriteString(conformanceResponsesEvent("response.output_item.added", `"output_index":`+outputIndex+`,"item":{"id":"`+itemID+`","type":"function_call","status":"in_progress","arguments":"","call_id":"`+callID+`","name":"`+name+`"}`))
	full := strings.Join(fragments, "")
	for _, fragment := range fragments {
		quoted, _ := json.Marshal(fragment)
		sb.WriteString(conformanceResponsesEvent("response.function_call_arguments.delta", at+`,"delta":`+string(quoted)))
	}
	quotedFull, _ := json.Marshal(full)
	sb.WriteString(conformanceResponsesEvent("response.function_call_arguments.done", at+`,"arguments":`+string(quotedFull)))
	sb.WriteString(conformanceResponsesEvent("response.output_item.done", `"output_index":`+outputIndex+`,"item":{"id":"`+itemID+`","type":"function_call","status":"completed","arguments":`+string(quotedFull)+`,"call_id":"`+callID+`","name":"`+name+`"}`))
}

// conformanceResponsesStream returns a realistic OpenAI Responses SSE body.
// Responses streams terminate with response.completed/response.incomplete and
// never send a [DONE] sentinel.
func conformanceResponsesStream(s claudeConformanceScenario) string {
	var sb strings.Builder
	inProgress := `"response":{"id":"resp_conf","object":"response","created_at":1,"status":"in_progress","model":"gpt-4o","output":[],"usage":null}`
	sb.WriteString(conformanceResponsesEvent("response.created", `"sequence_number":0,`+inProgress))
	sb.WriteString(conformanceResponsesEvent("response.in_progress", `"sequence_number":1,`+inProgress))
	switch s {
	case conformanceTools:
		conformanceResponsesTool(&sb, "0", "fc_1", "call_a", "get_weather", `{"city":`, `"Paris"}`)
		conformanceResponsesTool(&sb, "1", "fc_2", "call_b", "get_time", `{"tz":"CET"}`)
	case conformanceReasoning:
		at := `"item_id":"rs_1","output_index":0,"summary_index":0`
		sb.WriteString(conformanceResponsesEvent("response.output_item.added", `"output_index":0,"item":{"id":"rs_1","type":"reasoning","summary":[]}`))
		sb.WriteString(conformanceResponsesEvent("response.reasoning_summary_part.added", at+`,"part":{"type":"summary_text","text":""}`))
		sb.WriteString(conformanceResponsesEvent("response.reasoning_summary_text.delta", at+`,"delta":"Thinking"`))
		sb.WriteString(conformanceResponsesEvent("response.reasoning_summary_text.delta", at+`,"delta":" hard"`))
		sb.WriteString(conformanceResponsesEvent("response.reasoning_summary_text.done", at+`,"text":"Thinking hard"`))
		sb.WriteString(conformanceResponsesEvent("response.reasoning_summary_part.done", at+`,"part":{"type":"summary_text","text":"Thinking hard"}`))
		sb.WriteString(conformanceResponsesEvent("response.output_item.done", `"output_index":0,"item":{"id":"rs_1","type":"reasoning","summary":[{"type":"summary_text","text":"Thinking hard"}]}`))
		conformanceResponsesText(&sb, "1")
	default:
		conformanceResponsesText(&sb, "0")
	}
	terminal := "response.completed"
	if s == conformanceLength {
		terminal = "response.incomplete"
	}
	sb.WriteString(conformanceResponsesEvent(terminal, `"response":`+conformanceResponsesObject(s)))
	return sb.String()
}

// conformanceGeminiParts returns the Gemini candidate parts for a scenario.
func conformanceGeminiParts(s claudeConformanceScenario) []string {
	if s == conformanceTools {
		return []string{`{"functionCall":{"name":"get_weather","args":{"city":"Paris"}}}`, `{"functionCall":{"name":"get_time","args":{"tz":"CET"}}}`}
	}
	return []string{`{"text":"Hello"}`, `{"text":" world"}`}
}

// conformanceGeminiFinish maps a scenario to Gemini's finishReason. Gemini
// reports STOP for function-call turns.
func conformanceGeminiFinish(s claudeConformanceScenario) string {
	if s == conformanceLength {
		return "MAX_TOKENS"
	}
	return "STOP"
}

// conformanceGeminiUsage is the Gemini usageMetadata receipt.
const conformanceGeminiUsage = `"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":2,"totalTokenCount":11}`

// conformanceGeminiStream returns a Gemini streamGenerateContent?alt=sse body:
// one chunk per part, the last carrying finishReason and usage, no [DONE].
func conformanceGeminiStream(s claudeConformanceScenario) string {
	var sb strings.Builder
	parts := conformanceGeminiParts(s)
	for i, part := range parts {
		finish := ""
		if i == len(parts)-1 {
			finish = `,"finishReason":"` + conformanceGeminiFinish(s) + `"`
		}
		sb.WriteString(`data: {"candidates":[{"content":{"parts":[` + part + `],"role":"model"},"index":0` + finish + `}],` + conformanceGeminiUsage + `,"modelVersion":"gemini-2.5-flash","responseId":"gem-conf"}` + "\r\n\r\n")
	}
	return sb.String()
}

// conformanceGeminiJSON returns a non-streaming generateContent body.
func conformanceGeminiJSON(s claudeConformanceScenario) string {
	return `{"candidates":[{"content":{"parts":[` + strings.Join(conformanceGeminiParts(s), ",") + `],"role":"model"},"finishReason":"` +
		conformanceGeminiFinish(s) + `","index":0}],` + conformanceGeminiUsage + `,"modelVersion":"gemini-2.5-flash","responseId":"gem-conf"}`
}
