package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/channeltype"
	relaymeta "github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func float64PtrRT(v float64) *float64 {
	return &v
}

func stringPtrRT(s string) *string {
	return &s
}

func TestApplyRequestTransformations_ReasoningDefaults(t *testing.T) {
	adaptor := &Adaptor{}

	cases := []struct {
		name          string
		channelType   int
		expectNilTemp bool
	}{
		{name: "OpenAI Responses", channelType: channeltype.OpenAI, expectNilTemp: true},
		{name: "Azure Chat", channelType: channeltype.Azure, expectNilTemp: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta := &relaymeta.Meta{
				ChannelType:     tc.channelType,
				ActualModelName: "o1-preview",
			}

			req := &model.GeneralOpenAIRequest{
				Model:     "o1-preview",
				MaxTokens: 1500,
				Messages: []model.Message{
					{Role: "system", Content: "be precise"},
					{Role: "user", Content: "hi"},
				},
				Temperature: float64PtrRT(0.5),
				TopP:        float64PtrRT(0.9),
			}

			require.NoError(t, adaptor.applyRequestTransformations(meta, req), "applyRequestTransformations returned error")

			require.Zero(t, req.MaxTokens, "expected MaxTokens to be zeroed")

			require.NotNil(t, req.MaxCompletionTokens, "expected MaxCompletionTokens to be set")
			require.Equal(t, 1500, *req.MaxCompletionTokens, "expected MaxCompletionTokens to be 1500")

			if tc.expectNilTemp {
				require.Nil(t, req.Temperature, "expected Temperature to be removed")
			} else {
				require.NotNil(t, req.Temperature, "expected Temperature to be set")
				require.Equal(t, float64(1), *req.Temperature, "expected Temperature to be forced to 1")
			}

			require.Nil(t, req.TopP, "expected TopP to be cleared for reasoning models")

			require.NotNil(t, req.ReasoningEffort, "expected ReasoningEffort to be set")
			require.Equal(t, "medium", *req.ReasoningEffort, "expected ReasoningEffort to default to 'medium'")

			require.Len(t, req.Messages, 1, "expected system messages to be stripped for reasoning models")
			require.Equal(t, "user", req.Messages[0].Role, "expected remaining message to be user role")
		})
	}
}

func TestApplyRequestTransformations_DeepResearchAddsWebSearchTool(t *testing.T) {
	adaptor := &Adaptor{}

	meta := &relaymeta.Meta{
		ChannelType:     channeltype.OpenAI,
		ActualModelName: "o3-deep-research",
	}

	req := &model.GeneralOpenAIRequest{
		Model: "o3-deep-research",
		Messages: []model.Message{
			{Role: "user", Content: "summarize the news"},
		},
	}

	require.NoError(t, adaptor.applyRequestTransformations(meta, req), "applyRequestTransformations returned error")

	count := 0
	for _, tool := range req.Tools {
		if tool.Type == "web_search" {
			count++
		}
	}

	require.Equal(t, 1, count, "expected exactly one web_search tool after transformation")

	// Running transformations again should not duplicate the tool
	require.NoError(t, adaptor.applyRequestTransformations(meta, req), "second applyRequestTransformations returned error")

	count = 0
	for _, tool := range req.Tools {
		if tool.Type == "web_search" {
			count++
		}
	}

	require.Equal(t, 1, count, "expected web_search tool count to remain 1 after second pass")
}

func TestApplyRequestTransformations_WebSearchOptionsAddsWebSearchTool(t *testing.T) {
	adaptor := &Adaptor{}

	meta := &relaymeta.Meta{
		ChannelType:     channeltype.OpenAI,
		ActualModelName: "gpt-4o-search-preview",
	}

	req := &model.GeneralOpenAIRequest{
		Model:            "gpt-4o-search-preview",
		WebSearchOptions: &model.WebSearchOptions{},
	}

	require.NoError(t, adaptor.applyRequestTransformations(meta, req), "applyRequestTransformations returned error")

	count := 0
	for _, tool := range req.Tools {
		if tool.Type == "web_search" {
			count++
		}
	}

	require.Equal(t, 1, count, "expected exactly one web_search tool when web_search_options provided")

	// Running the transformation again should not duplicate the tool
	require.NoError(t, adaptor.applyRequestTransformations(meta, req), "second applyRequestTransformations returned error")

	count = 0
	for _, tool := range req.Tools {
		if tool.Type == "web_search" {
			count++
		}
	}

	require.Equal(t, 1, count, "expected web_search tool count to remain 1 after second pass")
}

func TestApplyRequestTransformations_DeepResearchReasoningEffort(t *testing.T) {
	adaptor := &Adaptor{}

	meta := &relaymeta.Meta{
		ChannelType:     channeltype.OpenAI,
		ActualModelName: "o4-mini-deep-research",
	}

	req := &model.GeneralOpenAIRequest{
		Model: "o4-mini-deep-research",
		Messages: []model.Message{
			{Role: "user", Content: "Summarize the latest research on fusion"},
		},
	}

	require.NoError(t, adaptor.applyRequestTransformations(meta, req), "applyRequestTransformations returned error")

	require.NotNil(t, req.ReasoningEffort, "expected ReasoningEffort to be set")
	require.Equal(t, "medium", *req.ReasoningEffort, "expected ReasoningEffort to default to 'medium'")

	// User-provided unsupported effort should be normalized to medium
	req = &model.GeneralOpenAIRequest{
		Model:           "o4-mini-deep-research",
		ReasoningEffort: stringPtrRT("high"),
		Messages:        []model.Message{{Role: "user", Content: "analyze"}},
	}

	require.NoError(t, adaptor.applyRequestTransformations(meta, req), "applyRequestTransformations returned error")

	require.NotNil(t, req.ReasoningEffort, "expected ReasoningEffort to be set")
	require.Equal(t, "medium", *req.ReasoningEffort, "expected ReasoningEffort to be normalized to 'medium'")
}

