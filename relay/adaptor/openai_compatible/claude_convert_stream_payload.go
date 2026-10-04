package openai_compatible

import (
	"encoding/json"
	"github.com/Laisky/errors/v2"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"strconv"
	"strings"
)

// parseStreamChunk decodes Chat or Responses stream payloads.
func parseStreamChunk(payload string) (ChatCompletionsStreamResponse, bool) {
	var chunk ChatCompletionsStreamResponse
	if err := json.Unmarshal([]byte(payload), &chunk); err == nil {
		if len(chunk.Choices) > 0 || chunk.Usage != nil || chunk.Id != "" {
			return chunk, true
		}
	}

	resp, outputIndex, err := parseResponseStreamPayload([]byte(payload))
	if err != nil || resp == nil {
		return ChatCompletionsStreamResponse{}, false
	}

	converted := responseAPIChunkToChatStream(resp, outputIndex)
	if converted == nil || len(converted.Choices) == 0 {
		return ChatCompletionsStreamResponse{}, false
	}

	return *converted, true
}

// parseResponseStreamPayload decodes Responses events and terminal response objects.
func parseResponseStreamPayload(data []byte) (*responseAPIResponse, *int, error) {
	var envelope struct {
		Response *responseAPIResponse `json:"response"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Response != nil && envelope.Response.Object == "response" {
		return envelope.Response, nil, nil
	}

	var resp responseAPIResponse
	if err := json.Unmarshal(data, &resp); err == nil && resp.Object == "response" {
		return &resp, nil, nil
	}

	var event responseAPIStreamEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, nil, errors.Wrap(err, "unmarshal response API stream event")
	}

	converted := convertResponseAPIStreamEventToResponse(&event)
	var idxPtr *int
	if event.OutputIndex != nil {
		idx := *event.OutputIndex
		idxPtr = &idx
	}
	return &converted, idxPtr, nil
}

// responseAPIChunkToChatStream projects Responses output into a Chat stream chunk.
func responseAPIChunkToChatStream(resp *responseAPIResponse, outputIndex *int) *ChatCompletionsStreamResponse {
	if resp == nil {
		return nil
	}

	delta := relaymodel.Message{Role: "assistant"}
	var deltaContent strings.Builder
	var reasoningBuilder strings.Builder
	toolCalls := make([]relaymodel.Tool, 0)

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			if strings.EqualFold(item.Role, "assistant") {
				for _, part := range item.Content {
					switch part.Type {
					case "output_text", "input_text", "text":
						deltaContent.WriteString(part.Text)
					case "output_json":
						if len(part.JSON) > 0 {
							deltaContent.Write(part.JSON)
						} else if part.Text != "" {
							deltaContent.WriteString(part.Text)
						}
					case "reasoning":
						reasoningBuilder.WriteString(part.Text)
					}
				}
			}
		case "reasoning":
			for _, part := range item.Summary {
				if part.Text != "" {
					reasoningBuilder.WriteString(part.Text)
				}
			}
		case "function_call":
			idx := len(toolCalls)
			if outputIndex != nil {
				idx = *outputIndex
			}
			tool := relaymodel.Tool{
				Id:   item.CallId,
				Type: "function",
				Function: &relaymodel.Function{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			}
			tool.Index = &idx
			toolCalls = append(toolCalls, tool)
		}
	}

	if contentStr := deltaContent.String(); contentStr != "" {
		delta.Content = contentStr
	}

	if reasoning := reasoningBuilder.String(); reasoning != "" {
		delta.Reasoning = &reasoning
	}

	if len(toolCalls) > 0 {
		delta.ToolCalls = toolCalls
	}

	choice := ChatCompletionsStreamResponseChoice{
		Index: 0,
		Delta: delta,
	}

	switch strings.ToLower(strings.TrimSpace(resp.Status)) {
	case "completed", "succeeded", "success":
		reason := "stop"
		choice.FinishReason = &reason
	case "incomplete":
		reason := "length"
		choice.FinishReason = &reason
	case "failed":
		reason := "stop"
		choice.FinishReason = &reason
	}

	stream := &ChatCompletionsStreamResponse{
		Id:      resp.Id,
		Object:  "chat.completion.chunk",
		Created: resp.CreatedAt,
		Model:   resp.Model,
		Choices: []ChatCompletionsStreamResponseChoice{choice},
	}

	if usage := responseAPIUsageToModel(resp.Usage); usage != nil {
		stream.Usage = usage
	}

	return stream
}

// responseAPIUsageToModel maps measured Responses counters into canonical usage.
func responseAPIUsageToModel(usage *responseAPIUsage) *relaymodel.Usage {
	if usage == nil {
		return nil
	}
	total := usage.TotalTokens
	if total == 0 {
		total = usage.InputTokens + usage.OutputTokens
	}
	return &relaymodel.Usage{
		PromptTokens:       usage.InputTokens,
		CompletionTokens:   usage.OutputTokens,
		TotalTokens:        total,
		CacheWrite5mTokens: usage.CacheWrite5mTokens,
		CacheWrite1hTokens: usage.CacheWrite1hTokens,
		PromptTokensDetails: func() *relaymodel.UsagePromptTokensDetails {
			if usage.InputTokensDetails == nil || usage.InputTokensDetails.CachedTokens <= 0 {
				return nil
			}
			return &relaymodel.UsagePromptTokensDetails{CachedTokens: usage.InputTokensDetails.CachedTokens}
		}(),
	}
}

type responseAPIStreamEvent struct {
	Type        string               `json:"type,omitempty"`
	Response    *responseAPIResponse `json:"response,omitempty"`
	OutputIndex *int                 `json:"output_index,omitempty"`
	Item        *responseAPIOutput   `json:"item,omitempty"`
	Part        *responseAPIContent  `json:"part,omitempty"`
	Delta       json.RawMessage      `json:"delta,omitempty"`
	Text        string               `json:"text,omitempty"`
	Arguments   string               `json:"arguments,omitempty"`
	Output      json.RawMessage      `json:"output,omitempty"`
	JSON        json.RawMessage      `json:"json,omitempty"`
	Usage       *responseAPIUsage    `json:"usage,omitempty"`
	Status      string               `json:"status,omitempty"`
	Id          string               `json:"id,omitempty"`
}

// convertResponseAPIStreamEventToResponse projects individual Responses events without changing event-specific precedence.
func convertResponseAPIStreamEventToResponse(event *responseAPIStreamEvent) responseAPIResponse {
	if event == nil {
		return responseAPIResponse{}
	}
	if event.Response != nil {
		return *event.Response
	}

	resp := responseAPIResponse{
		Status: "in_progress",
	}
	if event.Id != "" {
		resp.Id = event.Id
	}
	if event.Status != "" {
		resp.Status = event.Status
	}
	if event.Usage != nil {
		resp.Usage = event.Usage
	}

	switch {
	case strings.HasPrefix(event.Type, "response.reasoning_summary_text.delta"):
		if delta := extractStringFromRawMessage(event.Delta, "text", "delta"); delta != "" {
			resp.Output = []responseAPIOutput{{
				Type: "reasoning",
				Summary: []responseAPIContent{{
					Type: "summary_text",
					Text: delta,
				}},
			}}
		}
	case strings.HasPrefix(event.Type, "response.reasoning_summary_text.done"):
		if event.Text != "" {
			resp.Output = []responseAPIOutput{{
				Type: "reasoning",
				Summary: []responseAPIContent{{
					Type: "summary_text",
					Text: event.Text,
				}},
			}}
		}
	case strings.HasPrefix(event.Type, "response.reasoning_summary_part"):
		if event.Part != nil {
			resp.Output = []responseAPIOutput{{
				Type:    "reasoning",
				Summary: []responseAPIContent{*event.Part},
			}}
		}
	case strings.HasPrefix(event.Type, "response.output_text.delta"):
		if delta := extractStringFromRawMessage(event.Delta, "text", "delta"); delta != "" {
			resp.Output = []responseAPIOutput{{
				Type: "message",
				Role: "assistant",
				Content: []responseAPIContent{{
					Type: "output_text",
					Text: delta,
				}},
			}}
		}
	case strings.HasPrefix(event.Type, "response.output_text.done"):
		if event.Text != "" {
			resp.Output = []responseAPIOutput{{
				Type: "message",
				Role: "assistant",
				Content: []responseAPIContent{{
					Type: "output_text",
					Text: event.Text,
				}},
			}}
		}
	case strings.HasPrefix(event.Type, "response.output_json.delta"):
		if partial := extractStringFromRawMessage(event.Delta, "partial_json", "json", "text"); partial != "" {
			resp.Output = []responseAPIOutput{{
				Type: "message",
				Role: "assistant",
				Content: []responseAPIContent{{
					Type: "output_json",
					Text: partial,
				}},
			}}
		}
	case strings.HasPrefix(event.Type, "response.output_json.done"):
		if payload := extractJSONFromStreamEvent(event); len(payload) > 0 {
			resp.Output = []responseAPIOutput{{
				Type: "message",
				Role: "assistant",
				Content: []responseAPIContent{{
					Type: "output_json",
					JSON: payload,
				}},
			}}
			if strings.EqualFold(resp.Status, "in_progress") {
				resp.Status = "completed"
			}
		}
	case strings.HasPrefix(event.Type, "response.content_part"):
		if event.Part != nil {
			resp.Output = []responseAPIOutput{{
				Type:    "message",
				Role:    "assistant",
				Content: []responseAPIContent{*event.Part},
			}}
		}
	case strings.HasPrefix(event.Type, "response.output_item"):
		if event.Item != nil {
			resp.Output = []responseAPIOutput{*event.Item}
		}
	case strings.HasPrefix(event.Type, "response.function_call_arguments.delta"):
		output := responseAPIOutput{
			Type:      "function_call",
			Arguments: extractStringFromRawMessage(event.Delta, "partial_json", "text", "arguments", "delta"),
		}
		if event.Item != nil {
			output.CallId = event.Item.CallId
			output.Name = event.Item.Name
			if output.Arguments == "" {
				output.Arguments = event.Item.Arguments
			}
		}
		resp.Output = []responseAPIOutput{output}
	case strings.HasPrefix(event.Type, "response.function_call_arguments.done"):
		output := responseAPIOutput{
			Type:      "function_call",
			Arguments: event.Arguments,
		}
		if event.Item != nil {
			output.CallId = event.Item.CallId
			output.Name = event.Item.Name
			if output.Arguments == "" {
				output.Arguments = event.Item.Arguments
			}
		}
		resp.Output = []responseAPIOutput{output}
	}

	return resp
}

// extractStringFromRawMessage extracts delta protocol strings in caller-supplied order.
func extractStringFromRawMessage(raw json.RawMessage, keys ...string) string {
	if len(raw) == 0 {
		return ""
	}

	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str
	}

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, key := range keys {
			if key == "" {
				continue
			}
			if val, ok := obj[key]; ok {
				switch v := val.(type) {
				case string:
					return v
				case []byte:
					return string(v)
				default:
					if b, err := json.Marshal(v); err == nil {
						return string(b)
					}
				}
			}
		}
	}

	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote(trimmed); err == nil {
		trimmed = unquoted
	}
	return trimmed
}

// extractJSONFromStreamEvent supplies the same location-specific done-event contract used by billing.
func extractJSONFromStreamEvent(event *responseAPIStreamEvent) json.RawMessage {
	if event == nil {
		return nil
	}
	fields := relaymodel.ResponseOutputJSONDoneFields{JSON: event.JSON, Output: event.Output, Text: event.Text, Delta: event.Delta}
	if event.Part != nil {
		fields.PartJSON = event.Part.JSON
		fields.PartText = event.Part.Text
	}
	return relaymodel.ExtractResponseOutputJSONDone(fields)
}

// --- Minimal local response types to avoid import cycles ---

type responseAPIResponse struct {
	Id        string              `json:"id"`
	Object    string              `json:"object"`
	Model     string              `json:"model"`
	Output    []responseAPIOutput `json:"output"`
	Usage     *responseAPIUsage   `json:"usage,omitempty"`
	CreatedAt int64               `json:"created_at"`
	Status    string              `json:"status"`
}

type responseAPIUsage struct {
	InputTokens        int                            `json:"input_tokens"`
	OutputTokens       int                            `json:"output_tokens"`
	TotalTokens        int                            `json:"total_tokens"`
	InputTokensDetails *responseAPIInputTokensDetails `json:"input_tokens_details,omitempty"`
	CacheWrite5mTokens int                            `json:"cache_write_5m_tokens,omitempty"`
	CacheWrite1hTokens int                            `json:"cache_write_1h_tokens,omitempty"`
}

type responseAPIInputTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

type responseAPIOutput struct {
	Type      string               `json:"type"`
	Role      string               `json:"role,omitempty"`
	Content   []responseAPIContent `json:"content,omitempty"`
	Summary   []responseAPIContent `json:"summary,omitempty"`
	CallId    string               `json:"call_id,omitempty"`
	Name      string               `json:"name,omitempty"`
	Arguments string               `json:"arguments,omitempty"`
	Tools     []relaymodel.Tool    `json:"tools,omitempty"`
	Metadata  map[string]any       `json:"metadata,omitempty"`
}

type responseAPIContent struct {
	Type string          `json:"type"`
	Text string          `json:"text,omitempty"`
	JSON json.RawMessage `json:"json,omitempty"`
}

type chatTextResponse struct {
	Id      string           `json:"id"`
	Model   string           `json:"model"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Choices []chatTextChoice `json:"choices"`
	Usage   relaymodel.Usage `json:"usage"`
}

type chatTextChoice struct {
	Index        int                `json:"index"`
	Message      relaymodel.Message `json:"message"`
	FinishReason string             `json:"finish_reason"`
}
