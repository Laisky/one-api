package openai

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymeta "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// convertSystemPromptRequest converts one Chat or Claude-ingress request with a
// leading system message through the production adaptor. Parameters: t owns
// assertions, info is the attempt metadata, mode the client surface and
// messages the request history. It returns the upstream JSON root.
func convertSystemPromptRequest(t *testing.T, info *relaymeta.Meta, mode int, messages []relaymodel.Message) map[string]json.RawMessage {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	relaymeta.Set2Context(c, info)
	a := &Adaptor{ChannelType: info.ChannelType}
	request := &relaymodel.GeneralOpenAIRequest{Model: info.ActualModelName, Messages: messages}
	var converted any
	var err error
	if mode == relaymode.ClaudeMessages {
		converted, err = a.ConvertClaudeRequest(c, &relaymodel.ClaudeRequest{Model: info.ActualModelName, MaxTokens: 64,
			System: messages[0].StringContent(), Messages: []relaymodel.ClaudeMessage{{Role: "user", Content: "Find the release notes."}}})
	} else {
		converted, err = a.ConvertRequest(c, mode, request)
	}
	require.NoError(t, err)
	wire, err := json.Marshal(converted)
	require.NoError(t, err)
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(wire, &root))
	return root
}

// TestResponsesConversionKeepsSystemInstructions proves that every route that
// converts Chat or Claude ingress into a Responses body sends the leading system
// message as upstream instructions. Reasoning models previously lost it on every
// route except native OpenAI GPT-6: Azure GPT-6.2 deployments, Azure and native
// GPT-5.x and o-series models, and OpenAI-compatible Responses channels.
// Parameter t owns assertions.
func TestResponsesConversionKeepsSystemInstructions(t *testing.T) {
	const instructions = "Answer as the release manager."
	messages := []relaymodel.Message{{Role: "system", Content: instructions}, {Role: "user", Content: "Find the release notes."}}
	responseFormat := model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse}
	for _, tc := range []struct {
		name    string
		channel int
		model   string
		config  model.ChannelConfig
	}{
		{name: "azure_gpt62_sol", channel: channeltype.Azure, model: "gpt-6.2-sol"},
		{name: "azure_gpt62_astra", channel: channeltype.Azure, model: "gpt-6.2-astra"},
		{name: "azure_gpt6_luna", channel: channeltype.Azure, model: "gpt-6-luna"},
		{name: "azure_gpt54", channel: channeltype.Azure, model: "gpt-5.4"},
		{name: "native_gpt62_sol", channel: channeltype.OpenAI, model: "gpt-6.2-sol"},
		{name: "native_gpt54", channel: channeltype.OpenAI, model: "gpt-5.4"},
		{name: "native_o3", channel: channeltype.OpenAI, model: "o3"},
		{name: "compatible_responses_gpt62_sol", channel: channeltype.OpenAICompatible, model: "gpt-6.2-sol", config: responseFormat},
	} {
		for _, mode := range []int{relaymode.ChatCompletions, relaymode.ClaudeMessages} {
			t.Run(tc.name+"/"+relaymode.String(mode), func(t *testing.T) {
				info := &relaymeta.Meta{ChannelType: tc.channel, ActualModelName: tc.model, Mode: mode,
					RequestURLPath: "/v1/chat/completions", BaseURL: "https://upstream.example", Config: tc.config}
				require.True(t, shouldForceResponseAPI(info), "fixture must use a Responses-converted route")
				require.True(t, isModelSupportedReasoning(tc.model), "fixture must exercise the reasoning filter")
				root := convertSystemPromptRequest(t, info, mode, messages)
				require.JSONEq(t, `"`+instructions+`"`, string(root["instructions"]))
				var input []map[string]any
				require.NoError(t, json.Unmarshal(root["input"], &input))
				require.Len(t, input, 1, "the system message moves to instructions and the user turn remains")
				require.Equal(t, "user", input[0]["role"])
			})
		}
	}
}

// TestResponsesConversionLegacySystemFilter keeps the historical reasoning
// filter where it still applies: o1-preview and o1-mini never accepted system
// or developer messages, and Chat-routed reasoning requests keep their wire.
// Parameter t owns assertions.
func TestResponsesConversionLegacySystemFilter(t *testing.T) {
	messages := []relaymodel.Message{{Role: "system", Content: "be precise"}, {Role: "user", Content: "hi"}}
	for _, tc := range []struct {
		name    string
		channel int
		model   string
	}{
		{name: "native_o1_preview", channel: channeltype.OpenAI, model: "o1-preview"},
		{name: "native_o1_mini", channel: channeltype.OpenAI, model: "o1-mini"},
		{name: "azure_chat_o3", channel: channeltype.Azure, model: "o3"},
		{name: "custom_chat_o3_mini", channel: channeltype.Custom, model: "o3-mini"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaymeta.Meta{ChannelType: tc.channel, ActualModelName: tc.model, Mode: relaymode.ChatCompletions,
				RequestURLPath: "/v1/chat/completions", BaseURL: "https://upstream.example"}
			root := convertSystemPromptRequest(t, info, relaymode.ChatCompletions, messages)
			require.NotContains(t, root, "instructions")
			if raw, ok := root["messages"]; ok {
				var chat []map[string]any
				require.NoError(t, json.Unmarshal(raw, &chat))
				require.Len(t, chat, 1)
				require.Equal(t, "user", chat[0]["role"])
				return
			}
			var input []map[string]any
			require.NoError(t, json.Unmarshal(root["input"], &input))
			require.Len(t, input, 1)
			require.Equal(t, "user", input[0]["role"])
		})
	}
}

// TestConvertChatCompletionToResponseAPIEmptySystem proves an empty leading
// system message, which produces no input item, neither drops the following
// user turn nor panics when it is the only message. Parameter t owns assertions.
func TestConvertChatCompletionToResponseAPIEmptySystem(t *testing.T) {
	converted := ConvertChatCompletionToResponseAPI(&relaymodel.GeneralOpenAIRequest{Model: "gpt-6.2-sol",
		Messages: []relaymodel.Message{{Role: "system", Content: ""}, {Role: "user", Content: "hi"}}})
	require.Len(t, converted.Input, 1, "the user turn must survive an empty system message")
	item, ok := converted.Input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "user", item["role"])

	require.NotPanics(t, func() {
		only := ConvertChatCompletionToResponseAPI(&relaymodel.GeneralOpenAIRequest{Model: "gpt-6.2-sol",
			Messages: []relaymodel.Message{{Role: "system", Content: ""}}})
		require.Empty(t, only.Input)
	})
}
