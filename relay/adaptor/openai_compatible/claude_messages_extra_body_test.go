package openai_compatible

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

func TestConvertClaudeRequestCarriesExtraBody(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &model.ClaudeRequest{
		Model:     "Qwen/Qwen3.5-35B-A3B",
		MaxTokens: 128,
		Messages: []model.ClaudeMessage{{
			Role:    "user",
			Content: "hello",
		}},
		ExtraBody: map[string]any{
			"chat_template_kwargs": map[string]any{"enable_thinking": false},
		},
	}

	converted, err := ConvertClaudeRequest(c, req)
	require.NoError(t, err)

	chatReq, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	kwargs := chatReq.ExtraBody["chat_template_kwargs"].(map[string]any)
	require.Equal(t, false, kwargs["enable_thinking"])
}
