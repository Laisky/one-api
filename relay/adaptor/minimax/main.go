package minimax

import (
	"fmt"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// GetRequestURL returns the MiniMax chat completions URL. Claude Messages
// requests are converted to Chat Completions, so they use the same URL.
// Parameters: meta is the request meta. Returns: the URL or an error for any
// other relay mode.
func GetRequestURL(meta *meta.Meta) (string, error) {
	if meta.Mode == relaymode.ChatCompletions || meta.Mode == relaymode.ClaudeMessages {
		return fmt.Sprintf("%s/v1/text/chatcompletion_v2", meta.BaseURL), nil
	}
	return "", errors.Errorf("unsupported relay mode %d for minimax", meta.Mode)
}
