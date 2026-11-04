package model

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

// testingModelConcurrentWriterGrace bounds how long the concurrency scenario waits for
// a competing writer that should be blocked by the update's row lock.
const testingModelConcurrentWriterGrace = 750 * time.Millisecond

// useTestingModelMatrixBackend installs the requested dialect as the package database
// for one subtest and restores every global it swaps.
// Parameters: t is the subtest handle and dialect is "sqlite", "mysql", or "postgres".
// Returns: the installed database; MySQL/PostgreSQL skip without a DSN unless
// ONEAPI_REQUIRE_DB_BACKENDS=1, in which case the missing backend fails the test.
func useTestingModelMatrixBackend(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	// Capture globals before openBackend mutates the dialect flags.
	originalDB := DB
	originalSQLite := common.UsingSQLite.Load()
	originalMySQL := common.UsingMySQL.Load()
	originalPostgreSQL := common.UsingPostgreSQL.Load()
	t.Cleanup(func() {
		DB = originalDB
		common.UsingSQLite.Store(originalSQLite)
		common.UsingMySQL.Store(originalMySQL)
		common.UsingPostgreSQL.Store(originalPostgreSQL)
	})

	db := openChannelNullableBackend(t, dialect)
	if db == nil {
		if uuidBackendsAreMandatory() {
			t.Fatalf("%s=1 but no DSN is configured for %s", requireDBBackendsEnv, dialect)
		}
		t.Skipf("no DSN configured for %s; skipping testing_model matrix", dialect)
	}
	DB = db
	common.UsingSQLite.Store(dialect == "sqlite")
	common.UsingMySQL.Store(dialect == "mysql")
	common.UsingPostgreSQL.Store(dialect == "postgres")
	return db
}

// insertTestingModelMatrixChannel creates a channel serving gpt-4o-mini and gpt-4o with
// the given testing model.
// Parameters: t is the test handle, dialect and label make the name and group unique,
// and testingModel is the initial selection (nil for auto-select).
// Returns: the inserted channel.
func insertTestingModelMatrixChannel(t *testing.T, dialect, label string, testingModel *string) *Channel {
	t.Helper()
	suffix := time.Now().UnixNano() % 1_000_000
	channel := &Channel{
		Name:         fmt.Sprintf("tm-%s-%s-%d", dialect, label, suffix),
		Type:         1,
		Status:       ChannelStatusEnabled,
		Models:       "gpt-4o-mini,gpt-4o",
		Group:        fmt.Sprintf("tm-%s-%d", shortDialect(dialect), suffix),
		TestingModel: testingModel,
	}
	require.NoError(t, channel.Insert())
	return channel
}

// rawTestingModel reads the testing_model column with a raw query so SQL NULL is
// distinguishable from a stored string on every dialect.
// Parameters: t is the test handle, db is the database, and id identifies the channel.
// Returns: the raw column value.
func rawTestingModel(t *testing.T, db *gorm.DB, id int) sql.NullString {
	t.Helper()
	return readNullStringColumn(t, db, "channels", "testing_model", id)
}

// interleaveBeforeChannelStructUpdate registers a one-shot callback that runs action on
// the update's own connection immediately before the struct UPDATE of channel id. It
// simulates a competing writer whose change committed after the update began reading
// state but before it wrote the row.
// Parameters: t is the test handle, db is the database, id is the target channel, and
// action receives a session bound to the in-flight transaction.
// Returns: no values; the callback is removed when the test ends.
func interleaveBeforeChannelStructUpdate(t *testing.T, db *gorm.DB, id int, action func(tx *gorm.DB)) {
	t.Helper()
	name := "test:testing_model_interleave"
	var fired atomic.Bool
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		target, ok := tx.Statement.Dest.(*Channel)
		if !ok || target.Id != id || !fired.CompareAndSwap(false, true) {
			return
		}
		action(tx.Session(&gorm.Session{NewDB: true}))
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove(name)) })
}

