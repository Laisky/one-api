package realtime

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Receipts captured on 2026-09-18 from Google's Live API (v1alpha
// BidiGenerateContent) for gemini-3.8-live and gemini-3.8-live-extended-thinking.
// They are verbatim upstream frames, not hand-written fixtures: every one of them
// carries a prompt remainder that promptTokensDetails does not explain, and
// reports thinking tokens that the aggregate counts inconsistently.
const (
	liveOrdinaryTurnReceipt       = `{"promptTokenCount":548,"promptTokensDetails":[{"modality":"TEXT","tokenCount":306},{"modality":"AUDIO","tokenCount":222}],"responseTokenCount":61,"responseTokensDetails":[{"modality":"AUDIO","tokenCount":61}],"thoughtsTokenCount":100,"totalTokenCount":609}`
	liveThinkingBackgroundReceipt = `{"promptTokenCount":2947,"promptTokensDetails":[{"modality":"TEXT","tokenCount":2614},{"modality":"AUDIO","tokenCount":222}],"thoughtsTokenCount":95,"totalTokenCount":3042}`
	liveThinkingSpokenReceipt     = `{"promptTokenCount":3104,"promptTokensDetails":[{"modality":"TEXT","tokenCount":2668},{"modality":"AUDIO","tokenCount":289}],"responseTokenCount":42,"responseTokensDetails":[{"modality":"AUDIO","tokenCount":42}],"thoughtsTokenCount":49,"totalTokenCount":3146}`
)

// TestGeminiLiveCapturedReceipts decodes the receipts that real Live sessions
// actually send. Parameters: t is the test handle. Returns: none. Before this,
// every one of them was rejected, which billed a real conversation at zero and
// closed the socket on the first turn.
func TestGeminiLiveCapturedReceipts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		raw  string
		want Tokens
	}{
		{
			// Aggregate excludes thinking: total == prompt + response.
			name: "ordinary_turn",
			raw:  liveOrdinaryTurnReceipt,
			want: Tokens{Input: 548, Text: 306, Audio: 222, Unallocated: 20,
				Output: 161, OutputText: 100, OutputAudio: 61, ReasoningTokens: 100},
		},
		{
			// Aggregate includes thinking: total == prompt + thoughts, and the
			// spoken response has not started, so responseTokenCount is omitted.
			name: "thinking_background",
			raw:  liveThinkingBackgroundReceipt,
			want: Tokens{Input: 2947, Text: 2614, Audio: 222, Unallocated: 111,
				Output: 95, OutputText: 95, ReasoningTokens: 95},
		},
		{
			name: "thinking_spoken",
			raw:  liveThinkingSpokenReceipt,
			want: Tokens{Input: 3104, Text: 2668, Audio: 289, Unallocated: 147,
				Output: 91, OutputText: 49, OutputAudio: 42, ReasoningTokens: 49},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record, err := DecodeGeminiUsage([]byte(tc.raw))
			require.NoError(t, err)
			require.Equal(t, tc.want, record.Tokens)
			require.NoError(t, record.Tokens.Validate())
		})
	}
}

// TestGeminiLiveCapturedSessionBills replays a captured turn through the
// collector. Parameters: t is the test handle. Returns: none. A real turn must
// settle as one clean receipt: no usage gap, no metering error, and therefore no
// policy close of the conversation.
func TestGeminiLiveCapturedSessionBills(t *testing.T) {
	t.Parallel()
	collector := NewGeminiLedger()
	for _, frame := range [][]byte{
		[]byte(`{"sessionResumptionUpdate":{"newHandle":"6b65dd54","resumable":true}}`),
		[]byte(`{}`),
		geminiServerFixture("", `{}`),
		geminiServerFixture("", `{"modelTurn":{"parts":[{"inlineData":{"mimeType":"audio/pcm;rate=24000","data":"AAAA"}}]},"outputTranscription":{"text":"Hello, how are "}}`),
		geminiServerFixture("", `{"generationComplete":true}`),
		geminiServerFixture(liveOrdinaryTurnReceipt, `{"turnComplete":true}`),
	} {
		require.NoError(t, collector.Observe(frame))
	}
	ledger := collector.Finish(false)
	require.False(t, ledger.HasUsageGap())
	require.Empty(t, ledger.Issues)
	require.Len(t, ledger.Records, 1)
	require.EqualValues(t, 548, ledger.InputTokens)
	require.EqualValues(t, 161, ledger.OutputTokens)
}

// TestGeminiLiveUnattributedCostsCheapestModality prices an unexplained
// remainder. Parameters: t is the test handle. Returns: none. The remainder is
// charged, but only at the lowest rate the receipt could justify, so a provider
// gap can never inflate a caller's bill.
func TestGeminiLiveUnattributedCostsCheapestModality(t *testing.T) {
	t.Parallel()
	record, err := DecodeGeminiUsage([]byte(liveOrdinaryTurnReceipt))
	require.NoError(t, err)
	rates := Rates{Text: 0.75, Audio: 3, Image: 1, Video: 1, OutputText: 4.5, OutputAudio: 12}
	cost, err := Cost(record, rates)
	require.NoError(t, err)
	// 306*0.75 + 222*3 + 20*0.75 (unattributed at the text rate) + 100*4.5 + 61*12
	require.InDelta(t, 306*0.75+222*3+20*0.75+100*4.5+61*12, cost, 1e-9)

	// An unpriced modality must not drag the remainder down to free.
	cost, err = Cost(record, Rates{Text: 0, Audio: 3, Image: 1, Video: 1, OutputText: 4.5, OutputAudio: 12})
	require.NoError(t, err)
	require.InDelta(t, 222*3+20*1+100*4.5+61*12, cost, 1e-9)
}
