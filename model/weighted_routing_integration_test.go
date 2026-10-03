package model

import (
	"sync"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
)

// errUnexpectedChannel flags a routing result outside the expected candidate set
// during the concurrent stress test.
var errUnexpectedChannel = errors.New("routing selected an unexpected channel")

// newRoutingTestDB opens an isolated in-memory SQLite database and points the
// package globals at it. MaxOpenConns(1) pins every query to the same connection so
// the shared-nothing ":memory:" DB is not silently recreated per pooled connection.
func newRoutingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	originalDB := DB
	DB = db
	t.Cleanup(func() { DB = originalDB })

	originalSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	t.Cleanup(func() { common.UsingSQLite.Store(originalSQLite) })

	return db
}

// setupWeightedRoutingDB builds an isolated SQLite DB with the given channels and
// their abilities, and points the package globals at it.
func setupWeightedRoutingDB(t *testing.T, channels []*Channel) {
	t.Helper()
	db := newRoutingTestDB(t)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	for _, ch := range channels {
		require.NoError(t, db.Create(ch).Error)
		require.NoError(t, ch.AddAbilities())
	}
}

// TestWeightedRouting_DBvsCacheParity_Distribution verifies that both routing
// paths — the DB path and the cache path, which now both read channels.weight —
// converge on the same weighted distribution for a 7/3 split, and never select
// outside the candidate set.
func TestWeightedRouting_DBvsCacheParity_Distribution(t *testing.T) {
	const group, model = "default", "gpt-3.5-turbo"
	channels := []*Channel{
		{Id: 1, Name: "c1", Status: ChannelStatusEnabled, Models: model, Group: group, Weight: uintPtr(7)},
		{Id: 2, Name: "c2", Status: ChannelStatusEnabled, Models: model, Group: group, Weight: uintPtr(3)},
	}
	setupWeightedRoutingDB(t, channels)

	const n = 20000

	// --- DB path (MemoryCacheEnabled = false) ---
	originalCache := config.MemoryCacheEnabled
	config.MemoryCacheEnabled = false
	t.Cleanup(func() { config.MemoryCacheEnabled = originalCache })

	dbCounts := map[int]int{}
	for range n {
		ch, err := GetRandomSatisfiedChannel(group, model, false)
		require.NoError(t, err)
		require.Contains(t, []int{1, 2}, ch.Id)
		dbCounts[ch.Id]++
	}
	dbRatio := float64(dbCounts[1]) / float64(n)
	require.InDelta(t, 0.70, dbRatio, 0.05, "DB path channel 1 ratio %.3f", dbRatio)

	// --- Cache path (MemoryCacheEnabled = true) ---
	config.MemoryCacheEnabled = true
	InitChannelCache()

	cacheCounts := map[int]int{}
	for range n {
		ch, err := CacheGetRandomSatisfiedChannel(group, model, false)
		require.NoError(t, err)
		require.Contains(t, []int{1, 2}, ch.Id)
		cacheCounts[ch.Id]++
	}
	cacheRatio := float64(cacheCounts[1]) / float64(n)
	require.InDelta(t, 0.70, cacheRatio, 0.05, "cache path channel 1 ratio %.3f", cacheRatio)

	// Parity: the two independent paths must agree within statistical noise.
	require.InDelta(t, dbRatio, cacheRatio, 0.05, "DB and cache weighted ratios must match")
}

// TestWeightedRouting_ZeroWeightExcludedInMixedTier verifies the documented
// contract: within a candidate tier that has at least one positive weight, a
// zero-weight channel receives no traffic — on both routing paths.
func TestWeightedRouting_ZeroWeightExcludedInMixedTier(t *testing.T) {
	const group, model = "default", "gpt-3.5-turbo"
	channels := []*Channel{
		{Id: 1, Name: "c1", Status: ChannelStatusEnabled, Models: model, Group: group, Weight: uintPtr(0)},
		{Id: 2, Name: "c2", Status: ChannelStatusEnabled, Models: model, Group: group, Weight: uintPtr(5)},
	}
	setupWeightedRoutingDB(t, channels)

	originalCache := config.MemoryCacheEnabled
	t.Cleanup(func() { config.MemoryCacheEnabled = originalCache })

	// DB path.
	config.MemoryCacheEnabled = false
	for range 500 {
		ch, err := GetRandomSatisfiedChannel(group, model, false)
		require.NoError(t, err)
		require.Equal(t, 2, ch.Id, "DB path must never pick the zero-weight channel in a mixed tier")
	}

	// Cache path.
	config.MemoryCacheEnabled = true
	InitChannelCache()
	for range 500 {
		ch, err := CacheGetRandomSatisfiedChannel(group, model, false)
		require.NoError(t, err)
		require.Equal(t, 2, ch.Id, "cache path must never pick the zero-weight channel in a mixed tier")
	}
}

