package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func (a *Adaptor) DoRequest(c *gin.Context,
	meta *meta.Meta,
	requestBody io.Reader) (*http.Response, error) {
	lg := gmw.GetLogger(c)
	if shouldUseResponseAPIWebSocket(meta, c.Request.Method) {
		var requestPayload []byte
		if requestBody != nil {
			payload, err := io.ReadAll(requestBody)
			if err != nil {
				return nil, errors.Wrap(err, "read request payload before websocket transport decision")
			}
			requestPayload = payload
		}

		lg.Debug("openai response api transport decision",
			zap.String("transport", "websocket"),
			zap.Int("channel_type", meta.ChannelType),
			zap.String("request_path", meta.RequestURLPath),
			zap.String("model", meta.ActualModelName),
		)
		response, handled, err := doResponseAPIRequestViaWebSocket(c, a, meta, bytes.NewReader(requestPayload))
		if err != nil {
			lg.Warn("openai response api websocket request failed",
				zap.Error(err),
				zap.String("request_path", meta.RequestURLPath),
				zap.String("model", meta.ActualModelName),
			)
			return nil, err
		}
		if handled {
			lg.Debug("openai response api websocket request handled",
				zap.String("request_path", meta.RequestURLPath),
				zap.String("model", meta.ActualModelName),
			)
			return response, nil
		}
		lg.Debug("openai response api websocket skipped; fallback to http",
			zap.String("request_path", meta.RequestURLPath),
			zap.String("model", meta.ActualModelName),
		)
		return adaptor.DoRequestHelper(a, c, meta, bytes.NewReader(requestPayload))
	}

	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

// shouldUseResponseAPIWebSocket reports whether the request should use the OpenAI
// Responses WebSocket transport.
//
// Parameters:
//   - meta: request metadata containing channel and mode information.
//   - method: downstream HTTP method.
//
// Returns:
//   - bool: true when this request targets OpenAI upstream /v1/responses via POST.
func shouldUseResponseAPIWebSocket(meta *meta.Meta, method string) bool {
	if meta == nil {
		return false
	}
	if meta.ChannelType != channeltype.OpenAI {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(method), http.MethodPost) {
		return false
	}
	requestPath := strings.TrimSpace(meta.RequestURLPath)
	if requestPath == "" {
		requestPath = "/v1/chat/completions"
	}
	pathOnly := requestPath
	if idx := strings.Index(pathOnly, "?"); idx >= 0 {
		pathOnly = pathOnly[:idx]
	}
	if pathOnly == "/v1/responses" {
		return true
	}
	return meta.Mode == relaymode.ResponseAPI
}

func (a *Adaptor) DoResponse(c *gin.Context,
	resp *http.Response,
	meta *meta.Meta) (usage *model.Usage,
	err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		var responseText string
		handledClaudeStream := false

		if meta.Mode == relaymode.ClaudeMessages {
			if isClaudeConversion, exists := c.Get(ctxkey.ClaudeMessagesConversion); exists && isClaudeConversion.(bool) {
				handledClaudeStream = true
				var convErr *model.ErrorWithStatusCode
				usage, convErr = openai_compatible.ConvertOpenAIStreamToClaudeSSE(c, resp, meta.PromptTokens, meta.ActualModelName)
				if convErr != nil {
					return nil, convErr
				}
			}
		}

		if !handledClaudeStream {
			switch meta.Mode {
			case relaymode.ResponseAPI:
				err, responseText, usage = ResponseAPIDirectStreamHandler(c, resp, meta.Mode)
			default:
				if vi, ok := c.Get(ctxkey.ConvertedRequest); ok {
					if _, ok := vi.(*ResponseAPIRequest); ok {
						err, responseText, usage = ResponseAPIStreamHandler(c, resp, meta.Mode)
					} else {
						err, responseText, usage = StreamHandler(c, resp, meta.Mode)
					}
				} else {
					err, responseText, usage = StreamHandler(c, resp, meta.Mode)
				}
			}

			if usage == nil || usage.TotalTokens == 0 {
				usage = ResponseText2Usage(responseText, meta.ActualModelName, meta.PromptTokens)
			}
			if usage.TotalTokens != 0 && usage.PromptTokens == 0 {
				usage.PromptTokens = meta.PromptTokens
				usage.CompletionTokens = usage.TotalTokens - meta.PromptTokens
			}
		}
	} else {
		shouldConvertToClaude := false
		if meta.Mode == relaymode.ChatCompletions {
			if raw, exists := c.Get(ctxkey.ClaudeMessagesConversion); exists {
				if flag, ok := raw.(bool); ok && flag {
					shouldConvertToClaude = true
				}
			}
		}

		switch meta.Mode {
		case relaymode.ImagesGenerations,
			relaymode.ImagesEdits:
			err, usage = ImageHandler(c, resp)
		case relaymode.ResponseAPI:
			err, usage = ResponseAPIDirectHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
		case relaymode.Videos:
			err, usage = VideoHandler(c, resp)
		case relaymode.ClaudeMessages:
		case relaymode.ChatCompletions:
			if shouldConvertToClaude {
				break
			}
			if vi, ok := c.Get(ctxkey.ConvertedRequest); ok {
				if _, ok := vi.(*ResponseAPIRequest); ok {
					err, usage = ResponseAPIHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
				} else {
					err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
				}
			} else {
				err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
			}
		case relaymode.Embeddings:
			err, usage = EmbeddingHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
		default:
			err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
		}
	}

	recordWebSearchPreviewInvocation(c, meta)

	if !meta.IsStream && resp != nil {
		if isClaudeConversion, exists := c.Get(ctxkey.ClaudeMessagesConversion); exists && isClaudeConversion.(bool) {
			claudeResp, convertErr := a.convertToClaudeResponse(c, resp)
			if convertErr != nil {
				return nil, convertErr
			}

			c.Set(ctxkey.ConvertedResponse, claudeResp)
			return nil, nil
		}
	}

	return
}

