package deepseekcompat

import (
	"github.com/Laisky/one-api/relay/model"
)

// HistoryContractStats reports the repairs EnforceHistoryContract applied.
//
// Fields:
//   - UnansweredToolCallsDropped: tool calls removed because no tool message answered them.
//   - AssistantMessagesDropped: assistant messages removed after losing their last tool call.
//   - ReasoningPlaceholdersAdded: assistant messages given an empty reasoning_content.
type HistoryContractStats struct {
	UnansweredToolCallsDropped int
	AssistantMessagesDropped   int
	ReasoningPlaceholdersAdded int
}

// Changed reports whether any repair was applied.
//
// Returns:
//   - bool: true when the message slice was rewritten in some way.
func (s HistoryContractStats) Changed() bool {
	return s.UnansweredToolCallsDropped > 0 ||
		s.AssistantMessagesDropped > 0 ||
		s.ReasoningPlaceholdersAdded > 0
}

// EnforceHistoryContract rewrites replayed history so it satisfies the two
// whole-conversation rules DeepSeek's chat completions endpoint validates before
// it looks at the prompt. Both were verified live against api.deepseek.com with
// deepseek-flash on 2026-09-18:
//
//  1. Every tool_call_id in an assistant message must be answered by one of the
//     tool messages that directly follow it, otherwise the request fails with
//     "An assistant message with 'tool_calls' must be followed by tool messages
//     responding to each 'tool_call_id'". DeepSeek enforces this even when the
//     assistant message is the last one, where OpenAI accepts it as a prefill.
//  2. Thinking mode is on unless the caller disables it, and then every assistant
//     message positioned after the last user message must carry a
//     reasoning_content field unless it carries tool_calls; an empty string
//     satisfies it. A missing field fails with "The `reasoning_content` in the
//     thinking mode must be passed back to the API", regardless of whether the
//     conversation uses tools at all. Assistant messages before the last user
//     message are exempt, so completed turns keep whatever they replayed.
//
// Both repairs are lossless from the client's point of view: an unanswered call
// is dropped rather than answered with an invented tool result, and reasoning is
// only ever added where the field is absent, never overwritten.
//
// Messages are normalized in place, so the returned slice must replace the
// caller's; it is shorter than the input whenever a message had to be dropped.
//
// Parameters:
//   - messages: the chat history about to be sent upstream.
//
// Returns:
//   - []model.Message: the history to forward upstream.
//   - HistoryContractStats: what was repaired, for debug logging.
func EnforceHistoryContract(messages []model.Message) ([]model.Message, HistoryContractStats) {
	var stats HistoryContractStats
	if len(messages) == 0 {
		return messages, stats
	}

	repaired := dropUnansweredToolCalls(messages, &stats)
	addTrailingReasoningPlaceholders(repaired, &stats)
	return repaired, stats
}

// dropUnansweredToolCalls removes tool calls that no adjacent tool message answers.
//
// Parameters:
//   - messages: the chat history to inspect.
//   - stats: accumulator updated with the repairs applied.
//
// Returns:
//   - []model.Message: messages itself when nothing had to change, else a new slice.
func dropUnansweredToolCalls(messages []model.Message, stats *HistoryContractStats) []model.Message {
	repaired := messages
	copied := false

	for idx := 0; idx < len(repaired); idx++ {
		message := repaired[idx]
		if message.Role != "assistant" || len(message.ToolCalls) == 0 {
			continue
		}

		answered := answeredToolCallIDs(repaired, idx+1)
		retained := make([]model.Tool, 0, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			if _, ok := answered[call.Id]; ok {
				retained = append(retained, call)
				continue
			}
			stats.UnansweredToolCallsDropped++
		}
		if len(retained) == len(message.ToolCalls) {
			continue
		}

		if !copied {
			// Only clone once a repair is actually needed, so the common compliant
			// history is forwarded without extra allocation, and so the caller's
			// slice is never mutated behind its back.
			repaired = append(make([]model.Message, 0, len(messages)), messages...)
			copied = true
			message = repaired[idx]
		}

		if len(retained) == 0 {
			retained = nil
		}
		message.ToolCalls = retained
		if retained == nil && isBlankMessageContent(message.Content) && message.ReasoningContent == nil {
			// The message carried nothing but calls nobody answered, so forwarding it
			// would only add an empty assistant turn. Dropping it cannot orphan a tool
			// message: every tool message adjacent to it answered one of the calls we
			// just kept, and there are none.
			repaired = append(repaired[:idx], repaired[idx+1:]...)
			stats.AssistantMessagesDropped++
			idx--
			continue
		}
		repaired[idx] = message
	}

	return repaired
}

// answeredToolCallIDs collects the IDs answered by the tool messages at start.
//
// Parameters:
//   - messages: the chat history to scan.
//   - start: index of the first message following an assistant tool-call message.
//
// Returns:
//   - map[string]struct{}: the tool_call_id values of the directly following tool run.
func answeredToolCallIDs(messages []model.Message, start int) map[string]struct{} {
	answered := make(map[string]struct{})
	for idx := start; idx < len(messages); idx++ {
		if messages[idx].Role != "tool" {
			// Chat Completions only pairs the uninterrupted run of tool messages that
			// follows the assistant message, so the first non-tool message ends it.
			break
		}
		answered[messages[idx].ToolCallId] = struct{}{}
	}
	return answered
}

// addTrailingReasoningPlaceholders gives the in-flight assistant turn a reasoning field.
//
// Parameters:
//   - messages: the chat history, mutated in place.
//   - stats: accumulator updated with the repairs applied.
//
// Returns: nothing; only assistant messages after the last user message are touched.
func addTrailingReasoningPlaceholders(messages []model.Message, stats *HistoryContractStats) {
	lastUserIdx := -1
	for idx := range messages {
		if messages[idx].Role == "user" {
			lastUserIdx = idx
		}
	}

	for idx := lastUserIdx + 1; idx < len(messages); idx++ {
		message := &messages[idx]
		if message.Role != "assistant" || len(message.ToolCalls) > 0 || message.ReasoningContent != nil {
			continue
		}
		placeholder := ""
		message.ReasoningContent = &placeholder
		stats.ReasoningPlaceholdersAdded++
	}
}

// isBlankMessageContent reports whether content carries nothing to send upstream.
//
// Parameters:
//   - content: the message content, which may be nil, a string, or structured parts.
//
// Returns:
//   - bool: true for nil, an empty string, or an empty parts list.
func isBlankMessageContent(content any) bool {
	switch value := content.(type) {
	case nil:
		return true
	case string:
		return value == ""
	case []any:
		return len(value) == 0
	case []model.MessageContent:
		return len(value) == 0
	default:
		return false
	}
}
