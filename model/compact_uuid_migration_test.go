package model

// Automatic-completion, trigger-matrix, and topology tests for compact UUID storage
// (AUTO-T01, T15..T18, T20..T23, T27, T29, T30).
//
// These drive the real coordinator against a real SQLite engine. Nothing here emulates a
// trigger, a catalog read, or a derivation in Go and then asserts against its own emulation.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

func TestCompactUUIDSQLiteCapability(t *testing.T) {
	// AUTO-T27. The persistent triggers use core unhex(), which exists only in SQLite 3.41.0+
	// and runs inside whichever engine each supported binary links. If this probe fails, the
	// SQLite trigger contract is not implementable and compact work must block.
	db, _ := newCompactTestTopology(t)
	ctx := compactTestContext(t)

	var version string
	require.NoError(t, db.Raw("SELECT sqlite_version()").Scan(&version).Error)
	require.True(t, sqliteVersionAtLeast(version, 3, 41),
		"embedded sqlite %s is older than the required 3.41.0", version)

	capable, reason, err := compactSQLiteCapable(ctx, db)
	require.NoError(t, err)
	require.True(t, capable, "golden unhex probe failed: %s", reason)
}

// sqliteVersionAtLeast compares a reported SQLite version against a minimum.
// Parameters:
//   - version: reported version string such as "3.53.2".
//   - major: required major version.
//   - minor: required minor version.
//
// Return values:
//   - bool: true when the reported version is at least major.minor.
func sqliteVersionAtLeast(version string, major int, minor int) bool {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}
	reportedMajor, majorErr := atoiSafe(parts[0])
	reportedMinor, minorErr := atoiSafe(parts[1])
	if majorErr != nil || minorErr != nil {
		return false
	}
	if reportedMajor != major {
		return reportedMajor > major
	}
	return reportedMinor >= minor
}

// atoiSafe parses a non-negative decimal integer.
// Parameters:
//   - text: candidate decimal text.
//
// Return values:
//   - int: parsed value.
//   - error: wrapped error when the text is not a non-negative integer.
func atoiSafe(text string) (int, error) {
	value := 0
	for index := 0; index < len(text); index++ {
		if text[index] < '0' || text[index] > '9' {
			return 0, errCompactUUIDInvalid
		}
		value = value*10 + int(text[index]-'0')
	}
	return value, nil
}

func TestCompactUUIDSQLiteUnified(t *testing.T) {
	// AUTO-T01: an ordinary default startup on an empty supported schema must reach validated
	// completion with no command, no mode, and no finalizer action.
	db, topology := newCompactTestTopology(t)
	ctx := compactTestContext(t)

	// A populated valid schema, written the way any writer writes: legacy text only.
	for index := 1; index <= 5; index++ {
		seedCompactUser(t, db, index, compactUUIDTextFor(index))
	}

	coordinator := newCompactCoordinator(topology)
	result := driveCompactToReady(t, coordinator)
	require.Equal(t, compactStateReady, result.state)
	require.True(t, result.completed)

	// The applicable marker appears automatically.
	complete, err := isDataMigrationComplete(ctx, db, compactPrimaryMigrationKey)
	require.NoError(t, err)
	require.True(t, complete, "the primary compact marker must appear without an operator action")

	// Every shadow, trigger, and index exists and verifies.
	verified, reason, err := validateCompactObjects(ctx, topology)
	require.NoError(t, err)
	require.True(t, verified, "objects did not verify after completion: %s", reason)

	// Unified mode owns all 27 targets across 12 tables, INCLUDING the four logs targets:
	// its primary handle serves the log role too. The exact counts are asserted because an
	// earlier revision drove table work from markerRoles() — which is [primary] in unified —
	// and so silently skipped every logs target while still marking the migration complete.
	require.Len(t, compactTargetsForTopology(topology), 27, "unified mode owns all 27 targets")
	require.Len(t, compactTablesForTopology(topology), 12, "unified mode owns all 12 tables")
	logTargets := 0
	for _, target := range compactTargetsForTopology(topology) {
		if target.table == "logs" {
			logTargets++
		}
	}
	require.Equal(t, 4, logTargets, "unified mode must expand the logs table on the primary handle")

	// Each target must carry a valid compact index.
	for _, target := range compactTargetsForTopology(topology) {
		valid, err := verifyCompactIndex(ctx, topology.handle(target.role), target)
		require.NoError(t, err)
		require.True(t, valid, "compact index missing for %s", target.id())
	}

	// Historical rows were filled from authoritative text.
	for index := 1; index <= 5; index++ {
		shadow := readCompactShadowHex(t, db, "users", "uuid_compact", index)
		expected, err := parseCompactUUID(compactUUIDTextFor(index))
		require.NoError(t, err)
		require.Equal(t, strings.ToUpper(hexOf(expected)), shadow)
	}

	// The equality fingerprints match, and the raw-source stream proves the states were seen.
	evidence, matched, err := verifyCompactFingerprints(ctx, topology)
	require.NoError(t, err)
	require.True(t, matched, "global equality fingerprints must match at completion")
	require.NotEmpty(t, evidence[uuidRolePrimary].RawSourceDigest)
}

