package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseContent_ImageDetailPreserved verifies that image detail survives parsing for billing accuracy.
func TestParseContent_ImageDetailPreserved(t *testing.T) {
	t.Parallel()
	m := Message{
		Role: "user",
		Content: []any{
			map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url":    "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB",
					"detail": "low",
				},
			},
		},
	}

	parts := m.ParseContent()
	require.Len(t, parts, 1)
	require.NotNil(t, parts[0].ImageURL)
	require.Equal(t, "low", parts[0].ImageURL.Detail)
}

// TestMessageStringContent_OutputJSON ensures JSON fragments are aggregated.
func TestMessageStringContent_OutputJSON(t *testing.T) {
	t.Parallel()
	m := Message{
		Role: "assistant",
		Content: []any{
			map[string]any{
				"type":         "output_json_delta",
				"partial_json": "{\"topic\":\"AI\"",
			},
			map[string]any{
				"type":         "output_json_delta",
				"partial_json": ",\"confidence\":0.9}",
			},
		},
	}

	require.Equal(t, "{\"topic\":\"AI\",\"confidence\":0.9}", m.StringContent())

	parts := m.ParseContent()
	require.Len(t, parts, 1)
	require.NotNil(t, parts[0].Text)
	require.Equal(t, "{\"topic\":\"AI\",\"confidence\":0.9}", *parts[0].Text)
}

// TestSetReasoningContentThinking ensures thinking format only populates the thinking field.
func TestSetReasoningContentThinking(t *testing.T) {
	t.Parallel()
	msg := Message{}
	msg.Reasoning = stringPtr("legacy")

	msg.SetReasoningContent("thinking", "step by step")

	require.Nil(t, msg.Reasoning)
	require.Nil(t, msg.ReasoningContent)
	require.NotNil(t, msg.Thinking)
	require.Equal(t, "step by step", *msg.Thinking)
}

// TestSetReasoningContentReasoningContent ensures reasoning_content format clears other representations.
func TestSetReasoningContentReasoningContent(t *testing.T) {
	t.Parallel()
	msg := Message{
		Reasoning:        stringPtr("legacy"),
		Thinking:         stringPtr("chain"),
		ReasoningContent: stringPtr("structured"),
	}

	msg.SetReasoningContent("reasoning_content", "json payload")

	require.Nil(t, msg.Reasoning)
	require.Nil(t, msg.Thinking)
	require.NotNil(t, msg.ReasoningContent)
	require.Equal(t, "json payload", *msg.ReasoningContent)
}

// TestSetReasoningContentDefault ensures unspecified format defaults to reasoning only.
func TestSetReasoningContentDefault(t *testing.T) {
	t.Parallel()
	msg := Message{
		Thinking: stringPtr("chain"),
	}

	msg.SetReasoningContent("", "analysis")

	require.NotNil(t, msg.Reasoning)
	require.Equal(t, "analysis", *msg.Reasoning)
	require.Nil(t, msg.Thinking)
	require.Nil(t, msg.ReasoningContent)
}

// stringPtr returns a pointer to the provided string for test setup.
func stringPtr(s string) *string {
	return &s
}
