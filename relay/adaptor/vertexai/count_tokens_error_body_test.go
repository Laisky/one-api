package vertexai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// vertexCountTokensErrorBodySecret stands for provider-internal data in a
// countTokens error body that must never reach an API caller (OWASP API8:2023 /
// API10:2023).
const vertexCountTokensErrorBodySecret = "internal-account-123"

// vertexCountTokensRoundTripFunc serves an HTTP request with a function.
type vertexCountTokensRoundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper for vertexCountTokensRoundTripFunc.
//
// Parameters:
//   - req: the outgoing request.
//
// Returns:
//   - *http.Response: the synthetic response.
//   - error: the synthetic transport error, if any.
func (f vertexCountTokensRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// vertexCountTokensCountingBody serves remaining bytes of the secret and
// records how many bytes were consumed, proving the countTokens read is bounded.
type vertexCountTokensCountingBody struct {
	remaining int64
	read      atomic.Int64
}

// Read implements io.Reader for vertexCountTokensCountingBody.
//
// Parameters:
//   - p: the destination buffer.
//
// Returns:
//   - int: the number of bytes written to p.
//   - error: io.EOF once the configured size was served.
func (b *vertexCountTokensCountingBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), b.remaining)
	for i := range n {
		p[i] = vertexCountTokensErrorBodySecret[i%int64(len(vertexCountTokensErrorBodySecret))]
	}
	b.remaining -= n
	b.read.Add(n)
	return int(n), nil
}

// Close implements io.Closer for vertexCountTokensCountingBody.
//
// Returns:
//   - error: always nil.
func (b *vertexCountTokensCountingBody) Close() error { return nil }

// estimateVertexCountTokensFailure runs one multimodal Vertex AI Gemini
// embeddings estimate whose countTokens call receives the given synthetic
// response. A cached access token stands in for ADC credentials.
//
// Parameters:
//   - t: the running test.
//   - channelID: a channel id unique to the caller, keying the token cache.
//   - status: the countTokens HTTP status.
//   - contentType: the countTokens Content-Type header.
//   - body: the countTokens response body.
//
// Returns:
//   - *relaymodel.ErrorWithStatusCode: the estimate's error; the test fails when it is nil.
func estimateVertexCountTokensFailure(t *testing.T, channelID, status int, contentType string, body io.ReadCloser) *relaymodel.ErrorWithStatusCode {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
	cacheKey := fmt.Sprintf("vertexai-token-%d", channelID)
	Cache.Set(cacheKey, "provider-fixture", time.Minute)
	t.Cleanup(func() { Cache.Delete(cacheKey) })
	previous := client.HTTPClient
	client.HTTPClient = &http.Client{Transport: vertexCountTokensRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: body, Request: req}, nil
	})}
	t.Cleanup(func() { client.HTTPClient = previous })

	m := &meta.Meta{ChannelId: channelID, BaseURL: "https://vertex.example.invalid", ActualModelName: "gemini-embedding-2-preview"}
	m.Config.VertexAIProjectID = "fixture-project"
	_, _, bizErr := EstimateGeminiEmbeddingPromptUsage(c, m,
		&relaymodel.GeneralOpenAIRequest{Model: "gemini-embedding-2-preview", Input: map[string]any{"parts": []any{
			map[string]any{"text": "dog"},
			map[string]any{"inline_data": map[string]any{"mime_type": "image/png", "data": "ZmFrZQ=="}},
		}}})
	require.NotNil(t, bizErr)
	return bizErr
}

// TestVertexCountTokensErrorBodyNeverEchoed requires a Vertex AI countTokens
// error response to reach the caller with the provider status and either the
// parsed Google message or a generic description without body bytes.
func TestVertexCountTokensErrorBodyNeverEchoed(t *testing.T) {
	for i, tc := range []struct {
		name, contentType, body, want string
		status                        int
		code                          any
	}{
		{"unknown_json", "application/json", `{"private":"` + vertexCountTokensErrorBodySecret + `"}`,
			"upstream returned HTTP 403 with an unrecognized error body (content-type application/json, %d bytes)", http.StatusForbidden, "count_tokens_failed"},
		{"google_without_message", "application/json", `{"error":{"code":403,"status":"PERMISSION_DENIED"},"debug":"` + vertexCountTokensErrorBodySecret + `"}`,
			"upstream returned HTTP 403 with an unrecognized error body (content-type application/json, %d bytes)", http.StatusForbidden, 403},
		{"google_message", "application/json; charset=UTF-8", `{"error":{"code":403,"message":"Permission denied on resource project p.","status":"PERMISSION_DENIED"},"debug":"` + vertexCountTokensErrorBodySecret + `"}`,
			"Permission denied on resource project p.", http.StatusForbidden, 403},
		{"error_in_success_body", "application/json", `{"error":{"code":500,"status":"INTERNAL"},"debug":"` + vertexCountTokensErrorBodySecret + `"}`,
			"upstream returned HTTP 200 with an unrecognized error body (content-type application/json, %d bytes)", http.StatusOK, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bizErr := estimateVertexCountTokensFailure(t, 910000+i, tc.status, tc.contentType, io.NopCloser(strings.NewReader(tc.body)))
			want := tc.want
			if strings.Contains(want, "%d") {
				want = fmt.Sprintf(want, len(tc.body))
			}
			wantStatus := tc.status
			if wantStatus < http.StatusBadRequest {
				wantStatus = http.StatusBadGateway
			}
			require.Equal(t, wantStatus, bizErr.StatusCode)
			require.Equal(t, want, bizErr.Message)
			require.Equal(t, tc.code, bizErr.Code)
			clientJSON, err := json.Marshal(bizErr)
			require.NoError(t, err)
			require.NotContains(t, string(clientJSON), vertexCountTokensErrorBodySecret)
			require.NotContains(t, bizErr.RawError.Error(), vertexCountTokensErrorBodySecret, "diagnostics that reach logs must not carry the body")
		})
	}
}

// TestVertexCountTokensErrorBodyBoundedRead requires the Vertex AI countTokens
// read to stop one byte past the 1 MiB limit and an over-limit body to be
// described without parsing (OWASP API4:2023 unrestricted resource consumption).
func TestVertexCountTokensErrorBodyBoundedRead(t *testing.T) {
	for i, tc := range []struct {
		status, wantStatus int
		want               string
	}{
		{http.StatusForbidden, http.StatusForbidden, "upstream returned HTTP 403 with an error body larger than the 1048576-byte limit (content-type text/html)"},
		{http.StatusOK, http.StatusBadGateway, "count_tokens response exceeds the 1048576-byte limit"},
	} {
		body := &vertexCountTokensCountingBody{remaining: 8 << 20}
		bizErr := estimateVertexCountTokensFailure(t, 920000+i, tc.status, "text/html", body)
		require.LessOrEqual(t, body.read.Load(), int64(1<<20)+1, "status %d", tc.status)
		require.Equal(t, tc.wantStatus, bizErr.StatusCode)
		require.Contains(t, bizErr.Message, tc.want)
		require.NotContains(t, bizErr.Message, vertexCountTokensErrorBodySecret)
		require.NotContains(t, bizErr.RawError.Error(), vertexCountTokensErrorBodySecret)
	}
}
