package openai_compatible

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/render"
	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/relay/adaptor/common/toolnamesafe"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/streaming"
)

// UnifiedStreamProcessing handles the core streaming logic shared between handlers
func UnifiedStreamProcessing(c *gin.Context, resp *http.Response, promptTokens int, modelName string, enableThinking bool) (*model.ErrorWithStatusCode, *model.Usage) {
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
		responseBody, err, closeErr := readAndCloseResponseBody(c, resp.Body)
		if err != nil {
			return ErrorWrapper(err, "read_error_response_failed", http.StatusInternalServerError), nil
		}
		if closeErr != nil {
			logger.Debug("failed to close upstream non-streaming error response body", zap.Error(closeErr))
		}

		logger.Error("received error response in stream handler",
			zap.Int("body_bytes", len(responseBody)),
			zap.Bool("body_logging_suppressed", true))

		// Try to parse as error response
		var errorResponse SlimTextResponse
		if err := json.Unmarshal(responseBody, &errorResponse); err == nil && errorResponse.Error != nil && errorResponse.Error.Type != "" {
			return &model.ErrorWithStatusCode{
				Error:      *errorResponse.Error,
				StatusCode: resp.StatusCode,
			}, nil
		}

		// Return generic error if parsing fails
		return ErrorWrapper(errors.Errorf("unexpected non-streaming response with %d bytes", len(responseBody)),
			"unexpected_response_format", resp.StatusCode), nil
	}

	receiptComplete := false
	upstreamDone := false
	tracker := streaming.FromContext(c)
	if tracker != nil {
		streaming.ClaimProtocolObservation(c)
	}
	cleanup := watchStreamBody(c, resp)
	defer cleanup()
	lineReader := commonsse.NewLineReader(resp.Body, commonsse.DefaultLineBufferSize)

	common.SetEventStreamHeaders(c)

	var streamRewriter StreamRewriteHandler
	if rewriteAny, exists := c.Get(ctxkey.ResponseStreamRewriteHandler); exists {
		if rewriter, ok := rewriteAny.(StreamRewriteHandler); ok {
			streamRewriter = rewriter
		}
	}

	// Initialize unified streaming context
	streamCtx := NewStreamingContext(logger, enableThinking)

	for {
		line, err := lineReader.Next()
		if err != nil {
			// Closing a canceled body may surface as EOF. Preserve the caller's
			// stop condition and observed receipt instead of synthesizing success.
			if canceled := gmw.Ctx(c).Err(); canceled != nil && !upstreamDone && errors.Is(err, io.EOF) {
				err = canceled
			}
			if errors.Is(err, io.EOF) {
				break
			}

			if tracker != nil {
				return ErrorWrapper(err, "read_stream_failed", http.StatusInternalServerError), tracker.UsageSnapshot()
			}
			return ErrorWrapper(err, "read_stream_failed", http.StatusInternalServerError), streamCtx.usage
		}

		if line.Oversized {
			var streamResponse ChatCompletionsStreamResponse
			complete, err := decodeStreamReceipt(line.Large, &streamResponse)
			if err != nil {
				logger.Warn("failed to parse oversized streaming chunk, skipping", zap.Error(err))
				continue
			}

			receiptComplete = nextStreamReceiptCompleteness(receiptComplete, complete, &streamResponse)
			streamResponse.Id = tracing.GenerateChatCompletionID(c)
			if err := ObserveStreamChunk(c, &streamResponse); err != nil {
				usage := tracker.UsageSnapshot()
				failure := ErrorWrapper(err, "streaming_billing_failed", http.StatusForbidden)
				FailStreamWithBridge(c, failure, usage)
				streaming.StopUpstream(c)
				return failure, usage
			}

			modifiedChunk := streamCtx.ProcessStreamChunk(&streamResponse)
			for i := range streamResponse.Choices {
				toolnamesafe.RestoreToolCallNames(c, streamResponse.Choices[i].Delta.ToolCalls)
			}

			if streamRewriter != nil {
				handled, doneRendered := streamRewriter.HandleChunk(c, &streamResponse)
				if handled {
					if doneRendered {
						streamCtx.doneRendered = true
					}
					continue
				}
			}

			if enableThinking {
				reasoningFormat := c.Query("reasoning_format")
				if reasoningFormat == "" {
					reasoningFormat = string(model.ReasoningFormatReasoningContent)
				}

				for i := range streamResponse.Choices {
					if streamResponse.Choices[i].Delta.ReasoningContent != nil {
						rc := *streamResponse.Choices[i].Delta.ReasoningContent
						streamResponse.Choices[i].Delta.SetReasoningContent(reasoningFormat, rc)
						if strings.ToLower(strings.TrimSpace(reasoningFormat)) != string(model.ReasoningFormatReasoningContent) {
							streamResponse.Choices[i].Delta.ReasoningContent = nil
						}
					}
				}
			}

			payload, err := json.Marshal(streamResponse)
			if err != nil {
				logger.Warn("failed to marshal oversized streaming chunk, skipping", zap.Error(err))
				continue
			}

			if modifiedChunk {
				render.StringData(c, "data: "+string(payload))
			} else {
				render.StringData(c, "data: "+string(payload))
			}

			continue
		}

		data := NormalizeDataLine(line.Text())
		// logger.Debug("processing streaming chunk",
		// 	zap.String("chunk_data", data),
		// 	zap.Int("chunks_processed", streamCtx.chunksProcessed))

		if len(data) < DataPrefixLength {
			continue
		}

		if data[:DataPrefixLength] != DataPrefix && data[:DataPrefixLength] != Done {
			continue
		}

		if strings.HasPrefix(data[DataPrefixLength:], Done) {
			upstreamDone = true
			if streamRewriter != nil {
				handled, doneRendered := streamRewriter.HandleUpstreamDone(c)
				if handled {
					if doneRendered {
						streamCtx.doneRendered = true
					}
					continue
				}
			}
			render.StringData(c, data)
			streamCtx.doneRendered = true
			continue
		}

		// Parse the streaming chunk
		var streamResponse ChatCompletionsStreamResponse
		jsonData := data[DataPrefixLength:]
		complete, err := decodeStreamReceipt(strings.NewReader(jsonData), &streamResponse)
		if err != nil {
			logger.Warn("failed to parse streaming chunk, skipping",
				zap.String("chunk_data", jsonData),
				zap.Error(err))
			continue // Skip malformed chunks
		}

		// Replace upstream ID with our trace ID
		receiptComplete = nextStreamReceiptCompleteness(receiptComplete, complete, &streamResponse)
		streamResponse.Id = tracing.GenerateChatCompletionID(c)
		if err := ObserveStreamChunk(c, &streamResponse); err != nil {
			usage := tracker.UsageSnapshot()
			failure := ErrorWrapper(err, "streaming_billing_failed", http.StatusForbidden)
			FailStreamWithBridge(c, failure, usage)
			streaming.StopUpstream(c)
			return failure, usage
		}

		// Process chunk using unified logic
		modifiedChunk := streamCtx.ProcessStreamChunk(&streamResponse)
		for i := range streamResponse.Choices {
			toolnamesafe.RestoreToolCallNames(c, streamResponse.Choices[i].Delta.ToolCalls)
		}

		if streamRewriter != nil {
			handled, doneRendered := streamRewriter.HandleChunk(c, &streamResponse)
			if handled {
				if doneRendered {
					streamCtx.doneRendered = true
				}
				continue
			}
		}

		// Respect reasoning_format mapping when thinking is enabled by moving extracted
		// reasoning content to the requested field and clearing the source to avoid duplication
		if enableThinking {
			reasoningFormat := c.Query("reasoning_format")
			// This fixes an issue where other providers (such as self-hosted GPU) don't have query parameters, so we default to reasoning_content
			// when extracting <think></think> content
			if reasoningFormat == "" {
				reasoningFormat = string(model.ReasoningFormatReasoningContent)
			}

			for i := range streamResponse.Choices {
				if streamResponse.Choices[i].Delta.ReasoningContent != nil {
					rc := *streamResponse.Choices[i].Delta.ReasoningContent
					streamResponse.Choices[i].Delta.SetReasoningContent(reasoningFormat, rc)
					// If the requested format is not reasoning_content, clear ReasoningContent to avoid duplicate fields
					if strings.ToLower(strings.TrimSpace(reasoningFormat)) != string(model.ReasoningFormatReasoningContent) {
						streamResponse.Choices[i].Delta.ReasoningContent = nil
					}
				}
			}
		}

		// Forward the chunk to client (modified or original)
		if modifiedChunk {
			// Re-serialize the modified response
			if modifiedJSON, err := json.Marshal(streamResponse); err == nil {
				render.StringData(c, "data: "+string(modifiedJSON))
			} else {
				// Fallback to original data if serialization fails
				render.StringData(c, data)
			}
		} else {
			render.StringData(c, data)
		}
	}

	// Validate stream completion
	if errResp, ok := streamCtx.ValidateStreamCompletion(modelName, contentType); !ok {
		if tracker != nil {
			return errResp, tracker.UsageSnapshot()
		}
		return errResp, streamCtx.usage
	}

	// Calculate final usage with unified logic before emitting terminal events so
	// that any stream rewriter can include accurate metrics.
	missingUsage := !receiptComplete || streamCtx.usage == nil || (streamCtx.usage.CompletionTokens == 0 && (streamCtx.responseTextBuilder.Len() > 0 || streamCtx.toolArgsTextBuilder.Len() > 0))
	var finalUsage *model.Usage
	if !missingUsage {
		// Explicit input/output counters, including zero, remain measured.
		finalUsage = streamCtx.usage
	} else {
		finalUsage = streamCtx.CalculateUsage(promptTokens, modelName)
	}
	// A complete receipt may predate later output. Use the owned chronology
	// for terminal rendering, matching the usage preserved for settlement.
	if tracker != nil && streamCtx.usage != nil {
		finalUsage = tracker.UsageSnapshot()
	} else if tracker != nil && missingUsage {
		finalUsage.BillingEstimateReason = "stream_usage_missing_counters"
	}
	// Normalize the final owned snapshot, including provider cache fields,
	// before terminal rendering and controller settlement consume it.
	if finalUsage != nil {
		if finalUsage.TotalTokens == 0 {
			finalUsage.TotalTokens = finalUsage.PromptTokens + finalUsage.CompletionTokens
		}
		finalUsage.NormalizeCachedTokens()
		finalUsage.NormalizeCacheWriteTokens()
	}

	if streamRewriter != nil {
		streamRewriter.FinalizeUsage(finalUsage)
		handled, doneRendered := streamRewriter.HandleDone(c)
		if handled {
			if doneRendered {
				streamCtx.doneRendered = true
			}
		} else if !streamCtx.doneRendered {
			render.StringData(c, "data: "+Done)
			streamCtx.doneRendered = true
		}
	} else if !streamCtx.doneRendered {
		render.StringData(c, "data: "+Done)
		streamCtx.doneRendered = true
	}

	if err := resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), finalUsage
	}

	return nil, finalUsage
}