// convertToClaudeResponse converts OpenAI response format to Claude Messages format.
func (a *Adaptor) convertToClaudeResponse(c *gin.Context, resp *http.Response) (*http.Response, *model.ErrorWithStatusCode) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	resp.Body.Close()

	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return a.convertStreamingToClaudeResponse(c, resp, body)
	}

	return a.convertNonStreamingToClaudeResponse(c, resp, body)
}

// convertNonStreamingToClaudeResponse converts a non-streaming OpenAI response to Claude format.
func (a *Adaptor) convertNonStreamingToClaudeResponse(c *gin.Context, resp *http.Response, body []byte) (*http.Response, *model.ErrorWithStatusCode) {
	var responseAPIResp ResponseAPIResponse
	if err := json.Unmarshal(body, &responseAPIResp); err == nil && responseAPIResp.Object == "response" {
		return a.ConvertResponseAPIToClaudeResponse(c, resp, &responseAPIResp)
	}

	var openaiResp TextResponse
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		newResp := &http.Response{
			StatusCode: resp.StatusCode,
			Header:     resp.Header,
			Body:       io.NopCloser(bytes.NewReader(body)),
		}
		return newResp, nil
	}

	claudeResp := model.ClaudeResponse{
		ID:      openaiResp.Id,
		Type:    "message",
		Role:    "assistant",
		Model:   openaiResp.Model,
		Content: []model.ClaudeContent{},
		Usage: model.ClaudeUsage{
			InputTokens:  openaiResp.Usage.PromptTokens,
			OutputTokens: openaiResp.Usage.CompletionTokens,
		},
		StopReason: "end_turn",
	}

	for _, choice := range openaiResp.Choices {
		if choice.Message.Content != nil {
			switch content := choice.Message.Content.(type) {
			case string:
				claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{Type: "text", Text: content})
			case []model.MessageContent:
				for _, part := range content {
					if part.Type == "text" && part.Text != nil {
						claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{Type: "text", Text: *part.Text})
					}
				}
			}
		}

		if len(choice.Message.ToolCalls) > 0 {
			for _, toolCall := range choice.Message.ToolCalls {
				var input json.RawMessage
				if toolCall.Function.Arguments != nil {
					if argsStr, ok := toolCall.Function.Arguments.(string); ok {
						input = json.RawMessage(argsStr)
					} else if argsBytes, err := json.Marshal(toolCall.Function.Arguments); err == nil {
						input = json.RawMessage(argsBytes)
					}
				}
				claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{
					Type:  "tool_use",
					ID:    toolCall.Id,
					Name:  toolCall.Function.Name,
					Input: input,
				})
			}
		}

		switch choice.FinishReason {
		case "stop":
			claudeResp.StopReason = "end_turn"
		case "length":
			claudeResp.StopReason = "max_tokens"
		case "tool_calls":
			claudeResp.StopReason = "tool_use"
		case "content_filter":
			claudeResp.StopReason = "stop_sequence"
		}
	}

	claudeBody, err := json.Marshal(claudeResp)
	if err != nil {
		return nil, ErrorWrapper(err, "marshal_claude_response_failed", http.StatusInternalServerError)
	}

	newResp := &http.Response{
		StatusCode: resp.StatusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(claudeBody)),
	}

	maps.Copy(newResp.Header, resp.Header)
	newResp.Header.Set("Content-Type", "application/json")
	newResp.Header.Set("Content-Length", fmt.Sprintf("%d", len(claudeBody)))

	return newResp, nil
}

