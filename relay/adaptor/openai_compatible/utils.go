package openai_compatible

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

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
//
// Now uses the unified architecture for consistent performance and memory allocation patterns
func StreamHandler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	// Respect URL param `thinking` to enable <think></think> extraction for compatible providers
	enableThinking := isThinkingEnabled(c.Query("thinking"))
	return UnifiedStreamProcessing(c, resp, promptTokens, modelName, enableThinking)
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

	// Optionally extract <think> blocks when enabled via URL param and map to requested field
	if isThinkingEnabled(c.Query("thinking")) {
		for i := range textResponse.Choices {
			msg := &textResponse.Choices[i].Message
			content := msg.StringContent()
			if content == "" {
				continue
			}
			thinkingContent, clean := ExtractThinkingContent(content)
			if thinkingContent != "" {
				msg.SetReasoningContent(c.Query("reasoning_format"), thinkingContent)
				msg.Content = clean
			}
		}
	}

	// Forward the (possibly modified) response to client
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
// and supports both normal tags (<think></think>) and Unicode-escaped tags (\u003cthink\u003e)
//
// NOTE: Only processes the first think tag encountered; subsequent think tags are treated as regular content
func ExtractThinkingContent(content string) (thinkingContent, regularContent string) {
	if content == "" {
		return "", ""
	}

	// Fast string-based parsing - no regex for maximum performance
	// Only handle the FIRST think tag, treat subsequent ones as regular content
	// Support both normal and Unicode-escaped tags

	thinkStart, thinkEnd, openTagLen, closeTagLen := findFirstThinkTag(content)
	if thinkStart == -1 {
		// No think tag found, return all content as regular
		return "", strings.TrimSpace(content)
	}

	// Extract thinking content (between opening and closing tags)
	thinkingStart := thinkStart + openTagLen
	if thinkingStart < thinkEnd {
		thinkingContent = content[thinkingStart:thinkEnd]
	}

	// Build regular content: before first tag + after first tag
	beforeThink := content[:thinkStart]
	afterThink := content[thinkEnd+closeTagLen:]
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
	return UnifiedStreamProcessing(c, resp, promptTokens, modelName, true)
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
				// Map reasoning to the requested format field and clear source tag from content
				textResponse.Choices[i].Message.SetReasoningContent(c.Query("reasoning_format"), thinkingContent)

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

// isThinkingEnabled returns true when the `thinking` query param is a truthy value.
// Accepts: "1", "true", "yes", "on" (case-insensitive)
func isThinkingEnabled(val string) bool {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// findFirstThinkTag finds the first occurrence of either normal or Unicode-escaped thinking tags
// Returns the start position, end position (exclusive), and tag lengths for optimal processing
func findFirstThinkTag(content string) (startPos, endPos, openTagLen, closeTagLen int) {
	// Check for normal tags first
	normalStart := strings.Index(content, "<think>")
	normalEnd := -1
	if normalStart != -1 {
		normalEnd = strings.Index(content[normalStart:], "</think>")
		if normalEnd != -1 {
			normalEnd += normalStart
		}
	}

	// Check for Unicode-escaped tags
	unicodeStart := strings.Index(content, "\\u003cthink\\u003e")
	unicodeEnd := -1
	if unicodeStart != -1 {
		unicodeEnd = strings.Index(content[unicodeStart:], "\\u003c/think\\u003e")
		if unicodeEnd != -1 {
			unicodeEnd += unicodeStart
		}
	}

	// Determine which tag comes first (if any)
	if normalStart != -1 && normalEnd != -1 && (unicodeStart == -1 || normalStart < unicodeStart) {
		// Normal tag comes first (or only normal tag exists)
		return normalStart, normalEnd, 7, 8 // len("<think>"), len("</think>")
	} else if unicodeStart != -1 && unicodeEnd != -1 {
		// Unicode tag comes first (or only Unicode tag exists)
		return unicodeStart, unicodeEnd, 17, 18 // len("\\u003cthink\\u003e"), len("\\u003c/think\\u003e")
	}

	// No valid tag pair found
	return -1, -1, 0, 0
}

// findOpeningThinkTag finds the first occurrence of either normal or Unicode-escaped opening tag
// Returns position and tag length, or -1 if not found
func findOpeningThinkTag(content string) (pos, tagLen int) {
	normalPos := strings.Index(content, "<think>")
	unicodePos := strings.Index(content, "\\u003cthink\\u003e")

	if normalPos >= 0 && (unicodePos < 0 || normalPos < unicodePos) {
		return normalPos, 7 // len("<think>")
	} else if unicodePos >= 0 {
		return unicodePos, 17 // len("\\u003cthink\\u003e")
	}

	return -1, 0
}

// findClosingThinkTag finds the first occurrence of either normal or Unicode-escaped closing tag
// Returns position and tag length, or -1 if not found
func findClosingThinkTag(content string) (pos, tagLen int) {
	normalPos := strings.Index(content, "</think>")
	unicodePos := strings.Index(content, "\\u003c/think\\u003e")

	if normalPos >= 0 && (unicodePos < 0 || normalPos < unicodePos) {
		return normalPos, 8 // len("</think>")
	} else if unicodePos >= 0 {
		return unicodePos, 18 // len("\\u003c/think\\u003e")
	}

	return -1, 0
}
