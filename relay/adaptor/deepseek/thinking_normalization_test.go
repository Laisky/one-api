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

// TestConvertRequest_NormalizesAdaptiveThinkingType verifies DeepSeek adaptor converts
// unsupported thinking.type values into DeepSeek-compatible enums.
func TestConvertRequest_NormalizesAdaptiveThinkingType(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	request := &relaymodel.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []relaymodel.Message{
			{Role: "user", Content: "hello"},
		},
		Thinking: &relaymodel.Thinking{Type: "adaptive", BudgetTokens: 2048},
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
	require.Equal(t, 2048, converted.Thinking.BudgetTokens)
}

// TestConvertRequest_PreservesSupportedThinkingType verifies already-supported values are unchanged.
func TestConvertRequest_PreservesSupportedThinkingType(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	request := &relaymodel.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []relaymodel.Message{
			{Role: "user", Content: "hello"},
		},
		Thinking: &relaymodel.Thinking{Type: "enabled", BudgetTokens: 1024},
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
}
