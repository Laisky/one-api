package openai_compatible

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// ClaudeChatHandler renders one upstream answer as OpenAI Chat Completions
// output on c.Writer, the way legacy adaptors whose providers speak a
// proprietary wire format do: a chat.completion JSON body when not streaming,
// or chat.completion.chunk events through RenderStreamChunkWithBridge and
// FinalizeStreamWithBridge when streaming. It returns the billing usage and an
// error, exactly as the adaptor's DoResponse would.
type ClaudeChatHandler func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode)

// ClaudeChatBridgeActive reports whether the current attempt is a Claude
// Messages request converted to Chat Completions, so chat output produced by an
// adaptor must be rendered as Claude Messages. Parameters: c is the request
// context and m the request meta. Returns: true only for a Claude Messages
// client request that a converting ConvertClaudeRequest marked with
// ctxkey.ClaudeMessagesConversion; internal Chat rounds (MCP loops) clear the
// flag and switch the mode, so they are never bridged.
func ClaudeChatBridgeActive(c *gin.Context, m *meta.Meta) bool {
	return c != nil && m != nil && m.Mode == relaymode.ClaudeMessages && c.GetBool(ctxkey.ClaudeMessagesConversion)
}

// ConvertClaudeRequestThroughChat converts a Claude Messages request with the
// shared OpenAI-compatible conversion, which marks the attempt as a Claude
// conversion, and then into the provider request with convert, typically the
// adaptor's own ConvertRequest for relaymode.ChatCompletions. Parameters: c is
// the request context, request the Claude request, and convert the provider
// conversion. Returns: the provider-native request or a wrapped error.
func ConvertClaudeRequestThroughChat(c *gin.Context, request *relaymodel.ClaudeRequest, convert func(*relaymodel.GeneralOpenAIRequest) (any, error)) (any, error) {
	converted, err := ConvertClaudeRequest(c, request)
	if err != nil {
		return nil, errors.Wrap(err, "convert Claude Messages request to chat")
	}
	chatRequest, ok := converted.(*relaymodel.GeneralOpenAIRequest)
	if !ok || chatRequest == nil {
		return nil, errors.Errorf("unexpected chat conversion result %T", converted)
	}
	native, err := convert(chatRequest)
	if err != nil {
		return nil, errors.Wrap(err, "convert chat request to provider request")
	}
	return native, nil
}

// RenderChatHandlerAsClaude runs handler unchanged unless the attempt is a
// Claude Messages conversion (see ClaudeChatBridgeActive). When it is, the
// handler's chat output is rendered as Claude Messages: a non-stream chat body
// is captured and converted into one Claude message, and a stream is translated
// chunk by chunk through a Claude stream bridge, so SSE stays incremental. The
// handler's usage is returned unchanged, keeping billing identical to the Chat
// Completions path. Parameters: c is the request context, m the request meta,
// and handler the adaptor's chat-rendering response logic. Returns: the usage
// and error to hand back to the relay controller.
func RenderChatHandlerAsClaude(c *gin.Context, m *meta.Meta, handler ClaudeChatHandler) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
	if !ClaudeChatBridgeActive(c, m) {
		return handler()
	}
	if m.IsStream {
		return renderChatStreamAsClaude(c, m, handler)
	}
	return renderChatJSONAsClaude(c, m, handler)
}

