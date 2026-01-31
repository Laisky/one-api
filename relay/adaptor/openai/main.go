package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	stdErrors "errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/conv"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/render"
	"github.com/songquanpeng/one-api/common/tracing"
	relaymodel "github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	"github.com/songquanpeng/one-api/relay/relaymode"
	"github.com/songquanpeng/one-api/relay/streaming"
)

type responseStreamToolCallState struct {
	name     string
	index    int
	hasIndex bool
	args     strings.Builder
}

func (s *responseStreamToolCallState) setName(name string) {
	if name != "" {
		s.name = name
	}
}

func (s *responseStreamToolCallState) setIndex(idx int) {
	s.index = idx
	s.hasIndex = true
}

func (s *responseStreamToolCallState) appendArgs(delta string) {
	if delta != "" {
		s.args.WriteString(delta)
	}
}

func (s *responseStreamToolCallState) replaceArgs(full string) {
	s.args.Reset()
	s.args.WriteString(full)
}

func (s *responseStreamToolCallState) arguments() string {
	return s.args.String()
}

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

func shouldLogDetailedUpstreamBody(c *gin.Context) bool {
	if c == nil {
		return true
	}
	if skipRaw, exists := c.Get(ctxkey.SkipAdaptorResponseBodyLog); exists {
		if flag, ok := skipRaw.(bool); ok {
			return !flag
		}
	}
	return true
}

