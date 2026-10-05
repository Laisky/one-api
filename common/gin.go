package common

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"github.com/Laisky/one-api/common/ctxkey"
)

// GetRequestBody reads and caches the request body so it can be reused later in the handler chain.
// It returns the raw body bytes and wraps any I/O error encountered during the read.
func GetRequestBody(c *gin.Context) (requestBody []byte, err error) {
	if requestBodyCache, _ := c.Get(ctxkey.KeyRequestBody); requestBodyCache != nil {
		return requestBodyCache.([]byte), nil
	}
	requestBody, err = io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read request body failed")
	}
	_ = c.Request.Body.Close()
	c.Set(ctxkey.KeyRequestBody, requestBody)

	return requestBody, nil
}

// ErrAmbiguousRequestBody marks a JSON object body labeled as a urlencoded form.
// Such a body can be valid both ways, and the provider receives the client's
// Content-Type, so the gateway refuses to pick one reading for it.
var ErrAmbiguousRequestBody = errors.New("request body is a JSON object labeled application/x-www-form-urlencoded; send it as application/json")

// requestBodyFormat names the reader UnmarshalBodyReusable uses for a request body.
type requestBodyFormat int

const (
	// requestBodyQueryForm keeps gin's default binding, whose form reader also
	// reads the URL query. It is used for GET requests, which relay paths never
	// forward with a metered body, and for POST requests without a body.
	requestBodyQueryForm requestBodyFormat = iota
	// requestBodyJSON decodes the body as a JSON object.
	requestBodyJSON
	// requestBodyMultipart binds multipart/form-data fields from the body only.
	requestBodyMultipart
	// requestBodyPostForm binds urlencoded fields from the body only.
	requestBodyPostForm
	// requestBodyAmbiguous is a JSON object labeled as a urlencoded form.
	requestBodyAmbiguous
)

// classifyRequestBody picks how body is read into a typed request for c.
// Relay paths forward the raw body upstream, so whenever a body exists the typed
// request must come from that body alone: a JSON object is decoded as JSON when
// the Content-Type is JSON in any casing, missing, or names a type no form
// parser reads, and form payloads never read the URL query. It returns the
// chosen format.
func classifyRequestBody(c *gin.Context, body []byte) requestBodyFormat {
	mediaType := requestMediaType(c)
	switch {
	case mediaType == binding.MIMEJSON || strings.HasSuffix(mediaType, "+json"):
		return requestBodyJSON
	case mediaType == binding.MIMEMultipartPOSTForm:
		return requestBodyMultipart
	case c.Request.Method == http.MethodGet:
		return requestBodyQueryForm
	}

	trimmed := bytes.TrimLeft(body, " \t\r\n")
	switch {
	case len(trimmed) == 0:
		return requestBodyQueryForm
	case trimmed[0] != '{':
		return requestBodyPostForm
	case mediaType == binding.MIMEPOSTForm:
		return requestBodyAmbiguous
	default:
		return requestBodyJSON
	}
}

// requestMediaType returns the lower-cased media type of c's Content-Type header
// without parameters, or an empty string when the header is absent.
func requestMediaType(c *gin.Context) string {
	raw := c.Request.Header.Get("Content-Type")
	if mediaType, _, err := mime.ParseMediaType(raw); err == nil {
		return mediaType
	}

	mediaType, _, _ := strings.Cut(raw, ";")
	return strings.ToLower(strings.TrimSpace(mediaType))
}

// IsJSONRequestBody reports whether UnmarshalBodyReusable reads body as JSON for c.
// Paths that rewrite the forwarded body must use this same decision so the typed
// request and the forwarded bytes always come from one reading of the body.
func IsJSONRequestBody(c *gin.Context, body []byte) bool {
	return classifyRequestBody(c, body) == requestBodyJSON
}

// UnmarshalBodyReusable unmarshals the request body into the provided pointer while keeping the body reusable.
// It picks the reader with classifyRequestBody, so typed fields never come from
// the URL query while a body exists, and returns any read, decode or validation
// error, including ErrAmbiguousRequestBody, ErrAmbiguousJSONKey and
// ErrAmbiguousFormKey.
func UnmarshalBodyReusable(c *gin.Context, v any) error {
	requestBody, err := GetRequestBody(c)
	if err != nil {
		return errors.Wrap(err, "get request body failed")
	}

	if err = LogClientRequestPayload(c, "", DefaultLogBodyLimit); err != nil {
		return errors.Wrap(err, "log client request payload failed")
	}

	// check v should be a pointer
	if v == nil || reflect.TypeOf(v).Kind() != reflect.Pointer {
		return errors.Errorf("UnmarshalBodyReusable only accept pointer, got %v", reflect.TypeOf(v))
	}

	switch classifyRequestBody(c, requestBody) {
	case requestBodyJSON:
		err = json.Unmarshal(requestBody, v)
		if err == nil {
			// encoding/json folds key case and keeps the last duplicate, while
			// raw-forwarding paths and upstreams read exact keys. Reject bodies
			// where those two readings of one typed parameter can differ.
			err = validateDecodedJSONRootKeys(requestBody, v)
		}
	case requestBodyMultipart:
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		if err = c.ShouldBindWith(v, binding.FormMultipart); err == nil {
			err = validateParsedFormKeys(c.Request, requestBody)
		}
	case requestBodyPostForm:
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		if err = c.ShouldBindWith(v, binding.FormPost); err == nil {
			err = validateParsedFormKeys(c.Request, requestBody)
		}
	case requestBodyAmbiguous:
		err = ErrAmbiguousRequestBody
	default:
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		err = c.ShouldBind(v)
	}
	if err != nil {
		return errors.Wrap(err, "unmarshal request body failed")
	}

	// Reset request body
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	return nil
}

// LogClientRequestPayload emits shape-only DEBUG metadata for an inbound request
// once per request and restores its body for reuse. Payload content is never
// logged because prompts, reasoning, and tool results may contain secrets.
func LogClientRequestPayload(c *gin.Context, label string, limit int) error {
	if logged, ok := c.Get(ctxkey.ClientRequestPayloadLogged); ok {
		if loggedFlag, ok := logged.(bool); ok && loggedFlag {
			return nil
		}
	}

	body, err := GetRequestBody(c)
	if err != nil {
		return errors.Wrap(err, "get request body failed")
	}

	fields := []zap.Field{
		zap.String("method", c.Request.Method),
		zap.String("url", SanitizeURLForLogging(c.Request.URL.String())),
		zap.Int("body_bytes", len(body)),
		zap.Bool("body_truncated", limit > 0 && len(body) > limit),
		zap.Bool("body_logging_suppressed", true),
	}
	if label != "" {
		fields = append(fields, zap.String("label", label))
	}

	gmw.GetLogger(c).Debug("client request received", fields...)
	c.Set(ctxkey.ClientRequestPayloadLogged, true)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	return nil
}

// SetEventStreamHeaders configures the standard headers required for server-sent event responses.
// It also flushes the headers immediately so reverse proxies (like Cloudflare)
// see the 200 response right away, preventing premature timeout (524) errors.
func SetEventStreamHeaders(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Header().Set("Pragma", "no-cache") // This is for legacy HTTP; I'm pretty sure.
	c.Writer.Flush()                            // Send headers immediately to keep reverse proxies alive.
}
