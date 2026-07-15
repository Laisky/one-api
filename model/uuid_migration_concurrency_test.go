package model

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// uuidRaceWorkerCount is the number of coordinator goroutines raced against one topology.
const uuidRaceWorkerCount = 4

// pinUUIDRaceSQLiteConnection forces every statement of a concurrent test onto one connection.
//
// setupMigrationTestDB opens ":memory:", where each new SQLite connection is a brand new empty
// database rather than another view of the same one. Concurrent goroutines sharing a *gorm.DB
// would therefore either observe missing tables on a second connection or fail with
// "database table is locked". Capping the pool at a single connection makes the shared handle
// the single physical database and serializes the goroutines at the driver, which is exactly
// the isolation a real backend provides. MaxIdleConns is pinned to 1 as well so the one
// connection is never released and the in-memory database is never destroyed mid-test.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle whose underlying pool is pinned.
//
// Return values: none.
func pinUUIDRaceSQLiteConnection(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
}

// seedUUIDRaceLegacyTokens inserts count legacy tokens, token id N owned by user id N.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle receiving the rows.
//   - count: number of tokens to insert.
//
// Return values: none.
func seedUUIDRaceLegacyTokens(t *testing.T, db *gorm.DB, count int) {
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
				"id": id, "user_id": id,
				// "key" is reserved; the map form lets GORM quote it for the dialect.
				"key":  "legacy-key-" + strconv.Itoa(id),
				"name": "token-" + strconv.Itoa(id),
			})
		}
		require.NoError(t, db.Table("tokens").Create(rows).Error)
	}
}

// uuidRaceLoadPopulatedUUIDs returns every populated UUID of a table keyed by row id.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle owning the table.
//   - table: trusted target table name.
//
// Return values:
//   - map[int]string: row id to canonical external UUID for populated rows only.
func uuidRaceLoadPopulatedUUIDs(t *testing.T, db *gorm.DB, table string) map[int]string {
	t.Helper()
	rows := []struct {
		Id   int    `gorm:"column:id"`
		UUID string `gorm:"column:uuid"`
	}{}
	require.NoError(t, db.Table(table).
		Select("id, uuid").
		Where("uuid IS NOT NULL AND uuid != ''").
		Order("id ASC").
		Find(&rows).Error)

	populated := make(map[int]string, len(rows))
	for _, row := range rows {
		requireHyphenatedUUID(t, row.UUID)
		populated[row.Id] = row.UUID
	}
	return populated
}

// uuidRaceCountDistinctUUIDs returns the number of distinct populated UUIDs in a table.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle owning the table.
//   - table: trusted target table name.
//
// Return values:
//   - int: distinct populated UUID values.
func uuidRaceCountDistinctUUIDs(t *testing.T, db *gorm.DB, table string) int {
	t.Helper()
	var distinct int64
	require.NoError(t, db.Raw("SELECT COUNT(DISTINCT uuid) FROM "+table+
		" WHERE uuid IS NOT NULL AND uuid != ''").Scan(&distinct).Error)
	return int(distinct)
}

// runUUIDRaceCatchUpWorkers races count catch-up coordinators over one topology.
// Assertions never run inside the goroutines because testify's require calls t.FailNow,
// which is only legal on the test goroutine; each worker writes to its own slice index
// instead, which keeps the fan-out clean under -race.
// Parameters:
//   - topology: topology under test.
//   - count: number of concurrent coordinator goroutines.
//
// Return values:
//   - []uuidMigrationResult: per-worker coordinator results.
//   - []error: per-worker coordinator errors.
func runUUIDRaceCatchUpWorkers(topology *databaseTopology, count int) ([]uuidMigrationResult, []error) {
	results := make([]uuidMigrationResult, count)
	errs := make([]error, count)

	start := make(chan struct{})
	wg := sync.WaitGroup{}
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			// Releasing every worker from one barrier maximizes the overlap of the
			// candidate reads that the conditional update must arbitrate.
			<-start
			results[worker], errs[worker] = runUUIDMigrationCoordinator(
				context.Background(), topology, uuidMigrationModeCatchUp)
		}(i)
	}
	close(start)
	wg.Wait()
	return results, errs
}

