package model

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Concurrent application traffic during catch-up (UUID-A40).

func seedUUIDWriterLegacyLogs(t *testing.T, db *gorm.DB, count int, userCount int) {
	t.Helper()
	const chunk = 200
	for start := 1; start <= count; start += chunk {
		end := start + chunk - 1
		if end > count {
			end = count
		}
		rows := make([]map[string]any, 0, end-start+1)
		for id := start; id <= end; id++ {
			rows = append(rows, map[string]any{
				"id": id, "user_id": ((id - 1) % userCount) + 1,
				"type": LogTypeSystem, "content": "legacy log " + strconv.Itoa(id),
			})
		}
		require.NoError(t, db.Table("logs").Create(rows).Error)
	}
}

// uuidWriterAppWrite records one row an application goroutine wrote during catch-up so the test
// goroutine can prove it survived, kept its hook-assigned uuid, and kept its updated column.
type uuidWriterAppWrite struct {
	// userID is the primary key of the user the worker created.
	userID int
	// userUUID is the uuid the create hook assigned to that user.
	userUUID string
	// username is the unique username the worker wrote.
	username string
	// tokenID is the primary key of the token the worker created.
	tokenID int
	// tokenUUID is the uuid the create hook assigned to that token.
	tokenUUID string
	// remainQuota is the value the worker's non-UUID update wrote last.
	remainQuota int64
}

// runUUIDWriterAppWorker performs ordinary application reads and writes against the tables
// catch-up is reconciling, and returns what it wrote plus the slowest statement it observed.
// It never calls require: testify's FailNow is only legal on the test goroutine, so failures
// are returned and asserted after the wait group settles.
// Parameters:
//   - db: shared application handle.
//   - worker: worker index used to build unique keys.
//   - minRounds: rounds performed before the worker is allowed to stop.
//   - maxRounds: hard ceiling that bounds the test's runtime.
//   - catchUpDone: closed when the catch-up drain finishes.
//
// Return values:
//   - []uuidWriterAppWrite: rows this worker wrote.
//   - time.Duration: slowest single application statement observed.
//   - error: first failure encountered.
func runUUIDWriterAppWorker(db *gorm.DB, worker int, minRounds int, maxRounds int,
	catchUpDone <-chan struct{}) ([]uuidWriterAppWrite, time.Duration, error) {
	writes := make([]uuidWriterAppWrite, 0, minRounds)
	slowest := time.Duration(0)

	// timed runs one application statement and folds its latency into the worker's maximum.
	timed := func(statement func() error) error {
		start := time.Now()
		err := statement()
		if elapsed := time.Since(start); elapsed > slowest {
			slowest = elapsed
		}
		return err
	}

	for round := 0; round < maxRounds; round++ {
		if round >= minRounds {
			select {
			case <-catchUpDone:
				return writes, slowest, nil
			default:
			}
		}
		suffix := strconv.Itoa(worker) + "-" + strconv.Itoa(round)

		// A UUID-aware create: the hook assigns the owned uuid, and catch-up must leave it alone.
		user := &User{
			Username:    "live-user-" + suffix,
			Password:    "password-hash",
			AccessToken: "live-access-token-" + suffix,
			AffCode:     "lv" + suffix,
		}
		if err := timed(func() error { return db.Create(user).Error }); err != nil {
			return writes, slowest, errors.Wrapf(err, "worker %d create user", worker)
		}

		readUser := &User{}
		if err := timed(func() error { return db.First(readUser, "id = ?", user.Id).Error }); err != nil {
			return writes, slowest, errors.Wrapf(err, "worker %d read user %d", worker, user.Id)
		}
		if readUser.Username != user.Username || readUser.UUID != user.UUID {
			return writes, slowest, errors.Errorf("worker %d lost the write for user %d", worker, user.Id)
		}

		token := &Token{UserId: user.Id, UserUUID: &user.UUID,
			Key: "live-token-key-" + suffix, Name: "live-token-" + suffix}
		if err := timed(func() error { return token.Insert(context.Background()) }); err != nil {
			return writes, slowest, errors.Wrapf(err, "worker %d insert token", worker)
		}

		// An ordinary non-UUID update against a table the FK phase is reconciling.
		quota := int64(1000 + round)
		if err := timed(func() error {
			return db.Model(&Token{}).Where("id = ?", token.Id).
				Update("remain_quota", quota).Error
		}); err != nil {
			return writes, slowest, errors.Wrapf(err, "worker %d update token %d", worker, token.Id)
		}

		readToken := &Token{}
		if err := timed(func() error { return db.First(readToken, "id = ?", token.Id).Error }); err != nil {
			return writes, slowest, errors.Wrapf(err, "worker %d read token %d", worker, token.Id)
		}
		if readToken.RemainQuota != quota {
			return writes, slowest, errors.Errorf("worker %d lost the quota update for token %d", worker, token.Id)
		}

		writes = append(writes, uuidWriterAppWrite{
			userID: user.Id, userUUID: user.UUID, username: user.Username,
			tokenID: token.Id, tokenUUID: token.UUID, remainQuota: quota,
		})
	}
	return writes, slowest, nil
}

