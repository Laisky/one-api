package model

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

// compactFallbackFixture holds the real database and lookup target after shadow corruption.
type compactFallbackFixture struct {
	db     *gorm.DB
	ctx    context.Context
	target compactTarget
	reason string
}

// prepareCompactFallbackFixture creates a missing or mismatched shadow using real SQL.
// Parameters: t owns the fixture, mismatch chooses corruption, and delayedSetup stalls the SQL pool until audit expiry.
// Returns: the database, lookup context, registry target, and expected compact fallback reason.
func prepareCompactFallbackFixture(t *testing.T, mismatch, delayedSetup bool) compactFallbackFixture {
	t.Helper()
	withCompactPrometheusRecorder(t)
	db, topology := newCompactTestTopology(t)
	ctx := compactTestContext(t)
	seedCompactUser(t, db, 1, compactUUIDTextFor(1))
	if mismatch {
		seedCompactUser(t, db, 2, compactUUIDTextFor(2))
	}
	driveCompactToReady(t, newCompactCoordinator(topology))
	enableCompactReadsForTest(t, uuidRolePrimary)

	var finishDelay func()
	if delayedSetup {
		finishDelay = blockCompactPreparationUntilAuditExpires(t, db)
	}
	// The trigger self-heals direct corruption, so remove it before changing the shadow.
	dropCompactSyncTriggers(t, db, "users")
	if finishDelay != nil {
		finishDelay()
	}
	require.NoError(t, db.Exec("UPDATE users SET uuid_compact = NULL WHERE id = 1").Error)
	reason := compactFallbackMissing
	if mismatch {
		wrong, err := parseCompactUUID(compactUUIDTextFor(1))
		require.NoError(t, err)
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 2",
			compactBindValue(dialectName(db), wrong)).Error)
		reason = compactFallbackMismatch
	}
	target, err := compactLookupTarget("users")
	require.NoError(t, err)
	return compactFallbackFixture{db: db, ctx: ctx, target: target, reason: reason}
}

// exerciseCompactFallbackMetricFixture verifies the intended fallback counter and authoritative answer.
// Parameters: t owns assertions, mismatch chooses corruption, delayedSetup stalls preparation, and optional delayedLookup stalls after health publication.
// Returns: none.
func exerciseCompactFallbackMetricFixture(t *testing.T, mismatch, delayedSetup bool, delayedLookup ...bool) {
	t.Helper()
	fixture := prepareCompactFallbackFixture(t, mismatch, delayedSetup)
	compressedHealthTTL := compactHealthTTL()
	// Migration keeps its compressed intervals; only the healthy metric assertion gets a
	// one-minute lease so a runner pause at the lookup boundary cannot select expired health.
	originalIdle := config.CompactUUIDIdleInterval
	config.CompactUUIDIdleInterval = 30 * time.Second
	t.Cleanup(func() { config.CompactUUIDIdleInterval = originalIdle })
	before := gatherCompactMetrics(t)
	// Real preparation and a full scrape can outlive the compressed 100 ms health TTL.
	// Publish the intended healthy test state after both, immediately before the lookup.
	enableCompactReadsForTest(t, uuidRolePrimary)
	if len(delayedLookup) > 0 && delayedLookup[0] {
		// Block a real SQL operation at the lookup boundary for the original compressed lease.
		// The delay stays fixed if the healthy-path fixture later adopts a longer lease.
		finishDelay := blockCompactFixturePool(t, fixture.db, compressedHealthTTL, false)
		require.NoError(t, fixture.db.WithContext(fixture.ctx).Exec("SELECT 1").Error)
		finishDelay()
	}
	id, err := resolveIDByUUID(fixture.ctx, fixture.db, fixture.target, compactUUIDTextFor(1))
	require.NoError(t, err)
	require.Equal(t, int64(1), id, "corrupt derived shadows must still resolve through authoritative text")
	after := gatherCompactMetrics(t)
	requireCompactSeriesGrew(t, before, after, compactMetricFallback, map[string]string{
		"role": string(uuidRolePrimary), "reason": fixture.reason})
	if mismatch {
		value, found := compactSampleValue(after, compactMetricBacklog, map[string]string{
			"role": string(uuidRolePrimary), "target": "users.uuid", "kind": compactBacklogMismatch})
		require.True(t, found, "the mismatch backlog gauge must be published")
		require.Equal(t, 1.0, value, "one mismatched candidate must produce one bounded backlog observation")
	}
}

