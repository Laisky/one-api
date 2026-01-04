package openai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// convertResponseAPIIDToToolCall converts Response API function call IDs back to ChatCompletion format
// Removes the "fc_" and "call_" prefixes to get the original ID
func convertResponseAPIIDToToolCall(fcID, callID string) string {
	if fcID != "" && strings.HasPrefix(fcID, "fc_") {
		return strings.TrimPrefix(fcID, "fc_")
	}
	if callID != "" && strings.HasPrefix(callID, "call_") {
		return strings.TrimPrefix(callID, "call_")
	}
	// Fallback to using the ID as-is
	if fcID != "" {
		return fcID
	}
	return callID
}

// convertToolCallIDToResponseAPI converts a ChatCompletion tool call ID to Response API format
// The Response API expects IDs with "fc_" prefix for function calls and "call_" prefix for call_id
func convertToolCallIDToResponseAPI(originalID string) (fcID, callID string) {
	if originalID == "" {
		return "", ""
	}

	// If the ID already has the correct prefix, use it as-is
	if strings.HasPrefix(originalID, "fc_") {
		return originalID, strings.Replace(originalID, "fc_", "call_", 1)
	}
	if strings.HasPrefix(originalID, "call_") {
		return strings.Replace(originalID, "call_", "fc_", 1), originalID
	}

	// Otherwise, generate appropriate prefixes
	return "fc_" + originalID, "call_" + originalID
}

func stringifyFunctionCallArguments(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case nil:
		return ""
	default:
		if marshaled, err := json.Marshal(v); err == nil {
			return string(marshaled)
		}
		return fmt.Sprintf("%v", v)
	}
}

func stringifyFunctionCallOutput(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case nil:
		return ""
	default:
		if marshaled, err := json.Marshal(v); err == nil {
			return string(marshaled)
		}
		return fmt.Sprintf("%v", v)
	}
}
