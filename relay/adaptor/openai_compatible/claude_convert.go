package openai_compatible

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// ConvertOpenAIResponseToClaudeResponse converts an OpenAI-compatible response
// (Chat Completions or Response API) into Claude Messages JSON http.Response.
func ConvertOpenAIResponseToClaudeResponse(_ *gin.Context, resp *http.Response) (*http.Response, *relaymodel.ErrorWithStatusCode) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	// 1) Try Response API format first
	var responseAPIResp responseAPIResponse
	if err := json.Unmarshal(body, &responseAPIResp); err == nil && responseAPIResp.Object == "response" {
		claudeResp := responseAPIResponseToClaude(&responseAPIResp)
		return marshalClaudeHTTPResponse(resp, claudeResp)
	}

	// 2) Fallback: Chat Completions format
	var chatResp chatTextResponse
	if err := json.Unmarshal(body, &chatResp); err == nil && len(chatResp.Choices) > 0 {
		claudeResp := chatResponseToClaude(&chatResp)
		return marshalClaudeHTTPResponse(resp, claudeResp)
	}

	// 3) Unknown format – return original payload (controller may handle error)
	newResp := &http.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	return newResp, nil
}

// responseAPIResponseToClaude maps OpenAI Response API response to ClaudeMessages response
func responseAPIResponseToClaude(r *responseAPIResponse) relaymodel.ClaudeResponse {
	out := relaymodel.ClaudeResponse{
		ID:         r.Id,
		Type:       "message",
		Role:       "assistant",
		Model:      r.Model,
		Content:    []relaymodel.ClaudeContent{},
		StopReason: "end_turn",
	}

	if r.Usage != nil {
		out.Usage = relaymodel.ClaudeUsage{
			InputTokens:  r.Usage.InputTokens,
			OutputTokens: r.Usage.OutputTokens,
		}
	}

	for _, item := range r.Output {
		switch item.Type {
		case "message":
			if strings.EqualFold(item.Role, "assistant") {
				for _, c := range item.Content {
					if text := responseAPIContentText(c); text != "" {
						out.Content = append(out.Content, relaymodel.ClaudeContent{Type: "text", Text: text})
					}
				}
			}
		case "reasoning":
			for _, s := range item.Summary {
				if s.Type == "summary_text" && s.Text != "" {
					out.Content = append(out.Content, relaymodel.ClaudeContent{Type: "thinking", Thinking: s.Text})
				}
			}
		case "function_call":
			// Map to Claude tool_use block
			input := json.RawMessage(item.Arguments)
			out.Content = append(out.Content, relaymodel.ClaudeContent{
				Type:  "tool_use",
				ID:    item.CallId,
				Name:  item.Name,
				Input: input,
			})
		}
	}

	return out
}

func responseAPIContentText(content responseAPIContent) string {
	switch strings.ToLower(strings.TrimSpace(content.Type)) {
	case "output_text", "input_text", "text":
		return strings.TrimSpace(content.Text)
	case "output_json":
		if len(content.JSON) > 0 {
			return strings.TrimSpace(string(content.JSON))
		}
		return strings.TrimSpace(content.Text)
	default:
		return ""
	}
}

// chatResponseToClaude maps OpenAI Chat Completion response to ClaudeMessages response
func chatResponseToClaude(r *chatTextResponse) relaymodel.ClaudeResponse {
	out := relaymodel.ClaudeResponse{
		ID:         r.Id,
		Type:       "message",
		Role:       "assistant",
		Model:      r.Model,
		Content:    []relaymodel.ClaudeContent{},
		StopReason: "end_turn",
		Usage: relaymodel.ClaudeUsage{
			InputTokens:  r.Usage.PromptTokens,
			OutputTokens: r.Usage.CompletionTokens,
		},
	}

	for _, choice := range r.Choices {
		// Text content
		if choice.Message.Content != nil {
			switch content := choice.Message.Content.(type) {
			case string:
				if content != "" {
					out.Content = append(out.Content, relaymodel.ClaudeContent{Type: "text", Text: content})
				}
			case []relaymodel.MessageContent:
				for _, part := range content {
					if part.Type == "text" && part.Text != nil && *part.Text != "" {
						out.Content = append(out.Content, relaymodel.ClaudeContent{Type: "text", Text: *part.Text})
					}
				}
			}
		}

		// Tool calls -> tool_use blocks
		if len(choice.Message.ToolCalls) > 0 {
			for _, tc := range choice.Message.ToolCalls {
				var input json.RawMessage
				if tc.Function.Arguments != nil {
					switch v := tc.Function.Arguments.(type) {
					case string:
						input = json.RawMessage(v)
					default:
						if b, err := json.Marshal(v); err == nil {
							input = json.RawMessage(b)
						}
					}
				}
				out.Content = append(out.Content, relaymodel.ClaudeContent{
					Type:  "tool_use",
					ID:    tc.Id,
					Name:  tc.Function.Name,
					Input: input,
				})
			}
		}

		// Map finish reason
		switch choice.FinishReason {
		case "stop":
			out.StopReason = "end_turn"
		case "length":
			out.StopReason = "max_tokens"
		case "tool_calls":
			out.StopReason = "tool_use"
		case "content_filter":
			out.StopReason = "stop_sequence"
		}
	}

	return out
}

