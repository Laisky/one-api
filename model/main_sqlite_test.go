package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common"
)

func TestEnsureSQLitePathCreatesDirectory(t *testing.T) {
	originalSQLitePath := common.SQLitePath
	t.Cleanup(func() {
		common.SQLitePath = originalSQLitePath
	})

	baseDir := t.TempDir()
	dbPath := filepath.Join(baseDir, "nested", "one-api.db")
	common.SQLitePath = dbPath

	resolved, err := ensureSQLitePath()
	require.NoError(t, err)

	absExpected, err := filepath.Abs(dbPath)
	require.NoError(t, err)
	require.Equal(t, filepath.Clean(absExpected), resolved)

	info, err := os.Stat(filepath.Dir(absExpected))
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestEnsureSQLitePathFailsWhenUnwritable(t *testing.T) {
	originalSQLitePath := common.SQLitePath
	t.Cleanup(func() {
		common.SQLitePath = originalSQLitePath
	})

	baseDir := t.TempDir()
	lockedDir := filepath.Join(baseDir, "locked")
	require.NoError(t, os.MkdirAll(lockedDir, 0o555))
	t.Cleanup(func() {
		_ = os.Chmod(lockedDir, 0o755)
	})

	common.SQLitePath = filepath.Join(lockedDir, "db.sqlite")

	_, err := ensureSQLitePath()
	require.Error(t, err)
}
