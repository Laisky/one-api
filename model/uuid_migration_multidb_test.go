package model

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestUUIDMigrationMatrixOnLiveBackends covers UUID-A25: it runs the unified and
// split-database migration matrix on live MySQL and PostgreSQL with the finalizer both
// disabled and enabled, asserting owned and denormalized FK UUIDs are filled in every cell and
// that completion markers appear only when the finalizer was enabled.
func TestUUIDMigrationMatrixOnLiveBackends(t *testing.T) {
	for _, backend := range uuidMultiDBBackends {
		for _, mode := range []uuidTopologyMode{uuidTopologyUnified, uuidTopologySplit} {
			for _, finalizer := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/finalizer=%t", backend, mode, finalizer), func(t *testing.T) {
					fixture := newUUIDMultiDBFixture(t, backend, mode)
					withFinalizerEnabled(t, finalizer)
					fixture.seedLegacyRows(t)

					if finalizer {
						result, err := runFinalizer(t, fixture.topology)
						require.NoError(t, err, "%s %s finalization", backend, mode)
						require.True(t, result.completed, "an enabled finalizer must report completion")
					} else {
						result := runCatchUp(t, fixture.topology)
						require.False(t, result.completed, "catch-up must never report completion")
					}

					fixture.requireLegacyRowsReconciled(t)
					fixture.requireMarkers(t, finalizer)
				})
			}
		}
	}
}

// TestUUIDCandidateAndReferenceQueryPlans covers UUID-A26: it proves with dialect-specific
// EXPLAIN evidence that the NULL and empty-string candidate queries are index-driven against
// the UUID candidate index, that the bounded reference query is a primary key lookup, and that
// the bounded token lookup uses the (user_id, name) composite index. Every plan is logged so it
// can be attached to release evidence.
func TestUUIDCandidateAndReferenceQueryPlans(t *testing.T) {
	for _, backend := range uuidMultiDBBackends {
		t.Run(backend, func(t *testing.T) {
			fixture := newUUIDMultiDBFixture(t, backend, uuidTopologyUnified)
			db := fixture.primary

			// The candidate indexes exist for the steady state where a large, mostly reconciled
			// table still has a handful of rows to fill, so that is what is seeded: an
			// all-missing table would make a full scan the genuinely correct plan and prove
			// nothing about sargability.
			seedUUIDLiveUsers(t, db, "plan", uuidPlanUserRows, uuidPlanMissingPerPass)
			seedUUIDLiveTokens(t, db, uuidPlanTokenRows)

			run := &uuidMigrationRun{topology: fixture.topology, mode: uuidMigrationModeCatchUp}
			require.NoError(t, ensureUUIDCandidateIndexes(context.Background(), run),
				"create the catch-up candidate indexes")
			require.True(t, hasIndexNamed(context.Background(), db, "users", ordinaryUUIDIndexName("users")),
				"the uuid candidate index must exist before its plan is measured")
			require.True(t, hasIndexNamed(context.Background(), db, "tokens", tokenNameLookupIndexName),
				"the token name lookup index must exist before its plan is measured")

			uuidAnalyzeTable(t, db, "users")
			uuidAnalyzeTable(t, db, "tokens")

			idColumn := quoteIdentifier(db, "id")
			uuidColumn := quoteIdentifier(db, "uuid")

			// The NULL pass and the empty-string pass, exactly as backfillOwnedUUIDs issues them.
			for _, missing := range missingStringPredicates(db, "uuid") {
				label := "users candidate query [" + missing + "]"
				candidate := "SELECT " + idColumn +
					" FROM " + quoteIdentifier(db, "users") +
					" WHERE " + idColumn + " > 0 AND " + missing +
					" ORDER BY " + idColumn + " ASC" +
					" LIMIT " + strconv.Itoa(uuidBackfillBatchSize)

				plan := requireIndexedPlan(t, db, candidate, label)
				// MySQL's prefer_ordering_index heuristic may legitimately choose the primary
				// key to satisfy ORDER BY id LIMIT instead of the uuid index; both are indexed
				// plans, and possible_keys is the direct evidence that the predicate itself is
				// sargable against the uuid index. PostgreSQL is asserted on the chosen index.
				requirePlanConsidersIndex(t, db, plan, ordinaryUUIDIndexName("users"), label)
			}

			// The bounded reference query, exactly as loadUUIDReferences issues it.
			referenceIDs := make([]string, 0, 10)
			for id := 1; id <= 10; id++ {
				referenceIDs = append(referenceIDs, strconv.Itoa(id))
			}
			referenceLabel := "bounded user reference query"
			reference := "SELECT " + idColumn + ", " + uuidColumn +
				" FROM " + quoteIdentifier(db, "users") +
				" WHERE " + idColumn + " IN (" + strings.Join(referenceIDs, ", ") + ")" +
				" AND " + uuidColumn + " IS NOT NULL AND " + uuidColumn + " != ''"
			referencePlan := requireIndexedPlan(t, db, reference, referenceLabel)
			requirePlanUsesIndex(t, db, referencePlan, uuidPrimaryKeyIndexName(db, "users"), referenceLabel)

			// The bounded composite token lookup, exactly as resolveTokenUUIDsForKeys issues it.
			userIDColumn := quoteIdentifier(db, "user_id")
			nameColumn := quoteIdentifier(db, "name")
			tokenLabel := "bounded token composite lookup"
			tokenLookup := "SELECT " + userIDColumn + ", " + nameColumn +
				", MIN(" + uuidColumn + ") AS " + quoteIdentifier(db, "uuid") +
				", COUNT(*) AS " + quoteIdentifier(db, "total") +
				" FROM " + quoteIdentifier(db, "tokens") +
				" WHERE " + uuidColumn + " IS NOT NULL AND " + uuidColumn + " != ''" +
				" AND " + nameColumn + " != ''" +
				" AND ((" + userIDColumn + " = 1 AND " + nameColumn + " = 'plan-token-1')" +
				" OR (" + userIDColumn + " = 2 AND " + nameColumn + " = 'plan-token-2'))" +
				" GROUP BY " + userIDColumn + ", " + nameColumn
			tokenPlan := requireIndexedPlan(t, db, tokenLookup, tokenLabel)
			requirePlanUsesIndex(t, db, tokenPlan, tokenNameLookupIndexName, tokenLabel)
		})
	}
}

