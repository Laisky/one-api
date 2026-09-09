package realtime

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReviewTranscriptionIndexPresence verifies a malformed completion cannot
// masquerade as index zero and suppress the real, differently priced receipt.
func TestReviewTranscriptionIndexPresence(t *testing.T) {
	for _, index := range []string{"", `,"content_index":null`, `,"content_index":-1`, `,"content_index":0.5`} {
		t.Run(fmt.Sprintf("index_%s", index), func(t *testing.T) {
			ledger := NewLedger()
			require.NoError(t, ledger.Observe([]byte(`{"type":"session.updated","session":{"input_audio_transcription":{"model":"whisper-1"}}}`)))
			require.NoError(t, ledger.Observe([]byte(`{"type":"input_audio_buffer.committed","item_id":"audio"}`)))
			invalid := `{"type":"conversation.item.input_audio_transcription.completed","item_id":"audio"` + index + `,"usage":{"type":"duration","seconds":100}}`
			require.Error(t, ledger.Observe([]byte(invalid)), "an absent/null index is not index zero")
			require.Empty(t, ledger.Records)
			valid := []byte(`{"type":"conversation.item.input_audio_transcription.completed","item_id":"audio","content_index":0,"usage":{"type":"duration","seconds":1}}`)
			require.NoError(t, ledger.Observe(valid))
			require.NoError(t, ledger.Observe(valid))
			require.Len(t, ledger.Records, 1, "rejecting the malformed event must not poison deduplication")
			require.Equal(t, float64(1), ledger.Records[0].Seconds)
		})
	}
}
