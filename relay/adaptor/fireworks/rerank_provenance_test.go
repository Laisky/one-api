package fireworks

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestFireworksRerankErrorProvenance requires every non-200 Fireworks rerank
// answer to become a provider-marked error with the provider's status: a
// provider type of "one_api_error" must not read as one-api's own error, and a
// message-less body falls back to the status text. The client-visible JSON must
// stay byte-identical to the legacy construction.
func TestFireworksRerankErrorProvenance(t *testing.T) {
	cases := []struct {
		name, body, message, errType string
		status                       int
		code                         any
	}{
		{name: "provider_one_api_type_429", status: http.StatusTooManyRequests,
			body:    `{"error":{"type":"one_api_error","message":"rate limit exceeded","code":"rate_limited"}}`,
			message: "rate limit exceeded", errType: "one_api_error", code: "rate_limited"},
		{name: "message_only_400", status: http.StatusBadRequest, body: `{"message":"query is required"}`,
			message: "query is required", errType: "fireworks_rerank_error", code: http.StatusBadRequest},
		{name: "html_502", status: http.StatusBadGateway, body: `<html>bad gateway</html>`,
			message: http.StatusText(http.StatusBadGateway), errType: "fireworks_rerank_error", code: http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/rerank", nil)
			resp := &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}

			got, usage := handleRerankResponse(c, resp, 7)
			require.Nil(t, usage)
			require.NotNil(t, got)
			require.Equal(t, tc.status, got.StatusCode)
			require.True(t, got.FromUpstream, "a Fireworks rerank refusal must be provider-marked")
			require.False(t, got.IsOneAPIError())
			require.EqualError(t, got.RawError, tc.message)
			require.Empty(t, rec.Body.String())

			legacy := &model.ErrorWithStatusCode{
				Error: model.Error{
					Message:  tc.message,
					Type:     model.ErrorType(tc.errType),
					Code:     tc.code,
					RawError: errors.New(tc.message),
				},
				StatusCode: tc.status,
			}
			gotJSON, err := json.Marshal(got)
			require.NoError(t, err)
			wantJSON, err := json.Marshal(legacy)
			require.NoError(t, err)
			require.Equal(t, string(wantJSON), string(gotJSON), "client-visible JSON must stay byte-identical")
			gotError, err := json.Marshal(got.Error)
			require.NoError(t, err)
			wantError, err := json.Marshal(legacy.Error)
			require.NoError(t, err)
			require.Equal(t, string(wantError), string(gotError))
		})
	}
}
