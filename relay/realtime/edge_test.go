package realtime

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestItemModelBinding tests asynchronous transcription configuration changes,
// GA and legacy acknowledgements, direct audio items and text-only user messages.
func TestItemModelBinding(t *testing.T) {
	l := NewLedger()
	frames := []string{
		`{"type":"session.created","session":{"input_audio_transcription":{"model":"whisper-1"}}}`,
		`{"type":"conversation.item.added","item":{"id":"audio","role":"user","content":[{"type":"input_audio"}]}}`,
		`{"type":"session.update","session":{"input_audio_transcription":{"model":"client-lie"}}}`,
		`{"type":"conversation.item.created","item":{"id":"text","role":"user","content":[{"type":"input_text"}]}}`,
		`{"type":"session.updated","session":{"audio":{"input":{"transcription":null}}}}`,
		`{"type":"input_audio_buffer.committed","item_id":"audio"}`,
		`{"type":"conversation.item.input_audio_transcription.completed","item_id":"audio","content_index":0,"usage":{"type":"duration","seconds":1}}`,
		`{"type":"response.done","response":{"id":"done","usage":{"input_tokens":0,"output_tokens":0}}}`,
		`{"type":"response.created","response":{"id":"done"}}`,
	}
	for _, frame := range frames {
		require.NoError(t, l.Observe([]byte(frame)))
	}
	l.Finish()
	require.Empty(t, l.Issues)
	require.Len(t, l.Records, 2)
	require.Equal(t, "whisper-1", l.Records[0].Model)
}

// TestLedgerDiagnosticEdges covers malformed acknowledgement and identity
// paths without ever accepting a client-provided model as pricing authority.
func TestLedgerDiagnosticEdges(t *testing.T) {
	bad := []string{
		`{`, `{"type":"session.updated"}`,
		`{"type":"session.updated","session":{"audio":{"input":{"transcription":3}}}}`,
		`{"type":"response.done"}`,
		`{"type":"response.done","response":{"usage":{"input_tokens":0,"output_tokens":0}}}`,
		`{"type":"response.done","response":{"id":"x"}}`,
		`{"type":"conversation.item.input_audio_transcription.completed","content_index":0,"usage":{"type":"duration","seconds":0}}`,
		`{"type":"conversation.item.input_audio_transcription.completed","item_id":"unknown","content_index":0,"usage":{"type":"duration","seconds":0}}`,
	}
	for _, frame := range bad {
		l := NewLedger()
		require.Error(t, l.Observe([]byte(frame)), "accepted %s", frame)
		require.NotEmpty(t, l.Issues)
	}
	l := NewLedger()
	require.NoError(t, l.Observe([]byte(`{"type":"input_audio_buffer.committed","item_id":"off"}`)))
	require.Error(t, l.Observe([]byte(`{"type":"conversation.item.input_audio_transcription.completed","item_id":"off","content_index":0,"usage":{"type":"duration","seconds":1}}`)), "unconfigured transcription accepted")
	l = NewLedger()
	require.NoError(t, l.Observe([]byte(`{"type":"response.done","event_id":"fallback","response":{"usage":{"input_tokens":0,"output_tokens":0}}}`)))
	require.Len(t, l.Records, 1, "event-id fallback failed")
	l.InputTokens = math.MaxInt64
	require.Error(t, l.appendRecord(Record{Tokens: Tokens{Input: 1, Text: 1}}), "input overflow accepted")
	l = NewLedger()
	l.OutputTokens = math.MaxInt64
	require.Error(t, l.appendRecord(Record{Tokens: Tokens{Input: 1, Text: 1}}), "session overflow accepted")
}

// TestCostValidationEdges checks non-finite rates and duration values, malformed
// cache partitions and the final quota's integer-safety bound.
func TestCostValidationEdges(t *testing.T) {
	for _, r := range []Rates{{Audio: -1}, {Image: math.NaN()}, {Second: math.Inf(1)}} {
		_, err := Cost(Record{}, r)
		require.ErrorIs(t, err, ErrInvalidPrice)
	}
	for _, seconds := range []float64{-1, math.NaN(), math.Inf(1)} {
		_, err := Cost(Record{Duration: true, Seconds: seconds}, Rates{Second: 1})
		require.ErrorIs(t, err, ErrInvalidUsage)
	}
	_, err := Cost(Record{Duration: true, Seconds: math.MaxFloat64}, Rates{Second: 2})
	require.ErrorIs(t, err, ErrQuotaOverflow)
	_, err = RoundQuota(1, math.MaxFloat64, 0)
	require.ErrorIs(t, err, ErrQuotaOverflow)
	_, err = RoundQuota(2, 1, math.MaxInt64)
	require.ErrorIs(t, err, ErrQuotaOverflow)
	require.ErrorIs(t, (Tokens{Input: 1, Text: 1, CachedText: 2}).Validate(), ErrInvalidUsage)
}
