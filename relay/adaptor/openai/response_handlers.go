package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/common/toolnamesafe"
	"github.com/Laisky/one-api/relay/model"
)

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
	fields = append(fields, zap.Bool("body_logging_suppressed", true))
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

	// Surface the upstream Responses id so the controller can record a
	// stateless-client continuation checkpoint against it (ST-022). Internal only;
	// never written to the client body.
	if responseAPIResp.Id != "" {
		c.Set(ctxkey.ResponseAPIUpstreamID, responseAPIResp.Id)
	}

	// Convert Response API response to ChatCompletion format
	chatCompletionResp := ConvertResponseAPIToChatCompletion(&responseAPIResp)
	chatCompletionResp.Model = modelName

	// Surface the rendered assistant turn so a stateless-client checkpoint can key on
	// the full transcript the client will resend next time (ST-022).
	if len(chatCompletionResp.Choices) > 0 {
		c.Set(ctxkey.ResponseAPIAssistantMessage, chatCompletionResp.Choices[0].Message)
	}

	// Handle reasoning content in the choice
	if len(chatCompletionResp.Choices) > 0 {
		choice := &chatCompletionResp.Choices[0]
		if choice.Message.Reasoning != nil && *choice.Message.Reasoning != "" {
			choice.Message.SetReasoningContent(c.Query("reasoning_format"), *choice.Message.Reasoning)
		}
	}

	// Restore any sanitized tool names so the client receives the original
	// identifiers it submitted (no-op when no sanitization happened).
	toolNamesRestored := false
	for i := range chatCompletionResp.Choices {
		if toolnamesafe.RestoreToolCallNames(c, chatCompletionResp.Choices[i].Message.ToolCalls) {
			toolNamesRestored = true
		}
	}
	if toolNamesRestored {
		lg.Debug("restored sanitized tool names in Response API non-stream response")
	}

	// Set usage - prioritize API-provided usage, but fallback to calculation if needed.
	// An unfinished (queued/in_progress) reply has no authoritative receipt, so it
	// never synthesizes prompt-only usage that would release the reservation.
	var finalUsage *model.Usage

	if IsResponseStatusNonTerminal(responseAPIResp.Status) {
		finalUsage = nonTerminalResponseUsage(c, &responseAPIResp)
	} else if responseAPIResp.Usage != nil {
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

	lg.Debug("generate response to user",
		zap.Int("body_bytes", len(jsonResponse)),
		zap.Bool("body_logging_suppressed", true))

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

// ResponseAPIDirectHandler processes non-streaming responses from Response API format and passes them through directly
// This function is used for direct Response API requests that don't need conversion back to ChatCompletion format
// Returns error (if any) and token usage information
func ResponseAPIDirectHandler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	lg := gmw.GetLogger(c)
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
	fields = append(fields, zap.Bool("body_logging_suppressed", true))
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

	// Extract usage information for billing. An unfinished (queued/in_progress)
	// reply has no authoritative receipt, so it never synthesizes prompt-only
	// usage that would release the reservation before the work completes.
	var finalUsage *model.Usage
	if IsResponseStatusNonTerminal(responseAPIResp.Status) {
		finalUsage = nonTerminalResponseUsage(c, &responseAPIResp)
	} else if responseAPIResp.Usage != nil {
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
	lg.Debug("adjusted response content length", zap.String("original_content_length", resp.Header.Get("Content-Length")), zap.String("rewritten_content_length", newLength))
	if _, err = c.Writer.Write(responseBody); err != nil {
		// Return usage even on write failure so billing can proceed for forwarded requests
		return ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), finalUsage
	}

	c.Set(ctxkey.ConvertedResponse, responseAPIResp)

	return nil, finalUsage
}
