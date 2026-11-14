package openai

import (
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestGeminiCompatibleClaudeTransportURL selects the Chat endpoint only for translated Claude messages.
func TestGeminiCompatibleClaudeTransportURL(t *testing.T) {
	for _, tc := range []struct {
		name, path, model, want string
		mode                    int
	}{
		{"claude", "/v1/messages", "gemini-4-hydrogen", "/chat/completions", relaymode.ClaudeMessages},
		{"claude-mcp-chat", "/v1/messages", "gemini-4-hydrogen", "/chat/completions", relaymode.ChatCompletions},
		{"legacy-claude", "/v1/messages", "gemini-2.5-pro", "/chat/completions", relaymode.ClaudeMessages},
		{"chat", "/v1/chat/completions", "gemini-4-carbon", "/chat/completions", relaymode.ChatCompletions},
		{"proxy", "/v1/messages", "gemini-4-hydrogen", "/messages", relaymode.Proxy},
		{"embedding", "/v1/embeddings", "gemini-embedding-001", "/embeddings", relaymode.Embeddings},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &meta.Meta{ChannelType: channeltype.GeminiOpenAICompatible, Mode: tc.mode, ActualModelName: tc.model, BaseURL: "https://local.invalid/v1beta/openai/", RequestURLPath: tc.path}
			before := *info
			got, err := (&Adaptor{}).GetRequestURL(info)
			require.NoError(t, err)
			require.Equal(t, "https://local.invalid/v1beta/openai"+tc.want, got)
			require.Equal(t, before, *info)
		})
	}
}
