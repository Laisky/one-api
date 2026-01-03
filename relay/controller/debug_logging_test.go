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

	sanitizedValue := sanitizeJSONValue(rawPayload, debugLogBodyLimit)
	sanitizedMap, ok := sanitizedValue.(map[string]any)
	require.True(t, ok)

	truncatedText, ok := sanitizedMap["text"].(string)
	require.True(t, ok)
	require.LessOrEqual(t, len(truncatedText), debugLogBodyLimit)
	require.True(t, strings.HasSuffix(truncatedText, debugLogTruncationSuffix))

	nested, ok := sanitizedMap["nested"].(map[string]any)
	require.True(t, ok)
	nestedImage, ok := nested["image"].(string)
	require.True(t, ok)
	require.LessOrEqual(t, len(nestedImage), debugLogBodyLimit)
	require.True(t, strings.HasSuffix(nestedImage, debugLogTruncationSuffix))

	arr, ok := sanitizedMap["array"].([]any)
	require.True(t, ok)
	require.Len(t, arr, 1)
	arrItem, ok := arr[0].(string)
	require.True(t, ok)
	require.LessOrEqual(t, len(arrItem), debugLogBodyLimit)
	require.True(t, strings.HasSuffix(arrItem, debugLogTruncationSuffix))

	bytesPayload, err := json.Marshal(rawPayload)
	require.NoError(t, err)

	sanitizedPreview, truncated := sanitizeRequestBodyForLogging(bytesPayload, debugLogBodyLimit)
	require.True(t, truncated, "expected sanitization to mark payload as truncated")
	require.LessOrEqual(t, len(sanitizedPreview), debugLogBodyLimit)
	require.Contains(t, string(sanitizedPreview), debugLogTruncationSuffix)
}

func TestSanitizeRequestBodyForLoggingFallback(t *testing.T) {
	t.Parallel()
	payload := strings.Repeat("X", debugLogBodyLimit+500)
	sanitized, truncated := sanitizeRequestBodyForLogging([]byte(payload), debugLogBodyLimit)
	require.True(t, truncated)
	require.Len(t, sanitized, debugLogBodyLimit)
}
