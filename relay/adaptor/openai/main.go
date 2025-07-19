package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/conv"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/render"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// Use shared constants from openai_compatible package
const (
	dataPrefix       = openai_compatible.DataPrefix
	done             = openai_compatible.Done
	dataPrefixLength = openai_compatible.DataPrefixLength
)

// StreamHandler processes streaming responses from OpenAI API
// It handles incremental content delivery and accumulates the final response text
// Returns error (if any), accumulated response text, and token usage information
func StreamHandler(c *gin.Context, resp *http.Response, relayMode int) (*model.ErrorWithStatusCode, string, *model.Usage) {
	// Initialize accumulators for the response
	responseText := ""
	reasoningText := ""
	var usage *model.Usage

	// Set up scanner for reading the stream line by line
	scanner := bufio.NewScanner(resp.Body)
	buffer := make([]byte, 1024*1024) // 1MB buffer for large messages
	scanner.Buffer(buffer, len(buffer))
	scanner.Split(bufio.ScanLines)

	// Set response headers for SSE
	common.SetEventStreamHeaders(c)

	doneRendered := false

	// Process each line from the stream
	for scanner.Scan() {
		data := openai_compatible.NormalizeDataLine(scanner.Text())

		// logger.Debugf(c.Request.Context(), "stream response: %s", data)

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
			render.StringData(c, data)
			doneRendered = true
			continue
		}

		// Process based on relay mode
		switch relayMode {
		case relaymode.ChatCompletions:
			var streamResponse ChatCompletionsStreamResponse

			// Parse the JSON response
			err := json.Unmarshal([]byte(data[dataPrefixLength:]), &streamResponse)
			if err != nil {
				logger.Errorf(c.Request.Context(), "unmarshalling stream data %q got %+v", data, err)
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
			}

			// Send the processed data to the client
			render.StringData(c, data)

			// Update usage information if available
			if streamResponse.Usage != nil {
				usage = streamResponse.Usage
			}

		case relaymode.Completions:
			// Send the data immediately for Completions mode
			render.StringData(c, data)

			var streamResponse CompletionsStreamResponse
			err := json.Unmarshal([]byte(data[dataPrefixLength:]), &streamResponse)
			if err != nil {
				logger.SysError("error unmarshalling stream response: " + err.Error())
				continue
			}

			// Accumulate text from all choices
			for _, choice := range streamResponse.Choices {
				responseText += choice.Text
			}
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		logger.SysError("error reading stream: " + err.Error())
	}

	// Ensure stream termination is sent to client
	if !doneRendered {
		render.Done(c)
	}

	// Clean up resources
	if err := resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), "", nil
	}

	// Return the complete response text (reasoning + content) and usage
	return nil, reasoningText + responseText, usage
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
	// Read the entire response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}

	// Close the original response body
	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	// Parse the response JSON
	var textResponse SlimTextResponse
	if err = json.Unmarshal(responseBody, &textResponse); err != nil {
		return ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	// Check for API errors
	if textResponse.Error.Type != "" {
		return &model.ErrorWithStatusCode{
			Error:      textResponse.Error,
			StatusCode: resp.StatusCode,
		}, nil
	}

	// Process reasoning content in each choice
	for _, msg := range textResponse.Choices {
		reasoningContent := processReasoningContent(&msg)

		// Set reasoning in requested format if content exists
		if reasoningContent != "" {
			msg.SetReasoningContent(c.Query("reasoning_format"), reasoningContent)
		}
	}

	// Reset response body for forwarding to client
	resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))
	logger.Debugf(c.Request.Context(), "handler response: %s", string(responseBody))

	// Check if this is a Claude Messages conversion - if so, don't write response here
	// The DoResponse method will handle the conversion and response writing
	if isClaudeConversion, exists := c.Get(ctxkey.ClaudeMessagesConversion); exists && isClaudeConversion.(bool) {
		// For Claude Messages conversion, just return the usage information
		// The DoResponse method will handle the response conversion and writing
		calculateTokenUsage(&textResponse, promptTokens, modelName)
		return nil, &textResponse.Usage
	}

	// Forward all response headers (not just first value of each)
	for k, values := range resp.Header {
		for _, v := range values {
			c.Writer.Header().Add(k, v)
		}
	}

	// Set response status and copy body to client
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = io.Copy(c.Writer, resp.Body); err != nil {
		return ErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError), nil
	}

	// Close the reset body
	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	// Calculate token usage if not provided by API
	calculateTokenUsage(&textResponse, promptTokens, modelName)

	return nil, &textResponse.Usage
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
	if response.PromptTokensDetails != nil {
		response.Usage.PromptTokens = response.PromptTokensDetails.TextTokens +
			int(math.Ceil(
				float64(response.PromptTokensDetails.AudioTokens)*
					ratio.GetAudioPromptRatio(modelName),
			))
	}

	// Convert audio tokens for completion
	if response.CompletionTokensDetails != nil {
		response.Usage.CompletionTokens = response.CompletionTokensDetails.TextTokens +
			int(math.Ceil(
				float64(response.CompletionTokensDetails.AudioTokens)*
					ratio.GetAudioPromptRatio(modelName)*ratio.GetAudioCompletionRatio(modelName),
			))
	}

	// Calculate total tokens
	response.Usage.TotalTokens = response.Usage.PromptTokens + response.Usage.CompletionTokens
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

	// Log the response body for debugging
	logger.Debugf(c.Request.Context(),
		"got response from upstream: %s", string(responseBody))

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

	logger.Debugf(c.Request.Context(), "generate response to user: %s", string(jsonResponse))

	// Forward all response headers
	for k, values := range resp.Header {
		for _, v := range values {
			c.Writer.Header().Add(k, v)
		}
	}

	// Set response status and send the converted response to client
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = c.Writer.Write(jsonResponse); err != nil {
		return ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), nil
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

	// Set up scanner for reading the stream line by line
	scanner := bufio.NewScanner(resp.Body)
	buffer := make([]byte, 1024*1024) // 1MB buffer for large messages
	scanner.Buffer(buffer, len(buffer))
	scanner.Split(bufio.ScanLines)

	// Set response headers for SSE
	common.SetEventStreamHeaders(c)

	doneRendered := false

	// Process each line from the stream
	for scanner.Scan() {
		data := openai_compatible.NormalizeDataLine(scanner.Text())

		logger.Debugf(c.Request.Context(), "receive stream event: %s", data)

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
			logger.Debugf(c.Request.Context(), "skipping unparseable stream chunk: %s, error: %s", data, err.Error())
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
			if streamEvent.Delta != "" {
				if strings.Contains(streamEvent.Type, "reasoning_summary_text") {
					// This is reasoning content
					reasoningText += streamEvent.Delta
				} else {
					// This is regular content
					responseText += streamEvent.Delta
				}
			}
		}

		// Convert Response API chunk to ChatCompletion streaming format with proper index context
		chatCompletionChunk := ConvertResponseAPIStreamToChatCompletionWithIndex(&responseAPIChunk, outputIndex)

		// Accumulate usage information
		if chatCompletionChunk.Usage != nil {
			usage = chatCompletionChunk.Usage
		}

		// IMPORTANT: Only send ChatCompletion chunks to client for delta events ONLY
		// Completely discard ALL other events including completion events to prevent client-side duplication
		if streamEvent != nil {
			eventType := streamEvent.Type

			// Only send chunks for delta events (incremental content)
			if strings.Contains(eventType, "delta") {
				// Send the converted chunk to the client
				jsonStr, err := json.Marshal(chatCompletionChunk)
				if err != nil {
					logger.SysError("error marshalling stream chunk: " + err.Error())
					continue
				}

				c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonStr)})
			} else if eventType == "response.completed" && responseAPIChunk.Usage != nil {
				// Special handling for response.completed event to send usage information
				// Convert ResponseAPI usage to model.Usage format
				convertedUsage := responseAPIChunk.Usage.ToModelUsage()
				if convertedUsage != nil {
					// Create a usage-only streaming chunk with empty delta
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
									Content: "",
								},
								FinishReason: nil, // Don't set finish reason in usage chunk
							},
						},
						Usage: convertedUsage,
					}

					jsonStr, err := json.Marshal(usageChunk)
					if err != nil {
						logger.SysError("error marshalling usage chunk: " + err.Error())
						continue
					}

					c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonStr)})
					logger.Debugf(c.Request.Context(), "sent usage chunk from response.completed: %s", string(jsonStr))
				}
			}
			// ALL other events (done events, in_progress events, etc.) are completely discarded
			// This prevents ANY duplicate content from reaching the client
		}
	}

	if err := scanner.Err(); err != nil {
		logger.SysError("error reading stream: " + err.Error())
		return ErrorWrapper(err, "read_stream_failed", http.StatusInternalServerError), responseText, usage
	}

	if !doneRendered {
		c.Render(-1, common.CustomEvent{Data: "data: " + done})
	}

	if err := resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), responseText, usage
	}

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

	// Log the response body for debugging
	logger.Debugf(c.Request.Context(),
		"got response from upstream: %s", string(responseBody))

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
		for _, v := range values {
			c.Writer.Header().Add(k, v)
		}
	}

	// Set response status and send the response directly to client
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = c.Writer.Write(responseBody); err != nil {
		return ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), nil
	}

	return nil, finalUsage
}

