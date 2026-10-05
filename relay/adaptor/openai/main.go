package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/tracing"
	relaymodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/common/toolnamesafe"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

var errUpstreamEmbeddingResponse = errors.New("upstream embedding response error")

// Use shared constants from openai_compatible package
const (
	dataPrefix       = openai_compatible.DataPrefix
	done             = openai_compatible.Done
	dataPrefixLength = openai_compatible.DataPrefixLength
)

// Optionally: record when upstream streaming is completed (non-standard event)
func recordUpstreamCompleted(c *gin.Context) {
	// Only attempt to record trace timestamp when DB is initialized. In tests or
	// lightweight environments the global DB may be nil which would cause a
	// panic inside the model package. Guard to keep handler robust.
	if relaymodel.DB == nil {
		return
	}
	tracing.RecordTraceTimestamp(c, relaymodel.TimestampUpstreamCompleted)
}

// Handler processes non-streaming responses from OpenAI API
// Returns error (if any) and token usage information
func Handler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	logger := gmw.GetLogger(c)
	// Read the entire response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}

	// Close the original response body
	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	// Log the upstream response before any transformation so troubleshooting retains full context
	fields := []zap.Field{
		zap.Int("status_code", resp.StatusCode),
		zap.Int("body_bytes", len(responseBody)),
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		fields = append(fields, zap.String("content_type", contentType))
	}
	fields = append(fields, zap.Bool("body_logging_suppressed", true))
	logger.Debug("receive upstream response", fields...)

	// Parse the response JSON
	var textResponse SlimTextResponse
	if err = json.Unmarshal(responseBody, &textResponse); err != nil {
		return ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	// Check for API errors
	if textResponse.Error != nil && textResponse.Error.Type != "" {
		return &model.ErrorWithStatusCode{
			Error:      *textResponse.Error,
			StatusCode: resp.StatusCode,
		}, nil
	}

	// Forward responses that are not ChatCompletions when upstream omits choices without mutating the payload
	if len(textResponse.Choices) == 0 {
		logger.Debug("handler forwarding raw upstream response", zap.Int("status_code", resp.StatusCode))
		resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))

		for k, values := range resp.Header {
			for _, v := range values {
				c.Writer.Header().Add(k, v)
			}
		}

		c.Writer.WriteHeader(resp.StatusCode)
		if _, err = io.Copy(c.Writer, resp.Body); err != nil {
			return ErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError), nil
		}

		if err = resp.Body.Close(); err != nil {
			return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
		}

		return nil, nil
	}

	// Process reasoning content in each choice
	reasoningFormat := c.Query("reasoning_format")
	toolNamesRestored := false
	for i := range textResponse.Choices {
		choice := &textResponse.Choices[i]
		reasoningContent := processReasoningContent(choice)

		// Set reasoning in requested format if content exists
		if reasoningContent != "" {
			choice.SetReasoningContent(reasoningFormat, reasoningContent)
		}

		// Restore any sanitized tool names back to client-facing originals.
		if toolnamesafe.RestoreToolCallNames(c, choice.ToolCalls) {
			toolNamesRestored = true
		}
	}
	if toolNamesRestored {
		logger.Debug("restored sanitized tool names in non-stream response")
	}

	// Check if this is a Claude Messages conversion - if so, don't write response here
	// The DoResponse method will handle the conversion and response writing
	if isClaudeConversion, exists := c.Get(ctxkey.ClaudeMessagesConversion); exists && isClaudeConversion.(bool) {
		// Preserve the original response body so convertToClaudeResponse can consume it later.
		resp.Body = io.NopCloser(bytes.NewReader(responseBody))
		// For Claude Messages conversion, just return the usage information
		// The DoResponse method will handle the response conversion and writing
		calculateTokenUsage(&textResponse, promptTokens, modelName)
		return nil, &textResponse.Usage
	}

	// Calculate token usage BEFORE writing to client so we can still return usage
	// even if client disconnects causes a write error.
	calculateTokenUsage(&textResponse, promptTokens, modelName)

	if modifiedBody, marshalErr := json.Marshal(textResponse); marshalErr != nil {
		logger.Error("failed to marshal modified response body",
			zap.Error(marshalErr))
		resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))
	} else {
		responseBody = modifiedBody
		resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))
	}
	logger.Debug("handler converted response",
		zap.Int("body_bytes", len(responseBody)),
		zap.Bool("body_logging_suppressed", true))

	// Forward all response headers (not just first value of each)
	for k, values := range resp.Header {
		if strings.EqualFold(k, "Content-Length") ||
			strings.EqualFold(k, "Transfer-Encoding") ||
			strings.EqualFold(k, "Content-Encoding") {
			continue
		}
		for _, v := range values {
			c.Writer.Header().Add(k, v)
		}
	}

	// Ensure content length reflects the rewritten body size so clients do not wait for
	// more bytes than we send (e.g. when we drop upstream-only fields).
	newLength := strconv.Itoa(len(responseBody))
	c.Writer.Header().Set("Content-Length", newLength)
	logger.Debug("adjusted response content length",
		zap.String("original_content_length", resp.Header.Get("Content-Length")),
		zap.String("rewritten_content_length", newLength))

	// Set response status and copy body to client
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = io.Copy(c.Writer, resp.Body); err != nil {
		// Return usage even on write failure so billing can proceed for forwarded requests
		return ErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError), &textResponse.Usage
	}

	c.Set(ctxkey.ConvertedResponse, textResponse)

	// Close the reset body
	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	// Usage was already calculated above
	return nil, &textResponse.Usage
}

