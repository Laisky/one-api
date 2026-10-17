package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	openaipayload "github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestApplyThinkingQueryToDeepSeekV4NormalizesReasoningEffort verifies chat and
// Responses queries retain the current native DeepSeek effort aliases, with or
// without a provider catalog. Parameters: t is the test handle. Returns: nothing.
func TestApplyThinkingQueryToDeepSeekV4NormalizesReasoningEffort(t *testing.T) {
	t.Parallel()

	efforts := []struct{ input, expected string }{
		{"minimal", "low"},
		{"low", "low"},
		{"medium", "high"},
		{"high", "high"},
		{"xhigh", "high"},
		{"max", "max"},
		{"ultra", "max"},
	}
	for _, model := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp", "deepseek-v4-pro"} {
		for _, catalog := range []string{"native catalog", "historical fallback"} {
			for _, effort := range efforts {
				t.Run(model+"/"+catalog+"/"+effort.input, func(t *testing.T) {
					t.Parallel()
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost,
						"/v1/chat/completions?thinking=true&reasoning_effort="+effort.input, nil)
					var meta *metalib.Meta
					if catalog == "native catalog" {
						meta = &metalib.Meta{ActualModelName: model, APIType: apitype.OpenAI, ChannelType: channeltype.DeepSeek}
					}
					chat := &relaymodel.GeneralOpenAIRequest{Model: model}
					applyThinkingQueryToChatRequest(c, chat, meta)
					require.NotNil(t, chat.ReasoningEffort, "chat query effort")
					require.Equal(t, effort.expected, *chat.ReasoningEffort, "chat query effort")
					require.Equal(t, model, chat.Model)
					require.Nil(t, chat.Thinking)

					response := &openaipayload.ResponseAPIRequest{Model: model}
					applyThinkingQueryToResponseRequest(c, response, meta)
					require.NotNil(t, response.Reasoning, "Responses query reasoning")
					require.NotNil(t, response.Reasoning.Effort, "Responses query effort")
					require.Equal(t, effort.expected, *response.Reasoning.Effort, "Responses query effort")
					require.Equal(t, model, response.Model)
				})
			}
		}
	}
}

// TestApplyThinkingQueryDeepSeekCompatibilityControls preserves legacy chat and
// other providers while exercising the actual query path. Parameters: t is the
// test handle. Returns: nothing; assertions reject widened model/effort matching.
func TestApplyThinkingQueryDeepSeekCompatibilityControls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ model, effort, expected string }{
		{"deepseek-chat", "minimal", ""},
		{"deepseek-flash-other", "ultra", ""},
		{"gpt-5", "minimal", "minimal"},
		{"gpt-5", "medium", "medium"},
		{"grok-4", "minimal", "high"},
		{"deepseek-v4-future", "medium", "high"},
		{"deepseek-v4-future", "minimal", "high"},
		{"deepseek-v4-future", "ultra", "high"},
	} {
		t.Run(tc.model+"/"+tc.effort, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost,
				"/v1/chat/completions?thinking=true&reasoning_effort="+tc.effort, nil)
			chat := &relaymodel.GeneralOpenAIRequest{Model: tc.model}
			applyThinkingQueryToChatRequest(c, chat, nil)
			response := &openaipayload.ResponseAPIRequest{Model: tc.model}
			applyThinkingQueryToResponseRequest(c, response, nil)
			if tc.expected == "" {
				require.Nil(t, chat.ReasoningEffort)
				require.Nil(t, response.Reasoning)
			} else {
				require.NotNil(t, chat.ReasoningEffort)
				require.Equal(t, tc.expected, *chat.ReasoningEffort)
				require.NotNil(t, response.Reasoning)
				require.NotNil(t, response.Reasoning.Effort)
				require.Equal(t, tc.expected, *response.Reasoning.Effort)
			}
			require.Equal(t, tc.model, chat.Model)
			require.Equal(t, tc.model, response.Model)
		})
	}
}
