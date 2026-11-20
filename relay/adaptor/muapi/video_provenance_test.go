package muapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// muAPIVideoProvenanceFailingBody is a response body whose reads fail after the
// provider answered, simulating a connection reset while one-api reads the body.
type muAPIVideoProvenanceFailingBody struct{}

// Read always fails with io.ErrUnexpectedEOF. It takes the destination buffer and
// returns zero bytes plus the read failure.
func (muAPIVideoProvenanceFailingBody) Read(_ []byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// Close releases nothing. It returns nil so only the read failure is observed.
func (muAPIVideoProvenanceFailingBody) Close() error { return nil }

// runMuAPIVideoProvenance drives the real adaptor dispatch for one MuAPI video
// response. Parameters: t is the test, mode is the async video relay mode, method
// selects creation (POST) or polling (GET), and resp is the provider response.
// It returns the relay error and whether the creation was marked accepted.
func runMuAPIVideoProvenance(t *testing.T, mode int, method string, resp *http.Response) (*model.ErrorWithStatusCode, bool) {
	t.Helper()
	path := "/v1/async/videos"
	if method == http.MethodGet {
		path += "/job-123"
	}
	c, _ := newMuAPITestContextWithRecorder(method, path, "")
	usage, apiErr := (&Adaptor{}).DoResponse(c, resp, &meta.Meta{Mode: mode})
	require.Nil(t, usage)
	return apiErr, c.GetBool(adaptor.AsyncVideoAcceptedKey)
}

// TestMuAPIVideoProviderStatusErrorIsUpstream verifies that a MuAPI video HTTP
// failure keeps its client JSON byte-for-byte but is attributed to the
// provider, so provider 429/5xx reach the adaptive ability cooldown.
func TestMuAPIVideoProviderStatusErrorIsUpstream(t *testing.T) {
	for _, mode := range []int{relaymode.AsyncVideos, relaymode.MuAsyncVideos} {
		for _, method := range []string{http.MethodPost, http.MethodGet} {
			for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
				t.Run(fmt.Sprintf("%s_%s_%d", relaymode.String(mode), method, status), func(t *testing.T) {
					resp := &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}},
						Body: io.NopCloser(strings.NewReader(`{"detail":"upstream unavailable"}`))}
					apiErr, accepted := runMuAPIVideoProvenance(t, mode, method, resp)
					require.NotNil(t, apiErr)
					require.False(t, accepted)
					require.Equal(t, status, apiErr.StatusCode)
					encoded, err := json.Marshal(apiErr)
					require.NoError(t, err)
					require.Equal(t, fmt.Sprintf(`{"message":"MuAPI video request returned HTTP %d","type":"one_api_error","param":"","code":"upstream_video_error","status_code":%d}`, status, status), string(encoded))
					require.True(t, apiErr.FromUpstream, "a provider HTTP status must carry upstream provenance")
					require.False(t, apiErr.IsOneAPIError())
				})
			}
		}
	}
}

// TestMuAPIVideoLocalFailuresStayLocal verifies that failures one-api detects
// itself while handling a MuAPI video response remain local one_api_error values.
func TestMuAPIVideoLocalFailuresStayLocal(t *testing.T) {
	for _, tc := range []struct {
		name string
		resp *http.Response
	}{
		{"empty_response", &http.Response{StatusCode: http.StatusOK}},
		{"body_read_failure", &http.Response{StatusCode: http.StatusOK, Body: muAPIVideoProvenanceFailingBody{}}},
		{"malformed_success_body", &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`<html>proxy error</html>`))}},
		{"oversized_success_body", &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", maxMuAPIVideoResponseBytes+1)))}},
		{"missing_request_id", &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(`{"status":"processing"}`))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apiErr, accepted := runMuAPIVideoProvenance(t, relaymode.MuAsyncVideos, http.MethodPost, tc.resp)
			require.NotNil(t, apiErr)
			require.False(t, accepted)
			require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
			require.Equal(t, "invalid_video_response", apiErr.Code)
			require.False(t, apiErr.FromUpstream)
			require.True(t, apiErr.IsOneAPIError())
		})
	}
}
