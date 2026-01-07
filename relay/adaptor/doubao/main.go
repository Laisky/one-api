package doubao

import (
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func GetRequestURL(meta *meta.Meta) (string, error) {
	switch meta.Mode {
	case relaymode.ChatCompletions:
		if strings.HasPrefix(meta.ActualModelName, "bot") {
			return fmt.Sprintf("%s/api/v3/bots/chat/completions", meta.BaseURL), nil
		}
		return fmt.Sprintf("%s/api/v3/chat/completions", meta.BaseURL), nil
	case relaymode.Embeddings:
		return fmt.Sprintf("%s/api/v3/embeddings", meta.BaseURL), nil
	case relaymode.ClaudeMessages:
		// 豆包支持 Claude Messages API 格式，转换为 ChatCompletions 处理
		if strings.HasPrefix(meta.ActualModelName, "bot") {
			return fmt.Sprintf("%s/api/v3/bots/chat/completions", meta.BaseURL), nil
		}
		return fmt.Sprintf("%s/api/v3/chat/completions", meta.BaseURL), nil
	default:
	}
	return "", errors.Errorf("unsupported relay mode %d for doubao", meta.Mode)
}
