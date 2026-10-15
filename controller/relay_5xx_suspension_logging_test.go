package controller

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/helper"
	dbmodel "github.com/Laisky/one-api/model"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

const suspension5XXSuccess = "ability suspension persisted after relay failure (normalized 5xx)"
const suspension5XXFailure = "ability suspension failed after relay failure (normalized 5xx)"
const suspension5XXGroup = "suspension-fixture"
const suspension5XXModel = "synthetic-requested-model"
const suspension5XXChannel = 71001

// suspension5XXFixture owns synthetic ability state and a request-scoped observed logger.
type suspension5XXFixture struct {
	db     *gorm.DB
	reader *gorm.DB
	ctx    context.Context
	logs   *observer.ObservedLogs
	params processChannelRelayErrorParams
}

// newSuspension5XXFixture creates private SQLite connections and an observed processor context for t.
// observeSuccess, when present, receives the independent reader exactly when the success event is written.
// It returns the owned fixture and restores all changed globals and closes its pools during cleanup.
func newSuspension5XXFixture(t *testing.T, observeSuccess func(*gorm.DB)) suspension5XXFixture {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "abilities.db")
	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(filename+"?_busy_timeout=1000"), &gorm.Config{Logger: gormlogger.Discard})
		require.NoError(t, err)
		pool, err := db.DB()
		require.NoError(t, err)
		pool.SetMaxOpenConns(1)
		t.Cleanup(func() { require.NoError(t, pool.Close()) })
		return db
	}
	db, reader := open(), open()
	oldDB := dbmodel.DB
	oldSQLite, oldMySQL, oldPostgres := common.UsingSQLite.Load(), common.UsingMySQL.Load(), common.UsingPostgreSQL.Load()
	oldMetric, oldDisable, oldDuration := config.EnableMetric, config.AutomaticDisableChannelEnabled, config.ChannelSuspendSecondsFor5XX
	dbmodel.DB = db
	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(false)
	// Metric consumers can notify administrators; this fixture observes only the real ability update and logs.
	config.EnableMetric, config.AutomaticDisableChannelEnabled = false, false
	config.ChannelSuspendSecondsFor5XX = 37 * time.Second
	t.Cleanup(func() {
		dbmodel.DB = oldDB
		common.UsingSQLite.Store(oldSQLite)
		common.UsingMySQL.Store(oldMySQL)
		common.UsingPostgreSQL.Store(oldPostgres)
		config.EnableMetric, config.AutomaticDisableChannelEnabled, config.ChannelSuspendSecondsFor5XX = oldMetric, oldDisable, oldDuration
	})
	require.NoError(t, db.AutoMigrate(&dbmodel.Ability{}, &dbmodel.Channel{}))
	require.NoError(t, db.Create(&dbmodel.Channel{Id: suspension5XXChannel, Name: "synthetic-suspension-channel", Status: dbmodel.ChannelStatusEnabled}).Error)
	for _, ability := range []dbmodel.Ability{
		{Group: suspension5XXGroup, Model: suspension5XXModel, ChannelId: suspension5XXChannel, Enabled: true},
		{Group: suspension5XXGroup, Model: "synthetic-upstream-model", ChannelId: suspension5XXChannel, Enabled: true},
		{Group: "unrelated-fixture-group", Model: suspension5XXModel, ChannelId: suspension5XXChannel, Enabled: true},
		{Group: suspension5XXGroup, Model: suspension5XXModel, ChannelId: suspension5XXChannel + 1, Enabled: true},
	} {
		require.NoError(t, db.Create(&ability).Error)
	}
	core, logs := observer.New(zapcore.DebugLevel)
	hooked := zapcore.RegisterHooks(core, func(entry zapcore.Entry) error {
		if entry.Message == suspension5XXSuccess && observeSuccess != nil {
			observeSuccess(reader)
		}
		return nil
	})
	lg, err := glog.New(glog.WithName("synthetic-suspension"), glog.WithLevel(glog.LevelDebug), glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return hooked })))
	require.NoError(t, err)
	params := processChannelRelayErrorParams{
		RequestID: "synthetic-suspension-request", ChannelId: suspension5XXChannel,
		ChannelName: "synthetic-suspension-channel", Group: suspension5XXGroup,
		OriginalModel: suspension5XXModel, ActualModel: "synthetic-upstream-model",
		Err: relaymodel.ErrorWithStatusCode{StatusCode: http.StatusInternalServerError,
			Error: relaymodel.Error{Type: relaymodel.ErrorTypeUpstream, Message: "synthetic provider failure", Code: "synthetic_server_error", RawError: errors.New("synthetic provider failure")}},
	}
	return suspension5XXFixture{db: db, reader: reader, ctx: gmw.SetLogger(context.Background(), lg), logs: logs, params: params}
}

