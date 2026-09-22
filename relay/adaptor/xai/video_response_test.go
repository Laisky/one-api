package xai

import (
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
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestXAIVideoResponseContract runs responses through the real adaptor dispatch,
// checking native bytes, terminal statuses and errors without a chat envelope.
func TestXAIVideoResponseContract(t *testing.T) {
	for _, tc := range []struct {
		name, method, body string
		status, wantErr    int
	}{
		{"created", "POST", `{"request_id":"job-123","extra":true}`, 200, 0},
		{"accepted", "POST", `{"request_id":"job-123"}`, 202, 0},
		{"pending", "GET", `{"status":"pending"}`, 200, 0},
		{"done", "GET", `{"status":"done","video":{"url":"https://example.com/a.mp4","respect_moderation":true}}`, 200, 0},
		{"failed", "GET", `{"status":"failed","error":{"message":"generation failed"}}`, 200, 0},
		{"expired", "GET", `{"status":"expired"}`, 200, 0},
		{"provider_error", "POST", `{"error":"forbidden"}`, 403, 403},
		{"rate_limited", "POST", `{"error":{"message":"slow down"}}`, 429, 429},
		{"server_error", "POST", `upstream unavailable`, 503, 503},
		{"missing_id", "POST", `{"id":"not-xai"}`, 200, 502},
		{"unsafe_id", "POST", `{"request_id":"../other"}`, 200, 502},
		{"error_in_200", "POST", `{"request_id":"job-123","error":"failed"}`, 200, 502},
		{"missing_status", "GET", `{"error":"failed"}`, 200, 502},
		{"not_json", "POST", `<html>proxy error</html>`, 200, 502},
		{"oversized", "POST", strings.Repeat("x", maxVideoResponseBytes+1), 200, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			gmw.SetLogger(c, logger.Logger)
			c.Request = httptest.NewRequest(tc.method, "/v1/videos/job-123", nil)
			response := &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{"Content-Type": {"application/json"}, "X-Request-Id": {"trace-123"}, "Content-Length": {"999"}, "Connection": {"keep-alive"}}}
			usage, apiErr := (&Adaptor{}).DoResponse(c, response, &meta.Meta{Mode: relaymode.Videos})
			require.Nil(t, usage)
			if tc.wantErr != 0 {
				require.NotNil(t, apiErr)
				require.Equal(t, tc.wantErr, apiErr.StatusCode)
				require.False(t, c.GetBool(adaptor.AsyncVideoAcceptedKey))
				return
			}
			require.Nil(t, apiErr)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, tc.body, w.Body.String())
			require.Equal(t, "trace-123", w.Header().Get("X-Request-Id"))
			require.Empty(t, w.Header().Get("Content-Length"))
			require.Empty(t, w.Header().Get("Connection"))
			require.Equal(t, tc.method == http.MethodPost, c.GetBool(adaptor.AsyncVideoAcceptedKey))
		})
	}
}
