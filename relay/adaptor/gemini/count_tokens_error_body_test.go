package gemini

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// countTokensErrorBodySecret stands for provider-internal data in a countTokens
// error body that must never reach an API caller (OWASP API8:2023 / API10:2023).
const countTokensErrorBodySecret = "internal-account-123"

// countTokensRoundTripFunc serves an HTTP request with a function.
type countTokensRoundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper for countTokensRoundTripFunc.
//
// Parameters:
//   - req: the outgoing request.
//
// Returns:
//   - *http.Response: the synthetic response.
//   - error: the synthetic transport error, if any.
func (f countTokensRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// countTokensCountingBody serves remaining bytes of the secret and records how
// many bytes were consumed, proving the countTokens read is bounded.
type countTokensCountingBody struct {
	remaining int64
	read      atomic.Int64
}

// Read implements io.Reader for countTokensCountingBody.
//
// Parameters:
//   - p: the destination buffer.
//
// Returns:
//   - int: the number of bytes written to p.
//   - error: io.EOF once the configured size was served.
func (b *countTokensCountingBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), b.remaining)
	for i := range n {
		p[i] = countTokensErrorBodySecret[i%int64(len(countTokensErrorBodySecret))]
	}
	b.remaining -= n
	b.read.Add(n)
	return int(n), nil
}

// Close implements io.Closer for countTokensCountingBody.
//
// Returns:
//   - error: always nil.
func (b *countTokensCountingBody) Close() error { return nil }

// estimateCountTokensFailure runs one multimodal Gemini embeddings estimate
// whose countTokens call receives the given synthetic response. The image part
// requires countTokens, so its failure reaches the caller instead of falling
// back to local text counting.
//
// Parameters:
//   - t: the running test.
//   - status: the countTokens HTTP status.
//   - contentType: the countTokens Content-Type header.
//   - body: the countTokens response body.
//
// Returns:
//   - *relaymodel.ErrorWithStatusCode: the estimate's error; the test fails when it is nil.
func estimateCountTokensFailure(t *testing.T, status int, contentType string, body io.ReadCloser) *relaymodel.ErrorWithStatusCode {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
	previous := client.HTTPClient
	client.HTTPClient = &http.Client{Transport: countTokensRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: body, Request: req}, nil
	})}
	t.Cleanup(func() { client.HTTPClient = previous })

	_, _, bizErr := EstimateEmbeddingPromptUsage(c,
		&meta.Meta{BaseURL: "https://generativelanguage.example.invalid", APIKey: "fixture-key", ActualModelName: "gemini-embedding-2-preview"},
		&relaymodel.GeneralOpenAIRequest{Model: "gemini-embedding-2-preview", Input: map[string]any{"parts": []any{
			map[string]any{"text": "dog"},
			map[string]any{"inline_data": map[string]any{"mime_type": "image/png", "data": "ZmFrZQ=="}},
		}}})
	require.NotNil(t, bizErr)
	return bizErr
}

// TestGeminiCountTokensErrorBodyNeverEchoed requires a countTokens error
// response to reach the caller with the provider status and either the parsed
// Google message or a generic description that never contains body bytes.
func TestGeminiCountTokensErrorBodyNeverEchoed(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body, want string
		status                        int
		code                          any
	}{
		{"unknown_json", "application/json", `{"private":"` + countTokensErrorBodySecret + `"}`,
			"upstream returned HTTP 403 with an unrecognized error body (content-type application/json, %d bytes)", http.StatusForbidden, "count_tokens_failed"},
		{"google_without_message", "application/json", `{"error":{"code":403,"status":"PERMISSION_DENIED"},"debug":"` + countTokensErrorBodySecret + `"}`,
			"upstream returned HTTP 403 with an unrecognized error body (content-type application/json, %d bytes)", http.StatusForbidden, 403},
		{"google_message", "application/json; charset=UTF-8", `{"error":{"code":403,"message":"Permission denied on resource project p.","status":"PERMISSION_DENIED"},"debug":"` + countTokensErrorBodySecret + `"}`,
			"Permission denied on resource project p.", http.StatusForbidden, 403},
		{"fastapi_detail", "application/json", `{"detail":"model is not available in this region","trace":"` + countTokensErrorBodySecret + `"}`,
			"model is not available in this region", http.StatusBadRequest, "count_tokens_failed"},
		{"error_in_success_body", "application/json", `{"error":{"code":500,"status":"INTERNAL"},"debug":"` + countTokensErrorBodySecret + `"}`,
			"upstream returned HTTP 200 with an unrecognized error body (content-type application/json, %d bytes)", http.StatusOK, 500},
		{"html_forbidden", "text/html", "<html>" + countTokensErrorBodySecret + "</html>",
			"upstream returned HTTP 403 with an unrecognized error body (content-type text/html, %d bytes)", http.StatusForbidden, "count_tokens_failed"},
		{"google_string_code", "application/json", `{"error":{"code":"PERMISSION_DENIED","message":"denied"},"debug":"` + countTokensErrorBodySecret + `"}`,
			"denied", http.StatusForbidden, "count_tokens_failed"},
		{"undecodable_success", "application/json", `{"totalTokens":918273645546372819123456789}`,
			"unmarshal_count_tokens_response: countTokens returned an undecodable response", http.StatusOK, "count_tokens_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bizErr := estimateCountTokensFailure(t, tc.status, tc.contentType, io.NopCloser(strings.NewReader(tc.body)))
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
			require.NotContains(t, string(clientJSON), countTokensErrorBodySecret)
			require.NotContains(t, string(clientJSON), "918273645546372819", "a decoder error must not quote body literals to the client")
			require.NotContains(t, bizErr.RawError.Error(), countTokensErrorBodySecret, "diagnostics that reach logs must not carry the body")
		})
	}
}

// TestGeminiCountTokensErrorBodyBoundedRead requires the countTokens read to
// stop one byte past the 1 MiB limit and an over-limit body to be described
// without parsing (OWASP API4:2023 unrestricted resource consumption).
func TestGeminiCountTokensErrorBodyBoundedRead(t *testing.T) {
	for _, tc := range []struct {
		status, wantStatus int
		want               string
	}{
		{http.StatusForbidden, http.StatusForbidden, "upstream returned HTTP 403 with an error body larger than the 1048576-byte limit (content-type text/html)"},
		{http.StatusOK, http.StatusBadGateway, "count_tokens response exceeds the 1048576-byte limit"},
	} {
		body := &countTokensCountingBody{remaining: 8 << 20}
		bizErr := estimateCountTokensFailure(t, tc.status, "text/html", body)
		require.LessOrEqual(t, body.read.Load(), int64(1<<20)+1, "status %d", tc.status)
		require.Equal(t, tc.wantStatus, bizErr.StatusCode)
		require.Contains(t, bizErr.Message, tc.want)
		require.NotContains(t, bizErr.Message, countTokensErrorBodySecret)
		require.NotContains(t, bizErr.RawError.Error(), countTokensErrorBodySecret)
	}
}
