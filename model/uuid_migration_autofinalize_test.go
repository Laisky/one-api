package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestMigrationCompletesAutomaticallyByDefault proves the whole upgrade lifecycle runs and
// completes with no operator flag at all: legacy rows are reconciled by the background
// worker, sustained quiescence triggers automatic finalization, both the unique indexes and
// the completion marker appear, and a subsequent invocation is marker-only. The intervals
// and the quiescence threshold are shrunk so the test observes in seconds what a default
// deployment does over minutes; the policy flags themselves stay at their defaults.
func TestMigrationCompletesAutomaticallyByDefault(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	pinUUIDRaceSQLiteConnection(t, db)
	withFinalizerEnabled(t, false)
	require.True(t, config.ExternalUUIDBackfillAutoFinalize,
		"automatic completion must be the DEFAULT policy, not an opt-in")
	withAutoFinalize(t, true, 1)
	withCatchUpIntervals(t, 0, config.MinExternalUUIDBackfillIdleInterval)
	seedUnifiedLegacyRows(t, db)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, startExternalUUIDMigration(ctx, topology),
		"default startup must not fail and must not block on the migration")

	require.Eventually(t, func() bool {
		complete, err := isDataMigrationComplete(context.Background(), db, externalUUIDPrimaryMigrationKey)
		return err == nil && complete
	}, 30*time.Second, 25*time.Millisecond,
		"the migration must finalize automatically after sustained quiescence")

	// Completion is real, not just a marker: data reconciled and unique indexes promoted.
	var user User
	require.NoError(t, db.First(&user, "id = ?", 1).Error)
	requireHyphenatedUUID(t, user.UUID)
	var log Log
	require.NoError(t, db.First(&log, "id = ?", 1).Error)
	requireHyphenatedUUID(t, log.UUID)
	require.NotNil(t, log.TokenUUID)
	for _, target := range uuidOwnedRegistry() {
		requireUUIDUniqueIndex(t, db, target)
	}

	// Idempotence after completion: every later invocation, in either mode, is a
	// marker-only no-op that rewrites nothing.
	stopUUIDCatchUpWorker()
	firstUUID := user.UUID
	result := runCatchUp(t, topology)
	require.True(t, result.completed)
	require.Zero(t, result.updated)
	result, err := runFinalizer(t, topology)
	require.NoError(t, err)
	require.True(t, result.completed)
	require.Zero(t, result.updated)
	require.NoError(t, db.First(&user, "id = ?", 1).Error)
	require.Equal(t, firstUUID, user.UUID, "completion must be idempotent and never rewrite a uuid")
}

// TestOldWriterStillWorksAfterCompletion proves full backward compatibility on both sides of
// completion. Before completion the rolling-window tests already cover UUID-unaware writers;
// this test covers AFTER: a pre-UUID binary that knows nothing about the uuid columns must
// still be able to insert and read rows once the unique indexes and markers exist, because
// its inserts carry NULL uuids and every dialect's unique index treats NULLs as distinct.
func TestOldWriterStillWorksAfterCompletion(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, true)
	seedUnifiedLegacyRows(t, db)
	_, err := runFinalizer(t, topology)
	require.NoError(t, err)
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, true)

	// A pre-UUID writer issues inserts that simply omit every uuid column. Two rows per
	// table prove the promoted unique index does not collide on multiple NULLs.
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, username, password) VALUES (100, 'old-writer-a', 'password-hash'), (101, 'old-writer-b', 'password-hash')").Error,
		"an old writer must still be able to create users after completion")
	require.NoError(t, db.Exec(
		"INSERT INTO tokens (id, user_id, `key`, name) VALUES (100, 100, 'old-writer-key-a', 'old-a'), (101, 100, 'old-writer-key-b', 'old-b')").Error,
		"an old writer must still be able to create tokens after completion")
	require.NoError(t, db.Exec(
		"INSERT INTO logs (id, user_id, type, content) VALUES (100, 100, 1, 'old writer log a'), (101, 100, 1, 'old writer log b')").Error,
		"an old writer must still be able to create logs after completion")

	// The application reads and updates those rows normally.
	var old User
	require.NoError(t, db.First(&old, "id = ?", 100).Error)
	require.Empty(t, old.UUID)
	require.NoError(t, db.Model(&User{}).Where("id = ?", 100).Update("quota", 42).Error)

	// And completion state is untouched: later invocations stay marker-only no-ops even
	// though old-writer rows now exist. Their UUIDs are backfilled if the operator ever
	// deletes the markers and reruns, which is the documented rollback recovery.
	result := runCatchUp(t, topology)
	require.True(t, result.completed)
	require.Zero(t, result.updated)
}

// TestAutoFinalizeBacksOffAfterBlockedValidation proves a failed automatic attempt is safe:
// data an operator must remediate (a duplicate owned UUID) blocks finalization, the worker
// records the failure and keeps running, no marker appears, and the ordinary index survives
// so reads never degrade. Automatic completion may only ever succeed exactly once the data
// is actually completable.
func TestAutoFinalizeBacksOffAfterBlockedValidation(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	pinUUIDRaceSQLiteConnection(t, db)
	withFinalizerEnabled(t, false)
	withAutoFinalize(t, true, 1)
	withCatchUpIntervals(t, 0, config.MinExternalUUIDBackfillIdleInterval)

	shared := "018f0000-0000-7000-8000-000000000001"
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, uuid, username, password) VALUES (1, ?, 'root', 'password-hash'), (2, ?, 'other', 'password-hash')",
		shared, shared).Error)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, startExternalUUIDMigration(ctx, topology))

	// Give the worker time to reach and fail at least one automatic attempt.
	require.Eventually(t, func() bool {
		usable, err := hasUsableIndexNamed(context.Background(), db, "users", ordinaryUUIDIndexName("users"))
		return err == nil && usable
	}, 20*time.Second, 25*time.Millisecond, "the candidate index must exist before any attempt")
	time.Sleep(300 * time.Millisecond)

	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)
	require.False(t, db.Migrator().HasIndex(&User{}, uuidUniqueIndexName("users")),
		"a blocked automatic attempt must not promote the unique index")
	require.True(t, db.Migrator().HasIndex(&User{}, ordinaryUUIDIndexName("users")),
		"the ordinary index must survive every failed automatic attempt")
}