// TestWeightedRouting_AllZeroFallsBackToUniform verifies that when every candidate
// has zero weight, both paths still return a valid channel (uniform fallback) and
// can reach every candidate.
func TestWeightedRouting_AllZeroFallsBackToUniform(t *testing.T) {
	const group, model = "default", "gpt-3.5-turbo"
	channels := []*Channel{
		{Id: 1, Name: "c1", Status: ChannelStatusEnabled, Models: model, Group: group, Weight: uintPtr(0)},
		{Id: 2, Name: "c2", Status: ChannelStatusEnabled, Models: model, Group: group, Weight: uintPtr(0)},
		{Id: 3, Name: "c3", Status: ChannelStatusEnabled, Models: model, Group: group, Weight: uintPtr(0)},
	}
	setupWeightedRoutingDB(t, channels)

	originalCache := config.MemoryCacheEnabled
	t.Cleanup(func() { config.MemoryCacheEnabled = originalCache })

	config.MemoryCacheEnabled = false
	dbSeen := map[int]bool{}
	for range 1000 {
		ch, err := GetRandomSatisfiedChannel(group, model, false)
		require.NoError(t, err)
		dbSeen[ch.Id] = true
	}
	require.Len(t, dbSeen, 3, "DB uniform fallback must reach every candidate")

	config.MemoryCacheEnabled = true
	InitChannelCache()
	cacheSeen := map[int]bool{}
	for range 1000 {
		ch, err := CacheGetRandomSatisfiedChannel(group, model, false)
		require.NoError(t, err)
		cacheSeen[ch.Id] = true
	}
	require.Len(t, cacheSeen, 3, "cache uniform fallback must reach every candidate")
}

