package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func TestGetFullRequestURLForOpenAICompatible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		baseURL     string
		requestPath string
		expect      string
	}{
		{
			name:        "base-with-v1",
			baseURL:     "https://api.example.com/v1",
			requestPath: "/v1/chat/completions",
			expect:      "https://oneapi.laisky.com/v1/chat/completions",
		},
		{
			name:        "base-without-v1",
			baseURL:     "https://api.example.com",
			requestPath: "/v1/chat/completions",
			expect:      "https://oneapi.laisky.com/v1/chat/completions",
		},
		{
			name:        "non-v1-request",
			baseURL:     "https://api.example.com/v1",
			requestPath: "/dashboard/billing/subscription",
			expect:      "https://oneapi.laisky.com/v1/dashboard/billing/subscription",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := GetFullRequestURL(tt.baseURL, tt.requestPath, channeltype.OpenAICompatible)
			require.Equal(t, tt.expect, got)
		})
	}
}

func TestGetFullRequestURLForOtherTypes(t *testing.T) {
	t.Parallel()

	base := "https://api.openai.com"
	path := "/v1/chat/completions"

	got := GetFullRequestURL(base, path, channeltype.OpenAI)
	require.Equal(t, base+path, got)
}

func TestShouldForceResponseAPIForOpenAICompatible(t *testing.T) {
	t.Parallel()

	metaInfo := &meta.Meta{
		ChannelType: channeltype.OpenAICompatible,
		Config:      model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse},
	}
	require.True(t, shouldForceResponseAPI(metaInfo))

	metaInfo.Config.APIFormat = channeltype.OpenAICompatibleAPIFormatChatCompletion
	require.False(t, shouldForceResponseAPI(metaInfo))

	metaInfo.Config.APIFormat = channeltype.OpenAICompatibleAPIFormatResponse
	metaInfo.ResponseAPIFallback = true
	require.False(t, shouldForceResponseAPI(metaInfo))

	metaInfo.ResponseAPIFallback = false
	metaInfo.BaseURL = "https://models.github.ai"
	require.False(t, shouldForceResponseAPI(metaInfo))
}

func TestGetRequestURLForOpenAICompatible(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	metaInfo := &meta.Meta{
		ChannelType:    channeltype.OpenAICompatible,
		BaseURL:        "https://upstream.test",
		RequestURLPath: "/v1/chat/completions",
		Mode:           relaymode.ChatCompletions,
		Config:         model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse},
	}

	url, err := adaptor.GetRequestURL(metaInfo)
	require.NoError(t, err)
	require.Equal(t, "https://upstream.test/v1/responses", url)

	metaInfo.Config.APIFormat = channeltype.OpenAICompatibleAPIFormatChatCompletion
	metaInfo.RequestURLPath = "/v1/responses"
	url, err = adaptor.GetRequestURL(metaInfo)
	require.NoError(t, err)
	require.Equal(t, "https://upstream.test/v1/chat/completions", url)

	metaInfo.Config.APIFormat = channeltype.OpenAICompatibleAPIFormatResponse
	metaInfo.RequestURLPath = "/v1/chat/completions?foo=bar"
	url, err = adaptor.GetRequestURL(metaInfo)
	require.NoError(t, err)
	require.Equal(t, "https://upstream.test/v1/responses?foo=bar", url)

	metaInfo.BaseURL = "https://models.github.ai"
	metaInfo.RequestURLPath = "/v1/chat/completions"
	url, err = adaptor.GetRequestURL(metaInfo)
	require.NoError(t, err)
	require.Equal(t, "https://models.github.ai/inference/chat/completions", url)

	metaInfo.Mode = relaymode.Embeddings
	metaInfo.RequestURLPath = "/v1/embeddings"
	url, err = adaptor.GetRequestURL(metaInfo)
	require.NoError(t, err)
	require.Equal(t, "https://models.github.ai/inference/embeddings", url)
}
