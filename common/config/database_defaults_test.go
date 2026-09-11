package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDatabasePoolDefaultsTarget100RPS pins the bounded connection-pool policy
// for a single One API instance serving about 100 requests per second.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestDatabasePoolDefaultsTarget100RPS(t *testing.T) {
	require.Equal(t, 10, defaultSQLMaxIdleConns)
	require.Equal(t, 50, defaultSQLMaxOpenConns)
	require.Equal(t, 300, defaultSQLMaxLifetimeSeconds)
	require.LessOrEqual(t, defaultSQLMaxIdleConns, defaultSQLMaxOpenConns)
}
