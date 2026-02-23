package deepseek

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// TestConvertRequest_NormalizesAdaptiveThinkingType verifies Vertex DeepSeek adaptor
// normalizes unsupported thinking.type values before forwarding upstream.
func TestConvertRequest_NormalizesAdaptiveThinkingType(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	request := &relaymodel.GeneralOpenAIRequest{
		Model: "deepseek-v3.1",
		Messages: []relaymodel.Message{
			{Role: "user", Content: "hello"},
		},
		Thinking: &relaymodel.Thinking{Type: "adaptive", BudgetTokens: 1024},
		MaxCompletionTokens: func() *int {
			v := 512
			return &v
		}(),
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = &http.Request{}

	adaptor := &Adaptor{}
	convertedAny, err := adaptor.ConvertRequest(context, relaymode.ChatCompletions, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, converted.Thinking)
	require.Equal(t, "enabled", converted.Thinking.Type)
	require.Equal(t, 1024, converted.Thinking.BudgetTokens)
	require.Nil(t, converted.MaxCompletionTokens)
	require.Equal(t, 512, converted.MaxTokens)
}