// StreamHandler processes streaming responses from OpenAI API
// It handles incremental content delivery and accumulates the final response text
// Returns error (if any), accumulated response text, and token usage information
func StreamHandler(c *gin.Context, resp *http.Response, relayMode int) (*model.ErrorWithStatusCode, string, *model.Usage) {
	lg := gmw.GetLogger(c)
	metaInfo := metalib.GetByContext(c)
	tracker := streaming.FromContext(c)
	var trackerErr error
	// Initialize accumulators for the response
	responseText := ""
	reasoningText := ""
	var usage *model.Usage

	var streamRewriter openai_compatible.StreamRewriteHandler
	if rewriteAny, exists := c.Get(ctxkey.ResponseStreamRewriteHandler); exists {
		if rewriter, ok := rewriteAny.(openai_compatible.StreamRewriteHandler); ok {
			streamRewriter = rewriter
		}
	}

	// Set up scanner for reading the stream line by line
	scanner := bufio.NewScanner(resp.Body)
	helper.ConfigureScannerBuffer(scanner)
	scanner.Split(bufio.ScanLines)

	// Set response headers for SSE
	common.SetEventStreamHeaders(c)

	doneRendered := false
	sendStreamingError := func(code, message string) {
		if err := render.ObjectData(c, map[string]any{
			"error": map[string]any{
				"message": message,
				"type":    code,
				"code":    code,
			},
		}); err != nil {
			lg.Warn("failed to render streaming error", zap.Error(err))
		}
		render.Done(c)
		doneRendered = true
	}

	// Process each line from the stream
streamLoop:
	for scanner.Scan() {
		data := openai_compatible.NormalizeDataLine(scanner.Text())

		lg.Debug("stream response", zap.String("event", data))

		// Skip lines that don't match expected format
		if len(data) < dataPrefixLength {
			continue // Ignore blank line or wrong format
		}

		// Verify line starts with expected prefix
		if data[:dataPrefixLength] != dataPrefix && data[:dataPrefixLength] != done {
			continue
		}

		// Check for stream termination
		if strings.HasPrefix(data[dataPrefixLength:], done) {
			if streamRewriter != nil {
				handled, handledDone := streamRewriter.HandleUpstreamDone(c)
				if handled {
					if handledDone {
						doneRendered = true
					}
					continue
				}
			}
			render.StringData(c, data)
			doneRendered = true
			continue
		}

		// Process based on relay mode
		switch relayMode {
		case relaymode.ChatCompletions:
			var streamResponse openai_compatible.ChatCompletionsStreamResponse

			// Parse the JSON response
			err := json.Unmarshal([]byte(data[dataPrefixLength:]), &streamResponse)
			if err != nil {
				lg.Error("unmarshalling stream data",
					zap.String("data", data),
					zap.Error(err))
				render.StringData(c, data) // Pass raw data to client if parsing fails
				continue
			}

			// Skip empty choices (Azure specific behavior)
			if len(streamResponse.Choices) == 0 && streamResponse.Usage == nil {
				continue
			}

			// Process each choice in the response
			for _, choice := range streamResponse.Choices {
				// Extract reasoning content from different possible fields
				currentReasoningChunk := extractReasoningContent(&choice.Delta)

				// Update accumulated reasoning text
				if currentReasoningChunk != "" {
					reasoningText += currentReasoningChunk
				}

				// Set the reasoning content in the format requested by client
				choice.Delta.SetReasoningContent(c.Query("reasoning_format"), currentReasoningChunk)

				// Accumulate response content
				responseText += conv.AsString(choice.Delta.Content)

				if tracker != nil && metaInfo != nil {
					deltaTokens := 0
					if chunk := conv.AsString(choice.Delta.Content); chunk != "" {
						deltaTokens += CountTokenText(chunk, metaInfo.ActualModelName)
					}
					if currentReasoningChunk != "" {
						deltaTokens += CountTokenText(currentReasoningChunk, metaInfo.ActualModelName)
					}
					if deltaTokens > 0 {
						if err := tracker.RecordCompletionTokens(deltaTokens); err != nil {
							trackerErr = err
							if stdErrors.Is(err, streaming.ErrQuotaExceeded) {
								sendStreamingError("insufficient_user_quota", "user quota exhausted during streaming")
							} else {
								sendStreamingError("streaming_billing_failed", "failed to track streaming usage")
							}
							break streamLoop
						}
					}
				}
			}

			handledByRewriter := false
			if streamRewriter != nil {
				if handled, handledDone := streamRewriter.HandleChunk(c, &streamResponse); handled {
					handledByRewriter = true
					if handledDone {
						doneRendered = true
					}
				}
			}

			if !handledByRewriter {
				// Send the processed data to the client
				render.StringData(c, data)
			}

			// Update usage information if available
			if streamResponse.Usage != nil {
				usage = streamResponse.Usage
				if tracker != nil {
					tracker.UpdateFinalUsage(streamResponse.Usage)
				}
			}

			if handledByRewriter {
				continue
			}

		case relaymode.Completions:
			// Send the data immediately for Completions mode
			render.StringData(c, data)

			var streamResponse CompletionsStreamResponse
			err := json.Unmarshal([]byte(data[dataPrefixLength:]), &streamResponse)
			if err != nil {
				lg.Error("error unmarshalling stream response", zap.Error(err))
				continue
			}

			// Accumulate text from all choices
			for _, choice := range streamResponse.Choices {
				responseText += choice.Text
				if tracker != nil && metaInfo != nil {
					if tokens := CountTokenText(choice.Text, metaInfo.ActualModelName); tokens > 0 {
						if err := tracker.RecordCompletionTokens(tokens); err != nil {
							trackerErr = err
							if stdErrors.Is(err, streaming.ErrQuotaExceeded) {
								sendStreamingError("insufficient_user_quota", "user quota exhausted during streaming")
							} else {
								sendStreamingError("streaming_billing_failed", "failed to track streaming usage")
							}
							break streamLoop
						}
					}
				}
			}
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil && trackerErr == nil {
		lg.Error("error reading stream", zap.Error(err), zap.Int("scanner_max_token_size", helper.DefaultScannerMaxTokenSize))
	}

	// Ensure stream termination is sent to client
	if streamRewriter != nil {
		streamRewriter.FinalizeUsage(usage)
		handled, handledDone := streamRewriter.HandleDone(c)
		if handled {
			if handledDone {
				doneRendered = true
			}
		} else if !doneRendered {
			render.Done(c)
			doneRendered = true
		}
	} else if !doneRendered {
		render.Done(c)
		doneRendered = true
	}

	// Clean up resources
	if err := resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), "", nil
	}

	if trackerErr != nil {
		if stdErrors.Is(trackerErr, streaming.ErrQuotaExceeded) {
			return ErrorWrapper(trackerErr, "insufficient_user_quota", http.StatusForbidden), "", usage
		}
		return ErrorWrapper(trackerErr, "streaming_billing_failed", http.StatusInternalServerError), "", usage
	}

	// Record when upstream streaming is completed
	recordUpstreamCompleted(c)

	combined := reasoningText + responseText
	if combined != "" || usage != nil {
		c.Set(ctxkey.ConvertedResponse, map[string]any{
			"stream":    true,
			"reasoning": reasoningText,
			"content":   combined,
			"usage":     usage,
		})
	}

	// Return the complete response text (reasoning + content) and usage
	return nil, combined, usage
}