// TestBackwardCompat_OldWeightColumn proves the release auto-adapts to existing
// deployments that already carry an abilities.weight column from an earlier build.
// AutoMigrate never drops columns, so the column lingers inert. Across every
// physical column shape a prior build could have produced, and against BOTH routing
// paths, new inserts must still succeed and routing must weight solely by
// channels.weight — including when the orphaned column holds STALE (drifted) values.
func TestBackwardCompat_OldWeightColumn(t *testing.T) {
	const group, model = "default", "gpt-3.5-turbo"

	// createAbilities is the DDL for the pre-existing abilities table shape.
	cases := []struct {
		name        string
		createDDL   string
		seedStale   bool // pre-insert a drifted weight row that AddAbilities will overwrite
		description string
	}{
		{
			name:        "nullable_no_default",
			createDDL:   "CREATE TABLE abilities (`group` TEXT, model TEXT, channel_id INTEGER, enabled INTEGER, priority INTEGER, weight INTEGER, suspend_until DATETIME, created_at INTEGER, updated_at INTEGER, PRIMARY KEY (`group`, model, channel_id))",
			description: "column added nullable (the Part-0 shape)",
		},
		{
			name:        "nullable_default_0",
			createDDL:   "CREATE TABLE abilities (`group` TEXT, model TEXT, channel_id INTEGER, enabled INTEGER, priority INTEGER, weight INTEGER DEFAULT 0, suspend_until DATETIME, created_at INTEGER, updated_at INTEGER, PRIMARY KEY (`group`, model, channel_id))",
			description: "gorm default:0 shape",
		},
		{
			name:        "not_null_default_0",
			createDDL:   "CREATE TABLE abilities (`group` TEXT, model TEXT, channel_id INTEGER, enabled INTEGER, priority INTEGER, weight INTEGER NOT NULL DEFAULT 0, suspend_until DATETIME, created_at INTEGER, updated_at INTEGER, PRIMARY KEY (`group`, model, channel_id))",
			description: "NOT NULL with a default: INSERT omitting weight must still satisfy the constraint",
		},
	}

	for _, tc := range cases {
		for _, useCache := range []bool{false, true} {
			pathName := "db"
			if useCache {
				pathName = "cache"
			}
			t.Run(tc.name+"/"+pathName, func(t *testing.T) {
				db := newRoutingTestDB(t)
				require.NoError(t, db.Exec(tc.createDDL).Error, tc.description)
				require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}),
					"AutoMigrate must not drop or require the orphaned weight column")

				// Channels carry the authoritative 7/3 weights.
				c1 := &Channel{Id: 1, Name: "c1", Status: ChannelStatusEnabled, Models: model, Group: group, Weight: uintPtr(7)}
				c2 := &Channel{Id: 2, Name: "c2", Status: ChannelStatusEnabled, Models: model, Group: group, Weight: uintPtr(3)}
				require.NoError(t, db.Create(c1).Error)
				require.NoError(t, db.Create(c2).Error)

				// AddAbilities inserts rows WITHOUT a weight field; the orphaned column
				// (nullable, defaulted, or NOT NULL-with-default) must accept the INSERT.
				require.NoError(t, c1.AddAbilities())
				require.NoError(t, c2.AddAbilities())

				// Simulate DRIFTED old data: force the orphaned column to values that do
				// NOT match channels.weight, to prove routing ignores it entirely.
				require.NoError(t, db.Exec("UPDATE abilities SET weight = 999 WHERE channel_id = 1").Error)
				require.NoError(t, db.Exec("UPDATE abilities SET weight = 0 WHERE channel_id = 2").Error)

				originalCache := config.MemoryCacheEnabled
				config.MemoryCacheEnabled = useCache
				t.Cleanup(func() { config.MemoryCacheEnabled = originalCache })

				pick := func() (*Channel, error) { return GetRandomSatisfiedChannel(group, model, false) }
				if useCache {
					InitChannelCache() // rebuilds the in-memory cache; must tolerate SELECT * over the orphaned column
					pick = func() (*Channel, error) { return CacheGetRandomSatisfiedChannel(group, model, false) }
				}

				counts := map[int]int{}
				const n = 4000
				for range n {
					ch, err := pick()
					require.NoError(t, err)
					require.Contains(t, []int{1, 2}, ch.Id)
					counts[ch.Id]++
				}
				ratio1 := float64(counts[1]) / float64(n)
				// If the stale orphaned column (999/0) were used, ratio1 would be ~1.0;
				// channels.weight (7/3) yields ~0.70.
				require.InDelta(t, 0.70, ratio1, 0.06,
					"routing must weight by channels.weight (7/3), NOT the stale orphaned column; got %.3f", ratio1)
			})
		}
	}
}

