package model

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedUnifiedLegacyRows inserts legacy rows with no UUID values on a unified database.
func seedUnifiedLegacyRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec("INSERT INTO users (id, username, password, inviter_id) VALUES (1, 'root', 'password-hash', 0), (2, 'child', 'password-hash', 1)").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, type, name, models, config) VALUES (1, 1, 'primary', 'gpt-4o', '{}')").Error)
	require.NoError(t, db.Exec("INSERT INTO tokens (id, user_id, `key`, name) VALUES (1, 1, 'legacy-token-key', 'default')").Error)
	require.NoError(t, db.Exec("INSERT INTO logs (id, user_id, channel_id, type, token_name, content) VALUES (1, 1, 1, 1, 'default', 'legacy log')").Error)
	require.NoError(t, db.Exec("INSERT INTO redemptions (id, user_id, `key`, name) VALUES (1, 1, 'legacy-redemption-key', 'gift')").Error)
}

// TestUnifiedFinalizerBackfillsLegacyRows verifies owned and FK UUID backfill plus promotion.
func TestUnifiedFinalizerBackfillsLegacyRows(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, true)
	seedUnifiedLegacyRows(t, db)

	_, err := runFinalizer(t, topology)
	require.NoError(t, err)

	var user User
	require.NoError(t, db.First(&user, "id = ?", 1).Error)
	requireHyphenatedUUID(t, user.UUID)

	var child User
	require.NoError(t, db.First(&child, "id = ?", 2).Error)
	require.NotNil(t, child.InviterUUID)
	require.Equal(t, user.UUID, *child.InviterUUID)

	var channel Channel
	require.NoError(t, db.First(&channel, "id = ?", 1).Error)
	requireHyphenatedUUID(t, channel.UUID)

	var token Token
	require.NoError(t, db.First(&token, "id = ?", 1).Error)
	requireHyphenatedUUID(t, token.UUID)
	require.NotNil(t, token.UserUUID)
	require.Equal(t, user.UUID, *token.UserUUID)

	var log Log
	require.NoError(t, db.First(&log, "id = ?", 1).Error)
	requireHyphenatedUUID(t, log.UUID)
	require.NotNil(t, log.UserUUID)
	require.Equal(t, user.UUID, *log.UserUUID)
	require.NotNil(t, log.ChannelUUID)
	require.Equal(t, channel.UUID, *log.ChannelUUID)
	require.NotNil(t, log.TokenUUID)
	require.Equal(t, token.UUID, *log.TokenUUID)

	for _, target := range uuidOwnedRegistry() {
		requireUUIDUniqueIndex(t, db, target)
	}
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, true)
	require.Error(t, db.Model(&User{}).Where("id = ?", child.Id).Update("uuid", user.UUID).Error)
}