// Helper function to extract reasoning content from message delta
func extractReasoningContent(delta *model.Message) string {
	content := ""

	// Extract reasoning from different possible fields
	if delta.Reasoning != nil {
		content += *delta.Reasoning
		delta.Reasoning = nil
	}

	if delta.ReasoningContent != nil {
		content += *delta.ReasoningContent
		delta.ReasoningContent = nil
	}

	return content
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
	if shouldLogDetailedUpstreamBody(c) {
		fields = append(fields, zap.ByteString("body", responseBody))
	} else {
		fields = append(fields, zap.Bool("body_logging_suppressed", true))
	}
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
	for i := range textResponse.Choices {
		choice := &textResponse.Choices[i]
		reasoningContent := processReasoningContent(choice)

		// Set reasoning in requested format if content exists
		if reasoningContent != "" {
			choice.SetReasoningContent(reasoningFormat, reasoningContent)
		}
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
	logger.Debug("handler converted response", zap.ByteString("body", responseBody))

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
	if shouldLogDetailedUpstreamBody(c) {
		fields = append(fields, zap.ByteString("body", responseBody))
	} else {
		fields = append(fields, zap.Bool("body_logging_suppressed", true))
	}
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
			zap.ByteString("response_body", responseBody))
		return ErrorWrapper(err, "unmarshal_embedding_response_failed", http.StatusInternalServerError), nil
	}

	if embeddingResponse.Error != nil && embeddingResponse.Error.Type != "" {
		if embeddingResponse.Error.RawError == nil && embeddingResponse.Error.Message != "" {
			embeddingResponse.Error.RawError = stdErrors.New(embeddingResponse.Error.Message)
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
			zap.ByteString("response_body", responseBody))
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
}

// Helper function to check if response has audio tokens
func hasAudioTokens(response *SlimTextResponse) bool {
	return (response.PromptTokensDetails != nil && response.PromptTokensDetails.AudioTokens > 0) ||
		(response.CompletionTokensDetails != nil && response.CompletionTokensDetails.AudioTokens > 0)
}

// Helper function to calculate audio token usage
func calculateAudioTokens(response *SlimTextResponse, modelName string) {
	// Convert audio tokens for prompt
	audioCfg, found := pricing.ResolveAudioPricing(modelName, nil, &Adaptor{})
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

func deriveWebSearchInvocationCount(current int, usage *ResponseAPIUsage) (int, bool) {
	if current > 0 || usage == nil || usage.InputTokensDetails == nil {
		return current, false
	}
	if count := usage.InputTokensDetails.WebSearchInvocationCount(); count > 0 {
		return count, true
	}
	return current, false
}

// ResponseAPIHandler processes non-streaming responses from Response API format and converts them back to ChatCompletion format
// This function follows the same pattern as Handler but converts Response API responses to ChatCompletion format
// Returns error (if any) and token usage information
func ResponseAPIHandler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	// Read the entire response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}

	lg := gmw.GetLogger(c)
	fields := []zap.Field{
		zap.Int("status_code", resp.StatusCode),
		zap.Int("body_bytes", len(responseBody)),
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		fields = append(fields, zap.String("content_type", contentType))
	}
	if shouldLogDetailedUpstreamBody(c) {
		fields = append(fields, zap.ByteString("body", responseBody))
	} else {
		fields = append(fields, zap.Bool("body_logging_suppressed", true))
	}
	lg.Debug("got response from upstream", fields...)

	// Close the original response body
	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	// Parse the Response API response JSON
	var responseAPIResp ResponseAPIResponse
	if err = json.Unmarshal(responseBody, &responseAPIResp); err != nil {
		return ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	// Check for API errors
	if responseAPIResp.Error != nil {
		return &model.ErrorWithStatusCode{
			Error:      *responseAPIResp.Error,
			StatusCode: resp.StatusCode,
		}, nil
	}

	calls := countWebSearchSearchActions(responseAPIResp.Output)
	if derived, usedFallback := deriveWebSearchInvocationCount(calls, responseAPIResp.Usage); usedFallback {
		lg.Debug("web search count derived from usage details", zap.Int("web_search_requests", derived))
		calls = derived
	}
	if calls > 0 {
		c.Set(ctxkey.WebSearchCallCount, calls)
	}

	// Convert Response API response to ChatCompletion format
	chatCompletionResp := ConvertResponseAPIToChatCompletion(&responseAPIResp)
	chatCompletionResp.Model = modelName

	// Handle reasoning content in the choice
	if len(chatCompletionResp.Choices) > 0 {
		choice := &chatCompletionResp.Choices[0]
		if choice.Message.Reasoning != nil && *choice.Message.Reasoning != "" {
			choice.Message.SetReasoningContent(c.Query("reasoning_format"), *choice.Message.Reasoning)
		}
	}

	// Set usage - prioritize API-provided usage, but fallback to calculation if needed
	var finalUsage *model.Usage

	if responseAPIResp.Usage != nil {
		if convertedUsage := responseAPIResp.Usage.ToModelUsage(); convertedUsage != nil {
			// Check if the converted usage has meaningful token counts
			if convertedUsage.PromptTokens > 0 || convertedUsage.CompletionTokens > 0 {
				finalUsage = convertedUsage
			}
		}
	}

	// If we don't have valid usage data, calculate it from the response content
	if finalUsage == nil {
		var responseText string
		if len(chatCompletionResp.Choices) > 0 {
			if content, ok := chatCompletionResp.Choices[0].Message.Content.(string); ok {
				responseText = content
			}
		}
		finalUsage = ResponseText2Usage(responseText, modelName, promptTokens)
	}

	chatCompletionResp.Usage = *finalUsage

	// Convert the ChatCompletion response back to JSON
	jsonResponse, err := json.Marshal(chatCompletionResp)
	if err != nil {
		return ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}

	lg.Debug("generate response to user", zap.ByteString("body", jsonResponse))

	// Forward all response headers
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

	// Set response status and send the converted response to client
	newLength := strconv.Itoa(len(jsonResponse))
	c.Writer.Header().Set("Content-Length", newLength)
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	lg.Debug("adjusted response content length", zap.String("original_content_length", resp.Header.Get("Content-Length")), zap.String("rewritten_content_length", newLength))
	if _, err = c.Writer.Write(jsonResponse); err != nil {
		// Return usage even on write failure so billing can proceed for forwarded requests
		return ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), &chatCompletionResp.Usage
	}

	return nil, &chatCompletionResp.Usage
}

