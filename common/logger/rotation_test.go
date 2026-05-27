package logger

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRotationWindowBoundaries(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		when     time.Time
		interval rotationInterval
		start    time.Time
		next     time.Time
	}{
		{
			name:     "hourly",
			when:     time.Date(2025, time.March, 3, 13, 45, 0, 0, time.UTC),
			interval: rotationIntervalHourly,
			start:    time.Date(2025, time.March, 3, 13, 0, 0, 0, time.UTC),
			next:     time.Date(2025, time.March, 3, 14, 0, 0, 0, time.UTC),
		},
		{
			name:     "daily",
			when:     time.Date(2025, time.March, 3, 13, 45, 0, 0, time.UTC),
			interval: rotationIntervalDaily,
			start:    time.Date(2025, time.March, 3, 0, 0, 0, 0, time.UTC),
			next:     time.Date(2025, time.March, 4, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "weekly",
			when:     time.Date(2025, time.March, 6, 10, 0, 0, 0, time.UTC),
			interval: rotationIntervalWeekly,
			start:    time.Date(2025, time.March, 3, 0, 0, 0, 0, time.UTC),
			next:     time.Date(2025, time.March, 10, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, next := tc.interval.windowBounds(tc.when)
			require.Equal(t, tc.start, start)
			require.Equal(t, tc.next, next)
		})
	}
}

