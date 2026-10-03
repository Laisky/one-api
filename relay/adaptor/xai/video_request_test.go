package xai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/model"
)

// TestXAIVideoPreparation verifies that forwarded native JSON and the billing
// DTO agree, and that malformed/conflicting inputs fail before admission.
func TestXAIVideoPreparation(t *testing.T) {
	for _, tc := range []struct {
		name, fields, resolution string
		images                   int
		wantErr                  bool
	}{
		{"native", `"duration":5,"resolution":"720p"`, "720p", 0, false},
		{"default", `"duration":5`, "480p", 0, false},
		{"seconds", `"seconds":5`, "480p", 0, false},
		{"duration_seconds", `"duration_seconds":5`, "480p", 0, false},
		{"matching_aliases", `"duration":5,"seconds":5,"duration_seconds":5,"size":"720p","resolution":"720p"`, "720p", 0, false},
		{"image", `"duration":5,"resolution":"1080p","image":{"url":"https://example.com/start.jpg"},"generate_audio":false`, "1080p", 1, false},
		{"frames_and_references", `"duration":5,"image":{"url":"https://example.com/start.jpg"},"last_frame":{"url":"https://example.com/end.jpg"},"reference_images":[{"url":"data:image/png;base64,eA=="}],"reference_audios":[{"voice_id":"eve"}]`, "480p", 3, false},
		{"different_durations", `"duration":15,"seconds":1`, "", 0, true},
		{"different_duration_seconds", `"duration":15,"duration_seconds":1`, "", 0, true},
		{"zero_alias", `"duration":5,"seconds":0`, "", 0, true},
		{"negative", `"duration":-1`, "", 0, true},
		{"too_long", `"duration":16`, "", 0, true},
		{"fractional", `"duration":1.5`, "", 0, true},
		{"null_duration", `"duration":null`, "", 0, true},
		{"missing_duration", `"resolution":"720p"`, "", 0, true},
		{"different_resolution", `"duration":5,"size":"480p","resolution":"1080p"`, "", 0, true},
		{"pixel_size", `"duration":5,"size":"1280x720"`, "", 0, true},
		{"unknown_resolution", `"duration":5,"resolution":"4k"`, "", 0, true},
		{"video_edit", `"duration":5,"video":{"url":"https://example.com/a.mp4"}`, "", 0, true},
		{"batch", `"duration":5,"n":2`, "", 0, true},
		{"empty_image", `"duration":5,"image":{}`, "", 0, true},
		{"null_image", `"duration":5,"last_frame":null`, "", 0, true},
		{"bad_references", `"duration":5,"reference_images":{}`, "", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"model":"grok-imagine-video-1.5","prompt":"A paper boat.","custom_option":{"keep":true},` + tc.fields + `}`
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			gmw.SetLogger(c, logger.Logger)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json; charset=utf-8")
			var request model.VideoRequest
			require.NoError(t, json.Unmarshal([]byte(body), &request))
			count, err := (&Adaptor{}).PrepareVideoRequest(c, &request)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.images, count)
			require.Equal(t, float64(5), request.RequestedDurationSeconds())
			require.Equal(t, tc.resolution, request.RequestedResolution())
			raw, err := common.GetRequestBody(c)
			require.NoError(t, err)
			var forwarded map[string]any
			require.NoError(t, json.Unmarshal(raw, &forwarded))
			require.Equal(t, request.RequestedDurationSeconds(), forwarded["duration"])
			require.Equal(t, tc.resolution, forwarded["resolution"])
			require.Equal(t, map[string]any{"keep": true}, forwarded["custom_option"])
			require.NotContains(t, forwarded, "seconds")
			require.NotContains(t, forwarded, "duration_seconds")
			require.NotContains(t, forwarded, "size")
		})
	}
}

// TestXAIVideoOperations checks that unsupported video operations are rejected
// before invoking the generic HTTP client, while valid task IDs remain opaque.
func TestXAIVideoOperations(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		allowed      bool
	}{
		{"POST", "/v1/videos", true}, {"POST", "/v1/videos/generations", true},
		{"GET", "/v1/videos/job_123-abc", true}, {"GET", "/v1/videos", false},
		{"GET", "/v1/videos/job/content", false}, {"DELETE", "/v1/videos/job", false},
		{"POST", "/v1/videos/edits", false}, {"POST", "/v1/videos/extensions", false},
		{"GET", "/v1/videos/..", false}, {"GET", "/v1/videos/" + strings.Repeat("x", 192), false},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(tc.method, tc.path, nil)
			err := validateVideoOperation(c)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