// TestChannelTestingModelPersistenceMultiDB pins the testing_model persistence contract
// on SQLite, MySQL (MYSQL_DSN), and PostgreSQL (PG_DSN): SKIP is stored as a value, an
// explicit null is stored as SQL NULL, and an update that omits the field never
// re-asserts a selection that a concurrent writer replaced (issue #609 follow-up).
// Parameters: t is the test handle. Returns: no values.
func TestChannelTestingModelPersistenceMultiDB(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := useTestingModelMatrixBackend(t, dialect)
			skip := ChannelTestingModelSkip
			ctx := context.Background()

			t.Run("SKIP is stored as a value", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "store", nil)
				update := &Channel{Id: channel.Id, Name: channel.Name, TestingModel: &skip}
				require.NoError(t, update.UpdateWithContext(ctx))
				require.Equal(t, sql.NullString{String: skip, Valid: true}, rawTestingModel(t, db, channel.Id))
			})

			t.Run("explicit null is stored as SQL NULL", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "null", &skip)
				update := &Channel{Id: channel.Id, Name: channel.Name,
					NullableFieldsProvided: map[string]bool{ChannelTestingModelField: true}}
				require.NoError(t, update.UpdateWithContext(ctx))
				require.False(t, rawTestingModel(t, db, channel.Id).Valid)
			})

			t.Run("omitted field keeps SKIP across a model-list change", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "keep", &skip)
				update := &Channel{Id: channel.Id, Name: channel.Name, Models: "claude-sonnet-4-5"}
				require.NoError(t, update.UpdateWithContext(ctx))
				require.Equal(t, sql.NullString{String: skip, Valid: true}, rawTestingModel(t, db, channel.Id))
			})

			t.Run("omitted field never re-asserts a replaced selection", func(t *testing.T) {
				named := "gpt-4o"
				channel := insertTestingModelMatrixChannel(t, dialect, "stale", &named)
				interleaveBeforeChannelStructUpdate(t, db, channel.Id, func(tx *gorm.DB) {
					require.NoError(t, tx.Exec("UPDATE channels SET testing_model = ? WHERE id = ?", skip, channel.Id).Error)
				})
				priority := int64(7)
				update := &Channel{Id: channel.Id, Name: channel.Name, Priority: &priority}
				require.NoError(t, update.UpdateWithContext(ctx))
				require.Equal(t, sql.NullString{String: skip, Valid: true}, rawTestingModel(t, db, channel.Id),
					"a priority-only update must not overwrite the SKIP saved concurrently")
			})

			t.Run("model-list change re-validates the latest selection", func(t *testing.T) {
				named := "gpt-4o"
				channel := insertTestingModelMatrixChannel(t, dialect, "revalidate", &named)
				interleaveBeforeChannelStructUpdate(t, db, channel.Id, func(tx *gorm.DB) {
					require.NoError(t, tx.Exec("UPDATE channels SET testing_model = ? WHERE id = ?", skip, channel.Id).Error)
				})
				update := &Channel{Id: channel.Id, Name: channel.Name, Models: "gpt-4o-mini"}
				require.NoError(t, update.UpdateWithContext(ctx))
				require.Equal(t, sql.NullString{String: skip, Valid: true}, rawTestingModel(t, db, channel.Id),
					"dropping gpt-4o must not clear the SKIP saved concurrently")
			})

			if dialect == "sqlite" {
				// SQLite serialises writers database-wide and this harness uses an
				// in-memory database per connection, so the row-lock scenario below
				// only applies to the server backends.
				return
			}
			t.Run("a concurrent writer waits for the update's row lock", func(t *testing.T) {
				named := "gpt-4o"
				channel := insertTestingModelMatrixChannel(t, dialect, "lock", &named)
				writerDone := make(chan error, 1)
				var writerFinishedFirst atomic.Bool
				interleaveBeforeChannelStructUpdate(t, db, channel.Id, func(*gorm.DB) {
					go func() {
						competing := &Channel{Id: channel.Id, Name: channel.Name, TestingModel: &skip}
						writerDone <- competing.UpdateWithContext(context.Background())
					}()
					select {
					case err := <-writerDone:
						writerFinishedFirst.Store(true)
						writerDone <- err
					case <-time.After(testingModelConcurrentWriterGrace):
					}
				})
				priority := int64(9)
				update := &Channel{Id: channel.Id, Name: channel.Name, Priority: &priority}
				require.NoError(t, update.UpdateWithContext(ctx))

				select {
				case err := <-writerDone:
					require.NoError(t, err)
				case <-time.After(30 * time.Second):
					require.FailNow(t, "competing testing_model writer never finished")
				}
				require.False(t, writerFinishedFirst.Load(),
					"the competing writer must wait for the in-flight update's row lock")
				require.Equal(t, sql.NullString{String: skip, Valid: true}, rawTestingModel(t, db, channel.Id),
					"the later SKIP write must win over the priority-only update")
			})
		})
	}
}

