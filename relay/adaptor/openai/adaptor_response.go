package openai

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
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/common/toolnamesafe"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// DoRequest sends the converted request upstream over the standard HTTP
// transport and returns the raw upstream response or the transport error.
func (a *Adaptor) DoRequest(c *gin.Context,
	meta *meta.Meta,
	requestBody io.Reader) (*http.Response, error) {
	// WebSocket proxying for /v1/responses is handled at the controller layer
	// (maybeHandleResponseAPIWebSocket) only when the downstream client itself
	// initiates a WebSocket upgrade. For regular HTTP POST requests we always
	// use the standard HTTP transport — it is more stable and avoids the extra
	// complexity of a WS→SSE bridge.
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

// DoResponse renders the upstream response for the client mode (including
// Claude Messages conversion) and returns the usage for billing or an error.
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
				// The Claude SSE rendering drops web_search_call items; count them
				// on the wire so the shared tooling billing sees the paid searches.
				var counter *webSearchStreamCounter
				if resp != nil && resp.Body != nil {
					counter = newWebSearchStreamCounter(resp.Body)
					resp.Body = counter
				}
				usage, convErr = openai_compatible.ConvertOpenAIStreamToClaudeSSE(c, resp, meta.PromptTokens, meta.ActualModelName)
				if counter != nil {
					counter.record(c)
				}
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

			completeNativeReceipt := meta.Mode == relaymode.ChatCompletions && c.GetBool(nativeChatReceiptCompleteKey)
			if usage == nil || (usage.TotalTokens == 0 && !completeNativeReceipt) {
				usage = ResponseText2Usage(responseText, meta.ActualModelName, meta.PromptTokens)
				usage.BillingEstimateReason = "stream_usage_missing_counters"
			}
			AnnotateResponseStreamUsage(c, usage)
			if usage.TotalTokens != 0 && usage.PromptTokens == 0 && !completeNativeReceipt {
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
			if meta.ChannelType == channeltype.SiliconFlow {
				err, usage = SiliconFlowImageHandler(c, resp)
			} else {
				err, usage = ImageHandler(c, resp)
			}
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
		// The buffered SSE rendering carries no receipt; the controller parses it.
		openai_compatible.SetConvertedClaudeUsage(c, nil)
		return a.convertStreamingToClaudeResponse(c, resp, body)
	}

	return a.convertNonStreamingToClaudeResponse(c, resp, body)
}

