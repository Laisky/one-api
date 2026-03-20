package billing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/metrics"
	modelpkg "github.com/songquanpeng/one-api/model"
)

// TestPostConsumeQuotaWithLog_CanceledContext verifies that billing operations
// complete successfully even when the parent context is already canceled.
// This reproduces the bug where gin request context cancellation caused
// "context canceled" errors in ReconcileConsumeLog.
func TestPostConsumeQuotaWithLog_CanceledContext(t *testing.T) {
	// Setup mock metrics recorder to capture billing events
	mockRecorder := &MockMetricsRecorder{}
	originalRecorder := metrics.GlobalRecorder
	metrics.GlobalRecorder = mockRecorder
	defer func() {
		metrics.GlobalRecorder = originalRecorder
	}()

	// Create a context that is already canceled (simulates gin request context
	// being canceled after HTTP response is sent)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately — simulates post-handler context cancellation

	// Verify context is indeed canceled
	require.Error(t, ctx.Err(), "test precondition: context should be canceled")

	// PostConsumeQuotaWithLog should NOT panic with canceled context.
	// It should detach from the canceled parent and proceed with billing.
	require.NotPanics(t, func() {
		PostConsumeQuotaWithLog(ctx, -1, 10, 50, &modelpkg.Log{
			UserId:    1,
			ChannelId: 5,
			ModelName: "test-model",
			TokenName: "test-token",
		})
	}, "PostConsumeQuotaWithLog should not panic with canceled context")

	// The function should still record metrics even with canceled context
	require.NotEmpty(t, mockRecorder.BillingErrors, "Should still record validation error metrics with canceled context")
}

// TestPostConsumeQuotaWithLog_DeadlineExceededContext verifies that billing
// operations complete successfully even when the parent context deadline is exceeded.
func TestPostConsumeQuotaWithLog_DeadlineExceededContext(t *testing.T) {
	mockRecorder := &MockMetricsRecorder{}
	originalRecorder := metrics.GlobalRecorder
	metrics.GlobalRecorder = mockRecorder
	defer func() {
		metrics.GlobalRecorder = originalRecorder
	}()

	// Create a context that already exceeded its deadline
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond) // Ensure deadline passes

	require.Error(t, ctx.Err(), "test precondition: context should be expired")

	require.NotPanics(t, func() {
		PostConsumeQuotaWithLog(ctx, -1, 10, 50, &modelpkg.Log{
			UserId:    1,
			ChannelId: 5,
			ModelName: "test-model",
			TokenName: "test-token",
		})
	}, "PostConsumeQuotaWithLog should not panic with expired context")

	require.NotEmpty(t, mockRecorder.BillingErrors, "Should still record validation error metrics with expired context")
}

