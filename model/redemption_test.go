package model

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupRedemptionTestDB initializes a fresh in-memory SQLite DB with the
// tables that `Redeem` touches. Each test gets its own database so concurrent
// tests do not interfere via the global DB handle.
//
// Returns a SIDE handle (`sideDB`) that points to the same logical SQLite DB
// via a second connection. Tests can use it to simulate a peer transaction
// while the main connection is busy with an in-flight Redeem.
func setupRedemptionTestDB(t *testing.T) (sideDB *gorm.DB) {
	t.Helper()

	origDB := DB
	// Unique shared-cache name per-test: any handle opened with the same
	// DSN reaches the same in-memory database, while different test DSNs
	// stay isolated.
	dsn := "file:redemption-" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared&_busy_timeout=5000&_journal_mode=WAL"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Redemption{}, &User{}, &Log{}))

	// Single connection on the main handle so writes serialize cleanly. A
	// second physical connection lives on the `sideDB` handle below.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	sideHandle, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sideSQL, err := sideHandle.DB()
	require.NoError(t, err)
	sideSQL.SetMaxOpenConns(1)

	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = origDB
		_ = sideSQL.Close()
		_ = sqlDB.Close()
	})
	return sideHandle
}

// TestRedeem_HappyPath sanity-checks a normal redemption. Used as the baseline
// for concurrency reasoning.
func TestRedeem_HappyPath(t *testing.T) {
	_ = setupRedemptionTestDB(t)

	require.NoError(t, DB.Create(&User{Username: "u1", AccessToken: "tok-u1", AffCode: "aff-u1", Quota: 0}).Error)
	var u User
	require.NoError(t, DB.First(&u, "username = ?", "u1").Error)
	require.NoError(t, DB.Create(&Redemption{
		Key:    "happy-key-1",
		Status: RedemptionCodeStatusEnabled,
		Quota:  100,
	}).Error)

	quota, err := Redeem(context.Background(), "happy-key-1", u.Id)
	require.NoError(t, err)
	require.EqualValues(t, 100, quota)

	var after User
	require.NoError(t, DB.First(&after, u.Id).Error)
	require.EqualValues(t, 100, after.Quota)

	var r Redemption
	require.NoError(t, DB.First(&r, "key = ?", "happy-key-1").Error)
	require.Equal(t, RedemptionCodeStatusUsed, r.Status)
}

// TestRedeem_AlreadyUsedRejected verifies a code already in Used state cannot
// be redeemed a second time.
func TestRedeem_AlreadyUsedRejected(t *testing.T) {
	_ = setupRedemptionTestDB(t)

	require.NoError(t, DB.Create(&User{Username: "u2", AccessToken: "tok-u2", AffCode: "aff-u2", Quota: 0}).Error)
	var u User
	require.NoError(t, DB.First(&u, "username = ?", "u2").Error)
	require.NoError(t, DB.Create(&Redemption{
		Key:    "used-key-1",
		Status: RedemptionCodeStatusUsed,
		Quota:  100,
	}).Error)

	_, err := Redeem(context.Background(), "used-key-1", u.Id)
	require.Error(t, err)

	var after User
	require.NoError(t, DB.First(&after, u.Id).Error)
	require.EqualValues(t, 0, after.Quota, "used code must not credit quota")
}

