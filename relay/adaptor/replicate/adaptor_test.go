package replicate

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestConvertRequestRejectsImageModels verifies that Replicate image models are
// rejected by the chat conversion path so per-image billing cannot be bypassed.
func TestConvertRequestRejectsImageModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	adaptor := &Adaptor{}

	converted, err := adaptor.ConvertRequest(ctx, relaymode.ChatCompletions, &model.GeneralOpenAIRequest{
		Model: "google/imagen-4",
		Messages: []model.Message{
			{Role: "user", Content: "generate an image of an aardvark"},
		},
	})

	require.Error(t, err)
	require.Nil(t, converted)
	require.Contains(t, err.Error(), "please use image API")
}

// TestConvertRequestAllowsLanguageModels verifies that Replicate language
// models continue to use the chat conversion path.
func TestConvertRequestAllowsLanguageModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	adaptor := &Adaptor{}

	converted, err := adaptor.ConvertRequest(ctx, relaymode.ChatCompletions, &model.GeneralOpenAIRequest{
		Model: "meta/meta-llama-3-8b-instruct",
		Messages: []model.Message{
			{Role: "user", Content: "hello"},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, converted)
}
