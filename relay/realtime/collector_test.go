package realtime

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLedgerFinalReceipts checks authoritative response partitioning and replay.
func TestLedgerFinalReceipts(t *testing.T) {
	for _, status := range []string{"completed", "cancelled", "failed", "incomplete"} {
		t.Run(status, func(t *testing.T) {
			l := NewLedger()
			created := `{"type":"response.created","response":{"id":"r","usage":{"input_tokens":0,"output_tokens":0}}}`
			receipt := fmt.Sprintf(`{"type":"response.done","response":{"id":"r","status":%q,"usage":{"input_tokens":132,"output_tokens":121,"total_tokens":253,"input_token_details":{"text_tokens":119,"audio_tokens":13,"image_tokens":0,"cached_tokens":64,"cached_tokens_details":{"text_tokens":64,"audio_tokens":0,"image_tokens":0}},"output_token_details":{"text_tokens":30,"audio_tokens":91}}}}`, status)
			for _, event := range []string{created, receipt, receipt} {
				require.NoError(t, l.Observe([]byte(event)))
			}
			l.Finish()
			require.Len(t, l.Records, 1)
			require.Empty(t, l.Issues)
			require.Equal(t, int64(132), l.InputTokens)
			require.Equal(t, int64(121), l.OutputTokens)
			want := Tokens{Input: 132, Output: 121, Text: 119, Audio: 13, CachedText: 64, OutputText: 30, OutputAudio: 91}
			require.Equal(t, want, l.Records[0].Tokens)
		})
	}
}

// TestLedgerTranscriptionModels checks asynchronous model binding, GA and beta
// acknowledgements, explicit disable, duration precision, and content dedup keys.
func TestLedgerTranscriptionModels(t *testing.T) {
	l := NewLedger()
	events := []string{
		`{"type":"session.created","session":{"audio":{"input":{"transcription":{"model":"gpt-4o-transcribe"}}}}}`,
		`{"type":"input_audio_buffer.committed","item_id":"first"}`,
		`{"type":"session.updated","session":{"audio":{"input":{"transcription":{"model":"gpt-4o-mini-transcribe"}}}}}`,
		`{"type":"input_audio_buffer.committed","item_id":"second"}`,
		`{"type":"conversation.item.input_audio_transcription.completed","item_id":"second","content_index":0,"usage":{"type":"tokens","input_tokens":17,"output_tokens":9,"input_token_details":{"audio_tokens":17}}}`,
		`{"type":"session.updated","session":{"audio":{"input":{"transcription":null}}}}`,
		`{"type":"conversation.item.input_audio_transcription.completed","item_id":"first","content_index":0,"usage":{"type":"tokens","input_tokens":17,"output_tokens":9,"total_tokens":26,"input_token_details":{"audio_tokens":17}}}`,
		`{"type":"transcription_session.updated","session":{"input_audio_transcription":{"model":"whisper-1"}}}`,
		`{"type":"input_audio_buffer.committed","item_id":"third"}`,
		`{"type":"conversation.item.input_audio_transcription.completed","item_id":"third","content_index":0,"usage":{"type":"duration","seconds":0.125}}`,
	}
	for _, event := range events {
		require.NoError(t, l.Observe([]byte(event)))
	}
	// Replayed completed events cannot charge again, even after settings change.
	require.NoError(t, l.Observe([]byte(events[4])))
	require.NoError(t, l.Observe([]byte(strings.Replace(events[9], `"content_index":0`, `"content_index":1`, 1))))
	l.Finish()
	require.Len(t, l.Records, 4)
	require.Empty(t, l.Issues)
	for i, model := range []string{"gpt-4o-mini-transcribe", "gpt-4o-transcribe", "whisper-1", "whisper-1"} {
		require.Equal(t, model, l.Records[i].Model)
	}
	require.Equal(t, 0.125, l.Records[2].Seconds)
	require.Equal(t, int64(34), l.InputTokens)
	require.Equal(t, int64(18), l.OutputTokens)
}

