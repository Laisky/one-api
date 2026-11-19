package aiproxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestAIProxyLibraryErrorProvenance requires the handler's errCode error, whose
// type and code AIProxy's errCode supplies, to be provider-marked. The relay
// reaches this branch only for a 2xx answer, so the provider status is kept,
// and the client-visible JSON must stay byte-identical to the legacy
// construction.
func TestAIProxyLibraryErrorProvenance(t *testing.T) {
	cases := []struct {
		name, message string
		errCode       int
	}{
		{name: "library_not_found", message: "library not found", errCode: 1001},
		{name: "empty_message", message: "", errCode: 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			body := `{"success":false,"errCode":` + strconv.Itoa(tc.errCode) + `,"message":"` + tc.message + `"}`
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}

			got, usage := Handler(c, resp)
			require.Nil(t, usage)
			require.NotNil(t, got)
			require.True(t, got.FromUpstream, "an error built from AIProxy's answer must be provider-marked")
			require.False(t, got.IsOneAPIError())
			require.EqualError(t, got.RawError, tc.message)
			require.Empty(t, rec.Body.String())

			legacy := &model.ErrorWithStatusCode{
				Error: model.Error{
					Message:  tc.message,
					Type:     model.ErrorType(strconv.Itoa(tc.errCode)),
					Code:     tc.errCode,
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
