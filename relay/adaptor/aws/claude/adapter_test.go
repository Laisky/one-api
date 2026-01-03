package aws

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor/anthropic"
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
		Model:       "claude-sonnet-4-5",
		Messages:    []model.Message{{Role: "user", Content: "hello"}},
		Temperature: &temp,
		TopP:        &topP,
	}

	converted, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, req)
	require.NoError(t, err)

	claudeReq, ok := converted.(*anthropic.Request)
	require.True(t, ok)

	require.NotNil(t, claudeReq.Temperature)
	require.Nil(t, claudeReq.TopP)

	stored, exists := c.Get(ctxkey.ConvertedRequest)
	require.True(t, exists)
	storedReq, ok := stored.(*anthropic.Request)
	require.True(t, ok)
	require.Nil(t, storedReq.TopP)
}
