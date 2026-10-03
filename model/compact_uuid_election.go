package model

// This file implements single-owner election (AUTO-008, proposal section 8.3).
//
// The most important property here is what election is NOT: it is not a correctness
// dependency. Every mutating side effect is independently safe under concurrency — DDL is
// verify-before/create/verify-after, updates are conditional on the exact observed row state,
// triggers and indexes are body- and shape-verified, and marker inserts are idempotent after
// duplicate classification and reread. Election exists to stop several instances doing the
// same expensive work at once, and losing it must never corrupt anything.
//
// The lock key is derived from a compile-time namespace plus normalized database identity, so
// two unrelated one-api databases sharing one server never contend.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// compactLockNamespace scopes every ownership lock to this migration generation.
const compactLockNamespace = "one-api:" + compactMigrationGeneration

// compactOwnerCounter issues process-unique ownership tokens for pass-epoch identity.
var compactOwnerCounter atomic.Uint64

// compactInMemoryLocks serializes ownership for in-memory SQLite, which has no file to lock.
var compactInMemoryLocks sync.Map

// compactOwnership is one acquired ownership claim.
type compactOwnership struct {
	// token identifies this claim within the process, feeding the clean-pass epoch.
	token uint64
	// release relinquishes the claim; it is always non-nil for an acquired ownership.
	release func()
	// verify reports whether the claim is still held; it is always non-nil.
	verify func(ctx context.Context) (bool, error)
}

// compactLockKey derives the ownership key from normalized database identity.
//
// Identity comes from the dialect and the database's own name, not from the DSN: a DSN carries
// credentials, and two instances legitimately reach the same database through different DSNs
// (different hosts, pool settings, or socket paths). Hashing the DSN would let them both think
// they were sole owner.
// Parameters:
//   - ctx: context bounding the identity query.
//   - db: primary handle whose identity scopes the lock.
//
// Return values:
//   - string: normalized lock key.
//   - error: wrapped error when the identity cannot be read.
func compactLockKey(ctx context.Context, db *gorm.DB) (string, error) {
	dialect := dialectName(db)
	identity := ""
	switch dialect {
	case "postgres":
		if err := db.WithContext(ctx).Raw("SELECT current_database() || '.' || CURRENT_SCHEMA()").
			Scan(&identity).Error; err != nil {
			return "", errors.Wrap(err, "read postgres database identity for compact lock key")
		}
	case "mysql":
		if err := db.WithContext(ctx).Raw("SELECT DATABASE()").Scan(&identity).Error; err != nil {
			return "", errors.Wrap(err, "read mysql database identity for compact lock key")
		}
	case "sqlite":
		path, err := sqliteDatabasePath(ctx, db)
		if err != nil {
			return "", err
		}
		identity = path
		if identity == "" {
			// An in-memory database has no file, and every in-memory handle would otherwise
			// report the same empty path and therefore share one lock key — two unrelated
			// in-memory databases in one process would contend. They are process-local by
			// definition, so the pool's identity is the correct scope for them.
			pool, err := db.DB()
			if err != nil {
				return "", errors.Wrap(err, "read sqlite pool identity for compact lock key")
			}
			identity = "memory:" + fmt.Sprintf("%p", pool)
		}
	default:
		return "", errors.Errorf("compact uuid storage has no lock contract for dialect %q", dialect)
	}
	return compactLockNamespace + ":" + dialect + ":" + identity, nil
}

