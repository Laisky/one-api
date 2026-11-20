package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	rmeta "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// provenanceProviderStatuses are the provider statuses whose attribution drives
// the adaptive ability cooldown: rate limiting and server-side failures.
var provenanceProviderStatuses = []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable}

// provenanceProviderResponse starts a fake provider answering with status and
// body, and returns the real client response to a POST at path.
//
// Parameters:
//   - t: the running test; the server and body are closed on cleanup.
//   - status: the HTTP status the provider answers with.
//   - body: the JSON body the provider writes.
//
// Returns:
//   - *http.Response: the response exactly as the HTTP client received it.
func provenanceProviderResponse(t *testing.T, status int, body string) *http.Response {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := io.WriteString(w, body); err != nil {
			t.Errorf("write provider body: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/v1/videos", strings.NewReader(`{"model":"sora-2"}`))
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

// newProvenanceContext builds a gin context for a POST to path with body.
//
// Parameters:
//   - path: the downstream request path.
//   - body: the downstream request body.
//
// Returns:
//   - *gin.Context: the request context.
//   - *httptest.ResponseRecorder: the recorder capturing client output.
func newProvenanceContext(path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

// requireProviderMarked asserts that failure carries the provider status, is
// provider-attributed, and serializes to exactly expectedJSON.
//
// Parameters:
//   - t: the running test.
//   - failure: the error under test.
//   - status: the provider's HTTP status.
//   - expectedJSON: the byte-exact client-visible JSON.
//
// Returns: none.
func requireProviderMarked(t *testing.T, failure *model.ErrorWithStatusCode, status int, expectedJSON string) {
	t.Helper()
	require.NotNil(t, failure)
	require.Equal(t, status, failure.StatusCode)
	clientJSON, err := json.Marshal(failure)
	require.NoError(t, err)
	require.Equal(t, expectedJSON, string(clientJSON))
	require.True(t, failure.FromUpstream, "provider-status error must be provider-marked")
	require.False(t, failure.IsOneAPIError(), "provider-status error must not be one-api's own")
}

// TestVideoHandlerProviderErrorProvenance verifies that a provider video error
// decoded from the body keeps the provider's status, is attributed to the
// provider, and keeps its client JSON, both directly and through DoResponse.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestVideoHandlerProviderErrorProvenance(t *testing.T) {
	const providerBody = `{"error":{"message":"Rate limit reached for sora-2","type":"rate_limit_error","param":null,"code":"rate_limit_exceeded"}}`
	const expected = `{"message":"Rate limit reached for sora-2","type":"rate_limit_error","param":"","code":"rate_limit_exceeded","status_code":%d}`
	for _, status := range provenanceProviderStatuses {
		t.Run(fmt.Sprintf("handler_%d", status), func(t *testing.T) {
			c, _ := newProvenanceContext("/v1/videos", `{"model":"sora-2"}`)
			failure, usage := VideoHandler(c, provenanceProviderResponse(t, status, providerBody))
			require.Nil(t, usage)
			requireProviderMarked(t, failure, status, fmt.Sprintf(expected, status))
		})
		t.Run(fmt.Sprintf("adaptor_%d", status), func(t *testing.T) {
			c, _ := newProvenanceContext("/v1/videos", `{"model":"sora-2"}`)
			m := &rmeta.Meta{Mode: relaymode.Videos, ChannelType: channeltype.OpenAI, ActualModelName: "sora-2", OriginModelName: "sora-2"}
			usage, failure := (&Adaptor{}).DoResponse(c, provenanceProviderResponse(t, status, providerBody), m)
			require.Nil(t, usage)
			requireProviderMarked(t, failure, status, fmt.Sprintf(expected, status))
		})
	}
}

// provenanceFailingBody is a provider body whose read fails mid-response.
type provenanceFailingBody struct{}

// Read always fails, simulating a connection lost while reading the body.
//
// Parameters:
//   - p: the destination buffer, left untouched.
//
// Returns:
//   - int: always zero.
//   - error: always a read failure.
func (provenanceFailingBody) Read(p []byte) (int, error) {
	return 0, errors.New("provider connection reset")
}

// Close reports success; the read failure is the condition under test.
//
// Returns:
//   - error: always nil.
func (provenanceFailingBody) Close() error { return nil }

// TestVideoHandlerBodyReadFailureStaysLocal verifies that failing to read the
// provider body stays one-api's own 500 error even at a provider 429/503,
// because that error does not carry the provider status.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestVideoHandlerBodyReadFailureStaysLocal(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			c, _ := newProvenanceContext("/v1/videos", `{"model":"sora-2"}`)
			resp := &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: provenanceFailingBody{}}
			failure, usage := VideoHandler(c, resp)
			require.Nil(t, usage)
			require.NotNil(t, failure)
			require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
			require.Equal(t, "read_response_body_failed", failure.Code)
			require.False(t, failure.FromUpstream)
			require.True(t, failure.IsOneAPIError())
		})
	}
}