// blockCompactPreparationUntilAuditExpires occupies the SQLite pool until real preparation SQL waits and health expires.
// Parameters: t owns cleanup and assertions, and db is the isolated fixture handle.
// Returns: a join function proving actual connection contention and health expiry before corruption continues.
func blockCompactPreparationUntilAuditExpires(t *testing.T, db *gorm.DB) func() {
	t.Helper()
	return blockCompactFixturePool(t, db, 0, true)
}

// blockCompactFixturePool occupies the SQLite pool until an actual SQL wait satisfies the requested delay condition.
// Parameters: t owns cleanup, db is isolated, minimumWait is a fixed stall duration, and awaitExpiry selects the real expired-health control.
// Returns: a join function proving actual pool contention and the chosen release condition.
func blockCompactFixturePool(t *testing.T, db *gorm.DB, minimumWait time.Duration, awaitExpiry bool) func() {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousLimit := sqlDB.Stats().MaxOpenConnections
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.SetMaxOpenConns(previousLimit) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	blocker, err := sqlDB.Conn(ctx)
	require.NoError(t, err)
	initialWaits := sqlDB.Stats().WaitCount
	var release sync.Once
	var closeErr error
	closeBlocker := func() { release.Do(func() { closeErr = blocker.Close() }) }
	done := make(chan struct{})
	var observedDelay bool
	go func() {
		defer close(done)
		defer closeBlocker()
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		var waitingSince time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if sqlDB.Stats().WaitCount <= initialWaits {
					continue
				}
				if waitingSince.IsZero() {
					waitingSince = time.Now()
				}
				ready := time.Since(waitingSince) >= minimumWait
				if awaitExpiry {
					readsEnabled, _ := compactReadsEnabled(uuidRolePrimary)
					ready = ready && !readsEnabled
				}
				if ready {
					observedDelay = true
					return
				}
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		closeBlocker()
		<-done
		require.NoError(t, closeErr)
	})
	return func() {
		<-done
		require.NoError(t, closeErr)
		require.True(t, observedDelay, "real SQL must wait for its connection until the requested release condition holds")
	}
}

// TestCompactFallbackMetricsSurviveSlowPreparation verifies missing and mismatch fixtures reach their intended compact branch despite slow setup.
// Parameters: t owns the real Prometheus and isolated SQLite fixtures. Returns: none.
func TestCompactFallbackMetricsSurviveSlowPreparation(t *testing.T) {
	t.Run("missing shadow", func(t *testing.T) { exerciseCompactFallbackMetricFixture(t, false, true) })
	t.Run("mismatched shadow", func(t *testing.T) { exerciseCompactFallbackMetricFixture(t, true, true) })
}

// TestCompactFallbackMetricsSurviveLookupBoundaryDelay verifies the healthy-path counter remains stable when the runner stalls after publication.
// Parameters: t owns real SQLite contention and before/after Prometheus scrapes. Returns: none.
func TestCompactFallbackMetricsSurviveLookupBoundaryDelay(t *testing.T) {
	t.Run("missing shadow", func(t *testing.T) { exerciseCompactFallbackMetricFixture(t, false, false, true) })
	t.Run("mismatched shadow", func(t *testing.T) { exerciseCompactFallbackMetricFixture(t, true, false, true) })
}

// TestCompactFallbackMetricsPreserveExpiredHealth verifies real audit expiry still selects the legacy path and its own metric reason.
// Parameters: t owns the real SQL-pool stall, lookup, and scrape assertions. Returns: none.
func TestCompactFallbackMetricsPreserveExpiredHealth(t *testing.T) {
	fixture := prepareCompactFallbackFixture(t, false, true)
	enabled, _ := compactReadsEnabled(uuidRolePrimary)
	require.False(t, enabled, "real pool contention must have expired the audit before lookup")
	before := gatherCompactMetrics(t)
	id, err := resolveIDByUUID(fixture.ctx, fixture.db, fixture.target, compactUUIDTextFor(1))
	require.NoError(t, err)
	require.Equal(t, int64(1), id)
	after := gatherCompactMetrics(t)
	requireCompactSeriesGrew(t, before, after, compactMetricFallback, map[string]string{
		"role": string(uuidRolePrimary), "reason": compactFallbackExpiredHealth})
	missingLabels := map[string]string{"role": string(uuidRolePrimary), "reason": compactFallbackMissing}
	previous, _ := compactSampleValue(before, compactMetricFallback, missingLabels)
	current, _ := compactSampleValue(after, compactMetricFallback, missingLabels)
	require.Equal(t, previous, current, "expired health must not be reclassified as a compact shadow miss")
}
