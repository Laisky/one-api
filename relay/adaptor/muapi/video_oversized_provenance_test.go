package muapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/relaymode"
)

// muAPIOversizedPrivateMarker is a provider detail hidden in an oversized body;
// it must never be echoed by the relay error.
const muAPIOversizedPrivateMarker = "private-oversized-provider-detail"

// muAPIOversizedResponse returns a provider response whose JSON body, padded
// with trailing whitespace, exceeds the MuAPI video size limit while its
// buffered prefix stays valid JSON.
//
// Parameters:
//   - status: the provider HTTP status.
//
// Returns:
//   - *http.Response: the provider response with an oversized body.
func muAPIOversizedResponse(status int) *http.Response {
	body := `{"detail":"` + muAPIOversizedPrivateMarker + `"}` + strings.Repeat(" ", maxMuAPIVideoResponseBytes)
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(body))}
}

// TestMuAPIVideoOversizedProviderStatusIsUpstream verifies that a MuAPI video
// HTTP failure whose body exceeds the size limit still reports the provider
// status with provider provenance, without echoing the oversized body.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestMuAPIVideoOversizedProviderStatusIsUpstream(t *testing.T) {
	for _, mode := range []int{relaymode.AsyncVideos, relaymode.MuAsyncVideos} {
		for _, method := range []string{http.MethodPost, http.MethodGet} {
			for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
				t.Run(fmt.Sprintf("%s_%s_%d", relaymode.String(mode), method, status), func(t *testing.T) {
					apiErr, accepted := runMuAPIVideoProvenance(t, mode, method, muAPIOversizedResponse(status))
					require.NotNil(t, apiErr)
					require.False(t, accepted)
					require.Equal(t, status, apiErr.StatusCode)
					encoded, err := json.Marshal(apiErr)
					require.NoError(t, err)
					require.Equal(t, fmt.Sprintf(`{"message":"MuAPI video request returned HTTP %d","type":"one_api_error","param":"","code":"upstream_video_error","status_code":%d}`, status, status), string(encoded))
					require.NotContains(t, string(encoded), muAPIOversizedPrivateMarker)
					require.True(t, apiErr.FromUpstream, "an oversized provider failure keeps upstream provenance")
					require.False(t, apiErr.IsOneAPIError())
				})
			}
		}
	}
}

// TestMuAPIVideoOversizedSuccessStaysLocal verifies the control: an oversized
// 2xx MuAPI video body remains one-api's own 502 invalid_video_response and is
// never accepted.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestMuAPIVideoOversizedSuccessStaysLocal(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		t.Run(method, func(t *testing.T) {
			apiErr, accepted := runMuAPIVideoProvenance(t, relaymode.MuAsyncVideos, method, muAPIOversizedResponse(http.StatusOK))
			require.NotNil(t, apiErr)
			require.False(t, accepted)
			encoded, err := json.Marshal(apiErr)
			require.NoError(t, err)
			require.Equal(t, `{"message":"MuAPI video response exceeds JSON size limit","type":"one_api_error","param":"","code":"invalid_video_response","status_code":502}`, string(encoded))
			require.False(t, apiErr.FromUpstream)
			require.True(t, apiErr.IsOneAPIError())
		})
	}
}
