package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func TestApplyRequestTransformationsPreservesHighEffort(t *testing.T) {
	adaptor := &Adaptor{}
	metaInfo := &meta.Meta{
		ActualModelName: "gpt-5",
		ChannelType:     channeltype.OpenAI,
		Mode:            relaymode.ChatCompletions,
	}
	effort := "high"
	request := &model.GeneralOpenAIRequest{
		Model:           "gpt-5",
		Messages:        []model.Message{{Role: "user", Content: "hi"}},
		ReasoningEffort: &effort,
	}

	err := adaptor.applyRequestTransformations(metaInfo, request)
	require.NoError(t, err)
	require.NotNil(t, request.ReasoningEffort)
	require.Equal(t, "high", *request.ReasoningEffort)
}

func TestApplyRequestTransformationsClampsGpt51ChatEffort(t *testing.T) {
	adaptor := &Adaptor{}
	metaInfo := &meta.Meta{
		ActualModelName: "gpt-5.1-chat-latest",
		ChannelType:     channeltype.OpenAI,
		Mode:            relaymode.ChatCompletions,
	}
	effort := "high"
	request := &model.GeneralOpenAIRequest{
		Model:           "gpt-5.1-chat-latest",
		Messages:        []model.Message{{Role: "user", Content: "hi"}},
		ReasoningEffort: &effort,
	}

	err := adaptor.applyRequestTransformations(metaInfo, request)
	require.NoError(t, err)
	require.NotNil(t, request.ReasoningEffort)
	require.Equal(t, "medium", *request.ReasoningEffort)
}

func TestApplyRequestTransformationsDisablesGpt5ChatLatestReasoning(t *testing.T) {
	adaptor := &Adaptor{}
	metaInfo := &meta.Meta{
		ActualModelName: "gpt-5-chat-latest",
		ChannelType:     channeltype.OpenAI,
		Mode:            relaymode.ChatCompletions,
	}
	effort := "high"
	request := &model.GeneralOpenAIRequest{
		Model:           "gpt-5-chat-latest",
		Messages:        []model.Message{{Role: "user", Content: "hi"}},
		ReasoningEffort: &effort,
	}

	err := adaptor.applyRequestTransformations(metaInfo, request)
	require.NoError(t, err)
	require.Nil(t, request.ReasoningEffort)
}

func TestConvertChatToResponseAPIPreservesEffort(t *testing.T) {
	metaInfo := &meta.Meta{
		ActualModelName: "o3-mini",
		ChannelType:     channeltype.OpenAI,
		Mode:            relaymode.ChatCompletions,
	}
	adaptor := &Adaptor{}
	request := &model.GeneralOpenAIRequest{
		Model:           "o3-mini",
		Messages:        []model.Message{{Role: "user", Content: "test"}},
		ReasoningEffort: stringPtrReasoning("high"),
	}

	// Force default transformations (should clamp reasoning effort to medium for medium-only models)
	err := adaptor.applyRequestTransformations(metaInfo, request)
	require.NoError(t, err)
	require.NotNil(t, request.ReasoningEffort)
	require.Equal(t, "medium", *request.ReasoningEffort)

	responsePayload := ConvertChatCompletionToResponseAPI(request)
	require.NotNil(t, responsePayload)
	require.NotNil(t, responsePayload.Reasoning)
	require.NotNil(t, responsePayload.Reasoning.Effort)
	require.Equal(t, "medium", *responsePayload.Reasoning.Effort)
}

func TestConvertChatToResponseAPIUsesDetailedSummaryForO4(t *testing.T) {
	request := &model.GeneralOpenAIRequest{
		Model:    "o4-mini",
		Messages: []model.Message{{Role: "user", Content: "test"}},
	}

	responsePayload := ConvertChatCompletionToResponseAPI(request)
	require.NotNil(t, responsePayload)
	require.NotNil(t, responsePayload.Reasoning)
	require.NotNil(t, responsePayload.Reasoning.Summary)
	require.Equal(t, "detailed", *responsePayload.Reasoning.Summary)
}

func stringPtrReasoning(value string) *string {
	return &value
}
