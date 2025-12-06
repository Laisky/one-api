package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/channeltype"
	relaymeta "github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

func float64Ptr(v float64) *float64 {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func TestIsWebSearchModel(t *testing.T) {
	testCases := []struct {
		name     string
		model    string
		expected bool
	}{
		{name: "search preview model", model: "gpt-4o-mini-search-preview", expected: true},
		{name: "search dated model", model: "gpt-4o-search-2024-12-20", expected: true},
		{name: "non search model", model: "gpt-4o-mini", expected: false},
		{name: "empty model", model: "", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := isWebSearchModel(tc.model)
			require.Equal(t, tc.expected, got, "isWebSearchModel(%q)", tc.model)
		})
	}
}

func TestApplyRequestTransformations_WebSearchStripsUnsupportedParams(t *testing.T) {
	adaptor := &Adaptor{}
	req := &model.GeneralOpenAIRequest{
		Model:            "gpt-4o-mini-search-preview",
		Temperature:      float64Ptr(0.7),
		TopP:             float64Ptr(0.8),
		PresencePenalty:  float64Ptr(0.1),
		FrequencyPenalty: float64Ptr(0.2),
		N:                intPtr(2),
	}

	m := &relaymeta.Meta{
		ChannelType:     channeltype.OpenAI,
		ActualModelName: "gpt-4o-mini-search-preview",
	}

	require.NoError(t, adaptor.applyRequestTransformations(m, req), "applyRequestTransformations returned error")
	require.Nil(t, req.Temperature, "expected Temperature to be nil for web search model")
	require.Nil(t, req.TopP, "expected TopP to be nil for web search model")
	require.Nil(t, req.PresencePenalty, "expected PresencePenalty to be nil for web search model")
	require.Nil(t, req.FrequencyPenalty, "expected FrequencyPenalty to be nil for web search model")
	require.Nil(t, req.N, "expected N to be nil for web search model")
}