func marshalClaudeHTTPResponse(orig *http.Response, payload relaymodel.ClaudeResponse) (*http.Response, *relaymodel.ErrorWithStatusCode) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, ErrorWrapper(errors.Wrapf(err, "marshal_claude_response"), "marshal_claude_response_failed", http.StatusInternalServerError)
	}
	newResp := &http.Response{
		StatusCode: orig.StatusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(b)),
	}
	// Copy headers and set content type/length
	maps.Copy(newResp.Header, orig.Header)
	newResp.Header.Set("Content-Type", "application/json")
	newResp.Header.Set("Content-Length", fmt.Sprintf("%d", len(b)))
	return newResp, nil
}

// ConvertOpenAIStreamToClaudeSSE reads an OpenAI-compatible chat completion/response-api SSE stream
// and writes Claude-native SSE events to the client, returning computed usage.
func ConvertOpenAIStreamToClaudeSSE(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
	_ = gmw.GetLogger(c)

	// Prepare client for SSE
	common.SetEventStreamHeaders(c)

	scanner := bufio.NewScanner(resp.Body)
	buffer := make([]byte, 1024*1024)
	scanner.Buffer(buffer, len(buffer))
	scanner.Split(bufio.ScanLines)

	accumText := ""
	accumThinking := ""
	accumToolArgs := ""
	var usage *relaymodel.Usage

	// Track content blocks and indices
	nextIndex := 0
	thinkingIndex := -1
	textIndex := -1
	toolStarted := map[string]int{} // tool_call_id -> index

	// Emit message_start
	msgStart := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"type":    "message",
			"role":    "assistant",
			"model":   modelName,
			"content": []any{},
		},
	}
	if b, err := json.Marshal(msgStart); err == nil {
		c.Writer.Write([]byte("data: "))
		c.Writer.Write(b)
		c.Writer.Write([]byte("\n\n"))
		c.Writer.(http.Flusher).Flush()
	}

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		// Parse OpenAI-compatible streaming chunk (chat completions or response API event)
		chunk, ok := parseStreamChunk(payload)
		if !ok {
			continue
		}

		// Process choices
		for _, choice := range chunk.Choices {
			// Thinking delta - try Thinking field first, fallback to ReasoningContent
			var thinkingContent *string
			if choice.Delta.Thinking != nil && *choice.Delta.Thinking != "" {
				thinkingContent = choice.Delta.Thinking
			} else if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
				thinkingContent = choice.Delta.ReasoningContent
			}

			if thinkingContent != nil && *thinkingContent != "" {
				if thinkingIndex == -1 {
					// Start thinking block at next index
					start := map[string]any{
						"type":          "content_block_start",
						"index":         nextIndex,
						"content_block": map[string]any{"type": "thinking", "thinking": ""},
					}
					if b, e := json.Marshal(start); e == nil {
						c.Writer.Write([]byte("data: "))
						c.Writer.Write(b)
						c.Writer.Write([]byte("\n\n"))
						c.Writer.(http.Flusher).Flush()
					}
					thinkingIndex = nextIndex
					nextIndex++
				}
				thinkingDelta := *thinkingContent
				accumThinking += thinkingDelta
				delta := map[string]any{
					"type":  "content_block_delta",
					"index": thinkingIndex,
					"delta": map[string]any{"type": "thinking_delta", "thinking": thinkingDelta},
				}
				if b, e := json.Marshal(delta); e == nil {
					c.Writer.Write([]byte("data: "))
					c.Writer.Write(b)
					c.Writer.Write([]byte("\n\n"))
					c.Writer.(http.Flusher).Flush()
				}
			}

			// Signature delta (attached to thinking block)
			if choice.Delta.Signature != nil && *choice.Delta.Signature != "" {
				if thinkingIndex == -1 {
					// Start thinking block to attach signature
					start := map[string]any{
						"type":          "content_block_start",
						"index":         nextIndex,
						"content_block": map[string]any{"type": "thinking", "thinking": ""},
					}
					if b, e := json.Marshal(start); e == nil {
						c.Writer.Write([]byte("data: "))
						c.Writer.Write(b)
						c.Writer.Write([]byte("\n\n"))
						c.Writer.(http.Flusher).Flush()
					}
					thinkingIndex = nextIndex
					nextIndex++
				}
				sig := *choice.Delta.Signature
				delta := map[string]any{
					"type":  "content_block_delta",
					"index": thinkingIndex,
					"delta": map[string]any{"type": "signature_delta", "signature": sig},
				}
				if b, e := json.Marshal(delta); e == nil {
					c.Writer.Write([]byte("data: "))
					c.Writer.Write(b)
					c.Writer.Write([]byte("\n\n"))
					c.Writer.(http.Flusher).Flush()
				}
			}

			// Text delta
			deltaText := choice.Delta.StringContent()
			if deltaText != "" {
				if textIndex == -1 {
					// Start text content block at next index
					start := map[string]any{
						"type":          "content_block_start",
						"index":         nextIndex,
						"content_block": map[string]any{"type": "text", "text": ""},
					}
					if b, e := json.Marshal(start); e == nil {
						c.Writer.Write([]byte("data: "))
						c.Writer.Write(b)
						c.Writer.Write([]byte("\n\n"))
						c.Writer.(http.Flusher).Flush()
					}
					textIndex = nextIndex
					nextIndex++
				}
				accumText += deltaText
				delta := map[string]any{
					"type":  "content_block_delta",
					"index": textIndex,
					"delta": map[string]any{"type": "text_delta", "text": deltaText},
				}
				if b, e := json.Marshal(delta); e == nil {
					c.Writer.Write([]byte("data: "))
					c.Writer.Write(b)
					c.Writer.Write([]byte("\n\n"))
					c.Writer.(http.Flusher).Flush()
				}
			}

			// Tool call deltas
			if len(choice.Delta.ToolCalls) > 0 {
				for _, tc := range choice.Delta.ToolCalls {
					id := tc.Id
					if id == "" {
						id = fmt.Sprintf("tool_%d", nextIndex)
					}
					idx, exists := toolStarted[id]
					if !exists {
						// Start a new tool_use block
						idx = nextIndex
						toolStarted[id] = idx
						nextIndex++
						start := map[string]any{
							"type":  "content_block_start",
							"index": idx,
							"content_block": map[string]any{
								"type": "tool_use",
								"id":   id,
								"name": func() string {
									if tc.Function != nil {
										return tc.Function.Name
									}
									return ""
								}(),
								"input": map[string]any{},
							},
						}
						if b, e := json.Marshal(start); e == nil {
							c.Writer.Write([]byte("data: "))
							c.Writer.Write(b)
							c.Writer.Write([]byte("\n\n"))
							c.Writer.(http.Flusher).Flush()
						}
					}

					// Delta arguments
					var argStr string
					if tc.Function != nil && tc.Function.Arguments != nil {
						switch v := tc.Function.Arguments.(type) {
						case string:
							argStr = v
						default:
							if b, e := json.Marshal(v); e == nil {
								argStr = string(b)
							}
						}
					}
					if argStr != "" {
						accumToolArgs += argStr
						delta := map[string]any{
							"type":  "content_block_delta",
							"index": idx,
							"delta": map[string]any{"type": "input_json_delta", "partial_json": argStr},
						}
						if b, e := json.Marshal(delta); e == nil {
							c.Writer.Write([]byte("data: "))
							c.Writer.Write(b)
							c.Writer.Write([]byte("\n\n"))
							c.Writer.(http.Flusher).Flush()
						}
					}
				}
			}
		}

		// Usage delta
		if chunk.Usage != nil {
			usage = chunk.Usage
			msgDelta := map[string]any{
				"type": "message_delta",
				"usage": map[string]any{
					"input_tokens":  usage.PromptTokens,
					"output_tokens": usage.CompletionTokens,
				},
			}
			if b, e := json.Marshal(msgDelta); e == nil {
				c.Writer.Write([]byte("data: "))
				c.Writer.Write(b)
				c.Writer.Write([]byte("\n\n"))
				c.Writer.(http.Flusher).Flush()
			}
		}
	}

	// Close any started blocks
	if thinkingIndex >= 0 {
		stop := map[string]any{"type": "content_block_stop", "index": thinkingIndex}
		if b, e := json.Marshal(stop); e == nil {
			c.Writer.Write([]byte("data: "))
			c.Writer.Write(b)
			c.Writer.Write([]byte("\n\n"))
			c.Writer.(http.Flusher).Flush()
		}
	}
	if textIndex >= 0 {
		stop := map[string]any{"type": "content_block_stop", "index": textIndex}
		if b, e := json.Marshal(stop); e == nil {
			c.Writer.Write([]byte("data: "))
			c.Writer.Write(b)
			c.Writer.Write([]byte("\n\n"))
			c.Writer.(http.Flusher).Flush()
		}
	}
	for _, idx := range toolStarted {
		stop := map[string]any{"type": "content_block_stop", "index": idx}
		if b, e := json.Marshal(stop); e == nil {
			c.Writer.Write([]byte("data: "))
			c.Writer.Write(b)
			c.Writer.Write([]byte("\n\n"))
			c.Writer.(http.Flusher).Flush()
		}
	}

	// Finalize usage if upstream omitted
	if usage == nil {
		completion := CountTokenText(accumText, modelName) + CountTokenText(accumThinking, modelName) + CountTokenText(accumToolArgs, modelName)
		usage = &relaymodel.Usage{PromptTokens: promptTokens, CompletionTokens: completion, TotalTokens: promptTokens + completion}
	} else if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	// message_stop and [DONE]
	msgStop := map[string]any{"type": "message_stop"}
	if b, e := json.Marshal(msgStop); e == nil {
		c.Writer.Write([]byte("data: "))
		c.Writer.Write(b)
		c.Writer.Write([]byte("\n\n"))
		c.Writer.(http.Flusher).Flush()
	}
	c.Writer.Write([]byte("data: [DONE]\n\n"))
	c.Writer.(http.Flusher).Flush()
	_ = resp.Body.Close()
	return usage, nil
}

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
		return nil, nil, err
	}

	converted := convertResponseAPIStreamEventToResponse(&event)
	var idxPtr *int
	if event.OutputIndex != nil {
		idx := *event.OutputIndex
		idxPtr = &idx
	}
	return &converted, idxPtr, nil
}

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

