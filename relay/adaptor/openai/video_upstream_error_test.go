package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newVideoUpstreamErrorContext builds a gin context for a downstream request.
//
// Parameters:
//   - method: the downstream HTTP method.
//   - path: the downstream request path.
//
// Returns:
//   - *gin.Context: the request context.
//   - *httptest.ResponseRecorder: the recorder capturing client output.
func newVideoUpstreamErrorContext(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(`{"model":"sora-2","seconds":4}`))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

// videoUpstreamResponse builds a provider response with status, content type and body.
//
// Parameters:
//   - status: the provider HTTP status.
//   - contentType: the provider Content-Type header.
//   - body: the raw provider body.
//
// Returns:
//   - *http.Response: the provider response.
func videoUpstreamResponse(status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}, "X-Provider-Header": []string{"provider-only"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
	}
}

// TestVideoHandlerUntypedNon2xxBecomesProviderError verifies that every
// non-2xx video body without a typed nested error is withheld from the client
// and returned as a provider-attributed error with the provider status, a
// bounded provider message or a generic one, and an identifier-shaped code.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestVideoHandlerUntypedNon2xxBecomesProviderError(t *testing.T) {
	longMessage := strings.Repeat("é", 700) // 1400 bytes, two bytes per rune
	for _, tc := range []struct {
		name, method, contentType, body string
		status                          int
		wantMessage, wantCode           string
		forbidden                       []string
	}{
		{name: "flat_625_envelope", status: 429, contentType: "application/json",
			body:        `{"message":"You have exceeded the monthly usage quota","type":"TooManyRequests","param":"quota","code":"AccountQuotaExceeded"}`,
			wantMessage: "You have exceeded the monthly usage quota", wantCode: "AccountQuotaExceeded"},
		{name: "untyped_nested", status: 503, contentType: "application/json",
			body: `{"error":{"message":"overloaded"}}`, wantMessage: "overloaded", wantCode: "upstream_video_error"},
		{name: "nested_code_wins_over_flat", status: 502, contentType: "application/json",
			body:        `{"error":{"message":"  nested  ","code":"nested.code-1"},"message":"flat","code":"flat_code"}`,
			wantMessage: "nested", wantCode: "nested.code-1"},
		{name: "flat_fallback_when_nested_blank", status: 500, contentType: "application/json",
			body:        `{"error":{"message":"   ","code":7},"message":"flat message","code":"flat_code"}`,
			wantMessage: "flat message", wantCode: "flat_code"},
		{name: "numeric_code_rejected", status: 429, contentType: "application/json",
			body: `{"message":"slow down","code":1302}`, wantMessage: "slow down", wantCode: "upstream_video_error"},
		{name: "code_with_spaces_rejected", status: 400, contentType: "application/json",
			body:        `{"error":{"message":"bad","code":"has space <b>"}}`,
			wantMessage: "bad", wantCode: "upstream_video_error", forbidden: []string{"<b>"}},
		{name: "overlong_code_rejected", status: 400, contentType: "application/json",
			body:        `{"message":"bad","code":"` + strings.Repeat("a", 129) + `"}`,
			wantMessage: "bad", wantCode: "upstream_video_error"},
		{name: "string_error_field", status: 403, contentType: "application/json",
			body: `{"error":"provider refused RAW-STRING"}`, wantMessage: "OpenAI video upstream returned HTTP 403", wantCode: "upstream_video_error",
			forbidden: []string{"RAW-STRING"}},
		{name: "non_string_message", status: 500, contentType: "application/json",
			body: `{"message":{"nested":"RAW-OBJECT"}}`, wantMessage: "OpenAI video upstream returned HTTP 500", wantCode: "upstream_video_error",
			forbidden: []string{"RAW-OBJECT"}},
		{name: "html", status: 500, contentType: "text/html",
			body:        `<html><body>Internal Server Error RAW-HTML</body></html>`,
			wantMessage: "OpenAI video upstream returned HTTP 500", wantCode: "upstream_video_error", forbidden: []string{"<html", "RAW-HTML"}},
		{name: "json_array", status: 502, contentType: "application/json",
			body: `["RAW-ARRAY"]`, wantMessage: "OpenAI video upstream returned HTTP 502", wantCode: "upstream_video_error", forbidden: []string{"RAW-ARRAY"}},
		{name: "empty", status: 503, contentType: "application/json",
			wantMessage: "OpenAI video upstream returned HTTP 503", wantCode: "upstream_video_error"},
		{name: "get_content_404", method: http.MethodGet, status: 404, contentType: "application/json",
			body: `{"error":{"message":"video not found"}}`, wantMessage: "video not found", wantCode: "upstream_video_error"},
		{name: "long_message_bounded", status: 500, contentType: "application/json",
			body: `{"message":"` + longMessage + `"}`, wantMessage: strings.Repeat("é", 512), wantCode: "upstream_video_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			method := tc.method
			if method == "" {
				method = http.MethodPost
			}
			c, w := newVideoUpstreamErrorContext(method, "/v1/videos")
			failure, usage := VideoHandler(c, videoUpstreamResponse(tc.status, tc.contentType, tc.body))
			require.Nil(t, usage)
			require.NotNil(t, failure)
			require.Equal(t, tc.status, failure.StatusCode)
			require.Equal(t, tc.wantMessage, failure.Message)
			require.Equal(t, tc.wantCode, failure.Code)
			require.True(t, failure.FromUpstream)
			require.False(t, failure.IsOneAPIError())
			require.LessOrEqual(t, len(failure.Message), videoUpstreamErrorMessageLimit)
			require.True(t, utf8.ValidString(failure.Message))
			clientJSON, err := json.Marshal(failure)
			require.NoError(t, err)
			for _, raw := range tc.forbidden {
				require.NotContains(t, string(clientJSON), raw)
			}
			require.False(t, c.Writer.Written(), "a failed provider body must not reach the client")
			require.Empty(t, w.Body.String())
			require.Empty(t, w.Header().Get("X-Provider-Header"))
		})
	}
}

// TestVideoHandlerTypedErrorAndSuccessUnchanged pins the paths the untyped
// non-2xx fix must not touch: a typed nested provider error keeps its exact
// decoded JSON, and a 2xx receipt is forwarded byte-identically with headers.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestVideoHandlerTypedErrorAndSuccessUnchanged(t *testing.T) {
	t.Run("typed_nested_error", func(t *testing.T) {
		c, w := newVideoUpstreamErrorContext(http.MethodPost, "/v1/videos")
		failure, usage := VideoHandler(c, videoUpstreamResponse(400, "application/json",
			`{"error":{"message":"m","type":"invalid_request_error","code":"c"}}`))
		require.Nil(t, usage)
		require.NotNil(t, failure)
		clientJSON, err := json.Marshal(failure)
		require.NoError(t, err)
		require.Equal(t, `{"message":"m","type":"invalid_request_error","param":"","code":"c","status_code":400}`, string(clientJSON))
		require.False(t, c.Writer.Written())
		require.Empty(t, w.Body.String())
	})
	t.Run("accepted_receipt", func(t *testing.T) {
		const receipt = `{"id":"video_ok","object":"video","status":"queued"}`
		c, w := newVideoUpstreamErrorContext(http.MethodGet, "/v1/videos/video_ok")
		failure, usage := VideoHandler(c, videoUpstreamResponse(http.StatusOK, "application/json", receipt))
		require.Nil(t, usage)
		require.Nil(t, failure)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, receipt, w.Body.String())
		require.Equal(t, "provider-only", w.Header().Get("X-Provider-Header"))
	})
}
