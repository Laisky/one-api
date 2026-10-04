package openai_compatible

import (
	"encoding/json"
	"strings"
)

// estimateOriginalCompletionTokens measures an immutable response before any
// client-facing reasoning extraction or alias normalization. Identical native
// reasoning aliases, including a matching extracted block, count once. Distinct
// reasoning, visible text and tool arguments remain billable. The result is a
// local estimate, never a replacement for a nonzero authoritative receipt.
func estimateOriginalCompletionTokens(choices []TextResponseChoice, modelName string) int {
	total := 0
	for _, choice := range choices {
		message := choice.Message
		content := message.StringContent()
		total += CountTokenText(content, modelName)
		extracted, _ := ExtractThinkingContent(content)
		seen := map[string]struct{}{}
		if extracted != "" {
			seen[strings.TrimSpace(extracted)] = struct{}{}
		}
		for _, alias := range []*string{message.ReasoningContent, message.Reasoning, message.Thinking} {
			if alias == nil || *alias == "" {
				continue
			}
			key := strings.TrimSpace(*alias)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			total += CountTokenText(*alias, modelName)
		}
		for _, call := range message.ToolCalls {
			if call.Function == nil || call.Function.Arguments == nil {
				continue
			}
			if value, ok := call.Function.Arguments.(string); ok {
				total += CountTokenText(value, modelName)
			} else if value, err := json.Marshal(call.Function.Arguments); err == nil {
				total += CountTokenText(string(value), modelName)
			}
		}
	}
	return total
}
