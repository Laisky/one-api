package logger

// Tests for the writer-aware active-file size cap (W0.2).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// newSizeCappedWriter builds a daily rotation writer with a byte ceiling and a
// frozen clock, so a test can trigger size rotation without waiting for a
// window boundary.
//
// Parameters:
//   - t: the test handle; the writer is closed on cleanup.
//   - dir: the log directory.
//   - maxActiveBytes: the active-file ceiling in bytes.
//   - at: the frozen wall clock.
//
// Return values:
//   - *rotationWriter: the writer under test.
func newSizeCappedWriter(t *testing.T, dir string, maxActiveBytes int64, at time.Time) *rotationWriter {
	t.Helper()

	writer, err := newRotationWriter(filepath.Join(dir, "oneapi.log"), rotationIntervalDaily, 0)
	require.NoError(t, err)
	writer.now = func() time.Time { return at }
	writer.maxActiveBytes = maxActiveBytes
	t.Cleanup(func() { require.NoError(t, writer.Close()) })
	return writer
}

// TestRotationWriterRotatesOnSize verifies the active file is cut at the
// ceiling and the cut file gets a sequence suffix that cannot collide with the
// time-window name.
func TestRotationWriterRotatesOnSize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	day := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)
	writer := newSizeCappedWriter(t, dir, 32, day)

	line := []byte("0123456789abcdef\n") // 17 bytes
	for range 5 {
		_, err := writer.Write(line)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Sync())

	active := filepath.Join(dir, "oneapi-20260905.log")
	require.FileExists(t, active)

	// 17-byte lines against a 32-byte ceiling: every second line rotates.
	require.FileExists(t, filepath.Join(dir, "oneapi-20260905-0001.log"))
	require.FileExists(t, filepath.Join(dir, "oneapi-20260905-0002.log"))
	require.FileExists(t, filepath.Join(dir, "oneapi-20260905-0003.log"))
	require.FileExists(t, filepath.Join(dir, "oneapi-20260905-0004.log"))

	info, err := os.Stat(active)
	require.NoError(t, err)
	require.LessOrEqual(t, info.Size(), int64(32), "the active file must stay under the ceiling")

	// Nothing is lost: every line landed in exactly one file.
	var total int64
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, entry := range entries {
		fi, statErr := entry.Info()
		require.NoError(t, statErr)
		total += fi.Size()
	}
	require.Equal(t, int64(len(line)*5), total, "size rotation must not drop or duplicate bytes")
}

// TestRotationWriterDoesNotRescanSequencesOnEverySizeRotation verifies a
// running writer discovers existing suffixes once and then advances its local
// sequence. Re-reading a directory after every cut turns one busy window into
// quadratic metadata work.
func TestRotationWriterDoesNotRescanSequencesOnEverySizeRotation(t *testing.T) {
	dir := t.TempDir()
	original := rotationReadDir
	reads := 0
	rotationReadDir = func(path string) ([]os.DirEntry, error) {
		reads++
		return original(path)
	}
	t.Cleanup(func() { rotationReadDir = original })

	writer := newSizeCappedWriter(t, dir, 8, time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC))
	for range 10 {
		_, err := writer.Write([]byte("0123456789\n"))
		require.NoError(t, err)
	}
	require.Equal(t, 1, reads, "only initial open/restart may scan existing suffixes")
}

// TestRotationWriterSizeCapDisabled verifies a zero ceiling restores the
// pre-W0.2 behavior of a purely time-window writer.
func TestRotationWriterSizeCapDisabled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	day := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)
	writer := newSizeCappedWriter(t, dir, 0, day)

	for range 20 {
		_, err := writer.Write([]byte("0123456789abcdef\n"))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Sync())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no size rotation may happen when the ceiling is zero")
	require.Equal(t, "oneapi-20260905.log", entries[0].Name())
}

// TestRotationWriterOversizedEntryDoesNotSpin verifies a single entry larger
// than the whole ceiling is written intact into an empty file instead of
// rotating forever or being truncated.
func TestRotationWriterOversizedEntryDoesNotSpin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	day := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)
	writer := newSizeCappedWriter(t, dir, 8, day)

	huge := []byte(strings.Repeat("x", 64) + "\n")
	written, err := writer.Write(huge)
	require.NoError(t, err)
	require.Equal(t, len(huge), written, "an oversized entry must be written whole")

	written, err = writer.Write(huge)
	require.NoError(t, err)
	require.Equal(t, len(huge), written)
	require.NoError(t, writer.Sync())

	// The first oversized entry filled an empty file, so exactly one rotation
	// separates the two.
	require.FileExists(t, filepath.Join(dir, "oneapi-20260905-0001.log"))
	require.NoFileExists(t, filepath.Join(dir, "oneapi-20260905-0002.log"))
}