// TestLedgerNonReceipts checks that transport, transcript and playback events
// do not create fees, including sessions with transcription disabled.
func TestLedgerNonReceipts(t *testing.T) {
	events := []string{
		`{"type":"session.created","session":{"audio":{"input":{"transcription":null}}}}`,
		`{"type":"session.update","session":{"audio":{"input":{"transcription":{"model":"whisper-1"}}}}}`,
		`{"type":"input_audio_buffer.committed","item_id":"idle"}`,
		`{"type":"response.output_audio_transcript.done","transcript":"hello"}`,
		`{"type":"response.audio_transcript.done","transcript":"hello"}`,
		`{"type":"conversation.item.input_audio_transcription.delta","item_id":"idle","delta":"hello"}`,
		`{"type":"conversation.item.input_audio_transcription.failed","item_id":"idle"}`,
		`{"type":"conversation.item.truncated","audio_end_ms":1000}`,
		`{"type":"conversation.item.deleted","item_id":"idle"}`,
		`{"type":"rate_limits.updated","rate_limits":[]}`,
		`{"type":"response.output_audio.delta","delta":"AA=="}`,
	}
	l := NewLedger()
	for _, event := range events {
		require.NoError(t, l.Observe([]byte(event)))
	}
	l.Finish()
	require.Empty(t, l.Records)
	require.Empty(t, l.Issues)
}

// TestLedgerRejectsInvalidUsage checks malformed, inconsistent, overflowing and
// missing receipts, without poisoning final-receipt deduplication. Valid mixed
// cache ambiguity is separately covered by the reviewer regression tests.
func TestLedgerRejectsInvalidUsage(t *testing.T) {
	usages := []string{
		`null`, `{}`, `{"input_tokens":-1,"output_tokens":0}`, `{"input_tokens":1.5,"output_tokens":0}`,
		`{"input_tokens":9223372036854775808,"output_tokens":0}`,
		`{"input_tokens":9223372036854775807,"output_tokens":1}`,
		`{"input_tokens":3,"output_tokens":2,"total_tokens":9}`,
		`{"input_tokens":1,"output_tokens":0,"input_token_details":{"audio_tokens":2}}`,
		`{"input_tokens":1,"output_tokens":0,"input_token_details":{"cached_tokens":2}}`,
		`{"input_tokens":1,"output_tokens":0,"input_token_details":{"cached_tokens":0,"cached_tokens_details":{"text_tokens":1}}}`,
		`{"input_tokens":1,"output_tokens":0,"output_token_details":{"audio_tokens":1}}`,
		`{"type":"duration","seconds":1}`, `{"type":"unknown","input_tokens":0,"output_tokens":0}`,
	}
	for i, usage := range usages {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			l := NewLedger()
			event := `{"type":"response.done","response":{"id":"r","usage":` + usage + `}}`
			require.Error(t, l.Observe([]byte(event)))
			require.Empty(t, l.Records, "invalid receipt was recorded")
			valid := `{"type":"response.done","response":{"id":"r","usage":{"input_tokens":1,"output_tokens":0}}}`
			require.NoError(t, l.Observe([]byte(valid)))
			require.Len(t, l.Records, 1, "invalid receipt poisoned deduplication")
		})
	}
}

// TestLedgerCacheAndTotalNormalization checks modality cache subsets and omitted
// totals on every turn, rather than attempting a session-wide total fallback.
func TestLedgerCacheAndTotalNormalization(t *testing.T) {
	cases := []struct {
		name, details string
		want          Tokens
	}{
		{"text", `{"cached_tokens":2}`, Tokens{Input: 3, Text: 3, CachedText: 2}},
		{"audio", `{"audio_tokens":3,"cached_tokens":2}`, Tokens{Input: 3, Audio: 3, CachedAudio: 2}},
		{"image", `{"image_tokens":3,"cached_tokens":2}`, Tokens{Input: 3, Image: 3, CachedImage: 2}},
		{"mixed", `{"text_tokens":1,"audio_tokens":1,"image_tokens":1,"cached_tokens":3,"cached_tokens_details":{"text_tokens":1,"audio_tokens":1,"image_tokens":1}}`, Tokens{Input: 3, Text: 1, Audio: 1, Image: 1, CachedText: 1, CachedAudio: 1, CachedImage: 1}},
		{"full_cache_without_split", `{"text_tokens":1,"audio_tokens":1,"image_tokens":1,"cached_tokens":3}`, Tokens{Input: 3, Text: 1, Audio: 1, Image: 1, CachedText: 1, CachedAudio: 1, CachedImage: 1}},
		{"overhead", `{"text_tokens":1,"audio_tokens":1}`, Tokens{Input: 3, Text: 2, Audio: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewLedger()
			for i := 0; i < 2; i++ {
				event := fmt.Sprintf(`{"type":"response.done","response":{"id":"r%d","usage":{"input_tokens":3,"output_tokens":0,"input_token_details":%s}}}`, i, tc.details)
				require.NoError(t, l.Observe([]byte(event)))
			}
			require.Equal(t, int64(6), l.InputTokens)
			require.Len(t, l.Records, 2)
			require.Equal(t, tc.want, l.Records[0].Tokens)
			require.Empty(t, l.Issues)
		})
	}
}

