package model

// Real pinned old-binary compatibility qualification (AUTO-013, AUTO-T05/T06/T07/T08/T21).
//
// The proposal is explicit that "Both real artifacts must run in qualification. Raw SQL
// emulation is not an acceptable substitute." So this suite executes an actual pre-migration
// one-api binary, built from a pinned ref, against a compact-completed database, and then
// inspects what that binary did to the schema and the data.
//
// What makes this meaningful rather than ceremonial: the old binary has no knowledge of compact
// columns at all. It runs its own AutoMigrate over a schema full of objects it has never heard
// of, and it writes rows through its own v3 writer contract. If AutoMigrate dropped an unknown
// column, or if a write bypassed the trigger, this is the only place it would show.
//
// The binary path comes from COMPACT_UUID_TEST_OLD_BINARY, which the qualification workflow
// sets after building the pinned ref and recording its checksum. Absent that (or a DSN), the
// test skips locally; CI's no-skip guard fails the run instead.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// compactOldBinaryEnv names the environment variable carrying the pinned artifact's path.
const compactOldBinaryEnv = "COMPACT_UUID_TEST_OLD_BINARY"

// compactOldBinaryPortEnv names the port the old binary should listen on during the corpus.
const compactOldBinaryPortEnv = "COMPACT_UUID_TEST_OLD_BINARY_PORT"

// runPinnedOldBinary starts the pinned artifact against a DSN and waits for it to migrate.
//
// The binary is a server, so it is started and then stopped rather than run to completion. Its
// startup path is what this suite is actually exercising: it opens the database, runs the full
// legacy AutoMigrate, and creates the root account when no user exists — the last of which is a
// genuine write through the old writer contract.
// Parameters:
//   - t: test handle used for assertions.
//   - binary: absolute path to the pinned artifact.
//   - dsn: SQL_DSN value pointing at the database under test.
//   - settleFor: how long to let the binary run before stopping it.
//
// Return values:
//   - string: the binary's combined output, for diagnosis on failure.
func runPinnedOldBinary(t *testing.T, binary string, dsn string, settleFor time.Duration) string {
	t.Helper()

	port := strings.TrimSpace(os.Getenv(compactOldBinaryPortEnv))
	if port == "" {
		port = "13999"
	}
	logDir := t.TempDir()

	command := exec.Command(binary)
	command.Env = append(os.Environ(),
		"SQL_DSN="+dsn,
		"SESSION_SECRET=compact-uuid-oldbinary-qualification",
		"PORT="+port,
		"LOG_DIR="+filepath.Join(logDir, "logs"),
	)
	output := &strings.Builder{}
	command.Stdout = output
	command.Stderr = output

	require.NoError(t, command.Start(), "the pinned old binary must start")
	// The binary must be stopped even if an assertion fails, or it keeps the port and a
	// connection pool for the rest of the run.
	waited := false
	t.Cleanup(func() {
		if command.Process != nil && !waited {
			_ = command.Process.Kill()
			_ = command.Wait()
			waited = true
		}
	})

	time.Sleep(settleFor)
	// Signal 0 is the portable liveness probe: it performs the permission and existence checks
	// without delivering anything. Passing a nil signal is not a probe and always errors.
	require.NotNil(t, command.Process)
	livenessErr := command.Process.Signal(syscall.Signal(0))
	if livenessErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		waited = true
		require.NoError(t, livenessErr,
			"the pinned old binary must still be running after startup and AutoMigrate; output:\n%s",
			output.String())
	}

	_ = command.Process.Kill()
	// Cmd.Wait, unlike Process.Wait, also joins os/exec's stdout and stderr copy goroutines.
	// Reading the builder before those goroutines exit races with their final writes.
	_ = command.Wait()
	waited = true
	return output.String()
}

// compactCatalogFingerprint captures every object the compatibility contract protects.
//
// It deliberately covers both families: the compact objects that must SURVIVE the old binary
// untouched, and the legacy columns and indexes that must remain unchanged in name, type,
// nullability, and definition.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live PostgreSQL handle.
//
// Return values:
//   - string: a deterministic, newline-joined catalog rendering.
func compactCatalogFingerprint(t *testing.T, db *gorm.DB) string {
	t.Helper()
	lines := []string{}

	for _, query := range []string{
		// Compact shadows: name, type, nullability.
		`SELECT 'COL|'||table_name||'|'||column_name||'|'||data_type||'|'||is_nullable
		   FROM information_schema.columns WHERE column_name LIKE '%_compact' ORDER BY 1`,
		// Legacy UUID columns: the contract forbids any retype or nullability change.
		`SELECT 'LEGACY|'||table_name||'|'||column_name||'|'||data_type||'|'||
		        coalesce(character_maximum_length::text,'-')||'|'||is_nullable
		   FROM information_schema.columns
		  WHERE column_name IN ('uuid','user_uuid','token_uuid','channel_uuid','inviter_uuid','log_uuid','server_uuid')
		  ORDER BY 1`,
		// Sync triggers: timing/event bits, enabled state, security property, and body hash.
		`SELECT 'TRG|'||t.tgname||'|'||c.relname||'|'||t.tgtype::text||'|'||t.tgenabled::text||'|'||
		        p.prosecdef::text||'|'||md5(p.prosrc)
		   FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_proc p ON p.oid=t.tgfoid
		  WHERE t.tgname LIKE 'cuuid%' ORDER BY 1`,
		// Every index definition, compact and legacy alike.
		`SELECT 'IDX|'||indexname||'|'||md5(indexdef) FROM pg_indexes WHERE schemaname='public' ORDER BY 1`,
		// Markers, including their timestamps: completion history must be stable.
		`SELECT 'MARK|'||migration_key||'|'||completed_at FROM data_migrations ORDER BY 1`,
	} {
		rows := []string{}
		require.NoError(t, db.Raw(query).Scan(&rows).Error)
		lines = append(lines, rows...)
	}
	return strings.Join(lines, "\n")
}

