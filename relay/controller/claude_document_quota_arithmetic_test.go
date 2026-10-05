package controller

import (
	"math"
	"testing"

	"github.com/Laisky/one-api/model"
	"github.com/stretchr/testify/require"
)

// TestClaudeDocumentQuotaArithmetic rejects unrepresentable paid reservations
// before changing durable balances while preserving ordinary and free pricing.
func TestClaudeDocumentQuotaArithmetic(t *testing.T) {
	const balance = int64(100_000)
	for _, test := range []struct {
		name            string
		prompt          int
		maxOutput       int
		ratio           float64
		completionRatio float64
		wantReject      bool
		wantHeld        int64
	}{
		{name: "prompt_overflow", prompt: math.MaxInt, ratio: 2, completionRatio: 1, wantReject: true},
		{name: "float_boundary", prompt: math.MaxInt, ratio: 1, completionRatio: 1, wantReject: true},
		{name: "completion_overflow", prompt: 1, maxOutput: math.MaxInt, ratio: 1, completionRatio: 2, wantReject: true},
		{name: "combined_overflow", prompt: math.MaxInt / 2, maxOutput: math.MaxInt / 2, ratio: 2, completionRatio: 1, wantReject: true},
		{name: "infinite_total", prompt: 2, ratio: math.MaxFloat64, completionRatio: 1, wantReject: true},
		{name: "nan_total", prompt: 1, ratio: math.NaN(), completionRatio: 1, wantReject: true},
		{name: "normal", prompt: 10, maxOutput: 4, ratio: 1.5, completionRatio: 2, wantHeld: 27},
		{name: "fractional_truncation", prompt: 5, ratio: 0.5, completionRatio: 1, wantHeld: 2},
		{name: "minimum_paid_unit", prompt: 1, ratio: 0.25, completionRatio: 1, wantHeld: 1},
		{name: "free", prompt: math.MaxInt, maxOutput: math.MaxInt, ratio: 0, completionRatio: 2, wantHeld: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			securityAdmissionSetup(t, balance)
			require.Equal(t, "sqlite", model.DB.Dialector.Name(), "fixture must remain local SQLite")
			c, meta := securityAdmissionContext(fallbackTokenID, false)
			var beforeToken model.Token
			require.NoError(t, model.DB.First(&beforeToken, fallbackTokenID).Error)
			var beforeUser model.User
			require.NoError(t, model.DB.First(&beforeUser, fallbackUserID).Error)
			rowsBefore := claudeDocumentFundingRows(t)

			held, apiErr := preConsumeClaudeMessagesQuota(c,
				&ClaudeMessagesRequest{Model: "fixture", MaxTokens: test.maxOutput},
				test.prompt, test.ratio, test.completionRatio, meta)
			t.Logf("held=%d rejected=%t", held, apiErr != nil)
			if test.wantReject {
				require.NotNil(t, apiErr, "unrepresentable quota must reject rather than become a one-unit hold")
				require.Zero(t, held, "rejection owns no reservation")
			} else {
				require.Nil(t, apiErr)
				require.Equal(t, test.wantHeld, held)
			}
			var afterToken model.Token
			require.NoError(t, model.DB.First(&afterToken, fallbackTokenID).Error)
			var afterUser model.User
			require.NoError(t, model.DB.First(&afterUser, fallbackUserID).Error)
			require.Equal(t, beforeUser.Quota-test.wantHeld, afterUser.Quota)
			require.Equal(t, beforeUser.UsedQuota, afterUser.UsedQuota)
			require.Equal(t, beforeToken.RemainQuota-test.wantHeld, afterToken.RemainQuota)
			require.Equal(t, beforeToken.UsedQuota+test.wantHeld, afterToken.UsedQuota)
			require.Equal(t, rowsBefore, claudeDocumentFundingRows(t), "admission must not create settlement, log, or refund rows")
		})
	}
}

// claudeDocumentFundingRows observes durable settlement tables without inventing
// a replacement reservation implementation or starting billing goroutines.
func claudeDocumentFundingRows(t *testing.T) [3]int64 {
	t.Helper()
	var rows [3]int64
	for i, table := range []any{&model.UserRequestCost{}, &model.Log{}, &model.QuotaRefund{}} {
		require.NoError(t, model.DB.Model(table).Count(&rows[i]).Error)
	}
	return rows
}
