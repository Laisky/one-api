package openai

import (
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
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
)

// ResponseAPIDirectStreamHandler processes streaming responses from Response API format and passes them through directly
// This function is used for direct Response API streaming requests that don't need conversion back to ChatCompletion format
// Returns error (if any), accumulated response text, and token usage information
func ResponseAPIDirectStreamHandler(c *gin.Context, resp *http.Response, relayMode int) (apiErr *model.ErrorWithStatusCode, responseText string, usage *model.Usage) {
	lg := gmw.GetLogger(c)
	// Initialize accumulators for the response
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
	var lastFullResponse *ResponseAPIResponse
	flushSupported := false
	if _, ok := any(c.Writer).(http.Flusher); ok {
		flushSupported = true
	}

	lineReader := commonsse.NewLineReader(resp.Body, commonsse.DefaultLineBufferSize)

	// Set response headers for SSE
	common.SetEventStreamHeaders(c)

	// Wrap the reader with heartbeats to prevent reverse-proxy timeouts (e.g. Cloudflare 524).
	hbr := render.NewHeartbeatLineReader(c, lineReader, render.DefaultHeartbeatInterval)
	defer hbr.Close()

	lg.Debug("forwarding response api native stream to client",
		zap.Int("relay_mode", relayMode),
		zap.Bool("flush_supported", flushSupported),
	)

	doneRendered := false
	terminalEventSeen := false
	forwardedChunks := 0
	var streamErr error

	// pendingEventType tracks the SSE "event:" line that precedes each "data:" line.
	// The Responses API uses typed SSE events (e.g. "event: response.output_text.delta")
	// and we must forward them faithfully so the client sees the same wire format as the
	// upstream official API.
	pendingEventType := ""

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

		lineText := line.Text()

		// Capture SSE "event:" lines to forward alongside the subsequent "data:" line.
		if strings.HasPrefix(lineText, "event:") {
			pendingEventType = strings.TrimSpace(strings.TrimPrefix(lineText, "event:"))
			continue
		}

		data := openai_compatible.NormalizeDataLine(lineText)

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

		// Parse the Response API streaming chunk
		fullResponse, streamEvent, err := ParseResponseAPIStreamEvent([]byte(data))
		if err != nil {
			// Log the error with more context but continue processing
			lg.Debug("skipping unparseable stream chunk", zap.String("chunk", data), zap.Error(err))
			// Still forward the raw event to the client even if we can't parse it
			// internally — be a faithful proxy.
			render.SSEEvent(c, pendingEventType, data)
			pendingEventType = ""
			forwardedChunks++
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
			if streamEvent.Response != nil {
				lastFullResponse = streamEvent.Response
			}
			switch streamEvent.Type {
			case "response.completed", "response.failed", "response.incomplete":
				terminalEventSeen = true
			}
		} else {
			// Still forward — don't silently drop events the client expects.
			render.SSEEvent(c, pendingEventType, data)
			pendingEventType = ""
			forwardedChunks++
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

		// Full snapshots are cumulative, not extra deltas. Use the larger
		// billable view when a provider only supplies output at completion.
		if snapshot := responseStreamSnapshotText(responseAPIChunk.Output); len(snapshot) > len(responseText) {
			responseText = snapshot
		}

		// Accumulate usage information
		if responseAPIChunk.Usage != nil {
			lastUsage = responseAPIChunk.Usage
			if responseStreamHasTerminalUsage(fullResponse, streamEvent, &responseAPIChunk) {
				lifecycle.terminalReceipt = true
			}
			if convertedUsage := responseAPIChunk.Usage.ToModelUsage(); convertedUsage != nil {
				usage = convertedUsage
			}
		}

		// Pass through the original Response API event directly to client,
		// including the SSE event type to match upstream wire format.
		render.SSEEvent(c, pendingEventType, data)
		pendingEventType = ""
		forwardedChunks++
		if forwardedChunks == 1 {
			lg.Debug("first response api native stream chunk flushed to client")
		}
	}

	// Log heartbeat diagnostics regardless of error state — critical for
	// debugging reverse-proxy timeout (524) issues.
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
	// behaviour as the upstream API: if the upstream connection dropped before
	// sending [DONE], the client should see the connection close without it.
	if !doneRendered && !terminalEventSeen {
		lg.Warn("upstream stream ended without sending [DONE]",
			zap.Int("forwarded_chunks", forwardedChunks),
		)
	}

	// Record when upstream streaming is completed
	lg.Debug("completed response api native stream forwarding",
		zap.Int("forwarded_chunks", forwardedChunks),
		zap.Bool("done_rendered", doneRendered),
		zap.Bool("terminal_event_seen", terminalEventSeen),
		zap.Int("heartbeats_sent", hbr.HeartbeatsSent()),
	)

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
