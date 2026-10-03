package common

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSanitizePayloadForLogging_DataURL verifies base64 data URLs are redacted in log previews.
func TestSanitizePayloadForLogging_DataURL(t *testing.T) {
	base64Data := strings.Repeat("A", 1024)
	payload := map[string]any{
		"image_url": map[string]any{
			"url": "data:image/png;base64," + base64Data,
		},
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	preview, truncated := SanitizePayloadForLogging(body, 512)
	previewText := string(preview)

	require.Contains(t, previewText, "data:image/png;base64,[truncated base64 len=1024]")
	require.NotContains(t, previewText, base64Data)
	require.False(t, truncated)
}

// TestSanitizeURLForLogging_RedactsSensitiveQuery verifies secret-like query values are redacted from logged URLs.
func TestSanitizeURLForLogging_RedactsSensitiveQuery(t *testing.T) {
	rawURL := "/api/user/register?turnstile=secret-token&page=1&api_key=secret-key"

	sanitized := SanitizeURLForLogging(rawURL)

	require.Contains(t, sanitized, "page=1")
	require.Contains(t, sanitized, "turnstile=%5Bredacted%5D")
	require.Contains(t, sanitized, "api_key=%5Bredacted%5D")
	require.NotContains(t, sanitized, "secret-token")
	require.NotContains(t, sanitized, "secret-key")
}

// TestSanitizePayloadForLogging_Base64String verifies raw base64 strings are redacted.
func TestSanitizePayloadForLogging_Base64String(t *testing.T) {
	base64Data := strings.Repeat("B", 1024)
	payload := map[string]any{
		"audio": base64Data,
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	preview, truncated := SanitizePayloadForLogging(body, 512)
	previewText := string(preview)

	require.Contains(t, previewText, "[base64 len=1024]")
	require.NotContains(t, previewText, base64Data)
	require.False(t, truncated)
}