// uuidConcurrentWorkload drives reads and writes against users while migration DDL runs.
// It never touches the *testing.T from its goroutine: errors travel over a channel and
// observations over atomics, so the whole test stays safe under -race.
type uuidConcurrentWorkload struct {
	// stopCh is closed to end the loop.
	stopCh chan struct{}
	// doneCh carries the loop's terminal error, or nil.
	doneCh chan error
	// maxStall is the longest observed read/write round trip, in nanoseconds.
	maxStall atomic.Int64
	// ops counts completed read/write cycles.
	ops atomic.Int64
}

// startUUIDConcurrentWorkload begins a background read/write loop against the users table.
// Every inserted row carries a distinct canonical UUID so the workload cannot itself violate
// the unique index being built.
// Parameters:
//   - t: test handle used only for helper marking.
//   - db: live handle to drive traffic through.
//
// Return values:
//   - *uuidConcurrentWorkload: running workload; call stop to collect its observations.
func startUUIDConcurrentWorkload(t *testing.T, db *gorm.DB) *uuidConcurrentWorkload {
	t.Helper()
	workload := &uuidConcurrentWorkload{
		stopCh: make(chan struct{}),
		doneCh: make(chan error, 1),
	}
	go func() {
		nextID := uuidWorkloadFirstID
		for {
			select {
			case <-workload.stopCh:
				workload.doneCh <- nil
				return
			default:
			}

			started := time.Now()
			var observed int64
			if err := db.Table("users").Where("id = ?", 1).Count(&observed).Error; err != nil {
				workload.doneCh <- err
				return
			}
			if err := db.Table("users").Create(map[string]any{
				"id":       nextID,
				"username": "workload-user-" + strconv.Itoa(nextID),
				"password": "password-hash",
				"uuid":     uuidMultiDBFixtureUUID(nextID),
			}).Error; err != nil {
				workload.doneCh <- err
				return
			}
			if err := db.Table("users").Where("id = ?", nextID).
				Update("display_name", "workload").Error; err != nil {
				workload.doneCh <- err
				return
			}
			nextID++

			workload.recordStall(time.Since(started))
			workload.ops.Add(1)
		}
	}()
	return workload
}

