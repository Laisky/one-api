package openai_compatible

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/render"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/model"
)

const (
	DataPrefix       = "data: "
	DataPrefixLength = len(DataPrefix)
	Done             = "[DONE]"
)

// ChatCompletionsStreamResponse represents the streaming response structure
type ChatCompletionsStreamResponse struct {
	Id      string                                `json:"id"`
	Object  string                                `json:"object"`
	Created int64                                 `json:"created"`
	Model   string                                `json:"model"`
	Choices []ChatCompletionsStreamResponseChoice `json:"choices"`
	Usage   *model.Usage                          `json:"usage,omitempty"`
}

type ChatCompletionsStreamResponseChoice struct {
	Index        int           `json:"index"`
	Delta        model.Message `json:"delta"`
	FinishReason *string       `json:"finish_reason"`
}

// SlimTextResponse represents the non-streaming response structure
type SlimTextResponse struct {
	Choices     []TextResponseChoice `json:"choices"`
	model.Usage `json:"usage"`
	Error       model.Error `json:"error"`
}

type TextResponseChoice struct {
	Index        int           `json:"index"`
	Message      model.Message `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// NormalizeDataLine normalizes SSE data lines
func NormalizeDataLine(data string) string {
	if strings.HasPrefix(data, "data:") {
		content := strings.TrimLeft(data[len("data:"):], " ")
		return "data: " + content
	}
	return data
}

// ErrorWrapper creates an error response
func ErrorWrapper(err error, code string, statusCode int) *model.ErrorWithStatusCode {
	return &model.ErrorWithStatusCode{
		Error: model.Error{
			Message: err.Error(),
			Type:    "one_api_error",
			Code:    code,
		},
		StatusCode: statusCode,
	}
}

// CountTokenText estimates token count (simplified implementation)
func CountTokenText(text string, modelName string) int {
	// Simple estimation: ~4 characters per token
	return len(text) / 4
}

// GetFullRequestURL constructs the full request URL for OpenAI-compatible APIs
func GetFullRequestURL(baseURL string, requestURL string, channelType int) string {
	if channelType == channeltype.OpenAICompatible {
		return fmt.Sprintf("%s%s", strings.TrimSuffix(baseURL, "/"), strings.TrimPrefix(requestURL, "/v1"))
	}
	return fmt.Sprintf("%s%s", baseURL, requestURL)
}

// StreamHandler processes streaming responses from OpenAI-compatible APIs
func StreamHandler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	responseText := ""
	toolArgsText := ""
	var usage *model.Usage

	logger := gmw.GetLogger(c).With(
		zap.String("model", modelName),
	)

	// Check if response content type indicates an error (non-streaming response)
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") &&
		!strings.Contains(contentType, "text/event-stream") {
		logger.Error("unexpected content type for streaming request, possible error response",
			zap.String("content_type", contentType),
			zap.Int("status_code", resp.StatusCode))

		// Read response as potential error
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return ErrorWrapper(err, "read_error_response_failed", http.StatusInternalServerError), nil
		}

		logger.Error("received error response in stream handler",
			zap.ByteString("response_body", responseBody))

		// Try to parse as error response
		var errorResponse SlimTextResponse
		if err := json.Unmarshal(responseBody, &errorResponse); err == nil && errorResponse.Error.Type != "" {
			return &model.ErrorWithStatusCode{
				Error:      errorResponse.Error,
				StatusCode: resp.StatusCode,
			}, nil
		}

		// Return generic error if parsing fails
		return ErrorWrapper(errors.Errorf("unexpected non-streaming response: %s", string(responseBody)),
			"unexpected_response_format", resp.StatusCode), nil
	}

	scanner := bufio.NewScanner(resp.Body)
	buffer := make([]byte, 1024*1024) // 1MB buffer
	scanner.Buffer(buffer, len(buffer))
	scanner.Split(bufio.ScanLines)

	common.SetEventStreamHeaders(c)
	doneRendered := false
	chunksProcessed := 0

	for scanner.Scan() {
		data := NormalizeDataLine(scanner.Text())
		logger.Debug("processing streaming chunk",
			zap.String("chunk_data", data),
			zap.Int("chunks_processed", chunksProcessed))

		if len(data) < DataPrefixLength {
			continue
		}

		if data[:DataPrefixLength] != DataPrefix && data[:DataPrefixLength] != Done {
			continue
		}

		if strings.HasPrefix(data[DataPrefixLength:], Done) {
			render.StringData(c, data)
			doneRendered = true
			continue
		}

		// Parse the streaming chunk
		var streamResponse ChatCompletionsStreamResponse
		jsonData := data[DataPrefixLength:]
		if err := json.Unmarshal([]byte(jsonData), &streamResponse); err != nil {
			logger.Warn("failed to parse streaming chunk, skipping",
				zap.String("chunk_data", jsonData),
				zap.Error(err))
			continue // Skip malformed chunks
		}

		chunksProcessed++

		// Accumulate response text and tool call arguments
		for _, choice := range streamResponse.Choices {
			responseText += choice.Delta.StringContent()
			if len(choice.Delta.ToolCalls) > 0 {
				for _, tc := range choice.Delta.ToolCalls {
					if tc.Function != nil && tc.Function.Arguments != nil {
						switch v := tc.Function.Arguments.(type) {
						case string:
							toolArgsText += v
						default:
							if b, e := json.Marshal(v); e == nil {
								toolArgsText += string(b)
							}
						}
					}
				}
			}
		}

		// Accumulate usage information
		if streamResponse.Usage != nil {
			usage = streamResponse.Usage
		}

		// Forward the chunk to client
		render.StringData(c, data)
	}

	if err := scanner.Err(); err != nil {
		return ErrorWrapper(err, "read_stream_failed", http.StatusInternalServerError), usage
	}

	// Check if we processed any chunks - if not, this might indicate an error
	if chunksProcessed == 0 && responseText == "" {
		logger.Error("stream processing completed but no chunks were processed",
			zap.String("model", modelName),
			zap.String("content_type", resp.Header.Get("Content-Type")))
		return ErrorWrapper(errors.Errorf("no streaming data received from upstream"),
			"empty_stream_response", http.StatusInternalServerError), usage
	}

	if !doneRendered {
		render.StringData(c, "data: "+Done)
	}

	if err := resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), usage
	}

	// Calculate or fix usage when missing or incomplete
	if usage == nil {
		// No usage provided by upstream: compute from text
		logger.Warn("no usage provided by upstream, computing token count using CountTokenText fallback",
			zap.String("model", modelName),
			zap.Int("response_text_len", len(responseText)),
			zap.Int("tool_args_len", len(toolArgsText)))
		computed := CountTokenText(responseText, modelName) + CountTokenText(toolArgsText, modelName)
		usage = &model.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: computed,
			TotalTokens:      promptTokens + computed,
		}
		logger.Debug("computed usage for stream (no upstream usage)",
			zap.Int("prompt_tokens", usage.PromptTokens),
			zap.Int("completion_tokens", usage.CompletionTokens),
			zap.Int("total_tokens", usage.TotalTokens),
			zap.Int("response_text_len", len(responseText)),
			zap.Int("tool_args_len", len(toolArgsText)))
	} else {
		// Upstream provided some usage; fill missing parts
		if usage.PromptTokens == 0 {
			usage.PromptTokens = promptTokens
		}
		if usage.CompletionTokens == 0 {
			logger.Warn("no completion tokens provided by upstream, computing using CountTokenText fallback",
				zap.String("model", modelName),
				zap.Int("response_text_len", len(responseText)),
				zap.Int("tool_args_len", len(toolArgsText)))
			usage.CompletionTokens = CountTokenText(responseText, modelName) + CountTokenText(toolArgsText, modelName)
		}
		if usage.TotalTokens == 0 {
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
		logger.Debug("finalized usage for stream (with upstream usage)",
			zap.Int("prompt_tokens", usage.PromptTokens),
			zap.Int("completion_tokens", usage.CompletionTokens),
			zap.Int("total_tokens", usage.TotalTokens),
			zap.Int("response_text_len", len(responseText)),
			zap.Int("tool_args_len", len(toolArgsText)))
	}

	return nil, usage
}

// EmbeddingHandler processes embedding responses from OpenAI-compatible APIs
func EmbeddingHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	logger := gmw.GetLogger(c)
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}

	logger.Debug("receive embedding response from upstream channel", zap.ByteString("response_body", responseBody))
	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	// Check if response body is empty
	if len(responseBody) == 0 {
		logger.Error("received empty embedding response body from upstream",
			zap.Int("status_code", resp.StatusCode))
		return ErrorWrapper(errors.Errorf("empty response body from upstream"),
			"empty_response_body", http.StatusInternalServerError), nil
	}

	// Parse the embedding response to validate structure and extract usage
	var embeddingResponse struct {
		Object string `json:"object"`
		Data   []struct {
			Object    string    `json:"object"`
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Model string      `json:"model"`
		Usage model.Usage `json:"usage"`
		Error model.Error `json:"error"`
	}

	if err = json.Unmarshal(responseBody, &embeddingResponse); err != nil {
		logger.Error("failed to unmarshal embedding response body",
			zap.ByteString("response_body", responseBody),
			zap.Error(err))
		return ErrorWrapper(err, "unmarshal_embedding_response_failed", http.StatusInternalServerError), nil
	}

	// Check for API error in response
	if embeddingResponse.Error.Type != "" {
		logger.Debug("upstream returned embedding error response",
			zap.String("error_type", embeddingResponse.Error.Type),
			zap.String("error_message", embeddingResponse.Error.Message))
		return &model.ErrorWithStatusCode{
			Error:      embeddingResponse.Error,
			StatusCode: resp.StatusCode,
		}, nil
	}

	// Check if response has data - empty data might indicate an error
	if len(embeddingResponse.Data) == 0 {
		logger.Error("embedding response has no data, possible upstream error",
			zap.ByteString("response_body", responseBody))
		return ErrorWrapper(errors.Errorf("no embedding data in response from upstream"),
			"no_embedding_data", http.StatusInternalServerError), nil
	}

	// Forward the response to client
	c.Header("Content-Type", "application/json")
	c.Status(resp.StatusCode)
	c.Writer.Write(responseBody)

	// Return usage information
	usage := embeddingResponse.Usage
	if usage.TotalTokens == 0 && usage.PromptTokens > 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	logger.Debug("finalized usage for embedding (openai-compatible)",
		zap.Int("prompt_tokens", usage.PromptTokens),
		zap.Int("completion_tokens", usage.CompletionTokens),
		zap.Int("total_tokens", usage.TotalTokens))

	return nil, &usage
}

// Handler processes non-streaming responses from OpenAI-compatible APIs
func Handler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	logger := gmw.GetLogger(c)
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}

	logger.Debug("receive from upstream channel", zap.ByteString("response_body", responseBody))
	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	// Check if response body is empty
	if len(responseBody) == 0 {
		logger.Error("received empty response body from upstream",
			zap.String("model", modelName),
			zap.Int("status_code", resp.StatusCode))
		return ErrorWrapper(errors.Errorf("empty response body from upstream"),
			"empty_response_body", http.StatusInternalServerError), nil
	}

	var textResponse SlimTextResponse
	if err = json.Unmarshal(responseBody, &textResponse); err != nil {
		logger.Error("failed to unmarshal response body",
			zap.ByteString("response_body", responseBody),
			zap.Error(err))
		return ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	if textResponse.Error.Type != "" {
		logger.Debug("upstream returned error response",
			zap.String("error_type", textResponse.Error.Type),
			zap.String("error_message", textResponse.Error.Message))
		return &model.ErrorWithStatusCode{
			Error:      textResponse.Error,
			StatusCode: resp.StatusCode,
		}, nil
	}

	// Check if response has choices - empty choices might indicate an error
	if len(textResponse.Choices) == 0 {
		logger.Error("response has no choices, possible upstream error",
			zap.String("model", modelName),
			zap.ByteString("response_body", responseBody))
		return ErrorWrapper(errors.Errorf("no choices in response from upstream"),
			"no_choices_in_response", http.StatusInternalServerError), nil
	}

	// Forward the response to client
	c.JSON(resp.StatusCode, textResponse)

	// Calculate usage if not provided
	usage := textResponse.Usage
	if usage.PromptTokens == 0 {
		usage.PromptTokens = promptTokens
	}
	if usage.CompletionTokens == 0 {
		// Calculate completion tokens from response text and tool call arguments
		responseText := ""
		toolArgsText := ""
		for _, choice := range textResponse.Choices {
			responseText += choice.Message.StringContent()
			if len(choice.Message.ToolCalls) > 0 {
				for _, tc := range choice.Message.ToolCalls {
					if tc.Function != nil && tc.Function.Arguments != nil {
						switch v := tc.Function.Arguments.(type) {
						case string:
							toolArgsText += v
						default:
							if b, e := json.Marshal(v); e == nil {
								toolArgsText += string(b)
							}
						}
					}
				}
			}
		}
		logger.Warn("no completion tokens provided by upstream, computing using CountTokenText fallback",
			zap.String("model", modelName),
			zap.Int("response_text_len", len(responseText)),
			zap.Int("tool_args_len", len(toolArgsText)))
		usage.CompletionTokens = CountTokenText(responseText, modelName) + CountTokenText(toolArgsText, modelName)
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	logger.Debug("finalized usage for non-stream (openai-compatible)",
		zap.Int("prompt_tokens", usage.PromptTokens),
		zap.Int("completion_tokens", usage.CompletionTokens),
		zap.Int("total_tokens", usage.TotalTokens))

	return nil, &usage
}

// ExtractThinkingContent extracts content within the FIRST <think></think> tag only and returns
// both the extracted thinking content and the remaining regular content
// This high-performance implementation uses fast string scanning instead of regex for optimal latency
//
// NOTE: Only processes the first think tag encountered; subsequent think tags are treated as regular content
func ExtractThinkingContent(content string) (thinkingContent, regularContent string) {
	if content == "" {
		return "", ""
	}

	// Fast string-based parsing - no regex for maximum performance
	// Only handle the FIRST think tag, treat subsequent ones as regular content

	// Look for the first <think> tag
	thinkStart := strings.Index(content, "<think>")
	if thinkStart == -1 {
		// No <think> tag found, return all content as regular
		return "", strings.TrimSpace(content)
	}

	// Look for the first closing </think> tag after the opening tag
	thinkEnd := strings.Index(content[thinkStart:], "</think>")
	if thinkEnd == -1 {
		// No closing tag found, treat all as regular content
		return "", strings.TrimSpace(content)
	}

	// Adjust thinkEnd to absolute position
	thinkEnd += thinkStart

	// Extract thinking content (between <think> and </think>)
	thinkingStart := thinkStart + 7 // len("<think>")
	if thinkingStart < thinkEnd {
		thinkingContent = content[thinkingStart:thinkEnd]
	}

	// Build regular content: before first <think> + after first </think>
	beforeThink := content[:thinkStart]
	afterThink := content[thinkEnd+8:] // 8 is len("</think>")
	regularContent = beforeThink + afterThink

	// Clean up whitespace
	thinkingContent = strings.TrimSpace(thinkingContent)
	regularContent = strings.TrimSpace(regularContent)

	return thinkingContent, regularContent
}

// StreamHandlerWithThinking processes streaming responses with ultra-low-latency <think></think> block processing
// This specialized handler is designed for Other provider sequential thinking format where thinking content
// comes first, followed by the actual response content.
//
// Note: This now Optimized for efficiency with large streams using [strings.Builder] to avoid O(n²) string concatenation.
func StreamHandlerWithThinking(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	// Use strings.Builder for efficient text accumulation to avoid O(n²) complexity with large streams
	var responseTextBuilder strings.Builder
	var toolArgsTextBuilder strings.Builder
	var usage *model.Usage

	// Ultra-low-latency streaming state - minimal variables for maximum performance
	isInThinkingBlock := false    // Track if we're currently inside a <think> block
	hasProcessedThinkTag := false // Track if we've already processed the first (and only) think tag

	logger := gmw.GetLogger(c).With(
		zap.String("model", modelName),
	)

	// Check if response content type indicates an error (non-streaming response)
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") &&
		!strings.Contains(contentType, "text/event-stream") {
		logger.Error("unexpected content type for streaming request, possible error response",
			zap.String("content_type", contentType),
			zap.Int("status_code", resp.StatusCode))

		// Read response as potential error
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return ErrorWrapper(err, "read_error_response_failed", http.StatusInternalServerError), nil
		}

		logger.Error("received error response in stream handler",
			zap.ByteString("response_body", responseBody))

		// Try to parse as error response
		var errorResponse SlimTextResponse
		if err := json.Unmarshal(responseBody, &errorResponse); err == nil && errorResponse.Error.Type != "" {
			return &model.ErrorWithStatusCode{
				Error:      errorResponse.Error,
				StatusCode: resp.StatusCode,
			}, nil
		}

		// Return generic error if parsing fails
		return ErrorWrapper(errors.Errorf("unexpected non-streaming response: %s", string(responseBody)),
			"unexpected_response_format", resp.StatusCode), nil
	}

	scanner := bufio.NewScanner(resp.Body)
	buffer := make([]byte, 1024*1024) // 1MB buffer
	scanner.Buffer(buffer, len(buffer))
	scanner.Split(bufio.ScanLines)

	common.SetEventStreamHeaders(c)
	doneRendered := false
	chunksProcessed := 0

	for scanner.Scan() {
		data := NormalizeDataLine(scanner.Text())
		logger.Debug("processing streaming chunk",
			zap.String("chunk_data", data),
			zap.Int("chunks_processed", chunksProcessed))

		if len(data) < DataPrefixLength {
			continue
		}

		if data[:DataPrefixLength] != DataPrefix && data[:DataPrefixLength] != Done {
			continue
		}

		if strings.HasPrefix(data[DataPrefixLength:], Done) {
			render.StringData(c, data)
			doneRendered = true
			continue
		}

		// Parse the streaming chunk
		var streamResponse ChatCompletionsStreamResponse
		jsonData := data[DataPrefixLength:]
		if err := json.Unmarshal([]byte(jsonData), &streamResponse); err != nil {
			logger.Warn("failed to parse streaming chunk, skipping",
				zap.String("chunk_data", jsonData),
				zap.Error(err))
			continue // Skip malformed chunks
		}

		chunksProcessed++

		// Replace upstream ID with our trace ID
		streamResponse.Id = fmt.Sprintf("chatcmpl-oneapi-%s", tracing.GetTraceID(c))

		// Ultra-low-latency content processing - no buffering approach
		originalData := data
		modifiedChunk := true // Always mark as modified since we changed the ID

		// Process each choice with immediate streaming
		for i, choice := range streamResponse.Choices {
			deltaContent := choice.Delta.StringContent()
			responseTextBuilder.WriteString(deltaContent)

			// Process content immediately without accumulation
			if deltaContent != "" {
				// Ultra-fast thinking block detection and processing - only process FIRST think tag
				if !isInThinkingBlock && !hasProcessedThinkTag {
					if idx := strings.Index(deltaContent, "<think>"); idx >= 0 {
						// Entering thinking block
						isInThinkingBlock = true
						beforeThink := deltaContent[:idx]
						afterThink := deltaContent[idx+7:] // 7 is len("<think>")

						// Set regular content before <think>
						streamResponse.Choices[i].Delta.Content = beforeThink
						modifiedChunk = true

						// Handle thinking content after <think>
						if strings.Contains(afterThink, "</think>") {
							// Complete thinking block in single chunk
							if endIdx := strings.Index(afterThink, "</think>"); endIdx >= 0 {
								thinkingContent := afterThink[:endIdx]
								afterEndThink := afterThink[endIdx+8:] // 8 is len("</think>")

								// Stream thinking content immediately
								if thinkingContent != "" {
									// Now, it is explicitly assigned to the ReasoningContent field.
									streamResponse.Choices[i].Delta.ReasoningContent = &thinkingContent
								}

								// Append regular content after </think>
								if afterEndThink != "" {
									currentContent := streamResponse.Choices[i].Delta.Content.(string)
									streamResponse.Choices[i].Delta.Content = currentContent + afterEndThink
								}

								isInThinkingBlock = false
								hasProcessedThinkTag = true // Mark that we've processed the first (and only) think tag
							}
						} else {
							// Partial thinking content - stream immediately
							if afterThink != "" {
								// Now, it is explicitly assigned to the ReasoningContent field.
								streamResponse.Choices[i].Delta.ReasoningContent = &afterThink
							}
						}
					}
				} else if isInThinkingBlock {
					// Inside thinking block - process immediately
					if strings.Contains(deltaContent, "</think>") {
						// End of thinking block
						if idx := strings.Index(deltaContent, "</think>"); idx >= 0 {
							beforeEndThink := deltaContent[:idx]
							afterEndThink := deltaContent[idx+8:] // 8 is len("</think>")

							// Stream remaining thinking content
							if beforeEndThink != "" {
								// Now, it is explicitly assigned to the ReasoningContent field.
								streamResponse.Choices[i].Delta.ReasoningContent = &beforeEndThink
							}

							// Set regular content after </think>
							streamResponse.Choices[i].Delta.Content = afterEndThink
							modifiedChunk = true
							isInThinkingBlock = false
							hasProcessedThinkTag = true // Mark that we've processed the first (and only) think tag
						}
					} else {
						// Pure thinking content - stream as reasoning immediately
						// Now, it is explicitly assigned to the ReasoningContent field.
						streamResponse.Choices[i].Delta.ReasoningContent = &deltaContent
						streamResponse.Choices[i].Delta.Content = ""
						modifiedChunk = true
					}
				}
				// Note: Removed fallback processing to eliminate latency
			}

			if len(choice.Delta.ToolCalls) > 0 {
				for _, tc := range choice.Delta.ToolCalls {
					if tc.Function != nil && tc.Function.Arguments != nil {
						switch v := tc.Function.Arguments.(type) {
						case string:
							toolArgsTextBuilder.WriteString(v)
						default:
							if b, e := json.Marshal(v); e == nil {
								toolArgsTextBuilder.Write(b)
							}
						}
					}
				}
			}
		}

		// Accumulate usage information
		if streamResponse.Usage != nil {
			usage = streamResponse.Usage
		}

		// Forward the chunk to client (modified or original)
		if modifiedChunk {
			// Re-serialize the modified response
			if modifiedJSON, err := json.Marshal(streamResponse); err == nil {
				render.StringData(c, "data: "+string(modifiedJSON))
			} else {
				// Fallback to original data if serialization fails
				render.StringData(c, originalData)
			}
		} else {
			render.StringData(c, originalData)
		}
	}

	if err := scanner.Err(); err != nil {
		return ErrorWrapper(err, "read_stream_failed", http.StatusInternalServerError), usage
	}

	// Check if we processed any chunks - if not, this might indicate an error
	if chunksProcessed == 0 && responseTextBuilder.Len() == 0 {
		logger.Error("stream processing completed but no chunks were processed",
			zap.String("model", modelName),
			zap.String("content_type", resp.Header.Get("Content-Type")))
		return ErrorWrapper(errors.Errorf("no streaming data received from upstream"),
			"empty_stream_response", http.StatusInternalServerError), usage
	}

	if !doneRendered {
		render.StringData(c, "data: "+Done)
	}

	if err := resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), usage
	}

	// Calculate or fix usage when missing or incomplete
	responseText := responseTextBuilder.String()
	toolArgsText := toolArgsTextBuilder.String()
	if usage == nil {
		// No usage provided by upstream: compute from text
		logger.Warn("no usage provided by upstream in thinking stream, computing token count using CountTokenText fallback",
			zap.String("model", modelName),
			zap.Int("response_text_len", len(responseText)),
			zap.Int("tool_args_len", len(toolArgsText)))
		computed := CountTokenText(responseText, modelName) + CountTokenText(toolArgsText, modelName)
		usage = &model.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: computed,
			TotalTokens:      promptTokens + computed,
		}
		logger.Debug("computed usage for stream (no upstream usage)",
			zap.Int("prompt_tokens", usage.PromptTokens),
			zap.Int("completion_tokens", usage.CompletionTokens),
			zap.Int("total_tokens", usage.TotalTokens),
			zap.Int("response_text_len", len(responseText)),
			zap.Int("tool_args_len", len(toolArgsText)))
	} else {
		// Upstream provided some usage; fill missing parts
		if usage.PromptTokens == 0 {
			usage.PromptTokens = promptTokens
		}
		if usage.CompletionTokens == 0 {
			logger.Warn("no completion tokens provided by upstream in thinking stream, computing using CountTokenText fallback",
				zap.String("model", modelName),
				zap.Int("response_text_len", len(responseText)),
				zap.Int("tool_args_len", len(toolArgsText)))
			usage.CompletionTokens = CountTokenText(responseText, modelName) + CountTokenText(toolArgsText, modelName)
		}
		if usage.TotalTokens == 0 {
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
		logger.Debug("finalized usage for stream (with upstream usage)",
			zap.Int("prompt_tokens", usage.PromptTokens),
			zap.Int("completion_tokens", usage.CompletionTokens),
			zap.Int("total_tokens", usage.TotalTokens),
			zap.Int("response_text_len", len(responseText)),
			zap.Int("tool_args_len", len(toolArgsText)))
	}

	return nil, usage
}

// HandlerWithThinking processes non-streaming responses with <think></think> block extraction
// This handler uses high-performance string parsing to extract thinking content from Other provider responses
func HandlerWithThinking(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	logger := gmw.GetLogger(c)
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}

	logger.Debug("receive from upstream channel", zap.ByteString("response_body", responseBody))
	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	// Check if response body is empty
	if len(responseBody) == 0 {
		logger.Error("received empty response body from upstream",
			zap.String("model", modelName),
			zap.Int("status_code", resp.StatusCode))
		return ErrorWrapper(errors.Errorf("empty response body from upstream"),
			"empty_response_body", http.StatusInternalServerError), nil
	}

	var textResponse SlimTextResponse
	if err = json.Unmarshal(responseBody, &textResponse); err != nil {
		logger.Error("failed to unmarshal response body",
			zap.ByteString("response_body", responseBody),
			zap.Error(err))
		return ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	if textResponse.Error.Type != "" {
		logger.Debug("upstream returned error response",
			zap.String("error_type", textResponse.Error.Type),
			zap.String("error_message", textResponse.Error.Message))
		return &model.ErrorWithStatusCode{
			Error:      textResponse.Error,
			StatusCode: resp.StatusCode,
		}, nil
	}

	// Check if response has choices - empty choices might indicate an error
	if len(textResponse.Choices) == 0 {
		logger.Error("response has no choices, possible upstream error",
			zap.String("model", modelName),
			zap.ByteString("response_body", responseBody))
		return ErrorWrapper(errors.Errorf("no choices in response from upstream"),
			"no_choices_in_response", http.StatusInternalServerError), nil
	}

	// Process response choices to extract thinking content
	for i, choice := range textResponse.Choices {
		messageContent := choice.Message.StringContent()
		if messageContent != "" {
			// Extract thinking content from the message
			thinkingContent, cleanContent := ExtractThinkingContent(messageContent)

			if thinkingContent != "" {
				// Set the reasoning field if thinking content was found
				// Now, it is explicitly assigned to the ReasoningContent field.
				if textResponse.Choices[i].Message.ReasoningContent == nil {
					textResponse.Choices[i].Message.ReasoningContent = &thinkingContent
				}

				// Update the message content to be clean content (without <think> tags)
				textResponse.Choices[i].Message.Content = cleanContent
			}
		}
	}

	// Forward the modified response to client
	c.JSON(resp.StatusCode, textResponse)

	// Calculate usage if not provided
	usage := textResponse.Usage
	if usage.PromptTokens == 0 {
		usage.PromptTokens = promptTokens
	}
	if usage.CompletionTokens == 0 {
		// Calculate completion tokens from response text and tool call arguments
		responseText := ""
		toolArgsText := ""
		for _, choice := range textResponse.Choices {
			responseText += choice.Message.StringContent()
			if len(choice.Message.ToolCalls) > 0 {
				for _, tc := range choice.Message.ToolCalls {
					if tc.Function != nil && tc.Function.Arguments != nil {
						switch v := tc.Function.Arguments.(type) {
						case string:
							toolArgsText += v
						default:
							if b, e := json.Marshal(v); e == nil {
								toolArgsText += string(b)
							}
						}
					}
				}
			}
		}
		logger.Warn("no completion tokens provided by upstream in thinking handler, computing using CountTokenText fallback",
			zap.String("model", modelName),
			zap.Int("response_text_len", len(responseText)),
			zap.Int("tool_args_len", len(toolArgsText)))
		usage.CompletionTokens = CountTokenText(responseText, modelName) + CountTokenText(toolArgsText, modelName)
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	logger.Debug("finalized usage for non-stream with thinking",
		zap.Int("prompt_tokens", usage.PromptTokens),
		zap.Int("completion_tokens", usage.CompletionTokens),
		zap.Int("total_tokens", usage.TotalTokens))

	return nil, &usage
}