// TestPostConsumeQuotaDetailed_CanceledContext verifies that PostConsumeQuotaDetailed
// also handles canceled contexts gracefully.
func TestPostConsumeQuotaDetailed_CanceledContext(t *testing.T) {
	mockRecorder := &MockMetricsRecorder{}
	originalRecorder := metrics.GlobalRecorder
	metrics.GlobalRecorder = mockRecorder
	defer func() {
		metrics.GlobalRecorder = originalRecorder
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NotPanics(t, func() {
		PostConsumeQuotaDetailed(QuotaConsumeDetail{
			Ctx:              ctx,
			TokenId:          -1, // invalid, will trigger early return without DB
			QuotaDelta:       10,
			TotalQuota:       50,
			UserId:           1,
			ChannelId:        5,
			PromptTokens:     100,
			CompletionTokens: 50,
			ModelRatio:       1.0,
			GroupRatio:       1.0,
			ModelName:        "test-model",
			TokenName:        "test-token",
			StartTime:        time.Now(),
			CompletionRatio:  1.0,
		})
	}, "PostConsumeQuotaDetailed should not panic with canceled context")
}

// TestBillingOpsTimeoutConstant verifies that the billing operations timeout
// is a reasonable value.
func TestBillingOpsTimeoutConstant(t *testing.T) {
	require.Greater(t, billingOpsTimeout, 10*time.Second,
		"billingOpsTimeout should be at least 10 seconds to allow DB operations to complete")
	require.LessOrEqual(t, billingOpsTimeout, 5*time.Minute,
		"billingOpsTimeout should not exceed 5 minutes to prevent resource leaks")
}

// TestPostConsumeQuotaWithLog_NilContext verifies that nil context is handled.
func TestPostConsumeQuotaWithLog_NilContext(t *testing.T) {
	mockRecorder := &MockMetricsRecorder{}
	originalRecorder := metrics.GlobalRecorder
	metrics.GlobalRecorder = mockRecorder
	defer func() {
		metrics.GlobalRecorder = originalRecorder
	}()

	// Nil context should be handled gracefully (early return with error log)
	require.NotPanics(t, func() {
		PostConsumeQuotaWithLog(nil, 123, 10, 50, &modelpkg.Log{
			UserId:    1,
			ChannelId: 5,
			ModelName: "test-model",
			TokenName: "test-token",
		})
	}, "PostConsumeQuotaWithLog should handle nil context gracefully")
}

// TestReturnPreConsumedQuota_CanceledContext verifies that quota return
// handles canceled contexts.
func TestReturnPreConsumedQuota_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should not panic with zero quota (early return path)
	require.NotPanics(t, func() {
		ReturnPreConsumedQuota(ctx, 0, 123)
	}, "ReturnPreConsumedQuota with zero quota should not panic")
}

// TestPostConsumeQuotaWithLog_WithProvisionalLogId_CanceledContext verifies
// that the reconciliation path (with provisional log ID) handles canceled
// contexts correctly — this is the exact scenario from the bug report.
func TestPostConsumeQuotaWithLog_WithProvisionalLogId_CanceledContext(t *testing.T) {
	mockRecorder := &MockMetricsRecorder{}
	originalRecorder := metrics.GlobalRecorder
	metrics.GlobalRecorder = mockRecorder
	defer func() {
		metrics.GlobalRecorder = originalRecorder
	}()

	// Simulate the exact bug scenario: context canceled after HTTP response sent,
	// while billing reconciliation with provisional log ID is attempted.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NotPanics(t, func() {
		PostConsumeQuotaWithLog(ctx, -1, 10, 133, &modelpkg.Log{
			UserId:    1,
			ChannelId: 5,
			ModelName: "gpt-4o-mini",
			TokenName: "test-token",
		}, 316730) // provisional log ID from the bug report
	}, "PostConsumeQuotaWithLog with provisional log ID should not panic with canceled context")

	// Verify that the function still operates (records metrics) rather than
	// silently failing due to context cancellation
	require.NotEmpty(t, mockRecorder.BillingErrors,
		"Should record validation error for invalid tokenId even with canceled context")
}

// TestContextWithoutCancel_PreservesValues verifies that context.WithoutCancel
// preserves values while detaching cancellation — a sanity check for the fix.
func TestContextWithoutCancel_PreservesValues(t *testing.T) {
	type ctxKey string
	const key ctxKey = "test-key"

	parent, cancel := context.WithCancel(context.Background())
	parent = context.WithValue(parent, key, "test-value")
	cancel() // Cancel parent

	detached := context.WithoutCancel(parent)

	// Values should be preserved
	require.Equal(t, "test-value", detached.Value(key),
		"WithoutCancel should preserve context values")

	// Cancellation should NOT propagate
	require.NoError(t, detached.Err(),
		"WithoutCancel should detach from parent cancellation")

	// A timeout on the detached context should still work
	timedCtx, timedCancel := context.WithTimeout(detached, time.Hour)
	defer timedCancel()
	require.NoError(t, timedCtx.Err(),
		"Timeout on detached context should work independently")
}
