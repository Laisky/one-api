package controller

import (
	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// extractConvertedClaudeSSEUsage shares the bounded response observer used by
// proxy relays. Protocol conversion must not discard message_start input usage,
// nested Responses receipts, OpenAI counters, cache details, or tool arguments.
// Estimated receipts remain labelled for reservation-aware quota settlement.
func extractConvertedClaudeSSEUsage(body []byte, promptTokens int, modelName string) *relaymodel.Usage {
	return relaymodel.ParseResponseUsage(body, promptTokens, func(text string) int {
		return openai.CountTokenText(text, modelName)
	})
}
