package muapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/relay/model"
)

// newMuAPITestContext creates an isolated JSON request context for adaptor tests.
func newMuAPITestContext(method string, path string, body string) *gin.Context {
	c, _ := newMuAPITestContextWithRecorder(method, path, body)
	return c
}

// newMuAPITestContextWithRecorder creates a request context and exposes its recorder for response assertions.
func newMuAPITestContextWithRecorder(method string, path string, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, recorder
}

// TestPrepareVideoRequestNormalizesAliases verifies that MuAPI receives a
// model-free body while the gateway billing DTO retains the selected duration.
func TestPrepareVideoRequestNormalizesAliases(t *testing.T) {
	t.Parallel()
	c := newMuAPITestContext(http.MethodPost, "/v1/videos/generations", `{"model":"veo3-fast","prompt":"a lighthouse","seconds":5,"aspect_ratio":"16:9"}`)
	request := &model.VideoRequest{Model: "veo3-fast", Seconds: float64Ptr(5)}

	inputImages, err := (&Adaptor{}).PrepareVideoRequest(c, request)
	require.NoError(t, err)
	require.Zero(t, inputImages)
	require.NotNil(t, request.Duration)
	require.Equal(t, float64(5), *request.Duration)
	require.Nil(t, request.Seconds)

	body, err := common.GetRequestBody(c)
	require.NoError(t, err)
	require.JSONEq(t, `{"prompt":"a lighthouse","duration":5,"aspect_ratio":"16:9"}`, string(body))
}

// TestPrepareVideoRequestRejectsMissingDuration ensures dynamic pricing never
// submits an unpriceable request with a provider-selected implicit duration.
func TestPrepareVideoRequestRejectsMissingDuration(t *testing.T) {
	t.Parallel()
	c := newMuAPITestContext(http.MethodPost, "/v1/videos", `{"model":"veo3-fast","prompt":"a lighthouse"}`)
	_, err := (&Adaptor{}).PrepareVideoRequest(c, &model.VideoRequest{Model: "veo3-fast"})
	require.Error(t, err)
}

// TestPrepareVideoRequestRejectsUnsupportedOperation prevents MuAPI's native
// job adaptor from being used for listing or content-download routes.
func TestPrepareVideoRequestRejectsUnsupportedOperation(t *testing.T) {
	t.Parallel()
	c := newMuAPITestContext(http.MethodGet, "/v1/videos/job-123/content", "")
	_, err := (&Adaptor{}).PrepareVideoRequest(c, &model.VideoRequest{Model: "veo3-fast"})
	require.Error(t, err)
}

// TestPrepareMuAPIVideoBodyRemovesMappedModel verifies that the model remains
// in the URL and is not duplicated in MuAPI's model-specific JSON body.
func TestPrepareMuAPIVideoBodyRemovesMappedModel(t *testing.T) {
	t.Parallel()
	c := newMuAPITestContext(http.MethodPost, "/v1/videos", `{"model":"mapped-model","duration":5}`)
	reader, err := prepareMuAPIVideoBody(c, strings.NewReader(`{"model":"mapped-model","duration":5}`))
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.JSONEq(t, `{"duration":5}`, string(body))
}

// float64Ptr returns a pointer to a test duration value.
func float64Ptr(value float64) *float64 {
	return &value
}
