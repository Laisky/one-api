package model

// AUTO-T17's mixed-dialect leg: a split topology whose primary and log databases run DIFFERENT
// engines must enter compact `blocked_validation` while legacy readiness and legacy traffic are
// completely unaffected (proposal sections 4 and 8.2: "Topology | ... mixed-dialect split is
// rejected in v1" and "Mixed-dialect split and SQLite split enter blocked_validation for
// compact work; they do not fail otherwise-supported legacy readiness").
//
// This needs BOTH live engines at once — a MySQL primary with a PostgreSQL log database — which
// is why it lives beside the live matrix rather than the SQLite suite. Both DSNs gate it; CI's
// no-skip guard fails the run when either is absent.

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCompactUUIDUnsupportedTopologyMixedDialect(t *testing.T) {
	mysqlDialect := compactLiveDialects()[0]
	postgresDialect := compactLiveDialects()[1]
	mysqlDSN := strings.TrimSpace(os.Getenv(mysqlDialect.primaryEnv))
	postgresDSN := strings.TrimSpace(os.Getenv(postgresDialect.primaryEnv))
	if mysqlDSN == "" || postgresDSN == "" {
		t.Skipf("%s and %s are both required; CI's no-skip guard enforces this suite",
			mysqlDialect.primaryEnv, postgresDialect.primaryEnv)
	}
	withCompactTestSettings(t)
	ctx := compactTestContext(t)

	// A MySQL primary owning the non-log targets, and a PostgreSQL LOG_DB. Each handle gets
	// its own engine's legacy schema, exactly as a (mis)configured deployment would have.
	primary := openLiveCompactDB(t, mysqlDialect, mysqlDSN)
	logHandle := openLiveCompactDB(t, postgresDialect, postgresDSN)
	withTestDBGlobals(t, primary, logHandle)
	require.NoError(t, migrateDB())
	require.NoError(t, logHandle.AutoMigrate(&Log{}, &DataMigration{}))

	topology, err := newSplitTopology(primary, logHandle)
	require.NoError(t, err)
	requireV3Markers(t, topology)

	supported, reason, err := validateCompactTopology(topology)
	require.NoError(t, err)
	require.False(t, supported, "a mixed-dialect split must be rejected for compact work in v1")
	require.Contains(t, reason, "mixed-dialect split topology")

	coordinator := newCompactCoordinator(topology)
	result := runCompactCycleForTest(t, coordinator)
	require.Equal(t, compactStateBlockedValidation, result.state)

	t.Run("nothing is expanded and no marker is written on either engine", func(t *testing.T) {
		expanded, err := compactTableExpanded(ctx, primary, compactTablesForRole(uuidRolePrimary)[0])
		require.NoError(t, err)
		require.False(t, expanded, "a blocked topology must not expand the primary")

		for _, table := range compactTablesForRole(uuidRoleLog) {
			logExpanded, err := compactTableExpanded(ctx, logHandle, table)
			require.NoError(t, err)
			require.False(t, logExpanded, "a blocked topology must not expand the log database")
		}
		for _, pair := range []struct {
			handle *gorm.DB
			key    string
		}{
			{handle: primary, key: compactPrimaryMigrationKey},
			{handle: logHandle, key: compactLogMigrationKey},
		} {
			complete, err := isDataMigrationComplete(ctx, pair.handle, pair.key)
			require.NoError(t, err)
			require.False(t, complete, "a blocked topology must never write a completion marker")
		}
	})

	t.Run("legacy readiness and traffic continue on both engines", func(t *testing.T) {
		// The rejection's blast radius must be compact-only: ordinary legacy writes keep
		// working on the primary and on the log database alike.
		seedCompactUser(t, primary, 1, compactUUIDTextFor(1))
		var users int64
		require.NoError(t, primary.Raw("SELECT COUNT(*) FROM users").Scan(&users).Error)
		require.Equal(t, int64(1), users, "legacy primary traffic must be unaffected")

		require.NoError(t, logHandle.Exec(
			"INSERT INTO logs (user_id, created_at, type, content) VALUES (1, 1, 1, 'mixed-dialect probe')").Error)
		var logs int64
		require.NoError(t, logHandle.Raw("SELECT COUNT(*) FROM logs").Scan(&logs).Error)
		require.Equal(t, int64(1), logs, "legacy log traffic must be unaffected")
	})

	t.Run("the health gate forces legacy predicates on every process", func(t *testing.T) {
		runCompactHealthAudit(ctx, topology)
		for _, role := range topology.targetRoles() {
			enabled, gateReason := compactReadsEnabled(role)
			require.False(t, enabled, "compact reads must stay disabled on an unsupported topology")
			require.NotEmpty(t, gateReason)
		}
	})
}
