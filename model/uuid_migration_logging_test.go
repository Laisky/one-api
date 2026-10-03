package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
)

// captureUUIDMigrationLogs runs fn with a file-backed logger installed and returns everything
// the migration emitted. Capturing real output is what makes the secret assertions
// meaningful: it proves what the process would actually write to disk.
//
// The logger is installed BOTH on the package global and in the context handed to fn, because
// migration helpers take their logger from context. That also makes this a check of the
// context-logger contract itself: if a helper reached for a global instead, its events would
// be missing from the captured output and the field assertions below would fail.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//   - fn: function whose log output is captured; receives the logger-carrying context.
//
// Return values:
//   - string: captured log output.
func captureUUIDMigrationLogs(t *testing.T, fn func(ctx context.Context)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration.log")

	captured, err := glog.New(
		glog.WithName("uuid-migration-test"),
		glog.WithLevel(glog.LevelDebug),
		glog.WithEncoding(glog.EncodingJSON),
		glog.WithOutputPaths([]string{path}),
		glog.WithErrorOutputPaths([]string{path}),
	)
	require.NoError(t, err)

	original := logger.Logger
	logger.Logger = captured
	t.Cleanup(func() { logger.Logger = original })

	fn(gmw.SetLogger(context.Background(), captured))
	require.NoError(t, captured.Sync())

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

// TestUUIDMigrationProgressLogsExposeNoSecrets covers UUID-A38 and UUID-022: progress logs
// report aggregate counts and durations without exposing row content, DSNs, credentials,
// token keys, or UUID row values.
func TestUUIDMigrationProgressLogsExposeNoSecrets(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)

	const (
		secretPassword = "super-secret-password-hash"
		secretTokenKey = "sk-super-secret-token-key"
		secretContent  = "private-log-body-content"
	)
	require.NoError(t, db.Exec("INSERT INTO users (id, username, password) VALUES (1, 'root', ?)", secretPassword).Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, type, name, models, config) VALUES (1, 1, 'c', 'gpt-4o', '{}')").Error)
	require.NoError(t, db.Exec("INSERT INTO tokens (id, user_id, `key`, name) VALUES (1, 1, ?, 'default')", secretTokenKey).Error)
	require.NoError(t, db.Exec("INSERT INTO logs (id, user_id, channel_id, type, token_name, content) VALUES (1, 1, 1, 1, 'default', ?)", secretContent).Error)

	output := captureUUIDMigrationLogs(t, func(ctx context.Context) {
		_, err := runUUIDMigrationCoordinator(ctx, topology, uuidMigrationModeCatchUp)
		require.NoError(t, err)
	})
	require.NotEmpty(t, output, "reconciliation must emit structured progress events")

	var user User
	require.NoError(t, db.First(&user, "id = ?", 1).Error)
	requireHyphenatedUUID(t, user.UUID)
	var token Token
	require.NoError(t, db.First(&token, "id = ?", 1).Error)

	for _, forbidden := range []string{
		secretPassword,
		secretTokenKey,
		secretContent,
		user.UUID,
		token.UUID,
	} {
		require.NotContains(t, output, forbidden,
			"migration logs must not expose credentials, token keys, UUID row values, or row content")
	}

	// The aggregate fields the proposal requires must all be present.
	for _, field := range []string{
		"topology", "mode", "phase", "table", "column",
		"examined_rows", "updated_rows", "unresolved_rows",
	} {
		require.Contains(t, output, field, "structured progress logs must report %s", field)
	}
}

// TestUUIDFinalizerLogsReportMarkerStateWithoutSecrets covers the completion-key half of
// UUID-022: marker state and duration are reported without leaking DSNs or row values.
func TestUUIDFinalizerLogsReportMarkerStateWithoutSecrets(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, true)
	require.NoError(t, db.Exec("INSERT INTO users (id, username, password) VALUES (1, 'root', 'password-hash')").Error)

	output := captureUUIDMigrationLogs(t, func(ctx context.Context) {
		_, err := runUUIDMigrationCoordinator(ctx, topology, uuidMigrationModeFinalizer)
		require.NoError(t, err)
	})

	require.Contains(t, output, "external uuid completion markers written")
	require.Contains(t, output, "marker_count")
	require.Contains(t, output, "duration")

	var user User
	require.NoError(t, db.First(&user, "id = ?", 1).Error)
	require.NotContains(t, output, user.UUID, "marker logs must not contain UUID row values")
	require.NotContains(t, output, "password-hash", "marker logs must not contain credentials")
}
