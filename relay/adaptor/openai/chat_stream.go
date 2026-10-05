package openai

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
	"github.com/Laisky/one-api/common/conv"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/render"
	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/Laisky/one-api/relay/adaptor/common/toolnamesafe"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/Laisky/one-api/relay/streaming"
)

// StreamHandler processes streaming responses from OpenAI API
// It handles incremental content delivery and accumulates the final response text
// Returns error (if any), accumulated response text, and token usage information
func StreamHandler(c *gin.Context, resp *http.Response, relayMode int) (*model.ErrorWithStatusCode, string, *model.Usage) {
	lg := gmw.GetLogger(c)
	metaInfo := metalib.GetByContext(c)
	tracker := streaming.FromContext(c)
	if tracker != nil {
		streaming.ClaimProtocolObservation(c)
	}
	var trackerErr error
	// Initialize accumulators for the response
	var responseText strings.Builder
	var reasoningText strings.Builder
	var usage *model.Usage

	var streamRewriter openai_compatible.StreamRewriteHandler
	if rewriteAny, exists := c.Get(ctxkey.ResponseStreamRewriteHandler); exists {
		if rewriter, ok := rewriteAny.(openai_compatible.StreamRewriteHandler); ok {
			streamRewriter = rewriter
		}
	}

	lineReader := commonsse.NewLineReader(resp.Body, commonsse.DefaultLineBufferSize)

	// Set response headers for SSE
	common.SetEventStreamHeaders(c)

	// Wrap the reader with heartbeats to prevent reverse-proxy timeouts (e.g. Cloudflare 524).
	hbr := render.NewHeartbeatLineReader(c, lineReader, render.DefaultHeartbeatInterval)
	defer hbr.Close()

	doneRendered := false
	var streamErr error
	sendStreamingError := func(code, message string) {
		status := http.StatusInternalServerError
		if code == "insufficient_user_quota" {
			status = http.StatusForbidden
		}
		failure := ErrorWrapper(errors.New(message), code, status)
		if tracker != nil && usage != nil {
			usage = tracker.UsageSnapshot()
		}
		if openai_compatible.FailStreamWithBridge(c, failure, usage) {
			doneRendered = true
			return
		}

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
			switch relayMode {
			case relaymode.ChatCompletions:
				var streamResponse openai_compatible.ChatCompletionsStreamResponse
				if err := json.NewDecoder(line.Large).Decode(&streamResponse); err != nil {
					lg.Error("unmarshalling oversized stream data", zap.Error(err))
					continue
				}

				if len(streamResponse.Choices) == 0 && streamResponse.Usage == nil {
					continue
				}

				for i := range streamResponse.Choices {
					if toolnamesafe.RestoreToolCallNames(c, streamResponse.Choices[i].Delta.ToolCalls) {
						lg.Debug("restored sanitized tool names in oversized stream chunk")
					}
				}

				// A receipt decoded in this same frame is already observed work,
				// even if incremental enforcement stops before it is forwarded.
				if streamResponse.Usage != nil {
					usage = streamResponse.Usage
				}
				if tracker != nil {
					if err := openai_compatible.ObserveStreamChunk(c, &streamResponse, CountTokenText); err != nil {
						trackerErr = err
						if errors.Is(err, streaming.ErrQuotaExceeded) {
							sendStreamingError("insufficient_user_quota", "user quota exhausted during streaming")
						} else {
							sendStreamingError("streaming_billing_failed", "failed to track streaming usage")
						}
						break streamLoop
					}
				}
				for _, choice := range streamResponse.Choices {
					currentReasoningChunk := extractReasoningContent(&choice.Delta)
					if currentReasoningChunk != "" {
						reasoningText.WriteString(currentReasoningChunk)
					}

					choice.Delta.SetReasoningContent(c.Query("reasoning_format"), currentReasoningChunk)
					responseText.WriteString(conv.AsString(choice.Delta.Content))

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
					payload, err := json.Marshal(streamResponse)
					if err != nil {
						lg.Error("marshalling oversized stream response", zap.Error(err))
						continue
					}
					render.StringData(c, "data: "+string(payload))
				}

				if streamResponse.Usage != nil {
					usage = streamResponse.Usage
					if tracker != nil {
						tracker.UpdateFinalUsage(streamResponse.Usage)
						if err := tracker.CheckAffordability(); err != nil {
							trackerErr = err
							sendStreamingError("insufficient_user_quota", "user quota exhausted during streaming")
							break streamLoop
						}

					}
				}

				if handledByRewriter {
					continue
				}

			case relaymode.Completions:
				var streamResponse CompletionsStreamResponse
				if err := json.NewDecoder(line.Large).Decode(&streamResponse); err != nil {
					lg.Error("error unmarshalling oversized completion stream response", zap.Error(err))
					continue
				}

				payload, err := json.Marshal(streamResponse)
				if err != nil {
					lg.Error("error marshalling oversized completion stream response", zap.Error(err))
					continue
				}
				render.StringData(c, "data: "+string(payload))

				for _, choice := range streamResponse.Choices {
					responseText.WriteString(choice.Text)
					if tracker != nil && metaInfo != nil {
						if tokens := CountTokenText(choice.Text, metaInfo.ActualModelName); tokens > 0 {
							if err := tracker.RecordCompletionTokens(tokens); err != nil {
								trackerErr = err
								if errors.Is(err, streaming.ErrQuotaExceeded) {
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

			continue
		}

		data := openai_compatible.NormalizeDataLine(line.Text())

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

			// Restore any sanitized tool names back to client-facing originals
			// before forwarding. The normal path emits raw upstream JSON, so we
			// only re-marshal when a rename actually happened.
			toolNamesRestored := false
			for i := range streamResponse.Choices {
				if toolnamesafe.RestoreToolCallNames(c, streamResponse.Choices[i].Delta.ToolCalls) {
					toolNamesRestored = true
				}
			}
			if toolNamesRestored {
				lg.Debug("restored sanitized tool names in stream chunk")
			}

			// Preserve an already decoded same-frame receipt before any delta
			// can trigger enforcement and exit the stream loop.
			if streamResponse.Usage != nil {
				usage = streamResponse.Usage
			}
			// Process each choice in the response
			if tracker != nil {
				if err := openai_compatible.ObserveStreamChunk(c, &streamResponse, CountTokenText); err != nil {
					trackerErr = err
					if errors.Is(err, streaming.ErrQuotaExceeded) {
						sendStreamingError("insufficient_user_quota", "user quota exhausted during streaming")
					} else {
						sendStreamingError("streaming_billing_failed", "failed to track streaming usage")
					}
					break streamLoop
				}
			}
			for _, choice := range streamResponse.Choices {
				// Extract reasoning content from different possible fields
				currentReasoningChunk := extractReasoningContent(&choice.Delta)

				// Update accumulated reasoning text
				if currentReasoningChunk != "" {
					reasoningText.WriteString(currentReasoningChunk)
				}

				// Set the reasoning content in the format requested by client
				choice.Delta.SetReasoningContent(c.Query("reasoning_format"), currentReasoningChunk)

				// Accumulate response content
				responseText.WriteString(conv.AsString(choice.Delta.Content))

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
				if toolNamesRestored {
					payload, err := json.Marshal(streamResponse)
					if err != nil {
						lg.Error("marshalling stream response after tool name restore",
							zap.Error(err))
						render.StringData(c, data)
					} else {
						render.StringData(c, "data: "+string(payload))
					}
				} else {
					// Send the processed data to the client
					render.StringData(c, data)
				}
			}

			// Update usage information if available
			if streamResponse.Usage != nil {
				usage = streamResponse.Usage
				if tracker != nil {
					tracker.UpdateFinalUsage(streamResponse.Usage)
					if err := tracker.CheckAffordability(); err != nil {
						trackerErr = err
						sendStreamingError("insufficient_user_quota", "user quota exhausted during streaming")
						break streamLoop
					}

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
				responseText.WriteString(choice.Text)
				if tracker != nil && metaInfo != nil {
					if tokens := CountTokenText(choice.Text, metaInfo.ActualModelName); tokens > 0 {
						if err := tracker.RecordCompletionTokens(tokens); err != nil {
							trackerErr = err
							if errors.Is(err, streaming.ErrQuotaExceeded) {
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

	// Log heartbeat diagnostics for chat completion stream
	if hbr.HeartbeatsSent() > 0 || hbr.HeartbeatWriteErr() != nil {
		lg.Debug("heartbeat diagnostics",
			zap.Int("heartbeats_sent", hbr.HeartbeatsSent()),
			zap.NamedError("heartbeat_write_err", hbr.HeartbeatWriteErr()),
		)
	}

	// Use the owned receipt after later output before rewriting or final settlement.
	// Nil receipts retain the existing adaptor fallback policy.
	if tracker != nil && usage != nil {
		usage = tracker.UsageSnapshot()
	}

	// Promote any top-level cached_tokens into the nested
	// prompt_tokens_details.cached_tokens field so downstream billing applies
	// the cache-hit ratio. No-op for OpenAI-shaped responses.
	usage.NormalizeCachedTokens()
	usage.NormalizeCacheWriteTokens()

	// Let the streamRewriter finalize if present, but do NOT fabricate a
	// [DONE] when the upstream didn't send one — be an honest proxy.
	var readFailure *model.ErrorWithStatusCode
	if streamErr != nil && trackerErr == nil {
		readFailure = ErrorWrapper(streamErr, "read_stream_failed", http.StatusInternalServerError)
	} else if streamRewriter != nil {
		streamRewriter.FinalizeUsage(usage)
		handled, handledDone := streamRewriter.HandleDone(c)
		if handled {
			if handledDone {
				doneRendered = true
			}
		} else if !doneRendered {
			lg.Warn("upstream chat completion stream ended without sending [DONE]")
		}
	} else if !doneRendered {
		lg.Warn("upstream chat completion stream ended without sending [DONE]")
	}

	combined := reasoningText.String() + responseText.String()
	// Clean up resources
	if err := resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), combined, usage
	}

	if trackerErr != nil {
		if errors.Is(trackerErr, streaming.ErrQuotaExceeded) {
			return ErrorWrapper(trackerErr, "insufficient_user_quota", http.StatusForbidden), combined, usage
		}
		return ErrorWrapper(trackerErr, "streaming_billing_failed", http.StatusInternalServerError), combined, usage
	}

	if readFailure != nil {
		return readFailure, combined, usage
	}

	// Record when upstream streaming is completed
	recordUpstreamCompleted(c)

	if combined != "" || usage != nil {
		c.Set(ctxkey.ConvertedResponse, map[string]any{
			"stream":    true,
			"reasoning": reasoningText.String(),
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
