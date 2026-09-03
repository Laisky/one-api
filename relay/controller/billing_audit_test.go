package controller

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/model"
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

// TestBillingAuditSafetyNetRefundsOnlyWhenNotForwarded pins the one decision this
// safety net exists to make: refund unreconciled pre-consumed quota when the
// request never reached upstream, and deliberately do NOT refund when it may have,
// because refunding a request the provider actually served is systematic
// under-billing.
//
// Both cases previously called billingAuditSafetyNet with no assertion at all —
// the comments said "the important thing is it doesn't panic" — so inverting the
// forwarded check, the exact mistake that costs money, passed both tests.
func TestBillingAuditSafetyNetRefundsOnlyWhenNotForwarded(t *testing.T) {
	for _, tc := range []struct {
		name       string
		forwarded  bool
		wantRefund bool
	}{
		{name: "never forwarded upstream so the quota is returned", forwarded: false, wantRefund: true},
		{name: "possibly forwarded upstream so the quota is kept", forwarded: true, wantRefund: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanup := setupCacheBillingLogTest(t)
			defer cleanup()

			const preConsumed = 5000
			before := tokenRemainQuotaForAudit(t, 1)

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			c.Set(ctxkey.Id, 1)
			c.Set(ctxkey.TokenId, 1)
			c.Set(ctxkey.ChannelId, 5)
			c.Set(ctxkey.RequestId, "req_test_"+tc.name)

			markPreConsumed(c, preConsumed)
			if tc.forwarded {
				c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
			}
			// Deliberately not reconciled: that is what arms the safety net.

			billingAuditSafetyNet(c)
			// The refund runs on a detached critical goroutine; Drain waits for it.
			require.NoError(t, graceful.Drain(context.Background()))

			after := tokenRemainQuotaForAudit(t, 1)
			if tc.wantRefund {
				require.Equal(t, before+preConsumed, after,
					"a request that never reached upstream must have its pre-consumed quota returned")
				return
			}
			require.Equal(t, before, after,
				"a request that may have been served upstream must NOT be refunded")
		})
	}
}

// tokenRemainQuotaForAudit reads a token's remaining quota straight from the DB.
//
// Parameters:
//   - t: the running test.
//   - tokenID: the token to read.
//
// Return values:
//   - int64: the persisted remaining quota.
func tokenRemainQuotaForAudit(t *testing.T, tokenID int) int64 {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.First(&token, "id = ?", tokenID).Error)
	return token.RemainQuota
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
