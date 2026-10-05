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
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/render"
	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/Laisky/one-api/relay/adaptor/common/toolnamesafe"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
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

// ResponseAPIStreamHandler processes streaming responses from Response API format and converts them back to ChatCompletion format
// This function follows the same pattern as StreamHandler but handles Response API streaming responses
// Returns error (if any), accumulated response text, and token usage information
func ResponseAPIStreamHandler(c *gin.Context, resp *http.Response, relayMode int) (apiErr *model.ErrorWithStatusCode, responseText string, usage *model.Usage) {
	lg := gmw.GetLogger(c)
	// Initialize accumulators for the response
	reasoningText := ""
	var lastUsage *ResponseAPIUsage
	webSearchSeen := make(map[string]struct{})
	webSearchCount := 0
	lifecycle := beginResponseStream(c, resp)
	defer func() {
		if derived, fallback := deriveWebSearchInvocationCount(webSearchCount, lastUsage); fallback {
			webSearchCount = derived
		}
		if webSearchCount > 0 {
			c.Set(ctxkey.WebSearchCallCount, webSearchCount)
		}
		lifecycle.finish(c, &apiErr)
		if lifecycle.gap || !lifecycle.terminalReceipt {
			c.Set(responseStreamEstimateKey, "response_stream_incomplete_or_missing_receipt")
		}
	}()
	toolStates := make(map[string]*responseStreamToolCallState)
	flushSupported := false
	if _, ok := any(c.Writer).(http.Flusher); ok {
		flushSupported = true
	}

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

	lineReader := commonsse.NewLineReader(resp.Body, commonsse.DefaultLineBufferSize)

	// Set response headers for SSE
	common.SetEventStreamHeaders(c)

	// Wrap the reader with heartbeats to prevent reverse-proxy timeouts (e.g. Cloudflare 524).
	hbr := render.NewHeartbeatLineReader(c, lineReader, render.DefaultHeartbeatInterval)
	defer hbr.Close()

	lg.Debug("forwarding response api converted stream to client",
		zap.Int("relay_mode", relayMode),
		zap.Bool("flush_supported", flushSupported),
	)

	doneRendered := false
	forwardedChunks := 0
	var streamErr error

	// Process each line from the stream
	for {
		if lifecycle.writer.err != nil {
			streamErr = lifecycle.writer.err
			break
		}
		line, err := hbr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			streamErr = err
			break
		}

		line, err = boundedResponseStreamLine(line)
		if err != nil {
			lifecycle.gap = true
			streamErr = err
			break
		}

		data := openai_compatible.NormalizeDataLine(line.Text())

		lg.Debug("receive stream event", zap.String("event", data))

		if !strings.HasPrefix(data, dataPrefix) {
			continue
		}
		data = data[dataPrefixLength:]

		if data == done {
			if !doneRendered {
				render.Done(c)
				doneRendered = true
			}
			break
		}

		// Parse the Response API streaming chunk using flexible parsing
		fullResponse, streamEvent, err := ParseResponseAPIStreamEvent([]byte(data))
		if err != nil {
			// Log the error with more context but continue processing
			lg.Debug("skipping unparseable stream chunk", zap.String("chunk", data), zap.Error(err))
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

		if eventType != "" && streamEvent != nil {
			if streamEvent.Item != nil && streamEvent.Item.Type == "function_call" {
				if state := getToolState(streamEvent.Item.Id); state != nil {
					if streamEvent.OutputIndex >= 0 {
						state.setIndex(streamEvent.OutputIndex)
					}
					state.setName(toolnamesafe.RestoreToolName(c, streamEvent.Item.Name))
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
			if responseStreamHasTerminalUsage(fullResponse, streamEvent, &responseAPIChunk) {
				lifecycle.terminalReceipt = true
			}
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
					lg.Error("error marshalling stream chunk", zap.Error(err))
					continue
				}

				render.StringData(c, string(jsonStr))
				forwardedChunks++
				if forwardedChunks == 1 {
					lg.Debug("first response api converted stream chunk flushed to client")
				}
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
						lg.Error("error marshalling usage chunk", zap.Error(err))
						continue
					}

					render.StringData(c, string(jsonStr))
					forwardedChunks++
					if forwardedChunks == 1 {
						lg.Debug("first response api converted stream chunk flushed to client")
					}
					lg.Debug("sent usage chunk from response.completed", zap.Int("chunk_bytes", len(jsonStr)))
				}
			}
			// ALL other events (done events, in_progress events, etc.) are discarded to avoid duplicate content leakage
		}
	}

	if hbr.HeartbeatsSent() > 0 || hbr.HeartbeatWriteErr() != nil {
		lg.Debug("heartbeat diagnostics",
			zap.Int("heartbeats_sent", hbr.HeartbeatsSent()),
			zap.NamedError("heartbeat_write_err", hbr.HeartbeatWriteErr()),
		)
	}

	if streamErr != nil {
		lg.Debug("stream read failed",
			zap.Error(streamErr),
			zap.Int("forwarded_chunks", forwardedChunks),
		)
		return ErrorWrapper(streamErr, "read_stream_failed", http.StatusInternalServerError), responseText, usage
	}

	// Do NOT fabricate a [DONE] if the upstream didn't send one.
	// An honest proxy must let the client observe the same stream termination
	// behaviour as the upstream API.
	if !doneRendered {
		lg.Warn("upstream response api stream ended without sending [DONE]",
			zap.Int("forwarded_chunks", forwardedChunks),
		)
	}

	// Record when upstream streaming is completed
	lg.Debug("completed response api converted stream forwarding",
		zap.Int("forwarded_chunks", forwardedChunks),
		zap.Bool("done_rendered", doneRendered),
		zap.Int("heartbeats_sent", hbr.HeartbeatsSent()),
	)

	return nil, responseText, usage
}
