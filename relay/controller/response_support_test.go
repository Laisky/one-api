package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

func TestSupportsNativeResponseAPIOpenAICompatible(t *testing.T) {
	t.Parallel()
	metaInfo := &metalib.Meta{
		ChannelType: channeltype.OpenAICompatible,
		Config:      model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse},
	}
	require.True(t, supportsNativeResponseAPI(metaInfo))

	metaInfo.Config.APIFormat = channeltype.OpenAICompatibleAPIFormatChatCompletion
	require.False(t, supportsNativeResponseAPI(metaInfo))
}

func TestSupportsNativeResponseAPIDeepSeekContractForcesFallback(t *testing.T) {
	t.Parallel()
	metaInfo := &metalib.Meta{
		ChannelType:     channeltype.OpenAICompatible,
		Config:          model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse},
		BaseURL:         "https://api.deepseek.com/v1",
		ActualModelName: "deepseek-chat",
	}
	require.False(t, supportsNativeResponseAPI(metaInfo))

	metaInfo.ActualModelName = ""
	metaInfo.OriginModelName = "DeepSeek-Coder"
	require.False(t, supportsNativeResponseAPI(metaInfo))
}

// TestSupportsNativeResponseAPIDeepSeekV4 verifies that all current DeepSeek
// V4 models use the native Responses API for plaintext reasoning preservation.
// Parameters: t is the testing handle used for assertions and test lifecycle control.
// Returns: nothing; the test fails through t when a current model is routed through fallback.
func TestSupportsNativeResponseAPIDeepSeekV4(t *testing.T) {
	t.Parallel()

	for _, channelType := range []int{channeltype.DeepSeek, channeltype.OpenAICompatible} {
		for _, modelName := range []string{
			"deepseek-v4-flash",
			"deepseek-v4-flash-vision-exp",
			"deepseek-v4-pro",
		} {
			metaInfo := &metalib.Meta{
				ChannelType:     channelType,
				Config:          model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse},
				BaseURL:         "https://api.deepseek.com/v1",
				ActualModelName: modelName,
			}
			require.True(t, supportsNativeResponseAPI(metaInfo), "channel type %d must preserve native Responses state for %s", channelType, modelName)
		}
	}
}

func TestSupportsNativeResponseAPIDeepSeekModelOnNeutralProxyUsesConfiguredFormat(t *testing.T) {
	t.Parallel()
	metaInfo := &metalib.Meta{
		ChannelType:     channeltype.OpenAICompatible,
		Config:          model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse},
		BaseURL:         "https://proxy.example.com/v1",
		ActualModelName: "deepseek-chat",
	}
	require.True(t, supportsNativeResponseAPI(metaInfo))
}

func TestSupportsNativeResponseAPIAzureGpt5(t *testing.T) {
	t.Parallel()
	metaInfo := &metalib.Meta{
		ChannelType:     channeltype.Azure,
		ActualModelName: "gpt-5-nano",
	}
	require.True(t, supportsNativeResponseAPI(metaInfo))

	metaInfo.ActualModelName = "gpt-4o-mini"
	require.False(t, supportsNativeResponseAPI(metaInfo))
}

func TestSupportsNativeResponseAPISearchPreviewFallback(t *testing.T) {
	t.Parallel()
	metaInfo := &metalib.Meta{
		ChannelType:     channeltype.OpenAI,
		ActualModelName: "gpt-4o-mini-search-preview",
	}
	require.False(t, supportsNativeResponseAPI(metaInfo))

	metaInfo.ActualModelName = ""
	metaInfo.OriginModelName = "alias-search-preview"
	metaInfo.ModelMapping = map[string]string{"alias-search-preview": "gpt-4o-mini-search-preview"}
	metaInfo.ActualModelName = metalib.GetMappedModelName(metaInfo.OriginModelName, metaInfo.ModelMapping)
	require.False(t, supportsNativeResponseAPI(metaInfo))
}

func TestIsReasoningModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		modelName string
		expected  bool
	}{
		// Direct model names (prefix match)
		{"gpt-5-mini", true},
		{"gpt-5-nano", true},
		{"o1-preview", true},
		{"o1-mini", true},
		{"o3-mini", true},
		{"o4-preview", true},
		{"o-mini", true},

		// Prefixed model names (user-facing aliases)
		{"azure-gpt-5-nano", true},
		{"azure-gpt-5-mini", true},
		{"vertex-o1-mini", true},
		{"custom-o3-preview", true},
		{"myprefix-o4-latest", true},

		// Non-reasoning models
		{"gpt-4o-mini", false},
		{"gpt-4o", false},
		{"claude-3-5-sonnet", false},
		{"gemini-2.5-flash", false},
		{"deepseek-chat", false},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.modelName, func(t *testing.T) {
			t.Parallel()
			result := isReasoningModel(tc.modelName)
			require.Equal(t, tc.expected, result, "isReasoningModel(%q) = %v, want %v", tc.modelName, result, tc.expected)
		})
	}
}