// TestConcurrentCatchUpWorkersAssignDistinctUUIDs covers UUID-A17: concurrent catch-up workers
// racing on the same backlog all succeed, every target row ends up populated exactly once with a
// distinct UUID, and a settled follow-up cycle rewrites nothing.
func TestConcurrentCatchUpWorkersAssignDistinctUUIDs(t *testing.T) {
	const rowCount = 300

	db, topology := newUnifiedTestTopology(t)
	pinUUIDRaceSQLiteConnection(t, db)
	seedLegacyUsers(t, db, rowCount)
	seedUUIDRaceLegacyTokens(t, db, rowCount)

	_, errs := runUUIDRaceCatchUpWorkers(topology, uuidRaceWorkerCount)
	for worker, err := range errs {
		require.NoError(t, err, "catch-up worker %d must not fail on a marker or index race", worker)
	}

	users := uuidRaceLoadPopulatedUUIDs(t, db, "users")
	require.Len(t, users, rowCount, "every user must end up with a uuid")
	require.Equal(t, rowCount, uuidRaceCountDistinctUUIDs(t, db, "users"),
		"a losing worker must never assign a second uuid to a row")

	tokens := uuidRaceLoadPopulatedUUIDs(t, db, "tokens")
	require.Len(t, tokens, rowCount, "every token must end up with a uuid")
	require.Equal(t, rowCount, uuidRaceCountDistinctUUIDs(t, db, "tokens"))

	// Every denormalized FK uuid must agree with the live owner the workers raced to fill.
	var mismatched int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM tokens t JOIN users u ON u.id = t.user_id
		WHERE t.user_uuid IS NULL OR t.user_uuid = '' OR t.user_uuid != u.uuid`).Scan(&mismatched).Error)
	require.Zero(t, mismatched, "concurrent workers must leave every fk uuid consistent with its owner")

	// A settled backlog is a no-op: nothing is rewritten and catch-up still writes no marker.
	settled := runCatchUp(t, topology)
	require.Zero(t, settled.updated, "a settled catch-up pass must update 0 rows")
	require.Equal(t, users, uuidRaceLoadPopulatedUUIDs(t, db, "users"),
		"a settled catch-up pass must not change any assigned uuid")
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)
}

// TestConcurrentCatchUpNeverOverwritesPopulatedUUIDs covers UUID-A17 and invariants 1 and 2: under
// concurrency a populated owned UUID stays immutable and a populated FK UUID is never silently
// overwritten, even when it disagrees with the live owner.
func TestConcurrentCatchUpNeverOverwritesPopulatedUUIDs(t *testing.T) {
	const legacyCount = 200

	db, topology := newUnifiedTestTopology(t)
	pinUUIDRaceSQLiteConnection(t, db)
	seedLegacyUsers(t, db, legacyCount)
	seedUUIDRaceLegacyTokens(t, db, legacyCount)

	ownerUUID := "018f0000-0000-7000-8000-000000000001"
	otherUUID := "018f0000-0000-7000-8000-000000000002"
	tokenUUID := "018f0000-0000-7000-8000-000000000003"
	require.NoError(t, db.Exec(`INSERT INTO users (id, uuid, username, password) VALUES
		(900, ?, 'pinned-owner', 'password-hash'), (901, ?, 'pinned-other', 'password-hash')`,
		ownerUUID, otherUUID).Error)
	// tokens.user_uuid deliberately points at the wrong user: catch-up must not repair it.
	require.NoError(t, db.Exec("INSERT INTO tokens (id, uuid, user_id, user_uuid, `key`, name) VALUES (900, ?, 900, ?, 'pinned-key', 'pinned')",
		tokenUUID, otherUUID).Error)

	_, errs := runUUIDRaceCatchUpWorkers(topology, uuidRaceWorkerCount)
	for worker, err := range errs {
		require.NoError(t, err, "catch-up worker %d must not fail", worker)
	}

	var pinnedOwner User
	require.NoError(t, db.First(&pinnedOwner, "id = ?", 900).Error)
	require.Equal(t, ownerUUID, pinnedOwner.UUID, "a populated owned uuid is immutable under concurrency")

	var pinnedOther User
	require.NoError(t, db.First(&pinnedOther, "id = ?", 901).Error)
	require.Equal(t, otherUUID, pinnedOther.UUID, "a populated owned uuid is immutable under concurrency")

	var pinnedToken Token
	require.NoError(t, db.First(&pinnedToken, "id = ?", 900).Error)
	require.Equal(t, tokenUUID, pinnedToken.UUID, "a populated owned uuid is immutable under concurrency")
	require.NotNil(t, pinnedToken.UserUUID)
	require.Equal(t, otherUUID, *pinnedToken.UserUUID,
		"catch-up must not silently overwrite a populated fk uuid under concurrency")

	// The legacy backlog around the pinned rows still drains to distinct uuids.
	users := uuidRaceLoadPopulatedUUIDs(t, db, "users")
	require.Len(t, users, legacyCount+2)
	require.Equal(t, legacyCount+2, uuidRaceCountDistinctUUIDs(t, db, "users"))
	require.Equal(t, ownerUUID, users[900])
	require.Equal(t, otherUUID, users[901])

	// The disagreement is a finalization blocker, not something catch-up repairs.
	require.ErrorContains(t, validateExternalUUIDs(context.Background(), topology),
		"populated fk uuid disagrees with live owner")
}

// TestConcurrentMarkerWritesAreIdempotent covers UUID-A17: concurrent workers racing to write the
// same completion marker all succeed, produce exactly one row, and a second round preserves the
// original completion timestamp.
func TestConcurrentMarkerWritesAreIdempotent(t *testing.T) {
	const writerCount = 8

	db := setupMigrationTestDB(t)
	require.NoError(t, db.AutoMigrate(&DataMigration{}))
	pinUUIDRaceSQLiteConnection(t, db)

	key := externalUUIDPrimaryMigrationKey
	markConcurrently := func() []error {
		errs := make([]error, writerCount)
		start := make(chan struct{})
		wg := sync.WaitGroup{}
		for i := 0; i < writerCount; i++ {
			wg.Add(1)
			go func(writer int) {
				defer wg.Done()
				<-start
				errs[writer] = markDataMigrationComplete(context.Background(), db, key)
			}(i)
		}
		close(start)
		wg.Wait()
		return errs
	}

	for writer, err := range markConcurrently() {
		require.NoError(t, err, "marker writer %d must tolerate a duplicate-object race", writer)
	}

	var count int64
	require.NoError(t, db.Model(&DataMigration{}).Where("migration_key = ?", key).Count(&count).Error)
	require.EqualValues(t, 1, count, "a marker race must leave exactly one row")

	var first DataMigration
	require.NoError(t, db.Where("migration_key = ?", key).First(&first).Error)
	require.False(t, first.CompletedAt.IsZero())

	// A second round is a marker-only no-op that must not restamp the marker.
	for writer, err := range markConcurrently() {
		require.NoError(t, err, "marker writer %d must be a no-op on a present marker", writer)
	}

	require.NoError(t, db.Model(&DataMigration{}).Where("migration_key = ?", key).Count(&count).Error)
	require.EqualValues(t, 1, count, "a repeated marker write must not insert a second row")

	var second DataMigration
	require.NoError(t, db.Where("migration_key = ?", key).First(&second).Error)
	require.True(t, first.CompletedAt.Equal(second.CompletedAt),
		"a duplicate insertion must preserve the original completion timestamp: %s != %s",
		first.CompletedAt, second.CompletedAt)
}

// TestConcurrentIndexCreationIsIdempotent covers UUID-A17: concurrent workers racing to create the
// same catch-up index all succeed and the index exists exactly once.
func TestConcurrentIndexCreationIsIdempotent(t *testing.T) {
	const creatorCount = 8
	const indexName = "idx_test_race_uuid"

	db, _ := newUnifiedTestTopology(t)
	pinUUIDRaceSQLiteConnection(t, db)
	require.False(t, hasIndexNamed(context.Background(), db, "users", indexName))

	errs := make([]error, creatorCount)
	start := make(chan struct{})
	wg := sync.WaitGroup{}
	for i := 0; i < creatorCount; i++ {
		wg.Add(1)
		go func(creator int) {
			defer wg.Done()
			<-start
			errs[creator] = ensureNonUniqueIndex(context.Background(), db, "users", indexName, []string{"uuid"})
		}(i)
	}
	close(start)
	wg.Wait()

	for creator, err := range errs {
		require.NoError(t, err, "index creator %d must tolerate a duplicate-object race", creator)
	}

	require.True(t, hasIndexNamed(context.Background(), db, "users", indexName))
	var indexes int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?",
		indexName).Scan(&indexes).Error)
	require.EqualValues(t, 1, indexes, "a losing creator must not add a second index")
}

// TestCancelledFinalizerResumesIdempotently covers UUID-A18: a finalizer cancelled after a
// committed batch fails, writes no marker, and a later finalizer resumes to completion without
// rewriting any UUID that the interrupted run had already assigned.
func TestCancelledFinalizerResumesIdempotently(t *testing.T) {
	const rowCount = 2000

	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, true)
	seedLegacyUsers(t, db, rowCount)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var armed atomic.Bool
	var commits atomic.Int64
	armed.Store(true)
	t.Cleanup(func() { armed.Store(false) })

	// The cancellation must land on a durably committed batch, so it is registered after GORM's
	// commit callback rather than after "gorm:update". Cancelling from the earlier hook would
	// race the pending COMMIT through database/sql's Tx watchdog and could roll the batch back,
	// which is a different failure mode than the mid-run interruption under test.
	require.NoError(t, db.Callback().Update().
		After("gorm:commit_or_rollback_transaction").
		Register("uuidtest:cancel_after_first_commit", func(tx *gorm.DB) {
			if !armed.Load() || tx.Error != nil {
				return
			}
			if commits.Add(1) == 1 {
				cancel()
			}
		}))

	_, err := runUUIDMigrationCoordinator(ctx, topology, uuidMigrationModeFinalizer)
	require.Error(t, err, "an interrupted finalizer must not report success")
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)
	armed.Store(false)

	interrupted := uuidRaceLoadPopulatedUUIDs(t, db, "users")
	require.NotEmpty(t, interrupted, "the cancelled run must leave its committed batch behind")
	require.Less(t, len(interrupted), rowCount, "the cancelled run must not have drained the backlog")

	// A fresh finalizer resumes the remaining work from the durable UUID column work queue.
	_, err = runFinalizer(t, topology)
	require.NoError(t, err, "a resumed finalizer must complete")

	final := uuidRaceLoadPopulatedUUIDs(t, db, "users")
	require.Len(t, final, rowCount, "resume must fill every remaining row")
	require.Equal(t, rowCount, uuidRaceCountDistinctUUIDs(t, db, "users"),
		"resume must not duplicate a uuid across the interrupted boundary")
	for id, uuid := range interrupted {
		require.Equal(t, uuid, final[id], "resume must not rewrite the completed uuid of user %d", id)
	}
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, true)
}

// TestMarkDataMigrationCompleteRejectsNonDuplicateError covers UUID-A18: a genuine database
// failure is never converted into an idempotent duplicate-object success.
func TestMarkDataMigrationCompleteRejectsNonDuplicateError(t *testing.T) {
	// A handle with no data_migrations table: every marker statement is a hard failure.
	db := setupMigrationTestDB(t)

	err := markDataMigrationComplete(context.Background(), db, externalUUIDPrimaryMigrationKey)
	require.Error(t, err, "a missing marker table must never be swallowed as a duplicate race")
	require.ErrorContains(t, err, "data_migrations")
	require.False(t, isDuplicateObjectError(err),
		"a missing-table failure must not classify as a duplicate-object race")

	// The failure is also never mistaken for a written marker.
	_, err = isDataMigrationComplete(context.Background(), db, externalUUIDPrimaryMigrationKey)
	require.Error(t, err)

	require.ErrorContains(t, markDataMigrationComplete(context.Background(), nil, externalUUIDPrimaryMigrationKey),
		"database is nil")
}

// TestIsDuplicateObjectErrorClassification covers UUID-A18: only true duplicate-object messages
// may be converted into idempotent success, and every unrelated failure is rejected.
func TestIsDuplicateObjectErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{
			name: "sqlite unique constraint",
			err:  errors.New("UNIQUE constraint failed: data_migrations.migration_key"),
			want: true,
		},
		{
			name: "sqlite constraint failed unique",
			err:  errors.New("constraint failed: UNIQUE constraint failed"),
			want: true,
		},
		{
			name: "mysql duplicate entry",
			err:  errors.New("Error 1062 (23000): Duplicate entry 'external_uuid_backfill_v3_primary' for key 'PRIMARY'"),
			want: true,
		},
		{
			name: "mysql duplicate key name",
			err:  errors.New("Error 1061 (42000): Duplicate key name 'idx_users_uuid'"),
			want: true,
		},
		{
			name: "postgres unique violation",
			err:  errors.New(`ERROR: duplicate key value violates unique constraint "data_migrations_pkey" (SQLSTATE 23505)`),
			want: true,
		},
		{
			name: "postgres duplicate relation",
			err:  errors.New(`ERROR: relation "idx_users_uuid" already exists (SQLSTATE 42P07)`),
			want: true,
		},
		{
			name: "unrelated connection failure",
			err:  errors.New("dial tcp 127.0.0.1:5432: connect: connection refused"),
			want: false,
		},
		{
			name: "unrelated lock failure",
			err:  errors.New("database table is locked"),
			want: false,
		},
		{
			name: "unrelated missing table",
			err:  errors.New("no such table: data_migrations"),
			want: false,
		},
		{
			name: "wrapped duplicate stays classified",
			err:  errors.Wrap(errors.New("UNIQUE constraint failed: users.uuid"), "insert data migration marker"),
			want: true,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isDuplicateObjectError(tc.err))
		})
	}
}
