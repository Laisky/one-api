package monitor

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// setupMonitorChannelDB installs a private SQLite database with the channel tables and a
// local message pusher that counts the notifications it receives.
// Parameters: t is the test handle.
// Returns: the database and the delivered-notification counter; globals are restored on cleanup.
func setupMonitorChannelDB(t *testing.T) (*gorm.DB, *atomic.Int64) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/monitor.db"), &gorm.Config{Logger: glogger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelGroup{}, &model.Ability{}))

	var delivered atomic.Int64
	pusher := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		delivered.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"message":""}`))
	}))

	originalDB, originalSQLite := model.DB, common.UsingSQLite.Load()
	originalPusher, originalRootEmail := config.MessagePusherAddress, config.RootUserEmail
	model.DB = db
	common.UsingSQLite.Store(true)
	config.MessagePusherAddress = pusher.URL
	config.RootUserEmail = "root@example.com"
	t.Cleanup(func() {
		model.DB = originalDB
		common.UsingSQLite.Store(originalSQLite)
		config.MessagePusherAddress, config.RootUserEmail = originalPusher, originalRootEmail
		pusher.Close()
		require.NoError(t, sqlDB.Close())
	})
	return db, &delivered
}

// insertMonitoredChannel creates a channel with abilities in the given status.
// Parameters: t is the test handle, name labels it, and status is its initial status.
// Returns: the inserted channel.
func insertMonitoredChannel(t *testing.T, name string, status int) *model.Channel {
	t.Helper()
	channel := &model.Channel{Name: name, Type: 1, Key: "sk-test", Status: status,
		Models: "gpt-4o-mini", Group: "default"}
	require.NoError(t, channel.Insert())
	return channel
}

// failChannelWrites makes every UPDATE fail, as a lock timeout or lost connection would.
// Parameters: t is the test handle and db is the database. Returns: no values.
func failChannelWrites(t *testing.T, db *gorm.DB) {
	t.Helper()
	const name = "test:fail_channel_writes"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		_ = tx.AddError(errors.New("injected write failure"))
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove(name)) })
}

// storedChannelState reads a channel's status and its abilities' routing flags.
// Parameters: t is the test handle, db is the database, and id identifies the channel.
// Returns: the stored status and each ability's enabled flag.
func storedChannelState(t *testing.T, db *gorm.DB, id int) (int, []bool) {
	t.Helper()
	var channel model.Channel
	require.NoError(t, db.Select("id", "status").First(&channel, "id = ?", id).Error)
	var enabled []bool
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", id).Pluck("enabled", &enabled).Error)
	return channel.Status, enabled
}

// TestAutomaticStatusChangesAnnounceOnlyCommittedTransitions reproduces the review's P2:
// the relay-time, balance and success-rate disable paths announced a status change even
// when it was never persisted, and announced it again for a channel that was already
// disabled. Automatic paths must also never override an administrator's manual disable.
// Parameters: t is the test handle. Returns: no values.
func TestAutomaticStatusChangesAnnounceOnlyCommittedTransitions(t *testing.T) {
	disablers := map[string]func(channel *model.Channel){
		"DisableChannel": func(channel *model.Channel) {
			DisableChannel(channel.Id, channel.Name, "invalid api key")
		},
		"MetricDisableChannel": func(channel *model.Channel) {
			MetricDisableChannel(channel.Id, 0.1)
		},
	}
	for name, disable := range disablers {
		t.Run(name, func(t *testing.T) {
			t.Run("a persisted disable is announced once", func(t *testing.T) {
				db, delivered := setupMonitorChannelDB(t)
				channel := insertMonitoredChannel(t, "disable-once", model.ChannelStatusEnabled)

				disable(channel)

				status, enabled := storedChannelState(t, db, channel.Id)
				require.Equal(t, model.ChannelStatusAutoDisabled, status)
				require.Equal(t, []bool{false}, enabled)
				require.EqualValues(t, 1, delivered.Load())
			})

			t.Run("a failed write announces nothing", func(t *testing.T) {
				db, delivered := setupMonitorChannelDB(t)
				channel := insertMonitoredChannel(t, "disable-failed", model.ChannelStatusEnabled)
				failChannelWrites(t, db)

				disable(channel)

				status, _ := storedChannelState(t, db, channel.Id)
				require.Equal(t, model.ChannelStatusEnabled, status)
				require.Zero(t, delivered.Load(), "an uncommitted disable must not be announced")
			})

			t.Run("an already disabled channel is not announced again", func(t *testing.T) {
				db, delivered := setupMonitorChannelDB(t)
				channel := insertMonitoredChannel(t, "disable-repeat", model.ChannelStatusAutoDisabled)

				disable(channel)

				status, _ := storedChannelState(t, db, channel.Id)
				require.Equal(t, model.ChannelStatusAutoDisabled, status)
				require.Zero(t, delivered.Load())
			})

			t.Run("a manual disable is kept", func(t *testing.T) {
				db, delivered := setupMonitorChannelDB(t)
				channel := insertMonitoredChannel(t, "disable-manual", model.ChannelStatusManuallyDisabled)

				disable(channel)

				status, _ := storedChannelState(t, db, channel.Id)
				require.Equal(t, model.ChannelStatusManuallyDisabled, status,
					"an automatic disable must not turn a manual disable into an automatically re-enableable one")
				require.Zero(t, delivered.Load())
			})

			t.Run("abilities left routing by a legacy writer are repaired", func(t *testing.T) {
				db, delivered := setupMonitorChannelDB(t)
				channel := insertMonitoredChannel(t, "disable-drift", model.ChannelStatusAutoDisabled)
				require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", channel.Id).
					Update("enabled", true).Error)

				disable(channel)

				status, enabled := storedChannelState(t, db, channel.Id)
				require.Equal(t, model.ChannelStatusAutoDisabled, status)
				require.Equal(t, []bool{false}, enabled, "a disabled channel's abilities must stop routing")
				require.Zero(t, delivered.Load(), "repairing the projection is not a status change")
			})
		})
	}

	t.Run("EnableChannel", func(t *testing.T) {
		t.Run("a persisted re-enable is announced once", func(t *testing.T) {
			db, delivered := setupMonitorChannelDB(t)
			channel := insertMonitoredChannel(t, "enable-once", model.ChannelStatusAutoDisabled)

			EnableChannel(channel.Id, channel.Name)

			status, enabled := storedChannelState(t, db, channel.Id)
			require.Equal(t, model.ChannelStatusEnabled, status)
			require.Equal(t, []bool{true}, enabled)
			require.EqualValues(t, 1, delivered.Load())
		})

		t.Run("a failed write announces nothing", func(t *testing.T) {
			db, delivered := setupMonitorChannelDB(t)
			channel := insertMonitoredChannel(t, "enable-failed", model.ChannelStatusAutoDisabled)
			failChannelWrites(t, db)

			EnableChannel(channel.Id, channel.Name)

			status, _ := storedChannelState(t, db, channel.Id)
			require.Equal(t, model.ChannelStatusAutoDisabled, status)
			require.Zero(t, delivered.Load())
		})

		t.Run("a manual disable is kept", func(t *testing.T) {
			db, delivered := setupMonitorChannelDB(t)
			channel := insertMonitoredChannel(t, "enable-manual", model.ChannelStatusManuallyDisabled)

			EnableChannel(channel.Id, channel.Name)

			status, _ := storedChannelState(t, db, channel.Id)
			require.Equal(t, model.ChannelStatusManuallyDisabled, status)
			require.Zero(t, delivered.Load())
		})
	})
}
