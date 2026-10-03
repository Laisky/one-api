package jina

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/relaymode"
)

// TestReceiptQuotaCheckedArithmetic preserves exact ordinary pricing while
// retaining larger evidence without unchecked float conversion or silent caps
// at the admission token limit. A monetary overflow is explicitly reported.
func TestReceiptQuotaCheckedArithmetic(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input, output            int
		ratio, completion, group float64
		want                     int64
	}{
		{40, 0, .025, 0, 1, 1},
		{41, 0, .025, 0, 1, 2},
		{100, 20, .25, 4, 1, 45},
		{MaxBillingTokens + 1, 0, .025, 0, 1, 1677722},
		{MaxBillingTokens, MaxBillingTokens, .25, 4, 1, 83886080},
		{MaxBillingTokens + 1, 0, 1, .1, 2, 134217730},
		{math.MaxInt, math.MaxInt, 0, 4, 1, 0},
		{math.MaxInt, math.MaxInt, 1, 4, 0, 0},
		{1, 0, math.SmallestNonzeroFloat64, 0, math.SmallestNonzeroFloat64, 1},
	} {
		got, err := ReceiptQuota(tc.input, tc.output, tc.ratio, tc.completion, tc.group)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
	amount, err := ReceiptQuota(math.MaxInt, math.MaxInt, math.MaxFloat64, math.MaxFloat64, 1)
	require.Error(t, err)
	require.Equal(t, MaxReceiptQuota, amount, "unrepresentable positive work cannot become zero or wrap negative")
	for _, rate := range []float64{-1, math.NaN(), math.Inf(1)} {
		_, err := ReceiptQuota(MaxBillingTokens+1, 0, rate, 1, 1)
		require.Error(t, err)
	}
	_, err = ReceiptQuota(-1, 0, 1, 1, 1)
	require.Error(t, err)
}

// TestOversizedCountersAreEvidenceNotCertifiedUsage verifies the strict response
// decoder rejects large counters while preserving them for estimated billing.
func TestOversizedCountersAreEvidenceNotCertifiedUsage(t *testing.T) {
	t.Parallel()
	for _, count := range []int{MaxBillingTokens + 1, math.MaxInt} {
		body := []byte(fmt.Sprintf(`{"data":[],"usage":{"total_tokens":%d}}`, count))
		_, usage, err := normalizeSearchResponse(body, relaymode.Embeddings)
		require.Error(t, err)
		require.Nil(t, usage)
		evidence := partialReceiptEvidence(body)
		require.NotNil(t, evidence)
		require.Equal(t, count, evidence.TotalTokens, "discarding higher evidence could underbill")
	}
}

// FuzzReceiptQuotaBounded checks that large integer counters and decimal prices
// never yield a negative charge or escape the monetary representability ceiling.
func FuzzReceiptQuotaBounded(f *testing.F) {
	f.Add(uint64(67108865), uint64(0), uint32(25))
	f.Add(uint64(math.MaxInt64), uint64(math.MaxInt64), uint32(1000000))
	f.Fuzz(func(t *testing.T, input, output uint64, price uint32) {
		input &= uint64(math.MaxInt)
		output &= uint64(math.MaxInt)
		quota, err := ReceiptQuota(int(input), int(output), float64(price)/1000, 4, 1)
		require.GreaterOrEqual(t, quota, int64(0))
		require.LessOrEqual(t, quota, MaxReceiptQuota)
		if err != nil {
			require.Equal(t, MaxReceiptQuota, quota)
		}
		if price != 0 && (input != 0 || output != 0) {
			require.Positive(t, quota)
		}
	})
}
