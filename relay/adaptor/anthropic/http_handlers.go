package anthropic

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/render"
	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
)

// maxClaudeHTTPPayload bounds each decoded upstream JSON object, not the overall SSE stream.
const maxClaudeHTTPPayload = 32 << 20

// claudeHTTPError wraps protocol and delivery failures without logging payloads.
func claudeHTTPError(err error) *model.ErrorWithStatusCode {
	return &model.ErrorWithStatusCode{StatusCode: http.StatusBadGateway, Error: model.Error{
		Type: "upstream_error", Code: "claude_response_failed", Message: "Claude response failed", RawError: errors.Wrap(err, "Claude HTTP response"),
	}}
}

// retainedHTTPUsage preserves a verified receipt or labels an accepted response of unknown cost.
func retainedHTTPUsage(receipt *httpUsageReceipt) *model.Usage {
	usage := receipt.snapshot()
	if usage == nil {
		usage = &model.Usage{}
	}
	if receipt.input == nil || receipt.output == nil {
		usage.BillingEstimateReason = "incomplete_usage_after_accepted_claude_http"
	}
	return usage
}

// httpResponseWriter captures errors even when a legacy Responses bridge ignores a writer result.
type httpResponseWriter struct {
	gin.ResponseWriter
	err error
}

// Write forwards bytes and retains the first failed or short write.
func (w *httpResponseWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.ResponseWriter.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = errors.Wrap(err, "write Claude response")
	}
	return n, w.err
}

// WriteString routes string writes through the same checked delivery boundary.
func (w *httpResponseWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// ClaudeNativeStreamHandler preserves named native SSE and cumulative receipts through clean completion.
func ClaudeNativeStreamHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	return handleClaudeHTTPStream(c, resp, true)
}

// StreamHandler converts Claude SSE to Chat or Responses without inventing successful termination.
func StreamHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	return handleClaudeHTTPStream(c, resp, false)
}

// handleClaudeHTTPStream validates receipts and framing, retaining usage when reading or writing fails.
func handleClaudeHTTPStream(c *gin.Context, resp *http.Response, native bool) (result *model.ErrorWithStatusCode, usage *model.Usage) {
	state := &httpStreamState{native: native, blocks: map[int]*httpContentBlock{}, id: tracing.GenerateChatCompletionID(c), created: helper.GetTimestamp()}
	defer func() {
		if usage == nil {
			usage = retainedHTTPUsage(&state.receipt)
		}
		if result != nil && !state.finished {
			usage.BillingEstimateReason = "incomplete_stream_usage_after_accepted_claude_http"
		}
		// Server-tool work already performed upstream stays billable even when
		// the stream later fails; the tally never sums repeated receipts.
		RecordServerToolInvocations(c, state.receipt.tools.counts())
	}()
	if resp == nil || resp.Body == nil {
		return claudeHTTPError(errors.New("missing Claude response body")), nil
	}
	closed := false
	defer func() {
		if !closed {
			if err := resp.Body.Close(); err != nil && result == nil {
				result = claudeHTTPError(errors.Wrap(err, "close Claude stream"))
			}
		}
	}()
	original := c.Writer
	writer := &httpResponseWriter{ResponseWriter: original}
	c.Writer = writer
	defer func() { c.Writer = original }()
	common.SetEventStreamHeaders(c)
	reader := render.NewHeartbeatLineReader(c, commonsse.NewLineReader(resp.Body, commonsse.DefaultLineBufferSize), render.DefaultHeartbeatInterval)
	defer reader.Close()
	eventType := ""
	for {
		line, err := reader.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return claudeHTTPError(err), nil
			}
			break
		}
		if writer.err != nil {
			return claudeHTTPError(writer.err), nil
		}
		if line.Kind == commonsse.LineKindEvent {
			eventType = strings.TrimSpace(strings.TrimPrefix(line.Text(), "event:"))
			continue
		}
		if line.Kind != commonsse.LineKindData {
			continue
		}
		var raw []byte
		if line.Oversized {
			raw, err = io.ReadAll(io.LimitReader(line.Large, maxClaudeHTTPPayload+1))
			if err != nil {
				return claudeHTTPError(errors.Wrap(err, "read large Claude SSE object")), nil
			}
			if len(raw) > maxClaudeHTTPPayload {
				return claudeHTTPError(errors.New("Claude SSE object exceeds gateway 32 MiB limit")), nil
			}
		} else {
			raw = bytes.TrimSpace(bytes.TrimPrefix(line.Small, []byte("data:")))
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("[DONE]")) {
			eventType = ""
			continue
		}
		if eventType != "" {
			var envelope struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(raw, &envelope); err != nil {
				return claudeHTTPError(errors.Wrap(err, "decode Claude SSE object")), nil
			}
			if envelope.Type != eventType {
				return claudeHTTPError(errors.New("Claude SSE event label contradicts its JSON type")), nil
			}
		}
		eventType = ""
		if err := state.consume(c, raw); err != nil {
			return claudeHTTPError(err), nil
		}
	}
	if !state.stopped || !state.finished || state.receipt.input == nil || state.receipt.output == nil {
		return claudeHTTPError(errors.Wrap(io.ErrUnexpectedEOF, "incomplete Claude stream")), nil
	}
	err := resp.Body.Close()
	closed = true
	if err != nil {
		return claudeHTTPError(errors.Wrap(err, "close Claude stream")), nil
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		return claudeHTTPError(errors.WithStack(c.Request.Context().Err())), nil
	}
	if writer.err != nil {
		return claudeHTTPError(writer.err), nil
	}
	usage = state.receipt.snapshot()
	if native {
		if err := writeHTTPSSE(c, "message_stop", state.stopEvent); err != nil {
			return claudeHTTPError(err), usage
		}
	} else {
		if state.terminal != nil {
			if err := state.writeChat(c, state.terminal); err != nil {
				return claudeHTTPError(err), usage
			}
		}
		openai_compatible.FinalizeStreamWithBridge(c, usage)
		if writer.err != nil {
			return claudeHTTPError(writer.err), usage
		}
	}
	return nil, usage
}

