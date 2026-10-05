package zhipu

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestParseOCRCounterNeverChangesValue proves an accepted counter always has
// its exact mathematical value. A long mantissa that cancels a large exponent
// must either parse to that exact value or be rejected (keeping the labelled
// allowance); it must never be silently rescaled.
func TestParseOCRCounterNeverChangesValue(t *testing.T) {
	t.Parallel()
	zeros := func(n int) string { return strings.Repeat("0", n) }

	t.Run("long_mantissa_exact_one", func(t *testing.T) {
		t.Parallel()
		// "1" + 1,000,000 zeroes + "e-1000000" is exactly 1.
		value, problem := parseOCRCounter(json.RawMessage("1"+zeros(1_000_000)+"e-1000000"), model.MaxOCRReceiptTokens)
		require.True(t, problem != "" || value == 1,
			"an exact one must parse as 1 or be rejected, got value %d problem %q", value, problem)
	})
	t.Run("long_mantissa_tenth", func(t *testing.T) {
		t.Parallel()
		// "1" + 999,999 zeroes + "e-1000000" is exactly 0.1, a fraction.
		value, problem := parseOCRCounter(json.RawMessage("1"+zeros(999_999)+"e-1000000"), model.MaxOCRReceiptTokens)
		require.NotEmpty(t, problem, "a fraction must never be accepted as an integer, got %d", value)
	})
}

// TestParseOCRCounterExactControls pins ordinary, scaled, fractional, zero,
// overflow and oversized-representation behaviour of the counter parser.
func TestParseOCRCounterExactControls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		raw     string
		value   int
		problem string
	}{
		{raw: "3", value: 3},
		{raw: "3.0", value: 3},
		{raw: "3e0", value: 3},
		{raw: "0.3e1", value: 3},
		{raw: "300e-2", value: 3},
		{raw: "1000e-3", value: 1},
		{raw: "0", value: 0},
		{raw: "0.000", value: 0},
		{raw: "0e999999999999", value: 0},
		{raw: "0e-999999999999", value: 0},
		{raw: "3.5", problem: model.OCRReceiptInvalid},
		{raw: "10000e-5", problem: model.OCRReceiptInvalid},
		{raw: "1e-65", problem: model.OCRReceiptInvalid},
		{raw: "1e-999999999999", problem: model.OCRReceiptInvalid},
		{raw: "67108865", problem: model.OCRReceiptOverflow},
		{raw: "3e9", problem: model.OCRReceiptOverflow},
		{raw: "1e65", problem: model.OCRReceiptOverflow},
		{raw: "1e999999999999", problem: model.OCRReceiptOverflow},
		{raw: "99999999999999999999", problem: model.OCRReceiptOverflow},
		{raw: "1" + strings.Repeat("0", 64) + "e-64", problem: model.OCRReceiptInvalid},
	} {
		value, problem := parseOCRCounter(json.RawMessage(tc.raw), model.MaxOCRReceiptTokens)
		require.Equal(t, tc.problem, problem, "raw %.40s", tc.raw)
		require.Equal(t, tc.value, value, "raw %.40s", tc.raw)
	}
}
