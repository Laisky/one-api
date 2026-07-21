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

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
)

const (
	extraRequestHeaderPrefix = "X-"
	requestPreviewLimit      = 4096
	channelAPIKeyPlaceholder = "{{key}}"
)

// SetupCommonRequestHeader copies shared downstream headers into the upstream
// request before provider-specific and channel-specific headers are applied.
// Parameters: c is the incoming Gin context, req is the outbound upstream
// request, and meta carries stream state. Return value: none.
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

// applyChannelCustomHeaders overlays channel-configured upstream headers after
// client headers and provider defaults have been populated. Parameters: req is
// the outbound upstream request, and meta carries channel configuration and API
// key material. Return value: an error when a configured header name is invalid.
func applyChannelCustomHeaders(req *http.Request, meta *meta.Meta) error {
	if req == nil || meta == nil || len(meta.Config.CustomHeaders) == 0 {
		return nil
	}

	for rawName, rawValue := range meta.Config.CustomHeaders {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		if !isValidCustomHeaderName(name) {
			return errors.Errorf("invalid custom header name %q", name)
		}
		req.Header.Set(name, expandChannelCustomHeaderValue(rawValue, meta.APIKey))
	}

	return nil
}

// expandChannelCustomHeaderValue replaces the channel API-key placeholder in a
// configured header value. Parameters: value is the stored template, and apiKey
// is the selected channel credential. Return value: the rendered header value.
func expandChannelCustomHeaderValue(value string, apiKey string) string {
	return strings.ReplaceAll(value, channelAPIKeyPlaceholder, apiKey)
}

// isValidCustomHeaderName reports whether name is a conservative HTTP header
// field-name token. Parameters: name is the administrator-provided header key.
// Return value: true when the key is safe to pass to net/http.
func isValidCustomHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			continue
		case r >= 'A' && r <= 'Z':
			continue
		case r >= '0' && r <= '9':
			continue
		case strings.ContainsRune("!#$%&'*+-.^_`|~", r):
			continue
		default:
			return false
		}
	}
	return true
}

func DoRequestHelper(a Adaptor, c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	fullRequestURL, err := a.GetRequestURL(meta)
	if err != nil {
		return nil, errors.Wrap(err, "get request url failed")
	}
	if meta != nil {
		// Honor an administrator-configured per-endpoint upstream URL override.
		// When set, it fully replaces the adaptor-computed URL for this endpoint.
		if override := meta.UpstreamEndpointURLOverride(); override != "" {
			fullRequestURL = override
		}
		meta.UpstreamRequestURL = fullRequestURL
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
			if err != nil && !errors.Is(err, io.EOF) {
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
	if err = applyChannelCustomHeaders(req, meta); err != nil {
		return nil, errors.Wrap(err, "apply channel custom headers")
	}

	// Prepare tagged logger and propagate to context.
	// The request-scoped logger is already bound with the full user/token/channel
	// identity (id + uuid + name) by the auth and distributor middlewares, so only
	// the non-identity fields are added here. "adaptor" is the upstream provider
	// implementation name (e.g. "aws", "zhipu"), which is distinct from the
	// operator-chosen "channel_name" carried by the bound logger.
	lg := gmw.GetLogger(c).With(
		zap.String("url", fullRequestURL),
		zap.String("adaptor", a.GetChannelName()),
		zap.String("model", meta.ActualModelName),
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
		return nil, identity.Tag(
			errors.Wrapf(err, "upstream request failed for channel %s (id: %d)", a.GetChannelName(), meta.ChannelId),
			meta.Identity().Channel)
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
