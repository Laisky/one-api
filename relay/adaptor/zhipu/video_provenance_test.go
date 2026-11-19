package zhipu

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// videoProvenanceFailingBody is a response body whose reads fail after the
// provider answered, simulating a connection reset while one-api reads the body.
type videoProvenanceFailingBody struct{}

// Read always fails with io.ErrUnexpectedEOF. It takes the destination buffer and
// returns zero bytes plus the read failure.
func (videoProvenanceFailingBody) Read(_ []byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// Close releases nothing. It returns nil so only the read failure is observed.
func (videoProvenanceFailingBody) Close() error { return nil }

// runZhipuVideoProvenance drives the real adaptor dispatch for one BigModel video
// response. Parameters: t is the test, method selects creation (POST) or polling
// (GET), status is the provider HTTP status and body is the provider payload.
// It returns the relay error and whether the creation was marked accepted.
func runZhipuVideoProvenance(t *testing.T, method string, status int, body io.ReadCloser) (*model.ErrorWithStatusCode, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	gmw.SetLogger(c, logger.Logger)
	c.Request = httptest.NewRequest(method, "/v1/videos", nil)
	resp := &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: body}
	usage, apiErr := (&Adaptor{}).DoResponse(c, resp, &meta.Meta{Mode: relaymode.Videos})
	require.Nil(t, usage)
	return apiErr, c.GetBool(adaptor.AsyncVideoAcceptedKey)
}

// TestVideoProviderStatusErrorIsUpstream verifies that a BigModel video HTTP
// failure without an error envelope keeps its client JSON byte-for-byte but is
// attributed to the provider, so provider 429/5xx reach the adaptive cooldown.
func TestVideoProviderStatusErrorIsUpstream(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
			t.Run(fmt.Sprintf("%s_%d", method, status), func(t *testing.T) {
				apiErr, accepted := runZhipuVideoProvenance(t, method, status, io.NopCloser(strings.NewReader(`upstream unavailable`)))
				require.NotNil(t, apiErr)
				require.False(t, accepted)
				require.Equal(t, status, apiErr.StatusCode)
				encoded, err := json.Marshal(apiErr)
				require.NoError(t, err)
				require.Equal(t, fmt.Sprintf(`{"message":"BigModel video upstream returned HTTP %d","type":"one_api_error","param":"","code":"upstream_video_error","status_code":%d}`, status, status), string(encoded))
				require.True(t, apiErr.FromUpstream, "a provider HTTP status must carry upstream provenance")
				require.False(t, apiErr.IsOneAPIError())
			})
		}
	}
}

// TestVideoProviderErrorEnvelopeIsUpstream verifies that BigModel's own error
// envelope on an HTTP failure, which carries the provider's code and message,
// is attributed to the provider without changing the client JSON.
func TestVideoProviderErrorEnvelopeIsUpstream(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			body := io.NopCloser(strings.NewReader(`{"error":{"code":"1302","message":"rate limited"}}`))
			apiErr, accepted := runZhipuVideoProvenance(t, http.MethodPost, status, body)
			require.NotNil(t, apiErr)
			require.False(t, accepted)
			require.Equal(t, status, apiErr.StatusCode)
			encoded, err := json.Marshal(apiErr)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf(`{"message":"rate limited","type":"zhipu_error","param":"","code":"1302","status_code":%d}`, status), string(encoded))
			require.True(t, apiErr.FromUpstream, "a provider error envelope must carry upstream provenance")
			require.False(t, apiErr.IsOneAPIError())
		})
	}
}

// TestVideoLocalFailuresStayLocal verifies that failures one-api detects itself
// while handling a BigModel video response remain local one_api_error values.
func TestVideoLocalFailuresStayLocal(t *testing.T) {
	for _, tc := range []struct {
		name string
		body io.ReadCloser
		code string
	}{
		{"body_read_failure", videoProvenanceFailingBody{}, "invalid_video_response"},
		{"malformed_success_body", io.NopCloser(strings.NewReader(`<html>proxy error</html>`)), "invalid_video_response"},
		{"unknown_task_status", io.NopCloser(strings.NewReader(`{"id":"vidu_task_1","task_status":"MYSTERY"}`)), "invalid_video_response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apiErr, accepted := runZhipuVideoProvenance(t, http.MethodPost, http.StatusOK, tc.body)
			require.NotNil(t, apiErr)
			require.False(t, accepted)
			require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
			require.Equal(t, tc.code, apiErr.Code)
			require.False(t, apiErr.FromUpstream)
			require.True(t, apiErr.IsOneAPIError())
		})
	}
}