// EmbeddingHandler processes non-streaming embedding responses from the OpenAI API and derives usage
// information even when upstream omits the usage block.
func EmbeddingHandler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	logger := gmw.GetLogger(c)
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_embedding_response_body_failed", http.StatusInternalServerError), nil
	}

	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_embedding_response_body_failed", http.StatusInternalServerError), nil
	}

	fields := []zap.Field{
		zap.Int("status_code", resp.StatusCode),
		zap.Int("body_bytes", len(responseBody)),
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		fields = append(fields, zap.String("content_type", contentType))
	}
	fields = append(fields, zap.Bool("body_logging_suppressed", true))
	logger.Debug("receive upstream embedding response", fields...)

	if len(responseBody) == 0 {
		logger.Error("received empty embedding response body from upstream",
			zap.Int("status_code", resp.StatusCode),
			zap.String("model", modelName))
		return ErrorWrapper(errors.Errorf("empty embedding response body from upstream"),
			"empty_embedding_response", http.StatusInternalServerError), nil
	}

	var embeddingResponse EmbeddingResponse
	if err = json.Unmarshal(responseBody, &embeddingResponse); err != nil {
		logger.Error("failed to unmarshal embedding response body",
			zap.Error(err),
			zap.Int("body_bytes", len(responseBody)),
			zap.Bool("body_logging_suppressed", true))
		return ErrorWrapper(err, "unmarshal_embedding_response_failed", http.StatusInternalServerError), nil
	}

	if embeddingResponse.Error != nil && embeddingResponse.Error.Type != "" {
		if embeddingResponse.Error.RawError == nil && embeddingResponse.Error.Message != "" {
			embeddingResponse.Error.RawError = errors.Wrap(errUpstreamEmbeddingResponse, embeddingResponse.Error.Message)
		}
		logger.Debug("upstream returned embedding error response",
			zap.String("error_type", string(embeddingResponse.Error.Type)),
			zap.String("error_message", embeddingResponse.Error.Message),
			zap.Error(embeddingResponse.Error.RawError))
		return &model.ErrorWithStatusCode{
			Error:      *embeddingResponse.Error,
			StatusCode: resp.StatusCode,
		}, nil
	}

	if len(embeddingResponse.Data) == 0 {
		logger.Error("embedding response has no data, possible upstream error",
			zap.Int("body_bytes", len(responseBody)),
			zap.Bool("body_logging_suppressed", true))
		return ErrorWrapper(errors.Errorf("no embedding data in upstream response"),
			"missing_embedding_data", http.StatusInternalServerError), nil
	}

	base64Vectors := 0
	base64Dims := 0
	for _, item := range embeddingResponse.Data {
		if item.Base64Encoded {
			base64Vectors++
			if base64Dims == 0 {
				base64Dims = len(item.Embedding)
			}
		}
	}
	if base64Vectors > 0 {
		logger.Debug("decoded base64 embeddings",
			zap.Int("vectors", base64Vectors),
			zap.Int("dimensions", base64Dims))
	}

	usage := embeddingResponse.Usage
	if usage.PromptTokens == 0 && promptTokens > 0 {
		usage.PromptTokens = promptTokens
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	logger.Debug("finalized embedding usage",
		zap.Int("prompt_tokens", usage.PromptTokens),
		zap.Int("completion_tokens", usage.CompletionTokens),
		zap.Int("total_tokens", usage.TotalTokens))

	// Preserve aggregated response for downstream inspection (e.g. tests or converters)
	embeddingResponse.Usage = usage
	c.Set(ctxkey.ConvertedResponse, embeddingResponse)

	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = c.Writer.Write(responseBody); err != nil {
		return ErrorWrapper(err, "write_embedding_response_body_failed", http.StatusInternalServerError), &usage
	}

	return nil, &usage
}