// TestSplitFinalizerBackfillsCrossDatabaseUUIDs covers UUID-A01: one split invocation with the
// finalizer enabled fills primary owners, log owners, every cross-database FK UUID, and writes
// both current-generation markers.
func TestSplitFinalizerBackfillsCrossDatabaseUUIDs(t *testing.T) {
	primary, logDB, topology := newSplitTestTopology(t)
	withFinalizerEnabled(t, true)

	require.NoError(t, primary.Exec("INSERT INTO users (id, username, password) VALUES (1, 'root', 'password-hash')").Error)
	require.NoError(t, primary.Exec("INSERT INTO channels (id, type, name, models, config) VALUES (1, 1, 'primary', 'gpt-4o', '{}')").Error)
	require.NoError(t, primary.Exec("INSERT INTO tokens (id, user_id, `key`, name) VALUES (1, 1, 'legacy-token-key', 'default')").Error)
	require.NoError(t, primary.Exec("INSERT INTO token_transactions (id, transaction_id, token_id, user_id, status, pre_quota, log_id) VALUES (1, 'txn-split-log', 1, 1, 1, 10, 77)").Error)
	require.NoError(t, logDB.Exec("INSERT INTO logs (id, user_id, channel_id, type, token_name, content) VALUES (77, 1, 1, 1, 'default', 'split log')").Error)

	_, err := runFinalizer(t, topology)
	require.NoError(t, err)

	var user User
	require.NoError(t, primary.First(&user, "id = ?", 1).Error)
	var channel Channel
	require.NoError(t, primary.First(&channel, "id = ?", 1).Error)
	var token Token
	require.NoError(t, primary.First(&token, "id = ?", 1).Error)

	var log Log
	require.NoError(t, logDB.First(&log, "id = ?", 77).Error)
	requireHyphenatedUUID(t, log.UUID)
	require.NotNil(t, log.UserUUID)
	require.Equal(t, user.UUID, *log.UserUUID)
	require.NotNil(t, log.ChannelUUID)
	require.Equal(t, channel.UUID, *log.ChannelUUID)
	require.NotNil(t, log.TokenUUID)
	require.Equal(t, token.UUID, *log.TokenUUID)

	var txn TokenTransaction
	require.NoError(t, primary.First(&txn, "id = ?", 1).Error)
	require.NotNil(t, txn.LogUUID)
	require.Equal(t, log.UUID, *txn.LogUUID)

	requireMarker(t, primary, externalUUIDPrimaryMigrationKey, true)
	requireMarker(t, logDB, externalUUIDLogMigrationKey, true)
	requireUUIDUniqueIndex(t, logDB, uuidOwnedTarget{role: uuidRoleLog, table: "logs", model: &Log{}})
	require.Error(t, logDB.Exec("INSERT INTO logs (id, uuid, user_id, channel_id, type, content) VALUES (78, ?, 1, 1, 1, 'duplicate log')", log.UUID).Error)
}

// TestSplitOnlyAuthoritativeLogRowsAreTouched covers UUID-A02: a log FK UUID that is missing only
// because its primary owner has no UUID yet must not pass validation, and a conflicting primary
// logs row with the same id must never be read or mutated in split mode.
func TestSplitOnlyAuthoritativeLogRowsAreTouched(t *testing.T) {
	primary, logDB, topology := newSplitTestTopology(t)
	withFinalizerEnabled(t, true)

	require.NoError(t, primary.Exec("INSERT INTO users (id, username, password) VALUES (1, 'root', 'password-hash')").Error)
	// A stale primary logs row shares the authoritative row's id but is not authoritative.
	require.NoError(t, primary.Exec("INSERT INTO logs (id, user_id, type, content) VALUES (5, 1, 1, 'stale primary log')").Error)
	require.NoError(t, logDB.Exec("INSERT INTO logs (id, user_id, type, content) VALUES (5, 1, 1, 'authoritative log')").Error)

	_, err := runFinalizer(t, topology)
	require.NoError(t, err)

	var authoritative Log
	require.NoError(t, logDB.First(&authoritative, "id = ?", 5).Error)
	requireHyphenatedUUID(t, authoritative.UUID)
	require.NotNil(t, authoritative.UserUUID)
	require.Equal(t, "authoritative log", authoritative.Content)

	// The stale primary logs table is not authoritative, so it is neither scanned nor mutated.
	var stale Log
	require.NoError(t, primary.First(&stale, "id = ?", 5).Error)
	require.Empty(t, stale.UUID)
	require.Nil(t, stale.UserUUID)
	require.Equal(t, "stale primary log", stale.Content)
}

