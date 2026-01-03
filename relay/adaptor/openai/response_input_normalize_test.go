package openai

import (
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