// TestConcurrentApplicationTrafficDuringCatchUp covers UUID-A40: ordinary application reads and
// writes run against the very tables catch-up is reconciling, and none of them may be lost,
// deadlocked, or made to wait for a lock longer than the configured timeout, while catch-up
// still drains the whole legacy backlog without error.
func TestConcurrentApplicationTrafficDuringCatchUp(t *testing.T) {
	const (
		legacyUsers = 3000
		legacyLogs  = 3000
		workerCount = 4
		minRounds   = 8
		maxRounds   = 120
		maxCycles   = 60
	)

	db, topology := newUnifiedTestTopology(t)
	// ":memory:" gives every new connection its own empty database, so the shared handle must
	// be pinned to one connection for concurrent goroutines to see the same data.
	pinUUIDRaceSQLiteConnection(t, db)
	seedLegacyUsers(t, db, legacyUsers)
	seedUUIDWriterLegacyLogs(t, db, legacyLogs, legacyUsers)

	lockTimeout := uuidLockTimeout()
	require.Positive(t, lockTimeout, "the configured lock timeout bounds every application statement")

	catchUpDone := make(chan struct{})
	var catchUpErr error
	var catchUpCycles int
	go func() {
		defer close(catchUpDone)
		// The cycle deadline keeps a stalled drain from hanging the package's test binary.
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		for cycle := 1; cycle <= maxCycles; cycle++ {
			// One cycle is bounded by the 10,000-row budget, so a backlog this size
			// legitimately needs several passes, exactly like the background worker.
			result, err := runUUIDMigrationCoordinator(ctx, topology, uuidMigrationModeCatchUp)
			catchUpCycles = cycle
			if err != nil {
				catchUpErr = errors.Wrapf(err, "catch-up cycle %d", cycle)
				return
			}
			if result.updated == 0 && !result.budgetExhausted {
				return
			}
		}
		catchUpErr = errors.Errorf("catch-up did not settle within %d bounded cycles", maxCycles)
	}()

	writes := make([][]uuidWriterAppWrite, workerCount)
	slowest := make([]time.Duration, workerCount)
	errs := make([]error, workerCount)
	wg := sync.WaitGroup{}
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			// Each worker keeps writing until catch-up finishes, so the traffic spans the whole
			// reconciliation rather than only its first moments.
			writes[worker], slowest[worker], errs[worker] =
				runUUIDWriterAppWorker(db, worker, minRounds, maxRounds, catchUpDone)
		}(i)
	}
	wg.Wait()
	<-catchUpDone

	for worker, err := range errs {
		require.NoErrorf(t, err, "application worker %d must not observe an error during catch-up", worker)
	}
	require.NoError(t, catchUpErr, "catch-up must complete while the application keeps serving traffic")
	require.Positive(t, catchUpCycles)

	maxLatency := time.Duration(0)
	appWrites := 0
	for worker := range writes {
		appWrites += len(writes[worker])
		if slowest[worker] > maxLatency {
			maxLatency = slowest[worker]
		}
	}
	require.GreaterOrEqual(t, appWrites, workerCount*minRounds,
		"every worker must complete its minimum round of concurrent traffic")
	// "No lock wait beyond the configured timeout": no application statement may block longer
	// than the lock budget the migration is allowed to hold.
	require.Lessf(t, maxLatency, lockTimeout,
		"an application statement waited %s, beyond the configured %s lock timeout", maxLatency, lockTimeout)

	requireUUIDWriterTrafficDurable(t, db, writes)

	// The legacy backlog drained: every seeded row has a uuid, and no uuid repeats.
	users := uuidRaceLoadPopulatedUUIDs(t, db, "users")
	require.Len(t, users, legacyUsers+appWrites, "every legacy and concurrently written user must have a uuid")
	require.Equal(t, len(users), uuidRaceCountDistinctUUIDs(t, db, "users"),
		"concurrent catch-up and application writes must not collide on a uuid")

	logs := uuidRaceLoadPopulatedUUIDs(t, db, "logs")
	require.Len(t, logs, legacyLogs, "every legacy log must have a uuid")
	require.Equal(t, legacyLogs, uuidRaceCountDistinctUUIDs(t, db, "logs"))

	tokens := uuidRaceLoadPopulatedUUIDs(t, db, "tokens")
	require.Len(t, tokens, appWrites)
	require.Equal(t, appWrites, uuidRaceCountDistinctUUIDs(t, db, "tokens"))

	// Catch-up filled every legacy log's user_uuid from its primary owner without touching the
	// denormalized uuid the concurrent UUID-aware writer had already supplied.
	var mismatched int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM logs l JOIN users u ON u.id = l.user_id
		WHERE l.user_uuid IS NULL OR l.user_uuid = '' OR l.user_uuid != u.uuid`).Scan(&mismatched).Error)
	require.Zero(t, mismatched, "catch-up must leave every log fk uuid consistent with its live owner")

	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM tokens t JOIN users u ON u.id = t.user_id
		WHERE t.user_uuid IS NULL OR t.user_uuid = '' OR t.user_uuid != u.uuid`).Scan(&mismatched).Error)
	require.Zero(t, mismatched, "a concurrently written token must keep the owner uuid its writer supplied")

	// Catch-up is marker-free no matter how much it reconciled.
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)
	t.Logf("UUID-A40: cycles=%d app_writes=%d max_app_statement=%s lock_timeout=%s",
		catchUpCycles, appWrites, maxLatency, lockTimeout)
}