// recordStall keeps the longest observed read/write round trip.
// Parameters:
//   - stall: duration of the last cycle.
//
// Return values: none.
func (workload *uuidConcurrentWorkload) recordStall(stall time.Duration) {
	for {
		current := workload.maxStall.Load()
		if int64(stall) <= current || workload.maxStall.CompareAndSwap(current, int64(stall)) {
			return
		}
	}
}

// stop halts the workload and reports what it observed.
// Parameters: none.
//
// Return values:
//   - time.Duration: longest observed read/write round trip.
//   - int64: completed read/write cycles.
//   - error: the loop's terminal error, or nil.
func (workload *uuidConcurrentWorkload) stop() (time.Duration, int64, error) {
	close(workload.stopCh)
	err := <-workload.doneCh
	return time.Duration(workload.maxStall.Load()), workload.ops.Load(), err
}

// TestOnlineUniqueIndexPromotionDoesNotBlockTraffic covers UUID-A27: unique-index promotion
// succeeds while concurrent reads and writes continue, and never stalls them anywhere near the
// configured DDL lock timeout. On PostgreSQL this exercises CREATE UNIQUE INDEX CONCURRENTLY;
// on MySQL it exercises ALTER TABLE ... ALGORITHM=INPLACE, LOCK=NONE.
func TestOnlineUniqueIndexPromotionDoesNotBlockTraffic(t *testing.T) {
	for _, backend := range uuidMultiDBBackends {
		t.Run(backend, func(t *testing.T) {
			fixture := newUUIDMultiDBFixture(t, backend, uuidTopologyUnified)
			// A modest row count keeps CI fast while still forcing a real index build.
			seedUUIDLiveUsers(t, fixture.primary, "promote", uuidPromotionUserRows, 0)

			workload := startUUIDConcurrentWorkload(t, fixture.primary)
			promoteErr := promoteUUIDUniqueIndexes(context.Background(), fixture.topology)
			maxStall, ops, workloadErr := workload.stop()

			require.NoError(t, promoteErr, "online unique index promotion must succeed on %s", backend)
			require.NoError(t, workloadErr, "concurrent reads and writes must not fail during promotion")
			require.Positive(t, ops, "the concurrent workload must have made progress during promotion")

			t.Logf("%s online promotion: concurrent_ops=%d max_observed_stall=%s lock_timeout=%s ddl_timeout=%s",
				backend, ops, maxStall, uuidLockTimeout(), uuidDDLTimeout())
			// The acceptance protocol caps lock acquisition at five seconds, which is also
			// the configured lock-timeout default; the 30-minute statement timeout is not
			// the relevant bound for traffic impact.
			require.Less(t, maxStall, uuidLockTimeout(),
				"promotion must never block traffic beyond the configured lock timeout")
			require.Less(t, maxStall, uuidLockTimeout()/5,
				"an online build must keep the worst observed stall well under the lock timeout")

			requireUUIDUniqueIndex(t, fixture.primary,
				uuidOwnedTarget{role: uuidRolePrimary, table: "users", model: &User{}})
		})
	}
}