// TestRotationWriterResumesSequenceAfterRestart verifies a restarted process
// never overwrites the files a previous run rotated.
func TestRotationWriterResumesSequenceAfterRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	day := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)

	first := newSizeCappedWriter(t, dir, 32, day)
	for range 3 {
		_, err := first.Write([]byte("0123456789abcdef\n"))
		require.NoError(t, err)
	}
	require.NoError(t, first.Sync())
	require.FileExists(t, filepath.Join(dir, "oneapi-20260905-0001.log"))
	require.FileExists(t, filepath.Join(dir, "oneapi-20260905-0002.log"))

	before, err := os.ReadFile(filepath.Join(dir, "oneapi-20260905-0001.log"))
	require.NoError(t, err)

	second := newSizeCappedWriter(t, dir, 32, day)
	for range 3 {
		_, err := second.Write([]byte("0123456789abcdef\n"))
		require.NoError(t, err)
	}
	require.NoError(t, second.Sync())

	after, err := os.ReadFile(filepath.Join(dir, "oneapi-20260905-0001.log"))
	require.NoError(t, err)
	require.Equal(t, before, after, "a restart must not overwrite an already rotated file")
	require.FileExists(t, filepath.Join(dir, "oneapi-20260905-0004.log"))
}

// TestRotationWriterCarriesExistingSizeAcrossRestart verifies the byte counter
// starts from what is already on disk. Without that a restart would let the
// active file grow to the ceiling a second time.
func TestRotationWriterCarriesExistingSizeAcrossRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	day := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)

	active := filepath.Join(dir, "oneapi-20260905.log")
	require.NoError(t, os.WriteFile(active, make([]byte, 30), 0o600))

	writer := newSizeCappedWriter(t, dir, 32, day)
	_, err := writer.Write([]byte("0123456789abcdef\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Sync())

	require.FileExists(t, filepath.Join(dir, "oneapi-20260905-0001.log"),
		"the pre-existing 30 bytes must count against the 32-byte ceiling")
}

// TestSizeRotatedFilesAreSweepable is the containment guarantee: files produced
// by size rotation must be visible to BOTH sweeps, otherwise the new rotation
// would simply leak files the old one never created.
func TestSizeRotatedFilesAreSweepable(t *testing.T) {
	t.Parallel()

	// The size-ceiling and free-disk sweeps in disk_guard.go select by name.
	require.True(t, isLogFileName("oneapi-20260905-0001.log"))

	// The rotation writer's own age-based purge parses the name.
	stamp, ok := parseRotationStamp("20260905-0001")
	require.True(t, ok, "a sequenced name must parse as its window")
	require.Equal(t, time.Date(2026, time.September, 5, 0, 0, 0, 0, time.UTC), stamp)

	hourly, ok := parseRotationStamp("2026090513-0042")
	require.True(t, ok)
	require.Equal(t, time.Date(2026, time.September, 5, 13, 0, 0, 0, time.UTC), hourly)

	// A plain window name still parses, and a foreign component still does not.
	_, ok = parseRotationStamp("20260905")
	require.True(t, ok)
	_, ok = parseRotationStamp("backup-20260905-0001")
	require.False(t, ok)
}

// TestPurgeExpiredRemovesSizeRotatedFiles verifies age-based purging reaches
// the sequenced files, not only the time-window ones.
func TestPurgeExpiredRemovesSizeRotatedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writer, err := newRotationWriter(filepath.Join(dir, "oneapi.log"), rotationIntervalDaily, 2)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, writer.Close()) })

	stale := filepath.Join(dir, "oneapi-20260901-0001.log")
	fresh := filepath.Join(dir, "oneapi-20260905-0001.log")
	require.NoError(t, os.WriteFile(stale, []byte("old"), 0o600))
	require.NoError(t, os.WriteFile(fresh, []byte("new"), 0o600))

	require.NoError(t, writer.purgeExpired(time.Date(2026, time.September, 5, 0, 0, 0, 0, time.UTC)))

	require.NoFileExists(t, stale, "an expired size-rotated file must be purged")
	require.FileExists(t, fresh)
}

// TestActiveFileCeilingInertWithOnlyOneLogFile verifies the logger refuses to
// pretend it can bound a file it is forbidden to rotate. Rejecting the
// combination at startup is common/config's job; here the guarantee is only
// that the writer does no size rotation at all.
func TestActiveFileCeilingInertWithOnlyOneLogFile(t *testing.T) {
	prevOnly, prevMB := config.OnlyOneLogFile, config.LogMaxActiveFileSizeMB
	t.Cleanup(func() {
		config.OnlyOneLogFile, config.LogMaxActiveFileSizeMB = prevOnly, prevMB
	})

	config.LogMaxActiveFileSizeMB = 64
	config.OnlyOneLogFile = false
	require.Equal(t, int64(64)<<20, activeFileCeilingBytes())

	config.OnlyOneLogFile = true
	require.Zero(t, activeFileCeilingBytes(),
		"ONLY_ONE_LOG_FILE removes the rotation sink, so no size ceiling can be enforced by rotating")
}
