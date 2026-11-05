package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

// abilityBenchShapes are the channel sizes the projection benchmarks measure: a typical
// two-model channel, a 50-model channel in two groups, and a 300-model aggregator
// channel in three groups.
var abilityBenchShapes = []struct {
	name   string
	models int
	groups int
}{
	{name: "2x1", models: 2, groups: 1},
	{name: "50x2", models: 50, groups: 2},
	{name: "300x3", models: 300, groups: 3},
}

// useAbilityBenchDB installs a migrated benchmark database for backend as the package
// database and restores the globals afterwards.
// Parameters: b is the benchmark handle and backend is "sqlite", "mysql", or "postgres".
// Returns: no values; the benchmark is skipped when a server DSN is not set.
func useAbilityBenchDB(b *testing.B, backend string) {
	b.Helper()
	originalDB := DB
	originalSQLite, originalMySQL, originalPostgreSQL := common.UsingSQLite.Load(), common.UsingMySQL.Load(), common.UsingPostgreSQL.Load()
	b.Cleanup(func() {
		DB = originalDB
		common.UsingSQLite.Store(originalSQLite)
		common.UsingMySQL.Store(originalMySQL)
		common.UsingPostgreSQL.Store(originalPostgreSQL)
	})
	DB = openProbeBenchDB(b, backend)
	common.UsingSQLite.Store(backend == "sqlite")
	common.UsingMySQL.Store(backend == "mysql")
	common.UsingPostgreSQL.Store(backend == "postgres")
}

// insertAbilityBenchChannel creates a channel with the given number of models and groups.
// Parameters: b is the benchmark handle, label makes names unique, and models/groups size it.
// Returns: the inserted channel with its abilities.
func insertAbilityBenchChannel(b *testing.B, label string, models, groups int) *Channel {
	b.Helper()
	modelNames := make([]string, models)
	for i := range modelNames {
		modelNames[i] = fmt.Sprintf("bench-model-%03d", i)
	}
	groupNames := make([]string, groups)
	for i := range groupNames {
		groupNames[i] = fmt.Sprintf("bg-%s-%d", label, i)
	}
	channel := &Channel{Name: "ability-bench-" + label, Key: "sk-bench", Status: ChannelStatusEnabled,
		Models: strings.Join(modelNames, ","), Group: strings.Join(groupNames, ",")}
	require.NoError(b, channel.Insert())
	return channel
}

// BenchmarkChannelAbilityProjection isolates the abilities write of one channel save that
// leaves models and groups unchanged (a testing model, name, or pricing save), for the
// destructive rebuild every save used to run against the in-place sync that replaced it.
// Run: MYSQL_DSN=... PG_DSN=... go test ./model/ -run '^$' -bench BenchmarkChannelAbilityProjection
// Parameters: b is the benchmark handle. Returns: no values.
func BenchmarkChannelAbilityProjection(b *testing.B) {
	for _, backend := range []string{"sqlite", "mysql", "postgres"} {
		b.Run(backend, func(b *testing.B) {
			useAbilityBenchDB(b, backend)
			for _, shape := range abilityBenchShapes {
				channel := insertAbilityBenchChannel(b, shape.name, shape.models, shape.groups)
				b.Run(shape.name+"/rebuild", func(b *testing.B) {
					for b.Loop() {
						require.NoError(b, DB.Transaction(func(tx *gorm.DB) error {
							if err := deleteAbilitiesWithDB(tx, channel.Id); err != nil {
								return err
							}
							return addAbilitiesWithDB(tx, channel)
						}))
					}
				})
				b.Run(shape.name+"/sync", func(b *testing.B) {
					for b.Loop() {
						require.NoError(b, DB.Transaction(func(tx *gorm.DB) error {
							return syncAbilitiesWithDB(tx, channel, false)
						}))
					}
				})
			}
		})
	}
}

// BenchmarkChannelTestingModelSave measures the whole PUT path of a testing model save,
// alternating SKIP and auto-select, including the row lock, testing_model resolution,
// abilities write, and cache invalidation. It uses only APIs that predate the abilities
// sync, so the same file runs on the baseline tree for a before/after comparison.
// Run: MYSQL_DSN=... PG_DSN=... go test ./model/ -run '^$' -bench BenchmarkChannelTestingModelSave
// Parameters: b is the benchmark handle. Returns: no values.
func BenchmarkChannelTestingModelSave(b *testing.B) {
	for _, backend := range []string{"sqlite", "mysql", "postgres"} {
		b.Run(backend, func(b *testing.B) {
			useAbilityBenchDB(b, backend)
			ctx := context.Background()
			for _, shape := range abilityBenchShapes {
				channel := insertAbilityBenchChannel(b, shape.name, shape.models, shape.groups)
				b.Run(shape.name, func(b *testing.B) {
					skip, saveSkip := ChannelTestingModelSkip, true
					for b.Loop() {
						update := &Channel{Id: channel.Id,
							NullableFieldsProvided: map[string]bool{ChannelTestingModelField: true}}
						if saveSkip {
							update.TestingModel = &skip
						}
						saveSkip = !saveSkip
						require.NoError(b, update.UpdateWithContext(ctx))
					}
				})
			}
		})
	}
}
