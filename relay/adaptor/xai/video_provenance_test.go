package xai

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

// xaiVideoProvenanceFailingBody is a response body whose reads fail after the
// provider answered, simulating a connection reset while one-api reads the body.
type xaiVideoProvenanceFailingBody struct{}

// Read always fails with io.ErrUnexpectedEOF. It takes the destination buffer and
// returns zero bytes plus the read failure.
func (xaiVideoProvenanceFailingBody) Read(_ []byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// Close releases nothing. It returns nil so only the read failure is observed.
func (xaiVideoProvenanceFailingBody) Close() error { return nil }

// runXAIVideoProvenance drives the real adaptor dispatch for one xAI video
// response. Parameters: t is the test, method selects creation (POST) or polling
// (GET), status is the provider HTTP status and body is the provider payload.
// It returns the relay error and whether the creation was marked accepted.
func runXAIVideoProvenance(t *testing.T, method string, status int, body io.ReadCloser) (*model.ErrorWithStatusCode, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	gmw.SetLogger(c, logger.Logger)
	c.Request = httptest.NewRequest(method, "/v1/videos/job-123", nil)
	resp := &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: body}
	usage, apiErr := (&Adaptor{}).DoResponse(c, resp, &meta.Meta{Mode: relaymode.Videos})
	require.Nil(t, usage)
	return apiErr, c.GetBool(adaptor.AsyncVideoAcceptedKey)
}

// TestXAIVideoProviderStatusErrorIsUpstream verifies that an xAI video HTTP
// failure keeps its client JSON byte-for-byte but is attributed to the
// provider, so provider 429/5xx reach the adaptive ability cooldown.
func TestXAIVideoProviderStatusErrorIsUpstream(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
			t.Run(fmt.Sprintf("%s_%d", method, status), func(t *testing.T) {
				body := io.NopCloser(strings.NewReader(`{"error":{"message":"slow down"}}`))
				apiErr, accepted := runXAIVideoProvenance(t, method, status, body)
				require.NotNil(t, apiErr)
				require.False(t, accepted)
				require.Equal(t, status, apiErr.StatusCode)
				encoded, err := json.Marshal(apiErr)
				require.NoError(t, err)
				require.Equal(t, fmt.Sprintf(`{"message":"xAI video request returned HTTP %d","type":"one_api_error","param":"","code":"upstream_video_error","status_code":%d}`, status, status), string(encoded))
				require.True(t, apiErr.FromUpstream, "a provider HTTP status must carry upstream provenance")
				require.False(t, apiErr.IsOneAPIError())
			})
		}
	}
}

// TestXAIVideoLocalFailuresStayLocal verifies that failures one-api detects
// itself while handling an xAI video response remain local one_api_error values.
func TestXAIVideoLocalFailuresStayLocal(t *testing.T) {
	for _, tc := range []struct {
		name string
		body io.ReadCloser
	}{
		{"body_read_failure", xaiVideoProvenanceFailingBody{}},
		{"malformed_success_body", io.NopCloser(strings.NewReader(`<html>proxy error</html>`))},
		{"oversized_success_body", io.NopCloser(strings.NewReader(strings.Repeat("x", maxVideoResponseBytes+1)))},
		{"missing_request_id", io.NopCloser(strings.NewReader(`{"id":"not-xai"}`))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apiErr, accepted := runXAIVideoProvenance(t, http.MethodPost, http.StatusOK, tc.body)
			require.NotNil(t, apiErr)
			require.False(t, accepted)
			require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
			require.Equal(t, "invalid_video_response", apiErr.Code)
			require.False(t, apiErr.FromUpstream)
			require.True(t, apiErr.IsOneAPIError())
		})
	}
}
