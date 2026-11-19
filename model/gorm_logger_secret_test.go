package model

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestGormLoggerNeverLogsBoundValues proves SQL logging never prints bound
// values, so a token key used in a WHERE clause cannot leak into stdout even
// with per-query logging on. Defect: DEBUG=true switched the GORM logger to
// Info and printed relay token keys and access tokens inside SQL text.
func TestGormLoggerNeverLogsBoundValues(t *testing.T) {
	var out bytes.Buffer
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: newGormLoggerTo(&out, true)})
	require.NoError(t, err)
	db = db.Debug() // the DEBUG_SQL path: force per-query logging
	require.NoError(t, db.AutoMigrate(&Token{}))
	const secret = "sk-secret-fixture-key-0123456789abcdef"
	var token Token
	_ = db.Where("key = ?", secret).Take(&token).Error
	require.Contains(t, out.String(), "SELECT", "control: the query was logged")
	require.NotContains(t, out.String(), secret, "bound values must never be logged")
}
