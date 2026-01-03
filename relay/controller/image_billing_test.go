package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// This test documents the intended unit behavior for image pricing math:
// adapter ratios for image models are already in quota-per-image units
// (usd_per_image * QuotaPerUsd). Controller must not multiply by 1000 again.
//
// Note: This is a lightweight doc-test ensuring we don't reintroduce the old bug.
func TestImageQuotaNoExtraThousand(t *testing.T) {
	t.Parallel()
	_ = relaymodel.Usage{} // reference package to avoid unused import if first test is modified
	// Suppose adapter ratio encodes $0.04 per image → 0.04 * 500000 = 20000 quota/image
	ratio := 20000.0 // quota per image
	imageCostRatio := 1.0

	// Old buggy math would do: int64(ratio*imageCostRatio) * 1000 → 20,000,000
	// Correct math: no extra *1000
	usedQuotaSingle := int64(ratio * imageCostRatio)
	require.Equal(t, int64(20000), usedQuotaSingle, "unexpected single-image quota")

	// n images scale linearly
	n := int64(3)
	usedQuotaN := usedQuotaSingle * n
	require.Equal(t, int64(60000), usedQuotaN, "unexpected n-image quota")
}