// renderChatJSONAsClaude captures a non-stream chat.completion body written by
// handler and writes the equivalent Claude message to the client. Parameters:
// c is the request context, m the request meta, and handler the chat renderer.
// Returns: the handler's usage and error, or a write error with that usage.
func renderChatJSONAsClaude(c *gin.Context, m *meta.Meta, handler ClaudeChatHandler) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
	lg := gmw.GetLogger(c)
	original := c.Writer
	capture := &claudeChatCaptureWriter{ResponseWriter: original, header: make(http.Header)}
	c.Writer = capture
	// Restore the client writer even if the handler panics, so panic recovery
	// renders its error to the client instead of into the capture buffer.
	defer func() { c.Writer = original }()
	usage, respErr := handler()
	c.Writer = original
	if respErr != nil {
		// Captured bytes never reach the client; the controller renders the error.
		lg.Debug("legacy chat handler failed under Claude conversion",
			zap.Int("captured_bytes", capture.body.Len()),
			zap.Int("status_code", respErr.StatusCode))
		return usage, respErr
	}

	status := capture.statusCode()
	var chat chatTextResponse
	if err := json.Unmarshal(capture.body.Bytes(), &chat); err != nil || len(chat.Choices) == 0 {
		// Not a chat completion: forward it untouched rather than fabricate a message.
		lg.Warn("legacy chat handler output is not a chat completion; forwarding it unconverted to the Claude client",
			zap.Int("captured_bytes", capture.body.Len()),
			zap.Int("status_code", status))
		for key, values := range capture.header {
			switch http.CanonicalHeaderKey(key) {
			case "Content-Length", "Transfer-Encoding", "Content-Encoding":
				continue
			}
			original.Header()[key] = append([]string(nil), values...)
		}
		original.WriteHeader(status)
		if _, err := original.Write(capture.body.Bytes()); err != nil {
			return usage, ErrorWrapper(errors.Wrap(err, "write unconverted chat body to Claude client"), "write_response_body_failed", http.StatusInternalServerError)
		}
		return usage, nil
	}

	message := chatResponseToClaude(c, &chat)
	if message.Model == "" {
		message.Model = m.ActualModelName
	}
	if message.Content == nil {
		message.Content = []relaymodel.ClaudeContent{}
	}
	// The body presents the billed usage in Anthropic semantics; billing itself
	// keeps the handler's usage, so no converted receipt is recorded here.
	message.Usage = relaymodel.ClaudeUsageFromUsage(usage, ConvertedClaudeUsageModel(c, message.Model))
	payload, err := json.Marshal(message)
	if err != nil {
		return usage, ErrorWrapper(errors.Wrap(err, "marshal Claude message from chat completion"), "marshal_response_body_failed", http.StatusInternalServerError)
	}
	original.Header().Set("Content-Type", "application/json")
	original.WriteHeader(status)
	if _, err := original.Write(payload); err != nil {
		return usage, ErrorWrapper(errors.Wrap(err, "write Claude message to client"), "write_response_body_failed", http.StatusInternalServerError)
	}
	lg.Debug("rendered legacy chat completion as Claude message",
		zap.Int("content_blocks", len(message.Content)),
		zap.String("stop_reason", message.StopReason))
	return usage, nil
}

// renderChatStreamAsClaude installs a Claude stream bridge while handler
// streams chat chunks, then terminates the Claude message with the handler's
// final usage. The stream is complete only when the handler never reported an
// interruption (MarkStreamInterrupted) and either a chunk carried a
// finish_reason or the handler finalized a stream that delivered content.
// Legacy handlers finalize even after a read error, so they report the
// interruption explicitly; a stream request answered with a JSON error body
// delivers no content and is incomplete. A failed or unfinished stream gets a
// Claude error event (when anything was sent) and an error result, so it is
// never presented as a complete answer. Parameters: c is the request context,
// m the request meta, and handler the chat stream renderer. Returns: the usage
// to settle and the error, if any.
func renderChatStreamAsClaude(c *gin.Context, m *meta.Meta, handler ClaudeChatHandler) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
	lg := gmw.GetLogger(c)
	bridge := newClaudeChatStreamBridge(c, m)
	previous, _ := c.Get(ctxkey.ResponseStreamRewriteHandler)
	c.Set(ctxkey.ResponseStreamRewriteHandler, StreamRewriteHandler(bridge))
	// Restore the previous rewriter even if the handler panics.
	defer c.Set(ctxkey.ResponseStreamRewriteHandler, previous)
	usage, respErr := handler()
	c.Set(ctxkey.ResponseStreamRewriteHandler, previous)

	switch {
	case respErr != nil:
		if bridge.em.started {
			bridge.em.Fail("api_error", "upstream stream failed")
		}
		lg.Debug("legacy chat stream failed under Claude conversion",
			zap.Bool("claude_stream_started", bridge.em.started),
			zap.Int("status_code", respErr.StatusCode))
	case bridge.completed():
		if usage == nil {
			// The handler reported no usage although the client received a
			// complete answer: settle an estimate instead of letting the
			// controller re-read the consumed provider body.
			usage = bridge.estimatedUsage()
		}
		bridge.finish(usage)
	default:
		lg.Warn("legacy chat stream ended without an upstream completion signal; reporting it as incomplete",
			zap.Bool("claude_stream_started", bridge.em.started),
			zap.Bool("handler_done_seen", bridge.doneSeen),
			zap.Bool("handler_reported_interruption", bridge.interrupted),
			zap.Bool("content_received", bridge.sawContent))
		if bridge.em.started {
			bridge.em.Fail("api_error", "upstream stream ended before completion")
		}
		if !bridge.sawContent && (usage == nil || usage.CompletionTokens <= 0) {
			// No provider content was parsed (e.g. a JSON error body answered
			// the stream request) and nothing reports generated output, so the
			// handler's usage is only its own estimate. Drop it so the
			// controller refunds, as the Chat path does when it rejects a JSON
			// body on a stream request.
			usage = nil
		}
		respErr = ErrorWrapper(errors.New("upstream stream ended without a completion signal"),
			"upstream_stream_incomplete", http.StatusBadGateway)
	}
	if usage == nil && bridge.sawContent {
		// Delivered content is paid provider work even without a receipt.
		usage = bridge.estimatedUsage()
	}
	if writeErr := bridge.em.Err(); writeErr != nil {
		lg.Debug("Claude SSE downstream write failed for legacy chat stream", zap.Error(writeErr))
	}
	return usage, respErr
}

