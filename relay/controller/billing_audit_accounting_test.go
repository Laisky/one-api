package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
)

// These tests close the loop on billingAuditSafetyNet's MONEY effect against a real (sqlite)
// database. billing_audit_test.go covers the control-flow (no-op when reconciled / zero /
// absent; logs vs refunds); these assert the actual quota balance after the emergency-refund
// goroutine (goDetachedBillingWork on a detached, bounded context) drains:
//
//   - unreconciled + NOT forwarded => emergency refund lands, balance restored;
//   - unreconciled + forwarded     => no refund (conservative no-underbilling), balance kept;
//   - reconciled                   => no-op, balance kept.

// newAuditContext builds a gin context wired to the fixture user/token with the given
// forwarded marker and a pre-consumed (but unreconciled) amount.
func newAuditContext(t *testing.T, forwarded bool, preConsumed int64) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.RequestId, "audit-"+t.Name())
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, forwarded)
	markPreConsumed(c, preConsumed)
	return c
}

// TestBillingAuditSafetyNet_DBAccounting_NotForwardedRefunds asserts the emergency refund of
// an unreconciled, not-forwarded pre-consume actually credits the user's quota.
func TestBillingAuditSafetyNet_DBAccounting_NotForwardedRefunds(t *testing.T) {
	const K, Q = int64(10_000_000), int64(3210)
	start := billingAccountingSetup(t, K)

	preConsume(t, Q)
	require.Equal(t, start-Q, reloadUserQuota(t), "pre-consume should deduct Q")

	c := newAuditContext(t, false /*forwarded*/, Q)
	billingAuditSafetyNet(c) // spawns the emergency refund goroutine
	drainBilling(t)

	require.Equal(t, start, reloadUserQuota(t),
		"safety net must emergency-refund an unreconciled, not-forwarded pre-consume")
}

// TestBillingAuditSafetyNet_DBAccounting_ForwardedKeepsCharge asserts a forwarded request's
// unreconciled pre-consume is NOT refunded (conservative no-underbilling) — only the CRITICAL
// audit is logged.
func TestBillingAuditSafetyNet_DBAccounting_ForwardedKeepsCharge(t *testing.T) {
	const K, Q = int64(10_000_000), int64(6789)
	start := billingAccountingSetup(t, K)

	preConsume(t, Q)
	c := newAuditContext(t, true /*forwarded*/, Q)
	billingAuditSafetyNet(c)
	drainBilling(t)

	require.Equal(t, start-Q, reloadUserQuota(t),
		"forwarded request must NOT be auto-refunded by the safety net (no-underbilling)")
}

// TestBillingAuditSafetyNet_DBAccounting_ReconciledNoop asserts a reconciled request is left
// untouched (no double refund) regardless of the forwarded marker.
func TestBillingAuditSafetyNet_DBAccounting_ReconciledNoop(t *testing.T) {
	const K, Q = int64(10_000_000), int64(4444)
	start := billingAccountingSetup(t, K)

	preConsume(t, Q)
	c := newAuditContext(t, false /*forwarded*/, Q)
	markBillingReconciled(c) // billing already settled => safety net must be a no-op
	billingAuditSafetyNet(c)
	drainBilling(t)

	require.Equal(t, start-Q, reloadUserQuota(t),
		"a reconciled request must not be touched by the safety net (no double refund)")
}
