package controller

import "github.com/Laisky/one-api/relay/adaptor/common/claudevision"

// claudeReservationModel returns the canonical pricing model without replacing the wire deployment.
func claudeReservationModel(request *ClaudeMessagesRequest) string {
	if request == nil {
		return ""
	}
	if request.CompatibilityModel != "" {
		return request.CompatibilityModel
	}
	return request.Model
}

// countClaudeNativeImageAllowance includes nested tool-result and file-backed images.
// It reserves a model-specific allowance; it never substitutes for billed upstream usage.
func countClaudeNativeImageAllowance(request *ClaudeMessagesRequest) int {
	if request == nil {
		return 0
	}
	count := claudevision.CountNativeImages(request.System)
	for _, message := range request.Messages {
		count += claudevision.CountNativeImages(message.Content)
	}
	return count * claudevision.Sonnet55MaxImageTokens
}
