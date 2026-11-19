package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
)

// openAIRoutedClaudeFixture describes an OpenAI-wire provider served by the
// OpenAI adaptor through a provider-specific chat URL builder. Parameters:
// name labels the case, channel is the channel type, modelName the provider
// model, and chatPath the provider's chat completions path. Returns: the
// fixture.
func openAIRoutedClaudeFixture(name string, channel int, modelName, chatPath string) legacyClaudeFixture {
	return legacyClaudeFixture{
		name: name, channel: channel, model: modelName, key: name + "-fixture-key",
		chatPath: chatPath, receiptUsage: true,
		reply: legacyReplyByPath(chatPath, legacyOpenAIChatJSON(modelName), legacyOpenAIChatSSE(modelName), "text/event-stream"),
		wire: func(t *testing.T, _ bool, body map[string]any) {
			require.Equal(t, modelName, body["model"])
			require.Equal(t, []any{map[string]any{"role": "user", "content": "Say ack"}}, body["messages"])
		},
	}
}

// TestOpenAIRoutedClaudeMessagesWireContract drives Claude Messages requests
// to channels whose default endpoints advertise the Messages API but whose
// chat URL builder is provider specific (MiniMax, Novita, Baidu V2, Alibaba
// Bailian). Each must reach the provider's chat completions URL and return a
// Claude message, rather than failing before dispatch as an unsupported mode.
// Doubao and DeepSeek are controls that already routed and converted Claude
// Messages (through the OpenAI adaptor and HandleClaudeMessagesResponse).
func TestOpenAIRoutedClaudeMessagesWireContract(t *testing.T) {
	for _, fx := range []legacyClaudeFixture{
		openAIRoutedClaudeFixture("minimax", channeltype.Minimax, "MiniMax-M3", "/v1/text/chatcompletion_v2"),
		openAIRoutedClaudeFixture("novita", channeltype.Novita, "tencent/hy3", "/chat/completions"),
		openAIRoutedClaudeFixture("baiduv2", channeltype.BaiduV2, "DeepSeek-V4.1-Flash", "/v2/chat/completions"),
		openAIRoutedClaudeFixture("alibailian", channeltype.AliBailian, "qwen-flash", "/compatible-mode/v1/chat/completions"),
		openAIRoutedClaudeFixture("doubao_control", channeltype.Doubao, "doubao-seed-2.1-pro", "/api/v3/chat/completions"),
		openAIRoutedClaudeFixture("deepseek_control", channeltype.DeepSeek, "deepseek-chat", "/v1/chat/completions"),
	} {
		require.True(t, channeltype.IsEndpointSupported(relaymode.ClaudeMessages, channeltype.DefaultEndpointsForChannelType(fx.channel)), "%s advertises Claude Messages", fx.name)
		runLegacyClaudeContract(t, fx)
	}
}
