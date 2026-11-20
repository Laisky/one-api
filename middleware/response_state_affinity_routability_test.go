package middleware

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
)

const (
	// routabilityChannelID is the bound fixture channel.
	routabilityChannelID = 17
	// routabilityModel is the model the binding continues.
	routabilityModel = "gpt-5"
)

// setupAffinityRoutabilityFixture installs a single-connection in-memory SQLite
// database holding one enabled channel bound to routabilityModel (and gpt-4o) in
// the default group with its abilities persisted, as every saved production
// channel has, selects the routing path by memory and builds the routing cache.
// Cleanup empties the tables and rebuilds the cache while the fixture database is
// still installed, so no fixture channel survives in the process-wide cache, and
// then restores the database, dialect and memory-cache globals.
//
// Parameters:
//   - t: the test handle.
//   - memory: true routes through the memory cache, false through the database.
//
// Return values:
//   - *gorm.DB: the installed fixture database.
func setupAffinityRoutabilityFixture(t *testing.T, memory bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// One connection keeps every query on the same in-memory database.
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&dbmodel.Channel{}, &dbmodel.ChannelGroup{}, &dbmodel.Ability{}))

	originalDB, originalSQLite, originalMemory := dbmodel.DB, common.UsingSQLite.Load(), config.MemoryCacheEnabled
	t.Cleanup(func() {
		dbmodel.DB, config.MemoryCacheEnabled = originalDB, originalMemory
		common.UsingSQLite.Store(originalSQLite)
	})
	dbmodel.DB, config.MemoryCacheEnabled = db, memory
	common.UsingSQLite.Store(true)

	channel := &dbmodel.Channel{
		Id: routabilityChannelID, Type: channeltype.OpenAI, Name: "bound",
		Status: dbmodel.ChannelStatusEnabled, Models: routabilityModel + ",gpt-4o", Group: "default", Config: "{}",
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities())
	require.NoError(t, dbmodel.InitChannelCacheContext(context.Background()))
	// Cleanups run last-in-first-out, so this runs before the globals above are
	// restored: it leaves an empty routing cache for later tests.
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DELETE FROM abilities").Error)
		require.NoError(t, db.Exec("DELETE FROM channels").Error)
		require.NoError(t, dbmodel.InitChannelCacheContext(context.Background()))
	})
	return db
}

// abilityRoutabilityScope scopes a query to the bound (default,
// routabilityModel, routabilityChannelID) ability.
//
// Parameters:
//   - db: the fixture database.
//
// Return values:
//   - *gorm.DB: the unexecuted scoped query.
func abilityRoutabilityScope(db *gorm.DB) *gorm.DB {
	return db.Model(&dbmodel.Ability{}).
		Where("`group` = ? AND model = ? AND channel_id = ?", "default", routabilityModel, routabilityChannelID)
}

// TestResponseStateAffinityUnpinsUnroutableAbility requires provider affinity
// to apply normal routing's exact-ability eligibility, not only its cooldown:
// a binding whose channel is still enabled and still lists the model, but whose
// (group, model, channel) ability is missing (the model was hidden or the row
// removed), disabled, or adaptively cooling, must fall back to normal selection.
// A healthy binding still pins. Both routing paths are covered.
func TestResponseStateAffinityUnpinsUnroutableAbility(t *testing.T) {
	for _, tc := range []struct {
		name         string
		breakAbility func(t *testing.T, db *gorm.DB)
	}{
		{
			name: "model hidden on the channel",
			breakAbility: func(t *testing.T, db *gorm.DB) {
				var channel dbmodel.Channel
				require.NoError(t, db.Where("id = ?", routabilityChannelID).Take(&channel).Error)
				hidden := `["` + routabilityModel + `"]`
				channel.HiddenModels = &hidden
				channel.HiddenModelsProvided = true
				channel.TestingModel = nil
				require.NoError(t, channel.UpdateWithContext(context.Background()))
				var rows int64
				require.NoError(t, abilityRoutabilityScope(db).Count(&rows).Error)
				require.Zero(t, rows, "hiding the model must remove its ability")
			},
		},
		{
			name: "missing ability row",
			breakAbility: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, abilityRoutabilityScope(db).Delete(&dbmodel.Ability{}).Error)
			},
		},
		{
			name: "disabled ability row",
			breakAbility: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, abilityRoutabilityScope(db).Update("enabled", false).Error)
			},
		},
		{
			name: "adaptive cooldown",
			breakAbility: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, abilityRoutabilityScope(db).
					Update("adaptive_suspend_until", time.Now().UTC().Add(time.Hour)).Error)
			},
		},
	} {
		for _, memory := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cache=%t", tc.name, memory), func(t *testing.T) {
				db := setupAffinityRoutabilityFixture(t, memory)
				gwID := enableAffinityState(t, routabilityChannelID)

				got := responseStateAffinityChannel(affinityContext(t, gwID), relaymode.ResponseAPI, "default", routabilityModel, false)
				require.NotNil(t, got, "a routable binding must pin")
				require.Equal(t, routabilityChannelID, got.Id)

				tc.breakAbility(t, db)
				require.NoError(t, dbmodel.InitChannelCacheContext(context.Background()))
				channel, err := dbmodel.GetChannelById(routabilityChannelID, true)
				require.NoError(t, err)
				require.Equal(t, dbmodel.ChannelStatusEnabled, channel.Status, "the channel itself stays enabled")
				require.True(t, channel.SupportsModel(routabilityModel), "the channel still lists the model")

				require.Nil(t, responseStateAffinityChannel(affinityContext(t, gwID), relaymode.ResponseAPI, "default", routabilityModel, false),
					"a binding without a routable ability must fall back to normal selection")
			})
		}
	}
}

// TestResponseStateAffinityRoutabilityFailureFallsBack proves a failed
// routability lookup on the database path is handled locally: the binding is
// not pinned and the request is neither aborted nor answered, so normal
// selection proceeds. Only the abilities query fails, so the failure is the
// routability check's own.
func TestResponseStateAffinityRoutabilityFailureFallsBack(t *testing.T) {
	db := setupAffinityRoutabilityFixture(t, false)
	gwID := enableAffinityState(t, routabilityChannelID)
	var armed atomic.Bool
	var injected atomic.Int32
	// Registered after the fixture's cleanup, so it runs first and lets that
	// cleanup rebuild the routing cache from a healthy database.
	t.Cleanup(func() { armed.Store(false) })
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:fail_abilities", func(tx *gorm.DB) {
		if armed.Load() && tx.Statement.Table == "abilities" {
			injected.Add(1)
			require.Error(t, tx.AddError(errors.New("injected abilities failure")), "gorm must record the injected failure")
		}
	}))

	got := responseStateAffinityChannel(affinityContext(t, gwID), relaymode.ResponseAPI, "default", routabilityModel, false)
	require.NotNil(t, got, "control: the healthy binding pins while the lookup succeeds")

	armed.Store(true)
	c := affinityContext(t, gwID)
	require.Nil(t, responseStateAffinityChannel(c, relaymode.ResponseAPI, "default", routabilityModel, false),
		"a failed routability lookup must fall back to normal selection")
	require.Positive(t, injected.Load(), "the abilities lookup must have been attempted")
	require.False(t, c.IsAborted(), "the request must not be aborted")
	require.False(t, c.Writer.Written(), "no response may be written")
	require.Equal(t, http.StatusOK, c.Writer.Status())
}
