package muapi

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"mime"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

const (
	maxMuAPIModelNameLength = 128
	maxMuAPITaskIDLength    = 191
	maxMuAPIVideoDuration   = 3600
)

// PrepareVideoRequest validates and normalizes a gateway video request for a
// MuAPI model endpoint. Parameters: c carries the JSON request and request is
// the billing DTO. Return value is the number of separately billed input images;
// MuAPI's estimator includes all request inputs, so it returns zero here.
func (a *Adaptor) PrepareVideoRequest(c *gin.Context, request *model.VideoRequest) (int, error) {
	if err := validateMuAPIVideoOperation(c); err != nil {
		return 0, err
	}
	if request == nil || !validMuAPIModelName(strings.TrimSpace(request.Model)) {
		return 0, errors.New("MuAPI video requests require a valid model slug")
	}
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" {
		return 0, errors.New("MuAPI video generation requires application/json")
	}
	body, err := common.GetRequestBody(c)
	if err != nil {
		return 0, errors.Wrap(err, "read MuAPI video request")
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, errors.Wrap(err, "decode MuAPI video request")
	}
	if payload == nil {
		return 0, errors.New("MuAPI video request must be a JSON object")
	}

	duration, err := resolveMuAPIDuration(payload, request)
	if err != nil {
		return 0, err
	}
	payload["duration"], err = json.Marshal(duration)
	if err != nil {
		return 0, errors.Wrap(err, "encode MuAPI video duration")
	}
	delete(payload, "seconds")
	delete(payload, "duration_seconds")
	delete(payload, "model")

	normalized, err := json.Marshal(payload)
	if err != nil {
		return 0, errors.Wrap(err, "encode MuAPI video request")
	}
	request.Duration = &duration
	request.Seconds = nil
	request.DurationSeconds = nil
	c.Set(ctxkey.KeyRequestBody, normalized)
	return 0, nil
}

// resolveMuAPIDuration resolves the accepted duration aliases and rejects
// ambiguous or unsafe values before MuAPI pricing is estimated.
func resolveMuAPIDuration(payload map[string]json.RawMessage, request *model.VideoRequest) (float64, error) {
	var duration float64
	for _, field := range []string{"duration", "seconds", "duration_seconds"} {
		raw, exists := payload[field]
		if !exists {
			continue
		}
		var value float64
		if err := json.Unmarshal(raw, &value); err != nil {
			return 0, errors.Wrapf(err, "MuAPI video %s must be a number", field)
		}
		if value <= 0 || value > maxMuAPIVideoDuration || math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
			return 0, errors.Errorf("MuAPI video duration must be an integer from 1 through %d seconds", maxMuAPIVideoDuration)
		}
		if duration != 0 && duration != value {
			return 0, errors.New("conflicting MuAPI video duration aliases")
		}
		duration = value
	}
	if duration == 0 && request != nil {
		duration = request.RequestedDurationSeconds()
	}
	if duration <= 0 || duration > maxMuAPIVideoDuration || math.Trunc(duration) != duration {
		return 0, errors.New("MuAPI video generation requires a positive integer duration")
	}
	return duration, nil
}

// validateMuAPIVideoOperation permits only creation and opaque-task polling.
// Parameters: c carries the incoming HTTP method and path. Return value is an
// error for listing, content-download, deletion, or other unsupported routes.
func validateMuAPIVideoOperation(c *gin.Context) error {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return errors.New("MuAPI video request context is missing")
	}
	path := stripQuery(c.Request.URL.Path)
	if c.Request.Method == http.MethodPost && (path == "/v1/videos" || path == "/v1/videos/generations") {
		return nil
	}
	if c.Request.Method == http.MethodGet && strings.HasPrefix(path, "/v1/videos/") {
		taskID := strings.TrimPrefix(path, "/v1/videos/")
		if taskID != "generations" && validMuAPITaskID(taskID) {
			return nil
		}
	}
	return errors.New("MuAPI supports video creation and task polling only")
}

// stripMuAPIModelField removes the gateway-only model field before forwarding
// a mapped request to MuAPI's model-in-path endpoint. Return values are the
// sanitized JSON body or a wrapped decoding error.
func stripMuAPIModelField(body []byte) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.Wrap(err, "decode MuAPI video body before dispatch")
	}
	if payload == nil {
		return nil, errors.New("MuAPI video body must be a JSON object")
	}
	delete(payload, "model")
	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "encode MuAPI video body before dispatch")
	}
	return normalized, nil
}

// prepareMuAPIVideoBody removes a model field reintroduced by gateway model
// mapping and resets the reusable Gin body before the shared HTTP helper reads it.
func prepareMuAPIVideoBody(c *gin.Context, requestBody io.Reader) (io.Reader, error) {
	if requestBody == nil {
		return nil, errors.New("MuAPI video request body is missing")
	}
	body, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, errors.Wrap(err, "read MuAPI video body before dispatch")
	}
	normalized, err := stripMuAPIModelField(body)
	if err != nil {
		return nil, err
	}
	c.Set(ctxkey.KeyRequestBody, normalized)
	if c.Request != nil {
		c.Request.Body = io.NopCloser(bytes.NewReader(normalized))
	}
	return bytes.NewReader(normalized), nil
}

// stripQuery returns a URL path without its query component.
func stripQuery(path string) string {
	if index := strings.IndexByte(path, '?'); index >= 0 {
		return path[:index]
	}
	return path
}

// validMuAPIModelName validates a model slug before it becomes a URL path.
func validMuAPIModelName(name string) bool {
	if name == "" || len(name) > maxMuAPIModelNameLength {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

// validMuAPITaskID validates the opaque task id before it becomes a URL path.
func validMuAPITaskID(id string) bool {
	if id == "" || len(id) > maxMuAPITaskIDLength {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

// muAPIRootBaseURL normalizes the configured MuAPI host by removing optional
// /v1 or /api/v1 suffixes so both documented base-URL forms remain usable.
func muAPIRootBaseURL(base string) string {
	root := strings.TrimRight(strings.TrimSpace(base), "/")
	if root == "" {
		root = "https://api.muapi.ai"
	}
	for _, suffix := range []string{"/api/v1", "/v1"} {
		if strings.HasSuffix(strings.ToLower(root), suffix) {
			root = strings.TrimRight(root[:len(root)-len(suffix)], "/")
			break
		}
	}
	return root
}

// muAPICoreBaseURL returns the unified MuAPI submit-and-poll API base.
func muAPICoreBaseURL(base string) string {
	return muAPIRootBaseURL(base) + "/api/v1"
}
