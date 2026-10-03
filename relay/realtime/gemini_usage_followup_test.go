package realtime

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGeminiAdditionalTotalsTakePrecedenceOverRemainderCoincidence verifies that
// incomplete modality details cannot contradict an explicitly additional total.
// Parameters: t owns the test. Returns: none.
func TestGeminiAdditionalTotalsTakePrecedenceOverRemainderCoincidence(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		raw  string
		want Tokens
	}{
		{
			name: "prompt remainder happens to equal additional tool count",
			raw:  `{"promptTokenCount":100,"responseTokenCount":20,"totalTokenCount":135,"thoughtsTokenCount":10,"toolUsePromptTokenCount":5,"promptTokensDetails":[{"modality":"TEXT","tokenCount":95}],"responseTokensDetails":[{"modality":"TEXT","tokenCount":20}],"toolUsePromptTokensDetails":[{"modality":"TEXT","tokenCount":5}]}`,
			want: Tokens{Input: 105, Text: 100, Unallocated: 5, Output: 30, OutputText: 30, ReasoningTokens: 10},
		},
		{
			name: "response remainder happens to equal additional thinking count",
			raw:  `{"promptTokenCount":100,"responseTokenCount":20,"totalTokenCount":130,"thoughtsTokenCount":10,"promptTokensDetails":[{"modality":"TEXT","tokenCount":100}],"responseTokensDetails":[{"modality":"AUDIO","tokenCount":10}]}`,
			want: Tokens{Input: 100, Text: 100, Output: 30, OutputText: 10, OutputAudio: 10, OutputUnallocated: 10, ReasoningTokens: 10},
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

// TestGeminiInclusiveToolSubsetRefinesUnallocatedPrompt verifies that known
// tool modalities can fit inside an incomplete prompt without adding tokens.
// Parameters: t owns the test. Returns: none.
func TestGeminiInclusiveToolSubsetRefinesUnallocatedPrompt(t *testing.T) {
	t.Parallel()
	record, err := DecodeGeminiUsage([]byte(`{"promptTokenCount":100,"responseTokenCount":20,"totalTokenCount":120,"toolUsePromptTokenCount":5,"promptTokensDetails":[{"modality":"AUDIO","tokenCount":90}],"responseTokensDetails":[{"modality":"TEXT","tokenCount":20}],"toolUsePromptTokensDetails":[{"modality":"TEXT","tokenCount":5}]}`))
	require.NoError(t, err)
	require.Equal(t, Tokens{Input: 100, Text: 5, Audio: 90, Unallocated: 5, Output: 20, OutputText: 20}, record.Tokens)
}

// TestGeminiOutputRegressionRetainsEarlierEvidence verifies that a regressing
// aggregate is rejected without losing the larger accepted receipt.
// Parameters: t owns the test. Returns: none.
func TestGeminiOutputRegressionRetainsEarlierEvidence(t *testing.T) {
	t.Parallel()
	g := NewGeminiLedger()
	require.NoError(t, g.Observe(geminiServerFixture(`{"responseTokenCount":10,"totalTokenCount":10}`, `{"modelTurn":{}}`)))
	require.Error(t, g.Observe(geminiServerFixture(`{"responseTokenCount":6,"totalTokenCount":6,"responseTokensDetails":[{"modality":"TEXT","tokenCount":1}]}`, `{}`)))
	ledger := g.Finish(false)
	require.True(t, ledger.HasUsageGap())
	require.Len(t, ledger.Records, 1)
	require.EqualValues(t, 10, ledger.OutputTokens)
}
