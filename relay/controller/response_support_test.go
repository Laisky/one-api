package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
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

func TestSupportsNativeResponseAPIDeepSeekForcesFallback(t *testing.T) {
	t.Parallel()
	metaInfo := &metalib.Meta{
		ChannelType:     channeltype.OpenAICompatible,
		Config:          model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse},
		ActualModelName: "deepseek-chat",
	}
	require.False(t, supportsNativeResponseAPI(metaInfo))

	metaInfo.ActualModelName = ""
	metaInfo.OriginModelName = "DeepSeek-Coder"
	require.False(t, supportsNativeResponseAPI(metaInfo))
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
