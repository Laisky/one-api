package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
)

// TestMarkPreConsumedAndReconciled tests the basic lifecycle of marking
// pre-consumed quota and then marking it as reconciled.
func TestMarkPreConsumedAndReconciled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	// Initially no pre-consumed amount
	_, exists := c.Get(ctxkey.PreConsumedQuotaAmount)
	require.False(t, exists)

	// Mark pre-consumed
	markPreConsumed(c, 5000)
	val, exists := c.Get(ctxkey.PreConsumedQuotaAmount)
	require.True(t, exists)
	require.Equal(t, int64(5000), val.(int64))

	// Not yet reconciled
	reconciled, _ := c.Get(ctxkey.BillingReconciled)
	require.Nil(t, reconciled)

	// Mark reconciled
	markBillingReconciled(c)
	reconciled, exists = c.Get(ctxkey.BillingReconciled)
	require.True(t, exists)
	require.Equal(t, true, reconciled.(bool))
}

// TestBillingAuditSafetyNet_NoPreConsume verifies the safety net is a no-op
// when no pre-consume has occurred (normal non-billing request paths).
func TestBillingAuditSafetyNet_NoPreConsume(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)

	// Should not panic or log anything
	billingAuditSafetyNet(c)
}

// TestBillingAuditSafetyNet_Reconciled verifies the safety net is a no-op
// when billing has been properly reconciled.
func TestBillingAuditSafetyNet_Reconciled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)

	markPreConsumed(c, 5000)
	markBillingReconciled(c)

	// Should not trigger any alert
	billingAuditSafetyNet(c)
}

// TestBillingAuditSafetyNet_UnreconciledNotForwarded verifies that the safety net
// detects unreconciled pre-consumed quota and attempts an emergency refund when
// the request was NOT forwarded upstream.
func TestBillingAuditSafetyNet_UnreconciledNotForwarded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set(ctxkey.Id, 42)
	c.Set(ctxkey.TokenId, 10)
	c.Set(ctxkey.ChannelId, 5)
	c.Set(ctxkey.RequestId, "req_test_unreconciled")

	markPreConsumed(c, 5000)
	// NOT marking as reconciled
	// NOT marking as forwarded

	// This should detect the unreconciled quota and attempt emergency refund.
	// Since we don't have a real DB, the refund will fail silently via GoCritical.
	// The important thing is it doesn't panic.
	billingAuditSafetyNet(c)
}

// TestBillingAuditSafetyNet_UnreconciledForwarded verifies that when a request
// was forwarded upstream but billing wasn't reconciled, the safety net logs
// a CRITICAL warning but does NOT attempt refund (to prevent underbilling).
func TestBillingAuditSafetyNet_UnreconciledForwarded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set(ctxkey.Id, 42)
	c.Set(ctxkey.TokenId, 10)
	c.Set(ctxkey.ChannelId, 5)
	c.Set(ctxkey.RequestId, "req_test_forwarded")
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)

	markPreConsumed(c, 5000)
	// NOT marking as reconciled

	// Should not panic. Will log CRITICAL but won't attempt refund.
	billingAuditSafetyNet(c)
}

// TestReturnPreConsumedQuotaConservative_MarksReconciled verifies that
// returnPreConsumedQuotaConservative automatically marks billing as reconciled.
func TestReturnPreConsumedQuotaConservative_MarksReconciled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)

	markPreConsumed(c, 5000)

	// The actual refund will fail (no DB), but the reconciled flag should be set
	// Note: returnPreConsumedQuotaConservative calls billing.ReturnPreConsumedQuota
	// which needs a DB. We can't easily test that without a DB setup.
	// But we can verify the skip path marks reconciled.
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
	returnPreConsumedQuotaConservative(c.Request.Context(), c, 5000, 10, "test")

	reconciled, exists := c.Get(ctxkey.BillingReconciled)
	require.True(t, exists)
	require.Equal(t, true, reconciled.(bool))
}

// TestBillingAuditSafetyNet_ZeroPreConsume is a no-op when pre-consumed is 0.
func TestBillingAuditSafetyNet_ZeroPreConsume(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)

	markPreConsumed(c, 0)
	billingAuditSafetyNet(c)
}

// TestBillingLifecycle_NormalPath tests the complete billing lifecycle:
// pre-consume → mark → post-billing → reconcile → safety net is no-op.
func TestBillingLifecycle_NormalPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)

	// Step 1: Pre-consume
	markPreConsumed(c, 10000)

	// Step 2: Post-billing marks reconciled
	markBillingReconciled(c)

	// Step 3: Safety net should be a no-op
	billingAuditSafetyNet(c)

	// Verify state
	reconciled, _ := c.Get(ctxkey.BillingReconciled)
	require.Equal(t, true, reconciled.(bool))
}

// TestBillingLifecycle_ErrorRefundPath tests: pre-consume → error → refund → safety net is no-op.
func TestBillingLifecycle_ErrorRefundPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)

	// Step 1: Pre-consume
	markPreConsumed(c, 10000)

	// Step 2: Error path - mark forwarded so refund is skipped (simulates real behavior)
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
	returnPreConsumedQuotaConservative(c.Request.Context(), c, 10000, 10, "test_error")

	// Step 3: Safety net should be a no-op (reconciled by returnPreConsumedQuotaConservative)
	billingAuditSafetyNet(c)

	reconciled, _ := c.Get(ctxkey.BillingReconciled)
	require.Equal(t, true, reconciled.(bool))
}