// TestRealtimeSessionsHandlerProviderStatusProvenance verifies that a provider
// 429/5xx answer to the realtime sessions request is attributed to the provider
// and keeps its raw-passthrough client JSON unchanged.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestRealtimeSessionsHandlerProviderStatusProvenance(t *testing.T) {
	const providerBody = `{"error":{"message":"slow down","type":"rate_limit_error"}}`
	const expected = `{"message":"{\"error\":{\"message\":\"slow down\",\"type\":\"rate_limit_error\"}}","type":"upstream_error","param":"","code":%d,"status_code":%d}`
	for _, status := range provenanceProviderStatuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if _, err := io.WriteString(w, providerBody); err != nil {
					t.Errorf("write provider body: %v", err)
				}
			}))
			t.Cleanup(upstream.Close)

			c, w := newProvenanceContext("/v1/realtime/sessions", `{"model":"gpt-realtime"}`)
			failure, writeErr := RealtimeSessionsHandler(c, &rmeta.Meta{BaseURL: upstream.URL, APIKey: "sk-provenance-fixture"})
			require.NoError(t, writeErr)
			require.Equal(t, status, w.Code)
			require.Equal(t, providerBody, w.Body.String())
			requireProviderMarked(t, failure, status, fmt.Sprintf(expected, status, status))
		})
	}
}

// TestRealtimeSessionsHandlerWithoutProviderStatusStaysUnmarked verifies that a
// transport failure with no provider response, and a failure reading the
// provider body, keep their 502 and are not provider-status marked.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestRealtimeSessionsHandlerWithoutProviderStatusStaysUnmarked(t *testing.T) {
	t.Run("closed_port", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		closedURL := "http://" + listener.Addr().String()
		require.NoError(t, listener.Close())

		c, _ := newProvenanceContext("/v1/realtime/sessions", `{"model":"gpt-realtime"}`)
		failure, callErr := RealtimeSessionsHandler(c, &rmeta.Meta{BaseURL: closedURL, APIKey: "sk-provenance-fixture"})
		require.Error(t, callErr)
		require.NotNil(t, failure)
		require.Equal(t, http.StatusBadGateway, failure.StatusCode)
		require.Equal(t, "upstream_request_failed", failure.Code)
		require.False(t, failure.FromUpstream)
	})
	t.Run("truncated_body_503", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusServiceUnavailable)
			if _, err := io.WriteString(w, `{"err`); err != nil {
				t.Errorf("write truncated body: %v", err)
				return
			}
			w.(http.Flusher).Flush()
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack provider connection: %v", err)
				return
			}
			if err := conn.Close(); err != nil {
				t.Errorf("close provider connection: %v", err)
			}
		}))
		t.Cleanup(upstream.Close)

		c, _ := newProvenanceContext("/v1/realtime/sessions", `{"model":"gpt-realtime"}`)
		failure, callErr := RealtimeSessionsHandler(c, &rmeta.Meta{BaseURL: upstream.URL, APIKey: "sk-provenance-fixture"})
		require.Error(t, callErr)
		require.NotNil(t, failure)
		require.Equal(t, http.StatusBadGateway, failure.StatusCode)
		require.Equal(t, "read_upstream_failed", failure.Code)
		require.False(t, failure.FromUpstream)
	})
}
