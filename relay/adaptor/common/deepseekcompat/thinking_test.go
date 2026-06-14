package deepseekcompat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

func intPtr(v int) *int {
	return &v
}

func TestNormalizeThinkingType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		rawType         string
		budgetTokens    *int
		expectedNorm    string
		expectedChanged bool
	}{
		{name: "enabled unchanged", rawType: "enabled", budgetTokens: intPtr(1024), expectedNorm: "enabled", expectedChanged: false},
		{name: "disabled unchanged", rawType: "disabled", budgetTokens: nil, expectedNorm: "disabled", expectedChanged: false},
		{name: "adaptive to enabled", rawType: "adaptive", budgetTokens: nil, expectedNorm: "enabled", expectedChanged: true},
		{name: "adaptive with budget to enabled", rawType: "adaptive", budgetTokens: intPtr(4096), expectedNorm: "enabled", expectedChanged: true},
		{name: "empty with budget to enabled", rawType: "", budgetTokens: intPtr(2048), expectedNorm: "enabled", expectedChanged: true},
		{name: "empty without budget to disabled", rawType: "", budgetTokens: nil, expectedNorm: "disabled", expectedChanged: true},
		{name: "unknown with budget to enabled", rawType: "auto", budgetTokens: intPtr(1024), expectedNorm: "enabled", expectedChanged: true},
		{name: "unknown without budget to disabled", rawType: "auto", budgetTokens: nil, expectedNorm: "disabled", expectedChanged: true},
		{name: "ENABLED case normalized", rawType: "ENABLED", budgetTokens: nil, expectedNorm: "enabled", expectedChanged: true},
		{name: "enabled nil budget unchanged", rawType: "enabled", budgetTokens: nil, expectedNorm: "enabled", expectedChanged: false},
		{name: "zero budget treated as no budget", rawType: "", budgetTokens: intPtr(0), expectedNorm: "disabled", expectedChanged: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			normalized, changed := NormalizeThinkingType(tc.rawType, tc.budgetTokens)
			assert.Equal(t, tc.expectedNorm, normalized)
			assert.Equal(t, tc.expectedChanged, changed)
		})
	}
}

func TestHostUsesDeepSeekAPIContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		rawURL string
		want   bool
	}{
		{name: "api host", rawURL: "https://api.deepseek.com", want: true},
		{name: "api host with path", rawURL: "https://api.deepseek.com/v1", want: true},
		{name: "mixed case host", rawURL: "https://API.DeepSeek.com/v1", want: true},
		{name: "host without scheme", rawURL: "api.deepseek.com/v1", want: true},
		{name: "root host", rawURL: "https://deepseek.com", want: true},
		{name: "unrelated host path mentions deepseek", rawURL: "https://proxy.example.com/deepseek/v1", want: false},
		{name: "suffix trap", rawURL: "https://api.notdeepseek.com", want: false},
		{name: "nvidia host", rawURL: "https://integrate.api.nvidia.com/v1", want: false},
		{name: "empty", rawURL: "", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, HostUsesDeepSeekAPIContract(tc.rawURL))
		})
	}
}

func TestUsesDeepSeekAPIContract(t *testing.T) {
	t.Parallel()

	require.True(t, UsesDeepSeekAPIContract(&meta.Meta{ChannelType: channeltype.DeepSeek}))
	require.True(t, UsesDeepSeekAPIContract(&meta.Meta{
		ChannelType: channeltype.OpenAICompatible,
		BaseURL:     "https://api.deepseek.com/v1",
	}))
	require.False(t, UsesDeepSeekAPIContract(&meta.Meta{
		ChannelType: channeltype.OpenAICompatible,
		BaseURL:     "https://proxy.example.com/deepseek/v1",
	}))
	require.False(t, UsesDeepSeekAPIContract(nil))
}

func TestIsDeepSeekModel(t *testing.T) {
	t.Parallel()

	require.True(t, IsDeepSeekModel("deepseek-chat"))
	require.True(t, IsDeepSeekModel("DeepSeek-Coder"))
	require.True(t, IsDeepSeekModel("deepseek-ai/deepseek-v4-flash"))
	require.False(t, IsDeepSeekModel("my-deepseek-alias"))
	require.False(t, IsDeepSeekModel(""))
}