func responseAPIUsageToModel(usage *responseAPIUsage) *relaymodel.Usage {
	if usage == nil {
		return nil
	}
	total := usage.TotalTokens
	if total == 0 {
		total = usage.InputTokens + usage.OutputTokens
	}
	return &relaymodel.Usage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      total,
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

func extractJSONFromStreamEvent(event *responseAPIStreamEvent) json.RawMessage {
	if event == nil {
		return nil
	}
	if len(event.JSON) > 0 {
		return cloneRawMessageBytes(event.JSON)
	}
	if event.Part != nil {
		if len(event.Part.JSON) > 0 {
			return cloneRawMessageBytes(event.Part.JSON)
		}
		if event.Part.Text != "" {
			return normalizeJSONRawString(event.Part.Text)
		}
	}
	if len(event.Output) > 0 {
		if payload := decodeJSONBlobBytes(event.Output); len(payload) > 0 {
			return payload
		}
	}
	if event.Text != "" {
		return normalizeJSONRawString(event.Text)
	}
	if len(event.Delta) > 0 {
		if partial := extractStringFromRawMessage(event.Delta, "json", "partial_json", "text"); partial != "" {
			return normalizeJSONRawString(partial)
		}
	}
	return nil
}

func decodeJSONBlobBytes(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		return cloneRawMessageBytes(raw)
	}
	if payload := extractJSONValueNode(node); len(payload) > 0 {
		return payload
	}
	return cloneRawMessageBytes(raw)
}

