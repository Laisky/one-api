package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Laisky/zap"

	"github.com/songquanpeng/one-api/relay/adaptor/common/deepseekcompat"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

type deepSeekToolNormalizeLogger interface {
	Debug(msg string, fields ...zap.Field)
}

// deepSeekThinkingNormalizeLogger defines the logger interface used by DeepSeek thinking normalization.
type deepSeekThinkingNormalizeLogger interface {
	Debug(msg string, fields ...zap.Field)
}

// shouldNormalizeToolMessageContentForDeepSeek reports whether tool message content should
// be normalized to string for DeepSeek-compatible upstreams.
func shouldNormalizeToolMessageContentForDeepSeek(metaInfo *meta.Meta, request *model.GeneralOpenAIRequest) bool {
	if metaInfo != nil {
		if metaInfo.ChannelType == channeltype.DeepSeek {
			return true
		}
		if strings.Contains(strings.ToLower(strings.TrimSpace(metaInfo.BaseURL)), "deepseek") {
			return true
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(metaInfo.ActualModelName)), "deepseek-") {
			return true
		}
	}

	if request != nil {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(request.Model)), "deepseek-") {
			return true
		}
	}

	return false
}

// normalizeDeepSeekToolMessageContent converts non-string tool message content into strings.
func normalizeDeepSeekToolMessageContent(lg deepSeekToolNormalizeLogger, request *model.GeneralOpenAIRequest) {
	if request == nil {
		return
	}

	toolMessageCount := 0
	normalizedCount := 0

	for idx := range request.Messages {
		message := &request.Messages[idx]
		if message.Role != "tool" {
			continue
		}

		toolMessageCount++
		if _, ok := message.Content.(string); ok {
			continue
		}

		normalized := message.StringContent()
		if normalized == "" {
			if message.Content == nil {
				normalized = ""
			} else {
				encoded, err := json.Marshal(message.Content)
				if err != nil {
					normalized = fmt.Sprintf("%v", message.Content)
					if lg != nil {
						lg.Debug("deepseek tool message fallback marshal failed",
							zap.Int("message_index", idx),
							zap.String("original_content_type", fmt.Sprintf("%T", message.Content)),
							zap.Error(err),
						)
					}
				} else {
					normalized = string(encoded)
				}
			}
		}

		message.Content = normalized
		normalizedCount++
		if lg != nil {
			lg.Debug("normalized deepseek tool message content",
				zap.Int("message_index", idx),
				zap.Int("normalized_content_length", len(normalized)),
			)
		}
	}

	if lg != nil && toolMessageCount > 0 {
		lg.Debug("deepseek tool message normalization summary",
			zap.Int("tool_message_count", toolMessageCount),
			zap.Int("normalized_count", normalizedCount),
		)
	}
}

// normalizeClaudeThinkingForDeepSeek coerces Claude thinking payloads into DeepSeek-compatible values.
// DeepSeek currently accepts only `enabled` or `disabled` for thinking.type.
func normalizeClaudeThinkingForDeepSeek(lg deepSeekThinkingNormalizeLogger, request *model.GeneralOpenAIRequest) {
	if request == nil || request.Thinking == nil {
		return
	}

	originalType := request.Thinking.Type
	normalizedType, changed := deepseekcompat.NormalizeThinkingType(originalType, request.Thinking.BudgetTokens)
	if !changed {
		return
	}

	request.Thinking.Type = normalizedType
	if lg != nil {
		lg.Debug("normalized claude thinking type for deepseek compatibility",
			zap.String("model", request.Model),
			zap.String("original_type", originalType),
			zap.String("normalized_type", normalizedType),
			zap.Int("budget_tokens", request.Thinking.BudgetTokens),
		)
	}
}
