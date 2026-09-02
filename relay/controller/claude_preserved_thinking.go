package controller

import (
	"encoding/json"
	"strings"
)

func isClaudePreservedThinkingBindingError(message string) bool {
	normalized := strings.ToLower(message)
	return strings.Contains(normalized, "thinking") &&
		strings.Contains(normalized, "bound to a different conversation")
}

func claudeThinkingBindingRequiresError(requestBody []byte) bool {
	var request struct {
		Thinking struct {
			BlockBinding struct {
				PrefixMismatchBehavior string `json:"prefix_mismatch_behavior"`
			} `json:"block_binding"`
		} `json:"thinking"`
	}
	if err := json.Unmarshal(requestBody, &request); err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(request.Thinking.BlockBinding.PrefixMismatchBehavior), "error")
}

func shouldRetryClaudeThinkingReplay(statusCode int, responseBody, requestBody []byte) bool {
	return shouldRetryClaudeInvalidThinkingSignature(statusCode, responseBody) &&
		!claudeThinkingBindingRequiresError(requestBody)
}
