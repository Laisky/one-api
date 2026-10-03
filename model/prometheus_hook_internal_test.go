package model

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestInitPrometheusDBMonitoringDoesNotRaceWithBackgroundQueries reproduces
// the startup data race seen under `go run -race`: main.go registered the
// Prometheus gorm callbacks (a write to gorm's callback chain) after InitDB had
// already started background workers (UUID catch-up, option/channel sync) that
// were issuing queries through the same handle. gorm callback registration is
// not concurrency-safe, so the hook must be attached when the handle is opened
// and InitPrometheusDBMonitoring must be a no-op afterwards.
func TestInitPrometheusDBMonitoringDoesNotRaceWithBackgroundQueries(t *testing.T) {
	setupTestDatabase(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				// Same query shape as the UUID catch-up worker that raced in production.
				_ = hasIndexNamed(ctx, DB, "traces", "idx_traces_never_exists")
			}
		}()
	}

	// Let the workers get going before the (formerly late) registration.
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, InitPrometheusDBMonitoring())
	require.NoError(t, InitPrometheusDBMonitoring(), "registration must be idempotent")
	time.Sleep(50 * time.Millisecond)

	cancel()
	wg.Wait()

	_, registered := DB.Config.Plugins[(&PrometheusDBHook{}).Name()]
	require.True(t, registered, "hook must be registered on the primary handle")
}
