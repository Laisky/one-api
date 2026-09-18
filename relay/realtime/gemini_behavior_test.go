package realtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// geminiReceiptFixture returns a provider-shaped receipt. Parameters: input and
// output are text counts. Returns: a usageMetadata JSON object for tests.
func geminiReceiptFixture(input, output int64) string {
	return fmt.Sprintf(`{"promptTokenCount":%d,"responseTokenCount":%d,"totalTokenCount":%d,"promptTokensDetails":[{"modality":"TEXT","tokenCount":%d}],"responseTokensDetails":[{"modality":"TEXT","tokenCount":%d}]}`, input, output, input+output, input, output)
}

// geminiServerFixture wraps receipt data in a native server frame. Parameters:
// receipt is optional usage JSON, content is serverContent JSON. Returns: JSON.
func geminiServerFixture(receipt, content string) []byte {
	if receipt == "" {
		return []byte(`{"serverContent":` + content + `}`)
	}
	return []byte(`{"usageMetadata":` + receipt + `,"serverContent":` + content + `}`)
}

// TestGeminiUsagePartitionsAndTranscripts verifies billing partitions without
// estimating transcript length. Parameters: t is the test handle. Returns: none.
func TestGeminiUsagePartitionsAndTranscripts(t *testing.T) {
	t.Parallel()
	raw := `{"promptTokenCount":100,"responseTokenCount":50,"totalTokenCount":150,"thoughtsTokenCount":10,"promptTokensDetails":[{"modality":"TEXT","tokenCount":10},{"modality":"AUDIO","tokenCount":20},{"modality":"IMAGE","tokenCount":30},{"modality":"VIDEO","tokenCount":40}],"responseTokensDetails":[{"modality":"TEXT","tokenCount":20},{"modality":"AUDIO","tokenCount":30}]}`
	record, err := DecodeGeminiUsage([]byte(raw))
	require.NoError(t, err)
	require.Equal(t, Tokens{Input: 100, Output: 50, Text: 10, Audio: 20, Image: 30, Video: 40, OutputText: 20, OutputAudio: 30, ReasoningTokens: 10}, record.Tokens)
	collector := NewGeminiLedger()
	for _, content := range []string{`{"inputTranscription":{"text":"PRIVATE transcript"}}`, `{"outputTranscription":{"text":"PRIVATE transcript"}}`, `{"outputTranscription":{"text":"PRIVATE transcript"}}`} {
		require.NoError(t, collector.Observe(geminiServerFixture("", content)))
	}
	require.NoError(t, collector.Observe(geminiServerFixture(raw, `{"turnComplete":true}`)))
	ledger := collector.Finish(false)
	require.False(t, ledger.HasUsageGap())
	require.Len(t, ledger.Records, 1)
	require.Equal(t, record, ledger.Records[0])
	encoded, err := json.Marshal(ledger.Audit())
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "PRIVATE")
}