// sqliteDatabasePath returns the canonical file path backing a SQLite handle.
// Parameters:
//   - ctx: context bounding the metadata query.
//   - db: SQLite handle to inspect.
//
// Return values:
//   - string: canonical absolute path, or an empty string for an in-memory database.
//   - error: wrapped error when the metadata cannot be read.
func sqliteDatabasePath(ctx context.Context, db *gorm.DB) (string, error) {
	rows := []struct {
		File string `gorm:"column:file"`
	}{}
	if err := db.WithContext(ctx).Raw("PRAGMA database_list").Scan(&rows).Error; err != nil {
		return "", errors.Wrap(err, "read sqlite database path for compact lock key")
	}
	for _, row := range rows {
		if row.File == "" {
			continue
		}
		absolute, err := filepath.Abs(row.File)
		if err != nil {
			return "", errors.Wrap(err, "canonicalize sqlite database path")
		}
		return absolute, nil
	}
	return "", nil
}

// acquireCompactOwnership attempts to become the topology's single mutating owner.
//
// Acquisition is capped at compactLockTimeout (at most five seconds by configuration bound).
// A failure to acquire is an ordinary, expected outcome — another instance owns the work — and
// is reported as ok=false rather than as an error.
// Parameters:
//   - ctx: context bounding the acquisition.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - *compactOwnership: the claim when acquired.
//   - bool: true when this call took ownership.
//   - error: wrapped error only when the lock could not be attempted at all.
func acquireCompactOwnership(ctx context.Context, topology *databaseTopology) (*compactOwnership, bool, error) {
	lockCtx, cancel := context.WithTimeout(ctx, compactLockTimeout())
	defer cancel()

	key, err := compactLockKey(lockCtx, topology.primary)
	if err != nil {
		return nil, false, err
	}
	token := compactOwnerCounter.Add(1)

	switch dialectName(topology.primary) {
	case "postgres":
		return acquirePostgresOwnership(lockCtx, topology.primary, key, token)
	case "mysql":
		return acquireMySQLOwnership(lockCtx, topology.primary, key, token)
	case "sqlite":
		// The path is re-derived rather than parsed back out of the key: an in-memory
		// database's key carries a pool identity, not a filename, and mistaking one for the
		// other would try to create a sidecar lock file named after a pointer.
		path, err := sqliteDatabasePath(lockCtx, topology.primary)
		if err != nil {
			return nil, false, err
		}
		return acquireSQLiteOwnership(key, path, token)
	default:
		return nil, false, errors.Errorf("compact uuid storage has no lock contract for dialect %q",
			dialectName(topology.primary))
	}
}

// compactLockID hashes a lock key into the 64-bit space PostgreSQL advisory locks require.
// Parameters:
//   - key: normalized lock key.
//
// Return values:
//   - int64: stable 64-bit identifier.
func compactLockID(key string) int64 {
	digest := fnv.New64a()
	// fnv's Write never returns an error, which is why the result is discarded.
	_, _ = digest.Write([]byte(key))
	return int64(digest.Sum64())
}

// discardPinnedSession removes a pinned connection from the pool instead of returning it.
//
// This is the safety valve for every path where a server-side lock's state is unknown — an
// acquire whose result was never read, or a release whose unlock failed. A session lock lives
// exactly as long as its server session, so the only deterministic way to guarantee "not held"
// is to make the server session die. Returning driver.ErrBadConn from Raw marks the connection
// bad, and the pool then closes the real network connection rather than reusing it; a plain
// Close would pool it alive, lock and all.
//
// The cost is one reconnection on the next acquisition, once per fault — not per cycle.
// Parameters:
//   - session: pinned connection whose lock state is unknown.
//
// Return values: none.
func discardPinnedSession(session *sql.Conn) {
	_ = session.Raw(func(driverConn any) error {
		return driver.ErrBadConn
	})
	_ = session.Close()
}

