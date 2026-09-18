package openai

import (
	"maps"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

// ConvertResponseAPIToChatCompletionRequest converts a Response API request into a
// ChatCompletion request for providers that do not support Response API natively.
func ConvertResponseAPIToChatCompletionRequest(request *ResponseAPIRequest) (*model.GeneralOpenAIRequest, error) {
	if request == nil {
		return nil, errors.New("response api request is nil")
	}

	if request.Prompt != nil {
		return nil, errors.New("prompt templates are not supported for this channel")
	}

	if request.Background != nil && *request.Background {
		return nil, errors.New("background responses are not supported for this channel")
	}

	if normalized, changed := NormalizeToolChoice(request.ToolChoice); changed {
		request.ToolChoice = normalized
	}

	chatReq := &model.GeneralOpenAIRequest{
		Model:       request.Model,
		ExtraBody:   maps.Clone(request.ExtraBody),
		Store:       request.Store,
		Metadata:    request.Metadata,
		Stream:      request.Stream != nil && *request.Stream,
		Reasoning:   request.Reasoning,
		ServiceTier: request.ServiceTier,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		ToolChoice:  request.ToolChoice,
	}

	if request.MaxOutputTokens != nil {
		chatReq.MaxCompletionTokens = request.MaxOutputTokens
	}
	if request.User != nil {
		chatReq.User = *request.User
	}
	chatReq.ParallelTooCalls = request.ParallelToolCalls

	if request.Text != nil && request.Text.Format != nil {
		chatReq.ResponseFormat = &model.ResponseFormat{Type: request.Text.Format.Type}
		if strings.EqualFold(request.Text.Format.Type, "json_schema") {
			sanitized := sanitizeResponseAPIJSONSchema(request.Text.Format.Schema)
			schemaMap, _ := sanitized.(map[string]any)
			chatReq.ResponseFormat.JsonSchema = &model.JSONSchema{
				Name:        request.Text.Format.Name,
				Description: request.Text.Format.Description,
				Schema:      schemaMap,
			}
			chatReq.ResponseFormat.JsonSchema.Strict = nil
		}
	}

	// Handle verbosity parameter (GPT-5 series)
	// In Response API, verbosity is in text.verbosity; convert to top-level for ChatCompletion
	if request.Text != nil && request.Text.Verbosity != nil {
		chatReq.Verbosity = request.Text.Verbosity
	}

	if len(request.Tools) > 0 {
		chatReq.Tools = convertResponseAPITools(request.Tools)
		if len(chatReq.Tools) == 0 {
			chatReq.Tools = nil
		}
	}

	if chatReq.ToolChoice != nil {
		chatReq.ToolChoice = sanitizeToolChoiceAgainstTools(chatReq.ToolChoice, chatReq.Tools)
	}

	if request.Instructions != nil && *request.Instructions != "" {
		chatReq.Messages = append(chatReq.Messages, model.Message{
			Role:    "system",
			Content: *request.Instructions,
		})
	}

	// openToolCallMsgIdx tracks the assistant message of the turn currently being
	// lowered, which is still "open" to receive tool calls. The OpenAI Responses API
	// represents one assistant turn as several items: an optional reasoning item, an
	// optional assistant message, and one function_call item per (possibly parallel)
	// tool call. ChatCompletion upstreams such as DeepSeek require the whole turn to
	// live in ONE assistant message — its text, its tool_calls array and its
	// reasoning_content together — otherwise the trailing tool results end up
	// following a tool message instead of an assistant message with tool_calls,
	// producing the upstream 400 "Messages with role 'tool' must be a response to a
	// preceding message with 'tool_calls'".
	//
	// The index is reset at the start of every iteration, so only items that belong to
	// the same turn keep it alive: an assistant content item opens a turn, a
	// function_call joins or opens one, and a reasoning item passes through. Anything
	// that ends the turn (a user/system message, a tool output) leaves it at -1 and the
	// next function_call starts a fresh assistant message.
	openToolCallMsgIdx := -1
	// pendingToolCallIDs holds the normalized tool-call IDs from the current assistant
	// tool-call turn that are still eligible to be answered by an adjacent tool message. It
	// is populated by function_call items and cleared by anything that breaks the
	// assistant->tool adjacency (a string/text item, an unrelated content message, or an
	// orphan tool output we downgrade). A function_call_output whose ID is not pending is an
	// orphan: emitting it as a `tool` message would violate the ChatCompletion rule that a
	// tool message must follow an assistant message carrying the matching tool_calls, so it
	// is downgraded to a user message instead of forwarding an invalid sequence upstream.
	pendingToolCallIDs := make(map[string]struct{})
	// pendingReasoning holds replayable thinking state until its adjacent
	// assistant message is lowered. DeepSeek requires it on tool-call history.
	pendingReasoning := ""
	for _, item := range request.Input {
		currentToolCallMsgIdx := openToolCallMsgIdx
		openToolCallMsgIdx = -1
		switch v := item.(type) {
		case string:
			chatReq.Messages = append(chatReq.Messages, model.Message{Role: "user", Content: v})
			clear(pendingToolCallIDs)
			pendingReasoning = ""
		case map[string]any:
			if typeVal, ok := v["type"].(string); ok {
				switch strings.ToLower(typeVal) {
				case "reasoning":
					pendingReasoning = extractResponseAPIReasoningContent(v)
					clear(pendingToolCallIDs)
					// A reasoning item marks thinking *within* a turn, not a turn
					// boundary, so it must not close an open assistant message. This
					// gateway's own response.output orders a turn as message, reasoning,
					// function_call (see buildFinalResponse in the stream bridge), so
					// resetting here would split every replayed thinking turn into two
					// assistant messages.
					openToolCallMsgIdx = currentToolCallMsgIdx
					continue
				case "function_call":
					fcID, _ := v["id"].(string)
					callID, _ := v["call_id"].(string)
					normalizedID := convertResponseAPIIDToToolCall(fcID, callID)
					if normalizedID == "" && callID != "" {
						normalizedID = callID
					} else if normalizedID == "" && fcID != "" {
						normalizedID = fcID
					}

					name, _ := v["name"].(string)
					arguments := stringifyFunctionCallArguments(v["arguments"])

					toolCall := model.Tool{
						Id:   normalizedID,
						Type: "function",
						Function: &model.Function{
							Name:      name,
							Arguments: arguments,
						},
					}

					role := "assistant"

					// This tool call is now in-flight and may be answered by an adjacent
					// function_call_output later in the input.
					if normalizedID != "" {
						pendingToolCallIDs[normalizedID] = struct{}{}
					}

					// Merge consecutive function_call items from the same assistant turn into
					// a single assistant message so parallel tool calls share one tool_calls array.
					if currentToolCallMsgIdx >= 0 && chatReq.Messages[currentToolCallMsgIdx].Role == role {
						chatReq.Messages[currentToolCallMsgIdx].ToolCalls = append(
							chatReq.Messages[currentToolCallMsgIdx].ToolCalls, toolCall)
						// An assistant message item with empty or unrecognized content
						// decodes to a nil Content, which `json:"content,omitempty"` drops
						// entirely. DeepSeek requires non-null content on tool-call history,
						// so mirror the fresh-message branch below and pin it to "".
						if chatReq.Messages[currentToolCallMsgIdx].Content == nil {
							chatReq.Messages[currentToolCallMsgIdx].Content = ""
						}
						if pendingReasoning != "" && chatReq.Messages[currentToolCallMsgIdx].ReasoningContent == nil {
							reasoning := pendingReasoning
							chatReq.Messages[currentToolCallMsgIdx].ReasoningContent = &reasoning
						}
						pendingReasoning = ""
						openToolCallMsgIdx = currentToolCallMsgIdx
						continue
					}

					assistantMessage := model.Message{
						Role:      role,
						Content:   "",
						ToolCalls: []model.Tool{toolCall},
					}
					if pendingReasoning != "" {
						reasoning := pendingReasoning
						assistantMessage.ReasoningContent = &reasoning
					}
					pendingReasoning = ""
					chatReq.Messages = append(chatReq.Messages, assistantMessage)
					openToolCallMsgIdx = len(chatReq.Messages) - 1
					continue
				case "function_call_output":
					fcID, _ := v["id"].(string)
					callID, _ := v["call_id"].(string)
					normalizedID := convertResponseAPIIDToToolCall(fcID, callID)
					if normalizedID == "" && callID != "" {
						normalizedID = callID
					} else if normalizedID == "" && fcID != "" {
						normalizedID = fcID
					}

					output := stringifyFunctionCallOutput(v["output"])
					if output == "" {
						output = stringifyFunctionCallOutput(v["content"])
					}

					role := "tool"
					if r, ok := v["role"].(string); ok && r != "" {
						role = r
					}

					// Only emit a `tool` message when it answers an in-flight tool call from
					// the immediately preceding assistant turn. Otherwise it is an orphan
					// (e.g. trimmed history dropped the matching function_call, or a mismatched
					// call_id) and would be rejected upstream with "Messages with role 'tool'
					// must be a response to a preceding message with 'tool_calls'". Downgrade
					// such orphans to a user message so the content survives without forwarding
					// an invalid sequence.
					if _, answered := pendingToolCallIDs[normalizedID]; answered && role == "tool" && normalizedID != "" {
						chatReq.Messages = append(chatReq.Messages, model.Message{
							Role:       role,
							ToolCallId: normalizedID,
							Content:    output,
						})
						delete(pendingToolCallIDs, normalizedID)
						continue
					}

					chatReq.Messages = append(chatReq.Messages, model.Message{
						Role:    "user",
						Content: output,
					})
					// The downgraded user message breaks adjacency, so any tool calls still
					// awaiting a response can no longer be answered by a subsequent tool message.
					clear(pendingToolCallIDs)
					pendingReasoning = ""
					continue
				}
			}
			msg, err := responseContentItemToMessage(v)
			if err != nil {
				return nil, errors.Wrap(err, "convert response api content to chat message")
			}
			if pendingReasoning != "" && msg.Role == "assistant" {
				reasoning := pendingReasoning
				msg.ReasoningContent = &reasoning
			}
			pendingReasoning = ""
			chatReq.Messages = append(chatReq.Messages, *msg)
			// A non-tool content message ends the current tool-call turn.
			clear(pendingToolCallIDs)
			// An assistant text message stays open so directly following function_call
			// items join it: Chat Completions carries a turn's text and tool calls in one
			// assistant message, and DeepSeek requires that message to hold the turn's
			// reasoning_content.
			if msg.Role == "assistant" {
				openToolCallMsgIdx = len(chatReq.Messages) - 1
			}
		default:
			return nil, errors.Errorf("unsupported input item of type %T", item)
		}
	}

	return chatReq, nil
}

// extractResponseAPIReasoningContent returns replayable plaintext reasoning
// from a Responses reasoning item. Raw content takes precedence over summary;
// summaries remain a compatibility fallback for older one-api bridge output.
func extractResponseAPIReasoningContent(item map[string]any) string {
	for _, field := range []string{"content", "summary"} {
		raw, ok := item[field]
		if !ok {
			continue
		}

		parts := make([]string, 0)
		switch value := raw.(type) {
		case string:
			if value != "" {
				parts = append(parts, value)
			}
		case []any:
			for _, entry := range value {
				part, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				if text, ok := part["text"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			}
		case map[string]any:
			if text, ok := value["text"].(string); ok && text != "" {
				parts = append(parts, text)
			}
		}

		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return ""
}

func responseContentItemToMessage(item map[string]any) (*model.Message, error) {
	role := normalizeResponseMessageRoleForChat(item["role"], "user")

	var namePtr *string
	if name, ok := item["name"].(string); ok && name != "" {
		namePtr = &name
	}

	contentVal, ok := item["content"]
	if !ok {
		return &model.Message{Role: role, Name: namePtr, Content: ""}, nil
	}

	message := &model.Message{Role: role, Name: namePtr}

	switch content := contentVal.(type) {
	case string:
		message.Content = content
	case []any:
		parts := make([]model.MessageContent, 0, len(content))
		textSections := make([]string, 0, len(content))
		hasNonText := false
		for _, raw := range content {
			partMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			typeStr, _ := partMap["type"].(string)
			switch typeStr {
			case "input_text", "output_text":
				if text, ok := partMap["text"].(string); ok {
					parts = append(parts, model.MessageContent{Type: model.ContentTypeText, Text: &text})
					textSections = append(textSections, text)
				}
			case "input_image":
				if url, ok := partMap["image_url"].(string); ok && url != "" {
					image := &model.ImageURL{Url: url}
					if detail, ok := partMap["detail"].(string); ok {
						image.Detail = detail
					}
					parts = append(parts, model.MessageContent{Type: model.ContentTypeImageURL, ImageURL: image})
					hasNonText = true
				} else {
					fileID, _ := partMap["file_id"].(string)
					fileData, _ := partMap["file_data"].(string)
					filename, _ := partMap["filename"].(string)
					if fileID != "" || fileData != "" {
						parts = append(parts, model.MessageContent{
							Type:     model.ContentTypeFile,
							FileID:   fileID,
							FileData: fileData,
							Filename: filename,
						})
						hasNonText = true
					}
				}
			case "input_audio":
				if inputAudio, ok := partMap["input_audio"].(map[string]any); ok {
					data, _ := inputAudio["data"].(string)
					format, _ := inputAudio["format"].(string)
					parts = append(parts, model.MessageContent{
						Type:       model.ContentTypeInputAudio,
						InputAudio: &model.InputAudio{Data: data, Format: format},
					})
					hasNonText = true
				}
			case "reasoning":
				if text, ok := partMap["text"].(string); ok && text != "" {
					message.SetReasoningContent(string(model.ReasoningFormatReasoning), text)
				}
			default:
				if text, ok := partMap["text"].(string); ok {
					parts = append(parts, model.MessageContent{Type: model.ContentTypeText, Text: &text})
					textSections = append(textSections, text)
				} else {
					hasNonText = true
				}
			}
		}
		if len(parts) > 0 {
			if !hasNonText && len(textSections) == len(parts) && len(textSections) > 0 {
				message.Content = strings.Join(textSections, "\n")
			} else {
				message.Content = parts
			}
		}
	default:
		return nil, errors.Errorf("unsupported content type %T", contentVal)
	}

	return message, nil
}

// normalizeResponseMessageRoleForChat maps Responses API message roles to the
// portable ChatCompletion roles accepted by non-native fallback providers. The
// raw parameter may be any decoded JSON value, and defaultRole is returned when
// the value is empty or unsupported.
func normalizeResponseMessageRoleForChat(raw any, defaultRole string) string {
	role, _ := raw.(string)
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "developer":
		return "system"
	case "system", "user", "assistant", "tool":
		return strings.ToLower(strings.TrimSpace(role))
	case "":
		return defaultRole
	default:
		return defaultRole
	}
}