// claudeChatStreamBridge is a StreamRewriteHandler that translates the chat
// chunks a legacy adaptor renders through RenderStreamChunkWithBridge into
// Claude Messages SSE events. message_start is written lazily with the first
// event, so a handler that fails before streaming leaves the client untouched
// and the controller can still render the error. Terminal events are deferred
// until the handler returns its final usage (see renderChatStreamAsClaude).
type claudeChatStreamBridge struct {
	em           *ClaudeStreamEmitter
	tr           *claudeChatStreamTranslator
	model        string
	promptTokens int
	doneSeen     bool
	// interrupted records that the handler stopped reading the provider stream
	// early (a read error, or EOF before the provider's own end event).
	interrupted bool
	// sawContent records that a chunk carried text, reasoning or a tool call.
	sawContent bool
	// output keeps up to claudeChatBridgeTextCap bytes of streamed text for a
	// usage estimate; outputBytes counts every streamed byte.
	output      strings.Builder
	outputBytes int
}

// claudeChatBridgeTextCap bounds the streamed text retained for a usage
// estimate; bytes beyond it are counted conservatively as one token each.
const claudeChatBridgeTextCap = 1 << 20

// newClaudeChatStreamBridge returns a bridge writing Claude events to the
// client writer of c. Parameters: c is the request context and m the request
// meta that supplies the model and the prompt estimate. Returns: the bridge.
func newClaudeChatStreamBridge(c *gin.Context, m *meta.Meta) *claudeChatStreamBridge {
	flush := func() { c.Writer.Flush() }
	em := NewClaudeStreamEmitter(c.Writer, flush)
	return &claudeChatStreamBridge{
		em:           em,
		tr:           newClaudeChatStreamTranslator(c, em, NewClaudeToolIDAllocator()),
		model:        m.ActualModelName,
		promptTokens: m.PromptTokens,
	}
}

// start emits message_start once, before the first content event.
func (b *claudeChatStreamBridge) start() {
	b.em.Start("", b.model, relaymodel.ClaudeUsage{InputTokens: b.promptTokens})
}

// HandleChunk translates one chat chunk into Claude content events. It always
// reports the chunk as handled so no chat SSE reaches the Claude client.
func (b *claudeChatStreamBridge) HandleChunk(_ *gin.Context, chunk *ChatCompletionsStreamResponse) (bool, bool) {
	if chunk == nil {
		return true, false
	}
	b.start()
	b.recordOutput(chunk)
	b.tr.consume(chunk)
	return true, false
}

