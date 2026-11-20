package zhipu

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// zhipuOversizedPrivateMarker is a provider message hidden in an oversized
// body; it must never be parsed into, or echoed by, the relay error.
const zhipuOversizedPrivateMarker = "private-oversized-provider-message"

// zhipuOversizedEnvelopeBody returns a BigModel error envelope padded with
// trailing whitespace past the video response limit. The bytes the handler
// buffers (limit+1) remain valid JSON, so a handler that parsed the oversized
// body would surface the provider code and the private marker in its error.
//
// Parameters: none.
//
// Returns:
//   - io.ReadCloser: a body whose buffered prefix is a valid error envelope.
func zhipuOversizedEnvelopeBody() io.ReadCloser {
	const limit = 1 << 20
	body := `{"error":{"code":"1302","message":"` + zhipuOversizedPrivateMarker + `"}}` + strings.Repeat(" ", limit)
	return io.NopCloser(strings.NewReader(body))
}

// TestVideoOversizedProviderStatusIsUpstream verifies that a BigModel video HTTP
// failure whose body exceeds the size limit still reports the provider status
// with provider provenance, and that the oversized body is neither parsed nor
// echoed: the error is the generic provider-status error.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestVideoOversizedProviderStatusIsUpstream(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
			t.Run(fmt.Sprintf("%s_%d", method, status), func(t *testing.T) {
				apiErr, accepted := runZhipuVideoProvenance(t, method, status, zhipuOversizedEnvelopeBody())
				require.NotNil(t, apiErr)
				require.False(t, accepted)
				require.Equal(t, status, apiErr.StatusCode)
				encoded, err := json.Marshal(apiErr)
				require.NoError(t, err)
				require.Equal(t, fmt.Sprintf(`{"message":"BigModel video upstream returned HTTP %d","type":"one_api_error","param":"","code":"upstream_video_error","status_code":%d}`, status, status), string(encoded))
				require.NotContains(t, string(encoded), zhipuOversizedPrivateMarker)
				require.True(t, apiErr.FromUpstream, "an oversized provider failure keeps upstream provenance")
				require.False(t, apiErr.IsOneAPIError())
			})
		}
	}
}

// TestVideoOversizedSuccessStaysLocal verifies the control: an oversized 2xx
// BigModel video body, even one shaped like an error envelope, remains
// one-api's own 502 invalid_video_response and is never accepted.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestVideoOversizedSuccessStaysLocal(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		t.Run(method, func(t *testing.T) {
			apiErr, accepted := runZhipuVideoProvenance(t, method, http.StatusOK, zhipuOversizedEnvelopeBody())
			require.NotNil(t, apiErr)
			require.False(t, accepted)
			encoded, err := json.Marshal(apiErr)
			require.NoError(t, err)
			require.Equal(t, `{"message":"video response exceeds limit","type":"one_api_error","param":"","code":"invalid_video_response","status_code":502}`, string(encoded))
			require.False(t, apiErr.FromUpstream)
			require.True(t, apiErr.IsOneAPIError())
		})
	}
}