func TestCompactUUIDAutomaticMigration(t *testing.T) {
	t.Run("waits for the v3 prerequisite before marking complete", func(t *testing.T) {
		// A v2 marker never satisfies the prerequisite, and compact must not write its
		// marker before v3's exist — but it may still expand, trigger, and backfill.
		withCompactTestSettings(t)
		db, topology := newUnifiedTestTopology(t)
		ctx := compactTestContext(t)
		require.NoError(t, markDataMigrationComplete(ctx, db, externalUUIDPrimaryMigrationKeyV2))
		seedCompactUser(t, db, 1, compactUUIDTextFor(1))

		coordinator := newCompactCoordinator(topology)
		var result compactCycleResult
		for cycle := 0; cycle < 200; cycle++ {
			result = runCompactCycleForTest(t, coordinator)
			if result.state == compactStateWaitingPrerequisite {
				break
			}
			require.NotEqual(t, compactStateReady, result.state,
				"compact must not complete before the v3 markers exist")
		}
		require.Equal(t, compactStateWaitingPrerequisite, result.state)

		complete, err := isDataMigrationComplete(ctx, db, compactPrimaryMigrationKey)
		require.NoError(t, err)
		require.False(t, complete, "a v2 marker must never satisfy the v3 prerequisite")

		// Nothing is expanded either. Section 2.1 permits compact to expand while v3 runs,
		// but that permission cannot be taken while v3 still has legacy index DDL
		// outstanding: the legacy-index baseline captured then would mismatch as soon as v3
		// finalized. See "v3 finalization after compact starts still converges".
		expanded, err := compactTableExpanded(ctx, db, compactTablesForRole(uuidRolePrimary)[0])
		require.NoError(t, err)
		require.False(t, expanded, "compact must take no DDL step before its prerequisite is met")

		// Once the v3 markers land, compact completes on its own with no restart.
		requireV3Markers(t, topology)
		result = driveCompactToReady(t, coordinator)
		require.Equal(t, compactStateReady, result.state)
	})

	t.Run("v3 finalization after compact starts still converges", func(t *testing.T) {
		// AUTO-T20 and the section 2.1 source shape "Populated valid schema without v3
		// markers", whose required behavior is: "V3 completes automatically first; compact
		// waits safely and then completes."
		//
		// This is a regression test for a real deadlock. InitDatabases starts the v3 and
		// compact workers concurrently, so on any deployment where v3 has not already
		// finished, compact's first cycle runs while v3 still has legacy index DDL
		// outstanding. If compact captured its legacy-index baseline then, v3's finalizer
		// would promote the owned UUID indexes, the manifest would mismatch, and — because
		// the manifest is deliberately never rewritten — compact would block on its own
		// prerequisite forever. That was the default path, not an edge case.
		withCompactTestSettings(t)
		db, topology := newUnifiedTestTopology(t)
		ctx := compactTestContext(t)
		seedCompactUser(t, db, 1, compactUUIDTextFor(1))

		// Compact runs first, while v3 is still outstanding. It must take no baseline and do
		// no DDL: waiting is the specified behavior.
		coordinator := newCompactCoordinator(topology)
		result := runCompactCycleForTest(t, coordinator)
		require.Equal(t, compactStateWaitingPrerequisite, result.state)

		_, found, err := readLegacyIndexManifest(ctx, db, uuidRolePrimary)
		require.Error(t, err, "no manifest table may exist before the v3 prerequisite is met")
		require.False(t, found)

		expanded, err := compactTableExpanded(ctx, db, compactTablesForRole(uuidRolePrimary)[0])
		require.NoError(t, err)
		require.False(t, expanded, "compact must not expand while v3 index DDL is outstanding")

		// Now v3 finalizes, exactly as it does automatically after sustained quiescence. It
		// legitimately changes legacy UUID index metadata.
		_, err = runUUIDMigrationCoordinator(ctx, topology, uuidMigrationModeFinalizer)
		require.NoError(t, err)

		// Compact must now baseline against the post-v3 reality and complete on its own.
		result = driveCompactToReady(t, coordinator)
		require.Equal(t, compactStateReady, result.state)

		ok, reason, err := ensureLegacyIndexManifest(ctx, db, uuidRolePrimary)
		require.NoError(t, err)
		require.True(t, ok, "the baseline must match the post-v3 legacy indexes: %s", reason)
	})

	t.Run("emergency pause mutates nothing", func(t *testing.T) {
		// AUTO-T29: pause must not alter schema or markers and must preserve legacy service.
		db, topology := newCompactTestTopology(t)
		ctx := compactTestContext(t)
		original := config.CompactUUIDAutoMigrate
		config.CompactUUIDAutoMigrate = false
		t.Cleanup(func() { config.CompactUUIDAutoMigrate = original })

		coordinator := newCompactCoordinator(topology)
		delay := runCompactWorkerCycle(ctx, coordinator)
		require.Positive(t, delay)

		expanded, err := compactTableExpanded(ctx, db, compactTablesForRole(uuidRolePrimary)[0])
		require.NoError(t, err)
		require.False(t, expanded, "a paused worker must not expand any table")

		complete, err := isDataMigrationComplete(ctx, db, compactPrimaryMigrationKey)
		require.NoError(t, err)
		require.False(t, complete, "a paused worker must not write a marker")

		// Legacy traffic still works, which is the whole point of the pause being safe.
		seedCompactUser(t, db, 1, compactUUIDTextFor(1))
		enabled, _ := compactReadsEnabled(uuidRolePrimary)
		require.False(t, enabled, "a paused deployment must serve legacy predicates")
	})

	t.Run("markers never suppress the audit", func(t *testing.T) {
		// AUTO-A04/AUTO-T13: unlike the v3 worker, a completed compact worker keeps
		// auditing. A marker records history; it never authorizes an unverified read.
		db, topology := newCompactTestTopology(t)
		ctx := compactTestContext(t)
		seedCompactUser(t, db, 1, compactUUIDTextFor(1))

		coordinator := newCompactCoordinator(topology)
		driveCompactToReady(t, coordinator)

		runCompactHealthAudit(ctx, topology)
		enabled, reason := compactReadsEnabled(uuidRolePrimary)
		require.True(t, enabled, "a healthy audit after completion must enable compact reads: %s", reason)

		// Drop a trigger behind the worker's back, exactly as an operator or restore could.
		require.NoError(t, db.Exec("DROP TRIGGER "+compactInsertTriggerName("users")).Error)
		runCompactHealthAudit(ctx, topology)

		enabled, reason = compactReadsEnabled(uuidRolePrimary)
		require.False(t, enabled, "a missing trigger must disable compact reads immediately")
		require.NotEmpty(t, reason)

		// The marker's timestamp is stable: drift degrades health, it never rewrites history.
		complete, err := isDataMigrationComplete(ctx, db, compactPrimaryMigrationKey)
		require.NoError(t, err)
		require.True(t, complete, "drift must never delete or rewrite a completion marker")

		// The worker repairs the object automatically, with no command.
		driveCompactToReady(t, coordinator)
		runCompactHealthAudit(ctx, topology)
		enabled, _ = compactReadsEnabled(uuidRolePrimary)
		require.True(t, enabled, "a fresh full audit must restore ready after automatic repair")
	})
}

