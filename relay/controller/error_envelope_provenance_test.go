package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// chainedOneAPIErrorBody is what a chained one-api behind a channel answers when
// its own key is out of quota. It describes that provider, not this gateway.
const chainedOneAPIErrorBody = `{"error":{"message":"user quota is not enough","type":"one_api_error","code":"insufficient_user_quota"}}`

// TestRelayErrorEnvelopeDecodedProvenance requires every error decoded from a
// provider wire payload to report IsOneAPIError()==false, whether it reaches the
// relay through RelayErrorHandler or through an adaptor that decodes the body
// into model.Error itself (here the OpenAI text handler on a 2xx body). The
// relay must then apply the provider failure policy instead of exempting the
// failure as one-api's own.
func TestRelayErrorEnvelopeDecodedProvenance(t *testing.T) {
	viaHandler := RelayErrorHandler(&http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(chainedOneAPIErrorBody))})
	require.Equal(t, model.ErrorTypeOneAPI, viaHandler.Type, "the client still sees the provider's type")
	require.False(t, viaHandler.IsOneAPIError())

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(chainedOneAPIErrorBody))}
	viaAdaptor, _ := openai.Handler(c, resp, 1, "gpt-4o-mini")
	require.NotNil(t, viaAdaptor)
	require.Equal(t, model.ErrorTypeOneAPI, viaAdaptor.Type)
	require.Equal(t, "insufficient_user_quota", viaAdaptor.Code)
	require.True(t, viaAdaptor.FromUpstream, "an error decoded from provider JSON is provider-sourced")
	require.False(t, viaAdaptor.IsOneAPIError())
}
