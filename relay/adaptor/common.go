package adaptor

import (
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/meta"
)

const (
	extraRequestHeaderPrefix = "X-"
	requestPreviewLimit      = 4096
)

func SetupCommonRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) {
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	req.Header.Set("Accept", c.Request.Header.Get("Accept"))
	for key, values := range c.Request.Header {
		if strings.HasPrefix(key, extraRequestHeaderPrefix) {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	}
	if meta.IsStream && c.Request.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "text/event-stream")
	}
}

func DoRequestHelper(a Adaptor, c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	fullRequestURL, err := a.GetRequestURL(meta)
	if err != nil {
		return nil, errors.Wrap(err, "get request url failed")
	}

	var (
		preview   []byte
		truncated bool
		bodySize  = -1
	)
	if requestBody != nil {
		if sized, ok := requestBody.(interface{ Len() int }); ok {
			bodySize = sized.Len()
		} else if sized64, ok := requestBody.(interface{ Size() int64 }); ok {
			bodySize = int(sized64.Size())
		}
		if seeker, ok := requestBody.(io.ReadSeeker); ok {
			currentPos, _ := seeker.Seek(0, io.SeekCurrent)
			if bodySize < 0 {
				total, err := seeker.Seek(0, io.SeekEnd)
				if err == nil {
					bodySize = int(total)
				}
			}
			_, _ = seeker.Seek(currentPos, io.SeekStart)
			buf := make([]byte, requestPreviewLimit+1)
			n, err := seeker.Read(buf)
			if err != nil && err != io.EOF {
				n = 0
			}
			if n > requestPreviewLimit {
				preview = append([]byte(nil), buf[:requestPreviewLimit]...)
				truncated = true
			} else {
				preview = append([]byte(nil), buf[:n]...)
				truncated = false
			}
			_, _ = seeker.Seek(currentPos, io.SeekStart)
		}
	}

	req, err := gutils.NewReusableRequest(gmw.Ctx(c),
		c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return nil, errors.Wrap(err, "new request failed")
	}

	req.Header.Set("Content-Type", c.GetString(ctxkey.ContentType))

	err = a.SetupRequestHeader(c, req, meta)
	if err != nil {
		return nil, errors.Wrap(err, "setup request header failed")
	}

	// Prepare tagged logger and propagate to context
	lg := gmw.GetLogger(c).With(
		zap.String("url", fullRequestURL),
		zap.Int("channelId", meta.ChannelId),
		zap.Int("userId", meta.UserId),
		zap.String("model", meta.ActualModelName),
		zap.String("channelName", a.GetChannelName()),
	)
	ctx := gmw.Ctx(c)
	ctx = gmw.SetLogger(ctx, lg)

	// Log upstream request for billing tracking
	fields := []zap.Field{
		zap.String("method", req.Method),
		zap.String("url", fullRequestURL),
		zap.Bool("body_truncated", truncated),
		zap.ByteString("body_preview", preview),
	}
	if bodySize >= 0 {
		fields = append(fields, zap.Int("body_bytes", bodySize))
	}
	lg.Debug("forwarding request to upstream channel", fields...)

	// Optionally: Record when request is forwarded to upstream (non-standard event)
	tracing.RecordTraceTimestamp(c, model.TimestampRequestForwarded)
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)

	resp, err := DoRequest(c, req)
	if err != nil {
		// Return error without logging - let the calling ErrorWrapper function handle logging
		// This prevents duplicate logging when ErrorWrapper also logs the error
		return nil, errors.Wrapf(err, "upstream request failed for channel %s (id: %d)", a.GetChannelName(), meta.ChannelId)
	}
	// Add debug log for non-200 statuses to help diagnose model mapping issues
	if resp != nil && resp.StatusCode >= 400 {
		lg.Debug("upstream returned error status",
			zap.Int("status", resp.StatusCode),
			zap.String("model", meta.ActualModelName),
			zap.String("url", fullRequestURL),
		)
	}

	return resp, nil
}

func DoRequest(c *gin.Context, req *http.Request) (*http.Response, error) {
	// keep logger from context if available
	httpClient := client.HTTPClient
	if httpClient == nil {
		client.Init()
		httpClient = client.HTTPClient
		if httpClient == nil {
			httpClient = http.DefaultClient
		}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "perform upstream request")
	}
	if resp == nil {
		return nil, errors.New("resp is nil")
	}

	// Optionally: Record when first response is received from upstream (non-standard event)
	tracing.RecordTraceTimestamp(c, model.TimestampFirstUpstreamResponse)

	if req.Body != nil {
		_ = req.Body.Close()
	}
	if c.Request != nil && c.Request.Body != nil {
		_ = c.Request.Body.Close()
	}

	return resp, nil
}