func TestRotationWriterDaily(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	writer, err := newRotationWriter(logFile, rotationIntervalDaily, 0)
	require.NoError(t, err)
	defer require.NoError(t, writer.Close())

	dayOne := time.Date(2025, time.January, 1, 10, 0, 0, 0, time.UTC)
	writer.now = func() time.Time { return dayOne }

	_, err = writer.Write([]byte("first entry\n"))
	require.NoError(t, err)

	dayTwo := dayOne.Add(25 * time.Hour)
	writer.now = func() time.Time { return dayTwo }

	_, err = writer.Write([]byte("second entry\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Sync())

	dayOnePath := filepath.Join(dir, "app-20250101.log")
	dayTwoPath := filepath.Join(dir, "app-20250102.log")

	firstContent, err := os.ReadFile(dayOnePath)
	require.NoError(t, err)
	require.Contains(t, string(firstContent), "first entry")

	secondContent, err := os.ReadFile(dayTwoPath)
	require.NoError(t, err)
	require.Contains(t, string(secondContent), "second entry")
}

func TestRotationWriterRetention(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	writer, err := newRotationWriter(logFile, rotationIntervalDaily, 1)
	require.NoError(t, err)
	defer require.NoError(t, writer.Close())

	dayOne := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)
	dayTwo := dayOne.Add(24 * time.Hour)
	dayThree := dayOne.Add(48 * time.Hour)

	writer.now = func() time.Time { return dayOne }
	_, err = writer.Write([]byte("day one\n"))
	require.NoError(t, err)

	writer.now = func() time.Time { return dayTwo }
	_, err = writer.Write([]byte("day two\n"))
	require.NoError(t, err)

	firstLog := filepath.Join(dir, "app-20250101.log")
	secondLog := filepath.Join(dir, "app-20250102.log")
	thirdLog := filepath.Join(dir, "app-20250103.log")
	require.FileExists(t, firstLog)

	writer.now = func() time.Time { return dayThree }
	_, err = writer.Write([]byte("day three\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Sync())

	_, err = os.Stat(firstLog)
	require.ErrorIs(t, err, os.ErrNotExist)

	require.FileExists(t, secondLog)
	require.FileExists(t, thirdLog)
}

// TestRotationWriterHourly exercises the core symptom of gh#347: two writes in
// distinct hour windows must produce two distinct files. Before the fix the
// filename only encoded YYYYMMDD, so both writes landed in one file.
func TestRotationWriterHourly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	writer, err := newRotationWriter(logFile, rotationIntervalHourly, 0)
	require.NoError(t, err)
	defer require.NoError(t, writer.Close())

	hourOne := time.Date(2025, time.January, 1, 10, 30, 0, 0, time.UTC)
	writer.now = func() time.Time { return hourOne }
	_, err = writer.Write([]byte("hour 10 entry\n"))
	require.NoError(t, err)

	hourTwo := hourOne.Add(time.Hour) // 11:30 UTC, same day
	writer.now = func() time.Time { return hourTwo }
	_, err = writer.Write([]byte("hour 11 entry\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Sync())

	hourOnePath := filepath.Join(dir, "app-2025010110.log")
	hourTwoPath := filepath.Join(dir, "app-2025010111.log")

	firstContent, err := os.ReadFile(hourOnePath)
	require.NoError(t, err)
	require.Contains(t, string(firstContent), "hour 10 entry")
	require.NotContains(t, string(firstContent), "hour 11 entry")

	secondContent, err := os.ReadFile(hourTwoPath)
	require.NoError(t, err)
	require.Contains(t, string(secondContent), "hour 11 entry")
	require.NotContains(t, string(secondContent), "hour 10 entry")
}

// TestRotationWriterHourlySameHour confirms that writes within the same hour
// window share a single file (no spurious rotation on sub-hour calls).
func TestRotationWriterHourlySameHour(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	writer, err := newRotationWriter(logFile, rotationIntervalHourly, 0)
	require.NoError(t, err)
	defer require.NoError(t, writer.Close())

	base := time.Date(2025, time.January, 1, 10, 5, 0, 0, time.UTC)
	writer.now = func() time.Time { return base }
	_, err = writer.Write([]byte("first within hour\n"))
	require.NoError(t, err)

	later := base.Add(30 * time.Minute) // 10:35 UTC, same hour window
	writer.now = func() time.Time { return later }
	_, err = writer.Write([]byte("second within hour\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Sync())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "expected exactly one file for two writes in the same hour")

	content, err := os.ReadFile(filepath.Join(dir, "app-2025010110.log"))
	require.NoError(t, err)
	require.Contains(t, string(content), "first within hour")
	require.Contains(t, string(content), "second within hour")
}

// TestRotationWriterHourlyAcrossDay covers the UTC midnight boundary: the last
// hour of one day and the first hour of the next must land in separate files
// with correctly stamped names.
func TestRotationWriterHourlyAcrossDay(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	writer, err := newRotationWriter(logFile, rotationIntervalHourly, 0)
	require.NoError(t, err)
	defer require.NoError(t, writer.Close())

	lateHour := time.Date(2025, time.January, 1, 23, 45, 0, 0, time.UTC)
	writer.now = func() time.Time { return lateHour }
	_, err = writer.Write([]byte("late entry\n"))
	require.NoError(t, err)

	earlyHour := time.Date(2025, time.January, 2, 0, 5, 0, 0, time.UTC)
	writer.now = func() time.Time { return earlyHour }
	_, err = writer.Write([]byte("early entry\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Sync())

	late, err := os.ReadFile(filepath.Join(dir, "app-2025010123.log"))
	require.NoError(t, err)
	require.Contains(t, string(late), "late entry")

	early, err := os.ReadFile(filepath.Join(dir, "app-2025010200.log"))
	require.NoError(t, err)
	require.Contains(t, string(early), "early entry")
}

// TestRotationWriterHourlyRetention verifies the retention sweep deletes
// hour-stamped files older than the retention window and keeps newer ones.
func TestRotationWriterHourlyRetention(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	writer, err := newRotationWriter(logFile, rotationIntervalHourly, 1)
	require.NoError(t, err)
	defer require.NoError(t, writer.Close())

	// Seed a file from before the retention window (well beyond 1 day old).
	oldPath := filepath.Join(dir, "app-2024123012.log")
	require.NoError(t, os.WriteFile(oldPath, []byte("old\n"), 0o644))

	// Seed a file within the retention window (only a few hours old at the
	// time of the next write below).
	recentPath := filepath.Join(dir, "app-2025010111.log")
	require.NoError(t, os.WriteFile(recentPath, []byte("recent\n"), 0o644))

	// Write at 2025-01-02 02:30 UTC. currentStart = 02:00, threshold = 1 day
	// before that = 2025-01-01 02:00, so:
	//   - 2024-12-30 12:00 → deleted
	//   - 2025-01-01 11:00 → kept
	now := time.Date(2025, time.January, 2, 2, 30, 0, 0, time.UTC)
	writer.now = func() time.Time { return now }
	_, err = writer.Write([]byte("current\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Sync())

	_, err = os.Stat(oldPath)
	require.ErrorIs(t, err, os.ErrNotExist, "expired hourly file should have been purged")

	require.FileExists(t, recentPath, "file inside the retention window must remain")
	require.FileExists(t, filepath.Join(dir, "app-2025010202.log"), "active file must exist")
}

// TestRotationWriterHourlyRetentionPurgesLegacyDailyFiles guards against
// orphaned daily-named files left behind by the buggy release: when a user
// upgrades and switches to hourly, files written under the old YYYYMMDD-only
// layout must still be cleaned up by retention.
func TestRotationWriterHourlyRetentionPurgesLegacyDailyFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	writer, err := newRotationWriter(logFile, rotationIntervalHourly, 1)
	require.NoError(t, err)
	defer require.NoError(t, writer.Close())

	legacyPath := filepath.Join(dir, "app-20241201.log") // 8-char (daily) layout
	require.NoError(t, os.WriteFile(legacyPath, []byte("legacy daily-named\n"), 0o644))

	now := time.Date(2025, time.January, 2, 2, 30, 0, 0, time.UTC)
	writer.now = func() time.Time { return now }
	_, err = writer.Write([]byte("current\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Sync())

	_, err = os.Stat(legacyPath)
	require.ErrorIs(t, err, os.ErrNotExist, "legacy daily-named file must be purged when expired")
}

// TestCreateRotationSinkHourly is a behavior test that drives the full sink
// URL pathway used by SetupLogger: build a sink URL with interval=hourly,
// hand it to createRotationSink (the registered zap sink factory), write a
// line, and verify the resulting filename carries the hourly stamp.
func TestCreateRotationSinkHourly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	rawURL, err := buildRotationSinkURL(logFile, rotationIntervalHourly, 0)
	require.NoError(t, err)

	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	require.Equal(t, rotationScheme, parsed.Scheme)
	require.Equal(t, "hourly", parsed.Query().Get("interval"))

	sink, err := createRotationSink(parsed)
	require.NoError(t, err)
	defer func() { require.NoError(t, sink.Close()) }()

	_, err = sink.Write([]byte("hello\n"))
	require.NoError(t, err)
	require.NoError(t, sink.Sync())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	name := entries[0].Name()
	require.True(t, strings.HasPrefix(name, "app-"))
	require.True(t, strings.HasSuffix(name, ".log"))

	stamp := strings.TrimSuffix(strings.TrimPrefix(name, "app-"), ".log")
	require.Len(t, stamp, len("2006010215"),
		"hourly sink should produce YYYYMMDDHH-stamped filenames, got %s", name)
	_, err = time.ParseInLocation("2006010215", stamp, time.UTC)
	require.NoError(t, err, "filename stamp must parse as YYYYMMDDHH, got %s", stamp)
}