func TestCompactUUIDOldBinary(t *testing.T) {
	// AUTO-T06/T21: a real pinned pre-migration binary starts against a compact-completed
	// schema, runs its own AutoMigrate, and leaves every compact and legacy object unchanged.
	dialect := compactLiveDialects()[1] // PostgreSQL: native uuid is the strictest shadow type.
	dsn := strings.TrimSpace(os.Getenv(dialect.primaryEnv))
	if dsn == "" {
		compactLiveSkipf(t, "%s is not configured", dialect.primaryEnv)
	}
	// The artifact builds itself from the pinned commit; the env is only an override.
	binary := resolvePinnedCompactBinary(t, compactOldBinaryPinnedRef, compactOldBinaryEnv)

	db, topology, ok := newLiveCompactTopology(t, dialect, false)
	require.True(t, ok)

	// Reach validated completion first: the old binary must face a fully migrated schema.
	seedCompactUser(t, db, 1, compactUUIDTextFor(1))
	require.Equal(t, compactStateReady, driveCompactToReady(t, newCompactCoordinator(topology)).state)

	before := compactCatalogFingerprint(t, db)
	require.Contains(t, before, "COL|users|uuid_compact", "the fixture must really be expanded")
	require.Contains(t, before, "TRG|cuuid_v1_users_sync", "the fixture must really be triggered")
	require.Contains(t, before, "MARK|"+compactPrimaryMigrationKey, "the fixture must really be complete")

	output := runPinnedOldBinary(t, binary, oldBinaryDSN(dsn), 22*time.Second)
	require.Contains(t, output, "database schema migrated",
		"the old binary's own AutoMigrate must have run; output:\n%s", output)

	after := compactCatalogFingerprint(t, db)
	require.Equal(t, before, after,
		"the old binary's AutoMigrate must not drop, rename, retype, or rewrite any compact or legacy object")
}

func TestCompactUUIDCompatibilityCorpus(t *testing.T) {
	// AUTO-T07/T08: the old binary writes through its own v3 writer contract, with no knowledge
	// of compact columns, and the database derives the shadow atomically. A new reader then
	// resolves that row correctly through the verified compact path.
	dialect := compactLiveDialects()[1]
	dsn := strings.TrimSpace(os.Getenv(dialect.primaryEnv))
	if dsn == "" {
		compactLiveSkipf(t, "%s is not configured", dialect.primaryEnv)
	}
	binary := resolvePinnedCompactBinary(t, compactOldBinaryPinnedRef, compactOldBinaryEnv)

	db, topology, ok := newLiveCompactTopology(t, dialect, false)
	require.True(t, ok)
	ctx := compactTestContext(t)

	seedCompactUser(t, db, 1, compactUUIDTextFor(1))
	require.Equal(t, compactStateReady, driveCompactToReady(t, newCompactCoordinator(topology)).state)

	// Emptying users makes the old binary's startup create the root account, which is a real
	// insert through the old writer contract rather than a statement this test composed.
	require.NoError(t, db.Exec("DELETE FROM users").Error)

	runPinnedOldBinary(t, binary, oldBinaryDSN(dsn), 22*time.Second)

	rows := []struct {
		ID      int64   `gorm:"column:id"`
		UUID    string  `gorm:"column:uuid"`
		Shadow  *string `gorm:"column:shadow"`
		Agrees  bool    `gorm:"column:agrees"`
		Present bool    `gorm:"column:present"`
	}{}
	require.NoError(t, db.Raw(
		`SELECT id, uuid, uuid_compact::text AS shadow,
		        (uuid_compact::text = lower(trim(uuid))) AS agrees,
		        (uuid_compact IS NOT NULL) AS present
		   FROM users ORDER BY id`).Scan(&rows).Error)

	require.NotEmpty(t, rows, "the old binary must have written at least one user row")
	for _, row := range rows {
		require.True(t, row.Present,
			"an old-binary write must derive its compact shadow atomically, through the trigger")
		require.True(t, row.Agrees,
			"the derived shadow must equal the authoritative text the old binary wrote")
	}

	// AUTO-T08: a new reader resolves the old binary's row through the verified compact path.
	runCompactHealthAudit(ctx, topology)
	target, err := compactLookupTarget("users")
	require.NoError(t, err)
	id, err := resolveIDByUUID(ctx, db, target, strings.TrimSpace(rows[0].UUID))
	require.NoError(t, err, "a new reader must resolve a row the old binary wrote")
	require.Equal(t, rows[0].ID, id)
}

// oldBinaryDSN converts a GORM PostgreSQL DSN into the URL form the binary's SQL_DSN expects.
//
// The live suite configures a key/value DSN because that is what gorm.io/driver/postgres takes,
// while one-api's own SQL_DSN parsing expects a postgres:// URL. Translating here keeps the
// workflow to a single DSN variable per engine rather than two that could drift apart.
// Parameters:
//   - dsn: key/value PostgreSQL DSN.
//
// Return values:
//   - string: postgres:// URL form.
func oldBinaryDSN(dsn string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return dsn
	}
	fields := map[string]string{}
	for _, part := range strings.Fields(dsn) {
		key, value, found := strings.Cut(part, "=")
		if found {
			fields[key] = value
		}
	}
	return "postgres://" + fields["user"] + ":" + fields["password"] +
		"@" + fields["host"] + ":" + fields["port"] + "/" + fields["dbname"] +
		"?sslmode=" + fields["sslmode"]
}