// TestUUIDFinalizationFailuresLeaveNoMarker covers UUID-A28: a duplicate owned UUID, a
// duplicate-index race, and a reference-database outage each leave markers absent, and the
// duplicate UUID case is shown to be retryable after operator remediation. The bounded lock
// timeout is covered by asserting it is installed on the very session the production DDL path
// pins; see the subtest comment for what that does and does not prove.
func TestUUIDFinalizationFailuresLeaveNoMarker(t *testing.T) {
	for _, backend := range uuidMultiDBBackends {
		t.Run(backend, func(t *testing.T) {
			t.Run("duplicate uuid blocks then retry succeeds", func(t *testing.T) {
				fixture := newUUIDMultiDBFixture(t, backend, uuidTopologyUnified)
				withFinalizerEnabled(t, true)

				shared := uuidMultiDBFixtureUUID(1)
				require.NoError(t, fixture.primary.Table("users").Create([]map[string]any{
					{"id": 1, "username": "dup-root", "password": "password-hash", "uuid": shared},
					{"id": 2, "username": "dup-other", "password": "password-hash", "uuid": shared},
				}).Error)

				_, err := runFinalizer(t, fixture.topology)
				require.ErrorContains(t, err, "duplicate owned uuid",
					"a duplicate owned uuid must block promotion")
				requireMarker(t, fixture.primary, externalUUIDPrimaryMigrationKey, false)
				require.False(t, fixture.primary.Migrator().HasIndex(&User{}, uuidUniqueIndexName("users")),
					"a blocked promotion must not leave a unique index behind")
				require.True(t, fixture.primary.Migrator().HasIndex(&User{}, ordinaryUUIDIndexName("users")),
					"the ordinary candidate index must survive a failed promotion")

				// Operator remediation, then a plain retry of the same finalizer.
				require.NoError(t, fixture.primary.Table("users").Where("id = ?", 2).
					Update("uuid", uuidMultiDBFixtureUUID(2)).Error)

				_, err = runFinalizer(t, fixture.topology)
				require.NoError(t, err, "the finalizer must be retryable once the duplicate is remediated")
				requireMarker(t, fixture.primary, externalUUIDPrimaryMigrationKey, true)
				requireUUIDUniqueIndex(t, fixture.primary,
					uuidOwnedTarget{role: uuidRolePrimary, table: "users", model: &User{}})
			})

			t.Run("duplicate index race is idempotent", func(t *testing.T) {
				fixture := newUUIDMultiDBFixture(t, backend, uuidTopologyUnified)
				ctx := context.Background()
				require.NoError(t, fixture.primary.Table("users").Create(map[string]any{
					"id": 1, "username": "race-root", "password": "password-hash",
					"uuid": uuidMultiDBFixtureUUID(1),
				}).Error)

				uniqueName := uuidUniqueIndexName("users")
				require.NoError(t, createUUIDIndex(ctx, fixture.primary, "users", uniqueName, []string{"uuid"}, uuidIndexUnique),
					"pre-create the unique index a racing worker would have created")

				// The losing worker in the race sees the dialect's duplicate-object error, which
				// the migration must classify as a race rather than a hard failure. This is the
				// assertion the SQLite suite cannot make: the classifier matches real MySQL and
				// PostgreSQL error text.
				err := createUUIDIndex(ctx, fixture.primary, "users", uniqueName, []string{"uuid"}, uuidIndexUnique)
				require.Error(t, err, "a second create of the same index must fail")
				require.True(t, isDuplicateObjectError(err),
					"a duplicate index error must classify as a duplicate-object race on %s: %v", backend, err)

				// Promotion resolves the race by rereading metadata and confirming the exact
				// expected index, so it succeeds idempotently and still drops the ordinary index.
				require.NoError(t, promoteUUIDUniqueIndexes(ctx, fixture.topology),
					"promotion must tolerate an index that already exists")
				requireUUIDUniqueIndex(t, fixture.primary,
					uuidOwnedTarget{role: uuidRolePrimary, table: "users", model: &User{}})
				require.False(t, hasIndexNamed(ctx, fixture.primary, "users", ordinaryUUIDIndexName("users")),
					"the redundant ordinary uuid index must not survive promotion")
				require.NoError(t, promoteUUIDUniqueIndexes(ctx, fixture.topology),
					"repeating promotion must stay a no-op")
			})

			t.Run("reference database outage", func(t *testing.T) {
				fixture := newUUIDMultiDBFixture(t, backend, uuidTopologySplit)
				withFinalizerEnabled(t, true)
				fixture.seedLegacyRows(t)

				// Closing only the log handle's pool is a true outage of the reference database
				// even when both DSNs address one physical server, because the two handles own
				// independent pools.
				fixture.closeLogHandle(t)

				_, err := runFinalizer(t, fixture.topology)
				// The coordinator's first log-database read is the topology's data_migrations
				// probe, so a fully closed pool surfaces there rather than mid-reconciliation.
				// What UUID-A28 requires is asserted either way: the run errors and no marker is
				// written, so a later run against a healthy reference database retries every
				// phase from scratch (that retry is UUID-A25's finalizer matrix cell).
				require.Error(t, err, "a reference database outage must fail the coordinator")
				requireMarker(t, fixture.primary, externalUUIDPrimaryMigrationKey, false)
			})

			t.Run("bounded ddl lock timeout is applied", func(t *testing.T) {
				fixture := newUUIDMultiDBFixture(t, backend, uuidTopologyUnified)
				db := fixture.primary

				// Deterministically forcing a real lock-wait abort would need a second session
				// holding a conflicting lock for longer than the configured lock timeout while
				// the DDL waits, which no CI job can afford to do reliably. What is covered
				// instead is the production helper itself: the bounded timeouts really are
				// installed on the pinned DDL session, and — critically — they are restored
				// before the connection returns to the pool, so the migration's aggressive
				// timeouts cannot leak onto ordinary application traffic. What is NOT covered
				// here is the server actually aborting a contended statement at that bound.
				require.NoError(t, db.Connection(func(tx *gorm.DB) error {
					readLock := func() string {
						var value string
						switch dialectName(db) {
						case "postgres":
							// SHOW renders a friendly unit ("5s"), so read pg_settings, whose
							// value is always in the setting's base unit (ms).
							require.NoError(t, tx.Raw(
								"SELECT setting FROM pg_settings WHERE name = 'lock_timeout'").Scan(&value).Error)
						case "mysql":
							require.NoError(t, tx.Raw("SELECT @@SESSION.lock_wait_timeout").Scan(&value).Error)
						}
						return value
					}

					before := readLock()
					var inside string
					require.NoError(t, withUUIDDDLTimeouts(tx, func() error {
						inside = readLock()
						return nil
					}))
					after := readLock()

					switch dialectName(db) {
					case "postgres":
						want := strconv.Itoa(int(uuidLockTimeout() / time.Millisecond))
						require.Equal(t, want, inside,
							"the bounded lock timeout must be installed on the DDL session")
						var statement string
						require.NoError(t, withUUIDDDLTimeouts(tx, func() error {
							return tx.Raw(
								"SELECT setting FROM pg_settings WHERE name = 'statement_timeout'").Scan(&statement).Error
						}))
						require.Equal(t, strconv.Itoa(int(uuidDDLTimeout()/time.Millisecond)), statement,
							"the bounded statement timeout must be installed on the DDL session")
					case "mysql":
						require.Equal(t, strconv.FormatInt(int64(uuidLockTimeout()/time.Second), 10), inside,
							"the bounded lock timeout must be installed on the DDL session")
					default:
						t.Fatalf("unsupported dialect %q", dialectName(db))
					}

					t.Logf("%s ddl session lock timeout: before=%s inside=%s after=%s", backend, before, inside, after)
					require.Equal(t, before, after,
						"the DDL session timeout must be restored before the connection returns to the pool")
					return nil
				}))
			})
		})
	}
}

