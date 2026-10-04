package model

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// appendText retains a bounded prefix for tokenization while counting all text
// bytes. snapshot distinguishes complete output from incremental deltas.
func (a *ResponseUsageAccumulator) appendText(text string, snapshot bool) {
	builder, size := &a.text, &a.textBytes
	if snapshot {
		builder, size = &a.snapshot, &a.snapshotBytes
	}
	*size += len(text)
	remaining := responseUsageTextLimit - builder.Len()
	if len(text) > remaining {
		text = text[:remaining]
		for len(text) > 0 && !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	builder.WriteString(text)
}

// consumeOutput extracts billable text, reasoning, and tool arguments without
// adding terminal response snapshots to their already observed streamed deltas.
func (a *ResponseUsageAccumulator) consumeOutput(root map[string]json.RawMessage, snapshot bool) {
	if snapshot && (root["choices"] != nil || root["content"] != nil || root["output"] != nil) {
		a.snapshot.Reset()
		a.snapshotBytes = 0
	}
	var choices []struct {
		Text    string                     `json:"text"`
		Message map[string]json.RawMessage `json:"message"`
		Delta   map[string]json.RawMessage `json:"delta"`
	}
	if json.Unmarshal(root["choices"], &choices) == nil {
		for _, choice := range choices {
			a.appendText(choice.Text, snapshot)
			a.consumeChatContent(choice.Message, snapshot)
			a.consumeChatContent(choice.Delta, snapshot)
		}
	}
	a.consumeBlocks(root["content"], snapshot)
	var output []map[string]json.RawMessage
	if json.Unmarshal(root["output"], &output) == nil {
		for outputIndex, item := range output {
			a.consumeResponseBlocks(item, outputIndex, snapshot)
			if responseUsageString(item["type"]) == "function_call" {
				a.appendText(responseUsageString(item["name"]), snapshot)
				a.appendText(responseUsageJSONText(item["arguments"]), snapshot)
			}
		}
	}
	kind := responseUsageString(root["type"])
	switch kind {
	case "content_block_delta":
		var delta map[string]json.RawMessage
		if json.Unmarshal(root["delta"], &delta) == nil {
			for _, key := range []string{"text", "thinking", "partial_json"} {
				a.appendText(responseUsageString(delta[key]), false)
			}
		}
	case "content_block_start":
		var block map[string]json.RawMessage
		if json.Unmarshal(root["content_block"], &block) == nil {
			a.consumeBlock(block, false)
		}
	case "response.output_json.delta":
		text := responseUsageString(root["delta"])
		if text == "" {
			var delta map[string]json.RawMessage
			if json.Unmarshal(root["delta"], &delta) == nil {
				for _, key := range []string{"partial_json", "json", "text"} {
					if text = responseUsageJSONText(delta[key]); text != "" {
						break
					}
				}
			}
			if text == "" {
				text = strings.TrimSpace(string(root["delta"]))
			}
		}
		a.observeJSONText(root, text, false)
	case "response.output_json.done":
		text := responseUsageRawJSON(root["json"])
		if text == "" {
			for _, key := range []string{"part", "output", "text", "delta"} {
				if text = responseUsageJSONPayload(root[key]); text != "" {
					break
				}
			}
		}
		a.observeJSONText(root, text, true)
	case "response.output_text.delta", "response.function_call_arguments.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
		a.appendText(responseUsageString(root["delta"]), false)
	}
}

// consumeChatContent reads OpenAI message/delta content, refusal, reasoning,
// legacy function calls, and modern tool calls into the selected text buffer.
func (a *ResponseUsageAccumulator) consumeChatContent(message map[string]json.RawMessage, snapshot bool) {
	for _, key := range []string{"content", "refusal"} {
		if text := responseUsageString(message[key]); text != "" {
			a.appendText(text, snapshot)
		} else {
			a.consumeBlocks(message[key], snapshot)
		}
	}
	// These are wire aliases for reasoning. Count identical aliases once,
	// while retaining distinct pieces rather than selecting only one field.
	seen := map[string]struct{}{}
	for _, key := range []string{"reasoning_content", "reasoning", "thinking"} {
		if text := responseUsageString(message[key]); text != "" {
			if _, exists := seen[text]; exists {
				continue
			}
			seen[text] = struct{}{}
			a.appendText(text, snapshot)
		} else {
			a.consumeBlocks(message[key], snapshot)
		}
	}
	var calls []struct {
		Function map[string]json.RawMessage `json:"function"`
	}
	if json.Unmarshal(message["tool_calls"], &calls) == nil {
		for _, call := range calls {
			a.appendText(responseUsageString(call.Function["name"]), snapshot)
			a.appendText(responseUsageJSONText(call.Function["arguments"]), snapshot)
		}
	}
	var function map[string]json.RawMessage
	if json.Unmarshal(message["function_call"], &function) == nil {
		a.appendText(responseUsageString(function["name"]), snapshot)
		a.appendText(responseUsageJSONText(function["arguments"]), snapshot)
	}
}

// consumeBlocks handles array-shaped text and Claude content blocks.
func (a *ResponseUsageAccumulator) consumeBlocks(raw json.RawMessage, snapshot bool) {
	var blocks []map[string]json.RawMessage
	if json.Unmarshal(raw, &blocks) != nil {
		return
	}
	for _, block := range blocks {
		a.consumeBlock(block, snapshot)
	}
}

// consumeBlock extracts text, thinking, or tool-use input from one content block.
func (a *ResponseUsageAccumulator) consumeBlock(block map[string]json.RawMessage, snapshot bool) {
	if responseUsageString(block["type"]) == "output_json" && len(block["json"]) > 0 {
		a.appendText(responseUsageRawJSON(block["json"]), snapshot)
	} else {
		a.appendText(responseUsageString(block["text"]), snapshot)
	}
	a.appendText(responseUsageString(block["thinking"]), snapshot)
	if responseUsageString(block["type"]) == "tool_use" {
		a.appendText(responseUsageString(block["name"]), snapshot)
		if input := string(block["input"]); input != "{}" && input != "null" {
			a.appendText(input, snapshot)
		}
	}
}

// responseUsageRawJSON preserves application JSON syntax at explicit JSON leaf fields.
// Unlike protocol strings and tool arguments, quotes, null, and numeric lexemes are output text.
func responseUsageRawJSON(raw json.RawMessage) string {
	return strings.TrimSpace(string(raw))
}

// responseUsageJSONText returns string content or compact structured JSON, matching
// the converter's object argument serialization. Absent and null values add no text.
func responseUsageJSONText(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// responseUsageJSONPayload extracts JSON from the structured wrappers accepted
// by Responses JSON completion events, falling back to the entire JSON value.
func responseUsageJSONPayload(raw json.RawMessage) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) == nil {
		if rawJSON, ok := fields["json"]; ok {
			return responseUsageRawJSON(rawJSON)
		}
		for _, key := range []string{"text", "content", "partial_json"} {
			if nested, ok := fields[key]; ok {
				if text := responseUsageJSONText(nested); text != "" {
					return text
				}
			}
		}
	}
	return responseUsageJSONText(raw)
}
