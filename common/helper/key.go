package helper

const (
	// RequestIdKey stores the gin context key used to persist the current request identifier.
	RequestIdKey = "X-Oneapi-Request-Id"
)

// MaskAPIKey returns a masked version of an API key for safe logging.
// It shows the first 6 characters and last 4 characters, with "..." in between.
// For short keys (less than 12 chars), it returns "***" to avoid exposing too much.
// This function should be used when logging API key information for debugging
// without exposing the complete key.
func MaskAPIKey(key string) string {
	if len(key) < 12 {
		return "***"
	}
	return key[:6] + "..." + key[len(key)-4:]
}