// recordOutput accumulates the text, reasoning and tool-argument bytes of a
// chunk for estimatedUsage. Parameters: chunk is the chat chunk. Returns: none.
func (b *claudeChatStreamBridge) recordOutput(chunk *ChatCompletionsStreamResponse) {
	for i := range chunk.Choices {
		delta := chunk.Choices[i].Delta
		parts := []string{delta.StringContent()}
		for _, reasoning := range []*string{delta.ReasoningContent, delta.Reasoning, delta.Thinking} {
			if reasoning != nil {
				parts = append(parts, *reasoning)
			}
		}
		for _, call := range delta.ToolCalls {
			if call.Function != nil {
				if args, ok := call.Function.Arguments.(string); ok {
					parts = append(parts, args)
				}
			}
		}
		if len(delta.ToolCalls) > 0 {
			b.sawContent = true
		}
		for _, part := range parts {
			if part != "" {
				b.sawContent = true
			}
			b.outputBytes += len(part)
			if room := claudeChatBridgeTextCap - b.output.Len(); room > 0 {
				b.output.WriteString(part[:min(len(part), room)])
			}
		}
	}
}

// estimatedUsage returns a flagged usage estimate for a stream whose handler
// reported none: the admission prompt estimate plus the streamed output.
// It takes no parameters and returns the estimate.
func (b *claudeChatStreamBridge) estimatedUsage() *relaymodel.Usage {
	completion := CountTokenText(b.output.String(), b.model) + (b.outputBytes - b.output.Len())
	if completion <= 0 && b.outputBytes > 0 {
		completion = 1
	}
	return &relaymodel.Usage{
		PromptTokens:          b.promptTokens,
		CompletionTokens:      completion,
		TotalTokens:           b.promptTokens + completion,
		BillingEstimateReason: "legacy_chat_stream_missing_usage",
	}
}

// MarkInterrupted records that the handler stopped reading the provider
// stream before the provider finished it, so the Claude message must not be
// terminated as complete.
func (b *claudeChatStreamBridge) MarkInterrupted() {
	b.interrupted = true
}

// completed reports whether the stream may be terminated as a complete Claude
// message: no interruption was reported, and either a chunk carried a
// finish_reason or the handler finalized a stream that delivered content. An
// interruption overrides finish_reason because some handlers synthesize a
// final chunk with one even when the provider never finished.
func (b *claudeChatStreamBridge) completed() bool {
	return !b.interrupted && (b.tr.finishSeen || (b.doneSeen && b.sawContent))
}

// HandleUpstreamDone records the upstream [DONE] sentinel and suppresses it.
func (b *claudeChatStreamBridge) HandleUpstreamDone(_ *gin.Context) (bool, bool) {
	b.doneSeen = true
	return true, false
}

// HandleDone records the handler's end of stream and suppresses the chat
// [DONE] sentinel; the Claude terminal events follow once usage is final.
func (b *claudeChatStreamBridge) HandleDone(_ *gin.Context) (bool, bool) {
	b.doneSeen = true
	return true, false
}

// FinalizeUsage ignores the handler's interim usage: some adaptors compute
// their usage only after the stream handler returns, so the final usage comes
// from the handler result instead.
func (b *claudeChatStreamBridge) FinalizeUsage(_ *relaymodel.Usage) {}

// StreamWriteError returns the first downstream write failure so the
// rendering loop can stop streaming to a disconnected client.
func (b *claudeChatStreamBridge) StreamWriteError() error {
	return b.em.Err()
}

// finish closes open blocks and emits message_delta and message_stop carrying
// usage in Anthropic semantics. Parameters: usage is the handler's final usage.
func (b *claudeChatStreamBridge) finish(usage *relaymodel.Usage) {
	b.start()
	b.tr.flushPendingTools()
	b.em.Finish(b.tr.finalStopReason(), relaymodel.ClaudeUsageFromUsage(usage, b.model))
}

// claudeChatCaptureWriter buffers what a legacy handler writes so the bridge
// can convert it. Headers stay apart from the client's until published.
type claudeChatCaptureWriter struct {
	gin.ResponseWriter
	header      http.Header
	body        bytes.Buffer
	status      int
	wroteHeader bool
}

// Header returns the captured header map, never the client's.
func (w *claudeChatCaptureWriter) Header() http.Header {
	return w.header
}

// WriteHeader records the status code without sending it.
func (w *claudeChatCaptureWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
}

// WriteHeaderNow records the default status when none was set.
func (w *claudeChatCaptureWriter) WriteHeaderNow() {
	w.WriteHeader(http.StatusOK)
}

// Write buffers data. Returns: the number of bytes buffered and a nil error.
func (w *claudeChatCaptureWriter) Write(data []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.Write(data)
}

