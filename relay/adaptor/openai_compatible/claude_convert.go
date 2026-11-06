package openai_compatible

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/render"
	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/Laisky/one-api/relay/adaptor/common/toolnamesafe"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// ConvertOpenAIResponseToClaudeResponse converts an OpenAI-compatible response
// (Chat Completions or Response API) into Claude Messages JSON http.Response.
func ConvertOpenAIResponseToClaudeResponse(c *gin.Context, resp *http.Response) (*http.Response, *relaymodel.ErrorWithStatusCode) {
	body, err, closeErr := readAndCloseResponseBody(c, resp.Body)
	if err != nil {
		return nil, ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	if closeErr != nil {
		logger := gmw.GetLogger(c)
		logger.Debug("failed to close upstream response body after buffered conversion read", zap.Error(closeErr))
	}

	// 1) Try Response API format first
	var responseAPIResp responseAPIResponse
	if err := json.Unmarshal(body, &responseAPIResp); err == nil && responseAPIResp.Object == "response" {
		claudeResp := responseAPIResponseToClaude(c, &responseAPIResp)
		claudeResp.Usage = PresentConvertedClaudeUsage(c, responseAPIUsageToUsage(responseAPIResp.Usage), responseAPIResp.Model)
		return MarshalClaudeHTTPResponse(resp, claudeResp)
	}

	// 2) Fallback: Chat Completions format
	var chatResp chatTextResponse
	if err := json.Unmarshal(body, &chatResp); err == nil && len(chatResp.Choices) > 0 {
		claudeResp := chatResponseToClaude(c, &chatResp)
		claudeResp.Usage = PresentConvertedClaudeUsage(c, &chatResp.Usage, chatResp.Model)
		return MarshalClaudeHTTPResponse(resp, claudeResp)
	}

	// 3) Unknown format – return original payload (controller may handle error)
	SetConvertedClaudeUsage(c, nil)
	newResp := &http.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	return newResp, nil
}

// SetConvertedClaudeUsage records usage as the authoritative OpenAI-semantics
// settlement receipt of the Claude Messages response converted in this attempt
// (ctxkey.ConvertedResponseUsage), so the controller never re-derives billing
// from the Anthropic-semantics body. Parameters: c is the request context (nil
// is a no-op, as in unit tests) and usage the receipt, nil when the upstream
// reported no counters. Returns: nothing.
func SetConvertedClaudeUsage(c *gin.Context, usage *relaymodel.Usage) {
	if c == nil {
		return
	}
	c.Set(ctxkey.ConvertedResponseUsage, usage)
}

// ConvertedClaudeUsageModel returns the model a converted Claude response's
// usage is billed under: the mapped upstream model of the request meta when the
// context carries one (the model quota.Compute prices), otherwise upstreamModel.
// Parameters: c is the request context (may be nil) and upstreamModel the model
// the upstream echoed. Returns: the model name used to interpret cache buckets.
func ConvertedClaudeUsageModel(c *gin.Context, upstreamModel string) string {
	if c != nil {
		if raw, ok := c.Get(ctxkey.Meta); ok {
			if requestMeta, valid := raw.(*meta.Meta); valid && requestMeta != nil && requestMeta.ActualModelName != "" {
				return requestMeta.ActualModelName
			}
		}
	}
	return upstreamModel
}

// PresentConvertedClaudeUsage is the shared exit of every non-stream converter
// that renders an OpenAI-semantics upstream as a Claude Messages body. It
// records the settlement receipt derived from usage (see
// relaymodel.ConvertedClaudeBillingUsage) and returns that receipt presented in
// Anthropic semantics, so the client-visible input_tokens + cache buckets equal
// the billed prompt. Parameters: c is the request context (may be nil), usage
// the measured upstream receipt (may be nil), and upstreamModel the model the
// upstream echoed. Returns: the Claude usage object for the response body.
func PresentConvertedClaudeUsage(c *gin.Context, usage *relaymodel.Usage, upstreamModel string) relaymodel.ClaudeUsage {
	receipt := relaymodel.ConvertedClaudeBillingUsage(usage)
	SetConvertedClaudeUsage(c, receipt)
	if c != nil && receipt != nil {
		gmw.GetLogger(c).Debug("recorded converted Claude usage receipt",
			zap.Int("prompt_tokens", receipt.PromptTokens),
			zap.Int("completion_tokens", receipt.CompletionTokens),
			zap.Int("cache_write_5m_tokens", receipt.CacheWrite5mTokens),
			zap.Int("cache_write_1h_tokens", receipt.CacheWrite1hTokens),
		)
	}
	return relaymodel.ClaudeUsageFromUsage(receipt, ConvertedClaudeUsageModel(c, upstreamModel))
}

// responseAPIUsageToUsage lifts the minimal Responses usage receipt into the
// canonical usage counters. Parameters: r is the receipt (may be nil).
// Returns: canonical usage, or nil when r is nil.
func responseAPIUsageToUsage(r *responseAPIUsage) *relaymodel.Usage {
	if r == nil {
		return nil
	}
	usage := &relaymodel.Usage{
		PromptTokens:       r.InputTokens,
		CompletionTokens:   r.OutputTokens,
		TotalTokens:        r.TotalTokens,
		CacheWrite5mTokens: r.CacheWrite5mTokens,
		CacheWrite1hTokens: r.CacheWrite1hTokens,
	}
	if r.InputTokensDetails != nil {
		usage.PromptTokensDetails = &relaymodel.UsagePromptTokensDetails{CachedTokens: r.InputTokensDetails.CachedTokens}
	}
	return usage
}

// responseAPIResponseToClaude maps an OpenAI Response API response to a Claude
// Messages response. The stop_reason follows the response status
// (incomplete/max_output_tokens → max_tokens) and becomes tool_use when the
// turn requests tools. Usage is filled by the caller through
// PresentConvertedClaudeUsage.
func responseAPIResponseToClaude(c *gin.Context, r *responseAPIResponse) relaymodel.ClaudeResponse {
	out := relaymodel.ClaudeResponse{
		ID:      ClaudeMessageID(r.Id),
		Type:    "message",
		Role:    "assistant",
		Model:   r.Model,
		Content: []relaymodel.ClaudeContent{},
	}

	ids := NewClaudeToolIDAllocator()
	for _, item := range r.Output {
		switch item.Type {
		case "message":
			if strings.EqualFold(item.Role, "assistant") {
				for _, part := range item.Content {
					if text := responseAPIContentText(part); text != "" {
						out.Content = append(out.Content, relaymodel.ClaudeContent{Type: claudeBlockText, Text: text})
					}
				}
			}
		case "reasoning":
			for _, s := range item.Summary {
				if s.Type == "summary_text" && s.Text != "" {
					out.Content = append(out.Content, relaymodel.ClaudeContent{Type: claudeBlockThinking, Thinking: s.Text})
				}
			}
		case "function_call":
			// Restore any sanitized name so the client sees the original
			// identifier it submitted.
			callID := item.CallId
			if callID == "" {
				callID = item.Id
			}
			out.Content = append(out.Content, relaymodel.ClaudeContent{
				Type:  claudeBlockToolUse,
				ID:    ids.Allocate(callID),
				Name:  toolnamesafe.RestoreToolName(c, item.Name),
				Input: ClaudeToolInputFromArguments(item.Arguments),
			})
		}
	}

	incompleteReason := ""
	if r.IncompleteDetails != nil {
		incompleteReason = r.IncompleteDetails.Reason
	}
	out.StopReason = FinalizeClaudeStopReason(ClaudeStopReasonFromResponseStatus(r.Status, incompleteReason), ClaudeContentHasToolUse(out.Content))
	return out
}

// responseAPIContentText returns the client-visible text of a Responses message part.
func responseAPIContentText(content responseAPIContent) string {
	switch strings.ToLower(strings.TrimSpace(content.Type)) {
	case "output_text", "input_text", "text":
		return strings.TrimSpace(content.Text)
	case "output_json":
		if len(content.JSON) > 0 {
			return strings.TrimSpace(string(content.JSON))
		}
		return strings.TrimSpace(content.Text)
	default:
		return ""
	}
}

// chatResponseToClaude maps an OpenAI Chat Completion response to a Claude
// Messages response, mapping finish_reason to stop_reason and every tool call
// to a tool_use block with a valid unique id and an object input. Usage is
// filled by the caller through PresentConvertedClaudeUsage.
func chatResponseToClaude(c *gin.Context, r *chatTextResponse) relaymodel.ClaudeResponse {
	out := relaymodel.ClaudeResponse{
		ID:      ClaudeMessageID(r.Id),
		Type:    "message",
		Role:    "assistant",
		Model:   r.Model,
		Content: []relaymodel.ClaudeContent{},
	}

	ids := NewClaudeToolIDAllocator()
	stopReason := ""
	for _, choice := range r.Choices {
		if thinking := ChatMessageThinking(choice.Message); thinking != "" {
			out.Content = append(out.Content, relaymodel.ClaudeContent{Type: claudeBlockThinking, Thinking: thinking})
		}

		if choice.Message.Content != nil {
			switch content := choice.Message.Content.(type) {
			case string:
				if content != "" {
					out.Content = append(out.Content, relaymodel.ClaudeContent{Type: claudeBlockText, Text: content})
				}
			case []relaymodel.MessageContent:
				for _, part := range content {
					if part.Type == "text" && part.Text != nil && *part.Text != "" {
						out.Content = append(out.Content, relaymodel.ClaudeContent{Type: claudeBlockText, Text: *part.Text})
					}
				}
			}
		}

		// Tool calls -> tool_use blocks; restore any sanitized name so the
		// client sees the original identifier it submitted.
		if len(choice.Message.ToolCalls) > 0 {
			toolnamesafe.RestoreToolCallNames(c, choice.Message.ToolCalls)
			for _, tc := range choice.Message.ToolCalls {
				name := ""
				var args any
				if tc.Function != nil {
					name, args = tc.Function.Name, tc.Function.Arguments
				}
				out.Content = append(out.Content, relaymodel.ClaudeContent{
					Type:  claudeBlockToolUse,
					ID:    ids.Allocate(tc.Id),
					Name:  name,
					Input: ClaudeToolInputFromArguments(args),
				})
			}
		}

		if choice.FinishReason != "" {
			stopReason = ClaudeStopReasonFromChatFinish(choice.FinishReason)
		}
	}

	out.StopReason = FinalizeClaudeStopReason(stopReason, ClaudeContentHasToolUse(out.Content))
	return out
}

// ConvertOpenAIStreamToClaudeSSE reads an OpenAI-compatible Chat Completions or
// Responses SSE stream and writes a conformant Claude Messages SSE stream to the
// client, returning the usage measured from the upstream bytes.
//
// The stream terminates with message_delta (carrying the mapped stop_reason)
// and message_stop whenever the upstream signalled completion: a [DONE]
// sentinel, a Chat finish_reason, or a terminal Responses event (Responses
// upstreams never send [DONE]). A failed Responses upstream yields an Anthropic
// error event. An upstream that drops without any completion signal is not
// disguised as complete: the client observes the stream end without
// message_stop and can retry.
func ConvertOpenAIStreamToClaudeSSE(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
	lg := gmw.GetLogger(c)

	// Prepare client for SSE
	common.SetEventStreamHeaders(c)

	observer := relaymodel.NewResponseUsageAccumulator(true)
	lineReader := commonsse.NewLineReader(&claudeUsageReader{reader: resp.Body, observer: observer}, commonsse.DefaultLineBufferSize)

	// Wrap the reader with heartbeats to prevent reverse-proxy timeouts (e.g. Cloudflare 524).
	hbr := render.NewHeartbeatLineReader(c, lineReader, render.DefaultHeartbeatInterval)
	defer hbr.Close()

	flush := func() {
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	em := NewClaudeStreamEmitter(c.Writer, flush)
	em.Start("", modelName, relaymodel.ClaudeUsage{InputTokens: promptTokens})
	tr := newClaudeOpenAIStreamTranslator(c, em)

	upstreamDone := false
	var streamErr error
	for {
		line, err := hbr.Next()
		if err != nil {
			if !errkind.HasOnlyCause(err, io.EOF) {
				streamErr = err
			}
			break
		}

		var payload string
		if line.Oversized {
			payloadBytes, err := io.ReadAll(line.Large)
			if err != nil {
				streamErr = err
				break
			}
			payload = strings.TrimSpace(string(payloadBytes))
		} else {
			lineText := line.Text()
			if !strings.HasPrefix(lineText, "data:") {
				continue
			}
			payload = strings.TrimSpace(strings.TrimPrefix(lineText, "data:"))
		}
		if payload == "[DONE]" {
			upstreamDone = true
			break
		}
		tr.consume(payload)
	}

	if streamErr != nil {
		render.LogHeartbeatLineReaderError(c, lg, streamErr, hbr)
	}

	// Normalize raw provider evidence with the same bounded accumulator used
	// by converted-body fallback. Aggregate-only splits remain labelled estimates.
	// Responses upstreams never send [DONE]; their terminal event with a usage
	// receipt is the completion signal, as on the native Responses relay.
	// Without either, the stream was interrupted and output is estimated
	// conservatively from what was observed.
	if !upstreamDone && !tr.settledByReceipt() {
		observer.MarkIncomplete()
	}
	usage := observer.Finish(promptTokens, func(text string) int { return CountTokenText(text, modelName) })

	switch {
	case em.Failed():
		lg.Warn("upstream reported a failed response; emitted Claude error event instead of message_stop")
	case upstreamDone || tr.completed():
		// The final message_delta carries the measured receipt in Anthropic
		// semantics; message_start only had the gateway prompt estimate.
		tr.finish(relaymodel.ClaudeUsageFromUsage(usage, modelName))
	default:
		// The Claude Messages API does not use [DONE]; the stream simply closes
		// after message_stop. If upstream dropped without any completion signal,
		// do not fabricate a stop reason or message_stop (honest proxy).
		lg.Warn("upstream stream ended without a completion signal, not emitting message_delta/message_stop for Claude SSE conversion")
	}
	if dropped := em.Dropped(); dropped > 0 {
		lg.Debug("suppressed out-of-order Claude stream events", zap.Int("dropped", dropped))
	}
	if writeErr := em.Err(); writeErr != nil {
		lg.Debug("Claude SSE downstream write failed", zap.Error(writeErr))
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		lg.Debug("failed to close upstream body after Claude SSE conversion", zap.Error(closeErr))
	}
	return usage, nil
}