func TestApplyRequestTransformations_ResponseAPIRemovesSampling(t *testing.T) {
	adaptor := &Adaptor{}

	meta := &relaymeta.Meta{
		ChannelType:     channeltype.OpenAI,
		ActualModelName: "gpt-5-mini",
		Mode:            relaymode.ResponseAPI,
	}

	req := &model.GeneralOpenAIRequest{
		Model: "gpt-5-mini",
		Messages: []model.Message{
			{Role: "user", Content: "hello"},
		},
		Temperature: float64PtrRT(0.3),
		TopP:        float64PtrRT(0.2),
	}

	require.NoError(t, adaptor.applyRequestTransformations(meta, req), "applyRequestTransformations returned error")

	require.Nil(t, req.Temperature, "expected Temperature to be removed for Response API reasoning models")

	require.Nil(t, req.TopP, "expected TopP to be removed for Response API reasoning models")
}

func TestApplyRequestTransformations_ValidDataURLImage(t *testing.T) {
	adaptor := &Adaptor{}

	meta := &relaymeta.Meta{
		ChannelType:     channeltype.OpenAI,
		ActualModelName: "gpt-5-codex",
	}

	dataURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	req := &model.GeneralOpenAIRequest{
		Model: "gpt-5-codex",
		Messages: []model.Message{
			{
				Role: "user",
				Content: []model.MessageContent{
					{
						Type: model.ContentTypeText,
						Text: stringPtrRT("Describe the image"),
					},
					{
						Type:     model.ContentTypeImageURL,
						ImageURL: &model.ImageURL{Url: dataURL},
					},
				},
			},
		},
	}

	require.NoError(t, adaptor.applyRequestTransformations(meta, req), "applyRequestTransformations returned error for valid data URL")
}

func TestApplyRequestTransformations_InvalidDataURLImage(t *testing.T) {
	adaptor := &Adaptor{}

	meta := &relaymeta.Meta{
		ChannelType:     channeltype.OpenAI,
		ActualModelName: "gpt-5-codex",
	}

	req := &model.GeneralOpenAIRequest{
		Model: "gpt-5-codex",
		Messages: []model.Message{
			{
				Role: "user",
				Content: []model.MessageContent{
					{
						Type: model.ContentTypeText,
						Text: stringPtrRT("Describe the image"),
					},
					{
						Type:     model.ContentTypeImageURL,
						ImageURL: &model.ImageURL{Url: "data:image/png;base64,not-an-image"},
					},
				},
			},
		},
	}

	require.Error(t, adaptor.applyRequestTransformations(meta, req), "expected error for invalid data URL image")
}

func TestApplyRequestTransformations_PopulatesMetaActualModel(t *testing.T) {
	adaptor := &Adaptor{}

	meta := &relaymeta.Meta{
		ChannelType:  channeltype.OpenAI,
		ModelMapping: map[string]string{"gpt-x": "gpt-x-mapped"},
	}

	req := &model.GeneralOpenAIRequest{
		Model: "gpt-x",
		Messages: []model.Message{
			{Role: "user", Content: "hi"},
		},
	}

	require.NoError(t, adaptor.applyRequestTransformations(meta, req), "applyRequestTransformations returned error")

	require.Equal(t, "gpt-x", meta.OriginModelName, "expected OriginModelName to be populated")

	require.Equal(t, "gpt-x-mapped", meta.ActualModelName, "expected ActualModelName to use mapping")
}

func TestApplyRequestTransformations_NormalizesToolChoice(t *testing.T) {
	adaptor := &Adaptor{}

	meta := &relaymeta.Meta{
		ChannelType:     channeltype.OpenAI,
		ActualModelName: "gpt-4o-mini",
	}

	req := &model.GeneralOpenAIRequest{
		Model: "gpt-4o-mini",
		Messages: []model.Message{
			{Role: "user", Content: "Call the weather tool"},
		},
		ToolChoice: map[string]any{
			"type": "tool",
			"name": "get_weather",
		},
	}

	require.NoError(t, adaptor.applyRequestTransformations(meta, req), "applyRequestTransformations returned error")

	toolChoice, ok := req.ToolChoice.(map[string]any)
	require.True(t, ok, "expected tool_choice to be map after normalization, got %T", req.ToolChoice)

	require.Equal(t, "function", toolChoice["type"], "expected normalized tool_choice type 'function'")

	require.Equal(t, "get_weather", toolChoice["name"], "expected top-level name 'get_weather'")

	_, exists := toolChoice["function"]
	require.False(t, exists, "function block should be stripped for OpenAI upstream requests")
}
