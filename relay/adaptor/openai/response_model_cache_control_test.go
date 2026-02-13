package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

func TestConvertChatCompletionToResponseAPI_StripsCacheControl(t *testing.T) {
	request := &model.GeneralOpenAIRequest{
		Model: "gpt-4.1-nano",
		Messages: []model.Message{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type": "text",
						"text": "hello",
						"foo":  "bar",
						"cache_control": map[string]any{
							"type": "ephemeral",
						},
					},
					map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": "https://example.com/image.png",
						},
						"detail":  "high",
						"file_id": "file-abc",
						"noise":   true,
						"cache_control": map[string]any{
							"type": "ephemeral",
						},
					},
					map[string]any{
						"type": "unknown_custom",
						"text": "fallback-to-text",
						"x":    "y",
					},
				},
			},
		},
	}

	responseAPI := ConvertChatCompletionToResponseAPI(request)
	require.Len(t, responseAPI.Input, 1)

	input, ok := responseAPI.Input[0].(map[string]any)
	require.True(t, ok)
	content, ok := input["content"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, content, 3)
	require.Equal(t, "input_text", content[0]["type"])
	require.Equal(t, "input_image", content[1]["type"])
	require.Equal(t, "input_text", content[2]["type"])
	require.Equal(t, "fallback-to-text", content[2]["text"])
	require.Equal(t, "high", content[1]["detail"])
	require.Equal(t, "file-abc", content[1]["file_id"])
	_, hasFirstCacheControl := content[0]["cache_control"]
	_, hasSecondCacheControl := content[1]["cache_control"]
	_, hasFirstUnknownField := content[0]["foo"]
	_, hasSecondUnknownField := content[1]["noise"]
	require.False(t, hasFirstCacheControl)
	require.False(t, hasSecondCacheControl)
	require.False(t, hasFirstUnknownField)
	require.False(t, hasSecondUnknownField)
}

func TestCountResponseAPIUnsupportedContentFields(t *testing.T) {
	messages := []model.Message{
		{
			Role: "user",
			Content: []any{
				map[string]any{"type": "text", "text": "a", "cache_control": map[string]any{"type": "ephemeral"}, "foo": "bar"},
				map[string]any{"type": "text", "text": "b"},
				"unexpected",
			},
		},
		{
			Role: "assistant",
			Content: []any{
				map[string]any{"type": "text", "text": "c", "cache_control": map[string]any{"type": "ephemeral"}},
				map[string]any{"type": "unknown_custom", "text": "d", "cache_control": map[string]any{"type": "ephemeral"}},
			},
		},
		{
			Role:    "user",
			Content: "plain-text-content",
		},
	}

	require.Equal(t, 4, countResponseAPIUnsupportedContentFields(messages))
}