// ClaudeNativeHandler retains opaque native response fields while extracting a verified receipt.
func ClaudeNativeHandler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	return handleClaudeHTTPJSON(c, resp, modelName, true)
}

// Handler converts a non-streaming Claude response, returning usage even on failed delivery.
func Handler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	return handleClaudeHTTPJSON(c, resp, modelName, false)
}

// handleClaudeHTTPJSON validates usage independently of content and checks the final downstream write.
func handleClaudeHTTPJSON(c *gin.Context, resp *http.Response, modelName string, native bool) (result *model.ErrorWithStatusCode, usage *model.Usage) {
	var receipt httpUsageReceipt
	admissionRejected := false
	defer func() {
		if usage == nil && !admissionRejected {
			usage = retainedHTTPUsage(&receipt)
		}
		if !admissionRejected {
			RecordServerToolInvocations(c, receipt.tools.counts())
		}
	}()
	if resp == nil || resp.Body == nil {
		return claudeHTTPError(errors.New("missing Claude response body")), nil
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxClaudeHTTPPayload+1))
	closeErr := resp.Body.Close()
	if readErr != nil {
		return claudeHTTPError(errors.Wrap(readErr, "read Claude response")), nil
	}
	if len(raw) > maxClaudeHTTPPayload {
		return claudeHTTPError(errors.New("Claude response exceeds gateway 32 MiB limit")), nil
	}
	var envelope struct {
		Type    string          `json:"type"`
		Model   string          `json:"model"`
		Usage   json.RawMessage `json:"usage"`
		Content json.RawMessage `json:"content"`
		Error   Error           `json:"error"`
	}
	if err := decodeClaudeJSON(raw, &envelope); err != nil {
		return claudeHTTPError(err), nil
	}
	if err := receipt.apply(envelope.Usage); err != nil {
		return claudeHTTPError(err), nil
	}
	receipt.tools.observeContent(envelope.Content)
	if closeErr != nil {
		return claudeHTTPError(errors.Wrap(closeErr, "close Claude response")), nil
	}
	if status := claudeAdmissionStatus(resp.StatusCode, envelope.Type, envelope.Error.Type, envelope.Usage); status != 0 {
		// Clean read, JSON decoding, receipt validation, and Close all succeeded.
		// Only this narrow proof can release a forwarded request's reservation.
		admissionRejected = true
		return &model.ErrorWithStatusCode{StatusCode: status, Error: model.Error{
			Type: model.ErrorType(envelope.Error.Type), Code: envelope.Error.Type, Message: envelope.Error.Message,
			RawError: errors.WithStack(&admissionRejectionError{}),
		}}, nil
	}
	if envelope.Error.Type != "" {
		status := resp.StatusCode
		if status < 400 {
			status = http.StatusBadGateway
		}
		return &model.ErrorWithStatusCode{StatusCode: status, Error: model.Error{Type: model.ErrorType(envelope.Error.Type), Code: envelope.Error.Type, Message: envelope.Error.Message, RawError: errors.New("Claude returned an error envelope")}}, nil
	}
	if envelope.Type != "message" || receipt.input == nil || receipt.output == nil {
		return claudeHTTPError(errors.New("Claude response has no valid message and complete usage")), nil
	}
	usage = receipt.snapshot()
	body := raw
	if native {
		if modelName != "" && envelope.Model != modelName {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				return claudeHTTPError(errors.Wrap(err, "decode native Claude fields")), usage
			}
			name, err := json.Marshal(modelName)
			if err != nil {
				return claudeHTTPError(errors.Wrap(err, "encode response model")), usage
			}
			fields["model"] = name
			var buffer bytes.Buffer
			encoder := json.NewEncoder(&buffer)
			encoder.SetEscapeHTML(false)
			if err := encoder.Encode(fields); err != nil {
				return claudeHTTPError(errors.Wrap(err, "encode native Claude response")), usage
			}
			body = bytes.TrimSuffix(buffer.Bytes(), []byte("\n"))
		}
	} else {
		var response Response
		if err := decodeClaudeJSON(raw, &response); err != nil {
			return claudeHTTPError(err), usage
		}
		converted := ResponseClaude2OpenAI(c, &response)
		converted.Model, converted.Usage = modelName, *usage
		encoded, err := json.Marshal(converted)
		if err != nil {
			return claudeHTTPError(errors.Wrap(err, "encode converted Claude response")), usage
		}
		body = encoded
	}
	c.Header("Content-Type", "application/json")
	// Keep request correlation and rate limits, not hop-by-hop framing or upstream cookies.
	for key, values := range resp.Header {
		lower := strings.ToLower(key)
		if lower == "apim-request-id" || lower == "request-id" || lower == "x-request-id" || lower == "retry-after" || strings.HasPrefix(lower, "anthropic-ratelimit-") {
			for _, value := range values {
				c.Writer.Header().Add(key, value)
			}
		}
	}
	c.Status(resp.StatusCode)
	n, err := c.Writer.Write(body)
	if err != nil {
		return claudeHTTPError(errors.Wrap(err, "write Claude response")), usage
	}
	if n != len(body) {
		return claudeHTTPError(errors.WithStack(io.ErrShortWrite)), usage
	}
	return nil, usage
}
