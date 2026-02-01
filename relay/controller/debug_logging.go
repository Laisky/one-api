package controller

import (
	"bytes"
	"io"
	"net/http"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/ctxkey"
)

const debugLogBodyLimit = common.DefaultLogBodyLimit

// DebugResponseWriter wraps gin.ResponseWriter and captures a preview of the outbound body for debug logging.
type DebugResponseWriter struct {
	gin.ResponseWriter
	buffer bytes.Buffer
	limit  int
	total  int
}

func newDebugResponseWriter(w gin.ResponseWriter, limit int) *DebugResponseWriter {
	return &DebugResponseWriter{ResponseWriter: w, limit: limit}
}

// Write proxies the payload to the underlying writer while storing a limited preview for later logging.
func (w *DebugResponseWriter) Write(data []byte) (int, error) {
	w.capture(data)
	return w.ResponseWriter.Write(data)
}

// WriteString proxies the string payload to the underlying writer while storing a limited preview for later logging.
func (w *DebugResponseWriter) WriteString(s string) (int, error) {
	w.capture([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

func (w *DebugResponseWriter) capture(data []byte) {
	if len(data) == 0 {
		return
	}
	w.total += len(data)
	if w.limit <= 0 {
		return
	}
	if w.buffer.Len() >= w.limit {
		return
	}
	remaining := w.limit - w.buffer.Len()
	if remaining <= 0 {
		return
	}
	if len(data) > remaining {
		data = data[:remaining]
	}
	_, _ = w.buffer.Write(data)
}

// Snapshot returns the captured preview, whether it was truncated, and the total bytes written.
func (w *DebugResponseWriter) Snapshot() ([]byte, bool, int) {
	preview := w.buffer.Bytes()
	truncated := w.limit > 0 && w.total > w.limit
	return preview, truncated, w.total
}

// EnsureDebugResponseWriter attaches a response writer wrapper that records outbound bodies for logging.
func EnsureDebugResponseWriter(c *gin.Context) *DebugResponseWriter {
	if v, ok := c.Get(ctxkey.DebugResponseWriter); ok {
		if existing, ok := v.(*DebugResponseWriter); ok {
			c.Writer = existing
			return existing
		}
	}
	wrapper := newDebugResponseWriter(c.Writer, debugLogBodyLimit)
	c.Writer = wrapper
	c.Set(ctxkey.DebugResponseWriter, wrapper)
	return wrapper
}

// LogClientResponse emits a DEBUG log summarizing the HTTP response returned to the caller.
func LogClientResponse(c *gin.Context, message string) {
	lg := gmw.GetLogger(c)
	writer, _ := c.Get(ctxkey.DebugResponseWriter)
	drw, _ := writer.(*DebugResponseWriter)
	status := c.Writer.Status()
	fields := []zap.Field{
		zap.Int("status_code", status),
		zap.String("method", c.Request.Method),
		zap.String("url", c.Request.URL.String()),
	}
	if drw != nil {
		preview, truncated, total := drw.Snapshot()
		fields = append(fields,
			zap.Int("body_bytes", total),
			zap.Bool("body_truncated", truncated),
			zap.ByteString("body_preview", preview),
		)
	}
	lg.Debug(message, fields...)
}

func logClientRequestPayload(c *gin.Context, label string) error {
	lg := gmw.GetLogger(c)
	body, err := common.GetRequestBody(c)
	if err != nil {
		return err
	}
	preview, truncated := sanitizeRequestBodyForLogging(body, debugLogBodyLimit)
	lg.Debug("client request received",
		zap.String("label", label),
		zap.String("method", c.Request.Method),
		zap.String("url", c.Request.URL.String()),
		zap.Int("body_bytes", len(body)),
		zap.Bool("body_truncated", truncated),
		zap.ByteString("body_preview", preview),
	)
	return nil
}

func logUpstreamResponseFromCapture(lg glog.Logger, resp *http.Response, capture *loggingReadCloser, label string) {
	if resp == nil {
		lg.Debug("upstream response missing",
			zap.String("label", label),
		)
		return
	}
	preview, truncated, total := capture.Snapshot()
	fields := make([]zap.Field, 0, 7)
	fields = append(fields,
		zap.String("label", label),
		zap.Int("status_code", resp.StatusCode),
		zap.String("content_type", resp.Header.Get("Content-Type")),
		zap.Bool("body_truncated", truncated),
		zap.ByteString("body_preview", preview),
		zap.Int("body_bytes", total),
	)
	if resp.Request != nil {
		fields = append(fields,
			zap.String("method", resp.Request.Method),
			zap.String("url", resp.Request.URL.String()),
		)
	}
	lg.Debug("upstream response received", fields...)
}

func logUpstreamResponseFromBytes(lg glog.Logger, resp *http.Response, body []byte, label string) {
	if resp == nil {
		lg.Debug("upstream response missing",
			zap.String("label", label),
		)
		return
	}
	preview, truncated := truncateBytes(body, debugLogBodyLimit)
	fields := []zap.Field{
		zap.String("label", label),
		zap.Int("status_code", resp.StatusCode),
		zap.String("content_type", resp.Header.Get("Content-Type")),
		zap.Int("body_bytes", len(body)),
		zap.Bool("body_truncated", truncated),
		zap.ByteString("body_preview", preview),
	}
	if resp.Request != nil {
		fields = append(fields,
			zap.String("method", resp.Request.Method),
			zap.String("url", resp.Request.URL.String()),
		)
	}
	lg.Debug("upstream response received", fields...)
}

func truncateBytes(input []byte, limit int) ([]byte, bool) {
	if limit <= 0 || len(input) <= limit {
		return input, false
	}
	return input[:limit], true
}

// sanitizeRequestBodyForLogging ensures logged payload values do not exceed the limit by
// redacting base64 data and truncating oversized fields.
func sanitizeRequestBodyForLogging(body []byte, limit int) ([]byte, bool) {
	return common.SanitizePayloadForLogging(body, limit)
}

type loggingReadCloser struct {
	io.ReadCloser
	buffer bytes.Buffer
	limit  int
	total  int
}

func newLoggingReadCloser(rc io.ReadCloser, limit int) *loggingReadCloser {
	return &loggingReadCloser{ReadCloser: rc, limit: limit}
}

func (r *loggingReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		r.total += n
		if r.limit > 0 && r.buffer.Len() < r.limit {
			remaining := r.limit - r.buffer.Len()
			if remaining > 0 {
				toWrite := p[:n]
				if len(toWrite) > remaining {
					toWrite = toWrite[:remaining]
				}
				_, _ = r.buffer.Write(toWrite)
			}
		}
	}
	return n, err
}

func (r *loggingReadCloser) Snapshot() ([]byte, bool, int) {
	preview := r.buffer.Bytes()
	truncated := r.limit > 0 && r.total > r.limit
	return preview, truncated, r.total
}

func wrapUpstreamResponse(resp *http.Response) *loggingReadCloser {
	if resp == nil || resp.Body == nil {
		return nil
	}
	capture := newLoggingReadCloser(resp.Body, debugLogBodyLimit)
	resp.Body = capture
	return capture
}