// TestSplitValidationRejectsLogFKGapFromMissingPrimaryOwner covers the second half of UUID-A02:
// validation must classify a log FK gap as blocking while its primary owner UUID is fillable.
func TestSplitValidationRejectsLogFKGapFromMissingPrimaryOwner(t *testing.T) {
	primary, logDB, topology := newSplitTestTopology(t)

	require.NoError(t, primary.Exec("INSERT INTO users (id, uuid, username, password) VALUES (1, '018f0000-0000-7000-8000-000000000001', 'root', 'password-hash')").Error)
	require.NoError(t, logDB.Exec("INSERT INTO logs (id, uuid, user_id, type, content) VALUES (1, '018f0000-0000-7000-8000-0000000000aa', 1, 1, 'gap log')").Error)

	err := validateExternalUUIDs(context.Background(), topology)
	require.Error(t, err)
	require.Contains(t, err.Error(), "logs.user_uuid")
	require.Contains(t, err.Error(), "fillable missing fk uuid")
}

// TestCatchUpFillsDataButWritesNoMarker covers UUID-A03.
func TestCatchUpFillsDataButWritesNoMarker(t *testing.T) {
	primary, logDB, topology := newSplitTestTopology(t)
	withFinalizerEnabled(t, false)

	require.NoError(t, primary.Exec("INSERT INTO users (id, username, password) VALUES (1, 'root', 'password-hash')").Error)
	require.NoError(t, logDB.Exec("INSERT INTO logs (id, user_id, type, content) VALUES (1, 1, 1, 'catch-up log')").Error)

	runCatchUp(t, topology)

	var user User
	require.NoError(t, primary.First(&user, "id = ?", 1).Error)
	requireHyphenatedUUID(t, user.UUID)

	var log Log
	require.NoError(t, logDB.First(&log, "id = ?", 1).Error)
	requireHyphenatedUUID(t, log.UUID)
	require.NotNil(t, log.UserUUID)
	require.Equal(t, user.UUID, *log.UserUUID)

	requireMarker(t, primary, externalUUIDPrimaryMigrationKey, false)
	requireMarker(t, logDB, externalUUIDLogMigrationKey, false)
	// Catch-up never promotes, so the ordinary candidate index must still be the only one.
	require.False(t, logDB.Migrator().HasIndex(&Log{}, uuidUniqueIndexName("logs")))
	require.True(t, logDB.Migrator().HasIndex(&Log{}, ordinaryUUIDIndexName("logs")))
}

// TestFinalizerFindsRowsInsertedAfterCatchUp covers UUID-A04.
func TestFinalizerFindsRowsInsertedAfterCatchUp(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)

	require.NoError(t, db.Exec("INSERT INTO users (id, username, password) VALUES (1, 'root', 'password-hash')").Error)
	runCatchUp(t, topology)
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)

	// A UUID-unaware writer inserts after catch-up completed.
	require.NoError(t, db.Exec("INSERT INTO users (id, username, password) VALUES (2, 'late', 'password-hash')").Error)

	withFinalizerEnabled(t, true)
	_, err := runFinalizer(t, topology)
	require.NoError(t, err)

	var late User
	require.NoError(t, db.First(&late, "id = ?", 2).Error)
	requireHyphenatedUUID(t, late.UUID)
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, true)
}

// TestV2MarkersDoNotSuppressReconciliation covers UUID-A05 and invariant 10.
func TestV2MarkersDoNotSuppressReconciliation(t *testing.T) {
	primary, logDB, topology := newSplitTestTopology(t)
	withFinalizerEnabled(t, true)

	require.NoError(t, markDataMigrationComplete(context.Background(), primary, externalUUIDPrimaryMigrationKeyV2))
	require.NoError(t, markDataMigrationComplete(context.Background(), logDB, externalUUIDLogMigrationKeyV2))
	require.NoError(t, primary.Exec("INSERT INTO users (id, username, password) VALUES (1, 'root', 'password-hash')").Error)
	require.NoError(t, logDB.Exec("INSERT INTO logs (id, user_id, type, content) VALUES (1, 1, 1, 'v2 era log')").Error)

	_, err := runFinalizer(t, topology)
	require.NoError(t, err)

	var log Log
	require.NoError(t, logDB.First(&log, "id = ?", 1).Error)
	requireHyphenatedUUID(t, log.UUID)
	require.NotNil(t, log.UserUUID, "a v2 marker must not suppress v3 reconciliation")
	requireMarker(t, primary, externalUUIDPrimaryMigrationKey, true)
	requireMarker(t, logDB, externalUUIDLogMigrationKey, true)
}

