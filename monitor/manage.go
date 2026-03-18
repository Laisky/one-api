package monitor

import (
	"net/http"
	"strings"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/relay/model"
)

// ShouldDisableChannel determines if a channel should be automatically disabled based on the error received.
func ShouldDisableChannel(err *model.Error, statusCode int) bool {
	if !config.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil {
		return false
	}
	if statusCode == http.StatusUnauthorized {
		return true
	}

	switch err.Type {
	case model.ErrorTypeInsufficientQuota, model.ErrorTypeAuthentication, model.ErrorTypePermission, model.ErrorTypeForbidden:
		return true
	default:
		break
	}
	if err.Code == "invalid_api_key" || err.Code == "account_deactivated" {
		return true
	}

	lowerMessage := strings.ToLower(err.Message)
	if strings.Contains(lowerMessage, "your access was terminated") ||
		strings.Contains(lowerMessage, "violation of our policies") ||
		strings.Contains(lowerMessage, "your credit balance is too low") ||
		strings.Contains(lowerMessage, "organization has been disabled") ||
		strings.Contains(lowerMessage, "credit") ||
		strings.Contains(lowerMessage, "balance") ||
		strings.Contains(lowerMessage, "permission denied") ||
		strings.Contains(lowerMessage, "organization has been restricted") || // groq
		strings.Contains(lowerMessage, "api key not valid") || // gemini
		strings.Contains(lowerMessage, "api key expired") || // gemini
		strings.Contains(lowerMessage, "insufficient balance") || // Chinese: 已欠费
		strings.Contains(lowerMessage, "已欠费") {
		return true
	}
	return false
}

// ShouldEnableChannel determines if a channel should be automatically re-enabled based on the absence of errors.
func ShouldEnableChannel(err error, openAIErr *model.Error) bool {
	if !config.AutomaticEnableChannelEnabled {
		return false
	}
	if err != nil {
		return false
	}
	if openAIErr != nil {
		return false
	}
	return true
}
