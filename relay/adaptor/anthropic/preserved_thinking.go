package anthropic

import "github.com/Laisky/one-api/relay/model"

// AnthropicBetaThinkingBindingControls enables preserved-thinking mismatch controls
// and the input_transformations response field.
const AnthropicBetaThinkingBindingControls = "thinking-binding-controls-2026-08-01"

// hasClaudeThinkingBindingControls reports whether thinking explicitly contains
// Anthropic block-binding controls. The parameter may be nil, and the return value
// determines whether the adaptor adds the matching Anthropic beta header.
func hasClaudeThinkingBindingControls(thinking *model.Thinking) bool {
	return thinking != nil && thinking.BlockBinding != nil
}