// acquirePostgresOwnership takes a session advisory lock on a pinned connection.
//
// The connection is pinned deliberately: a session advisory lock lives on one backend, so a
// lock taken through the pool could be released by a later statement landing on a different
// connection, or silently persist on a connection returned to the pool.
// Parameters:
//   - ctx: context bounding the acquisition.
//   - db: PostgreSQL primary handle.
//   - key: normalized lock key.
//   - token: ownership token for this claim.
//
// Return values:
//   - *compactOwnership: the claim when acquired.
//   - bool: true when this call took ownership.
//   - error: wrapped error when the lock statement fails.
func acquirePostgresOwnership(ctx context.Context, db *gorm.DB, key string, token uint64) (*compactOwnership, bool, error) {
	lockID := compactLockID(key)
	conn, err := db.WithContext(context.WithoutCancel(ctx)).DB()
	if err != nil {
		return nil, false, errors.Wrap(err, "obtain postgres pool for compact ownership")
	}
	session, err := conn.Conn(ctx)
	if err != nil {
		return nil, false, errors.Wrap(err, "pin postgres connection for compact ownership")
	}

	acquired := false
	if err := session.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired); err != nil {
		// The lock's server-side state is UNKNOWN here: a cancellation or timeout can land
		// after the server granted the lock but before the client read the result. For a
		// canceled query the driver usually marks the connection bad on its own, so Close
		// would discard it — measured under AUTO-T11's kill sweep, that self-healing clears
		// the lock within milliseconds. The explicit discard exists for the errors that do
		// NOT poison the connection: pooling a healthy session whose lock state is unknown
		// would strand the advisory lock for up to the connection's pooled lifetime, and no
		// process could acquire ownership while it lived. Discarding makes release
		// deterministic instead of driver-dependent.
		discardPinnedSession(session)
		return nil, false, errors.Wrap(err, "acquire postgres advisory lock for compact ownership")
	}
	if !acquired {
		// The server answered: no lock is held on this session, so pooling it is safe.
		_ = session.Close()
		return nil, false, nil
	}

	return &compactOwnership{
		token: token,
		release: func() {
			// Release on a detached context: the usual reason a cycle ends is cancellation,
			// and an unlock issued on the cancelled context would fail and leak the lock
			// until the backend exits. If even the detached unlock fails, the session's
			// state is unknown and it must not be pooled — see the acquire path.
			detached := context.WithoutCancel(ctx)
			if _, err := session.ExecContext(detached, "SELECT pg_advisory_unlock($1)", lockID); err != nil {
				discardPinnedSession(session)
				return
			}
			_ = session.Close()
		},
		verify: func(verifyCtx context.Context) (bool, error) {
			// Ownership is checked before and after each side effect. A dropped connection
			// silently releases the session lock, so "still connected and still holding" is
			// the only meaningful evidence.
			//
			// pg_locks does not store a 64-bit advisory key in one column: it splits it into
			// classid (high 32 bits) and objid (low 32 bits), both of type oid, with
			// objsubid = 1 marking the single-argument form. Comparing the whole key against
			// objid fails outright with "OID out of range" rather than simply not matching.
			held := false
			key := uint64(lockID)
			classID := int64(uint32(key >> 32))
			objID := int64(uint32(key))
			query := "SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype = 'advisory'" +
				" AND classid = $1::bigint::oid AND objid = $2::bigint::oid AND objsubid = 1" +
				" AND pid = pg_backend_pid() AND granted)"
			if err := session.QueryRowContext(verifyCtx, query, classID, objID).Scan(&held); err != nil {
				return false, errors.Wrap(err, "verify postgres advisory lock for compact ownership")
			}
			return held, nil
		},
	}, true, nil
}

