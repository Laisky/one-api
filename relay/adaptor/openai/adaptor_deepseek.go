package openai

import (
	"encoding/json"
	"fmt"

	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/relay/adaptor/common/deepseekcompat"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
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
	return deepseekcompat.UsesDeepSeekAPIContract(metaInfo)
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

// enforceDeepSeekHistoryContract repairs replayed history that DeepSeek would reject
// outright: tool calls nobody answered and an in-flight assistant turn whose thinking
// was not replayed. See deepseekcompat.EnforceHistoryContract for the verified rules.
// Parameters: lg receives a debug summary and may be nil; request is mutated in place.
// Returns: nothing.
func enforceDeepSeekHistoryContract(lg deepSeekThinkingNormalizeLogger, request *model.GeneralOpenAIRequest) {
	if request == nil {
		return
	}

	repaired, stats := deepseekcompat.EnforceHistoryContract(request.Messages)
	request.Messages = repaired
	if !stats.Changed() || lg == nil {
		return
	}

	lg.Debug("repaired deepseek history for provider validation",
		zap.String("model", request.Model),
		zap.Int("unanswered_tool_calls_dropped", stats.UnansweredToolCallsDropped),
		zap.Int("assistant_messages_dropped", stats.AssistantMessagesDropped),
		zap.Int("reasoning_placeholders_added", stats.ReasoningPlaceholdersAdded),
	)
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
			zap.Intp("budget_tokens", request.Thinking.BudgetTokens),
		)
	}
}
