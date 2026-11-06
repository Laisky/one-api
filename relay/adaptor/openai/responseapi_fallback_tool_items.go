package openai

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

// responseEmptyToolOutputText stands in for an empty tool output that must be
// carried as a standalone message, which Claude requires to be nonempty.
const responseEmptyToolOutputText = "(empty tool output)"

// responseFallbackToolSummaryLimit bounds each loaded-tool line in a lowered
// tool_search_output so a large catalog cannot dominate the tool result text.
const responseFallbackToolSummaryLimit = 160

// lowerResponseFallbackToolItems rewrites Responses history items that have no
// Chat Completions counterpart into the function_call / function_call_output /
// message items the fallback converter already lowers, using catalog's flat
// names. It returns the rewritten input, the effective reasoning effort carried
// by the last configuration_update item ("" when none), or an error when an
// item cannot be represented faithfully. The original input is never mutated.
func lowerResponseFallbackToolItems(input ResponseAPIInput, catalog *ResponseFallbackToolCatalog) (ResponseAPIInput, string, error) {
	if len(input) == 0 {
		return input, "", nil
	}
	discovery, err := responseFallbackToolSearchItems(input)
	if err != nil {
		return nil, "", errors.Wrap(err, "inspect response discovery history")
	}
	byIndex := make(map[int]ResponseToolSearchInputItem, len(discovery))
	for _, item := range discovery {
		byIndex[item.Index] = item
	}

	lowered := make(ResponseAPIInput, 0, len(input))
	effort := ""
	for index, raw := range input {
		if discovered, ok := byIndex[index]; ok {
			item, err := lowerResponseDiscoveryItem(discovered, catalog)
			if err != nil {
				return nil, "", err
			}
			if item != nil {
				lowered = append(lowered, item)
			}
			continue
		}
		item, ok := raw.(map[string]any)
		if !ok {
			lowered = append(lowered, raw)
			continue
		}
		kind, _ := item["type"].(string)
		switch strings.ToLower(strings.TrimSpace(kind)) {
		case "function_call":
			namespace, _ := item["namespace"].(string)
			name, _ := item["name"].(string)
			_, hasNamespace := item["namespace"]
			flat := catalog.FlatName(namespace, name)
			if !hasNamespace && flat == name {
				lowered = append(lowered, raw)
				continue
			}
			clone := cloneResponseInputItem(item)
			delete(clone, "namespace")
			clone["name"] = flat
			lowered = append(lowered, clone)
		case "function_call_output", "custom_tool_call_output":
			clone := cloneResponseInputItem(item)
			clone["type"] = "function_call_output"
			delete(clone, "namespace")
			delete(clone, "name")
			lowered = append(lowered, clone)
		case "custom_tool_call":
			namespace, _ := item["namespace"].(string)
			name, _ := item["name"].(string)
			arguments, err := json.Marshal(map[string]any{responseCustomToolInputArgument: item["input"]})
			if err != nil {
				return nil, "", errors.Wrap(err, "encode custom tool input")
			}
			lowered = append(lowered, responseLoweredFunctionCall(item, catalog.FlatName(namespace, name), string(arguments)))
		case "agent_message":
			message, err := lowerResponseAgentMessage(item)
			if err != nil {
				return nil, "", err
			}
			lowered = append(lowered, message)
		case "web_search_call":
			// Replayed hosted search calls have no Chat or Claude counterpart;
			// dropping them keeps the surrounding assistant turn contiguous.
			continue
		case "configuration_update":
			if reasoning, ok := item["reasoning"].(map[string]any); ok {
				if value, ok := reasoning["effort"].(string); ok && strings.TrimSpace(value) != "" {
					value = strings.ToLower(strings.TrimSpace(value))
					if !validResponseReasoningEffort(value) {
						return nil, "", errors.Wrap(ErrResponseFallbackNotRepresentable, "configuration_update carries an unknown reasoning effort")
					}
					effort = value
				}
			}
		default:
			lowered = append(lowered, raw)
		}
	}
	return lowered, effort, nil
}