// TestRouting_IgnoreFirstPriority_DBvsCacheParity guards the reconciliation of the
// non-excluding ignoreFirstPriority contract across the DB and cache paths:
//   - false            -> highest tier only
//   - true, multi-tier -> lower tiers only (the highest tier is skipped)
//   - true, single tier-> lenient fall back to the (only) tier, so it still routes
//
// Before the fix the DB path selected ALL tiers on true (including the highest),
// diverging from the cache path which already skipped it.
func TestRouting_IgnoreFirstPriority_DBvsCacheParity(t *testing.T) {
	const group, model = "default", "gpt-3.5-turbo"
	p100, p50, p10 := int64(100), int64(50), int64(10)

	originalCache := config.MemoryCacheEnabled
	t.Cleanup(func() { config.MemoryCacheEnabled = originalCache })

	// Multi-tier: 100 / 50 / 10.
	t.Run("multi-tier skips highest on both paths", func(t *testing.T) {
		channels := []*Channel{
			{Id: 1, Name: "p100", Status: ChannelStatusEnabled, Models: model, Group: group, Priority: &p100},
			{Id: 2, Name: "p50", Status: ChannelStatusEnabled, Models: model, Group: group, Priority: &p50},
			{Id: 3, Name: "p10", Status: ChannelStatusEnabled, Models: model, Group: group, Priority: &p10},
		}
		setupWeightedRoutingDB(t, channels)

		assertLowerTiers := func(t *testing.T, pick func() (*Channel, error)) {
			seen := map[int]bool{}
			for range 2000 {
				ch, err := pick()
				require.NoError(t, err)
				require.NotEqual(t, 1, ch.Id, "ignoreFirstPriority=true must never select the highest tier")
				seen[ch.Id] = true
			}
			require.True(t, seen[2] && seen[3], "all strictly-lower tiers must be eligible")
		}

		config.MemoryCacheEnabled = false
		assertLowerTiers(t, func() (*Channel, error) { return GetRandomSatisfiedChannel(group, model, true) })

		config.MemoryCacheEnabled = true
		InitChannelCache()
		assertLowerTiers(t, func() (*Channel, error) { return CacheGetRandomSatisfiedChannel(group, model, true) })
	})

	// Single tier: ignoreFirstPriority=true must still route (lenient fallback).
	t.Run("single tier routes leniently on both paths", func(t *testing.T) {
		p0 := int64(0)
		channels := []*Channel{
			{Id: 1, Name: "only", Status: ChannelStatusEnabled, Models: model, Group: group, Priority: &p0},
		}
		setupWeightedRoutingDB(t, channels)

		config.MemoryCacheEnabled = false
		ch, err := GetRandomSatisfiedChannel(group, model, true)
		require.NoError(t, err, "DB: single-tier ignoreFirstPriority=true must fall back and route")
		require.Equal(t, 1, ch.Id)

		config.MemoryCacheEnabled = true
		InitChannelCache()
		ch, err = CacheGetRandomSatisfiedChannel(group, model, true)
		require.NoError(t, err, "cache: single-tier ignoreFirstPriority=true must fall back and route")
		require.Equal(t, 1, ch.Id)
	})
}

// TestWeightedRouting_OnlyTopPriorityTierIsWeighted verifies that weighting is
// confined to the top priority tier: a lower-priority channel must never receive
// traffic on the primary (ignoreFirstPriority=false) path, on both DB and cache.
func TestWeightedRouting_OnlyTopPriorityTierIsWeighted(t *testing.T) {
	const group, model = "default", "gpt-3.5-turbo"
	p100, p50 := int64(100), int64(50)
	channels := []*Channel{
		{Id: 1, Name: "top-a", Status: ChannelStatusEnabled, Models: model, Group: group, Priority: &p100, Weight: uintPtr(7)},
		{Id: 2, Name: "top-b", Status: ChannelStatusEnabled, Models: model, Group: group, Priority: &p100, Weight: uintPtr(3)},
		{Id: 3, Name: "low-c", Status: ChannelStatusEnabled, Models: model, Group: group, Priority: &p50, Weight: uintPtr(9)},
	}
	setupWeightedRoutingDB(t, channels)

	originalCache := config.MemoryCacheEnabled
	t.Cleanup(func() { config.MemoryCacheEnabled = originalCache })

	assertTopTierOnly := func(t *testing.T, pick func() (*Channel, error)) {
		seen := map[int]bool{}
		for range 2000 {
			ch, err := pick()
			require.NoError(t, err)
			require.NotEqual(t, 3, ch.Id, "lower-priority channel must never be selected on the primary path")
			seen[ch.Id] = true
		}
		require.True(t, seen[1] && seen[2], "both top-tier channels must be reachable (weighting active)")
	}

	config.MemoryCacheEnabled = false
	assertTopTierOnly(t, func() (*Channel, error) { return GetRandomSatisfiedChannel(group, model, false) })

	config.MemoryCacheEnabled = true
	InitChannelCache()
	assertTopTierOnly(t, func() (*Channel, error) { return CacheGetRandomSatisfiedChannel(group, model, false) })
}

