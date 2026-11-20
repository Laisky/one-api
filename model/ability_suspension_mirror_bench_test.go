package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// seedMirrorBenchAbilities fills the abilities table with rows distinct abilities
// (250 channels x 2 groups x rows/500 models), of which 20% carry expired
// deadlines in both columns, 5% are disabled, and exactly active enabled rows carry a
// deadline one hour ahead, alternating between the fixed and the adaptive column
// and spread evenly through the table (worst case for locality).
//
// Parameters:
//   - b: the benchmark handle.
//   - rows: the table size.
//   - active: how many enabled abilities are cooling.
func seedMirrorBenchAbilities(b *testing.B, rows, active int) {
	b.Helper()
	now := time.Now().UTC()
	expired, future := now.Add(-time.Hour), now.Add(time.Hour)
	stride := rows + 1
	if active > 0 {
		stride = rows / active
	}
	batch := make([]Ability, 0, 1000)
	seeded := 0
	for i := range rows {
		ability := Ability{Group: fmt.Sprintf("g%d", (i/250)%2), Model: fmt.Sprintf("bench-model-%05d", i/500), ChannelId: i%250 + 1, Enabled: true}
		switch {
		case active > 0 && i%stride == 0 && seeded < active:
			if seeded%2 == 0 {
				ability.SuspendUntil = &future
			} else {
				ability.AdaptiveSuspendUntil = &future
			}
			seeded++
		case i%20 == 1:
			ability.Enabled = false
		case i%5 == 2:
			ability.SuspendUntil, ability.AdaptiveSuspendUntil = &expired, &expired
		}
		batch = append(batch, ability)
		if len(batch) == cap(batch) {
			require.NoError(b, DB.Create(&batch).Error)
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		require.NoError(b, DB.Create(&batch).Error)
	}
	require.Equal(b, active, seeded)
}

// BenchmarkAbilitySuspensionMirror measures one mirror tick (the two-index
// UNION ALL read plus the map swap) on a 100k-row abilities table with 0, 50 and
// 1,000 active cooldowns, and the SELECT 1 round-trip floor of the same handle.
// Run: MYSQL_DSN=... PG_DSN=... go test ./model/ -run '^$' -bench BenchmarkAbilitySuspensionMirror
func BenchmarkAbilitySuspensionMirror(b *testing.B) {
	for _, backend := range []string{"sqlite", "mysql", "postgres"} {
		b.Run(backend+"/roundtrip", func(b *testing.B) {
			useAbilityBenchDB(b, backend)
			for b.Loop() {
				var one int
				if err := DB.Raw("SELECT 1").Scan(&one).Error; err != nil {
					b.Fatal(err)
				}
			}
		})
		for _, active := range []int{0, 50, 1000} {
			b.Run(fmt.Sprintf("%s/rows=100k/active=%d", backend, active), func(b *testing.B) {
				useAbilityBenchDB(b, backend)
				oldMemory := config.MemoryCacheEnabled
				config.MemoryCacheEnabled = true
				channelSyncLock.Lock()
				oldSuspensions := cachedAbilitySuspensions
				channelSyncLock.Unlock()
				b.Cleanup(func() {
					config.MemoryCacheEnabled = oldMemory
					channelSyncLock.Lock()
					cachedAbilitySuspensions = oldSuspensions
					channelSyncLock.Unlock()
				})
				seedMirrorBenchAbilities(b, 100_000, active)
				if backend == "postgres" {
					require.NoError(b, DB.Exec("VACUUM (ANALYZE) abilities").Error)
				} else if backend == "mysql" {
					require.NoError(b, DB.Exec("ANALYZE TABLE abilities").Error)
				}
				ctx := context.Background()
				require.NoError(b, refreshCachedAbilitySuspensionsContext(ctx))
				channelSyncLock.RLock()
				published := len(cachedAbilitySuspensions)
				channelSyncLock.RUnlock()
				require.Equal(b, active, published, "the mirror publishes exactly the active cooldowns")
				b.ReportAllocs()
				for b.Loop() {
					if err := refreshCachedAbilitySuspensionsContext(ctx); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
