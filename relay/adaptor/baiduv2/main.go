package baiduv2

import (
	"fmt"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// GetRequestURL returns the request URL for the given meta information.
func GetRequestURL(meta *meta.Meta) (string, error) {
	switch meta.Mode {
	case relaymode.ChatCompletions:
		return fmt.Sprintf("%s/v2/chat/completions", meta.BaseURL), nil
	case relaymode.Rerank:
		return fmt.Sprintf("%s/v2/rerankers", meta.BaseURL), nil
	default:
		return "", errors.Errorf("unsupported relay mode %d for baidu v2", meta.Mode)
	}
}
