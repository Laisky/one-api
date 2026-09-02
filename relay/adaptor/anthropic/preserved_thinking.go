package anthropic

import "github.com/Laisky/one-api/relay/model"

// AnthropicBetaThinkingBindingControls enables preserved-thinking mismatch controls
// and the input_transformations response field.
const AnthropicBetaThinkingBindingControls = "thinking-binding-controls-2026-08-01"

func hasClaudeThinkingBindingControls(thinking *model.Thinking) bool {
	return thinking != nil && thinking.BlockBinding != nil
}