// WriteString buffers s. Returns: the number of bytes buffered and a nil error.
func (w *claudeChatCaptureWriter) WriteString(s string) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.WriteString(s)
}

// Flush is a no-op: nothing is sent until the captured body is converted.
func (w *claudeChatCaptureWriter) Flush() {}

// Status returns the captured status code, or 200 when none was written.
func (w *claudeChatCaptureWriter) Status() int {
	return w.statusCode()
}

// Written reports whether the handler wrote a status or body.
func (w *claudeChatCaptureWriter) Written() bool {
	return w.wroteHeader
}

// Size returns the number of buffered body bytes.
func (w *claudeChatCaptureWriter) Size() int {
	return w.body.Len()
}

// statusCode returns the captured status, defaulting to 200.
func (w *claudeChatCaptureWriter) statusCode() int {
	if w.status > 0 {
		return w.status
	}
	return http.StatusOK
}

// streamInterruptionMarker is implemented by stream rewriters that must know
// when a legacy handler stopped reading its provider stream early.
type streamInterruptionMarker interface {
	MarkInterrupted()
}

// MarkStreamInterrupted reports to the installed stream rewriter, if it cares,
// that the calling stream handler stopped before the provider finished the
// stream: a read error, or EOF before the provider's own end event. Handlers
// call it right before FinalizeStreamWithBridge; it never changes what Chat
// Completions clients receive. Parameters: c is the request context. Returns:
// nothing.
func MarkStreamInterrupted(c *gin.Context) {
	if marker, ok := StreamRewriterFromContext(c).(streamInterruptionMarker); ok {
		marker.MarkInterrupted()
	}
}

// ConvertClaudeMessageToChat converts one Claude message into the equivalent
// Chat Completions messages, including tool_use blocks as assistant tool calls
// and tool_result blocks as tool messages. Parameters: msg is the Claude
// message. Returns: one or more Chat messages.
func ConvertClaudeMessageToChat(msg relaymodel.ClaudeMessage) []relaymodel.Message {
	return convertClaudeMessageToOpenAI(msg)
}

// ConvertClaudeToolsToChat converts Claude tool declarations into Chat
// Completions tools. A server tool (a type without input_schema) becomes a
// built-in tool of the normalized type; a client tool becomes a function whose
// parameters are a copy of its input_schema, or an empty object when the
// schema is absent or not an object. Parameters: tools are the Claude tools.
// Returns: the Chat tools, nil when tools is empty.
func ConvertClaudeToolsToChat(tools []relaymodel.ClaudeTool) []relaymodel.Tool {
	var converted []relaymodel.Tool
	for _, claudeTool := range tools {
		if strings.TrimSpace(claudeTool.Type) != "" && claudeTool.InputSchema == nil {
			converted = append(converted, relaymodel.Tool{Type: normalizeClaudeBuiltinToolType(claudeTool.Type)})
			continue
		}
		parameters, ok := claudeTool.InputSchema.(map[string]any)
		if !ok {
			parameters = map[string]any{}
		}
		// Downstream normalization owns its map; retain caller tool schemas.
		parameters = deepCopyMapAny(parameters)
		converted = append(converted, relaymodel.Tool{
			Type: "function",
			Function: &relaymodel.Function{
				Name:        claudeTool.Name,
				Description: claudeTool.Description,
				Parameters:  parameters,
			},
		})
	}
	return converted
}

// ClaudeSystemText returns the text of a Claude system prompt: the string
// itself, or the non-empty text blocks joined by newlines. Parameters: system
// is the Claude request's system field. Returns: the text, "" when absent.
func ClaudeSystemText(system any) string {
	switch value := system.(type) {
	case string:
		return value
	case []any:
		var parts []string
		for _, block := range value {
			blockMap, ok := block.(map[string]any)
			if !ok || blockMap["type"] != "text" {
				continue
			}
			if text, ok := blockMap["text"].(string); ok && text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// ConvertClaudeToolChoiceToChat converts a Claude tool_choice into its Chat
// Completions form, for example {"type":"tool","name":"x"} into a function
// choice. Parameters: choice is the Claude tool_choice. Returns: the Chat
// tool_choice.
func ConvertClaudeToolChoiceToChat(choice any) any {
	return normalizeClaudeToolChoice(choice)
}