// ResponseAPIStreamHandler processes streaming responses from Response API format and converts them back to ChatCompletion format
// This function follows the same pattern as StreamHandler but handles Response API streaming responses
// Returns error (if any), accumulated response text, and token usage information
func ResponseAPIStreamHandler(c *gin.Context, resp *http.Response, relayMode int) (*model.ErrorWithStatusCode, string, *model.Usage) {
	// Initialize accumulators for the response
	responseText := ""
	reasoningText := ""
	var usage *model.Usage
	var lastUsage *ResponseAPIUsage
	webSearchSeen := make(map[string]struct{})
	webSearchCount := 0
	toolStates := make(map[string]*responseStreamToolCallState)

	// Track output item IDs for which we've already forwarded delta content.
	seenOutputItems := make(map[string]struct{})

	getToolState := func(id string) *responseStreamToolCallState {
		if id == "" {
			return nil
		}
		state, ok := toolStates[id]
		if !ok {
			state = &responseStreamToolCallState{}
			toolStates[id] = state
		}
		return state
	}

	// Set up scanner for reading the stream line by line
	scanner := bufio.NewScanner(resp.Body)
	helper.ConfigureScannerBuffer(scanner)
	scanner.Split(bufio.ScanLines)

	// Set response headers for SSE
	common.SetEventStreamHeaders(c)

	doneRendered := false

	// Process each line from the stream
	for scanner.Scan() {
		data := openai_compatible.NormalizeDataLine(scanner.Text())

		gmw.GetLogger(c).Debug("receive stream event", zap.String("event", data))

		if !strings.HasPrefix(data, dataPrefix) {
			continue
		}
		data = data[dataPrefixLength:]

		if data == done {
			if !doneRendered {
				c.Render(-1, common.CustomEvent{Data: "data: " + done})
				doneRendered = true
			}
			break
		}

		// Parse the Response API streaming chunk using flexible parsing
		fullResponse, streamEvent, err := ParseResponseAPIStreamEvent([]byte(data))
		if err != nil {
			// Log the error with more context but continue processing
			gmw.GetLogger(c).Debug("skipping unparseable stream chunk", zap.String("chunk", data), zap.Error(err))
			continue
		}

		// Handle full response events (like response.completed)
		var responseAPIChunk ResponseAPIResponse
		var outputIndex *int
		if fullResponse != nil {
			responseAPIChunk = *fullResponse
		} else if streamEvent != nil {
			// Convert streaming event to ResponseAPIResponse for processing
			responseAPIChunk = ConvertStreamEventToResponse(streamEvent)
			// Preserve the output_index from the streaming event for proper tool call indexing
			if streamEvent.OutputIndex >= 0 {
				outputIndex = &streamEvent.OutputIndex
			}
		} else {
			// Skip this chunk if we can't parse it
			continue
		}

		if newCalls := countNewWebSearchSearchActions(responseAPIChunk.Output, webSearchSeen); newCalls > 0 {
			webSearchCount += newCalls
		}

		// IMPORTANT: Accumulate response text for token counting - but only from delta events to avoid duplicates
		//
		// The Response API emits both:
		// 1. Delta events (response.output_text.delta) - contain incremental content: "Hi", " there!", " How..."
		// 2. Done events (response.output_text.done, response.content_part.done, etc.) - contain complete content: "Hi there! How..."
		//
		// If we accumulate both types, we get duplicate content in the final response text.
		// Solution: Only accumulate delta events for final response text counting.
		if streamEvent != nil && strings.Contains(streamEvent.Type, "delta") {
			// Only accumulate content from delta events to prevent duplication
			if delta := extractStringFromRaw(streamEvent.Delta, "partial_json", "json", "text", "delta"); delta != "" {
				if strings.Contains(streamEvent.Type, "reasoning_summary_text") {
					// This is reasoning content
					reasoningText += delta
				} else {
					// This is regular content
					responseText += delta
				}
			}
		}

		// Update tool state tracking with metadata from the streaming event
		// Derive a canonical event type string for both streaming events and
		// full response events so the downstream emission logic can handle
		// both uniformly.
		eventType := ""
		if streamEvent != nil {
			eventType = streamEvent.Type
		} else if fullResponse != nil {
			if responseAPIChunk.Status != "" {
				eventType = "response." + responseAPIChunk.Status
			} else {
				eventType = "response.completed"
			}
		}

		if eventType != "" {
			if streamEvent.Item != nil && streamEvent.Item.Type == "function_call" {
				if state := getToolState(streamEvent.Item.Id); state != nil {
					if streamEvent.OutputIndex >= 0 {
						state.setIndex(streamEvent.OutputIndex)
					}
					state.setName(streamEvent.Item.Name)
					if streamEvent.Item.Arguments != "" {
						state.appendArgs(streamEvent.Item.Arguments)
					}
				}
			}
			if strings.HasPrefix(eventType, "response.function_call_arguments.delta") {
				if state := getToolState(streamEvent.ItemId); state != nil {
					if streamEvent.OutputIndex >= 0 {
						state.setIndex(streamEvent.OutputIndex)
					}
					state.appendArgs(extractStringFromRaw(streamEvent.Delta, "partial_json", "text", "arguments", "delta"))
				}
			}
			if strings.HasPrefix(eventType, "response.function_call_arguments.done") {
				if state := getToolState(streamEvent.ItemId); state != nil {
					if streamEvent.OutputIndex >= 0 {
						state.setIndex(streamEvent.OutputIndex)
					}
					if streamEvent.Arguments != "" {
						state.replaceArgs(streamEvent.Arguments)
					}
				}
			}
		}

		// Convert Response API chunk to ChatCompletion streaming format with proper index context
		chatCompletionChunk := ConvertResponseAPIStreamToChatCompletionWithIndex(&responseAPIChunk, outputIndex)

		// If this is a done/complete event and the output item was already emitted
		// from prior delta events, clear content to avoid duplicate text emission.
		if streamEvent != nil {
			eventType := streamEvent.Type
			if !strings.Contains(eventType, "delta") {
				seen := false
				for _, out := range responseAPIChunk.Output {
					if out.Id != "" {
						if _, ok := seenOutputItems[out.Id]; ok {
							seen = true
							break
						}
					}
				}

				if seen {
					// For intermediate done events (content_part.done, output_item.done), drop
					// content to avoid duplicates (we already sent deltas). For the final
					// response.completed event, re-emit a single terminal chunk with the
					// accumulated responseText so clients receive a full-text final chunk
					// but only once.
					if eventType == "response.completed" {
						if len(chatCompletionChunk.Choices) > 0 {
							delta := &chatCompletionChunk.Choices[0].Delta
							// Use accumulated responseText (from prior delta events) as the
							// final content to avoid duplication while preserving the final
							// combined message for clients.
							delta.Content = responseText
							delta.Reasoning = nil
							delta.ToolCalls = nil
						}
					} else {
						if len(chatCompletionChunk.Choices) > 0 {
							delta := &chatCompletionChunk.Choices[0].Delta
							delta.Content = ""
							delta.Reasoning = nil
							delta.ToolCalls = nil
						}
					}
				}
			}
		}

		if len(chatCompletionChunk.Choices) > 0 {
			delta := &chatCompletionChunk.Choices[0].Delta
			candidateIDs := make([]string, 0, 3)
			for _, tc := range delta.ToolCalls {
				candidateIDs = append(candidateIDs, tc.Id)
			}
			if streamEvent != nil {
				if streamEvent.Item != nil && streamEvent.Item.Type == "function_call" && streamEvent.Item.Id != "" {
					candidateIDs = append(candidateIDs, streamEvent.Item.Id)
				}
				if streamEvent.ItemId != "" {
					candidateIDs = append(candidateIDs, streamEvent.ItemId)
				}
			}

			// Ensure tool call deltas include accumulated state
			for idx := range delta.ToolCalls {
				tc := &delta.ToolCalls[idx]
				callID := tc.Id
				if callID == "" && streamEvent != nil {
					if streamEvent.Item != nil && streamEvent.Item.Type == "function_call" && streamEvent.Item.Id != "" {
						callID = streamEvent.Item.Id
						tc.Id = callID
					} else if streamEvent.ItemId != "" {
						callID = streamEvent.ItemId
						tc.Id = callID
					}
				}
				if state := getToolState(callID); state != nil {
					if tc.Function == nil {
						tc.Function = &model.Function{}
					}
					tc.Function.Name = state.name
					tc.Function.Arguments = state.arguments()
					if state.hasIndex {
						idxCopy := state.index
						tc.Index = &idxCopy
					}
				}
			}

			if len(delta.ToolCalls) == 0 && len(candidateIDs) > 0 {
				for _, id := range candidateIDs {
					if state := toolStates[id]; state != nil {
						tool := model.Tool{
							Id:   id,
							Type: "function",
							Function: &model.Function{
								Name:      state.name,
								Arguments: state.arguments(),
							},
						}
						if state.hasIndex {
							idxCopy := state.index
							tool.Index = &idxCopy
						}
						delta.ToolCalls = append(delta.ToolCalls, tool)
						break
					}
				}
			}

			// Mark that we've seen delta content for this item id so later done events
			// referencing the same item won't re-emit the full content.
			if streamEvent != nil && strings.Contains(streamEvent.Type, "delta") {
				itemId := streamEvent.ItemId
				if itemId == "" && streamEvent.Item != nil {
					itemId = streamEvent.Item.Id
				}
				if itemId != "" {
					seenOutputItems[itemId] = struct{}{}
				}
			}
		}

		// Accumulate usage information
		if responseAPIChunk.Usage != nil {
			lastUsage = responseAPIChunk.Usage
		}
		if chatCompletionChunk.Usage != nil {
			usage = chatCompletionChunk.Usage
		}

		if eventType != "" {
			// Prevent duplicate payloads for terminal events by clearing content deltas
			if strings.HasPrefix(eventType, "response.completed") && len(chatCompletionChunk.Choices) > 0 {
				// If this completed event corresponds to a fullResponse (not a
				// streaming event) and we have accumulated deltas, prefer to
				// re-emit the accumulated responseText as the final chunk
				// rather than the upstream-provided content to avoid
				// duplication.
				if fullResponse != nil {
					if len(chatCompletionChunk.Choices) > 0 {
						delta := &chatCompletionChunk.Choices[0].Delta
						delta.Content = responseText
						delta.Reasoning = nil
						delta.ToolCalls = nil
					}
				} else {
					delta := &chatCompletionChunk.Choices[0].Delta
					if content, ok := delta.Content.(string); ok && content != "" {
						delta.Content = ""
					}
					delta.Reasoning = nil
					delta.ToolCalls = nil
				}
			}

			hasMeaningfulDelta := func() bool {
				if len(chatCompletionChunk.Choices) == 0 {
					return false
				}
				delta := chatCompletionChunk.Choices[0].Delta
				if delta.Reasoning != nil && *delta.Reasoning != "" {
					return true
				}
				if len(delta.ToolCalls) > 0 {
					return true
				}
				switch v := delta.Content.(type) {
				case string:
					return v != ""
				case []byte:
					return len(v) > 0
				}
				return false
			}()

			hasToolCalls := len(chatCompletionChunk.Choices) > 0 && len(chatCompletionChunk.Choices[0].Delta.ToolCalls) > 0
			hasFinishReason := len(chatCompletionChunk.Choices) > 0 && chatCompletionChunk.Choices[0].FinishReason != nil
			shouldSendChunk := false

			if strings.Contains(eventType, "delta") {
				shouldSendChunk = hasMeaningfulDelta
			} else if hasToolCalls {
				shouldSendChunk = true
			} else if eventType == "response.completed" && hasFinishReason {
				shouldSendChunk = true
			} else if hasMeaningfulDelta &&
				!strings.Contains(eventType, "output_text.done") &&
				!strings.Contains(eventType, "content_part.done") &&
				!strings.Contains(eventType, "output_item.done") &&
				!strings.Contains(eventType, "reasoning_summary_text.done") {
				shouldSendChunk = true
			}

			if shouldSendChunk {
				jsonStr, err := json.Marshal(chatCompletionChunk)
				if err != nil {
					lg := gmw.GetLogger(c)
					lg.Error("error marshalling stream chunk", zap.Error(err))
					continue
				}

				c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonStr)})
			} else if eventType == "response.completed" && responseAPIChunk.Usage != nil {
				// Special handling for response.completed when no terminal chunk was
				// emitted above. Emit a single terminal chunk that includes the
				// accumulated content (from deltas) and the usage payload so
				// clients receive a final message plus billing info without
				// duplication.
				convertedUsage := responseAPIChunk.Usage.ToModelUsage()
				if convertedUsage != nil {
					finalContent := ""
					var finalFinish *string
					if len(chatCompletionChunk.Choices) > 0 {
						// Prefer finish reason from the generated chunk if present
						if chatCompletionChunk.Choices[0].FinishReason != nil {
							fr := *chatCompletionChunk.Choices[0].FinishReason
							finalFinish = &fr
						}
						if content, ok := chatCompletionChunk.Choices[0].Delta.Content.(string); ok && content != "" {
							finalContent = content
						}
					}
					// If upstream did not include final content (we suppressed it),
					// fall back to the accumulated responseText built from deltas.
					if finalContent == "" {
						finalContent = responseText
					}
					// Ensure there's a finish reason
					if finalFinish == nil {
						fr := "stop"
						finalFinish = &fr
					}

					usageChunk := ChatCompletionsStreamResponse{
						Id:      responseAPIChunk.Id,
						Object:  "chat.completion.chunk",
						Created: responseAPIChunk.CreatedAt,
						Model:   responseAPIChunk.Model,
						Choices: []ChatCompletionsStreamResponseChoice{
							{
								Index: 0,
								Delta: model.Message{
									Role:    "assistant",
									Content: finalContent,
								},
								FinishReason: finalFinish,
							},
						},
						Usage: convertedUsage,
					}

					jsonStr, err := json.Marshal(usageChunk)
					if err != nil {
						lg := gmw.GetLogger(c)
						lg.Error("error marshalling usage chunk", zap.Error(err))
						continue
					}

					c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonStr)})
					gmw.GetLogger(c).Debug("sent usage chunk from response.completed", zap.ByteString("chunk", jsonStr))
				}
			}
			// ALL other events (done events, in_progress events, etc.) are discarded to avoid duplicate content leakage
		}
	}

	if err := scanner.Err(); err != nil {
		// Let ErrorWrapper handle the logging to avoid duplicate logging
		return ErrorWrapper(err, "read_stream_failed", http.StatusInternalServerError), responseText, usage
	}

	if !doneRendered {
		c.Render(-1, common.CustomEvent{Data: "data: " + done})
	}

	if err := resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), responseText, usage
	}

	if derived, usedFallback := deriveWebSearchInvocationCount(webSearchCount, lastUsage); usedFallback {
		gmw.GetLogger(c).Debug("web search count derived from usage details (stream)", zap.Int("web_search_requests", derived))
		webSearchCount = derived
	}
	if webSearchCount > 0 {
		c.Set(ctxkey.WebSearchCallCount, webSearchCount)
	}

	// Record when upstream streaming is completed
	recordUpstreamCompleted(c)

	return nil, responseText, usage
}

