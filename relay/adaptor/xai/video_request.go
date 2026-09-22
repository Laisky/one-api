package xai

import (
	"encoding/json"
	"math"
	"mime"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

// PrepareVideoRequest converts duration aliases into the native xAI JSON field,
// updates the billing DTO to match, and returns the billable input image count.
// Unknown provider options remain intact; no input URLs are fetched by one-api.
func (a *Adaptor) PrepareVideoRequest(c *gin.Context, request *model.VideoRequest) (int, error) {
	if err := validateVideoOperation(c); err != nil {
		return 0, err
	}
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/json" {
		return 0, errors.New("xAI video generation requires application/json")
	}
	body, err := common.GetRequestBody(c)
	if err != nil {
		return 0, errors.Wrap(err, "read xAI video request")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, errors.Wrap(err, "decode xAI video request")
	}
	if payload == nil || strings.TrimSpace(request.Model) == "" {
		return 0, errors.New("xAI video request requires a model and JSON object")
	}
	var duration float64
	for _, field := range []string{"duration", "seconds", "duration_seconds"} {
		raw, exists := payload[field]
		if !exists {
			continue
		}
		var value float64
		if err := json.Unmarshal(raw, &value); err != nil {
			return 0, errors.Wrapf(err, "xAI video %s must be a number", field)
		}
		if value < 1 || value > 15 || math.Trunc(value) != value || math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, errors.New("xAI video duration must be an integer from 1 through 15 seconds")
		}
		if duration != 0 && duration != value {
			return 0, errors.New("conflicting video duration, seconds or duration_seconds")
		}
		duration = value
	}
	if duration == 0 {
		return 0, errors.New("specify duration explicitly so the video job can be priced before submission")
	}
	resolution := strings.ToLower(strings.TrimSpace(request.Resolution))
	size := strings.ToLower(strings.TrimSpace(request.Size))
	if size != "" {
		// Only resolution labels are aliases. Pixel dimensions need an explicit
		// aspect-ratio conversion and must not silently change the provider job.
		if resolution != "" && size != resolution {
			return 0, errors.New("conflicting video size and resolution; use resolution")
		}
		resolution = size
	}
	if resolution == "" {
		resolution = "480p"
	}
	if resolution != "480p" && resolution != "720p" && resolution != "1080p" {
		return 0, errors.New("xAI video resolution must be 480p, 720p or 1080p")
	}
	for _, field := range []string{"video", "remix_id", "reference_id"} {
		if _, exists := payload[field]; exists {
			return 0, errors.Errorf("%s is not supported by the xAI video generation endpoint", field)
		}
	}
	if raw, exists := payload["n"]; exists && string(raw) != "1" {
		return 0, errors.New("xAI video generation supports one job per request")
	}
	images, err := countVideoInputImages(payload)
	if err != nil {
		return 0, err
	}
	if images == 0 && strings.TrimSpace(request.Prompt) == "" {
		return 0, errors.New("xAI text-to-video requires a prompt")
	}
	// Re-encode only normalized fields; image/frame/voice references and other
	// native options retain their original JSON representation and semantics.
	payload["duration"], err = json.Marshal(duration)
	if err != nil {
		return 0, errors.Wrap(err, "encode xAI video duration")
	}
	payload["resolution"], err = json.Marshal(resolution)
	if err != nil {
		return 0, errors.Wrap(err, "encode xAI video resolution")
	}
	delete(payload, "seconds")
	delete(payload, "duration_seconds")
	delete(payload, "size")
	body, err = json.Marshal(payload)
	if err != nil {
		return 0, errors.Wrap(err, "encode normalized xAI video request")
	}
	request.Duration, request.Seconds, request.DurationSeconds = &duration, nil, nil
	request.Resolution, request.Size = resolution, ""
	c.Set(ctxkey.KeyRequestBody, body)
	if lg := gmw.GetLogger(c); lg != nil {
		lg.Debug("normalized xAI video billing inputs", zap.Float64("duration_seconds", duration), zap.String("resolution", resolution), zap.Int("input_images", images))
	}
	return images, nil
}

// countVideoInputImages validates image references in a JSON object and returns
// the number of independently billed input images, including pinned frames.
func countVideoInputImages(payload map[string]json.RawMessage) (int, error) {
	var images []json.RawMessage
	if raw, exists := payload["reference_images"]; exists {
		if err := json.Unmarshal(raw, &images); err != nil {
			return 0, errors.Wrap(err, "decode xAI video reference_images")
		}
	}
	for _, field := range []string{"image", "last_frame"} {
		if raw, exists := payload[field]; exists {
			images = append(images, raw)
		}
	}
	for _, raw := range images {
		var image struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(raw, &image); err != nil {
			return 0, errors.Wrap(err, "decode xAI video input image")
		}
		if strings.TrimSpace(image.URL) == "" {
			return 0, errors.New("xAI video input images require a nonempty url")
		}
	}
	return len(images), nil
}

// validateVideoOperation rejects unsupported xAI collection/content operations
// before any HTTP request. It accepts native creation, its gateway alias, and
// GET of one opaque job ID; it returns an error for all other method/path pairs.
func validateVideoOperation(c *gin.Context) error {
	path := c.Request.URL.Path
	if c.Request.Method == http.MethodPost && (path == "/v1/videos" || path == "/v1/videos/generations") {
		return nil
	}
	if c.Request.Method == http.MethodGet && strings.HasPrefix(path, "/v1/videos/") {
		id := strings.TrimPrefix(path, "/v1/videos/")
		if validVideoTaskID(id) && id != "generations" {
			return nil
		}
	}
	return errors.New("xAI supports video creation and task polling, not listing, deletion or /content; use video.url from the completed task")
}

// validVideoTaskID checks that an upstream job ID fits the binding column and
// one URL path segment. It accepts ASCII letters, digits, hyphens and underscores.
func validVideoTaskID(id string) bool {
	if len(id) == 0 || len(id) > 191 {
		return false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
