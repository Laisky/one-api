package logger

import (
	"os"
	"path/filepath"
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