// requireUUIDWriterTrafficDurable asserts every concurrently written row survived catch-up with
// the values and hook-assigned UUIDs its writer produced.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle owning the rows.
//   - writes: per-worker records of what was written.
//
// Return values: none.
func requireUUIDWriterTrafficDurable(t *testing.T, db *gorm.DB, writes [][]uuidWriterAppWrite) {
	t.Helper()
	for worker, workerWrites := range writes {
		for _, write := range workerWrites {
			user := &User{}
			require.NoErrorf(t, db.First(user, "id = ?", write.userID).Error,
				"worker %d lost user %d", worker, write.userID)
			require.Equal(t, write.username, user.Username)
			require.Equal(t, write.userUUID, user.UUID,
				"catch-up must not overwrite the uuid the create hook assigned to user %d", write.userID)

			token := &Token{}
			require.NoErrorf(t, db.First(token, "id = ?", write.tokenID).Error,
				"worker %d lost token %d", worker, write.tokenID)
			require.Equal(t, write.tokenUUID, token.UUID,
				"catch-up must not overwrite the uuid the create hook assigned to token %d", write.tokenID)
			require.Equal(t, write.remainQuota, token.RemainQuota,
				"catch-up must not clobber a concurrent non-uuid update on token %d", write.tokenID)
			require.NotNil(t, token.UserUUID)
			require.Equal(t, write.userUUID, *token.UserUUID,
				"the UUID-aware writer's denormalized owner uuid must survive catch-up")
		}
	}
}
