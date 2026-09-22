package xai

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
	"github.com/Laisky/one-api/relay/adaptor"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

var _ adaptor.VideoRequestPreparer = (*Adaptor)(nil)

// PrepareVideoRequest validates and synchronizes the video wire and billing
// representations. Parameters: c contains the original body, metadata selects
// the mapped model, and request receives canonical selectors. Returns: a wrapped
// validation error or nil; no network call or quota mutation occurs here.
func (a *Adaptor) PrepareVideoRequest(c *gin.Context, metadata *metalib.Meta, request *model.VideoRequest) error {
	if c == nil || c.Request == nil || metadata == nil || request == nil {
		return errors.New("video request context and metadata are required")
	}
	if c.Request.Method != http.MethodPost || (c.Request.URL.Path != "/v1/videos" && c.Request.URL.Path != "/v1/videos/generations") {
		return errors.New("xAI video creation requires POST /v1/videos or /v1/videos/generations")
	}
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("xAI video requests require application/json; send image references as image.url")
	}
	raw, err := common.GetRequestBody(c)
	if err != nil {
		return errors.Wrap(err, "read xAI video request")
	}
	body, err := normalizeVideoRequest(raw, metadata.ActualModelName, request)
	if err != nil {
		return errors.Wrap(err, "normalize xAI video request")
	}
	c.Set(ctxkey.KeyRequestBody, body)
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.ContentLength = int64(len(body))
	return nil
}

// normalizeVideoRequest translates gateway aliases without losing native JSON
// extensions. Parameters: raw is the input object, modelName is already mapped,
// and request receives billing selectors. Returns: canonical JSON or a wrapped
// error. The explicit gateway defaults are six seconds and 480p, sent upstream.
func normalizeVideoRequest(raw []byte, modelName string, request *model.VideoRequest) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, errors.Wrap(err, "decode video object")
	}
	if root == nil || request == nil || strings.TrimSpace(modelName) == "" {
		return nil, errors.New("video object, request, and model are required")
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, errors.New("video prompt is required")
	}
	for _, key := range []string{"remix_id", "reference_id", "video"} {
		if value, ok := root[key]; ok && string(value) != "null" && string(value) != `""` {
			return nil, errors.New("xAI video generation does not accept remix, reference_id, or video editing requests")
		}
	}
	if value, ok := root["n"]; ok {
		var n int
		if err := json.Unmarshal(value, &n); err != nil || n != 1 {
			return nil, errors.New("xAI generates one video per request; n must be 1")
		}
		delete(root, "n")
	}

	duration := 6.0
	selected := false
	for _, value := range []*float64{request.Duration, request.Seconds, request.DurationSeconds} {
		if value == nil {
			continue
		}
		if math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 1 || *value > 15 || math.Trunc(*value) != *value {
			return nil, errors.New("xAI video duration must be an integer from 1 to 15 seconds")
		}
		if selected && duration != *value {
			return nil, errors.New("video duration selectors disagree")
		}
		duration, selected = *value, true
	}
	resolution := strings.ToLower(strings.TrimSpace(request.Resolution))
	size := strings.ToLower(strings.TrimSpace(request.Size))
	if resolution != "" && size != "" && resolution != size {
		return nil, errors.New("video size and resolution selectors disagree; use resolution 480p, 720p, or 1080p")
	}
	if resolution == "" {
		resolution = size
	}
	if resolution == "" {
		resolution = "480p"
	}
	switch resolution {
	case "480p", "720p":
	case "1080p":
		if strings.HasPrefix(modelName, "grok-imagine-video") && !strings.HasPrefix(modelName, "grok-imagine-video-1.5") {
			return nil, errors.New("1080p requires Grok Imagine Video 1.5")
		}
	default:
		return nil, errors.New("xAI video resolution must be 480p, 720p, or 1080p")
	}

	// Marshal only replacement scalar fields. Native image/reference/audio
	// objects remain RawMessage values, including any large integer extensions.
	for key, value := range map[string]any{"model": modelName, "duration": duration, "resolution": resolution} {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, errors.Wrap(err, "encode canonical video selector")
		}
		root[key] = encoded
	}
	for _, key := range []string{"seconds", "duration_seconds", "size"} {
		delete(root, key)
	}
	body, err := json.Marshal(root)
	if err != nil {
		return nil, errors.Wrap(err, "encode xAI video request")
	}
	request.Model = modelName
	request.Duration = &duration
	request.Seconds = nil
	request.DurationSeconds = nil
	request.Resolution = resolution
	request.Size = ""
	return body, nil
}
