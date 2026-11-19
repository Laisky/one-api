package baiduv2

import (
	"fmt"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// GetRequestURL returns the Baidu V2 (Qianfan) URL for the relay mode. Claude
// Messages requests are converted to Chat Completions, so they use the chat
// URL. Parameters: meta is the request meta. Returns: the URL or an error for
// an unsupported relay mode.
func GetRequestURL(meta *meta.Meta) (string, error) {
	switch meta.Mode {
	case relaymode.ChatCompletions, relaymode.ClaudeMessages:
		return fmt.Sprintf("%s/v2/chat/completions", meta.BaseURL), nil
	case relaymode.Rerank:
		return fmt.Sprintf("%s/v2/rerankers", meta.BaseURL), nil
	default:
		return "", errors.Errorf("unsupported relay mode %d for baidu v2", meta.Mode)
	}
}