// ResponseAPIDirectHandler processes non-streaming responses from Response API format and passes them through directly
// This function is used for direct Response API requests that don't need conversion back to ChatCompletion format
// Returns error (if any) and token usage information
func ResponseAPIDirectHandler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	// Read the entire response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}

	fields := []zap.Field{
		zap.Int("status_code", resp.StatusCode),
		zap.Int("body_bytes", len(responseBody)),
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		fields = append(fields, zap.String("content_type", contentType))
	}
	if shouldLogDetailedUpstreamBody(c) {
		fields = append(fields, zap.ByteString("body", responseBody))
	} else {
		fields = append(fields, zap.Bool("body_logging_suppressed", true))
	}
	gmw.GetLogger(c).Debug("got response from upstream", fields...)

	// Close the original response body
	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	// Parse the Response API response JSON
	var responseAPIResp ResponseAPIResponse
	if err = json.Unmarshal(responseBody, &responseAPIResp); err != nil {
		return ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	// Check for API errors
	if responseAPIResp.Error != nil {
		return &model.ErrorWithStatusCode{
			Error:      *responseAPIResp.Error,
			StatusCode: resp.StatusCode,
		}, nil
	}

	calls := countWebSearchSearchActions(responseAPIResp.Output)
	if derived, usedFallback := deriveWebSearchInvocationCount(calls, responseAPIResp.Usage); usedFallback {
		gmw.GetLogger(c).Debug("web search count derived from usage details", zap.Int("web_search_requests", derived))
		calls = derived
	}
	if calls > 0 {
		c.Set(ctxkey.WebSearchCallCount, calls)
	}

	// Extract usage information for billing
	var finalUsage *model.Usage
	if responseAPIResp.Usage != nil {
		if convertedUsage := responseAPIResp.Usage.ToModelUsage(); convertedUsage != nil {
			// Check if the converted usage has meaningful token counts
			if convertedUsage.PromptTokens > 0 || convertedUsage.CompletionTokens > 0 {
				finalUsage = convertedUsage
			}
		}
	}

	// If we don't have valid usage data, calculate it from the response content
	if finalUsage == nil {
		var responseText string
		for _, output := range responseAPIResp.Output {
			if output.Type == "message" {
				for _, content := range output.Content {
					if content.Type == "output_text" {
						responseText += content.Text
					}
				}
			}
		}
		finalUsage = ResponseText2Usage(responseText, modelName, promptTokens)
	}

	// Forward all response headers
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

	// Set response status and send the response directly to client
	newLength := strconv.Itoa(len(responseBody))
	c.Writer.Header().Set("Content-Length", newLength)
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	gmw.GetLogger(c).Debug("adjusted response content length", zap.String("original_content_length", resp.Header.Get("Content-Length")), zap.String("rewritten_content_length", newLength))
	if _, err = c.Writer.Write(responseBody); err != nil {
		// Return usage even on write failure so billing can proceed for forwarded requests
		return ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), finalUsage
	}

	c.Set(ctxkey.ConvertedResponse, responseAPIResp)

	return nil, finalUsage
}

