package common

import (
	"sync/atomic"

	"github.com/Laisky/one-api/common/config"
)

// UsingSQLite reports whether the runtime is currently configured to use the SQLite backend.
var UsingSQLite atomic.Bool

// UsingPostgreSQL reports whether PostgreSQL is active for persistence.
var UsingPostgreSQL atomic.Bool

// UsingMySQL reports whether MySQL is active for persistence.
var UsingMySQL atomic.Bool

// SQLitePath stores the absolute path of the SQLite database file.
var SQLitePath = config.SQLitePath

// SQLiteBusyTimeout contains the configured busy timeout applied to SQLite connections.
var SQLiteBusyTimeout = config.SQLiteBusyTimeout