// TestLedgerUnreconciledDisconnect checks accounting gaps remain visible and
// audit data does not retain a transcript or secret from a server frame.
func TestLedgerUnreconciledDisconnect(t *testing.T) {
	l := NewLedger()
	for _, e := range []string{
		`{"type":"response.created","response":{"id":"r"}}`,
		`{"type":"session.updated","session":{"input_audio_transcription":{"model":"whisper-1"}}}`,
		`{"type":"input_audio_buffer.committed","item_id":"i"}`,
		`{"type":"response.audio_transcript.done","transcript":"private-secret-text"}`,
	} {
		require.NoError(t, l.Observe([]byte(e)))
	}
	l.Finish()
	require.Len(t, l.Issues, 2)
	data, err := json.Marshal(l)
	require.NoError(t, err)
	require.NotContains(t, string(data), "private-secret-text")
	for i := 0; i < 100; i++ {
		_ = l.Observe([]byte(`{`))
	}
	require.Len(t, l.Issues, 16, "unbounded issue storage")
}

// TestCostAndRounding checks all eight token prices, independent duration,
// grouping, exact boundaries, free/idle sessions, overflow and a single rounding.
func TestCostAndRounding(t *testing.T) {
	rates := Rates{Text: 4, Audio: 32, Image: 5, CachedText: 0.4, CachedAudio: 0.4, CachedImage: 0.5, OutputText: 24, OutputAudio: 64, Second: 100}
	record := Record{Tokens: Tokens{Input: 60, Text: 10, Audio: 20, Image: 30, CachedText: 2, CachedAudio: 3, CachedImage: 4, Output: 12, OutputText: 5, OutputAudio: 7}}
	got, err := Cost(record, rates)
	require.NoError(t, err)
	want := 8.*4 + 2*.4 + 17*32 + 3*.4 + 26*5 + 4*.5 + 5*24 + 7*64
	require.InDelta(t, want, got, 1e-10)
	duration, err := Cost(Record{Duration: true, Seconds: 0.125}, rates)
	require.NoError(t, err)
	require.Equal(t, 12.5, duration)
	cases := []struct {
		name        string
		cost, group float64
		tools, want int64
	}{
		{"idle", 0, 1, 0, 0}, {"free group", 100, 0, 0, 0}, {"group once", 1.2, 2, 0, 3},
		{"tools once", 1.2, 2, 7, 10}, {"aggregate once", 0.1 + 0.1 + 0.1, 1, 0, 1},
		{"boundary", math.Nextafter(10, math.Inf(1)), 1, 0, 10}, {"fractional", 10.00001, 1, 0, 11},
		{"tiny positive", 1e-100, 1, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RoundQuota(tc.cost, tc.group, tc.tools)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1), float64(1 << 53)} {
		_, err := RoundQuota(bad, 1, 0)
		require.Error(t, err, "accepted invalid cost %g", bad)
	}
	_, err = RoundQuota(1, 1, math.MaxInt64)
	require.Error(t, err, "accepted quota overflow")
	_, err = Cost(record, Rates{Audio: -1})
	require.Error(t, err, "accepted negative price")
	_, err = Cost(Record{Duration: true, Seconds: -1}, rates)
	require.Error(t, err, "accepted negative duration")
}

// FuzzLedgerUsage ensures arbitrary server frames never panic or produce
// invalid stored token partitions, even when malformed frames are replayed.
func FuzzLedgerUsage(f *testing.F) {
	for _, seed := range []string{`{}`, `{"type":"response.done"}`, `{"type":"response.done","response":{"id":"r","usage":{"input_tokens":1,"output_tokens":0}}}`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, event string) {
		l := NewLedger()
		_ = l.Observe([]byte(event))
		_ = l.Observe([]byte(event))
		l.Finish()
		for _, record := range l.Records {
			require.NoError(t, record.Tokens.Validate())
		}
	})
}
