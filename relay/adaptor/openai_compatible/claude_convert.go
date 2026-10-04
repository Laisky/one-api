package openai_compatible

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/render"
	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/Laisky/one-api/relay/adaptor/common/toolnamesafe"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// ConvertOpenAIResponseToClaudeResponse converts an OpenAI-compatible response
// (Chat Completions or Response API) into Claude Messages JSON http.Response.
func ConvertOpenAIResponseToClaudeResponse(c *gin.Context, resp *http.Response) (*http.Response, *relaymodel.ErrorWithStatusCode) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	// 1) Try Response API format first
	var responseAPIResp responseAPIResponse
	if err := json.Unmarshal(body, &responseAPIResp); err == nil && responseAPIResp.Object == "response" {
		claudeResp := responseAPIResponseToClaude(c, &responseAPIResp)
		return marshalClaudeHTTPResponse(resp, claudeResp)
	}

	// 2) Fallback: Chat Completions format
	var chatResp chatTextResponse
	if err := json.Unmarshal(body, &chatResp); err == nil && len(chatResp.Choices) > 0 {
		claudeResp := chatResponseToClaude(c, &chatResp)
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
func responseAPIResponseToClaude(c *gin.Context, r *responseAPIResponse) relaymodel.ClaudeResponse {
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
			InputTokens:              r.Usage.InputTokens,
			OutputTokens:             r.Usage.OutputTokens,
			CacheCreationInputTokens: r.Usage.CacheWrite5mTokens + r.Usage.CacheWrite1hTokens,
		}
		if r.Usage.InputTokensDetails != nil && r.Usage.InputTokensDetails.CachedTokens > 0 {
			out.Usage.CacheReadInputTokens = r.Usage.InputTokensDetails.CachedTokens
		}
		if r.Usage.CacheWrite5mTokens > 0 || r.Usage.CacheWrite1hTokens > 0 {
			out.Usage.CacheCreation = &relaymodel.ClaudeCacheCreation{
				Ephemeral5mInputTokens: r.Usage.CacheWrite5mTokens,
				Ephemeral1hInputTokens: r.Usage.CacheWrite1hTokens,
			}
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
			// Map to Claude tool_use block; restore any sanitized name so the
			// client sees the original identifier it submitted.
			input := json.RawMessage(item.Arguments)
			out.Content = append(out.Content, relaymodel.ClaudeContent{
				Type:  "tool_use",
				ID:    item.CallId,
				Name:  toolnamesafe.RestoreToolName(c, item.Name),
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
func chatResponseToClaude(c *gin.Context, r *chatTextResponse) relaymodel.ClaudeResponse {
	out := relaymodel.ClaudeResponse{
		ID:         r.Id,
		Type:       "message",
		Role:       "assistant",
		Model:      r.Model,
		Content:    []relaymodel.ClaudeContent{},
		StopReason: "end_turn",
		Usage: relaymodel.ClaudeUsage{
			InputTokens:              r.Usage.PromptTokens,
			OutputTokens:             r.Usage.CompletionTokens,
			CacheCreationInputTokens: r.Usage.CacheWrite5mTokens + r.Usage.CacheWrite1hTokens,
		},
	}
	if r.Usage.PromptTokensDetails != nil && r.Usage.PromptTokensDetails.CachedTokens > 0 {
		out.Usage.CacheReadInputTokens = r.Usage.PromptTokensDetails.CachedTokens
	}
	if r.Usage.CacheWrite5mTokens > 0 || r.Usage.CacheWrite1hTokens > 0 {
		out.Usage.CacheCreation = &relaymodel.ClaudeCacheCreation{
			Ephemeral5mInputTokens: r.Usage.CacheWrite5mTokens,
			Ephemeral1hInputTokens: r.Usage.CacheWrite1hTokens,
		}
	}

	for _, choice := range r.Choices {
		// Thinking/Reasoning content - try Thinking field first, fallback to ReasoningContent/Reasoning
		var thinkingContent *string
		if choice.Message.Thinking != nil && *choice.Message.Thinking != "" {
			thinkingContent = choice.Message.Thinking
		} else if choice.Message.ReasoningContent != nil && *choice.Message.ReasoningContent != "" {
			thinkingContent = choice.Message.ReasoningContent
		} else if choice.Message.Reasoning != nil && *choice.Message.Reasoning != "" {
			thinkingContent = choice.Message.Reasoning
		}

		if thinkingContent != nil && *thinkingContent != "" {
			out.Content = append(out.Content, relaymodel.ClaudeContent{Type: "thinking", Thinking: *thinkingContent})
		}

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

		// Tool calls -> tool_use blocks; restore any sanitized name so the
		// client sees the original identifier it submitted.
		if len(choice.Message.ToolCalls) > 0 {
			toolnamesafe.RestoreToolCallNames(c, choice.Message.ToolCalls)
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
	lg := gmw.GetLogger(c)

	// Prepare client for SSE
	common.SetEventStreamHeaders(c)

	observer := relaymodel.NewResponseUsageAccumulator(true)
	lineReader := commonsse.NewLineReader(&claudeUsageReader{reader: resp.Body, observer: observer}, commonsse.DefaultLineBufferSize)

	// Wrap the reader with heartbeats to prevent reverse-proxy timeouts (e.g. Cloudflare 524).
	hbr := render.NewHeartbeatLineReader(c, lineReader, render.DefaultHeartbeatInterval)
	defer hbr.Close()

	var usage *relaymodel.Usage

	// Track content blocks and indices
	nextIndex := 0
	thinkingIndex := -1
	textIndex := -1
	toolStarted := map[string]int{} // tool_call_id -> index

	// writeClaudeSSE writes a Claude-format SSE event: "event: <type>\ndata: <json>\n\n".
	// The event type is extracted from the "type" field of the payload.
	writeClaudeSSE := func(event map[string]any) {
		b, err := json.Marshal(event)
		if err != nil {
			return
		}
		eventType, _ := event["type"].(string)
		if eventType != "" {
			c.Writer.Write([]byte("event: " + eventType + "\n")) //nolint:errcheck
		}
		c.Writer.Write([]byte("data: ")) //nolint:errcheck
		c.Writer.Write(b)                //nolint:errcheck
		c.Writer.Write([]byte("\n\n"))   //nolint:errcheck
		c.Writer.(http.Flusher).Flush()
	}

	// Emit message_start
	writeClaudeSSE(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"type":    "message",
			"role":    "assistant",
			"model":   modelName,
			"content": []any{},
		},
	})

	upstreamDone := false
	var streamErr error
	for {
		line, err := hbr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			streamErr = err
			break
		}

		if line.Oversized {
			payloadBytes, err := io.ReadAll(line.Large)
			if err != nil {
				streamErr = err
				break
			}

			chunk, ok := parseStreamChunk(string(payloadBytes))
			if !ok {
				continue
			}

			for _, choice := range chunk.Choices {
				var thinkingContent *string
				if choice.Delta.Thinking != nil && *choice.Delta.Thinking != "" {
					thinkingContent = choice.Delta.Thinking
				} else if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
					thinkingContent = choice.Delta.ReasoningContent
				} else if choice.Delta.Reasoning != nil && *choice.Delta.Reasoning != "" {
					thinkingContent = choice.Delta.Reasoning
				}

				if thinkingContent != nil && *thinkingContent != "" {
					if thinkingIndex == -1 {
						writeClaudeSSE(map[string]any{
							"type":          "content_block_start",
							"index":         nextIndex,
							"content_block": map[string]any{"type": "thinking", "thinking": ""},
						})
						thinkingIndex = nextIndex
						nextIndex++
					}
					thinkingDelta := *thinkingContent
					writeClaudeSSE(map[string]any{
						"type":  "content_block_delta",
						"index": thinkingIndex,
						"delta": map[string]any{"type": "thinking_delta", "thinking": thinkingDelta},
					})
				}

				if choice.Delta.Signature != nil && *choice.Delta.Signature != "" {
					if thinkingIndex == -1 {
						writeClaudeSSE(map[string]any{
							"type":          "content_block_start",
							"index":         nextIndex,
							"content_block": map[string]any{"type": "thinking", "thinking": ""},
						})
						thinkingIndex = nextIndex
						nextIndex++
					}
					writeClaudeSSE(map[string]any{
						"type":  "content_block_delta",
						"index": thinkingIndex,
						"delta": map[string]any{"type": "signature_delta", "signature": *choice.Delta.Signature},
					})
				}

				deltaText := choice.Delta.StringContent()
				if deltaText != "" {
					if textIndex == -1 {
						writeClaudeSSE(map[string]any{
							"type":          "content_block_start",
							"index":         nextIndex,
							"content_block": map[string]any{"type": "text", "text": ""},
						})
						textIndex = nextIndex
						nextIndex++
					}
					writeClaudeSSE(map[string]any{
						"type":  "content_block_delta",
						"index": textIndex,
						"delta": map[string]any{"type": "text_delta", "text": deltaText},
					})
				}

				if len(choice.Delta.ToolCalls) > 0 {
					// Restore any sanitized tool names in-place before emitting
					// Claude tool_use content blocks so the client receives the
					// original identifier it submitted.
					toolnamesafe.RestoreToolCallNames(c, choice.Delta.ToolCalls)
					for _, tc := range choice.Delta.ToolCalls {
						id := tc.Id
						if id == "" {
							id = fmt.Sprintf("tool_%d", nextIndex)
						}
						idx, exists := toolStarted[id]
						if !exists {
							idx = nextIndex
							toolStarted[id] = idx
							nextIndex++
							writeClaudeSSE(map[string]any{
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
							})
						}

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
							writeClaudeSSE(map[string]any{
								"type":  "content_block_delta",
								"index": idx,
								"delta": map[string]any{"type": "input_json_delta", "partial_json": argStr},
							})
						}
					}
				}
			}

			if chunk.Usage != nil {
				usage = chunk.Usage
				usageDelta := map[string]any{
					"input_tokens":  usage.PromptTokens,
					"output_tokens": usage.CompletionTokens,
				}
				if usage.PromptTokensDetails != nil && usage.PromptTokensDetails.CachedTokens > 0 {
					usageDelta["cache_read_input_tokens"] = usage.PromptTokensDetails.CachedTokens
				}
				if usage.CacheWrite5mTokens > 0 || usage.CacheWrite1hTokens > 0 {
					usageDelta["cache_creation_input_tokens"] = usage.CacheWrite5mTokens + usage.CacheWrite1hTokens
					usageDelta["cache_creation"] = map[string]any{
						"ephemeral_5m_input_tokens": usage.CacheWrite5mTokens,
						"ephemeral_1h_input_tokens": usage.CacheWrite1hTokens,
					}
				}
				writeClaudeSSE(map[string]any{
					"type":  "message_delta",
					"usage": usageDelta,
				})
			}

			continue
		}

		lineText := line.Text()
		if !strings.HasPrefix(lineText, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(lineText, "data:"))
		if payload == "[DONE]" {
			upstreamDone = true
			break
		}

		// Parse OpenAI-compatible streaming chunk (chat completions or response API event)
		chunk, ok := parseStreamChunk(payload)
		if !ok {
			continue
		}

		// Process choices
		for _, choice := range chunk.Choices {
			// Thinking delta - try Thinking field first, fallback to ReasoningContent, then Reasoning
			var thinkingContent *string
			if choice.Delta.Thinking != nil && *choice.Delta.Thinking != "" {
				thinkingContent = choice.Delta.Thinking
			} else if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
				thinkingContent = choice.Delta.ReasoningContent
			} else if choice.Delta.Reasoning != nil && *choice.Delta.Reasoning != "" {
				thinkingContent = choice.Delta.Reasoning
			}

			if thinkingContent != nil && *thinkingContent != "" {
				if thinkingIndex == -1 {
					writeClaudeSSE(map[string]any{
						"type":          "content_block_start",
						"index":         nextIndex,
						"content_block": map[string]any{"type": "thinking", "thinking": ""},
					})
					thinkingIndex = nextIndex
					nextIndex++
				}
				thinkingDelta := *thinkingContent
				writeClaudeSSE(map[string]any{
					"type":  "content_block_delta",
					"index": thinkingIndex,
					"delta": map[string]any{"type": "thinking_delta", "thinking": thinkingDelta},
				})
			}

			// Signature delta (attached to thinking block)
			if choice.Delta.Signature != nil && *choice.Delta.Signature != "" {
				if thinkingIndex == -1 {
					writeClaudeSSE(map[string]any{
						"type":          "content_block_start",
						"index":         nextIndex,
						"content_block": map[string]any{"type": "thinking", "thinking": ""},
					})
					thinkingIndex = nextIndex
					nextIndex++
				}
				writeClaudeSSE(map[string]any{
					"type":  "content_block_delta",
					"index": thinkingIndex,
					"delta": map[string]any{"type": "signature_delta", "signature": *choice.Delta.Signature},
				})
			}

			// Text delta
			deltaText := choice.Delta.StringContent()
			if deltaText != "" {
				if textIndex == -1 {
					writeClaudeSSE(map[string]any{
						"type":          "content_block_start",
						"index":         nextIndex,
						"content_block": map[string]any{"type": "text", "text": ""},
					})
					textIndex = nextIndex
					nextIndex++
				}
				writeClaudeSSE(map[string]any{
					"type":  "content_block_delta",
					"index": textIndex,
					"delta": map[string]any{"type": "text_delta", "text": deltaText},
				})
			}

			// Tool call deltas
			if len(choice.Delta.ToolCalls) > 0 {
				// Restore any sanitized tool names in-place before emitting Claude
				// tool_use content blocks so the client sees the original names.
				toolnamesafe.RestoreToolCallNames(c, choice.Delta.ToolCalls)
				for _, tc := range choice.Delta.ToolCalls {
					id := tc.Id
					if id == "" {
						id = fmt.Sprintf("tool_%d", nextIndex)
					}
					idx, exists := toolStarted[id]
					if !exists {
						idx = nextIndex
						toolStarted[id] = idx
						nextIndex++
						writeClaudeSSE(map[string]any{
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
						})
					}

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
						writeClaudeSSE(map[string]any{
							"type":  "content_block_delta",
							"index": idx,
							"delta": map[string]any{"type": "input_json_delta", "partial_json": argStr},
						})
					}
				}
			}
		}

		// Usage delta
		if chunk.Usage != nil {
			usage = chunk.Usage
			usageDelta := map[string]any{
				"input_tokens":  usage.PromptTokens,
				"output_tokens": usage.CompletionTokens,
			}
			if usage.PromptTokensDetails != nil && usage.PromptTokensDetails.CachedTokens > 0 {
				usageDelta["cache_read_input_tokens"] = usage.PromptTokensDetails.CachedTokens
			}
			if usage.CacheWrite5mTokens > 0 || usage.CacheWrite1hTokens > 0 {
				usageDelta["cache_creation_input_tokens"] = usage.CacheWrite5mTokens + usage.CacheWrite1hTokens
				usageDelta["cache_creation"] = map[string]any{
					"ephemeral_5m_input_tokens": usage.CacheWrite5mTokens,
					"ephemeral_1h_input_tokens": usage.CacheWrite1hTokens,
				}
			}
			writeClaudeSSE(map[string]any{
				"type":  "message_delta",
				"usage": usageDelta,
			})
		}
	}

	if streamErr != nil {
		render.LogHeartbeatLineReaderError(c, lg, streamErr, hbr)
	}

	// Close any started content blocks.
	if thinkingIndex >= 0 {
		writeClaudeSSE(map[string]any{"type": "content_block_stop", "index": thinkingIndex})
	}
	if textIndex >= 0 {
		writeClaudeSSE(map[string]any{"type": "content_block_stop", "index": textIndex})
	}
	for _, idx := range toolStarted {
		writeClaudeSSE(map[string]any{"type": "content_block_stop", "index": idx})
	}

	// Normalize raw provider evidence with the same bounded accumulator used
	// by converted-body fallback. Aggregate-only splits remain labelled estimates.
	if !upstreamDone {
		observer.MarkIncomplete()
	}
	usage = observer.Finish(promptTokens, func(text string) int { return CountTokenText(text, modelName) })
	finalDelta := map[string]any{"input_tokens": usage.PromptTokens, "output_tokens": usage.CompletionTokens}
	if usage.PromptTokensDetails != nil && usage.PromptTokensDetails.CachedTokens > 0 {
		finalDelta["cache_read_input_tokens"] = usage.PromptTokensDetails.CachedTokens
	}
	if usage.CacheWrite5mTokens > 0 || usage.CacheWrite1hTokens > 0 {
		finalDelta["cache_creation_input_tokens"] = usage.CacheWrite5mTokens + usage.CacheWrite1hTokens
		finalDelta["cache_creation"] = map[string]any{"ephemeral_5m_input_tokens": usage.CacheWrite5mTokens, "ephemeral_1h_input_tokens": usage.CacheWrite1hTokens}
	}
	writeClaudeSSE(map[string]any{"type": "message_delta", "usage": finalDelta})

	// Only emit terminal message_stop when the upstream completed normally.
	// The Claude Messages API does NOT use [DONE] — the stream simply closes
	// after message_stop. If upstream dropped, do not fabricate message_stop;
	// let the client observe the connection close without it (honest proxy).
	if upstreamDone {
		writeClaudeSSE(map[string]any{"type": "message_stop"})
	} else {
		lg.Warn("upstream stream ended without [DONE], not emitting message_stop for Claude SSE conversion")
	}
	_ = resp.Body.Close()
	return usage, nil
}