// TestProcessChannelRelayError5XXFailedPersistenceDoesNotLogSuccess verifies failed updates and missing rows never emit a success event.
// Its t parameter owns the real SQLite failure controls and assertions; it returns no value.
func TestProcessChannelRelayError5XXFailedPersistenceDoesNotLogSuccess(t *testing.T) {
	for _, failure := range []string{"update_rejected", "missing_ability"} {
		t.Run(failure, func(t *testing.T) {
			fixture := newSuspension5XXFixture(t, nil)
			if failure == "update_rejected" {
				require.NoError(t, fixture.db.Exec("CREATE TRIGGER reject_suspension BEFORE UPDATE OF suspend_until ON abilities BEGIN SELECT RAISE(ABORT, 'synthetic suspension write rejected'); END").Error)
			} else {
				require.NoError(t, fixture.db.Where("\"group\" = ? AND model = ? AND channel_id = ?", suspension5XXGroup, suspension5XXModel, suspension5XXChannel).Delete(&dbmodel.Ability{}).Error)
			}
			processChannelRelayError(fixture.ctx, fixture.params)
			require.Len(t, fixture.logs.FilterMessage(suspension5XXFailure).All(), 1, "the real failed update must remain visible")
			require.Empty(t, fixture.logs.FilterMessage(suspension5XXSuccess).All(), "a rejected or zero-row update must not claim that the ability was suspended")
			var rows []dbmodel.Ability
			require.NoError(t, fixture.db.Order("model, channel_id").Find(&rows).Error)
			expectedRows := 4
			if failure == "missing_ability" {
				expectedRows = 3
			}
			require.Len(t, rows, expectedRows)
			for _, row := range rows {
				require.Nil(t, row.SuspendUntil)
				require.True(t, row.Enabled)
			}
		})
	}
}

