package controller

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/graceful"
)

// TestGoAudioRollbackPreConsumed_UsesLiveContext reproduces the silent refund-loss
// bug at audio.go: the pre-consume rollback refund goroutine was spawned with the
// REQUEST context (gmw.Ctx(c)). That goroutine runs from inside a defer that executes
// AFTER the handler returns, so the request context is (being) cancelled by the time
// the refund DB write runs -> the write can be aborted and the refund is silently lost.
//
// The test installs a release gate so the rollback goroutine is parked until after the
// request context is cancelled (simulating "handler returned, then goroutine ran"), and
// records the context error the goroutine observes BEFORE the DB call. The refund must
// run on a live (non-cancelled) context, so the observed error must be nil.
//
// Against the buggy implementation (gmw.Ctx(c)) the goroutine observes context.Canceled
// and the assertion fails; with the fix (gmw.BackgroundCtx(c)) it observes nil and passes.
//
// Note: PostConsumeTokenQuota may error without a DB; that is fine — the assertion is on
// the context error recorded before the DB call, which is deterministic.
func TestGoAudioRollbackPreConsumed_UsesLiveContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gate := make(chan struct{})
	observed := make(chan error, 1)
	audioRollbackGateForTest = gate
	audioRollbackObservedCtxErrForTest = func(err error) {
		observed <- err
	}
	t.Cleanup(func() {
		audioRollbackGateForTest = nil
		audioRollbackObservedCtxErrForTest = nil
	})

	// Build a gin context whose request carries a CANCELLABLE context, so gmw.Ctx(c)
	// returns a context we can cancel to simulate the handler returning.
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("POST", "/v1/audio/transcriptions", nil)
	reqCtx, cancelReq := context.WithCancel(context.Background())
	c.Request = req.WithContext(reqCtx)

	// Sanity: gmw.Ctx(c) reflects c.Request's context cancellation while
	// gmw.BackgroundCtx(c) does not. This is the contract the fix relies on.
	require.NoError(t, gmw.Ctx(c).Err(), "request context should start live")

	// Spawn the rollback goroutine (parked on the gate).
	goAudioRollbackPreConsumed(c, 0, 1207)

	// Simulate the handler returning: the request context is cancelled.
	cancelReq()
	require.Error(t, gmw.Ctx(c).Err(), "request context must be cancelled after handler returns")

	// Release the parked goroutine so it observes the context and runs the refund.
	close(gate)

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelDrain()
	require.NoError(t, graceful.Drain(drainCtx))

	select {
	case err := <-observed:
		require.NoError(t, err,
			"audio rollback refund must run on a live (non-cancelled) context; "+
				"spawning it on the request context lets the refund DB write abort "+
				"after the handler returns, silently losing the refund")
	case <-time.After(time.Second):
		t.Fatal("rollback goroutine did not record an observed context error")
	}
}