// TestUUIDIndexMetadataAfterFinalization covers UUID-A29: after a successful finalizer run
// every owned UUID table carries exactly one valid unique index and no redundant ordinary
// index, every denormalized FK UUID column carries a non-unique index, and index validation
// reports zero issues.
func TestUUIDIndexMetadataAfterFinalization(t *testing.T) {
	for _, backend := range uuidMultiDBBackends {
		t.Run(backend, func(t *testing.T) {
			fixture := newUUIDMultiDBFixture(t, backend, uuidTopologyUnified)
			withFinalizerEnabled(t, true)
			fixture.seedLegacyRows(t)

			_, err := runFinalizer(t, fixture.topology)
			require.NoError(t, err, "%s finalization", backend)

			ctx := context.Background()
			for _, target := range uuidOwnedRegistry() {
				db := fixture.topology.handle(target.role)
				requireUUIDUniqueIndex(t, db, target)
				require.False(t, hasIndexNamed(ctx, db, target.table, ordinaryUUIDIndexName(target.table)),
					"the redundant ordinary uuid index on %s must be dropped after promotion", target.table)
			}

			for _, target := range uuidFKRegistry() {
				db := fixture.topology.handle(target.role)
				name := fkUUIDIndexName(target.table, target.uuidColumn)
				indexes, err := db.Migrator().GetIndexes(target.model)
				require.NoError(t, err, "read index metadata for %s", target.table)

				found := false
				for _, index := range indexes {
					if index.Name() != name {
						continue
					}
					found = true
					unique, ok := index.Unique()
					require.True(t, ok, "%s must report index uniqueness on %s", name, backend)
					require.False(t, unique,
						"denormalized fk uuid index %s must stay non-unique: many rows share one owner's uuid", name)
				}
				require.True(t, found, "missing denormalized fk uuid index %s", name)
			}

			issues, err := validateUUIDIndexes(ctx, fixture.topology)
			require.NoError(t, err)
			require.Empty(t, issues, "index validation must report no issues after finalization")
		})
	}
}

