package model

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// redemptionMultidbCases lists the live-database backends to exercise. Each
// case is gated on its own DSN env variable so the suite stays opt-in for
// developers without a local MySQL/Postgres.
var redemptionMultidbCases = []struct {
	name    string
	backend string
}{
	{name: "PostgreSQL", backend: "postgres"},
	{name: "MySQL", backend: "mysql"},
}

// TestRedeem_ConcurrentSingleSuccess_LiveDB is the gh #2398 reproducer
// against real MySQL/PostgreSQL. SQLite serializes writes at the file level
// so the in-memory test variant cannot deterministically expose the race;
// this one can.
//
// The test only runs when the matching DSN env variable is set, mirroring
// the existing multidb test pattern (boot_migration_multidb_test.go).
//
// Pre-fix expectation: multiple goroutines succeed and the user table sums
// to N * Quota. Post-fix expectation: exactly one goroutine succeeds and the
// user table sums to exactly Quota.
func TestRedeem_ConcurrentSingleSuccess_LiveDB(t *testing.T) {
	for _, tc := range redemptionMultidbCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			db := openBackend(t, tc.backend)
			if db == nil {
				t.Skipf("%s DSN not set; skipping %s concurrent redemption test", tc.backend, tc.name)
			}
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(32)

			// Use a dedicated schema-test-namespace via a unique table prefix
			// to avoid colliding with existing data on the developer's DB.
			require.NoError(t, db.Migrator().DropTable(&Redemption{}, &User{}, &Log{}))
			require.NoError(t, db.AutoMigrate(&Redemption{}, &User{}, &Log{}))

			origDB := DB
			origLog := LOG_DB
			DB = db
			LOG_DB = db
			t.Cleanup(func() {
				_ = db.Migrator().DropTable(&Redemption{}, &User{}, &Log{})
				DB = origDB
				LOG_DB = origLog
				resetBackendFlags()
				_ = sqlDB.Close()
			})

			const N = 20
			userIDs := make([]int, 0, N)
			for i := 0; i < N; i++ {
				u := &User{
					Username:    fmt.Sprintf("live-concurrent-%d", i),
					AccessToken: fmt.Sprintf("tok-live-%d", i),
					AffCode:     fmt.Sprintf("aff-live-%d", i),
					Quota:       0,
				}
				require.NoError(t, db.Create(u).Error)
				userIDs = append(userIDs, u.Id)
			}

			require.NoError(t, db.Create(&Redemption{
				Key:    "live-race-1",
				Status: RedemptionCodeStatusEnabled,
				Quota:  1000,
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
					if _, err := Redeem(context.Background(), "live-race-1", uid); err == nil {
						successes.Add(1)
					}
				}()
			}
			close(start)
			wg.Wait()

			require.EqualValues(t, 1, successes.Load(),
				"%s: exactly one goroutine must succeed; got %d", tc.name, successes.Load())

			var totalCredited int64
			require.NoError(t, db.Model(&User{}).
				Where("id IN ?", userIDs).
				Select("COALESCE(SUM(quota), 0)").
				Scan(&totalCredited).Error)
			require.EqualValues(t, 1000, totalCredited,
				"%s: exactly the redemption quota must be credited in total; got %d",
				tc.name, totalCredited)
		})
	}
}