// convertNonStreamingToClaudeResponse converts a non-streaming OpenAI response
// (Responses object or Chat Completion) to a Claude Messages response. A body in
// neither shape is passed through unchanged for the controller's error handling.
func (a *Adaptor) convertNonStreamingToClaudeResponse(c *gin.Context, resp *http.Response, body []byte) (*http.Response, *model.ErrorWithStatusCode) {
	var responseAPIResp ResponseAPIResponse
	if err := json.Unmarshal(body, &responseAPIResp); err == nil && responseAPIResp.Object == "response" {
		return a.ConvertResponseAPIToClaudeResponse(c, resp, &responseAPIResp)
	}

	var openaiResp TextResponse
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		openai_compatible.SetConvertedClaudeUsage(c, nil)
		newResp := &http.Response{
			StatusCode: resp.StatusCode,
			Header:     resp.Header,
			Body:       io.NopCloser(bytes.NewReader(body)),
		}
		return newResp, nil
	}

	// Settle on the cache-aware receipt, normalized as the native Chat handler
	// does, so cached input is discounted exactly as on the streamed and native
	// /v1/chat/completions paths; the body presents it in Anthropic semantics.
	upstreamUsage := openaiResp.Usage
	upstreamUsage.NormalizeCachedTokens()
	upstreamUsage.NormalizeCacheWriteTokens()
	claudeResp := model.ClaudeResponse{
		ID:      openai_compatible.ClaudeMessageID(openaiResp.Id),
		Type:    "message",
		Role:    "assistant",
		Model:   openaiResp.Model,
		Content: []model.ClaudeContent{},
		Usage:   openai_compatible.PresentConvertedClaudeUsage(c, &upstreamUsage, openaiResp.Model),
	}

	ids := openai_compatible.NewClaudeToolIDAllocator()
	stopReason := ""
	for _, choice := range openaiResp.Choices {
		if thinking := openai_compatible.ChatMessageThinking(choice.Message); thinking != "" {
			claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{Type: "thinking", Thinking: thinking})
		}
		if choice.Message.Content != nil {
			switch content := choice.Message.Content.(type) {
			case string:
				if content != "" {
					claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{Type: "text", Text: content})
				}
			case []model.MessageContent:
				for _, part := range content {
					if part.Type == "text" && part.Text != nil && *part.Text != "" {
						claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{Type: "text", Text: *part.Text})
					}
				}
			}
		}

		if len(choice.Message.ToolCalls) > 0 {
			// Restore sanitized tool names so the client sees the names it sent.
			toolnamesafe.RestoreToolCallNames(c, choice.Message.ToolCalls)
			for _, toolCall := range choice.Message.ToolCalls {
				name := ""
				var args any
				if toolCall.Function != nil {
					name, args = toolCall.Function.Name, toolCall.Function.Arguments
				}
				claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{
					Type:  "tool_use",
					ID:    ids.Allocate(toolCall.Id),
					Name:  name,
					Input: openai_compatible.ClaudeToolInputFromArguments(args),
				})
			}
		}

		if choice.FinishReason != "" {
			stopReason = openai_compatible.ClaudeStopReasonFromChatFinish(choice.FinishReason)
		}
	}
	claudeResp.StopReason = openai_compatible.FinalizeClaudeStopReason(stopReason, openai_compatible.ClaudeContentHasToolUse(claudeResp.Content))

	return openai_compatible.MarshalClaudeHTTPResponse(resp, claudeResp)
}