// probeResultMarker marks the context of the probe-result transaction a test intercepts.
type probeResultMarker struct{}

// abilitiesEnabled reports the routing flag of every ability row of a channel.
// Parameters: t is the test handle, db is the database, and id identifies the channel.
// Returns: the enabled flags, one per ability row.
func abilitiesEnabled(t *testing.T, db *gorm.DB, id int) []bool {
	t.Helper()
	var flags []bool
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", id).Pluck("enabled", &flags).Error)
	return flags
}

// TestApplyChannelProbeResultMultiDB pins the atomic, fail-closed application of a
// health-check result on SQLite, MySQL, and PostgreSQL: SKIP discards the whole result,
// the status transition is a compare-and-swap on the probed status, abilities follow the
// status, and a competing SKIP write cannot slip in between the check and the change.
// Parameters: t is the test handle. Returns: no values.
func TestApplyChannelProbeResultMultiDB(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := useTestingModelMatrixBackend(t, dialect)
			skip := ChannelTestingModelSkip
			ctx := context.Background()
			// disable builds the result a failed probe of the channel's current
			// configuration asks for.
			disable := func(channel *Channel) ChannelProbeResult {
				probed := &Channel{Id: channel.Id}
				require.NoError(t, probed.ReloadForProbeWithContext(ctx))
				return ChannelProbeResult{Revision: probed.ProbeConfigRevision(), ObservedStatus: ChannelStatusEnabled,
					TargetStatus: ChannelStatusAutoDisabled, ResponseTimeMs: 50}
			}
			stored := func(id int) Channel {
				var channel Channel
				require.NoError(t, db.Select("id", "status", "response_time", "test_time", ChannelTestingModelField).
					First(&channel, "id = ?", id).Error)
				return channel
			}

			t.Run("SKIP discards the whole result", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "probe-skip", &skip)
				outcome, err := ApplyChannelProbeResultWithContext(ctx, channel.Id, disable(channel))
				require.NoError(t, err)
				require.True(t, outcome.OptedOut)
				require.False(t, outcome.StatusChanged)
				row := stored(channel.Id)
				require.Equal(t, ChannelStatusEnabled, row.Status)
				require.Zero(t, row.TestTime)
				require.NotContains(t, abilitiesEnabled(t, db, channel.Id), false)
			})

			t.Run("transition applies when the status is unchanged", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "probe-apply", nil)
				outcome, err := ApplyChannelProbeResultWithContext(ctx, channel.Id, disable(channel))
				require.NoError(t, err)
				require.True(t, outcome.StatusChanged)
				require.Equal(t, ChannelStatusAutoDisabled, outcome.Channel.Status)
				row := stored(channel.Id)
				require.Equal(t, ChannelStatusAutoDisabled, row.Status)
				require.Equal(t, 50, row.ResponseTime)
				require.NotZero(t, row.TestTime)
				flags := abilitiesEnabled(t, db, channel.Id)
				require.NotEmpty(t, flags)
				require.NotContains(t, flags, true, "abilities must stop routing to the disabled channel")
			})

			t.Run("a status changed since the probe is kept", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "probe-cas", nil)
				require.NoError(t, SetChannelStatusWithContext(ctx, channel.Id, ChannelStatusManuallyDisabled))
				outcome, err := ApplyChannelProbeResultWithContext(ctx, channel.Id, disable(channel))
				require.NoError(t, err)
				require.False(t, outcome.StatusChanged)
				row := stored(channel.Id)
				require.Equal(t, ChannelStatusManuallyDisabled, row.Status)
				require.NotZero(t, row.TestTime, "the measurement is still recorded")
			})

			t.Run("a transition that already happened is not repeated", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "probe-noop", nil)
				require.NoError(t, SetChannelStatusWithContext(ctx, channel.Id, ChannelStatusAutoDisabled))
				outcome, err := ApplyChannelProbeResultWithContext(ctx, channel.Id, disable(channel))
				require.NoError(t, err)
				require.False(t, outcome.StatusChanged, "no second transition, so no second announcement")
			})

			t.Run("a configuration changed during the probe discards the whole result", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "probe-config", nil)
				result := disable(channel)
				require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("models", "gpt-4o").Error)
				outcome, err := ApplyChannelProbeResultWithContext(ctx, channel.Id, result)
				require.NoError(t, err)
				require.True(t, outcome.ConfigChanged)
				require.False(t, outcome.StatusChanged)
				row := stored(channel.Id)
				require.Equal(t, ChannelStatusEnabled, row.Status)
				require.Zero(t, row.TestTime, "a discarded result records nothing")
			})

			t.Run("a missing revision is rejected", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "probe-norev", nil)
				_, err := ApplyChannelProbeResultWithContext(ctx, channel.Id, ChannelProbeResult{TargetStatus: ChannelStatusAutoDisabled})
				require.Error(t, err)
				require.Equal(t, ChannelStatusEnabled, stored(channel.Id).Status)
			})

			t.Run("a missing channel fails without effects", func(t *testing.T) {
				_, err := ApplyChannelProbeResultWithContext(ctx, 987654321, ChannelProbeResult{Revision: "r", TargetStatus: ChannelStatusAutoDisabled})
				require.Error(t, err)
			})

			if dialect == "sqlite" {
				return
			}
			t.Run("a competing SKIP write waits for the probe transaction", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "probe-lock", nil)
				marked := context.WithValue(ctx, probeResultMarker{}, true)
				writerDone := make(chan error, 1)
				var writerFinishedFirst, fired atomic.Bool
				const name = "test:probe_result_interleave"
				require.NoError(t, db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
					if tx.Statement.Context.Value(probeResultMarker{}) == nil || !fired.CompareAndSwap(false, true) {
						return
					}
					go func() {
						competing := &Channel{Id: channel.Id, Name: channel.Name, TestingModel: &skip}
						writerDone <- competing.UpdateWithContext(context.Background())
					}()
					select {
					case err := <-writerDone:
						writerFinishedFirst.Store(true)
						writerDone <- err
					case <-time.After(testingModelConcurrentWriterGrace):
					}
				}))
				t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(name)) })

				outcome, err := ApplyChannelProbeResultWithContext(marked, channel.Id, disable(channel))
				require.NoError(t, err)
				select {
				case err := <-writerDone:
					require.NoError(t, err)
				case <-time.After(30 * time.Second):
					require.FailNow(t, "competing SKIP writer never finished")
				}

				require.True(t, fired.Load())
				require.False(t, writerFinishedFirst.Load(),
					"the SKIP write must wait for the probe transaction that already checked the selection")
				require.True(t, outcome.StatusChanged, "the probe checked first, so its transition stands")
				row := stored(channel.Id)
				require.Equal(t, ChannelStatusAutoDisabled, row.Status)
				require.NotNil(t, row.TestingModel)
				require.Equal(t, skip, *row.TestingModel, "the later SKIP is persisted for future sweeps")
			})
		})
	}
}