// TestRecoveryFromLogMarkerOnlyState covers UUID-A06.
func TestRecoveryFromLogMarkerOnlyState(t *testing.T) {
	primary, logDB, topology := newSplitTestTopology(t)
	withFinalizerEnabled(t, true)

	require.NoError(t, markDataMigrationComplete(context.Background(), logDB, externalUUIDLogMigrationKey))
	require.NoError(t, primary.Exec("INSERT INTO users (id, username, password) VALUES (1, 'root', 'password-hash')").Error)
	require.NoError(t, logDB.Exec("INSERT INTO logs (id, user_id, type, content) VALUES (1, 1, 1, 'log marker only')").Error)

	_, err := runFinalizer(t, topology)
	require.NoError(t, err)

	// The absent primary marker forces a full rerun, so log rows are reconciled even though
	// the log database already carried its marker.
	var log Log
	require.NoError(t, logDB.First(&log, "id = ?", 1).Error)
	requireHyphenatedUUID(t, log.UUID)
	require.NotNil(t, log.UserUUID)
	requireMarker(t, primary, externalUUIDPrimaryMigrationKey, true)
}

// TestRecoveryFromPrimaryMarkerOnlyState covers UUID-A07: a primary marker alone must not be
// reported as success while log state is incomplete.
func TestRecoveryFromPrimaryMarkerOnlyState(t *testing.T) {
	primary, logDB, topology := newSplitTestTopology(t)
	withFinalizerEnabled(t, true)

	require.NoError(t, markDataMigrationComplete(context.Background(), primary, externalUUIDPrimaryMigrationKey))
	require.NoError(t, primary.Exec("INSERT INTO users (id, username, password) VALUES (1, 'root', 'password-hash')").Error)
	require.NoError(t, logDB.Exec("INSERT INTO logs (id, user_id, type, content) VALUES (1, 1, 1, 'primary marker only')").Error)

	result, err := runFinalizer(t, topology)
	require.NoError(t, err)
	require.True(t, result.completed)

	var log Log
	require.NoError(t, logDB.First(&log, "id = ?", 1).Error)
	requireHyphenatedUUID(t, log.UUID)
	require.NotNil(t, log.UserUUID)
	requireMarker(t, logDB, externalUUIDLogMigrationKey, true)
}

// TestCompletedStartupIsMarkerOnly covers UUID-A08 and UUID-009: completed unified startup
// issues one marker lookup, completed split startup issues two, with zero UUID target or
// reference queries and zero migration writes.
func TestCompletedStartupIsMarkerOnly(t *testing.T) {
	t.Run("unified", func(t *testing.T) {
		db, topology := newUnifiedTestTopology(t)
		require.NoError(t, markDataMigrationComplete(context.Background(), db, externalUUIDPrimaryMigrationKey))

		counter := installQueryCounter(t, db)
		result := runCatchUp(t, topology)
		require.True(t, result.completed)

		require.Equal(t, 1, counter.count("data_migrations"), "exactly one marker lookup")
		requireNoUUIDTableAccess(t, counter, uuidRolePrimary)
		requireNoUUIDTableAccess(t, counter, uuidRoleLog)
	})

	t.Run("split", func(t *testing.T) {
		primary, logDB, topology := newSplitTestTopology(t)
		require.NoError(t, markDataMigrationComplete(context.Background(), primary, externalUUIDPrimaryMigrationKey))
		require.NoError(t, markDataMigrationComplete(context.Background(), logDB, externalUUIDLogMigrationKey))

		primaryCounter := installQueryCounter(t, primary)
		logCounter := installQueryCounter(t, logDB)
		result := runCatchUp(t, topology)
		require.True(t, result.completed)

		require.Equal(t, 1, primaryCounter.count("data_migrations"), "exactly one primary marker lookup")
		require.Equal(t, 1, logCounter.count("data_migrations"), "exactly one log marker lookup")
		requireNoUUIDTableAccess(t, primaryCounter, uuidRolePrimary)
		requireNoUUIDTableAccess(t, logCounter, uuidRoleLog)
	})
}

