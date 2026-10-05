package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecurityOCRIntegralReceiptNotation proves receipt counters are read as
// JSON numbers: mathematically integral decimal or scientific notation is the
// same measured usage as plain integers, while fractional, negative-exponent
// fractions and out-of-range exponents keep the labelled allowance.
func TestSecurityOCRIntegralReceiptNotation(t *testing.T) {
	for _, tc := range []struct {
		name, requestID, usage, reason string
	}{
		{name: "decimal_integral", requestID: "ocr472-form-decimal",
			usage: `{"prompt_tokens":100.0,"completion_tokens":40.00,"total_tokens":140.0}`},
		{name: "scientific_integral", requestID: "ocr472-form-sci",
			usage: `{"prompt_tokens":1e2,"completion_tokens":4E+1,"total_tokens":1.4e2}`},
		{name: "scaled_fraction_integral", requestID: "ocr472-form-scaled",
			usage: `{"prompt_tokens":0.1e3,"completion_tokens":400e-1,"total_tokens":140}`},
		{name: "negative_exponent_fraction", requestID: "ocr472-form-frac",
			usage: `{"prompt_tokens":1005e-1,"completion_tokens":40}`, reason: "ocr_receipt_invalid"},
		{name: "huge_exponent", requestID: "ocr472-form-huge",
			usage: `{"prompt_tokens":1e400,"completion_tokens":40}`, reason: "ocr_receipt_overflow"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := runOCRSettlementCase(t, ocrSettlementCase{requestID: tc.requestID, groupRatio: 1, configs: ocrTestTokenTariff,
				upstreamBody: ocrTestReceipt(tc.usage), userQuota: ocrTestLargeQuota, tokenQuota: ocrTestLargeQuota})
			require.Nil(t, res.apiErr)
			if tc.reason != "" {
				requireOCRSettledOnce(t, res, ocrTestPageQuote)
				requireOCREstimate(t, res, tc.reason)
				return
			}
			requireOCRSettledOnce(t, res, 100*2+40*6)
			requireOCRMeasured(t, res)
		})
	}
}

// TestSecurityOCRTransportErrorAfterReceipt proves a complete receipt that
// arrives before a transport error (an overstated Content-Length) is settled
// as measured work while the error is still reported and replay is blocked,
// and that a genuinely truncated body keeps the labelled allowance.
func TestSecurityOCRTransportErrorAfterReceipt(t *testing.T) {
	complete := ocrTestReceipt(`{"prompt_tokens":100,"completion_tokens":40,"total_tokens":140}`)
	t.Run("complete_receipt_then_eof", func(t *testing.T) {
		res := runOCRSettlementCase(t, ocrSettlementCase{requestID: "ocr472-eof-complete", groupRatio: 1, configs: ocrTestTokenTariff,
			upstreamBody: complete, upstreamContentLength: len(complete) + 64,
			userQuota: ocrTestLargeQuota, tokenQuota: ocrTestLargeQuota})
		require.NotNil(t, res.apiErr, "the transport error must still be reported")
		requireOCRSettledOnce(t, res, 100*2+40*6)
		requireOCRMeasured(t, res)
	})
	t.Run("truncated_receipt_then_eof", func(t *testing.T) {
		truncated := complete[:len(complete)-20]
		res := runOCRSettlementCase(t, ocrSettlementCase{requestID: "ocr472-eof-truncated", groupRatio: 1, configs: ocrTestTokenTariff,
			upstreamBody: truncated, upstreamContentLength: len(complete) + 64,
			userQuota: ocrTestLargeQuota, tokenQuota: ocrTestLargeQuota})
		require.NotNil(t, res.apiErr)
		requireOCRSettledOnce(t, res, ocrTestPageQuote)
		requireOCREstimate(t, res, "ocr_receipt_unreadable")
	})
}
