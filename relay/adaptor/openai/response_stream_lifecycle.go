package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
)

// MaxResponseStreamEventBytes bounds a supported Responses data payload before
// parsing or forwarding. The line reader's smaller buffer is not a billing bypass.
const MaxResponseStreamEventBytes = 4 << 20
const responseStreamEstimateKey = "one_api.response_stream_estimate_reason"

// AnnotateResponseStreamUsage attaches the transport's uncertainty to the
// caller's existing fallback estimate without inventing another settlement path.
func AnnotateResponseStreamUsage(c *gin.Context, usage *model.Usage) {
	if usage != nil {
		if reason := c.GetString(responseStreamEstimateKey); reason != "" {
			usage.BillingEstimateReason = reason
		}
	}
}

// boundedResponseStreamLine converts a supported large payload into the same
// parsing path as a small line, rejecting unbounded/malformed work before delivery.
func boundedResponseStreamLine(line commonsse.Line) (commonsse.Line, error) {
	if !line.Oversized {
		return line, nil
	}
	payload, err := io.ReadAll(io.LimitReader(line.Large, MaxResponseStreamEventBytes+1))
	if err != nil {
		return commonsse.Line{}, errors.Wrap(err, "read bounded Responses event")
	}
	if len(payload) > MaxResponseStreamEventBytes {
		return commonsse.Line{}, errors.New("Responses event exceeds the supported 4 MiB payload limit")
	}
	if !json.Valid(payload) {
		return commonsse.Line{}, errors.New("malformed oversized Responses event")
	}
	return commonsse.Line{Kind: commonsse.LineKindData, Small: append([]byte("data: "), payload...)}, nil
}

// responseStreamBody closes a provider body exactly once across cancellation and
// ordinary cleanup; sync.Once also publishes any close error to the caller.
type responseStreamBody struct {
	io.ReadCloser
	once sync.Once
	err  error
}

// Close releases the provider transport while retaining its established error contract.
func (b *responseStreamBody) Close() error {
	b.once.Do(func() { b.err = b.ReadCloser.Close() })
	return b.err
}

// responseStreamWriter remembers delivery errors even when a rendering helper
// can only report them through Gin. It never substitutes a successful short write.
type responseStreamWriter struct {
	gin.ResponseWriter
	err error
}

// Write preserves the original writer result and records the first delivery failure.
func (w *responseStreamWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil && w.err == nil {
		w.err = err
	}
	return n, err
}

// WriteString follows the same checked delivery boundary as Write.
func (w *responseStreamWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// responseStreamLifecycle owns only transport cleanup and delivery state, never
// quota settlement. Already observed counters remain owned by the handler.
type responseStreamLifecycle struct {
	body                 *responseStreamBody
	writer               *responseStreamWriter
	stopCancellation     func() bool
	gap, terminalReceipt bool
}

// beginResponseStream makes cancellation interrupt a stalled oversized read too.
// The cancellation callback captures a body, not a pooled Gin context.
func beginResponseStream(c *gin.Context, resp *http.Response) *responseStreamLifecycle {
	c.Set(responseStreamEstimateKey, "")
	body := &responseStreamBody{ReadCloser: resp.Body}
	writer := &responseStreamWriter{ResponseWriter: c.Writer}
	resp.Body = body
	c.Writer = writer
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	stop := context.AfterFunc(ctx, func() { _ = body.Close() })
	return &responseStreamLifecycle{body: body, writer: writer, stopCancellation: stop}
}

// finish publishes cleanup failures without erasing usage returned by the handler.
// A secondary close failure does not replace the primary read/delivery failure.
func (s *responseStreamLifecycle) finish(c *gin.Context, apiErr **model.ErrorWithStatusCode) {
	s.stopCancellation()
	closeErr := s.body.Close()
	c.Writer = s.writer.ResponseWriter
	if *apiErr == nil && s.writer.err != nil {
		*apiErr = ErrorWrapper(s.writer.err, "write_response_body_failed", http.StatusInternalServerError)
	}
	if *apiErr == nil && closeErr != nil {
		*apiErr = ErrorWrapper(closeErr, "close_response_body_failed", http.StatusInternalServerError)
	}
	if *apiErr == nil {
		recordUpstreamCompleted(c)
	}
}

// responseStreamSnapshotText counts provider-visible billable content from a
// cumulative output snapshot without adding it to already counted deltas.
func responseStreamSnapshotText(outputs []OutputItem) string {
	var text strings.Builder
	for _, item := range outputs {
		for _, content := range item.Content {
			text.WriteString(content.Text)
			if len(content.JSON) > 0 {
				text.Write(content.JSON)
			}
		}
		for _, content := range item.Summary {
			text.WriteString(content.Text)
		}
		if item.Type == "function_call" {
			text.WriteString(item.Arguments)
		}
	}
	return text.String()
}
