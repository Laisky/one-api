package controller

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
)

// TestShouldSkipPreConsumedRefund verifies that forwarding marker enables
// conservative no-underbilling refund skipping.
//
// Parameters:
//   - t: Go testing handle.
func TestShouldSkipPreConsumedRefund(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.False(t, shouldSkipPreConsumedRefund(ctx))

	ctx.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
	require.True(t, shouldSkipPreConsumedRefund(ctx))
}

// TestReturnPreConsumedQuotaConservativeSkip verifies that conservative refund
// helper skips refunds once forwarding marker is set.
//
// Parameters:
//   - t: Go testing handle.
func TestReturnPreConsumedQuotaConservativeSkip(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)

	refunded := returnPreConsumedQuotaConservative(context.Background(), ctx, 123, 1, "test_skip")
	require.False(t, refunded)
}

// TestReturnPreConsumedQuotaConservativeZero verifies that zero pre-consume does
// not trigger refund operations.
//
// Parameters:
//   - t: Go testing handle.
func TestReturnPreConsumedQuotaConservativeZero(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	refunded := returnPreConsumedQuotaConservative(context.Background(), ctx, 0, 1, "test_zero")
	require.False(t, refunded)
}
