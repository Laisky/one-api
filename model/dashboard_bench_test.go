package model

// Dashboard cost benchmark (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0.5).
//
// Two claims are measured here.
//
// (1) GetSiteWideQuotaStats is a SUM/COUNT over the whole users table with no
//     predicate an index can serve, and it ran on every root dashboard load.
//     The measurement is its cost as a function of user count, and what the TTL
//     cache removes.
//
// (2) The cache takes a process-global mutex and holds it across the query on a
//     miss. That is a legitimate concern: if the waiters did not re-check the
//     cache after acquiring the lock it would serialize every concurrent cold
//     viewer behind one full scan, which would be strictly worse than the
//     concurrent scans it replaced. The concurrent arm below settles it
//     empirically rather than by reading the code.

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/benchdb"
	"github.com/Laisky/one-api/common/config"
)

// dashboardUserCounts are the user-table sizes the scaling arm measures.
var dashboardUserCounts = []int{10_000, 100_000, 1_000_000}

// seedDashboardUsers fills the users table with the requested number of rows.
//
// Parameters:
//   - tb: the test or benchmark, used to fail fast.
//   - db: the handle owning the users table.
//   - count: how many user rows to create.
//
// Return values: none.
func seedDashboardUsers(tb testing.TB, db *gorm.DB, count int) {
	tb.Helper()

	const perStatement = 2000
	for start := 0; start < count; start += perStatement {
		end := min(start+perStatement, count)
		var sb strings.Builder
		sb.WriteString("INSERT INTO users (uuid, username, password, display_name, role, status, quota, used_quota, request_count, aff_code, access_token, created_at, updated_at) VALUES ")
		args := make([]any, 0, (end-start)*13)
		for i := start; i < end; i++ {
			if i > start {
				sb.WriteString(",")
			}
			sb.WriteString("(?,?,?,?,?,?,?,?,?,?,?,?,?)")
			status := UserStatusEnabled
			if i%17 == 0 {
				status = UserStatusDisabled
			}
			args = append(args,
				fmt.Sprintf("bench-uuid-%09d", i),
				fmt.Sprintf("bench-user-%09d", i),
				"x", fmt.Sprintf("Bench User %d", i),
				RoleCommonUser, status,
				int64(1_000_000+i), int64(i*7),
				i%1000,
				fmt.Sprintf("aff-%09d", i),
				fmt.Sprintf("bench-token-%09d", i),
				int64(1757030400000), int64(1757030400000),
			)
		}
		require.NoError(tb, db.Exec(sb.String(), args...).Error)
	}
}

// setupDashboardBench provisions a users table of the requested size.
//
// Parameters:
//   - b: the benchmark, used to fail fast and register cleanup.
//   - target: the engine to measure against.
//   - users: how many user rows to seed.
//
// Return values: none.
func setupDashboardBench(b *testing.B, target benchdb.Target, users int) {
	b.Helper()

	db, restore := benchdb.Open(b, target)
	require.NoError(b, db.AutoMigrate(&User{}))
	benchdb.Reset(b, db, "users")
	seedDashboardUsers(b, db, users)

	// Verify the fixture. Without this a partially seeded or externally mutated
	// table yields a fast, wrong number instead of a failure -- and the engine
	// targets are fixed databases with fixed table names, so two concurrent runs
	// against the same DSN would silently corrupt each other.
	var seeded int64
	require.NoError(b, db.Model(&User{}).Count(&seeded).Error)
	require.EqualValues(b, users, seeded, "the users fixture must be exactly the requested size")

	prevDB := DB
	DB = db
	b.Cleanup(func() {
		DB = prevDB
		restore()
	})
}

// BenchmarkSiteWideQuotaStats measures the uncached full-table aggregate that
// ran on every root dashboard load, as a function of user count.
//
// Parameters:
//   - b: the benchmark handle.
//
// Return values: none.
func BenchmarkSiteWideQuotaStats(b *testing.B) {
	for _, target := range benchdb.Targets() {
		for _, users := range dashboardUserCounts {
			b.Run(fmt.Sprintf("%s/uncached_%dk_users", target.Engine, users/1000), func(b *testing.B) {
				setupDashboardBench(b, target, users)

				b.ResetTimer()
				for range b.N {
					_, _, _, err := GetSiteWideQuotaStats()
					require.NoError(b, err)
				}
			})

		}
	}
}

