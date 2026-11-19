package cohere

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

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// provenanceResponse builds a Cohere response with the given status and body.
//
// Parameters:
//   - status: the provider HTTP status.
//   - body: the response body.
//
// Returns:
//   - *http.Response: the response.
func provenanceResponse(status int, body string) *http.Response {
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

// newProvenanceGinContext builds a request context with a response recorder.
//
// Returns:
//   - *gin.Context: the request context.
func newProvenanceGinContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/rerank", nil)
	return c
}

// requireProvenanceJSON asserts got carries the provider mark and that its
// client-visible JSON (the whole error and the bare "error" member) is
// byte-identical to the legacy construction.
//
// Parameters:
//   - t: the test.
//   - got: the error under test.
//   - legacy: the error as it was built before provenance was recorded.
func requireProvenanceJSON(t *testing.T, got, legacy *model.ErrorWithStatusCode) {
	t.Helper()
	require.NotNil(t, got)
	require.True(t, got.FromUpstream, "an error built from the provider's answer must be provider-marked")
	require.False(t, got.IsOneAPIError(), "a provider error must never read as one-api's own error")
	require.EqualError(t, got.RawError, legacy.Message)
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
}

// TestCohereChatErrorProvenance requires the chat handler's "no response_id"
// error, whose type Cohere's message supplies, to be provider-marked: a message
// of "one_api_error" must not read as one-api's own error. The relay reaches
// this branch only for a 2xx answer, so the provider status (200) is kept.
func TestCohereChatErrorProvenance(t *testing.T) {
	for _, message := range []string{"one_api_error", "invalid api token"} {
		t.Run(message, func(t *testing.T) {
			got, usage := Handler(newProvenanceGinContext(), provenanceResponse(http.StatusOK, `{"message":"`+message+`"}`), 0, "command-r")
			require.Nil(t, usage)
			legacy := &model.ErrorWithStatusCode{
				Error: model.Error{
					Message:  message,
					Type:     model.ErrorType(message),
					Param:    "",
					Code:     http.StatusOK,
					RawError: errors.New(message),
				},
				StatusCode: http.StatusOK,
			}
			requireProvenanceJSON(t, got, legacy)
		})
	}
}

// TestCohereRerankErrorProvenance requires the rerank and embedding error
// builder to provider-mark every answer it describes: a non-2xx rerank answer
// (including a provider type of "one_api_error"), a message-less one that falls
// back to the status text, and a 2xx embedding answer that carries an error
// message (reported as 502).
func TestCohereRerankErrorProvenance(t *testing.T) {
	cases := []struct {
		name, body, message, errType string
		status                       int
		code                         any
	}{
		{name: "provider_one_api_type_429", status: http.StatusTooManyRequests,
			body:    `{"error":{"type":"one_api_error","message":"too many requests","code":"rate_limited"}}`,
			message: "too many requests", errType: "one_api_error", code: "rate_limited"},
		{name: "message_only_400", status: http.StatusBadRequest,
			body:    `{"message":"invalid request: documents is required"}`,
			message: "invalid request: documents is required", errType: "cohere_rerank_error", code: http.StatusBadRequest},
		{name: "empty_body_503", status: http.StatusServiceUnavailable, body: ``,
			message: http.StatusText(http.StatusServiceUnavailable), errType: "cohere_rerank_error", code: http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, usage := RerankHandler(newProvenanceGinContext(), provenanceResponse(tc.status, tc.body), &meta.Meta{PromptTokens: 3})
			require.Nil(t, usage, "a rejected rerank carries no usage evidence")
			require.Equal(t, tc.status, got.StatusCode)
			legacy := &model.ErrorWithStatusCode{
				Error: model.Error{
					Message:  tc.message,
					Type:     model.ErrorType(tc.errType),
					Code:     tc.code,
					RawError: errors.New(tc.message),
				},
				StatusCode: tc.status,
			}
			requireProvenanceJSON(t, got, legacy)
		})
	}

	t.Run("embedding_2xx_message_502", func(t *testing.T) {
		usage, got := embedResponse(newProvenanceGinContext(), provenanceResponse(http.StatusOK, `{"message":"embedding failed"}`), &meta.Meta{})
		require.Nil(t, usage)
		legacy := &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  "embedding failed",
				Type:     model.ErrorType("cohere_rerank_error"),
				Code:     http.StatusBadGateway,
				RawError: errors.New("embedding failed"),
			},
			StatusCode: http.StatusBadGateway,
		}
		requireProvenanceJSON(t, got, legacy)
	})
}