// TestTopologyIsNotInferredFromHandleIdentity covers UUID-A09 and UUID-002: a session clone or
// instrumentation wrapper must not change an explicitly selected topology mode.
func TestTopologyIsNotInferredFromHandleIdentity(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	require.Equal(t, uuidTopologyUnified, topology.mode)

	clone := db.Session(&gorm.Session{})
	require.NotSame(t, db, clone)
	cloned, err := newUnifiedTopology(clone)
	require.NoError(t, err)
	require.Equal(t, uuidTopologyUnified, cloned.mode, "a session clone must not select split mode")
	require.Equal(t, []uuidDBRole{uuidRolePrimary}, cloned.markerRoles())

	// Pointing both roles at one physical handle is still split when configuration says so.
	split, err := newSplitTopology(db, db)
	require.NoError(t, err)
	require.Equal(t, uuidTopologySplit, split.mode)
	require.Equal(t, []uuidDBRole{uuidRolePrimary, uuidRoleLog}, split.markerRoles())
}

// TestCoordinatorRejectsInvalidInput covers UUID-A10 and UUID-015.
func TestCoordinatorRejectsInvalidInput(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)

	_, err := runUUIDMigrationCoordinator(context.Background(), topology, uuidMigrationMode("bogus"))
	require.ErrorContains(t, err, "unknown external uuid migration mode")

	_, err = runUUIDMigrationCoordinator(context.Background(), nil, uuidMigrationModeCatchUp)
	require.ErrorContains(t, err, "not initialized")

	_, err = newUnifiedTopology(nil)
	require.ErrorContains(t, err, "primary database handle is nil")

	_, err = newSplitTopology(db, nil)
	require.ErrorContains(t, err, "log database handle is nil")

	// A schema-incomplete handle is rejected before any target access or marker write.
	bare := setupMigrationTestDB(t)
	bareTopology, err := newUnifiedTopology(bare)
	require.NoError(t, err)
	_, err = runUUIDMigrationCoordinator(context.Background(), bareTopology, uuidMigrationModeCatchUp)
	require.ErrorContains(t, err, "data_migrations")
}

// TestTargetBoundaryRowCounts covers UUID-A11: batch boundaries produce no skipped or
// duplicate updates.
func TestTargetBoundaryRowCounts(t *testing.T) {
	for _, total := range []int{0, 1, 999, 1000, 1001, 2001} {
		total := total
		t.Run(strconv.Itoa(total), func(t *testing.T) {
			db, topology := newUnifiedTestTopology(t)
			seedLegacyUsers(t, db, total)

			runCatchUp(t, topology)

			var filled int64
			require.NoError(t, db.Table("users").Where("uuid IS NOT NULL AND uuid != ''").Count(&filled).Error)
			require.EqualValues(t, total, filled)

			var distinct int64
			require.NoError(t, db.Raw("SELECT COUNT(DISTINCT uuid) FROM users WHERE uuid IS NOT NULL AND uuid != ''").Scan(&distinct).Error)
			require.EqualValues(t, total, distinct, "no duplicate uuid assignment")
		})
	}
}