// ConvertResponseAPIToClaudeResponse converts a Response API response to Claude
// Messages format. The stop_reason follows the response status (an incomplete
// max_output_tokens response maps to max_tokens) and becomes tool_use when the
// turn requests tools; tool ids, names, and inputs are normalized for Claude
// clients. It also records the upstream id, paid web searches, and the rendered
// assistant turn for the stateless-client checkpoint.
func (a *Adaptor) ConvertResponseAPIToClaudeResponse(c *gin.Context, resp *http.Response, responseAPIResp *ResponseAPIResponse) (*http.Response, *model.ErrorWithStatusCode) {
	// Surface the upstream Responses id for a stateless-client checkpoint (ST-022).
	if c != nil && responseAPIResp.Id != "" {
		c.Set(ctxkey.ResponseAPIUpstreamID, responseAPIResp.Id)
	}
	// The Claude rendering drops web_search_call items, so record the paid
	// searches for the shared tooling billing exactly as ResponseAPIHandler does.
	if c != nil {
		calls := countWebSearchSearchActions(responseAPIResp.Output)
		if derived, usedFallback := deriveWebSearchInvocationCount(calls, responseAPIResp.Usage); usedFallback {
			calls = derived
		}
		if calls > 0 {
			recordWebSearchInvocationCount(c, calls)
		}
	}
	// Settle on the cache-aware receipt the native Responses handler bills
	// (ToModelUsage), so cached input is discounted exactly as on the streamed
	// and native paths; the body presents it in Anthropic semantics.
	claudeResp := model.ClaudeResponse{
		ID:      openai_compatible.ClaudeMessageID(responseAPIResp.Id),
		Type:    "message",
		Role:    "assistant",
		Model:   responseAPIResp.Model,
		Content: []model.ClaudeContent{},
		Usage:   openai_compatible.PresentConvertedClaudeUsage(c, responseAPIResp.Usage.ToModelUsage(), responseAPIResp.Model),
	}

	ids := openai_compatible.NewClaudeToolIDAllocator()
	for _, outputItem := range responseAPIResp.Output {
		switch {
		case outputItem.Type == "reasoning":
			for _, summary := range outputItem.Summary {
				if summary.Type == "summary_text" && summary.Text != "" {
					claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{Type: "thinking", Thinking: summary.Text})
				}
			}
		case outputItem.Type == "message" && outputItem.Role == "assistant":
			for _, content := range outputItem.Content {
				if text := claudeResponseOutputText(content); text != "" {
					claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{Type: "text", Text: text})
				}
			}
		case outputItem.Type == "function_call":
			name := strings.TrimSpace(outputItem.Name)
			if name == "" {
				continue
			}
			id := outputItem.CallId
			if id == "" {
				id = outputItem.Id
			}
			claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{
				Type:  "tool_use",
				ID:    ids.Allocate(id),
				Name:  toolnamesafe.RestoreToolName(c, name),
				Input: openai_compatible.ClaudeToolInputFromArguments(outputItem.Arguments),
			})
		}
	}

	incompleteReason := ""
	if responseAPIResp.IncompleteDetails != nil {
		incompleteReason = responseAPIResp.IncompleteDetails.Reason
	}
	claudeResp.StopReason = openai_compatible.FinalizeClaudeStopReason(
		openai_compatible.ClaudeStopReasonFromResponseStatus(responseAPIResp.Status, incompleteReason),
		openai_compatible.ClaudeContentHasToolUse(claudeResp.Content),
	)

	// Surface the rendered assistant turn for a stateless-client checkpoint (ST-022),
	// as the client will echo the content blocks back on its next request.
	if c != nil {
		c.Set(ctxkey.ResponseAPIAssistantMessage, model.Message{Role: "assistant", Content: claudeResp.Content})
	}

	return openai_compatible.MarshalClaudeHTTPResponse(resp, claudeResp)
}

// claudeResponseOutputText returns the Claude text of a Responses message part:
// output_text verbatim, or trimmed output_json (raw JSON or its text form).
func claudeResponseOutputText(content OutputContent) string {
	switch content.Type {
	case "output_text":
		return content.Text
	case "output_json":
		if len(content.JSON) > 0 {
			return strings.TrimSpace(string(content.JSON))
		}
		return strings.TrimSpace(content.Text)
	default:
		return ""
	}
}

// convertStreamingToClaudeResponse converts a streaming OpenAI response to Claude format.
func (a *Adaptor) convertStreamingToClaudeResponse(c *gin.Context, resp *http.Response, body []byte) (*http.Response, *model.ErrorWithStatusCode) {
	lg := gmw.GetLogger(c)
	reader, writer := io.Pipe()

	go func() {
		defer writer.Close()
		write := func(data []byte) bool {
			if _, err := writer.Write(data); err != nil {
				lg.Debug("Claude stream downstream closed", zap.Error(err))
				return false
			}
			return true
		}

		lineReader := commonsse.NewLineReader(bytes.NewReader(body), commonsse.DefaultLineBufferSize)
		for {
			line, err := lineReader.Next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}

				lg.Debug("error reading OpenAI stream for Claude conversion", zap.Error(err))
				break
			}

			var lineText string
			if line.Oversized {
				payload, err := io.ReadAll(line.Large)
				if err != nil {
					lg.Debug("error reading oversized OpenAI stream payload for Claude conversion", zap.Error(err))
					continue
				}
				lineText = "data: " + string(payload)
			} else {
				lineText = line.Text()
			}

			if after, ok := strings.CutPrefix(lineText, "data: "); ok {
				data := after

				if data == "[DONE]" {
					if !write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")) {
						return
					}
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
						if !write(fmt.Appendf(nil, "event: content_block_delta\ndata: %s\n\n", claudeData)) {
							return
						}
					}
				}
			}
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
