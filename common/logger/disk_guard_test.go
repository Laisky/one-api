package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/stretchr/testify/require"
)

// writeLogFile creates a log file of a given size and modification time.
//
// Parameters:
//   - t: the test, used to fail fast.
//   - dir: the directory to write into.
//   - name: the file name.
//   - size: how many bytes to write.
//   - age: how far in the past to stamp the modification time.
//
// Return values:
//   - string: the full path of the created file.
func writeLogFile(t *testing.T, dir, name string, size int, age time.Duration) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, make([]byte, size), 0o600))
	stamp := time.Now().Add(-age)
	require.NoError(t, os.Chtimes(path, stamp, stamp))
	if isLogFileName(name) {
		setActiveLogFile(path)
	}
	return path
}

// TestListLogFilesOrdersOldestFirst verifies the sweeper sees candidates in
// deletion order and ignores non-log entries.
func TestListLogFilesOrdersOldestFirst(t *testing.T) {
	dir := t.TempDir()
	oldest := writeLogFile(t, dir, "oneapi-20260101.log", 10, 72*time.Hour)
	middle := writeLogFile(t, dir, "oneapi-20260102.log", 20, 48*time.Hour)
	newest := writeLogFile(t, dir, "oneapi-20260103.log", 30, time.Hour)
	writeLogFile(t, dir, "notes.txt", 999, time.Hour)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nested"), 0o755))

	files, total, err := listLogFiles(Logger, dir)
	require.NoError(t, err)
	require.Len(t, files, 3, "only log files count")
	require.Equal(t, int64(60), total, "the non-log file must not count toward the budget")
	require.Equal(t, oldest, files[0].path)
	require.Equal(t, middle, files[1].path)
	require.Equal(t, newest, files[2].path)
}

// TestActiveLogFileTracksTheOpenWriterRatherThanDirectoryMetadata verifies a
// size-rotated sibling with the same timestamp cannot be mistaken for the
// active file. Filesystems commonly have coarse modification timestamps, and
// the sequenced sibling sorts before the unsequenced active name.
func TestActiveLogFileTracksTheOpenWriterRatherThanDirectoryMetadata(t *testing.T) {
	dir := t.TempDir()
	day := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)
	writer := newSizeCappedWriter(t, dir, 8, day)

	_, err := writer.Write([]byte("first log entry\n"))
	require.NoError(t, err)
	_, err = writer.Write([]byte("second log entry\n"))
	require.NoError(t, err)

	active := filepath.Join(dir, "oneapi-20260905.log")
	rotated := filepath.Join(dir, "oneapi-20260905-0001.log")
	require.FileExists(t, rotated)
	stamp := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(active, stamp, stamp))
	require.NoError(t, os.Chtimes(rotated, stamp, stamp))
	require.Equal(t, active, activeLogFile(dir),
		"the writer's open path, not newest metadata, owns active-file protection")

	deleted, err := enforceSizeCeiling(Logger, dir, 1)
	require.NoError(t, err)
	require.Equal(t, 1, deleted)
	require.FileExists(t, active, "a retention sweep must never unlink the writer's live file")
}

// TestListLogFilesMissingDirectory verifies a missing directory is not an error.
func TestListLogFilesMissingDirectory(t *testing.T) {
	files, total, err := listLogFiles(Logger, filepath.Join(t.TempDir(), "does-not-exist"))
	require.NoError(t, err)
	require.Empty(t, files)
	require.Zero(t, total)
}

// TestIsLogFileName verifies which entries the sweeper considers log files.
func TestIsLogFileName(t *testing.T) {
	require.True(t, isLogFileName("oneapi.log"))
	require.True(t, isLogFileName("oneapi-20260101.log"))
	require.True(t, isLogFileName("oneapi.log.1"))
	require.True(t, isLogFileName("ONEAPI.LOG"))
	require.False(t, isLogFileName("notes.txt"))
	require.False(t, isLogFileName("logrotate.conf"))

	// Deletion is scoped to this process's files: a shared log directory may
	// hold another program's logs and they must never be touched.
	require.False(t, isLogFileName("nginx-access.log"))
	require.False(t, isLogFileName("supervisord.log"))
	require.False(t, isLogFileName("syslog.log.1"))
}

// TestEnforceSizeCeilingDeletesOldestFirst verifies the directory is brought
// back under budget by removing the oldest rotated files, and that the newest
// file is never removed because the logger still holds it open.
func TestEnforceSizeCeilingDeletesOldestFirst(t *testing.T) {
	dir := t.TempDir()
	oldest := writeLogFile(t, dir, "oneapi-20260101.log", 100, 72*time.Hour)
	middle := writeLogFile(t, dir, "oneapi-20260102.log", 100, 48*time.Hour)
	newest := writeLogFile(t, dir, "oneapi-20260103.log", 100, time.Hour)

	deleted, err := enforceSizeCeiling(Logger, dir, 150)
	require.NoError(t, err)
	require.Equal(t, 2, deleted)

	require.NoFileExists(t, oldest)
	require.NoFileExists(t, middle)
	require.FileExists(t, newest, "the active log file must survive")
}

// TestEnforceSizeCeilingUnderBudget verifies a directory within budget is left
// alone.
func TestEnforceSizeCeilingUnderBudget(t *testing.T) {
	dir := t.TempDir()
	a := writeLogFile(t, dir, "oneapi-20260101.log", 10, 48*time.Hour)
	b := writeLogFile(t, dir, "oneapi-20260102.log", 10, time.Hour)

	deleted, err := enforceSizeCeiling(Logger, dir, 1000)
	require.NoError(t, err)
	require.Zero(t, deleted)
	require.FileExists(t, a)
	require.FileExists(t, b)
}

