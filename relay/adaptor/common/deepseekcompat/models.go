package deepseekcompat

import "strings"

// IsFlashVisionModel reports whether modelName is an official multimodal Flash
// API name or compatibility alias. It accepts surrounding whitespace and case
// differences, but does not match Pro or third-party open-weight identifiers.
func IsFlashVisionModel(modelName string) bool {
	switch strings.ToLower(strings.TrimSpace(modelName)) {
	case "deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp":
		return true
	default:
		return false
	}
}
