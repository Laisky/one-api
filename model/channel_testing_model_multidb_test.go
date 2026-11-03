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
