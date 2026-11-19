package openai_compatible

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// unifiedStreamProviderResponse starts a fake provider that answers a streaming
// chat request with a non-SSE JSON body, and returns the real client response.
//
// Parameters:
//   - t: the running test; the server and response body are closed on cleanup.
//   - status: the HTTP status the provider answers with.
//   - body: the JSON body the provider writes.
//
// Returns:
//   - *http.Response: the response exactly as the HTTP client received it.
func unifiedStreamProviderResponse(t *testing.T, status int, body string) *http.Response {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := io.WriteString(w, body); err != nil {
			t.Errorf("write provider body: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"stream":true}`))
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("close provider response: %v", closeErr)
		}
	})
	require.Equal(t, status, resp.StatusCode)
	return resp
}

// newUnifiedStreamProvenanceContext builds a gin context with a request-scoped
// logger for driving UnifiedStreamProcessing directly.
//
// Parameters:
//   - t: the running test.
//
// Returns:
//   - *gin.Context: a test context whose writer records the client output.
func newUnifiedStreamProvenanceContext(t *testing.T) *gin.Context {
	t.Helper()
	c, _ := newBridgeTestCtx(t)
	gmw.SetLogger(c, createTestLogger())
	return c
}

// TestUnifiedStreamProcessingProviderJSONErrorProvenance verifies that a JSON
// error a provider returns to a streaming request is attributed to the provider
// when it carries the provider's HTTP status, without changing the client JSON.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestUnifiedStreamProcessingProviderJSONErrorProvenance(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "decoded_provider_error",
			body:     `{"error":{"message":"provider overloaded","type":"server_error","code":"overloaded"}}`,
			expected: `{"message":"provider overloaded","type":"server_error","param":"","code":"overloaded","status_code":%d}`,
		},
		{
			// Another one-api deployment behind this channel reports its own
			// one_api_error type; it still describes the provider, not this gateway.
			name:     "decoded_nested_one_api_error",
			body:     `{"error":{"message":"nested gateway busy","type":"one_api_error","code":"busy"}}`,
			expected: `{"message":"nested gateway busy","type":"one_api_error","param":"","code":"busy","status_code":%d}`,
		},
		{
			name:     "unrecognized_provider_body",
			body:     `{"detail":"rate limited"}`,
			expected: `{"message":"unexpected non-streaming response with 25 bytes","type":"one_api_error","param":"","code":"unexpected_response_format","status_code":%d}`,
		},
	}
	// 200 is the status the relay controllers can actually deliver here (their
	// non-2xx guards run before DoResponse); 429/500/503 pin direct callers.
	statuses := []int{http.StatusOK, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable}
	for _, tc := range cases {
		for _, status := range statuses {
			t.Run(fmt.Sprintf("%s_%d", tc.name, status), func(t *testing.T) {
				c := newUnifiedStreamProvenanceContext(t)
				resp := unifiedStreamProviderResponse(t, status, tc.body)

				failure, usage := UnifiedStreamProcessing(c, resp, testPromptTokens, testModelName, false)
				require.Nil(t, usage)
				require.NotNil(t, failure)
				require.Equal(t, status, failure.StatusCode)
				clientJSON, err := json.Marshal(failure)
				require.NoError(t, err)
				require.Equal(t, fmt.Sprintf(tc.expected, status), string(clientJSON))
				require.True(t, failure.FromUpstream, "provider-status error must be provider-marked")
				require.False(t, failure.IsOneAPIError(), "provider-status error must not be one-api's own")
			})
		}
	}
}

// unifiedStreamFailingBody is a provider body whose read fails mid-response.
type unifiedStreamFailingBody struct{}

// Read always fails, simulating a connection lost while reading the body.
//
// Parameters:
//   - p: the destination buffer, left untouched.
//
// Returns:
//   - int: always zero.
//   - error: always a read failure.
func (unifiedStreamFailingBody) Read(p []byte) (int, error) {
	return 0, errors.New("provider connection reset")
}

// Close reports success; the read failure is the condition under test.
//
// Returns:
//   - error: always nil.
func (unifiedStreamFailingBody) Close() error { return nil }

// TestUnifiedStreamProcessingBodyReadFailureStaysLocal verifies that a failure
// to read the provider's error body stays one-api's own 500 error even when the
// provider status was 429 or 5xx, because the error does not carry that status.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestUnifiedStreamProcessingBodyReadFailureStaysLocal(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			c := newUnifiedStreamProvenanceContext(t)
			resp := &http.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       unifiedStreamFailingBody{},
			}

			failure, usage := UnifiedStreamProcessing(c, resp, testPromptTokens, testModelName, false)
			require.Nil(t, usage)
			require.NotNil(t, failure)
			require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
			require.Equal(t, "read_error_response_failed", failure.Code)
			require.False(t, failure.FromUpstream)
			require.True(t, failure.IsOneAPIError())
		})
	}
}