// TestOrphansAndAmbiguityDoNotBlockCompletion covers UUID-A15 and UUID-A36.
func TestOrphansAndAmbiguityDoNotBlockCompletion(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, true)

	require.NoError(t, db.Exec("INSERT INTO users (id, username, password) VALUES (1, 'root', 'password-hash')").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, type, name, models, config) VALUES (1, 1, 'c', 'gpt-4o', '{}')").Error)
	// Two tokens share one (user_id, name): the key is permanently ambiguous.
	require.NoError(t, db.Exec("INSERT INTO tokens (id, user_id, `key`, name) VALUES (1, 1, 'k1', 'dup'), (2, 1, 'k2', 'dup'), (3, 1, 'k3', 'unique')").Error)
	require.NoError(t, db.Exec(`INSERT INTO logs (id, user_id, channel_id, type, token_name, content) VALUES
		(1, 999, 1, 1, 'unique', 'orphan user'),
		(2, 1, 1, 1, 'dup', 'ambiguous token'),
		(3, 1, 1, 1, '', 'empty token name'),
		(4, 1, 1, 1, 'unique', 'fillable')`).Error)
	require.NoError(t, db.Exec("INSERT INTO token_transactions (id, transaction_id, token_id, user_id, status, pre_quota, log_id) VALUES (1, 'txn-null-log', 3, 1, 1, 10, NULL)").Error)

	_, err := runFinalizer(t, topology)
	require.NoError(t, err, "orphans and ambiguous names must not block completion")

	var orphan Log
	require.NoError(t, db.First(&orphan, "id = ?", 1).Error)
	require.Nil(t, orphan.UserUUID, "an orphan reference stays unresolved")

	var ambiguous Log
	require.NoError(t, db.First(&ambiguous, "id = ?", 2).Error)
	require.Nil(t, ambiguous.TokenUUID, "an ambiguous token name stays unresolved")

	var emptyName Log
	require.NoError(t, db.First(&emptyName, "id = ?", 3).Error)
	require.Nil(t, emptyName.TokenUUID)

	// A later fillable row is not blocked by the unresolved rows before it.
	var fillable Log
	require.NoError(t, db.First(&fillable, "id = ?", 4).Error)
	require.NotNil(t, fillable.TokenUUID)

	var txn TokenTransaction
	require.NoError(t, db.First(&txn, "id = ?", 1).Error)
	require.Nil(t, txn.LogUUID, "an absent nullable reference stays unresolved")

	requireMarker(t, db, externalUUIDPrimaryMigrationKey, true)
}

// TestConditionalUpdateRechecksObservedReference covers UUID-A16 and invariants 3 and 4.
func TestConditionalUpdateRechecksObservedReference(t *testing.T) {
	t.Run("integer fk", func(t *testing.T) {
		db, _ := newUnifiedTestTopology(t)
		require.NoError(t, db.Exec("INSERT INTO users (id, uuid, username, password) VALUES (1, '018f0000-0000-7000-8000-000000000001', 'root', 'password-hash'), (2, '018f0000-0000-7000-8000-000000000002', 'other', 'password-hash')").Error)
		require.NoError(t, db.Exec("INSERT INTO logs (id, user_id, type, content) VALUES (1, 1, 1, 'moving ref')").Error)

		rows := []uuidRefRow{{Id: 1, RefID: 1}}
		// The FK moves after the candidate read.
		require.NoError(t, db.Model(&Log{}).Where("id = ?", 1).Update("user_id", 2).Error)

		target := uuidFKTarget{role: uuidRoleLog, table: "logs", model: &Log{}, fkColumn: "user_id",
			uuidColumn: "user_uuid", refRole: uuidRolePrimary, refTable: "users", resolver: uuidResolverIntFK}
		updated, err := applyFKUUIDRows(context.Background(), db, db, target, rows)
		require.NoError(t, err)
		require.Zero(t, updated, "a stale observed FK must not be written")

		var logRow Log
		require.NoError(t, db.First(&logRow, "id = ?", 1).Error)
		require.Nil(t, logRow.UserUUID)
	})

	t.Run("token name", func(t *testing.T) {
		db, topology := newUnifiedTestTopology(t)
		require.NoError(t, db.Exec("INSERT INTO users (id, uuid, username, password) VALUES (1, '018f0000-0000-7000-8000-000000000001', 'root', 'password-hash')").Error)
		require.NoError(t, db.Exec("INSERT INTO tokens (id, uuid, user_id, `key`, name) VALUES (1, '018f0000-0000-7000-8000-000000000003', 1, 'k', 'alpha')").Error)
		require.NoError(t, db.Exec("INSERT INTO logs (id, user_id, type, token_name, content) VALUES (1, 1, 1, 'alpha', 'renamed')").Error)

		run := &uuidMigrationRun{topology: topology, mode: uuidMigrationModeCatchUp}
		values := []uuidConditionalValue{{
			id: 1,
			conditions: []uuidColumnValue{
				{column: "user_id", value: 1},
				{column: "token_name", value: "alpha"},
			},
			value: "018f0000-0000-7000-8000-000000000003",
		}}
		// token_name changes after the candidate read.
		require.NoError(t, db.Model(&Log{}).Where("id = ?", 1).Update("token_name", "beta").Error)
		updated, err := applyConditionalStringColumnRows(context.Background(), db, "logs", "token_uuid", values)
		require.NoError(t, err)
		require.Zero(t, updated, "a stale observed token_name must not be written")
		_ = run
	})
}

