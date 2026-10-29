package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMalformedInputAudioDoesNotPanic covers malformed, user-controlled
// multimodal parts before any provider-specific admission layer sees them.
func TestMalformedInputAudioDoesNotPanic(t *testing.T) {
	for _, tc := range []struct {
		name  string
		audio map[string]any
	}{
		{"missing_data", map[string]any{"format": "wav"}},
		{"missing_format", map[string]any{"data": "YQ=="}},
		{"non_string_data", map[string]any{"data": 42, "format": "wav"}},
		{"non_string_format", map[string]any{"data": "YQ==", "format": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := Message{Role: "user", Content: []any{
				map[string]any{"type": ContentTypeInputAudio, "input_audio": tc.audio},
			}}
			var got []MessageContent
			require.NotPanics(t, func() { got = msg.ParseContent() })
			require.Empty(t, got, "malformed input audio must not be decoded into a valid part")
		})
	}
	// Preserve the established behavior for valid audio on other providers.
	valid := Message{Role: "user", Content: []any{
		map[string]any{"type": ContentTypeInputAudio,
			"input_audio": map[string]any{"data": "YQ==", "format": "wav"}},
	}}
	parts := valid.ParseContent()
	require.Len(t, parts, 1)
	require.NotNil(t, parts[0].InputAudio)
	require.Equal(t, "YQ==", parts[0].InputAudio.Data)
	require.Equal(t, "wav", parts[0].InputAudio.Format)
}