// ResponseAPIDirectStreamHandler processes streaming responses from Response API format and passes them through directly
// This function is used for direct Response API streaming requests that don't need conversion back to ChatCompletion format
// Returns error (if any), accumulated response text, and token usage information
func ResponseAPIDirectStreamHandler(c *gin.Context, resp *http.Response, relayMode int) (*model.ErrorWithStatusCode, string, *model.Usage) {
	// Initialize accumulators for the response
	responseText := ""
	var usage *model.Usage
	var lastUsage *ResponseAPIUsage
	webSearchSeen := make(map[string]struct{})
	webSearchCount := 0
	var lastFullResponse *ResponseAPIResponse

	// Set up scanner for reading the stream line by line
	scanner := bufio.NewScanner(resp.Body)
	helper.ConfigureScannerBuffer(scanner)
	scanner.Split(bufio.ScanLines)

	// Set response headers for SSE
	common.SetEventStreamHeaders(c)

	doneRendered := false

	// Process each line from the stream
	for scanner.Scan() {
		data := openai_compatible.NormalizeDataLine(scanner.Text())

		gmw.GetLogger(c).Debug("receive stream event", zap.String("event", data))

		if !strings.HasPrefix(data, dataPrefix) {
			continue
		}
		data = data[dataPrefixLength:]

		if data == done {
			if !doneRendered {
				c.Render(-1, common.CustomEvent{Data: "data: " + done})
				doneRendered = true
			}
			break
		}

		// Parse the Response API streaming chunk
		fullResponse, streamEvent, err := ParseResponseAPIStreamEvent([]byte(data))
		if err != nil {
			// Log the error with more context but continue processing
			gmw.GetLogger(c).Debug("skipping unparseable stream chunk", zap.String("chunk", data), zap.Error(err))
			continue
		}

		// Handle full response events (like response.completed)
		var responseAPIChunk ResponseAPIResponse
		if fullResponse != nil {
			responseAPIChunk = *fullResponse
			lastFullResponse = fullResponse
		} else if streamEvent != nil {
			// Convert streaming event to ResponseAPIResponse for processing
			responseAPIChunk = ConvertStreamEventToResponse(streamEvent)
		} else {
			// Skip this chunk if we can't parse it
			continue
		}

		if newCalls := countNewWebSearchSearchActions(responseAPIChunk.Output, webSearchSeen); newCalls > 0 {
			webSearchCount += newCalls
		}

		// Accumulate response text for token counting - only from delta events to avoid duplicates
		if streamEvent != nil && strings.Contains(streamEvent.Type, "delta") {
			// Only accumulate content from delta events to prevent duplication
			if delta := extractStringFromRaw(streamEvent.Delta, "partial_json", "json", "text", "delta"); delta != "" {
				responseText += delta
			}
		}

		// Accumulate usage information
		if responseAPIChunk.Usage != nil {
			lastUsage = responseAPIChunk.Usage
			if convertedUsage := responseAPIChunk.Usage.ToModelUsage(); convertedUsage != nil {
				usage = convertedUsage
			}
		}

		// Pass through the original Response API event directly to client
		c.Render(-1, common.CustomEvent{Data: "data: " + string(data)})
	}

	if err := scanner.Err(); err != nil {
		// Let ErrorWrapper handle the logging to avoid duplicate logging
		return ErrorWrapper(err, "read_stream_failed", http.StatusInternalServerError), responseText, usage
	}

	if !doneRendered {
		c.Render(-1, common.CustomEvent{Data: "data: " + done})
	}

	if err := resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), responseText, usage
	}

	if derived, usedFallback := deriveWebSearchInvocationCount(webSearchCount, lastUsage); usedFallback {
		gmw.GetLogger(c).Debug("web search count derived from usage details (direct stream)", zap.Int("web_search_requests", derived))
		webSearchCount = derived
	}
	if webSearchCount > 0 {
		c.Set(ctxkey.WebSearchCallCount, webSearchCount)
	}

	// Record when upstream streaming is completed
	recordUpstreamCompleted(c)

	if lastFullResponse != nil {
		c.Set(ctxkey.ConvertedResponse, *lastFullResponse)
	} else if responseText != "" || usage != nil {
		c.Set(ctxkey.ConvertedResponse, map[string]any{
			"stream":  true,
			"content": responseText,
			"usage":   usage,
		})
	}

	return nil, responseText, usage
}
