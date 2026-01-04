package openai

import (
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/relay/model"
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

	for _, item := range request.Input {
		switch v := item.(type) {
		case string:
			chatReq.Messages = append(chatReq.Messages, model.Message{Role: "user", Content: v})
		case map[string]any:
			if typeVal, ok := v["type"].(string); ok {
				switch strings.ToLower(typeVal) {
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
					if r, ok := v["role"].(string); ok && r != "" {
						role = r
					}

					chatReq.Messages = append(chatReq.Messages, model.Message{
						Role:      role,
						ToolCalls: []model.Tool{toolCall},
					})
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

					chatReq.Messages = append(chatReq.Messages, model.Message{
						Role:       role,
						ToolCallId: normalizedID,
						Content:    output,
					})
					continue
				}
			}
			msg, err := responseContentItemToMessage(v)
			if err != nil {
				return nil, errors.Wrap(err, "convert response api content to chat message")
			}
			chatReq.Messages = append(chatReq.Messages, *msg)
		default:
			return nil, errors.Errorf("unsupported input item of type %T", item)
		}
	}

	return chatReq, nil
}

func responseContentItemToMessage(item map[string]any) (*model.Message, error) {
	role := "user"
	if r, ok := item["role"].(string); ok && r != "" {
		role = r
	}

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
				if url, ok := partMap["image_url"].(string); ok {
					image := &model.ImageURL{Url: url}
					if detail, ok := partMap["detail"].(string); ok {
						image.Detail = detail
					}
					parts = append(parts, model.MessageContent{Type: model.ContentTypeImageURL, ImageURL: image})
					hasNonText = true
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