// TestPopulatedUUIDsAreNeverOverwritten covers invariants 1 and 2.
func TestPopulatedUUIDsAreNeverOverwritten(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)

	ownedUUID := "018f0000-0000-7000-8000-000000000001"
	otherUUID := "018f0000-0000-7000-8000-000000000002"
	require.NoError(t, db.Exec("INSERT INTO users (id, uuid, username, password) VALUES (1, ?, 'root', 'password-hash'), (2, ?, 'other', 'password-hash')", ownedUUID, otherUUID).Error)
	// tokens.user_uuid deliberately points at the wrong user; catch-up must not silently fix it.
	require.NoError(t, db.Exec("INSERT INTO tokens (id, uuid, user_id, user_uuid, `key`, name) VALUES (1, '018f0000-0000-7000-8000-000000000003', 1, ?, 'k', 'n')", otherUUID).Error)

	runCatchUp(t, topology)

	var user User
	require.NoError(t, db.First(&user, "id = ?", 1).Error)
	require.Equal(t, ownedUUID, user.UUID, "a populated owned uuid is immutable")

	var token Token
	require.NoError(t, db.First(&token, "id = ?", 1).Error)
	require.NotNil(t, token.UserUUID)
	require.Equal(t, otherUUID, *token.UserUUID, "catch-up must not overwrite a populated fk uuid")

	// The mismatch is a finalization blocker, not something catch-up repairs.
	err := validateExternalUUIDs(context.Background(), topology)
	require.ErrorContains(t, err, "populated fk uuid disagrees with live owner")
}

// TestMalformedAndDuplicateOwnedUUIDsBlockFinalization covers UUID-A19, UUID-A23, and UUID-A44.
func TestMalformedAndDuplicateOwnedUUIDsBlockFinalization(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		db, topology := newUnifiedTestTopology(t)
		withFinalizerEnabled(t, true)
		require.NoError(t, db.Exec("INSERT INTO users (id, uuid, username, password) VALUES (1, 'not-a-uuid', 'root', 'password-hash')").Error)

		runCatchUp(t, topology)
		var user User
		require.NoError(t, db.First(&user, "id = ?", 1).Error)
		require.Equal(t, "not-a-uuid", user.UUID, "catch-up preserves a malformed legacy value")

		_, err := runFinalizer(t, topology)
		require.ErrorContains(t, err, "malformed owned uuid")
		requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)
		require.False(t, db.Migrator().HasIndex(&User{}, uuidUniqueIndexName("users")))
		require.True(t, db.Migrator().HasIndex(&User{}, ordinaryUUIDIndexName("users")),
			"the ordinary index must survive a failed promotion")
	})

	t.Run("duplicate", func(t *testing.T) {
		db, topology := newUnifiedTestTopology(t)
		withFinalizerEnabled(t, true)
		shared := "018f0000-0000-7000-8000-000000000001"
		require.NoError(t, db.Exec("INSERT INTO users (id, uuid, username, password) VALUES (1, ?, 'root', 'password-hash'), (2, ?, 'other', 'password-hash')", shared, shared).Error)

		_, err := runFinalizer(t, topology)
		require.ErrorContains(t, err, "duplicate owned uuid")
		requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)
		require.False(t, db.Migrator().HasIndex(&User{}, uuidUniqueIndexName("users")))
		require.True(t, db.Migrator().HasIndex(&User{}, ordinaryUUIDIndexName("users")))
	})
}

