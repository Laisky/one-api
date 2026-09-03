package controller

import "strings"

// isClaudePreservedThinkingBindingError reports whether message is Anthropic's
// preserved-thinking prefix-binding failure. The message parameter is an upstream
// error string, and the return value is true only for the recognized failure shape.
func isClaudePreservedThinkingBindingError(message string) bool {
	normalized := strings.ToLower(message)
	return strings.Contains(normalized, "thinking") &&
		strings.Contains(normalized, "bound to a different conversation")
}

// claudeThinkingBindingRequiresError reports whether request explicitly selects
// prefix_mismatch_behavior=error. The request parameter is the validated Claude
// request, and the return value is false for nil or absent binding controls.
func claudeThinkingBindingRequiresError(request *ClaudeMessagesRequest) bool {
	if request == nil || request.Thinking == nil || request.Thinking.BlockBinding == nil {
		return false
	}

	return strings.EqualFold(
		strings.TrimSpace(request.Thinking.BlockBinding.PrefixMismatchBehavior),
		"error",
	)
}

// shouldRetryClaudeThinkingReplay reports whether one-api may retry a rejected
// thinking replay after removing prior thinking blocks. The status and response
// describe the upstream failure, request carries the validated caller policy, and
// the return value is false when the caller explicitly requested strict errors.
func shouldRetryClaudeThinkingReplay(statusCode int, responseBody []byte, request *ClaudeMessagesRequest) bool {
	return shouldRetryClaudeInvalidThinkingSignature(statusCode, responseBody) &&
		!claudeThinkingBindingRequiresError(request)
}
