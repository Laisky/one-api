package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestImageResponseRequiresFailureReconciliationAcceptsAll2xx verifies billing
// uses the same HTTP success range as the response handler. In particular, a
// valid 202 response must not be reconciled as an upstream failure.
func TestImageResponseRequiresFailureReconciliationAcceptsAll2xx(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			require.False(t, imageResponseRequiresFailureReconciliation(&http.Response{StatusCode: status}))
		})
	}
	require.True(t, imageResponseRequiresFailureReconciliation(&http.Response{StatusCode: http.StatusBadRequest}))
	require.False(t, imageResponseRequiresFailureReconciliation(nil))
}

// TestGPTImage1MiniPayloadUsesSupportedDefaults verifies the gateway-generated
// request shape independently of upstream availability, ruling out a translation
// regression as the cause of a downstream HTTP 400.
func TestGPTImage1MiniPayloadUsesSupportedDefaults(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/images/generations", bytes.NewBufferString(`{"model":"gpt-image-1-mini","prompt":"a small blue bird"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	imageRequest, err := getImageRequest(ctx, 0)
	require.NoError(t, err)
	config, ok := pricing.ResolveModelConfig("gpt-image-1-mini", nil, &openai.Adaptor{}, time.Now().UTC())
	require.True(t, ok)
	require.NotNil(t, config.Image)
	applyImageDefaults(imageRequest, config.Image)

	payload, err := json.Marshal(buildOpenAIImageRequest(imageRequest))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(payload, &got))
	require.Equal(t, "gpt-image-1-mini", got["model"])
	require.Equal(t, "a small blue bird", got["prompt"])
	require.Equal(t, float64(1), got["n"])
	require.Equal(t, "1024x1536", got["size"])
	require.Equal(t, "high", got["quality"])
	require.NotContains(t, got, "response_format")
}

// TestOpenAIImagePayloadFiltersProviderSpecificFields verifies that fields from
// the shared image DTO cannot leak into a GPT Image request after model mapping.
func TestOpenAIImagePayloadFiltersProviderSpecificFields(t *testing.T) {
	t.Parallel()

	responseFormat := "b64_json"
	request := &relaymodel.ImageRequest{
		Model:          "gpt-image-1-mini",
		Prompt:         "a small blue bird",
		N:              1,
		Size:           "1024x1536",
		Quality:        "high",
		Resolution:     "1k",
		ResponseFormat: &responseFormat,
		Style:          "vivid",
	}

	payload, err := json.Marshal(buildOpenAIImageRequest(request))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(payload, &got))
	require.NotContains(t, got, "resolution")
	require.NotContains(t, got, "response_format")
	require.NotContains(t, got, "style")
}

// TestGetImageRequestPreservesResponseFormatUntilModelMapping verifies that
// parsing does not discard a legacy DALL-E field based on the public alias.
// The caller may map a GPT-looking alias to a DALL-E upstream model later.
func TestGetImageRequestPreservesResponseFormatUntilModelMapping(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/images/generations", bytes.NewBufferString(
		`{"model":"gpt-image-public-alias","prompt":"a small blue bird","response_format":"b64_json"}`,
	))
	ctx.Request.Header.Set("Content-Type", "application/json")

	imageRequest, err := getImageRequest(ctx, 0)
	require.NoError(t, err)
	require.NotNil(t, imageRequest.ResponseFormat,
		"response_format must survive parsing until the mapped upstream model is known")
	require.Equal(t, "b64_json", *imageRequest.ResponseFormat)

	imageRequest.Model = "dall-e-3"
	payload, err := json.Marshal(buildOpenAIImageRequest(imageRequest))
	require.NoError(t, err)
	require.Contains(t, string(payload), `"response_format":"b64_json"`)
}