// TestRepeatedCatchUpIsStable covers UUID-A24.
func TestRepeatedCatchUpIsStable(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	seedUnifiedLegacyRows(t, db)

	runCatchUp(t, topology)
	var first []User
	require.NoError(t, db.Order("id").Find(&first).Error)
	require.Len(t, first, 2)

	for i := 0; i < 4; i++ {
		result := runCatchUp(t, topology)
		require.Zero(t, result.updated, "a settled catch-up pass rewrites nothing")
		var users []User
		require.NoError(t, db.Order("id").Find(&users).Error)
		require.Equal(t, first[0].UUID, users[0].UUID, "uuids stay stable across restarts")
		require.Equal(t, first[1].UUID, users[1].UUID)
		requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)
	}
}

// TestCancellationWritesNoMarker covers UUID-A18.
func TestCancellationWritesNoMarker(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, true)
	seedLegacyUsers(t, db, 50)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runUUIDMigrationCoordinator(ctx, topology, uuidMigrationModeFinalizer)
	require.Error(t, err)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)
}

// TestCatchUpBudgetBoundsOneCycle covers UUID-A37: a cycle stops at its row budget and the
// next cycle resumes the remaining work.
func TestCatchUpBudgetBoundsOneCycle(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	seedLegacyUsers(t, db, 2500)

	run := &uuidMigrationRun{topology: topology, mode: uuidMigrationModeCatchUp,
		budget: newUUIDCatchUpBudget(1000, uuidCatchUpTimeBudget())}
	require.NoError(t, backfillOwnedUUIDsForRole(context.Background(), run, uuidRolePrimary))
	require.True(t, run.budget.spent(), "the row budget must stop the cycle")
	require.LessOrEqual(t, run.updated, 2000, "a bounded cycle cannot drain the whole backlog")

	var remaining int64
	require.NoError(t, db.Table("users").Where("uuid IS NULL OR uuid = ''").Count(&remaining).Error)
	require.Positive(t, remaining)

	// A later unbounded cycle finishes the rest.
	runCatchUp(t, topology)
	require.NoError(t, db.Table("users").Where("uuid IS NULL OR uuid = ''").Count(&remaining).Error)
	require.Zero(t, remaining)
}

// TestCatchUpBudgetRespectsDeadline verifies the time half of the cycle budget.
func TestCatchUpBudgetRespectsDeadline(t *testing.T) {
	budget := newUUIDCatchUpBudget(uuidCatchUpRowBudget(), time.Nanosecond)
	time.Sleep(time.Millisecond)
	require.True(t, budget.consume(1), "an expired deadline must stop the cycle")
	require.True(t, budget.spent())
}

// seedLegacyUsers inserts count legacy users with no UUID values.
func seedLegacyUsers(t *testing.T, db *gorm.DB, count int) {
	t.Helper()
	const chunk = 200
	for start := 1; start <= count; start += chunk {
		end := start + chunk - 1
		if end > count {
			end = count
		}
		rows := make([]map[string]any, 0, end-start+1)
		for id := start; id <= end; id++ {
			rows = append(rows, map[string]any{
				"id": id, "username": "user-" + strconv.Itoa(id), "password": "password-hash",
			})
		}
		require.NoError(t, db.Table("users").Create(rows).Error)
	}
}