func extractJSONValueNode(node any) json.RawMessage {
	switch v := node.(type) {
	case map[string]any:
		if val, ok := v["json"]; ok {
			if payload := extractJSONValueNode(val); len(payload) > 0 {
				return payload
			}
		}
		if val, ok := v["text"]; ok {
			if payload := extractJSONValueNode(val); len(payload) > 0 {
				return payload
			}
		}
		if val, ok := v["content"]; ok {
			if payload := extractJSONValueNode(val); len(payload) > 0 {
				return payload
			}
		}
		if b, err := json.Marshal(v); err == nil {
			return b
		}
	case []any:
		for _, child := range v {
			if payload := extractJSONValueNode(child); len(payload) > 0 {
				return payload
			}
		}
		if b, err := json.Marshal(v); err == nil {
			return b
		}
	case string:
		return normalizeJSONRawString(v)
	case json.RawMessage:
		return cloneRawMessageBytes(v)
	}
	return nil
}

func normalizeJSONRawString(text string) json.RawMessage {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) >= 2 && ((trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') || (trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'')) {
		if unquoted, err := strconv.Unquote(trimmed); err == nil {
			trimmed = unquoted
		}
	}
	bytes := []byte(trimmed)
	dup := make([]byte, len(bytes))
	copy(dup, bytes)
	return json.RawMessage(dup)
}

func cloneRawMessageBytes(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	dup := make([]byte, len(raw))
	copy(dup, raw)
	return json.RawMessage(dup)
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
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
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
