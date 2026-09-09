package realtime

import (
	"math"
	"testing"
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
		`{"type":"conversation.item.input_audio_transcription.completed","item_id":"audio","usage":{"type":"duration","seconds":1}}`,
		`{"type":"response.done","response":{"id":"done","usage":{"input_tokens":0,"output_tokens":0}}}`,
		`{"type":"response.created","response":{"id":"done"}}`,
	}
	for _, frame := range frames {
		if err := l.Observe([]byte(frame)); err != nil {
			t.Fatal(err)
		}
	}
	l.Finish()
	if len(l.Issues) != 0 || len(l.Records) != 2 || l.Records[0].Model != "whisper-1" {
		t.Fatalf("bad ledger: %+v", l)
	}
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
		`{"type":"conversation.item.input_audio_transcription.completed","usage":{"type":"duration","seconds":0}}`,
		`{"type":"conversation.item.input_audio_transcription.completed","item_id":"unknown","usage":{"type":"duration","seconds":0}}`,
	}
	for _, frame := range bad {
		l := NewLedger()
		if l.Observe([]byte(frame)) == nil || len(l.Issues) == 0 {
			t.Errorf("accepted %s", frame)
		}
	}
	l := NewLedger()
	_ = l.Observe([]byte(`{"type":"input_audio_buffer.committed","item_id":"off"}`))
	if l.Observe([]byte(`{"type":"conversation.item.input_audio_transcription.completed","item_id":"off","usage":{"type":"duration","seconds":1}}`)) == nil {
		t.Fatal("unconfigured transcription accepted")
	}
	l = NewLedger()
	if err := l.Observe([]byte(`{"type":"response.done","event_id":"fallback","response":{"usage":{"input_tokens":0,"output_tokens":0}}}`)); err != nil {
		t.Fatal(err)
	}
	if len(l.Records) != 1 {
		t.Fatal("event-id fallback failed")
	}
	l.InputTokens = math.MaxInt64
	if l.appendRecord(Record{Tokens: Tokens{Input: 1, Text: 1}}) == nil {
		t.Fatal("input overflow accepted")
	}
	l = NewLedger()
	l.OutputTokens = math.MaxInt64
	if l.appendRecord(Record{Tokens: Tokens{Input: 1, Text: 1}}) == nil {
		t.Fatal("session overflow accepted")
	}
}

// TestCostValidationEdges checks non-finite rates and duration values, malformed
// cache partitions and the final quota's integer-safety bound.
func TestCostValidationEdges(t *testing.T) {
	for _, r := range []Rates{{Audio: -1}, {Image: math.NaN()}, {Second: math.Inf(1)}} {
		if _, err := Cost(Record{}, r); err == nil {
			t.Fatal("invalid rate accepted")
		}
	}
	for _, seconds := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := Cost(Record{Duration: true, Seconds: seconds}, Rates{Second: 1}); err == nil {
			t.Fatal("invalid duration accepted")
		}
	}
	if _, err := Cost(Record{Duration: true, Seconds: math.MaxFloat64}, Rates{Second: 2}); err == nil {
		t.Fatal("cost overflow accepted")
	}
	if _, err := RoundQuota(1, math.MaxFloat64, 0); err == nil {
		t.Fatal("quota overflow accepted")
	}
	if _, err := RoundQuota(2, 1, math.MaxInt64); err == nil {
		t.Fatal("tool addition overflow accepted")
	}
	if err := (Tokens{Input: 1, Text: 1, CachedText: 2}).Validate(); err == nil {
		t.Fatal("cache overflow accepted")
	}
}