func TestCompactUUIDTriggerMatrix(t *testing.T) {
	// AUTO-T22/T23: every parity vector is driven through the REAL trigger, so the Go codec
	// and the dialect's SQL are held to one accept/reject boundary.
	db, _ := newCompactTestTopology(t)
	expandAndTriggerAll(t, db, uuidRolePrimary)

	for index, vector := range compactCodecVectors() {
		t.Run(vector.name, func(t *testing.T) {
			id := 1000 + index
			if vector.input == "" {
				// An empty owned uuid is a distinct legacy state from NULL; both derive NULL.
				seedCompactUser(t, db, id, "")
			} else {
				// A previously accepted legacy write must never start failing because its
				// UUID is invalid: the insert must succeed regardless.
				seedCompactUser(t, db, id, vector.input)
			}

			shadow := readCompactShadowHex(t, db, "users", "uuid_compact", id)
			if !vector.accepted {
				require.Empty(t, shadow, "invalid legacy text must derive NULL, never a value")
			} else {
				require.Equal(t, vector.hex, shadow, "accepted text must derive identical RFC bytes")
			}

			// The authoritative text is never rewritten by the trigger.
			var stored *string
			require.NoError(t, db.Raw("SELECT uuid FROM users WHERE id = ?", id).Scan(&stored).Error)
			require.NotNil(t, stored)
			require.Equal(t, vector.input, *stored, "the trigger must never rewrite authoritative text")
		})
	}
}