// TestEnforceSizeCeilingDisabled verifies a zero budget disables the ceiling.
func TestEnforceSizeCeilingDisabled(t *testing.T) {
	dir := t.TempDir()
	a := writeLogFile(t, dir, "oneapi-20260101.log", 100, 48*time.Hour)

	deleted, err := enforceSizeCeiling(Logger, dir, 0)
	require.NoError(t, err)
	require.Zero(t, deleted)
	require.FileExists(t, a)
}

// TestEnforceFreeDiskFloorDisabled verifies a zero floor disables the guard.
func TestEnforceFreeDiskFloorDisabled(t *testing.T) {
	dir := t.TempDir()
	a := writeLogFile(t, dir, "oneapi-20260101.log", 100, 48*time.Hour)

	deleted, err := enforceFreeDiskFloor(Logger, dir, 0)
	require.NoError(t, err)
	require.Zero(t, deleted)
	require.FileExists(t, a)
}

// TestEnforceFreeDiskFloorPurgesAndEscalates verifies the last-resort behavior:
// with a floor no filesystem can satisfy, every rotated file is removed, the
// newest is kept, and the process log level is raised so logging stops
// consuming the little disk that is left.
func TestEnforceFreeDiskFloorPurgesAndEscalates(t *testing.T) {
	if !diskFreeSupported() {
		t.Skip("free disk inspection is not supported on this platform")
	}

	dir := t.TempDir()
	oldest := writeLogFile(t, dir, "oneapi-20260101.log", 100, 72*time.Hour)
	middle := writeLogFile(t, dir, "oneapi-20260102.log", 100, 48*time.Hour)
	newest := writeLogFile(t, dir, "oneapi-20260103.log", 100, time.Hour)

	restoreLevel := Logger.Level()
	levelEscalated.Store(false)
	t.Cleanup(func() {
		levelEscalated.Store(false)
		_ = Logger.ChangeLevel(restoreLevel)
	})

	// An exabyte floor cannot be met, so the guard must exhaust its options.
	const impossibleFloor = int64(1) << 60
	deleted, err := enforceFreeDiskFloor(Logger, dir, impossibleFloor)
	require.NoError(t, err)
	require.Equal(t, 2, deleted)

	require.NoFileExists(t, oldest)
	require.NoFileExists(t, middle)
	require.FileExists(t, newest)

	require.True(t, levelEscalated.Load(), "the guard must escalate when purging is not enough")
	require.Equal(t, glog.LevelWarn, Logger.Level())
}

// TestEnforceFreeDiskFloorRestoresLevel verifies the escalation is reversed once
// free space is back above the floor.
func TestEnforceFreeDiskFloorRestoresLevel(t *testing.T) {
	if !diskFreeSupported() {
		t.Skip("free disk inspection is not supported on this platform")
	}

	dir := t.TempDir()
	restoreLevel := Logger.Level()
	t.Cleanup(func() {
		levelEscalated.Store(false)
		_ = Logger.ChangeLevel(restoreLevel)
	})

	levelEscalated.Store(true)
	require.NoError(t, Logger.ChangeLevel(glog.LevelWarn))

	// A one-byte floor any filesystem satisfies.
	deleted, err := enforceFreeDiskFloor(Logger, dir, 1)
	require.NoError(t, err)
	require.Zero(t, deleted)
	require.False(t, levelEscalated.Load())
	require.Equal(t, defaultLevel(), Logger.Level())
}

// TestFreeDiskBytesReportsSpace verifies the platform probe returns a plausible
// value for an existing directory.
func TestFreeDiskBytesReportsSpace(t *testing.T) {
	if !diskFreeSupported() {
		t.Skip("free disk inspection is not supported on this platform")
	}
	free, err := freeDiskBytes(t.TempDir())
	require.NoError(t, err)
	require.Positive(t, free)
}

// TestEnforceSizeCeilingIgnoresForeignLogs verifies a shared log directory is
// safe: files this process did not create are never deleted, however old or
// large they are.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestEnforceSizeCeilingIgnoresForeignLogs(t *testing.T) {
	dir := t.TempDir()
	foreignOld := writeLogFile(t, dir, "nginx-access.log", 5000, 96*time.Hour)
	ownOld := writeLogFile(t, dir, "oneapi-20260101.log", 100, 72*time.Hour)
	ownActive := writeLogFile(t, dir, "oneapi-20260103.log", 100, time.Hour)

	deleted, err := enforceSizeCeiling(Logger, dir, 150)
	require.NoError(t, err)
	require.Equal(t, 1, deleted)

	require.FileExists(t, foreignOld, "another program's log must never be deleted")
	require.NoFileExists(t, ownOld)
	require.FileExists(t, ownActive, "the file the logger holds open must survive")
}

// TestEnforceFreeDiskFloorKeepsActiveFile verifies the active log file is
// identified by name rather than by modification time, so a restored or foreign
// file cannot trick the guard into unlinking the file being written.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestEnforceFreeDiskFloorKeepsActiveFile(t *testing.T) {
	if !diskFreeSupported() {
		t.Skip("free disk inspection is not supported on this platform")
	}

	dir := t.TempDir()
	older := writeLogFile(t, dir, "oneapi-20260101.log", 100, 72*time.Hour)
	active := writeLogFile(t, dir, "oneapi-20260103.log", 100, time.Hour)

	restoreLevel := Logger.Level()
	levelEscalated.Store(false)
	t.Cleanup(func() {
		levelEscalated.Store(false)
		_ = Logger.ChangeLevel(restoreLevel)
	})

	const impossibleFloor = int64(1) << 60
	_, err := enforceFreeDiskFloor(Logger, dir, impossibleFloor)
	require.NoError(t, err)

	require.NoFileExists(t, older)
	require.FileExists(t, active, "the active log file must survive even under maximum disk pressure")
}
