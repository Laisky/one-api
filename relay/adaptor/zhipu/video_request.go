package zhipu

import (
	"encoding/json"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
)

// videoRequestURL maps gateway creation and owner-bound polling to BigModel's
// native endpoints, retaining query parameters and accepting versioned bases.
func videoRequestURL(m *meta.Meta) (string, error) {
	path := m.RequestURLPath
	// Older mode-only adaptor callers use the creation endpoint implicitly.
	if path == "" {
		path = "/v1/videos/generations"
	}
	parsed, err := url.ParseRequestURI(path)
	if err != nil {
		return "", errors.Wrap(err, "parse video path")
	}
	base := strings.TrimRight(m.BaseURL, "/")
	base = strings.TrimSuffix(strings.TrimSuffix(base, "/paas/v4"), "/api") + "/api/paas/v4"
	var endpoint string
	switch parsed.Path {
	case "/v1/videos", "/v1/videos/generations":
		endpoint = "/videos/generations"
	default:
		id, ok := strings.CutPrefix(parsed.Path, "/v1/videos/")
		if !ok || !validVideoID(id) {
			return "", errors.New("unsupported BigModel video operation")
		}
		endpoint = "/async-result/" + url.PathEscape(id)
	}
	if parsed.RawQuery != "" {
		endpoint += "?" + parsed.RawQuery
	}
	return base + endpoint, nil
}

// validVideoID permits opaque native task IDs but excludes path separators and
// control characters so a task lookup cannot escape the async-result endpoint.
func validVideoID(id string) bool {
	if len(id) == 0 || len(id) > 256 || id == "." || id == ".." {
		return false
	}
	for _, ch := range id {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

// PrepareVideoRequest normalizes duration aliases before billing and dispatch.
// Native generation options are preserved; unsupported operations fail before
// any quota reservation. Per-invocation models do not require a duration.
func (a *Adaptor) PrepareVideoRequest(c *gin.Context, request *model.VideoRequest) (int, error) {
	path := c.Request.URL.Path
	if c.Request.Method != http.MethodPost || (path != "/v1/videos" && path != "/v1/videos/generations") {
		return 0, errors.New("BigModel supports video creation and task polling only")
	}
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" {
		return 0, errors.New("BigModel video generation requires application/json")
	}
	body, err := common.GetRequestBody(c)
	if err != nil {
		return 0, errors.Wrap(err, "read video request")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, errors.Wrap(err, "decode video request")
	}
	if payload == nil {
		return 0, errors.New("video request must be an object")
	}
	if request.Model == "cogviewx" || request.Model == "cogviewx-flash" {
		return 0, errors.New("legacy misspelled video model; configure an explicit mapping to a provider model")
	}
	var duration float64
	for _, key := range []string{"duration", "seconds", "duration_seconds"} {
		if raw, ok := payload[key]; ok {
			var value float64
			if err := json.Unmarshal(raw, &value); err != nil {
				return 0, errors.Wrapf(err, "parse %s", key)
			}
			if value <= 0 || math.Trunc(value) != value || math.IsInf(value, 0) {
				return 0, errors.New("video duration must be a positive integer")
			}
			if duration != 0 && duration != value {
				return 0, errors.New("conflicting duration aliases")
			}
			duration = value
		}
	}
	if strings.HasPrefix(request.Model, "cogvideox") && duration != 0 && duration != 5 && duration != 10 {
		return 0, errors.New("CogVideoX duration must be 5 or 10 seconds")
	}
	if strings.HasPrefix(request.Model, "viduq1-") && duration != 0 && duration != 5 {
		return 0, errors.New("Vidu Q1 duration is fixed at 5 seconds")
	}
	if strings.HasPrefix(request.Model, "vidu2-") && duration != 0 && duration != 4 {
		return 0, errors.New("Vidu 2 duration is fixed at 4 seconds")
	}
	if raw, ok := payload["n"]; ok {
		var count int
		if err := json.Unmarshal(raw, &count); err != nil || count != 1 {
			return 0, errors.New("one video is generated per invocation; n must be 1")
		}
		delete(payload, "n")
	}
	// Extensions can change paid work and cannot be silently ignored by a raw
	// proxy. Send supported provider options at the top level instead.
	if _, ok := payload["extra_body"]; ok {
		return 0, errors.New("send native video options at the top level, not extra_body")
	}
	for _, key := range []string{"input_reference", "remix_id", "reference_id"} {
		if _, ok := payload[key]; ok {
			return 0, errors.Errorf("unsupported video field %s; use native image_url", key)
		}
	}
	delete(payload, "seconds")
	delete(payload, "duration_seconds")
	if duration != 0 {
		encoded, err := json.Marshal(duration)
		if err != nil {
			return 0, errors.Wrap(err, "encode video duration")
		}
		payload["duration"] = encoded
		request.Duration = &duration
	}
	request.Seconds, request.DurationSeconds = nil, nil
	encoded, err := json.Marshal(request.Model)
	if err != nil {
		return 0, errors.Wrap(err, "encode video model")
	}
	payload["model"] = encoded
	body, err = json.Marshal(payload)
	if err != nil {
		return 0, errors.Wrap(err, "encode video request")
	}
	c.Set(ctxkey.KeyRequestBody, body)
	return 0, nil
}