func TestCompactUUIDTriggerRecursionTerminates(t *testing.T) {
	// The SQLite sync trigger is an AFTER trigger that updates its own row, so termination is
	// a real risk. The null-safe mismatch WHEN clause must make it terminate with
	// recursive_triggers both ON and OFF.
	db, _ := newCompactTestTopology(t)
	expandAndTriggerAll(t, db, uuidRolePrimary)

	for _, recursive := range []string{"ON", "OFF"} {
		t.Run("recursive_triggers="+recursive, func(t *testing.T) {
			require.NoError(t, db.Exec("PRAGMA recursive_triggers = "+recursive).Error)

			id := 2000
			if recursive == "OFF" {
				id = 2001
			}
			seedCompactUser(t, db, id, compactUUIDTextFor(id))
			expected, err := parseCompactUUID(compactUUIDTextFor(id))
			require.NoError(t, err)
			require.Equal(t, strings.ToUpper(hexOf(expected)),
				readCompactShadowHex(t, db, "users", "uuid_compact", id))

			// An update re-derives, and also terminates.
			next := compactUUIDTextFor(id + 500)
			require.NoError(t, db.Exec("UPDATE users SET uuid = ? WHERE id = ?", next, id).Error)
			expected, err = parseCompactUUID(next)
			require.NoError(t, err)
			require.Equal(t, strings.ToUpper(hexOf(expected)),
				readCompactShadowHex(t, db, "users", "uuid_compact", id))

			// An update that does not touch the uuid must not spin either. The username is
			// per-row because the legacy schema keeps it unique.
			require.NoError(t, db.Exec("UPDATE users SET username = ? WHERE id = ?",
				fmt.Sprintf("renamed-%d", id), id).Error)
		})
	}
}

func TestCompactUUIDTriggerTransactionSemantics(t *testing.T) {
	// AUTO-T07/T22: commit must expose both text and shadow, rollback must expose neither, and
	// the derivation must be atomic with the write rather than a later repair.
	db, _ := newCompactTestTopology(t)
	expandAndTriggerAll(t, db, uuidRolePrimary)

	t.Run("rollback exposes neither change", func(t *testing.T) {
		err := db.Transaction(func(tx *gorm.DB) error {
			require.NoError(t, tx.Exec(
				"INSERT INTO users (id, username, password, uuid) VALUES (3000, 'u', 'x', ?)",
				compactUUIDTextFor(3000)).Error)
			// Inside the writer's own transaction, text and shadow already agree.
			var shadow *string
			require.NoError(t, tx.Raw("SELECT hex(uuid_compact) FROM users WHERE id = 3000").Scan(&shadow).Error)
			require.NotNil(t, shadow, "the shadow must be derived in the same transaction as the write")
			return errCompactUUIDInvalid
		})
		require.Error(t, err)

		var count int64
		require.NoError(t, db.Raw("SELECT COUNT(*) FROM users WHERE id = 3000").Scan(&count).Error)
		require.Zero(t, count, "rollback must expose neither the text nor the shadow")
	})

	t.Run("commit exposes both changes", func(t *testing.T) {
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			return tx.Exec("INSERT INTO users (id, username, password, uuid) VALUES (3001, 'u', 'x', ?)",
				compactUUIDTextFor(3001)).Error
		}))
		expected, err := parseCompactUUID(compactUUIDTextFor(3001))
		require.NoError(t, err)
		require.Equal(t, strings.ToUpper(hexOf(expected)),
			readCompactShadowHex(t, db, "users", "uuid_compact", 3001))
	})
}