// TestProcessChannelRelayError5XXSuccessLogFollowsCommit verifies persisted state is visible when the success event reaches its sink.
// Its t parameter owns the independent-reader observations and unchanged-scope assertions; it returns no value.
func TestProcessChannelRelayError5XXSuccessLogFollowsCommit(t *testing.T) {
	var atLog struct {
		SuspendUntil *time.Time
		Enabled      bool
	}
	var readErr error
	var successWrites int
	fixture := newSuspension5XXFixture(t, func(reader *gorm.DB) {
		successWrites++
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		readErr = reader.WithContext(ctx).Raw("SELECT suspend_until, enabled FROM abilities WHERE \"group\" = ? AND model = ? AND channel_id = ?", suspension5XXGroup, suspension5XXModel, suspension5XXChannel).Scan(&atLog).Error
	})
	updates, logsBeforeCommit := 0, -1
	require.NoError(t, fixture.db.Callback().Update().Before("gorm:commit_or_rollback_transaction").Register("test:observe_suspension_before_commit", func(tx *gorm.DB) {
		updates++
		logsBeforeCommit = fixture.logs.FilterMessage(suspension5XXSuccess).Len()
	}))
	before := time.Now().UTC()
	processChannelRelayError(fixture.ctx, fixture.params)
	after := time.Now().UTC()
	require.Equal(t, 1, updates)
	require.Zero(t, logsBeforeCommit, "the success event must not exist before the update transaction commits")
	require.NoError(t, readErr)
	require.Equal(t, 1, successWrites)
	require.NotNil(t, atLog.SuspendUntil, "the independent reader must observe the committed suspension at success-log emission")
	require.True(t, atLog.Enabled)
	require.False(t, atLog.SuspendUntil.Before(before.Add(config.ChannelSuspendSecondsFor5XX)))
	require.False(t, atLog.SuspendUntil.After(after.Add(config.ChannelSuspendSecondsFor5XX)))
	entries := fixture.logs.FilterMessage(suspension5XXSuccess).All()
	require.Len(t, entries, 1)
	require.Equal(t, zapcore.ErrorLevel, entries[0].Level)
	fields := entries[0].ContextMap()
	require.Equal(t, fixture.params.RequestID, fields["request_id"])
	require.EqualValues(t, suspension5XXChannel, fields["channel_id"])
	require.Equal(t, suspension5XXGroup, fields["group"])
	require.Equal(t, suspension5XXModel, fields["origin_model"])
	require.Equal(t, fixture.params.ActualModel, fields["actual_model"])
	require.EqualValues(t, http.StatusInternalServerError, fields["status_code"])
	require.Equal(t, fixture.params.Err.Code, fields["error_code"])
	require.Equal(t, config.ChannelSuspendSecondsFor5XX, fields["suspension_duration"])
	require.Equal(t, "normalized relay 5xx; applying configured ability recovery suspension", fields["suspension_rationale"])
	require.Empty(t, fixture.logs.FilterMessage(suspension5XXFailure).All())
	var rows []dbmodel.Ability
	require.NoError(t, fixture.db.Find(&rows).Error)
	require.Len(t, rows, 4)
	for _, row := range rows {
		require.True(t, row.Enabled)
		if row.Group == suspension5XXGroup && row.Model == suspension5XXModel && row.ChannelId == suspension5XXChannel {
			require.Equal(t, atLog.SuspendUntil, row.SuspendUntil)
		} else {
			require.Nil(t, row.SuspendUntil, "other groups, models and channels must remain unchanged")
		}
	}
	var channel dbmodel.Channel
	require.NoError(t, fixture.db.First(&channel, suspension5XXChannel).Error)
	require.Equal(t, dbmodel.ChannelStatusEnabled, channel.Status)
}

// TestProcessChannelRelayError5XXExcludedBranchesDoNotSuspend checks ordinary non-5xx and existing exclusions without changing health policy.
// Its t parameter owns each synthetic error and verifies untouched ability state; it returns no value.
func TestProcessChannelRelayError5XXExcludedBranchesDoNotSuspend(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		kind   string
	}{
		{"bad_request", http.StatusBadRequest, "provider"},
		{"unprocessable", http.StatusUnprocessableEntity, "provider"},
		{"too_large", http.StatusRequestEntityTooLarge, "provider"},
		{"request_timeout", http.StatusRequestTimeout, "provider"},
		{"internal_adaptor", http.StatusInternalServerError, "internal"},
		{"internal_infrastructure", http.StatusInternalServerError, "infrastructure"},
		{"cancelled", http.StatusInternalServerError, "cancelled"},
		{"deadline", http.StatusInternalServerError, "deadline"},
		{"provider_retry_hint", http.StatusServiceUnavailable, "retry_hint"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newSuspension5XXFixture(t, nil)
			fixture.params.Err.StatusCode = scenario.status
			switch scenario.kind {
			case "internal":
				fixture.params.Err.Type = relaymodel.ErrorTypeOneAPI
			case "infrastructure":
				fixture.params.Err.RawError = helper.ErrFFProbeUnavailable
			case "cancelled":
				fixture.params.Err.RawError = context.Canceled
			case "deadline":
				fixture.params.Err.RawError = context.DeadlineExceeded
			case "retry_hint":
				fixture.params.Err.Message = "synthetic provider temporarily unavailable; please retry"
			}
			processChannelRelayError(fixture.ctx, fixture.params)
			require.Empty(t, fixture.logs.FilterMessage(suspension5XXSuccess).All())
			require.Empty(t, fixture.logs.FilterMessage(suspension5XXFailure).All())
			var rows []dbmodel.Ability
			require.NoError(t, fixture.db.Find(&rows).Error)
			require.Len(t, rows, 4)
			for _, row := range rows {
				require.Nil(t, row.SuspendUntil)
				require.True(t, row.Enabled)
			}
		})
	}
}