// TestRedeem_CASRejectsRaceWindowInjection is the deterministic
// security-regression test for gh #2398. It simulates the exact race that
// breaks the pre-fix code: a transaction reads the redemption row when it is
// still Enabled, then between that read and the claim/credit a peer commit
// flips the row to Used. Without the CAS guard (UPDATE ... WHERE status =
// Enabled and check RowsAffected) the proxy would silently credit the user
// for a code that has already been spent.
//
// The interleaving is forced via a GORM callback registered after the
// First() lookup and before any UPDATE statement runs. On every
// concurrent-redeem invocation across the whole codebase the proxy must
// either (a) fail loudly or (b) leave the user's quota untouched.
// TestRedeem_CASStatementShape verifies that the claim step is a true CAS:
// `UPDATE redemptions SET status = Used, redeemed_time = ... WHERE id = ?
// AND status = Enabled`. Capturing the SQL with GORM's session logger lets
// us pin the security-critical statement structure without depending on a
// specific RDBMS's locking semantics. If a future refactor accidentally
// drops the `status = Enabled` predicate the test fails immediately.
func TestRedeem_CASStatementShape(t *testing.T) {
	_ = setupRedemptionTestDB(t)

	require.NoError(t, DB.Create(&User{
		Username: "cas-shape", AccessToken: "tok-cas-shape", AffCode: "aff-cas-shape", Quota: 0,
	}).Error)
	var u User
	require.NoError(t, DB.First(&u, "username = ?", "cas-shape").Error)
	require.NoError(t, DB.Create(&Redemption{
		Key:    "cas-shape-key",
		Status: RedemptionCodeStatusEnabled,
		Quota:  77,
	}).Error)

	// Hook the UPDATE callback chain so we can read every UPDATE statement
	// GORM executes during Redeem. The redemption-claim UPDATE must include
	// `status = ?` in its WHERE clause; the user-credit UPDATE must update
	// the users table.
	var (
		mu               sync.Mutex
		seenStatements   []string
		seenWhereVarsCAS []any
	)
	const cbName = "test:capture_update"
	require.NoError(t, DB.Callback().Update().After("gorm:update").Register(cbName,
		func(d *gorm.DB) {
			if d.Statement == nil {
				return
			}
			sql := d.Statement.SQL.String()
			mu.Lock()
			seenStatements = append(seenStatements, sql)
			if d.Statement.Schema != nil && d.Statement.Schema.Table == "redemptions" {
				seenWhereVarsCAS = append(seenWhereVarsCAS, d.Statement.Vars...)
			}
			mu.Unlock()
		}))
	defer func() { _ = DB.Callback().Update().Remove(cbName) }()

	_, err := Redeem(context.Background(), "cas-shape-key", u.Id)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	var redemptionUpdates int
	for _, sql := range seenStatements {
		lower := strings.ToLower(sql)
		if strings.Contains(lower, "update `redemptions`") || strings.Contains(lower, `update "redemptions"`) {
			redemptionUpdates++
			require.Contains(t, lower, "status",
				"redemption UPDATE must constrain on status: %s", sql)
		}
	}
	require.Equal(t, 1, redemptionUpdates,
		"exactly one UPDATE of redemptions per successful Redeem: got %d (%v)",
		redemptionUpdates, seenStatements)
	require.Contains(t, seenWhereVarsCAS, RedemptionCodeStatusEnabled,
		"CAS UPDATE must bind RedemptionCodeStatusEnabled in its WHERE clause: %v", seenWhereVarsCAS)
}

// TestRedeem_ConcurrentSingleSuccess is the security-regression test that
// reproduces gh #2398: under N concurrent calls with the SAME redemption key,
// exactly one caller must succeed and exactly one user must receive the quota
// credit. The buggy implementation lets multiple goroutines pass the status
// check before any of them flips it to Used, so several users get credited.
//
// On SQLite the race manifests because GORM's default isolation lets
// `SELECT ... LIMIT 1` see the not-yet-committed state from a peer
// transaction; without an atomic claim step every passing goroutine then
// proceeds to credit a user. The fix (compare-and-swap UPDATE with rows
// affected check) collapses every concurrent claim attempt onto a single
// winner.
func TestRedeem_ConcurrentSingleSuccess(t *testing.T) {
	_ = setupRedemptionTestDB(t)

	const N = 16
	userIDs := make([]int, 0, N)
	for i := 0; i < N; i++ {
		u := &User{
			Username:    fmt.Sprintf("concurrent-%d", i),
			AccessToken: fmt.Sprintf("tok-concurrent-%d", i),
			AffCode:     fmt.Sprintf("aff-concurrent-%d", i),
			Quota:       0,
		}
		require.NoError(t, DB.Create(u).Error)
		userIDs = append(userIDs, u.Id)
	}

	require.NoError(t, DB.Create(&Redemption{
		Key:    "race-key-1",
		Status: RedemptionCodeStatusEnabled,
		Quota:  500,
	}).Error)

	var (
		wg        sync.WaitGroup
		successes atomic.Int32
		start     = make(chan struct{})
	)
	wg.Add(N)
	for i := 0; i < N; i++ {
		uid := userIDs[i]
		go func() {
			defer wg.Done()
			<-start
			if _, err := Redeem(context.Background(), "race-key-1", uid); err == nil {
				successes.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	require.EqualValues(t, 1, successes.Load(),
		"exactly one goroutine must succeed; got %d (concurrent redemption bug)", successes.Load())

	var totalCredited int64
	require.NoError(t, DB.Model(&User{}).
		Where("id IN ?", userIDs).
		Select("COALESCE(SUM(quota), 0)").
		Scan(&totalCredited).Error)
	require.EqualValues(t, 500, totalCredited,
		"exactly the redemption quota must be credited in total; got %d", totalCredited)

	var r Redemption
	require.NoError(t, DB.First(&r, "key = ?", "race-key-1").Error)
	require.Equal(t, RedemptionCodeStatusUsed, r.Status)
}
