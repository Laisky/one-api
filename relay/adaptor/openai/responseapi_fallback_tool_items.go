package openai

import (
	"bytes"
	"encoding/json"
	"io"
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
	// The converter builds catalog from the same input, so its decoded
	// discovery items are reused; a catalog without them decodes afresh.
	discovery, cached := catalog.cachedDiscoveryFor(input)
	if !cached {
		decoded, err := responseFallbackToolSearchItems(input)
		if err != nil {
			return nil, "", errors.Wrap(err, "inspect response discovery history")
		}
		discovery = decoded
	}
	byIndex := make(map[int]ResponseToolSearchInputItem, len(discovery))
	for _, item := range discovery {
		byIndex[item.Index] = item
	}

	serverTools := inspectResponseServerToolSegments(input, byIndex)

	lowered := make(ResponseAPIInput, 0, len(input))
	effort := ""
	for index, raw := range input {
		if serverTools.dropReasoning[index] {
			// Fallback cannot replay the provider's server_tool_use and
			// web_search_tool_result blocks, and Claude rejects replayed thinking
			// whose assistant message was modified, so thinking from a response
			// that ran provider searches is omitted (Claude accepts omission).
			continue
		}
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
		if note, annotate := serverTools.notes[index]; annotate {
			// The provider's search blocks cannot be replayed; this note keeps
			// the model aware that the answer came from a web search.
			item = annotateResponseAssistantMessage(item, note)
			raw = item
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

// responseServerToolSegments describes, for model responses that also ran
// provider-executed web searches, which reasoning items must be omitted and
// which assistant message should carry a note naming the searches.
type responseServerToolSegments struct {
	dropReasoning map[int]bool
	notes         map[int]string
}

// inspectResponseServerToolSegments groups input into model responses: maximal
// runs of model output items (reasoning, thinking, assistant messages and tool
// calls), ended by caller inputs such as user messages and tool outputs. For a
// response that produced a web_search_call it marks its reasoning items for
// omission and assigns a search note to its first assistant message.
// Parameters: input is the request input, discovery its raw discovery items.
func inspectResponseServerToolSegments(input ResponseAPIInput, discovery map[int]ResponseToolSearchInputItem) responseServerToolSegments {
	result := responseServerToolSegments{}
	var reasoning []int
	var searches []string
	assistant := -1
	flush := func() {
		if len(searches) > 0 {
			if result.dropReasoning == nil {
				result.dropReasoning, result.notes = make(map[int]bool), make(map[int]string)
			}
			for _, index := range reasoning {
				result.dropReasoning[index] = true
			}
			if assistant >= 0 {
				result.notes[assistant] = "[Web search: " + strings.Join(searches, "; ") + "]"
			}
		}
		reasoning, searches, assistant = reasoning[:0], searches[:0], -1
	}
	for index, raw := range input {
		kind, role := "", ""
		item, isMap := raw.(map[string]any)
		if discovered, ok := discovery[index]; ok {
			kind, isMap = strings.ToLower(strings.TrimSpace(discovered.Type)), false
		} else if isMap {
			kind, _ = item["type"].(string)
			kind = strings.ToLower(strings.TrimSpace(kind))
			role, _ = item["role"].(string)
		}
		switch {
		case kind == "reasoning" || kind == "thinking":
			reasoning = append(reasoning, index)
		case kind == "web_search_call":
			searches = append(searches, describeResponseWebSearchCall(item))
		case kind == "function_call" || kind == "custom_tool_call" || kind == "tool_search_call":
		case (kind == "message" || kind == "") && role == "assistant":
			if assistant < 0 {
				assistant = index
			}
		default:
			flush()
		}
	}
	flush()
	return result
}

// describeResponseWebSearchCall renders one web_search_call action as a short
// bounded phrase, e.g. a query, an opened URL, or a find-in-page pattern.
func describeResponseWebSearchCall(item map[string]any) string {
	action, _ := item["action"].(map[string]any)
	kind, _ := action["type"].(string)
	text := ""
	switch kind {
	case "open_page":
		url, _ := action["url"].(string)
		text = "opened " + url
	case "find_in_page":
		url, _ := action["url"].(string)
		pattern, _ := action["pattern"].(string)
		text = "searched " + url + " for " + pattern
	default:
		if query, _ := action["query"].(string); query != "" {
			text = query
		} else if queries, ok := action["queries"].([]any); ok {
			parts := make([]string, 0, len(queries))
			for _, raw := range queries {
				if query, ok := raw.(string); ok && query != "" {
					parts = append(parts, query)
				}
			}
			text = strings.Join(parts, "; ")
		}
	}
	if text = firstResponseLine(text); text == "" {
		return "query not recorded"
	}
	return text
}

// annotateResponseAssistantMessage returns a shallow copy of an assistant
// message item whose content carries note first: string content becomes note,
// a newline and the original text; array content gets a new array whose first
// part is an output_text part holding note; any other content is replaced by
// note. Parameters: item is the assistant message and note the annotation.
// The caller's item and its content array are never modified.
func annotateResponseAssistantMessage(item map[string]any, note string) map[string]any {
	clone := cloneResponseInputItem(item)
	switch content := item["content"].(type) {
	case string:
		clone["content"] = note + "\n" + content
	case []any:
		parts := []any{map[string]any{"type": "output_text", "text": note}}
		parts = append(parts, content...)
		clone["content"] = parts
	default:
		clone["content"] = note
	}
	return clone
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
// message whose single input_text part is a "[Agent message from A to B]"
// header line followed by the message text. A string content is the text
// verbatim; an array content contributes the text of its input_text, text and
// output_text parts joined by newlines. Encrypted or other non-text parts are
// bound to the provider that produced them, and missing, null or non-string
// non-array content carries no representable text, so both return an error
// wrapping ErrResponseFallbackNotRepresentable that never quotes the content.
// Parameters: item is the decoded agent_message item. Returns the lowered user
// message or that error.
func lowerResponseAgentMessage(item map[string]any) (map[string]any, error) {
	author, _ := item["author"].(string)
	recipient, _ := item["recipient"].(string)
	var texts []string
	switch content := item["content"].(type) {
	case string:
		texts = []string{content}
	case []any:
		for _, rawPart := range content {
			part, ok := rawPart.(map[string]any)
			if !ok {
				return nil, errors.Wrap(ErrResponseFallbackNotRepresentable, "invalid agent message content")
			}
			switch kind, _ := part["type"].(string); kind {
			case "input_text", "text", "output_text":
				// An absent text member is an empty part; a present non-string
				// one (null, number, object, array) is rejected, as for tool
				// output text parts, instead of being lowered as "".
				rawText, present := part["text"]
				text, isString := rawText.(string)
				if present && !isString {
					return nil, errors.Wrap(ErrResponseFallbackNotRepresentable, "agent message text part must carry string text")
				}
				texts = append(texts, text)
			default:
				return nil, errors.Wrap(ErrResponseFallbackNotRepresentable, "encrypted or non-text agent message content requires native Responses support")
			}
		}
	default:
		// Lowering this as an empty message would silently drop what the
		// agent said, so a shape with no representable text is rejected.
		return nil, errors.Wrap(ErrResponseFallbackNotRepresentable, "agent message content must be a string or an array of text parts")
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
// namespace), custom_tool_call (raw input), or a client tool_search_call whose
// arguments follow the contract of normalizeResponseToolSearchArguments.
// Unknown names render as plain function_call items.
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

// responseToolSearchEmptyArguments renders tool_search arguments that carry no
// usable JSON object: an empty query, which Codex answers with an empty
// tool_search_output instead of failing to parse the call.
const responseToolSearchEmptyArguments = `{"query":""}`

// responseToolSearchMaxExactLimit bounds limits read through float64, the
// largest integer it represents exactly.
const responseToolSearchMaxExactLimit = 1 << 53

// normalizeResponseToolSearchArguments renders model-generated tool_search
// arguments so a client that parses them as {query: required string, limit:
// optional unsigned integer, unknown keys ignored} (Codex's
// SearchToolCallParams) always succeeds; a parse failure there leaves an
// orphan call the client cannot answer. Parameters: arguments is the complete
// upstream argument string. The result is one compact JSON object, keys
// sorted, in which:
//   - "query" is a string: a string is kept, a number or boolean becomes its
//     JSON text, and a missing, null, array or object query becomes "";
//   - "limit" is present only as a positive JSON integer, read from an
//     integral JSON number or a base-10 integer string; any other limit is
//     removed so the client's default applies;
//   - every other member keeps its value (the last one for a repeated key).
//
// Empty, invalid or non-object input becomes {"query":""}.
func normalizeResponseToolSearchArguments(arguments string) json.RawMessage {
	var members map[string]json.RawMessage
	decoder := json.NewDecoder(strings.NewReader(arguments))
	if decoder.Decode(&members) != nil || members == nil {
		return json.RawMessage(responseToolSearchEmptyArguments)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		// Trailing data after the object, such as a second value.
		return json.RawMessage(responseToolSearchEmptyArguments)
	}
	members["query"] = responseToolSearchQuery(members["query"])
	limit, ok := responseToolSearchLimit(members["limit"])
	delete(members, "limit")
	if ok {
		members["limit"] = json.RawMessage(strconv.FormatInt(limit, 10))
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(members); err != nil {
		return json.RawMessage(responseToolSearchEmptyArguments)
	}
	return json.RawMessage(bytes.TrimRight(encoded.Bytes(), "\n"))
}

// responseToolSearchQuery renders a tool_search query member as a JSON
// string. Parameters: raw is the member's value, nil when absent. Strings are
// returned unchanged, numbers and booleans become a string of their JSON
// text, and every other value, including bytes that are not exactly one JSON
// value, becomes "". The result is always one valid JSON string.
func responseToolSearchQuery(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || !json.Valid(raw) {
		return json.RawMessage(`""`)
	}
	switch first := raw[0]; {
	case first == '"':
		return raw
	case first == 't' || first == 'f' || first == '-' || (first >= '0' && first <= '9'):
		// A valid JSON value led by these bytes is a boolean or number
		// literal. Encode its text through the JSON encoder rather than
		// splicing it between quotes, so the string can never be broken out of.
		quoted, err := json.Marshal(string(raw))
		if err != nil {
			return json.RawMessage(`""`)
		}
		return quoted
	default:
		return json.RawMessage(`""`)
	}
}

// responseToolSearchLimit reads a tool_search limit member. Parameters: raw is
// the member's value, nil when absent. It returns the value and true only for
// a positive integer given as a base-10 integer string, an integer literal
// within int64, or an integral number literal (8.0, 1e2) up to 2^53.
func responseToolSearchLimit(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	text := string(raw)
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return 0, false
		}
		value, err := strconv.ParseInt(text, 10, 64)
		return value, err == nil && value > 0
	}
	if value, err := strconv.ParseInt(text, 10, 64); err == nil {
		return value, value > 0
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || value != math.Trunc(value) || value < 1 || value > responseToolSearchMaxExactLimit {
		return 0, false
	}
	return int64(value), true
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
	if responseHasGoogleTextMetadata(target) || responseHasGoogleTextMetadata(msg) {
		target.Content = append(responseGoogleMessageParts(target), responseGoogleMessageParts(msg)...)
		target.ExtraContent = nil
	} else if text := msg.StringContent(); text != "" {
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