// acquireMySQLOwnership takes a named lock on a pinned connection.
// Parameters:
//   - ctx: context bounding the acquisition.
//   - db: MySQL primary handle.
//   - key: normalized lock key.
//   - token: ownership token for this claim.
//
// Return values:
//   - *compactOwnership: the claim when acquired.
//   - bool: true when this call took ownership.
//   - error: wrapped error when the lock statement fails.
func acquireMySQLOwnership(ctx context.Context, db *gorm.DB, key string, token uint64) (*compactOwnership, bool, error) {
	// GET_LOCK names are limited to 64 characters, so the normalized key is hashed rather
	// than truncated: truncation would let two long identities collide.
	name := compactLockNamespace + ":" + strings.ToLower(strconv.FormatUint(uint64(compactLockID(key)), 16))

	conn, err := db.WithContext(context.WithoutCancel(ctx)).DB()
	if err != nil {
		return nil, false, errors.Wrap(err, "obtain mysql pool for compact ownership")
	}
	session, err := conn.Conn(ctx)
	if err != nil {
		return nil, false, errors.Wrap(err, "pin mysql connection for compact ownership")
	}

	var acquired *int64
	timeout := int(compactLockTimeout().Seconds())
	if timeout < 1 {
		timeout = 1
	}
	if err := session.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", name, timeout).Scan(&acquired); err != nil {
		// Same stranded-lock window as the PostgreSQL acquire: the server may have granted
		// GET_LOCK even though the client never read the answer, so this session must not be
		// pooled alive. See discardPinnedSession.
		discardPinnedSession(session)
		return nil, false, errors.Wrap(err, "acquire mysql lock for compact ownership")
	}
	if acquired == nil || *acquired != 1 {
		_ = session.Close()
		return nil, false, nil
	}

	return &compactOwnership{
		token: token,
		release: func() {
			detached := context.WithoutCancel(ctx)
			if _, err := session.ExecContext(detached, "SELECT RELEASE_LOCK(?)", name); err != nil {
				discardPinnedSession(session)
				return
			}
			_ = session.Close()
		},
		verify: func(verifyCtx context.Context) (bool, error) {
			var held *int64
			if err := session.QueryRowContext(verifyCtx, "SELECT IS_USED_LOCK(?) = CONNECTION_ID()", name).
				Scan(&held); err != nil {
				return false, errors.Wrap(err, "verify mysql lock for compact ownership")
			}
			return held != nil && *held == 1, nil
		},
	}, true, nil
}

// acquireSQLiteOwnership takes a non-blocking advisory claim for a SQLite database.
//
// A file-backed database uses an OS advisory lock on a sidecar path derived from the canonical
// database path, which the kernel releases on process exit. It deliberately does not hold a
// database writer lock: doing so would block every ordinary application write for the whole
// cycle. An in-memory database is process-local by definition and uses a process mutex.
// Parameters:
//   - key: normalized lock key scoping this claim.
//   - path: canonical database file path, or an empty string for an in-memory database.
//   - token: ownership token for this claim.
//
// Return values:
//   - *compactOwnership: the claim when acquired.
//   - bool: true when this call took ownership.
//   - error: wrapped error when the sidecar lock cannot be attempted.
func acquireSQLiteOwnership(key string, path string, token uint64) (*compactOwnership, bool, error) {
	if path == "" {
		// In-memory SQLite: no file exists to lock, and no other process can reach it.
		if _, loaded := compactInMemoryLocks.LoadOrStore(key, token); loaded {
			return nil, false, nil
		}
		return &compactOwnership{
			token:   token,
			release: func() { compactInMemoryLocks.Delete(key) },
			verify: func(context.Context) (bool, error) {
				held, ok := compactInMemoryLocks.Load(key)
				return ok && held == any(token), nil
			},
		}, true, nil
	}

	sidecar := path + ".compact-uuid.lock"
	file, err := os.OpenFile(sidecar, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, errors.Wrap(err, "open compact ownership sidecar lock")
	}
	locked, err := tryLockFile(file)
	if err != nil {
		_ = file.Close()
		return nil, false, err
	}
	if !locked {
		_ = file.Close()
		return nil, false, nil
	}

	return &compactOwnership{
		token: token,
		release: func() {
			_ = unlockFile(file)
			_ = file.Close()
		},
		verify: func(context.Context) (bool, error) {
			// The kernel holds this lock for the lifetime of the open descriptor, so it
			// cannot be lost while the process lives; process exit releases it implicitly.
			return true, nil
		},
	}, true, nil
}
