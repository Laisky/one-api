package model

// Metric-emission proof for the lookup fallback reasons and the cycle-failure counter
// (AUTO-T24's remaining corners).
//
// The main observability test proves states, bounded labels, and no leakage; the lookup suite
// proves the missing/mismatch/capability CODE PATHS return correct rows. What neither proved is
// that those paths actually EMIT their series — a wiring mistake (wrong reason constant, counter
// recorded on the wrong branch) would pass both. Each subtest here drives the real corruption
// through the real lookup and asserts the exact series moved, in a separate file only to respect
// the 600-line limit.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/idresolve"
)

func TestCompactUUIDObservabilityFallbackEmission(t *testing.T) {
	t.Run("a shadow gap emits the missing fallback", func(t *testing.T) {
		withCompactPrometheusRecorder(t)
		db, topology := newCompactTestTopology(t)
		ctx := compactTestContext(t)
		seedCompactUser(t, db, 1, compactUUIDTextFor(1))
		driveCompactToReady(t, newCompactCoordinator(topology))
		enableCompactReadsForTest(t, uuidRolePrimary)

		// The trigger self-heals a direct corruption, so it must go first — exactly the
		// missing-trigger state section 7 names.
		dropCompactSyncTriggers(t, db, "users")
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = NULL WHERE id = 1").Error)

		target, err := compactLookupTarget("users")
		require.NoError(t, err)
		before := gatherCompactMetrics(t)
		id, err := resolveIDByUUID(ctx, db, target, compactUUIDTextFor(1))
		require.NoError(t, err)
		require.Equal(t, int64(1), id, "the fallback must still answer from authoritative text")
		after := gatherCompactMetrics(t)

		requireCompactSeriesGrew(t, before, after, compactMetricFallback, map[string]string{
			"role": string(uuidRolePrimary), "reason": compactFallbackMissing})
	})

	t.Run("a wrong shadow emits the mismatch fallback and the mismatch backlog", func(t *testing.T) {
		withCompactPrometheusRecorder(t)
		db, topology := newCompactTestTopology(t)
		ctx := compactTestContext(t)
		seedCompactUser(t, db, 1, compactUUIDTextFor(1))
		seedCompactUser(t, db, 2, compactUUIDTextFor(2))
		driveCompactToReady(t, newCompactCoordinator(topology))
		enableCompactReadsForTest(t, uuidRolePrimary)

		// Row 1's shadow is cleared first so pointing row 2's shadow at row 1's identifier
		// does not collide with the unique compact index.
		dropCompactSyncTriggers(t, db, "users")
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = NULL WHERE id = 1").Error)
		wrong, err := parseCompactUUID(compactUUIDTextFor(1))
		require.NoError(t, err)
		require.NoError(t, db.Exec("UPDATE users SET uuid_compact = ? WHERE id = 2",
			compactBindValue(dialectName(db), wrong)).Error)

		target, err := compactLookupTarget("users")
		require.NoError(t, err)
		before := gatherCompactMetrics(t)
		id, err := resolveIDByUUID(ctx, db, target, compactUUIDTextFor(1))
		require.NoError(t, err)
		require.Equal(t, int64(1), id, "a candidate whose text disagrees must never be returned")
		after := gatherCompactMetrics(t)

		requireCompactSeriesGrew(t, before, after, compactMetricFallback, map[string]string{
			"role": string(uuidRolePrimary), "reason": compactFallbackMismatch})
		value, found := compactSampleValue(after, compactMetricBacklog, map[string]string{
			"role": string(uuidRolePrimary), "target": "users.uuid", "kind": compactBacklogMismatch})
		require.True(t, found, "the mismatch backlog gauge must be published")
		require.Equal(t, 1.0, value, "the gauge reports the bounded observation, one mismatched row")
	})

	t.Run("a vanished shadow column emits the capability fallback and disables compact reads", func(t *testing.T) {
		// This subtest is live-PostgreSQL-gated, and the reason is an engine quirk worth
		// recording: on SQLite, a quoted identifier that no longer resolves as a column falls
		// back to a STRING LITERAL (the double-quoted-string misfeature), so the probe against
		// a dropped "uuid_compact" silently becomes an always-false comparison — zero rows, no
		// error, classified as a MISSING fallback. The answer stays correct via the legacy
		// path and the audit still catches the vanished column, but the capability branch
		// itself can only genuinely fire on an engine that errors, as PostgreSQL does with
		// undefined_column.
		dialect := compactLiveDialects()[1]
		db, topology, ok := newLiveCompactTopology(t, dialect, false)
		if !ok {
			t.Skipf("%s is not configured; CI's no-skip guard enforces this suite", dialect.primaryEnv)
		}
		withCompactPrometheusRecorder(t)
		ctx := compactTestContext(t)
		seedCompactUser(t, db, 1, compactUUIDTextFor(1))
		driveCompactToReady(t, newCompactCoordinator(topology))

		require.NoError(t, db.Exec("DROP INDEX idx_users_uuid_compact_unique").Error)
		require.NoError(t, db.Exec("DROP TRIGGER "+compactSyncTriggerName("users")+" ON users").Error)
		require.NoError(t, db.Exec("ALTER TABLE users DROP COLUMN uuid_compact").Error)

		// Health is published after the DDL: the compressed test TTL (twice a 50ms idle
		// interval) could otherwise expire in transit and emit expired_health instead of
		// exercising the capability branch this subtest exists to prove.
		enableCompactReadsForTest(t, uuidRolePrimary)

		target, err := compactLookupTarget("users")
		require.NoError(t, err)
		before := gatherCompactMetrics(t)
		id, err := resolveIDByUUID(ctx, db, target, compactUUIDTextFor(1))
		require.NoError(t, err, "a capability race is handled once, by fallback, not returned")
		require.Equal(t, int64(1), id)
		after := gatherCompactMetrics(t)

		requireCompactSeriesGrew(t, before, after, compactMetricFallback, map[string]string{
			"role": string(uuidRolePrimary), "reason": compactFallbackCapability})
		enabled, reason := compactReadsEnabled(uuidRolePrimary)
		require.False(t, enabled, "a capability error must disable compact reads process-wide")
		require.NotEmpty(t, reason)

		// The identifier itself stays resolvable and a canonical unknown stays not-found:
		// losing the shadow column must not change either answer.
		_, err = resolveIDByUUID(ctx, db, target, compactUUIDTextFor(4242))
		require.ErrorIs(t, err, idresolve.ErrNotFound)
	})

	t.Run("a failed cycle emits the cycle failure action", func(t *testing.T) {
		withCompactPrometheusRecorder(t)
		db, topology := newCompactTestTopology(t)
		ctx := compactTestContext(t)
		seedCompactUser(t, db, 1, compactUUIDTextFor(1))
		driveCompactToReady(t, newCompactCoordinator(topology))

		// Ownership must SUCCEED and the cycle itself must fail, or the error lands on the
		// acquisition branch, which deliberately does not count a cycle failure. Dropping
		// data_migrations fails the cycle's very first read after the lock is held.
		require.NoError(t, db.Exec("DROP TABLE data_migrations").Error)

		coordinator := newCompactCoordinator(topology)
		before := gatherCompactMetrics(t)
		delay := runCompactWorkerCycle(withCompactLogger(context.Background()), coordinator)
		require.Positive(t, delay, "a failed cycle must schedule a backoff delay")
		after := gatherCompactMetrics(t)

		requireCompactSeriesGrew(t, before, after, compactMetricActions, map[string]string{
			"role": string(uuidRolePrimary), "action": compactActionCycle, "result": uuidResultFailure})
		requireCompactExactlyOneState(t, topology, compactStateRetryWait)
		_ = ctx
	})
}