// lowerResponseDiscoveryItem lowers one raw-preserved discovery item: a client
// tool_search_call becomes a call to the flattened search tool, its output a
// tool result naming the loaded tools, and additional_tools is consumed by the
// catalog. It returns nil for items with no conversational content.
func lowerResponseDiscoveryItem(item ResponseToolSearchInputItem, catalog *ResponseFallbackToolCatalog) (map[string]any, error) {
	var envelope struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(item.Raw, &envelope); err != nil {
		return nil, errors.Wrap(err, "decode discovery item envelope")
	}
	fields := map[string]any{"call_id": item.CallID}
	if envelope.ID != "" {
		fields["id"] = envelope.ID
	}
	if envelope.Status != "" {
		fields["status"] = envelope.Status
	}
	switch strings.ToLower(strings.TrimSpace(item.Type)) {
	case "tool_search_call":
		arguments := bytes.TrimSpace(item.Arguments)
		if len(arguments) == 0 {
			arguments = []byte("{}")
		}
		return responseLoweredFunctionCall(fields, catalog.SearchFlatName(), string(arguments)), nil
	case "tool_search_output":
		output := map[string]any{
			"type":    "function_call_output",
			"call_id": item.CallID,
			"output":  summarizeResponseLoadedTools(item.Tools, catalog),
		}
		if envelope.ID != "" {
			output["id"] = envelope.ID
		}
		return output, nil
	default:
		// additional_tools: the catalog already carries these definitions.
		return nil, nil
	}
}

// responseLoweredFunctionCall builds a function_call item that keeps the
// original item and call identifiers so the converter pairs it with its output.
func responseLoweredFunctionCall(item map[string]any, name, arguments string) map[string]any {
	lowered := map[string]any{
		"type":      "function_call",
		"name":      name,
		"arguments": arguments,
	}
	for _, key := range []string{"id", "call_id", "status"} {
		if value, ok := item[key]; ok {
			lowered[key] = value
		}
	}
	return lowered
}

// cloneResponseInputItem returns a shallow copy of item for key rewrites.
func cloneResponseInputItem(item map[string]any) map[string]any {
	clone := make(map[string]any, len(item))
	for key, value := range item {
		clone[key] = value
	}
	return clone
}

// summarizeResponseLoadedTools renders the tool result text for a lowered
// tool_search_output: which flattened tools became callable. Definitions
// themselves travel in the request tool list built by the catalog.
func summarizeResponseLoadedTools(tools []ResponseAPITool, catalog *ResponseFallbackToolCatalog) string {
	var lines []string
	appendLine := func(namespace string, tool ResponseAPITool) {
		name := tool.Name
		description := tool.Description
		if tool.Function != nil {
			if tool.Function.Name != "" {
				name = tool.Function.Name
			}
			if tool.Function.Description != "" {
				description = tool.Function.Description
			}
		}
		if strings.TrimSpace(name) == "" {
			return
		}
		line := "- " + catalog.FlatName(namespace, name)
		if summary := firstResponseLine(description); summary != "" {
			line += ": " + summary
		}
		lines = append(lines, line)
	}
	for _, tool := range tools {
		switch strings.ToLower(strings.TrimSpace(tool.Type)) {
		case "namespace":
			for _, child := range tool.Tools {
				appendLine(tool.Name, child)
			}
		case "function", "custom":
			appendLine("", tool)
		}
	}
	if len(lines) == 0 {
		return "Tool search found no matching tools."
	}
	return "Tool search loaded " + strconv.Itoa(len(lines)) + " tool(s); they are now available to call:\n" + strings.Join(lines, "\n")
}

// firstResponseLine returns the first nonempty line of text, bounded in length.
func firstResponseLine(text string) string {
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > responseFallbackToolSummaryLimit {
			cut := responseFallbackToolSummaryLimit
			for cut > 0 && (line[cut]&0xC0) == 0x80 {
				cut--
			}
			line = line[:cut] + "…"
		}
		return line
	}
	return ""
}

// lowerResponseAgentMessage turns a multi-agent agent_message item into a user
// message that keeps its author and recipient. Encrypted agent content is bound
// to the provider that produced it and cannot be represented on fallback.
func lowerResponseAgentMessage(item map[string]any) (map[string]any, error) {
	author, _ := item["author"].(string)
	recipient, _ := item["recipient"].(string)
	var texts []string
	parts, _ := item["content"].([]any)
	for _, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok {
			return nil, errors.Wrap(ErrResponseFallbackNotRepresentable, "invalid agent message content")
		}
		switch kind, _ := part["type"].(string); kind {
		case "input_text", "text", "output_text":
			text, _ := part["text"].(string)
			texts = append(texts, text)
		default:
			return nil, errors.Wrap(ErrResponseFallbackNotRepresentable, "encrypted or non-text agent message content requires native Responses support")
		}
	}
	header := "[Agent message"
	if author != "" {
		header += " from " + author
	}
	if recipient != "" {
		header += " to " + recipient
	}
	header += "]"
	return map[string]any{
		"type":    "message",
		"role":    "user",
		"content": []any{map[string]any{"type": "input_text", "text": header + "\n" + strings.Join(texts, "\n")}},
	}, nil
}

