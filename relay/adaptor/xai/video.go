package xai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

const maxVideoResponseBytes = 4 << 20

// videoRequestPath maps gateway creation to xAI's asynchronous API.
// Parameters: path contains a gateway path and optional query. Returns: the
// native path, or an error for unsupported list/content/delete-style routes.
func videoRequestPath(path string) (string, error) {
	parsed, err := url.ParseRequestURI(path)
	if err != nil {
		return "", errors.Wrap(err, "parse video request path")
	}
	if parsed.Path == "/v1/videos" || parsed.Path == "/v1/videos/generations" {
		parsed.Path = "/v1/videos/generations"
		return parsed.String(), nil
	}
	id := strings.TrimPrefix(parsed.Path, "/v1/videos/")
	if id == parsed.Path || !validVideoTaskID(id) {
		return "", errors.New("xAI supports video creation and GET /v1/videos/{request_id}; download the returned video.url directly")
	}
	return parsed.String(), nil
}

// validVideoTaskID validates an opaque ID before it is used as a path segment.
// Parameters: id is an upstream task ID. Returns: true for bounded ASCII IDs
// without slashes, dot segments, URL escapes, or query delimiters.
func validVideoTaskID(id string) bool {
	if len(id) == 0 || len(id) > 191 {
		return false
	}
	for _, ch := range id {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

// videoOperationError rejects operations not offered by xAI without contacting
// an upstream. Parameters: c identifies the method and route. Returns: a JSON
// HTTP error response for unsupported operations, or nil for create and poll.
func videoOperationError(c *gin.Context) *http.Response {
	path := c.Request.URL.Path
	create := path == "/v1/videos" || path == "/v1/videos/generations"
	poll := strings.HasPrefix(path, "/v1/videos/") && !create && validVideoTaskID(strings.TrimPrefix(path, "/v1/videos/"))
	if create && c.Request.Method == http.MethodPost || poll && c.Request.Method == http.MethodGet {
		return nil
	}
	return &http.Response{
		StatusCode: http.StatusMethodNotAllowed,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"xAI supports video creation and task polling, not list/delete/remix/content. Download the completed response video.url directly."}}`)),
	}
}

// handleVideoResponse validates a bounded JSON response and preserves xAI's
// native polling states and media URL. Parameters: c identifies create versus
// poll and resp is the provider response. Returns: no token usage and an error
// on invalid/failed submission; successful submissions persist an added id alias.
func (a *Adaptor) handleVideoResponse(c *gin.Context, resp *http.Response) (*model.Usage, *model.ErrorWithStatusCode) {
	if resp == nil || resp.Body == nil {
		return nil, openai.ErrorWrapper(errors.New("missing xAI video response"), "invalid_video_response", http.StatusBadGateway)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxVideoResponseBytes+1))
	closeErr := resp.Body.Close()
	if readErr != nil {
		return nil, openai.ErrorWrapper(errors.Wrap(readErr, "read xAI video response"), "invalid_video_response", http.StatusBadGateway)
	}
	if closeErr != nil {
		return nil, openai.ErrorWrapper(errors.Wrap(closeErr, "close xAI video response"), "invalid_video_response", http.StatusBadGateway)
	}
	if len(body) > maxVideoResponseBytes {
		return nil, openai.ErrorWrapper(errors.New("xAI video response exceeds metadata limit"), "invalid_video_response", http.StatusBadGateway)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Never treat an HTTP rejection as an accepted, chargeable render.
		return nil, openai.ErrorWrapper(errors.Errorf("xAI video request rejected with HTTP %d", resp.StatusCode), "upstream_video_error", resp.StatusCode)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return nil, openai.ErrorWrapper(errors.New("xAI video response must be a JSON object"), "invalid_video_response", http.StatusBadGateway)
	}
	if value, ok := root["error"]; ok && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		if c.Request.Method == http.MethodPost {
			return nil, openai.ErrorWrapper(errors.New("xAI rejected video submission"), "upstream_video_error", http.StatusBadGateway)
		}
	}
	if c.Request.Method == http.MethodPost {
		var id string
		if err := json.Unmarshal(root["request_id"], &id); err != nil || !validVideoTaskID(id) {
			return nil, openai.ErrorWrapper(errors.New("xAI video submission did not return a valid request_id"), "invalid_video_response", http.StatusBadGateway)
		}
		root["id"] = root["request_id"]
		var err error
		body, err = json.Marshal(root)
		if err != nil {
			return nil, openai.ErrorWrapper(errors.Wrap(err, "encode video task envelope"), "invalid_video_response", http.StatusBadGateway)
		}
		openai.PersistAsyncVideoTask(c, body)
	}
	// Do not copy stale Content-Length or encoding headers after JSON rewriting.
	c.Header("Content-Type", "application/json")
	for _, key := range []string{"X-Request-Id", "Retry-After"} {
		if value := resp.Header.Get(key); value != "" {
			c.Header(key, value)
		}
	}
	c.Status(resp.StatusCode)
	if _, err := c.Writer.Write(body); err != nil {
		return nil, openai.ErrorWrapper(errors.Wrap(err, "write video response"), "write_video_response_failed", http.StatusInternalServerError)
	}
	return nil, nil
}