// BenchmarkSiteWideQuotaStatsCachedHit measures the steady-state cache hit.
//
// It is a separate benchmark, not a per-user-count arm, for two reasons. The hit
// path reads one struct under a mutex and never touches the database, so a
// figure per table size would be reporting noise as if it were scaling. And at
// ~100 ns it needs a large -benchtime to rise above timer resolution, which is
// wasteful to pair with seeding a million rows.
//
// TestSiteWideQuotaCacheHitIsSizeIndependent pins the independence claim
// separately, so it is asserted rather than implied.
//
// Parameters:
//   - b: the benchmark handle.
//
// Return values: none.
func BenchmarkSiteWideQuotaStatsCachedHit(b *testing.B) {
	for _, target := range benchdb.Targets() {
		b.Run(string(target.Engine), func(b *testing.B) {
			setupDashboardBench(b, target, 10_000)

			prevTTL := config.DashboardCacheTTLSec
			config.DashboardCacheTTLSec = 3600
			b.Cleanup(func() {
				config.DashboardCacheTTLSec = prevTTL
				ResetSiteWideQuotaStatsCache()
			})
			ResetSiteWideQuotaStatsCache()

			// Warm the cache outside the timed region: this arm measures the
			// hit every dashboard load after the first pays.
			_, _, _, err := GetSiteWideQuotaStatsCached()
			require.NoError(b, err)

			b.ResetTimer()
			b.ReportAllocs()
			for range b.N {
				if _, _, _, err := GetSiteWideQuotaStatsCached(); err != nil {
					b.Fatalf("cached read failed: %+v", err)
				}
			}
		})
	}
}

// TestSiteWideQuotaCacheHitIsSizeIndependent verifies the cache hit path does
// not touch the database, so its cost cannot depend on how many users exist.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestSiteWideQuotaCacheHitIsSizeIndependent(t *testing.T) {
	prevTTL := config.DashboardCacheTTLSec
	config.DashboardCacheTTLSec = 3600
	t.Cleanup(func() {
		config.DashboardCacheTTLSec = prevTTL
		ResetSiteWideQuotaStatsCache()
	})

	setupTestDatabase(t)
	ResetSiteWideQuotaStatsCache()

	_, _, _, err := GetSiteWideQuotaStatsCached()
	require.NoError(t, err)

	// With the handle removed entirely, a hit must still succeed: that is only
	// possible if it issues no query, which is what makes the cost independent
	// of table size.
	prevDB := DB
	DB = nil
	t.Cleanup(func() { DB = prevDB })

	require.NotPanics(t, func() {
		if _, _, _, err := GetSiteWideQuotaStatsCached(); err != nil {
			t.Errorf("a cache hit must not require a database: %+v", err)
		}
	})
}

// BenchmarkSiteWideQuotaStatsColdMiss measures what concurrent viewers pay when
// the cache is cold, against the uncached path they replaced.
//
// The cache holds a process-global mutex across the underlying query. This arm
// exists to determine whether that serializes concurrent cold viewers (a
// regression) or collapses them into a single query whose result the waiters
// then find already cached (single-flight, an improvement). The two hypotheses
// predict opposite scaling in viewer count, so the measurement distinguishes
// them.
//
// Parameters:
//   - b: the benchmark handle.
//
// Return values: none.
func BenchmarkSiteWideQuotaStatsColdMiss(b *testing.B) {
	viewerCounts := []int{1, 8, 32}

	for _, target := range benchdb.Targets() {
		for _, viewers := range viewerCounts {
			for _, cached := range []bool{false, true} {
				label := "uncached"
				if cached {
					label = "cached"
				}
				b.Run(fmt.Sprintf("%s/%s_%d_concurrent_viewers", target.Engine, label, viewers), func(b *testing.B) {
					setupDashboardBench(b, target, 100_000)

					prevTTL := config.DashboardCacheTTLSec
					if cached {
						config.DashboardCacheTTLSec = 3600
					} else {
						config.DashboardCacheTTLSec = 0
					}
					b.Cleanup(func() {
						config.DashboardCacheTTLSec = prevTTL
						ResetSiteWideQuotaStatsCache()
					})

					b.ResetTimer()
					for range b.N {
						// Every iteration starts cold, so each measures the
						// worst case: a herd of viewers arriving at a
						// just-expired cache.
						b.StopTimer()
						ResetSiteWideQuotaStatsCache()
						b.StartTimer()

						var wg sync.WaitGroup
						start := time.Now()
						for range viewers {
							wg.Add(1)
							go func() {
								defer wg.Done()
								if _, _, _, err := GetSiteWideQuotaStatsCached(); err != nil {
									b.Errorf("cold read failed: %+v", err)
								}
							}()
						}
						wg.Wait()
						_ = time.Since(start)
					}
				})
			}
		}
	}
}
