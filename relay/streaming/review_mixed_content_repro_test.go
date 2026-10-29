package streaming

import (
	"strings"
	"testing"

	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestMixedTypedContentObservationCountsOnlyText verifies pre-receipt text accounting for structured content.
func TestMixedTypedContentObservationCountsOnlyText(t *testing.T) {
	text := "hello"
	image := "data:image/png;base64," + strings.Repeat("A", 2000)
	audio := strings.Repeat("B", 2000)
	for _, tc := range []struct {
		name    string
		content any
	}{
		{"typed", []relaymodel.MessageContent{{Type: relaymodel.ContentTypeText, Text: &text}, {Type: relaymodel.ContentTypeImageURL, ImageURL: &relaymodel.ImageURL{Url: image}}, {Type: relaymodel.ContentTypeInputAudio, InputAudio: &relaymodel.InputAudio{Data: audio, Format: "wav"}}}},
		{"wire", []any{map[string]any{"type": "text", "text": text}, map[string]any{"type": "image_url", "image_url": map[string]any{"url": image}}, map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": audio, "format": "wav"}}}},
		{"string", text},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracker := NewQuotaTracker(QuotaTrackerParams{ModelName: "gpt-4", ModelRatio: 1, GroupRatio: 1, PreConsumedQuota: 1000000, TokenCounter: func(s, _ string) int { return len(s) }})
			require.NoError(t, tracker.ObserveMessages([]relaymodel.Message{{Content: tc.content}}, nil))
			require.Equal(t, len(text), tracker.UsageSnapshot().CompletionTokens)
		})
	}
}