// TestWeightedRouting_ExclusionPathIsWeighted covers the retry/exclusion path on
// both DB and cache: weighted selection must still apply after channels are
// excluded, a zero-weight channel must stay starved in a mixed tier, and an
// excluded channel must never be returned.
func TestWeightedRouting_ExclusionPathIsWeighted(t *testing.T) {
	const group, model = "default", "gpt-3.5-turbo"
	channels := []*Channel{
		{Id: 1, Name: "c1", Status: ChannelStatusEnabled, Models: model, Group: group, Weight: uintPtr(7)},
		{Id: 2, Name: "c2", Status: ChannelStatusEnabled, Models: model, Group: group, Weight: uintPtr(3)},
		{Id: 3, Name: "c3", Status: ChannelStatusEnabled, Models: model, Group: group, Weight: uintPtr(0)},
	}
	setupWeightedRoutingDB(t, channels)

	originalCache := config.MemoryCacheEnabled
	t.Cleanup(func() { config.MemoryCacheEnabled = originalCache })

	// pick is either the DB or cache exclusion entrypoint.
	type excludeFn func(exclude map[int]bool) (*Channel, error)
	dbExclude := excludeFn(func(exclude map[int]bool) (*Channel, error) {
		return GetRandomSatisfiedChannelExcluding(group, model, false, exclude)
	})
	cacheExclude := excludeFn(func(exclude map[int]bool) (*Channel, error) {
		return CacheGetRandomSatisfiedChannelExcluding(group, model, false, exclude, false)
	})

	run := func(t *testing.T, pick excludeFn) {
		// No exclusions: weighted among positive weights; zero-weight c3 starved.
		seen := map[int]bool{}
		for range 2000 {
			ch, err := pick(nil)
			require.NoError(t, err)
			require.NotEqual(t, 3, ch.Id, "zero-weight channel must not receive traffic in a mixed tier")
			seen[ch.Id] = true
		}
		require.True(t, seen[1] && seen[2], "both positively-weighted channels must be reachable")

		// Exclude c1: only c2 (w=3) and c3 (w=0) remain; c2 must always win, c1 never returned.
		for range 1000 {
			ch, err := pick(map[int]bool{1: true})
			require.NoError(t, err)
			require.Equal(t, 2, ch.Id, "excluded channel must never be returned; zero-weight sibling stays starved")
		}
	}

	config.MemoryCacheEnabled = false
	run(t, dbExclude)

	config.MemoryCacheEnabled = true
	InitChannelCache()
	run(t, cacheExclude)
}

// TestWeightedRouting_ConcurrentSelection runs many goroutines through the shared
// weighted picker concurrently. Combined with `go test -race`, it guards against
// data races and panics in the routing hot path. The picker itself is a pure
// function over local slices and only touches math/rand's concurrency-safe
// top-level API, so no selection should ever fail.
func TestWeightedRouting_ConcurrentSelection(t *testing.T) {
	const group, model = "default", "gpt-3.5-turbo"
	channels := []*Channel{
		{Id: 1, Name: "c1", Status: ChannelStatusEnabled, Models: model, Group: group, Weight: uintPtr(7)},
		{Id: 2, Name: "c2", Status: ChannelStatusEnabled, Models: model, Group: group, Weight: uintPtr(3)},
	}
	setupWeightedRoutingDB(t, channels)

	originalCache := config.MemoryCacheEnabled
	config.MemoryCacheEnabled = true
	t.Cleanup(func() { config.MemoryCacheEnabled = originalCache })
	InitChannelCache()

	const goroutines, perGoroutine = 32, 200
	var wg sync.WaitGroup
	errs := make(chan error, goroutines*perGoroutine)
	for g := range goroutines {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for range perGoroutine {
				var (
					ch  *Channel
					err error
				)
				// Exercise both the pure pickers and the full cache path.
				if g%2 == 0 {
					ch, err = CacheGetRandomSatisfiedChannel(group, model, false)
				} else {
					local := make([]*Channel, len(channels))
					copy(local, channels)
					ch = pickWeightedChannel(local)
				}
				if err != nil {
					errs <- err
					return
				}
				if ch == nil || (ch.Id != 1 && ch.Id != 2) {
					errs <- errUnexpectedChannel
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}
