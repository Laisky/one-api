package realtime

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDecodeGeminiUsageKeepsExplicitVideoOutput verifies that a Live receipt
// attributing response tokens to VIDEO (an avatar) keeps that count as a subset
// of the unattributed output, so pricing can flag it, without changing how the
// tokens are charged or which receipts are accepted. It accepts t and returns
// no values.
func TestDecodeGeminiUsageKeepsExplicitVideoOutput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                       string
		raw                        string
		output, unallocated, video int64
		outputText                 int64
	}{
		{"avatar only", `{"promptTokenCount":10,"responseTokenCount":100,"totalTokenCount":110,` +
			`"responseTokensDetails":[{"modality":"VIDEO","tokenCount":100}]}`, 100, 100, 100, 0},
		{"avatar with speech", `{"promptTokenCount":10,"responseTokenCount":100,"totalTokenCount":110,` +
			`"responseTokensDetails":[{"modality":"AUDIO","tokenCount":40},{"modality":"VIDEO","tokenCount":60}]}`, 100, 60, 60, 0},
		// Thinking beyond attributed text is reclassified from unallocated output,
		// exactly as before; the video subset shrinks to what remains.
		{"thoughts reclassified", `{"promptTokenCount":10,"responseTokenCount":100,"totalTokenCount":110,"thoughtsTokenCount":30,` +
			`"responseTokensDetails":[{"modality":"VIDEO","tokenCount":100}]}`, 100, 70, 70, 30},
		// A repeated VIDEO entry was never rejected for output; it still is not.
		{"split video entries", `{"promptTokenCount":10,"responseTokenCount":100,"totalTokenCount":110,` +
			`"responseTokensDetails":[{"modality":"VIDEO","tokenCount":30},{"modality":"VIDEO","tokenCount":20}]}`, 100, 100, 50, 0},
		// IMAGE output stays unattributed and unflagged, as before.
		{"image output", `{"promptTokenCount":10,"responseTokenCount":100,"totalTokenCount":110,` +
			`"responseTokensDetails":[{"modality":"IMAGE","tokenCount":100}]}`, 100, 100, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			record, err := DecodeGeminiUsage(json.RawMessage(tc.raw))
			require.NoError(t, err)
			require.Equal(t, tc.output, record.Tokens.Output)
			require.Equal(t, tc.unallocated, record.Tokens.OutputUnallocated)
			require.Equal(t, tc.video, record.Tokens.OutputVideo)
			require.Equal(t, tc.outputText, record.Tokens.OutputText)
		})
	}
}

// TestTokensValidateBoundsOutputVideo verifies that the video subset can never
// exceed the unattributed output it is drawn from. It accepts t and returns no
// values.
func TestTokensValidateBoundsOutputVideo(t *testing.T) {
	t.Parallel()
	valid := Tokens{Output: 10, OutputUnallocated: 10, OutputVideo: 10}
	require.NoError(t, valid.Validate())
	invalid := Tokens{Output: 10, OutputAudio: 5, OutputUnallocated: 5, OutputVideo: 6}
	require.ErrorIs(t, invalid.Validate(), ErrInvalidUsage)
	negative := Tokens{Output: 10, OutputUnallocated: 10, OutputVideo: -1}
	require.ErrorIs(t, negative.Validate(), ErrInvalidUsage)
}
