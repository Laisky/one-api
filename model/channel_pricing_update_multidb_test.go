package model

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/errkind"
)

// interleaveBeforeChannelPricingWrite registers a one-shot callback that runs action on
// the pricing transaction's own connection immediately before it writes the pricing
// columns of channel id, i.e. after it has locked and read the row.
// Parameters: t is the test handle, db is the database, id is the target channel, and
// action receives a session bound to the in-flight transaction.
// Returns: no values; the callback is removed when the test ends.
func interleaveBeforeChannelPricingWrite(t *testing.T, db *gorm.DB, id int, action func(tx *gorm.DB)) {
	t.Helper()
	name := "test:channel_pricing_interleave"
	var fired atomic.Bool
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		values, ok := tx.Statement.Dest.(map[string]any)
		if !ok || tx.Statement.Table != "channels" {
			return
		}
		_, pricing := values["model_configs"]
		_, config := values["config"]
		if !pricing && !config || !fired.CompareAndSwap(false, true) {
			return
		}
		action(tx.Session(&gorm.Session{NewDB: true}))
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove(name)) })
}

// loadMatrixChannel reads the full stored channel row.
// Parameters: t is the test handle, db is the database, and id identifies the channel.
// Returns: the stored row.
func loadMatrixChannel(t *testing.T, db *gorm.DB, id int) *Channel {
	t.Helper()
	var stored Channel
	require.NoError(t, db.First(&stored, "id = ?", id).Error)
	return &stored
}

// TestChannelPricingUpdateMultiDB pins the pricing write contract on SQLite, MySQL
// (MYSQL_DSN), and PostgreSQL (PG_DSN): the update writes only model_configs and config,
// clears them with SQL NULL and an empty blob, classifies its failures, and holds the
// row lock so a concurrent edit of other columns waits and is kept.
// Parameters: t is the test handle. Returns: no values.
func TestChannelPricingUpdateMultiDB(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := useTestingModelMatrixBackend(t, dialect)
			ctx := context.Background()
			pricing := ChannelPricingUpdate{
				ModelConfigs:         map[string]ModelConfigLocal{"gpt-4o": {Ratio: 2.5, MaxTokens: 4096}},
				ModelConfigsProvided: true,
				Tooling:              &ChannelToolingConfig{Whitelist: []string{"web_search"}},
				ToolingProvided:      true,
			}

			t.Run("writes only the pricing columns", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "pricing-columns", nil)
				require.NoError(t, db.Exec("UPDATE channels SET updated_at = 0 WHERE id = ?", channel.Id).Error)
				const editedModels = "gpt-4o-mini,gpt-4o,o3-mini"
				interleaveBeforeChannelPricingWrite(t, db, channel.Id, func(tx *gorm.DB) {
					require.NoError(t, tx.Exec("UPDATE channels SET priority = ?, models = ? WHERE id = ?",
						7, editedModels, channel.Id).Error)
				})

				require.NoError(t, UpdateChannelPricingWithContext(ctx, channel.Id, pricing))

				stored := loadMatrixChannel(t, db, channel.Id)
				require.EqualValues(t, 7, stored.GetPriority(), "priority is not a pricing column")
				require.Equal(t, editedModels, stored.Models, "models is not a pricing column")
				require.Equal(t, pricing.ModelConfigs, stored.GetModelPriceConfigs())
				require.Equal(t, pricing.Tooling, stored.GetToolingConfig())
				require.Positive(t, stored.UpdatedAt, "a pricing write must still bump updated_at")
			})

			t.Run("clearing stores SQL NULL and an empty config", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "pricing-clear", nil)
				require.NoError(t, UpdateChannelPricingWithContext(ctx, channel.Id, pricing))

				require.NoError(t, UpdateChannelPricingWithContext(ctx, channel.Id, ChannelPricingUpdate{
					ModelConfigsProvided: true,
					ToolingProvided:      true,
				}))

				require.False(t, readNullStringColumn(t, db, "channels", "model_configs", channel.Id).Valid)
				require.Equal(t, "", readNullStringColumn(t, db, "channels", "config", channel.Id).String)
			})

			t.Run("an empty update writes nothing", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "pricing-empty", nil)
				require.NoError(t, UpdateChannelPricingWithContext(ctx, channel.Id, pricing))
				require.NoError(t, db.Exec("UPDATE channels SET updated_at = 0 WHERE id = ?", channel.Id).Error)

				require.NoError(t, UpdateChannelPricingWithContext(ctx, channel.Id, ChannelPricingUpdate{}))

				stored := loadMatrixChannel(t, db, channel.Id)
				require.Zero(t, stored.UpdatedAt)
				require.Equal(t, pricing.ModelConfigs, stored.GetModelPriceConfigs())
			})

			t.Run("a missing channel is reported as not found", func(t *testing.T) {
				err := UpdateChannelPricingWithContext(ctx, 987654321, pricing)
				require.ErrorIs(t, err, gorm.ErrRecordNotFound)
				require.Equal(t, errkind.NotFound, errkind.Of(err))
			})

			t.Run("invalid pricing is a client error and writes nothing", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "pricing-invalid", nil)
				require.NoError(t, UpdateChannelPricingWithContext(ctx, channel.Id, pricing))

				err := UpdateChannelPricingWithContext(ctx, channel.Id, ChannelPricingUpdate{
					ModelConfigs:         map[string]ModelConfigLocal{"gpt-4o": {Ratio: -1}},
					ModelConfigsProvided: true,
					ToolingProvided:      true,
				})
				require.Error(t, err)
				require.Equal(t, errkind.InvalidRequest, errkind.Of(err))
				stored := loadMatrixChannel(t, db, channel.Id)
				require.Equal(t, pricing.ModelConfigs, stored.GetModelPriceConfigs())
				require.Equal(t, pricing.Tooling, stored.GetToolingConfig(), "the failed update must roll back")
			})

			if dialect == "sqlite" {
				// SQLite serialises writers database-wide and this harness uses an
				// in-memory database per connection, so the row-lock scenario below
				// only applies to the server backends.
				return
			}
			t.Run("a concurrent channel edit waits for the pricing row lock and is kept", func(t *testing.T) {
				channel := insertTestingModelMatrixChannel(t, dialect, "pricing-lock", nil)
				writerDone := make(chan error, 1)
				var writerFinishedFirst atomic.Bool
				interleaveBeforeChannelPricingWrite(t, db, channel.Id, func(*gorm.DB) {
					go func() {
						priority := int64(9)
						competing := &Channel{Id: channel.Id, Name: channel.Name, Priority: &priority}
						writerDone <- competing.UpdateWithContext(context.Background())
					}()
					select {
					case err := <-writerDone:
						writerFinishedFirst.Store(true)
						writerDone <- err
					case <-time.After(testingModelConcurrentWriterGrace):
					}
				})

				require.NoError(t, UpdateChannelPricingWithContext(ctx, channel.Id, pricing))

				select {
				case err := <-writerDone:
					require.NoError(t, err)
				case <-time.After(30 * time.Second):
					require.FailNow(t, "competing channel writer never finished")
				}
				require.False(t, writerFinishedFirst.Load(),
					"the competing writer must wait for the in-flight pricing update's row lock")
				stored := loadMatrixChannel(t, db, channel.Id)
				require.EqualValues(t, 9, stored.GetPriority(), "the competing priority edit must be kept")
				require.Equal(t, pricing.ModelConfigs, stored.GetModelPriceConfigs(), "the pricing write must be kept")
			})
		})
	}
}
