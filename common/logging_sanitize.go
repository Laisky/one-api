package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// DefaultLogBodyLimit defines the maximum number of bytes to emit for log previews.
	DefaultLogBodyLimit = 4096
	// LogTruncationSuffix marks truncated log values.
	LogTruncationSuffix = "...[truncated]"
	// base64RedactionThreshold is the minimum length that triggers base64 redaction.
	base64RedactionThreshold = 256
	// base64SampleSize controls how many characters are sampled for base64 detection.
	base64SampleSize = 256
)

// SanitizePayloadForLogging returns a sanitized preview of the payload and whether it was truncated.
// Parameters: body is the raw payload; limit caps the returned preview length.
// Returns: preview bytes and a truncation flag.
func SanitizePayloadForLogging(body []byte, limit int) ([]byte, bool) {
	if limit <= 0 {
		return body, false
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		var payload any
		if err := json.Unmarshal(body, &payload); err == nil {
			sanitized := sanitizeJSONValueForLogging(payload, limit)
			if sanitizedBytes, err := json.Marshal(sanitized); err == nil {
				truncated := len(sanitizedBytes) > limit
				if truncated {
					sanitizedBytes = truncateWithSuffix(sanitizedBytes, limit)
				}
				return sanitizedBytes, truncated
			}
		}
	}

	return truncateBytes(body, limit)
}

// sanitizeJSONValueForLogging walks JSON values and truncates or redacts string leaves.
// Parameters: value is the decoded JSON node; limit is the maximum length per string.
// Returns: a sanitized JSON node suitable for logging.
func sanitizeJSONValueForLogging(value any, limit int) any {
	switch v := value.(type) {
	case map[string]any:
		sanitized := make(map[string]any, len(v))
		for key, inner := range v {
			sanitized[key] = sanitizeJSONValueForLogging(inner, limit)
		}
		return sanitized
	case []any:
		sanitized := make([]any, len(v))
		for i, inner := range v {
			sanitized[i] = sanitizeJSONValueForLogging(inner, limit)
		}
		return sanitized
	case string:
		return sanitizeStringForLogging(v, limit)
	default:
		return v
	}
}

// sanitizeStringForLogging truncates long strings and redacts base64-like payloads.
// Parameters: value is the string to sanitize; limit is the max length to preserve.
// Returns: a sanitized string safe for logging.
func sanitizeStringForLogging(value string, limit int) string {
	if value == "" {
		return value
	}
	if sanitized := sanitizeDataURL(value, limit); sanitized != "" {
		return sanitized
	}
	if isLikelyBase64(value) {
		placeholder := fmt.Sprintf("[base64 len=%d]", len(value))
		return truncateStringWithSuffix(placeholder, limit)
	}
	if len(value) <= limit {
		return value
	}
	return truncateStringWithSuffix(value, limit)
}

// sanitizeDataURL redacts base64 data URLs while preserving the prefix metadata.
// Parameters: value is the data URL string; limit is the max length to preserve.
// Returns: sanitized data URL or empty string when value is not a data URL.
func sanitizeDataURL(value string, limit int) string {
	lower := strings.ToLower(value)
	if !strings.HasPrefix(lower, "data:") {
		return ""
	}
	idx := strings.Index(lower, "base64,")
	if idx < 0 {
		return ""
	}
	header := value[:idx+len("base64,")]
	dataLen := len(value) - (idx + len("base64,"))
	placeholder := fmt.Sprintf("[truncated base64 len=%d]", dataLen)
	sanitized := header + placeholder
	return truncateStringWithSuffix(sanitized, limit)
}

// isLikelyBase64 reports whether the string looks like a raw base64 payload.
// Parameters: value is the string to inspect.
// Returns: true when the value resembles base64 data and is large enough to redact.
func isLikelyBase64(value string) bool {
	if len(value) < base64RedactionThreshold {
		return false
	}
	if strings.ContainsAny(value, " \n\r\t") {
		return false
	}
	sampleLen := base64SampleSize
	if len(value) < sampleLen {
		sampleLen = len(value)
	}
	for i := 0; i < sampleLen; i++ {
		ch := value[i]
		if (ch >= 'A' && ch <= 'Z') ||
			(ch >= 'a' && ch <= 'z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '+' || ch == '/' || ch == '=' || ch == '-' || ch == '_' {
			continue
		}
		return false
	}
	return true
}

// truncateStringWithSuffix truncates a string and appends a suffix indicating truncation.
// Parameters: value is the string to truncate; limit is the maximum length.
// Returns: the truncated string with a suffix when needed.
func truncateStringWithSuffix(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	if limit <= len(LogTruncationSuffix) {
		return LogTruncationSuffix[:limit]
	}
	headLen := limit - len(LogTruncationSuffix)
	return value[:headLen] + LogTruncationSuffix
}

// truncateWithSuffix truncates byte slices and appends the standard suffix.
// Parameters: data is the byte slice to truncate; limit is the maximum length.
// Returns: the truncated slice containing the suffix.
func truncateWithSuffix(data []byte, limit int) []byte {
	if limit <= 0 {
		return nil
	}
	suffix := []byte(LogTruncationSuffix)
	if limit <= len(suffix) {
		return append([]byte{}, suffix[:limit]...)
	}
	headLen := limit - len(suffix)
	truncated := make([]byte, 0, limit)
	truncated = append(truncated, data[:headLen]...)
	truncated = append(truncated, suffix...)
	return truncated
}

// truncateBytes truncates raw byte slices without suffixes.
// Parameters: input is the byte slice to truncate; limit is the maximum length.
// Returns: the truncated bytes and whether truncation occurred.
func truncateBytes(input []byte, limit int) ([]byte, bool) {
	if limit <= 0 || len(input) <= limit {
		return input, false
	}
	return input[:limit], true
}