// TestGeminiReceiptValidation covers protobuf zero omissions, malformed counts,
// unknown modalities and ambiguous totals. Parameters: t is a test. Returns: none.
func TestGeminiReceiptValidation(t *testing.T) {
	t.Parallel()
	valid := geminiReceiptFixture(100, 20)
	tests := []struct {
		name, raw string
		valid     bool
	}{
		{"explicit_zero", `{"totalTokenCount":0}`, true},
		{"input_only", `{"promptTokenCount":10,"totalTokenCount":10,"promptTokensDetails":[{"modality":"AUDIO","tokenCount":10}]}`, true},
		{"output_only", `{"responseTokenCount":10,"totalTokenCount":10,"responseTokensDetails":[{"modality":"AUDIO","tokenCount":10}]}`, true},
		{"normal", valid, true},
		{"empty", `{}`, false}, {"null", `null`, false}, {"malformed", `{`, false},
		{"missing_modality", `{"promptTokenCount":10,"totalTokenCount":10}`, true},
		{"negative", strings.Replace(valid, `"promptTokenCount":100`, `"promptTokenCount":-100`, 1), false},
		{"fractional", strings.Replace(valid, `"promptTokenCount":100`, `"promptTokenCount":1.5`, 1), false},
		{"string_count", strings.Replace(valid, `"promptTokenCount":100`, `"promptTokenCount":"100"`, 1), false},
		{"overflow", strings.Replace(valid, `"promptTokenCount":100`, `"promptTokenCount":2147483648`, 1), false},
		{"unknown_modality", strings.Replace(valid, `"TEXT"`, `"FUTURE_MODALITY"`, 1), true},
		{"modality_exceeds_parent", `{"promptTokenCount":10,"totalTokenCount":10,"promptTokensDetails":[{"modality":"TEXT","tokenCount":11}]}`, false},
		{"modality_sum_exceeds_parent", `{"promptTokenCount":10,"totalTokenCount":10,"promptTokensDetails":[{"modality":"TEXT","tokenCount":6},{"modality":"AUDIO","tokenCount":5}]}`, false},
		{"inconsistent_total", strings.Replace(valid, `"totalTokenCount":120`, `"totalTokenCount":121`, 1), false},
		{"duplicate_key", strings.Replace(valid, `"promptTokenCount":100`, `"promptTokenCount":100,"promptTokenCount":0`, 1), false},
		{"duplicate_modality", `{"promptTokenCount":2,"totalTokenCount":2,"promptTokensDetails":[{"modality":"TEXT","tokenCount":1},{"modality":"TEXT","tokenCount":1}]}`, false},
		{"unknown_cache_discount", `{"promptTokenCount":10,"totalTokenCount":10,"cachedContentTokenCount":5,"promptTokensDetails":[{"modality":"AUDIO","tokenCount":10}],"cacheTokensDetails":[{"modality":"AUDIO","tokenCount":5}]}`, false},
		{"explicit_zero_cache", `{"totalTokenCount":0,"cacheTokensDetails":[{"modality":"TEXT","tokenCount":0}]}`, true},
		{"trailing_json", valid + `{}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := DecodeGeminiUsage([]byte(tc.raw))
			if tc.valid {
				require.NoError(t, err)
				require.NoError(t, r.Tokens.Validate())
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestGeminiThinkingAndToolConservation ensures additional and inclusive counters
// are never billed twice. Parameters: t is the test handle. Returns: none.
func TestGeminiThinkingAndToolConservation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		total           int
		wantIn, wantOut int64
	}{
		{"inclusive", 120, 100, 20}, {"additional", 135, 105, 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := strings.Replace(geminiReceiptFixture(100, 20), `"totalTokenCount":120`, fmt.Sprintf(`"totalTokenCount":%d,"thoughtsTokenCount":10,"toolUsePromptTokenCount":5,"toolUsePromptTokensDetails":[{"modality":"TEXT","tokenCount":5}]`, tc.total), 1)
			r, err := DecodeGeminiUsage([]byte(raw))
			require.NoError(t, err)
			require.Equal(t, tc.wantIn, r.Tokens.Input)
			require.Equal(t, tc.wantOut, r.Tokens.Output)
			require.EqualValues(t, 10, r.Tokens.ReasoningTokens)
		})
	}
}

// TestGeminiTurnAccountingBehavior verifies repeated context, compression and
// turn-scoped snapshot handling. Parameters: t is the test handle. Returns: none.
func TestGeminiTurnAccountingBehavior(t *testing.T) {
	t.Parallel()
	g := NewGeminiLedger()
	for _, pair := range [][2]int64{{100, 20}, {100, 20}, {50, 10}} {
		require.NoError(t, g.Observe(geminiServerFixture("", `{"modelTurn":{"parts":[{"text":"payload"}]}}`)))
		raw := geminiReceiptFixture(pair[0], pair[1])
		require.NoError(t, g.Observe(geminiServerFixture(raw, `{}`)))
		require.NoError(t, g.Observe(geminiServerFixture(raw, `{}`)))
		require.NoError(t, g.Observe(geminiServerFixture("", `{"turnComplete":true}`)))
	}
	l := g.Finish(false)
	require.Len(t, l.Records, 3)
	require.EqualValues(t, 250, l.InputTokens)
	require.EqualValues(t, 50, l.OutputTokens)
	require.False(t, l.HasUsageGap())
	require.Same(t, l, g.Finish(false))
	require.Len(t, l.Records, 3)
}

// TestGeminiLatePartialAndIdleUsage covers late receipts, interruptions, active
// disconnects and idle refunds. Parameters: t is the test handle. Returns: none.
func TestGeminiLatePartialAndIdleUsage(t *testing.T) {
	t.Parallel()
	t.Run("late_receipt", func(t *testing.T) {
		g := NewGeminiLedger()
		require.NoError(t, g.Observe(geminiServerFixture("", `{"modelTurn":{},"turnComplete":true}`)))
		require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(10, 20), `{}`)))
		l := g.Finish(false)
		require.False(t, l.HasUsageGap())
		require.Len(t, l.Records, 1)
	})
	t.Run("partial_interrupt_disconnect", func(t *testing.T) {
		g := NewGeminiLedger()
		require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(10, 20), `{"modelTurn":{},"interrupted":true}`)))
		l := g.Finish(true)
		require.True(t, l.HasUsageGap())
		require.Len(t, l.Records, 1)
		require.EqualValues(t, 20, l.OutputTokens)
	})
	t.Run("interrupted_final_receipt", func(t *testing.T) {
		g := NewGeminiLedger()
		require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(10, 20), `{"modelTurn":{},"interrupted":true,"turnComplete":true}`)))
		require.False(t, g.Finish(false).HasUsageGap())
	})
	t.Run("idle", func(t *testing.T) {
		g := NewGeminiLedger()
		require.NoError(t, g.Observe([]byte(`{"setupComplete":{}}`)))
		l := g.Finish(false)
		require.False(t, l.HasUsageGap())
		require.Empty(t, l.Records)
	})
	t.Run("missing_input_receipt", func(t *testing.T) { require.True(t, NewGeminiLedger().Finish(true).HasUsageGap()) })
	t.Run("proactive_equal_input_only_turns", func(t *testing.T) {
		g := NewGeminiLedger()
		for range 2 {
			require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(10, 0), `{"turnComplete":true}`)))
		}
		require.Len(t, g.Finish(false).Records, 2)
	})
	t.Run("background_thinking", func(t *testing.T) {
		g := NewGeminiLedger()
		require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(10, 2), `{"modelTurn":{},"turnComplete":true,"interactionStatus":"IN_PROGRESS"}`)))
		require.Empty(t, g.Ledger.Records)
		require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(10, 20), `{"turnComplete":true,"interactionStatus":"IDLE"}`)))
		l := g.Finish(false)
		require.False(t, l.HasUsageGap())
		require.Len(t, l.Records, 1)
		require.EqualValues(t, 20, l.OutputTokens)
	})
	t.Run("malformed_then_corrected", func(t *testing.T) {
		g := NewGeminiLedger()
		require.Error(t, g.Observe(geminiServerFixture(`{}`, `{"modelTurn":{}}`)))
		require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(10, 20), `{"turnComplete":true}`)))
		require.False(t, g.Finish(false).HasUsageGap())
		require.NotEmpty(t, g.Ledger.Issues)
	})
	t.Run("regression_within_turn", func(t *testing.T) {
		g := NewGeminiLedger()
		require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(10, 20), `{"modelTurn":{}}`)))
		require.Error(t, g.Observe(geminiServerFixture(geminiReceiptFixture(10, 10), `{}`)))
		l := g.Finish(false)
		require.True(t, l.HasUsageGap())
		require.EqualValues(t, 20, l.OutputTokens)
	})
}

// TestGeminiLedgerCapacityPreservesBoundary verifies bounded memory without
// silently evicting chargeable records. Parameters: t is a test. Returns: none.
func TestGeminiLedgerCapacityPreservesBoundary(t *testing.T) {
	t.Parallel()
	g := NewGeminiLedger()
	for i := 0; i < MaxRecords; i++ {
		err := g.Observe(geminiServerFixture(geminiReceiptFixture(1, 1), `{"modelTurn":{},"turnComplete":true}`))
		if i == MaxRecords-1 {
			require.ErrorIs(t, err, ErrLedgerLimit)
		} else {
			require.NoError(t, err)
		}
	}
	require.Len(t, g.Finish(false).Records, MaxRecords)
}

// TestGeminiInclusiveCounterPartitions resolves only explicitly labeled missing
// thinking/tool subsets. Parameters: t is a test. Returns: none.
func TestGeminiInclusiveCounterPartitions(t *testing.T) {
	t.Parallel()
	raw := `{"promptTokenCount":100,"responseTokenCount":20,"totalTokenCount":120,"thoughtsTokenCount":10,"toolUsePromptTokenCount":5,"promptTokensDetails":[{"modality":"TEXT","tokenCount":95}],"toolUsePromptTokensDetails":[{"modality":"TEXT","tokenCount":5}],"responseTokensDetails":[{"modality":"AUDIO","tokenCount":10}]}`
	record, err := DecodeGeminiUsage([]byte(raw))
	require.NoError(t, err)
	require.Equal(t, Tokens{Input: 100, Text: 100, Output: 20, OutputText: 10, OutputAudio: 10, ReasoningTokens: 10}, record.Tokens)
	_, err = DecodeGeminiUsage([]byte(strings.Replace(raw, `"totalTokenCount":120`, `"totalTokenCount":135`, 1)))
	require.Error(t, err, "do not add inclusive subsets twice")
}

// TestGeminiEqualStandaloneReceiptsNeedTurnScope prevents a legitimate equal-cost
// next turn from disappearing. Parameters: t is a test. Returns: none.
func TestGeminiEqualStandaloneReceiptsNeedTurnScope(t *testing.T) {
	t.Parallel()
	g := NewGeminiLedger()
	for range 2 {
		require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(10, 0), `{}`)))
		require.NoError(t, g.Observe(geminiServerFixture("", `{"turnComplete":true}`)))
	}
	require.EqualValues(t, 20, g.Ledger.InputTokens)
	require.Len(t, g.Ledger.Records, 2)
	// An orphan repeated snapshot is not proof of a third billable turn.
	require.NoError(t, g.Observe(geminiServerFixture(geminiReceiptFixture(10, 0), `{}`)))
	l := g.Finish(false)
	require.Len(t, l.Records, 2)
	require.True(t, l.HasUsageGap())
}
