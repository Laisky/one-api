package openai

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeResponseAPIInputContentTypes_AssistantInputTextToOutputText(t *testing.T) {
	t.Parallel()

	input := ResponseAPIInput{
		map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "input_text", "text": "hello"},
			},
		},
	}

	stats, changed := NormalizeResponseAPIInputContentTypes(&input)
	require.True(t, changed)
	require.Equal(t, 1, stats.AssistantInputTextFixed)

	msg := input[0].(map[string]any)
	content := msg["content"].([]any)
	part := content[0].(map[string]any)
	require.Equal(t, "output_text", part["type"])
}

func TestNormalizeResponseAPIInputContentTypes_UserOutputTextToInputText(t *testing.T) {
	t.Parallel()

	input := ResponseAPIInput{
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "output_text", "text": "hi"},
			},
		},
	}

	stats, changed := NormalizeResponseAPIInputContentTypes(&input)
	require.True(t, changed)
	require.Equal(t, 1, stats.NonAssistantOutputTextFixed)

	msg := input[0].(map[string]any)
	content := msg["content"].([]any)
	part := content[0].(map[string]any)
	require.Equal(t, "input_text", part["type"])
}

func TestNormalizeResponseAPIInputContentTypes_NoChangeForAssistantOutputText(t *testing.T) {
	t.Parallel()

	input := ResponseAPIInput{
		map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "output_text", "text": "hello"},
				map[string]any{"type": "refusal", "refusal": "no"},
			},
		},
	}

	stats, changed := NormalizeResponseAPIInputContentTypes(&input)
	require.False(t, changed)
	require.Equal(t, 0, stats.AssistantInputTextFixed)
	require.Equal(t, 0, stats.NonAssistantOutputTextFixed)
}

func TestNormalizeResponseAPIInputEmbeddedImageDataURLs_AssistantRedactsLargePayload(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("A", responseAPIEmbeddedImageDataURLRedactionThreshold+1)
	text := "Here is the image: data:image/png;base64," + payload + ")"
	input := ResponseAPIInput{
		map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "output_text", "text": text},
			},
		},
	}

	stats, changed := NormalizeResponseAPIInputEmbeddedImageDataURLs(&input)
	require.True(t, changed)
	require.Equal(t, 1, stats.DataURLRedacted)
	require.Equal(t, len(payload), stats.DataURLRedactedBytes)

	msg := input[0].(map[string]any)
	content := msg["content"].([]any)
	part := content[0].(map[string]any)
	sanitized := part["text"].(string)
	require.Contains(t, sanitized, "data:image/png;base64,[truncated base64 len="+strconv.Itoa(len(payload))+"]")
}

func TestNormalizeResponseAPIInputEmbeddedImageDataURLs_AssistantKeepsSmallPayload(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("A", responseAPIEmbeddedImageDataURLRedactionThreshold-1)
	text := "data:image/png;base64," + payload
	input := ResponseAPIInput{
		map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "output_text", "text": text},
			},
		},
	}

	stats, changed := NormalizeResponseAPIInputEmbeddedImageDataURLs(&input)
	require.False(t, changed)
	require.Equal(t, 0, stats.DataURLRedacted)
	require.Equal(t, 0, stats.DataURLRedactedBytes)

	msg := input[0].(map[string]any)
	content := msg["content"].([]any)
	part := content[0].(map[string]any)
	require.Equal(t, text, part["text"])
}

func TestNormalizeResponseAPIInputEmbeddedImageDataURLs_UserUnchanged(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("A", responseAPIEmbeddedImageDataURLRedactionThreshold+10)
	text := "data:image/png;base64," + payload
	input := ResponseAPIInput{
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": text},
			},
		},
	}

	stats, changed := NormalizeResponseAPIInputEmbeddedImageDataURLs(&input)
	require.False(t, changed)
	require.Equal(t, 0, stats.DataURLRedacted)
	require.Equal(t, 0, stats.DataURLRedactedBytes)

	msg := input[0].(map[string]any)
	content := msg["content"].([]any)
	part := content[0].(map[string]any)
	require.Equal(t, text, part["text"])
}