func TestCompactUUIDUnsupportedTopologySQLiteSplit(t *testing.T) {
	// AUTO-T17: a SQLite split topology must block compact work while legacy readiness and
	// legacy behavior remain completely unaffected.
	withCompactTestSettings(t)
	primary, logDB, topology := newSplitTestTopology(t)
	ctx := compactTestContext(t)
	requireV3Markers(t, topology)

	supported, reason, err := validateCompactTopology(topology)
	require.NoError(t, err)
	require.False(t, supported)
	require.Contains(t, reason, "sqlite split topology")

	coordinator := newCompactCoordinator(topology)
	result := runCompactCycleForTest(t, coordinator)
	require.Equal(t, compactStateBlockedValidation, result.state)

	// Nothing was expanded and no marker was written on either database.
	expanded, err := compactTableExpanded(ctx, primary, compactTablesForRole(uuidRolePrimary)[0])
	require.NoError(t, err)
	require.False(t, expanded, "an unsupported topology must not expand anything")

	for _, handle := range []*gorm.DB{primary, logDB} {
		for _, key := range []string{compactPrimaryMigrationKey, compactLogMigrationKey} {
			complete, err := isDataMigrationComplete(ctx, handle, key)
			require.NoError(t, err)
			require.False(t, complete, "an unsupported topology must never write a marker")
		}
	}

	// Legacy traffic is untouched.
	seedCompactUser(t, primary, 1, compactUUIDTextFor(1))
	var count int64
	require.NoError(t, primary.Raw("SELECT COUNT(*) FROM users").Scan(&count).Error)
	require.Equal(t, int64(1), count, "legacy reads and writes must be unaffected")
}

func TestCompactUUIDUnsupportedTopology(t *testing.T) {
	t.Run("unified sqlite is supported", func(t *testing.T) {
		_, topology := newCompactTestTopology(t)
		supported, reason, err := validateCompactTopology(topology)
		require.NoError(t, err)
		require.True(t, supported, "unified sqlite must be supported: %s", reason)
	})

	t.Run("nil topology is rejected before any access", func(t *testing.T) {
		_, _, err := validateCompactTopology(nil)
		require.Error(t, err)
	})
}

func TestCompactUUIDForbiddenDDL(t *testing.T) {
	// AUTO-T30/AUTO-A12: destructive cleanup must be impossible while the contract is active,
	// and must fail BEFORE any DDL reaches the database.
	t.Run("destructive legacy statements are rejected", func(t *testing.T) {
		for _, statement := range []string{
			`ALTER TABLE "users" DROP COLUMN "uuid"`,
			`ALTER TABLE users DROP COLUMN uuid`,
			`ALTER TABLE "tokens" RENAME COLUMN "user_uuid" TO "legacy_user_uuid"`,
			`ALTER TABLE "users" MODIFY COLUMN "uuid" VARBINARY(16)`,
			`ALTER TABLE "logs" ALTER COLUMN "uuid" TYPE bytea`,
			`ALTER TABLE "channels" CHANGE COLUMN "uuid" "uuid2" CHAR(36)`,
			`DROP INDEX "uuid" ON "users"`,

			// These two mention the compact shadow as well as the legacy column, and an
			// earlier revision of the guard skipped exactly that case as "operating on our
			// own additive object". Both destroy the authoritative column, so they are
			// permanent positive controls rather than examples.
			`ALTER TABLE "users" RENAME COLUMN "uuid" TO "uuid_compact"`,
			`ALTER TABLE "users" DROP COLUMN "uuid_compact", DROP COLUMN "uuid"`,
		} {
			err := compactGuardedDDL(statement)
			require.Error(t, err, "must reject: %s", statement)
			require.ErrorIs(t, err, errForbiddenLegacyDDL)
		}
	})

	t.Run("additive compact statements are permitted", func(t *testing.T) {
		// The guard must not be so broad that it blocks the migration's own additive work:
		// uuid is a prefix of uuid_compact, so a substring test would break everything.
		db, _ := newCompactTestTopology(t)
		for _, table := range compactTablesForRole(uuidRolePrimary) {
			for _, target := range table.targets {
				statement, err := compactAddColumnSQL(db, target)
				require.NoError(t, err)
				require.NoError(t, compactGuardedDDL(statement),
					"additive expansion must be permitted: %s", statement)
			}
			statements, err := compactTriggerDDL(db, table)
			require.NoError(t, err)
			for _, statement := range statements {
				require.NoError(t, compactGuardedDDL(statement),
					"trigger installation must be permitted: %s", statement)
			}
		}
		// Operating on the compact shadow itself is this migration's own object, so allowed.
		require.NoError(t, compactGuardedDDL(`ALTER TABLE "users" DROP COLUMN "uuid_compact"`))
	})

	t.Run("no production path can destroy legacy storage", func(t *testing.T) {
		// The runtime guard sits on the single path compact DDL takes to the database.
		db, _ := newCompactTestTopology(t)
		ctx := context.Background()
		err := execCompactDDL(ctx, db, `ALTER TABLE "users" DROP COLUMN "uuid"`)
		require.Error(t, err)
		require.ErrorIs(t, err, errForbiddenLegacyDDL)

		// The column is still there: the guard fired before DDL, not after.
		require.True(t, db.Migrator().HasColumn(&User{}, "uuid"))
	})
}
