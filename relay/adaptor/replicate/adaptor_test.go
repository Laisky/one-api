package replicate

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestConvertRequestRejectsImagePricedModels verifies that Replicate image
// models cannot be converted through chat completions, because image-priced
// models must use the image endpoint to receive per-image billing.
func TestConvertRequestRejectsImagePricedModels(t *testing.T) {
	t.Parallel()

	ctx := newReplicateTestContext()
	adaptor := &Adaptor{}
	req := &model.GeneralOpenAIRequest{
		Model: "black-forest-labs/flux-pro",
		Messages: []model.Message{
			{Role: "user", Content: "draw a red cube"},
		},
	}

	converted, err := adaptor.ConvertRequest(ctx, 0, req)

	require.Error(t, err)
	require.Nil(t, converted)
	require.Contains(t, err.Error(), "please use image API")
}

// TestConvertRequestAllowsLanguageModels verifies that regular Replicate chat
// models continue to convert to Replicate chat prediction requests.
func TestConvertRequestAllowsLanguageModels(t *testing.T) {
	t.Parallel()

	ctx := newReplicateTestContext()
	adaptor := &Adaptor{}
	req := &model.GeneralOpenAIRequest{
		Model: "anthropic/claude-3.5-haiku",
		Messages: []model.Message{
			{Role: "user", Content: "hello"},
		},
	}

	converted, err := adaptor.ConvertRequest(ctx, 0, req)

	require.NoError(t, err)
	require.NotNil(t, converted)
	replicateReq, ok := converted.(ReplicateChatRequest)
	require.True(t, ok)
	require.Contains(t, replicateReq.Input.Prompt, "user: hello")
}

// newReplicateTestContext creates a Gin context for Replicate adaptor tests and
// returns it without registering routes because ConvertRequest only needs the
// request-scoped context storage.
func newReplicateTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return ctx
}