// ConvertResponseAPIToClaudeResponse converts a Response API response to Claude Messages format.
func (a *Adaptor) ConvertResponseAPIToClaudeResponse(c *gin.Context, resp *http.Response, responseAPIResp *ResponseAPIResponse) (*http.Response, *model.ErrorWithStatusCode) {
	claudeResp := model.ClaudeResponse{
		ID:         responseAPIResp.Id,
		Type:       "message",
		Role:       "assistant",
		Model:      responseAPIResp.Model,
		Content:    []model.ClaudeContent{},
		StopReason: "end_turn",
	}

	toolUseAdded := false
	if responseAPIResp.Usage != nil {
		claudeResp.Usage = model.ClaudeUsage{
			InputTokens:  responseAPIResp.Usage.InputTokens,
			OutputTokens: responseAPIResp.Usage.OutputTokens,
		}
	}

	for _, outputItem := range responseAPIResp.Output {
		if outputItem.Type == "reasoning" {
			for _, summary := range outputItem.Summary {
				if summary.Type == "summary_text" && summary.Text != "" {
					claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{Type: "thinking", Thinking: summary.Text})
				}
			}
		} else if outputItem.Type == "message" && outputItem.Role == "assistant" {
			for _, content := range outputItem.Content {
				if content.Type == "output_text" && content.Text != "" {
					claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{Type: "text", Text: content.Text})
				} else if content.Type == "output_json" {
					var jsonText string
					if len(content.JSON) > 0 {
						jsonText = string(content.JSON)
					} else {
						jsonText = content.Text
					}
					jsonText = strings.TrimSpace(jsonText)
					if jsonText != "" {
						claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{Type: "text", Text: jsonText})
					}
				}
			}
		} else if outputItem.Type == "function_call" {
			name := strings.TrimSpace(outputItem.Name)
			if name == "" {
				continue
			}
			id := outputItem.CallId
			if id == "" {
				id = outputItem.Id
			}
			var input json.RawMessage
			if args := strings.TrimSpace(outputItem.Arguments); args != "" {
				raw := []byte(args)
				if json.Valid(raw) {
					input = json.RawMessage(raw)
				} else if marshaled, err := json.Marshal(args); err == nil {
					input = json.RawMessage(marshaled)
				}
			}
			claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{
				Type:  "tool_use",
				ID:    id,
				Name:  name,
				Input: input,
			})
			toolUseAdded = true
		}
	}

	if toolUseAdded {
		claudeResp.StopReason = "tool_use"
	}

	claudeBody, err := json.Marshal(claudeResp)
	if err != nil {
		return nil, ErrorWrapper(err, "marshal_claude_response_failed", http.StatusInternalServerError)
	}

	newResp := &http.Response{
		StatusCode: resp.StatusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(claudeBody)),
	}

	maps.Copy(newResp.Header, resp.Header)
	newResp.Header.Set("Content-Type", "application/json")
	newResp.Header.Set("Content-Length", fmt.Sprintf("%d", len(claudeBody)))

	return newResp, nil
}

// convertStreamingToClaudeResponse converts a streaming OpenAI response to Claude format.
func (a *Adaptor) convertStreamingToClaudeResponse(c *gin.Context, resp *http.Response, body []byte) (*http.Response, *model.ErrorWithStatusCode) {
	lg := gmw.GetLogger(c)
	reader, writer := io.Pipe()

	go func() {
		defer writer.Close()

		scanner := bufio.NewScanner(bytes.NewReader(body))
		helper.ConfigureScannerBuffer(scanner)
		for scanner.Scan() {
			line := scanner.Text()

			if after, ok := strings.CutPrefix(line, "data: "); ok {
				data := after

				if data == "[DONE]" {
					writer.Write([]byte("event: message_stop\n"))
					writer.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
					break
				}

				var chunk ChatCompletionsStreamResponse
				if err := json.Unmarshal([]byte(data), &chunk); err != nil {
					continue
				}

				if len(chunk.Choices) > 0 {
					choice := chunk.Choices[0]
					if choice.Delta.Content != nil {
						claudeChunk := map[string]any{
							"type":  "content_block_delta",
							"index": 0,
							"delta": map[string]any{
								"type": "text_delta",
								"text": choice.Delta.Content,
							},
						}

						claudeData, _ := json.Marshal(claudeChunk)
						writer.Write([]byte("event: content_block_delta\n"))
						writer.Write(fmt.Appendf(nil, "data: %s\n\n", claudeData))
					}
				}
			}
		}
		if err := scanner.Err(); err != nil {
			lg.Debug("error scanning OpenAI stream for Claude conversion",
				zap.Error(err),
				zap.Int("scanner_max_token_size", helper.DefaultScannerMaxTokenSize))
		}
	}()

	newResp := &http.Response{
		StatusCode: resp.StatusCode,
		Header:     make(http.Header),
		Body:       reader,
	}

	maps.Copy(newResp.Header, resp.Header)

	return newResp, nil
}