// ResponseFallbackToolCallItem renders one upstream Chat tool call as the
// Responses output item its declaring definition expects: function_call (with
// namespace), custom_tool_call (raw input), or a client tool_search_call
// (object arguments). Unknown names render as plain function_call items.
// Parameters: callID is the normalized Responses call ID, flatName the
// upstream tool name, arguments the complete JSON argument string.
func (c *ResponseFallbackToolCatalog) ResponseFallbackToolCallItem(callID, flatName, arguments string) OutputItem {
	item := OutputItem{Type: "function_call", Status: "completed", Id: callID, CallId: callID, Name: flatName, Arguments: arguments}
	binding, ok := c.Binding(flatName)
	if !ok {
		return item
	}
	switch binding.Kind {
	case ResponseFallbackToolCustom:
		return OutputItem{Type: "custom_tool_call", Status: "completed", Id: callID, CallId: callID, Name: binding.Name, Namespace: binding.Namespace, Input: ResponseCustomToolInput(arguments)}
	case ResponseFallbackToolSearch:
		return OutputItem{Type: "tool_search_call", Status: "completed", Id: callID, CallId: callID, Execution: "client", ToolSearchArguments: normalizeResponseToolSearchArguments(arguments)}
	default:
		item.Name = binding.Name
		item.Namespace = binding.Namespace
		if binding.Namespace != "" {
			// Fallback never encrypts function arguments. Codex reads an empty
			// encrypted_function_args list as "plaintext arguments", so its
			// multi-agent messages stay plaintext instead of being stored as
			// opaque encrypted_content that no fallback route can replay.
			item.EncryptedFunctionArgs = &[]string{}
		}
		return item
	}
}

// ResponseCustomToolInput extracts the raw custom-tool input from the
// function-call arguments fallback used to carry it. When the upstream model
// did not produce the expected {"input": "..."} object the raw arguments are
// returned so the client still receives the model's text.
func ResponseCustomToolInput(arguments string) string {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &envelope); err != nil {
		return arguments
	}
	raw, ok := envelope[responseCustomToolInputArgument]
	if !ok {
		return arguments
	}
	var input string
	if err := json.Unmarshal(raw, &input); err != nil {
		return string(raw)
	}
	return input
}

// normalizeResponseToolSearchArguments returns object-valued tool_search
// arguments. Integral numeric "limit" values are rewritten as JSON integers
// because clients parse the field as an integer; invalid input becomes {}.
func normalizeResponseToolSearchArguments(arguments string) json.RawMessage {
	trimmed := bytes.TrimSpace([]byte(arguments))
	var fields map[string]json.RawMessage
	if len(trimmed) == 0 || json.Unmarshal(trimmed, &fields) != nil || fields == nil {
		return json.RawMessage("{}")
	}
	if raw, ok := fields["limit"]; ok {
		var limit float64
		if json.Unmarshal(raw, &limit) == nil && limit == math.Trunc(limit) && !math.IsInf(limit, 0) && math.Abs(limit) < 1<<53 {
			fields["limit"] = json.RawMessage(strconv.FormatInt(int64(limit), 10))
			if encoded, err := json.Marshal(fields); err == nil {
				return encoded
			}
		}
	}
	return append(json.RawMessage(nil), trimmed...)
}

// validResponseReasoningEffort reports whether effort is one of the reasoning
// effort values the request binding accepts.
func validResponseReasoningEffort(effort string) bool {
	switch effort {
	case "none", "default", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		return true
	default:
		return false
	}
}

// mergeAssistantTurnText folds an assistant message item into the open
// assistant tool-call message of the same turn. Parameters: target is the open
// assistant message, msg the lowered assistant item, pendingReasoning staged
// reasoning that belongs to this turn. Text is joined with a newline; staged
// reasoning attaches when the turn has none yet and is then cleared.
func mergeAssistantTurnText(target *model.Message, msg *model.Message, pendingReasoning *string) {
	if target == nil || msg == nil {
		return
	}
	if text := msg.StringContent(); text != "" {
		if existing := target.StringContent(); existing != "" {
			target.Content = existing + "\n" + text
		} else {
			target.Content = text
		}
	}
	reasoning := ""
	if msg.ReasoningContent != nil {
		reasoning = *msg.ReasoningContent
	}
	if reasoning == "" && pendingReasoning != nil {
		reasoning = *pendingReasoning
	}
	if reasoning != "" && (target.ReasoningContent == nil || *target.ReasoningContent == "") {
		target.ReasoningContent = &reasoning
	}
	if pendingReasoning != nil {
		*pendingReasoning = ""
	}
}