// processReasoningContent is a helper function to extract and process reasoning content from the message
func processReasoningContent(msg *TextResponseChoice) string {
	var reasoningContent string

	// Check different locations for reasoning content
	switch {
	case msg.Reasoning != nil:
		reasoningContent = *msg.Reasoning
		msg.Reasoning = nil
	case msg.ReasoningContent != nil:
		reasoningContent = *msg.ReasoningContent
		msg.ReasoningContent = nil
	case msg.Message.Reasoning != nil:
		reasoningContent = *msg.Message.Reasoning
		msg.Message.Reasoning = nil
	case msg.Message.ReasoningContent != nil:
		reasoningContent = *msg.Message.ReasoningContent
		msg.Message.ReasoningContent = nil
	case msg.Thinking != nil:
		reasoningContent = *msg.Thinking
		msg.Thinking = nil
	case msg.Message.Thinking != nil:
		reasoningContent = *msg.Message.Thinking
		msg.Message.Thinking = nil
	}

	return reasoningContent
}

// Helper function to calculate token usage
func calculateTokenUsage(response *SlimTextResponse, promptTokens int, modelName string) {
	// Calculate tokens if not provided by the API
	if response.Usage.TotalTokens == 0 ||
		(response.Usage.PromptTokens == 0 && response.Usage.CompletionTokens == 0) {

		completionTokens := 0
		for _, choice := range response.Choices {
			// Count content tokens
			completionTokens += CountTokenText(choice.Message.StringContent(), modelName)

			// Count reasoning tokens in all possible locations
			if choice.Message.Reasoning != nil {
				completionTokens += CountToken(*choice.Message.Reasoning)
			}
			if choice.Message.ReasoningContent != nil {
				completionTokens += CountToken(*choice.Message.ReasoningContent)
			}
			if choice.Reasoning != nil {
				completionTokens += CountToken(*choice.Reasoning)
			}
			if choice.ReasoningContent != nil {
				completionTokens += CountToken(*choice.ReasoningContent)
			}
		}

		// Set usage values
		response.Usage = model.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		}
	} else if hasAudioTokens(response) {
		// Handle audio tokens conversion
		calculateAudioTokens(response, modelName)
	}

	// Promote any top-level cached_tokens into the nested
	// prompt_tokens_details.cached_tokens field so downstream billing applies
	// the cache-hit ratio. No-op for OpenAI-shaped responses.
	response.Usage.NormalizeCachedTokens()
	response.Usage.NormalizeCacheWriteTokens()
}

// Helper function to check if response has audio tokens
func hasAudioTokens(response *SlimTextResponse) bool {
	return (response.PromptTokensDetails != nil && response.PromptTokensDetails.AudioTokens > 0) ||
		(response.CompletionTokensDetails != nil && response.CompletionTokensDetails.AudioTokens > 0)
}

// Helper function to calculate audio token usage
func calculateAudioTokens(response *SlimTextResponse, modelName string) {
	// Convert audio tokens for prompt
	audioCfg, found := pricing.ResolveAudioPricing(modelName, nil, &Adaptor{}, time.Time{})
	promptRatio := pricing.DefaultAudioPromptRatio
	completionRatio := pricing.DefaultAudioCompletionRatio
	if found && audioCfg != nil {
		promptRatio = audioCfg.PromptRatio
		completionRatio = audioCfg.CompletionRatio
	}

	if response.PromptTokensDetails != nil {
		response.Usage.PromptTokens = response.PromptTokensDetails.TextTokens +
			int(math.Ceil(float64(response.PromptTokensDetails.AudioTokens)*promptRatio))
	}

	// Convert audio tokens for completion
	if response.CompletionTokensDetails != nil {
		response.Usage.CompletionTokens = response.CompletionTokensDetails.TextTokens +
			int(math.Ceil(float64(response.CompletionTokensDetails.AudioTokens)*promptRatio*completionRatio))
	}

	// Calculate total tokens
	response.Usage.TotalTokens = response.Usage.PromptTokens + response.Usage.CompletionTokens
}