// TestMarkerTimestampsAreUTCAndStable covers UUID-A30: a marker records the completion instant
// in UTC on each dialect, and a duplicate marker insertion preserves the original timestamp
// without creating a second row.
func TestMarkerTimestampsAreUTCAndStable(t *testing.T) {
	for _, backend := range uuidMultiDBBackends {
		t.Run(backend, func(t *testing.T) {
			fixture := newUUIDMultiDBFixture(t, backend, uuidTopologyUnified)
			db := fixture.primary
			ctx := context.Background()
			key := externalUUIDPrimaryMigrationKey

			before := time.Now().UTC()
			require.NoError(t, markDataMigrationComplete(ctx, db, key))
			after := time.Now().UTC()

			var first DataMigration
			require.NoError(t, db.First(&first, "migration_key = ?", key).Error)

			// Drivers hand back either a UTC or a Local time.Time depending on the DSN's
			// timezone options and on whether the column carries a zone, so asserting
			// Location() == time.UTC would test the DSN, not the migration. The assertion that
			// actually matters is on the stored INSTANT: markDataMigrationComplete writes
			// time.Now().UTC(), so a correct round trip reproduces that instant, and any
			// timezone mishandling shows up as a whole-offset error rather than a few seconds.
			t.Logf("%s marker completed_at: value=%s location=%s",
				backend, first.CompletedAt, first.CompletedAt.Location())
			stored := first.CompletedAt.UTC()
			require.False(t, stored.Before(before.Truncate(time.Second)),
				"the marker instant must not predate the write")
			require.WithinDuration(t, after, stored, 2*time.Second,
				"the marker must record the completion instant in UTC")

			// Sleep past the coarsest DATETIME precision any supported backend uses, so a
			// rewritten timestamp would be observable rather than hidden by truncation.
			time.Sleep(1100 * time.Millisecond)

			require.NoError(t, markDataMigrationComplete(ctx, db, key),
				"a repeated marker write must be a no-op, not an error")

			var second DataMigration
			require.NoError(t, db.First(&second, "migration_key = ?", key).Error)
			require.True(t, first.CompletedAt.Equal(second.CompletedAt),
				"a duplicate marker insertion must preserve the original timestamp: %s became %s",
				first.CompletedAt, second.CompletedAt)

			var count int64
			require.NoError(t, db.Model(&DataMigration{}).Where("migration_key = ?", key).Count(&count).Error)
			require.EqualValues(t, 1, count, "a duplicate marker insertion must not create a second row")
		})
	}
}

