package deepseek

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
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
		Thinking: &relaymodel.Thinking{Type: "adaptive", BudgetTokens: relaymodel.IntPtr(2048)},
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
	require.Equal(t, 2048, *converted.Thinking.BudgetTokens)
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
		Thinking: &relaymodel.Thinking{Type: "enabled", BudgetTokens: relaymodel.IntPtr(1024)},
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
	require.Equal(t, 1024, *converted.Thinking.BudgetTokens)
}

// TestConvertClaudeRequest_RemovesAnthropicThinkingBinding verifies a Claude request
// routed to DeepSeek keeps portable reasoning settings but drops Anthropic-only controls.
// It accepts the test context and reports all validation failures through that context.
func TestConvertClaudeRequest_RemovesAnthropicThinkingBinding(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	request := &relaymodel.ClaudeRequest{
		Model:     "deepseek-chat",
		MaxTokens: 4096,
		Messages: []relaymodel.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
		Thinking: &relaymodel.Thinking{
			Type:         "adaptive",
			BudgetTokens: relaymodel.IntPtr(2048),
			BlockBinding: &relaymodel.ThinkingBlockBinding{
				PrefixMismatchBehavior: "drop_block",
			},
		},
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = &http.Request{}

	convertedAny, err := (&Adaptor{}).ConvertClaudeRequest(context, request)
	require.NoError(t, err)
	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, converted.Thinking)
	require.Equal(t, "enabled", converted.Thinking.Type)
	require.Equal(t, 2048, *converted.Thinking.BudgetTokens)
	require.Nil(t, converted.Thinking.BlockBinding)

	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"block_binding"`)

	// Conversion must not mutate the caller's reusable request object.
	require.NotNil(t, request.Thinking.BlockBinding)
	require.Equal(t, "drop_block", request.Thinking.BlockBinding.PrefixMismatchBehavior)
}
