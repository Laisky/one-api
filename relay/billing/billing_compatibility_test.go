package billing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	modelpkg "github.com/Laisky/one-api/model"
)

// TestOriginModelNamePreserved verifies that OriginModelName is correctly stored in the Log entry
// when PostConsumeQuotaDetailed is called. This is critical for model mapping transparency.
func TestOriginModelNamePreserved(t *testing.T) {
	ctx := context.Background()
	validTime := time.Unix(1_700_000_000, 0).UTC()
	logChan := make(chan *modelpkg.Log, 1)
	originalPostConsume := postConsumeQuotaWithLogFn
	t.Cleanup(func() {
		postConsumeQuotaWithLogFn = originalPostConsume
	})
	postConsumeQuotaWithLogFn = func(ctx context.Context, tokenId int, quotaDelta int64, totalQuota int64, logEntry *modelpkg.Log, provisionalLogId ...int) {
		logChan <- logEntry
	}

	detail := QuotaConsumeDetail{
		Ctx:                ctx,
		TokenId:            123,
		QuotaDelta:         10,
		TotalQuota:         50,
		UserId:             1,
		ChannelId:          5,
		PromptTokens:       100,
		CompletionTokens:   50,
		ModelRatio:         1.0,
		GroupRatio:         1.0,
		ModelName:          "gpt-4",
		OriginModelName:    "my-model",
		TokenName:          "test-token",
		IsStream:           false,
		StartTime:          validTime,
		SystemPromptReset:  false,
		CompletionRatio:    1.0,
		ToolsCost:          0,
		CachedPromptTokens: 0,
	}

	PostConsumeQuotaDetailed(detail)

	select {
	case entry := <-logChan:
		require.Equal(t, "gpt-4", entry.ModelName)
		require.Equal(t, "my-model", entry.OriginModelName)
		require.Equal(t, 100, entry.PromptTokens)
		require.Equal(t, 50, entry.CompletionTokens)
		require.NotEmpty(t, entry.Content)
	case <-time.After(time.Second):
		require.Fail(t, "expected PostConsumeQuotaDetailed to emit a log entry")
	}
}

// This file previously also held TestBackwardCompatibility (its only call was
// commented out; the body was a t.Log), TestInputValidation (each helper was
// `defer recover(); call(); return true`, so it could distinguish nothing but
// panic from no-panic, and its shouldFail field was inverted relative to its
// name), and TestBillingConsistency (it recomputed the quota formula inside the
// test and compared the result to itself). None executed the billing arithmetic.
// Input validation is now covered for real in zero_quota_fix_test.go; the
// arithmetic as it reaches the persisted log is covered below.

// TestPostConsumeQuotaDetailedRecordsBilledAmounts pins what PostConsumeQuotaDetailed
// actually writes into the consume log, using the same seam TestOriginModelNamePreserved
// uses, so the assertions observe production behavior rather than a restatement of it.
func TestPostConsumeQuotaDetailedRecordsBilledAmounts(t *testing.T) {
	for _, tc := range []struct {
		name             string
		promptTokens     int
		completionTokens int
		totalQuota       int64
		toolsCost        int64
	}{
		{name: "ordinary turn", promptTokens: 100, completionTokens: 50, totalQuota: 150},
		{name: "turn with a tool charge", promptTokens: 100, completionTokens: 50, totalQuota: 175, toolsCost: 25},
		{name: "free turn still recorded", promptTokens: 10, completionTokens: 0, totalQuota: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logChan := make(chan *modelpkg.Log, 1)
			original := postConsumeQuotaWithLogFn
			t.Cleanup(func() { postConsumeQuotaWithLogFn = original })
			postConsumeQuotaWithLogFn = func(ctx context.Context, tokenId int, quotaDelta int64, totalQuota int64, logEntry *modelpkg.Log, provisionalLogId ...int) {
				logEntry.Quota = int(totalQuota)
				logChan <- logEntry
			}

			PostConsumeQuotaDetailed(QuotaConsumeDetail{
				Ctx:              context.Background(),
				TokenId:          123,
				QuotaDelta:       tc.totalQuota,
				TotalQuota:       tc.totalQuota,
				UserId:           1,
				ChannelId:        5,
				PromptTokens:     tc.promptTokens,
				CompletionTokens: tc.completionTokens,
				ModelRatio:       1.0,
				GroupRatio:       1.0,
				CompletionRatio:  1.0,
				ToolsCost:        tc.toolsCost,
				ModelName:        "gpt-4",
				TokenName:        "test-token",
				StartTime:        time.Unix(1_700_000_000, 0).UTC(),
			})

			select {
			case entry := <-logChan:
				require.Equal(t, tc.promptTokens, entry.PromptTokens)
				require.Equal(t, tc.completionTokens, entry.CompletionTokens)
				require.Equal(t, int(tc.totalQuota), entry.Quota,
					"the persisted log must record the quota that was billed")
				require.NotEmpty(t, entry.Content, "the log must describe how the charge was derived")
			case <-time.After(time.Second):
				require.Fail(t, "PostConsumeQuotaDetailed must emit a log entry for every request, including free ones")
			}
		})
	}
}
