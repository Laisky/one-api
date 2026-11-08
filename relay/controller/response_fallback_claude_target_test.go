package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// TestResponseFallbackClaudeTargetIsOneDecision proves state hydration and
// Claude thinking replay classify every upstream the same way, through the
// single responseFallbackLowersToClaude predicate. Bedrock follows the AWS
// registry (model ids and ARNs routed to its Claude adaptor), so a non-Claude
// Bedrock model is a Chat target for hydration too, where it used to be
// classified as Claude for hydration but not for replay.
func TestResponseFallbackClaudeTargetIsOneDecision(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		meta   *metalib.Meta
		claude bool
	}{
		{name: "anthropic", meta: &metalib.Meta{APIType: apitype.Anthropic, ChannelType: channeltype.Anthropic, ActualModelName: "claude-opus-5-5"}, claude: true},
		{name: "bedrock_claude_id", meta: &metalib.Meta{APIType: apitype.AwsClaude, ChannelType: channeltype.AwsClaude, ActualModelName: "claude-opus-5-5"}, claude: true},
		{name: "bedrock_claude_arn", meta: &metalib.Meta{APIType: apitype.AwsClaude, ChannelType: channeltype.AwsClaude, ActualModelName: "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-opus-5-5-v1:0"}, claude: true},
		{name: "bedrock_llama", meta: &metalib.Meta{APIType: apitype.AwsClaude, ChannelType: channeltype.AwsClaude, ActualModelName: "llama3-70b"}, claude: false},
		{name: "vertex_claude", meta: &metalib.Meta{APIType: apitype.VertexAI, ChannelType: channeltype.VertextAI, ActualModelName: "claude-sonnet-5"}, claude: true},
		{name: "vertex_gemini", meta: &metalib.Meta{APIType: apitype.VertexAI, ChannelType: channeltype.VertextAI, ActualModelName: "gemini-2.5-pro"}, claude: false},
		{name: "azure_foundry_claude", meta: &metalib.Meta{APIType: apitype.Azure, ChannelType: channeltype.Azure, ActualModelName: "claude-opus-5-5"}, claude: true},
		{name: "azure_gpt", meta: &metalib.Meta{APIType: apitype.Azure, ChannelType: channeltype.Azure, ActualModelName: "gpt-5"}, claude: false},
		{name: "openai_compatible_claude_name", meta: &metalib.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenAICompatible, ActualModelName: "claude-opus-5-5"}, claude: false},
		{name: "deepseek", meta: &metalib.Meta{APIType: apitype.DeepSeek, ChannelType: channeltype.DeepSeek, ActualModelName: "deepseek-v4"}, claude: false},
		{name: "nil", meta: nil, claude: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.claude, responseFallbackLowersToClaude(tc.meta))
			require.Equal(t, tc.claude, responseFallbackReplaysClaudeThinking(tc.meta), "thinking replay follows the shared decision")
			want := targetChatFallback
			if tc.claude {
				want = targetClaudeFallback
			}
			require.Equal(t, want, responseFallbackTarget(tc.meta), "hydration follows the shared decision")
		})
	}
}
