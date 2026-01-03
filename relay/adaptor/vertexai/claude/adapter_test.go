package vertexai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func TestAdaptorConvertRequestClearsTopPWhenTemperatureProvided(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	adaptor := Adaptor{}
	temp := 0.6
	topP := 0.5
	req := &model.GeneralOpenAIRequest{
		Model:       "claude-sonnet-4-5@20250929",
		Messages:    []model.Message{{Role: "user", Content: "hello"}},
		Temperature: &temp,
		TopP:        &topP,
	}

	converted, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, req)
	require.NoError(t, err)

	vertexReq, ok := converted.(Request)
	require.True(t, ok)

	require.NotNil(t, vertexReq.Temperature)
	require.Nil(t, vertexReq.TopP)

	stored, exists := c.Get(ctxkey.ConvertedRequest)
	require.True(t, exists)
	storedReq, ok := stored.(Request)
	require.True(t, ok)
	require.Nil(t, storedReq.TopP)
}
