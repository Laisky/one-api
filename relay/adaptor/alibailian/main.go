package alibailian

import (
	"fmt"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// GetRequestURL returns the Bailian compatible-mode URL for the relay mode.
// Claude Messages requests are converted to Chat Completions, so they use the
// chat URL. Parameters: meta is the request meta. Returns: the URL or an error
// for an unsupported relay mode.
func GetRequestURL(meta *meta.Meta) (string, error) {
	switch meta.Mode {
	case relaymode.ChatCompletions, relaymode.ClaudeMessages:
		return fmt.Sprintf("%s/compatible-mode/v1/chat/completions", meta.BaseURL), nil
	case relaymode.Embeddings:
		return fmt.Sprintf("%s/compatible-mode/v1/embeddings", meta.BaseURL), nil
	default:
	}
	return "", errors.Errorf("unsupported relay mode %d for ali bailian", meta.Mode)
}