// TestBackendAvailabilityGuard covers UUID-A31: the acceptance job can make live MySQL and
// PostgreSQL mandatory so their subtests cannot silently skip during release qualification.
// The guard's decision function is asserted directly, because requireBackend's mandatory branch
// calls t.Fatal and cannot be exercised from a passing test.
func TestBackendAvailabilityGuard(t *testing.T) {
	require.Equal(t, "PG_DSN", uuidBackendDSNEnv("postgres"))
	require.Equal(t, "MYSQL_DSN", uuidBackendDSNEnv("mysql"))
	require.Equal(t, "PG_LOG_DSN", uuidBackendLogDSNEnv("postgres"))
	require.Equal(t, "MYSQL_LOG_DSN", uuidBackendLogDSNEnv("mysql"))

	t.Setenv(requireDBBackendsEnv, "")
	require.False(t, uuidBackendsAreMandatory(),
		"an unset guard must let a developer run skip a missing backend")

	t.Setenv(requireDBBackendsEnv, "0")
	require.False(t, uuidBackendsAreMandatory(), "only an explicit 1 may arm the guard")

	t.Setenv(requireDBBackendsEnv, "1")
	require.True(t, uuidBackendsAreMandatory(),
		"%s=1 must turn a missing DSN into a failure instead of a skip", requireDBBackendsEnv)
}

// TestInvalidPostgresIndexIsRebuiltNotAccepted covers UUID-A28 and UUID-040: PostgreSQL keeps
// an index left behind by a failed CREATE INDEX CONCURRENTLY in the catalog and still answers
// a name lookup for it, but marks it invalid and never uses it for reads.
//
// Accepting that name is a silent, permanent livelock: the candidate index is never rebuilt,
// every NULL/empty candidate query degrades into a sequential scan, each bounded cycle times
// out with zero rows updated, and the coordinator reports success forever. The index phase
// must therefore prove validity from metadata and rebuild.
func TestInvalidPostgresIndexIsRebuiltNotAccepted(t *testing.T) {
	db := requireBackend(t, "postgres")
	if db == nil {
		t.Skip("PG_DSN not set; skipping PostgreSQL invalid-index recovery test")
	}
	ctx := context.Background()
	t.Cleanup(func() { require.NoError(t, db.Exec("DROP TABLE IF EXISTS uuid_invalid_index_probe").Error) })

	require.NoError(t, db.Exec("DROP TABLE IF EXISTS uuid_invalid_index_probe").Error)
	require.NoError(t, db.Exec(
		"CREATE TABLE uuid_invalid_index_probe (id serial primary key, uuid char(36))").Error)
	// Duplicate values make a concurrent UNIQUE build fail, which is the documented way an
	// invalid index is left behind.
	require.NoError(t, db.Exec(
		"INSERT INTO uuid_invalid_index_probe (uuid) VALUES ('dup'), ('dup')").Error)
	require.Error(t, db.Exec(
		"CREATE UNIQUE INDEX CONCURRENTLY idx_uuid_invalid_index_probe_uuid ON uuid_invalid_index_probe (uuid)").Error,
		"the concurrent unique build must fail on duplicate data")

	const name = "idx_uuid_invalid_index_probe_uuid"
	require.True(t, hasIndexNamed(ctx, db, "uuid_invalid_index_probe", name),
		"postgres still reports the invalid index by name, which is the trap")
	valid, err := isIndexValid(ctx, db, name)
	require.NoError(t, err)
	require.False(t, valid, "the leftover index must be invalid")

	usable, err := hasUsableIndexNamed(ctx, db, "uuid_invalid_index_probe", name)
	require.NoError(t, err)
	require.False(t, usable, "a name match must not count as a usable index")

	// The candidate phase must drop the invalid index and rebuild a usable one.
	require.NoError(t, ensureNonUniqueIndex(ctx, db, "uuid_invalid_index_probe", name, []string{"uuid"}))

	valid, err = isIndexValid(ctx, db, name)
	require.NoError(t, err)
	require.True(t, valid, "the index phase must leave a valid, usable index behind")
}
