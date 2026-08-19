package controller

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestScheduleWebSocketPostBilling_UsesCapturedQuotaID reproduces the *gin.Context
// recycling race in maybeHandleResponseAPIWebSocket's post-billing path
// (response_ws.go). gin v1.12.0 recycles *gin.Context via sync.Pool the instant
// the handler returns, so reading quotaId via c.GetInt(ctxkey.Id) from inside the
// spawned post-billing goroutine races the next request's c.reset()/Set and bills
// the wrong user.
//
// The test freezes the post-billing goroutine on a release gate AFTER the
// (zero-usage, DB-free) quota computation but BEFORE the quotaId is consumed, then
// mutates ctxkey.Id on the same context to 999 (simulating gin handing the
// recycled context to the next request). It asserts the goroutine finalizes the
// cost with the quotaId captured at spawn time (100), not the mutated value.
//
// Against the buggy form (quotaId read off *c inside the goroutine) the observed
// value is 999 and the assertion fails; with the pre-captured value param it is
// 100 and the assertion passes. The assertion is on the VALUE the goroutine
// observes (deterministic), not on the -race detector firing.
func TestScheduleWebSocketPostBilling_UsesCapturedQuotaID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const capturedQuotaID = 100
	const recycledQuotaID = 999

	gate := make(chan struct{})
	observed := make(chan int, 1)
	wsPostBillingGateForTest = gate
	wsPostBillingObservedQuotaIDForTest = observed
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate) }) }
	// Always release and drain the parked post-billing goroutine before clearing
	// the seams, even when the assertion below fails via FailNow (which still runs
	// deferred funcs). releaseOnce makes the close idempotent so the body and this
	// cleanup can both call it without panicking, preventing a goroutine leak.
	defer func() {
		release()
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = graceful.Drain(drainCtx)
		wsPostBillingGateForTest = nil
		wsPostBillingObservedQuotaIDForTest = nil
	}()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set(ctxkey.Id, capturedQuotaID)
	c.Set(ctxkey.RequestId, "req_ws_postbilling_repro")

	lg := gmw.GetLogger(c)
	meta := &metalib.Meta{ActualModelName: "gpt-4o"}
	// Zero usage: postConsumeResponseAPIQuota returns early (quota = preConsumed)
	// without touching the DB, so the goroutine reaches the quotaId consumption
	// deterministically and the test needs no database.
	usage := &relaymodel.Usage{}

	// Mirror the handler: capture quotaId on the request goroutine BEFORE spawning.
	quotaId := c.GetInt(ctxkey.Id)
	requestId := c.GetString(ctxkey.RequestId)

	scheduleWebSocketPostBilling(c, lg, meta, usage, quotaId, requestId, 1207,
		1, nil, 1, nil, nil, 1)

	// Simulate gin recycling the *gin.Context for the next request while the
	// post-billing goroutine is parked on the gate.
	c.Set(ctxkey.Id, recycledQuotaID)

	// Release the goroutine so it consumes the quotaId.
	release()

	select {
	case got := <-observed:
		require.Equal(t, capturedQuotaID, got,
			"post-billing goroutine must use the quotaId captured at spawn time (%d), "+
				"not the value the recycled *gin.Context was mutated to afterwards (%d)",
			capturedQuotaID, recycledQuotaID)
	case <-time.After(5 * time.Second):
		t.Fatal("post-billing goroutine did not finalize request cost in time")
	}
}
