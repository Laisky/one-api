package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeDateRange(t *testing.T) {
	t.Parallel()
	t.Run("single day", func(t *testing.T) {
		t.Parallel()
		s, e, err := NormalizeDateRange("2025-01-15", "2025-01-15", 10)
		require.NoError(t, err)
		require.Equal(t, int64(24*3600), e-s, "expected 1 day span")
	})

	t.Run("multi day inclusive", func(t *testing.T) {
		t.Parallel()
		s, e, err := NormalizeDateRange("2025-01-01", "2025-01-03", 10)
		require.NoError(t, err)
		require.Equal(t, int64(3*24*3600), e-s, "expected 3 day span")
	})

	t.Run("leap day", func(t *testing.T) {
		t.Parallel()
		s, e, err := NormalizeDateRange("2024-02-28", "2024-03-01", 10)
		require.NoError(t, err)
		require.Equal(t, int64(3*24*3600), e-s, "expected 3 day span across leap day")
	})

	t.Run("max days exceeded", func(t *testing.T) {
		t.Parallel()
		_, _, err := NormalizeDateRange("2025-01-01", "2025-01-10", 5)
		require.Error(t, err, "expected error for exceeding max days")
	})

	t.Run("invalid order", func(t *testing.T) {
		t.Parallel()
		_, _, err := NormalizeDateRange("2025-01-10", "2025-01-01", 10)
		require.Error(t, err, "expected error for reversed dates")
	})
}

// Ensure UTC correctness by comparing boundaries explicitly.
func TestNormalizeDateRangeUTC(t *testing.T) {
	t.Parallel()
	s, e, err := NormalizeDateRange("2025-05-05", "2025-05-05", 1)
	require.NoError(t, err)
	require.Equal(t, 0, time.Unix(s, 0).UTC().Hour(), "start not at midnight UTC")
	require.Equal(t, 0, time.Unix(e, 0).UTC().Hour(), "endExclusive not at midnight UTC")
}