// ResponseAPIDirectStreamHandler processes streaming responses from Response API format and passes them through directly
// This function is used for direct Response API streaming requests that don't need conversion back to ChatCompletion format
// Returns error (if any), accumulated response text, and token usage information
func ResponseAPIDirectStreamHandler(c *gin.Context, resp *http.Response, relayMode int) (*model.ErrorWithStatusCode, string, *model.Usage) {
	// Initialize accumulators for the response
	responseText := ""
	var usage *model.Usage

	// Set up scanner for reading the stream line by line
	scanner := bufio.NewScanner(resp.Body)
	buffer := make([]byte, 1024*1024) // 1MB buffer for large messages
	scanner.Buffer(buffer, len(buffer))
	scanner.Split(bufio.ScanLines)

	// Set response headers for SSE
	common.SetEventStreamHeaders(c)

	doneRendered := false

	// Process each line from the stream
	for scanner.Scan() {
		data := openai_compatible.NormalizeDataLine(scanner.Text())

		logger.Debugf(c.Request.Context(), "receive stream event: %s", data)

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
			logger.Debugf(c.Request.Context(), "skipping unparseable stream chunk: %s, error: %s", data, err.Error())
			continue
		}

		// Handle full response events (like response.completed)
		var responseAPIChunk ResponseAPIResponse
		if fullResponse != nil {
			responseAPIChunk = *fullResponse
		} else if streamEvent != nil {
			// Convert streaming event to ResponseAPIResponse for processing
			responseAPIChunk = ConvertStreamEventToResponse(streamEvent)
		} else {
			// Skip this chunk if we can't parse it
			continue
		}

		// Accumulate response text for token counting - only from delta events to avoid duplicates
		if streamEvent != nil && strings.Contains(streamEvent.Type, "delta") {
			// Only accumulate content from delta events to prevent duplication
			if streamEvent.Delta != "" {
				responseText += streamEvent.Delta
			}
		}

		// Accumulate usage information
		if responseAPIChunk.Usage != nil {
			if convertedUsage := responseAPIChunk.Usage.ToModelUsage(); convertedUsage != nil {
				usage = convertedUsage
			}
		}

		// Pass through the original Response API event directly to client
		c.Render(-1, common.CustomEvent{Data: "data: " + string(data)})
	}

	if err := scanner.Err(); err != nil {
		logger.SysError("error reading stream: " + err.Error())
		return ErrorWrapper(err, "read_stream_failed", http.StatusInternalServerError), responseText, usage
	}

	if !doneRendered {
		c.Render(-1, common.CustomEvent{Data: "data: " + done})
	}

	if err := resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), responseText, usage
	}

	return nil, responseText, usage
}
