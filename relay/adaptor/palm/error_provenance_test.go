package palm

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

// TestPaLMErrorProvenance requires the handler's error, whose type PaLM's
// error status supplies, to be provider-marked: a status of "one_api_error"
// must not read as one-api's own error. The relay reaches this branch only for
// a 2xx answer, so the provider status is kept, and the client-visible JSON
// must stay byte-identical to the legacy construction.
func TestPaLMErrorProvenance(t *testing.T) {
	cases := []struct {
		name, body, message, status string
		code                        int
	}{
		{name: "provider_one_api_status", body: `{"error":{"code":403,"message":"quota exhausted","status":"one_api_error"}}`,
			message: "quota exhausted", status: "one_api_error", code: 403},
		{name: "permission_denied", body: `{"error":{"code":403,"message":"API key not valid","status":"PERMISSION_DENIED"}}`,
			message: "API key not valid", status: "PERMISSION_DENIED", code: 403},
		{name: "no_candidates", body: `{"candidates":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}

			got, usage := Handler(c, resp, 5, "chat-bison-001")
			require.Nil(t, usage)
			require.NotNil(t, got)
			require.True(t, got.FromUpstream, "an error built from PaLM's answer must be provider-marked")
			require.False(t, got.IsOneAPIError())
			require.EqualError(t, got.RawError, tc.message)
			require.Empty(t, rec.Body.String())

			legacy := &model.ErrorWithStatusCode{
				Error: model.Error{
					Message:  tc.message,
					Type:     model.ErrorType(tc.status),
					Param:    "",
					Code:     tc.code,
					RawError: errors.New(tc.message),
				},
				StatusCode: http.StatusOK,
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
