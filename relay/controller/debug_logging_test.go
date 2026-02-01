package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeRequestBodyForLoggingTruncatesStrings(t *testing.T) {
	t.Parallel()
	rawPayload := map[string]any{
		"text": strings.Repeat("A", debugLogBodyLimit+100),
		"nested": map[string]any{
			"image": strings.Repeat("B", debugLogBodyLimit+50),
		},
		"array": []any{strings.Repeat("C", debugLogBodyLimit+10)},
	}

	bytesPayload, err := json.Marshal(rawPayload)
	require.NoError(t, err)

	sanitizedPreview, truncated := sanitizeRequestBodyForLogging(bytesPayload, debugLogBodyLimit)
	require.LessOrEqual(t, len(sanitizedPreview), debugLogBodyLimit)
	require.NotEmpty(t, sanitizedPreview)

	var sanitized map[string]any
	require.NoError(t, json.Unmarshal(sanitizedPreview, &sanitized))

	text, ok := sanitized["text"].(string)
	require.True(t, ok)
	require.Equal(t, "[base64 len=4196]", text)

	nested, ok := sanitized["nested"].(map[string]any)
	require.True(t, ok)
	nestedImage, ok := nested["image"].(string)
	require.True(t, ok)
	require.Equal(t, "[base64 len=4146]", nestedImage)

	arr, ok := sanitized["array"].([]any)
	require.True(t, ok)
	require.Len(t, arr, 1)
	arrItem, ok := arr[0].(string)
	require.True(t, ok)
	require.Equal(t, "[base64 len=4106]", arrItem)
	require.False(t, truncated)
}

func TestSanitizeRequestBodyForLoggingFallback(t *testing.T) {
	t.Parallel()
	payload := strings.Repeat("X", debugLogBodyLimit+500)
	sanitized, truncated := sanitizeRequestBodyForLogging([]byte(payload), debugLogBodyLimit)
	require.True(t, truncated)
	require.Len(t, sanitized, debugLogBodyLimit)
}