// TestChannelUpdateHonoursCallerDeadlineMultiDB reproduces the review's unbounded lock
// wait: UpdateWithContext detaches from request cancellation so a disconnect cannot
// abort a half-applied update, but it must still respect the caller's deadline instead
// of waiting indefinitely behind another transaction's row lock.
// Parameters: t is the test handle. Returns: no values.
func TestChannelUpdateHonoursCallerDeadlineMultiDB(t *testing.T) {
	for _, dialect := range []string{"mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := useTestingModelMatrixBackend(t, dialect)
			channel := insertTestingModelMatrixChannel(t, dialect, "deadline", nil)

			holder := db.Begin()
			require.NoError(t, holder.Error)
			require.NoError(t, holder.Exec("UPDATE channels SET models = models WHERE id = ?", channel.Id).Error)
			released := make(chan struct{})
			go func() {
				defer close(released)
				time.Sleep(3 * time.Second)
				require.NoError(t, holder.Rollback().Error)
			}()
			t.Cleanup(func() { <-released })

			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			priority := int64(3)
			started := time.Now()
			err := (&Channel{Id: channel.Id, Name: channel.Name, Priority: &priority}).UpdateWithContext(ctx)
			elapsed := time.Since(started)

			require.Error(t, err, "the update must give up when the caller's deadline passes")
			require.Less(t, elapsed, 2*time.Second, "the lock wait must be bounded by the caller's deadline")
		})
	}
}
