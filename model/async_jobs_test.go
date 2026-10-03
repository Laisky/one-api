package model

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
)

// durableJobTestStore uses file-backed SQLite and a separate log database so
// crash recovery and split-store replay are real persistence tests, not mocks.
func durableJobTestStore(t *testing.T) (string, *AsyncTask) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tasks.db")
	primary, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "logs.db")), &gorm.Config{})
	require.NoError(t, err)
	oldDB, oldLog, oldRedis, oldSQLite, oldLogging := DB, LOG_DB, common.IsRedisEnabled(), common.UsingSQLite.Load(), config.IsLogConsumeEnabled()
	DB, LOG_DB = primary, logDB
	common.SetRedisEnabled(false)
	common.UsingSQLite.Store(true)
	config.SetLogConsumeEnabled(true)
	dbConn, err := primary.DB()
	require.NoError(t, err)
	dbConn.SetMaxOpenConns(1)
	logsConn, err := logDB.DB()
	require.NoError(t, err)
	logsConn.SetMaxOpenConns(1)
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLog
		common.SetRedisEnabled(oldRedis)
		common.UsingSQLite.Store(oldSQLite)
		config.SetLogConsumeEnabled(oldLogging)
		_ = dbConn.Close()
		_ = logsConn.Close()
	})
	require.NoError(t, primary.AutoMigrate(&User{}, &Token{}, &Channel{}, &UserRequestCost{}, &AsyncTask{}))
	require.NoError(t, logDB.AutoMigrate(&Log{}, &AsyncTaskLogReceipt{}))
	user := &User{Id: 987001, UUID: uuid.NewString(), Username: "async-owner", Status: UserStatusEnabled, Quota: 1000000}
	require.NoError(t, primary.Create(user).Error)
	token := &Token{Id: 987002, UUID: uuid.NewString(), UserId: user.Id, Key: uuid.NewString(), Status: TokenStatusEnabled, RemainQuota: 1000000, ExpiredTime: -1}
	require.NoError(t, primary.Create(token).Error)
	channel := &Channel{Id: 987003, UUID: uuid.NewString(), Type: 61, Name: "async-provider", Status: ChannelStatusEnabled}
	require.NoError(t, primary.Create(channel).Error)
	body := `{"prompt":"private input"}`
	hash, err := AsyncTaskRequestHash([]byte(body))
	require.NoError(t, err)
	return path, &AsyncTask{DedupKey: AsyncTaskDedupKey(user.Id, user.UUID, "original"), RequestHash: hash, UserID: user.Id, UserUUID: user.UUID, TokenID: token.Id, TokenUUID: token.UUID, ChannelID: channel.Id, ChannelUUID: channel.UUID, ChannelType: channel.Type, BaseURL: "https://provider.example", OriginModel: "video-model", ActualModel: "video-model", RequestBody: body, Quota: 200000, RequestID: "async-fixture"}
}

// reserveDurableJob asserts a fresh atomic debit and returns the persisted job.
func reserveDurableJob(t *testing.T, task *AsyncTask) *AsyncTask {
	t.Helper()
	saved, created, err := ReserveAsyncTask(context.Background(), task)
	require.NoError(t, err)
	require.True(t, created)
	return saved
}

// completeDurableJob advances a lease through acknowledgement and a normalized
// final result, exercising the actual transaction and fencing implementation.
func completeDurableJob(t *testing.T, task *AsyncTask) {
	t.Helper()
	ctx := context.Background()
	lease, err := ClaimAsyncTask(ctx, time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.Equal(t, task.ID, lease.ID)
	require.NoError(t, ApplyAsyncTaskUpdate(ctx, lease, AsyncTaskUpdate{State: AsyncTaskQueued, UpstreamID: "upstream-job"}))
	lease, err = ClaimAsyncTask(ctx, time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.NoError(t, ApplyAsyncTaskUpdate(ctx, lease, AsyncTaskUpdate{State: AsyncTaskCompleted, ResultJSON: `{"videos":[{"url":"https://media.example/video.mp4"}]}`}))
	require.ErrorIs(t, ApplyAsyncTaskUpdate(ctx, lease, AsyncTaskUpdate{State: AsyncTaskCompleted, ResultJSON: `{"videos":[{"url":"https://media.example/video.mp4"}]}`}), ErrAsyncLeaseLost)
}

// TestAsyncTaskRestartResumesWithoutAnotherDebit closes and reopens the actual
// primary database after reservation. The idempotency receipt and hold survive.
func TestAsyncTaskRestartResumesWithoutAnotherDebit(t *testing.T) {
	path, template := durableJobTestStore(t)
	task := reserveDurableJob(t, template)
	connection, err := DB.DB()
	require.NoError(t, err)
	require.NoError(t, connection.Close())
	reopened, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	DB = reopened
	reopenedSQL, err := reopened.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopenedSQL.Close() })
	again, created, err := ReserveAsyncTask(context.Background(), template)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, task.ID, again.ID)
	completeDurableJob(t, task)
	var user User
	var token Token
	var channel Channel
	require.NoError(t, DB.Where("id = ?", task.UserID).Take(&user).Error)
	require.NoError(t, DB.Where("id = ?", task.TokenID).Take(&token).Error)
	require.NoError(t, DB.Where("id = ?", task.ChannelID).Take(&channel).Error)
	require.EqualValues(t, 800000, user.Quota)
	require.EqualValues(t, 200000, user.UsedQuota)
	require.EqualValues(t, 1, user.RequestCount)
	require.EqualValues(t, 800000, token.RemainQuota)
	require.EqualValues(t, 200000, token.UsedQuota)
	require.EqualValues(t, 200000, channel.UsedQuota)
	stored, err := GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	require.Empty(t, stored.RequestBody)
}

// TestAsyncTaskAdmissionRollbackCannotLeaveAnOrphan fails the finite token write
// after the user debit and task insert. The transaction must roll back all three.
func TestAsyncTaskAdmissionRollbackCannotLeaveAnOrphan(t *testing.T) {
	_, template := durableJobTestStore(t)
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("async_test_fail_token", func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(errors.New("injected token write failure"))
		}
	}))
	_, _, err := ReserveAsyncTask(context.Background(), template)
	require.Error(t, err)
	require.NoError(t, DB.Callback().Update().Remove("async_test_fail_token"))
	var user User
	require.NoError(t, DB.Where("id = ?", template.UserID).Take(&user).Error)
	require.EqualValues(t, 1000000, user.Quota)
	var count int64
	require.NoError(t, DB.Model(&AsyncTask{}).Count(&count).Error)
	require.Zero(t, count)
}

// TestAsyncTaskSettlementFailureIsAtomic rejects the user usage write after the
// task terminal CAS. Neither the terminal state nor any accounting may commit.
func TestAsyncTaskSettlementFailureIsAtomic(t *testing.T) {
	_, template := durableJobTestStore(t)
	task := reserveDurableJob(t, template)
	lease, err := ClaimAsyncTask(context.Background(), time.Now())
	require.NoError(t, err)
	require.NoError(t, ApplyAsyncTaskUpdate(context.Background(), lease, AsyncTaskUpdate{State: AsyncTaskQueued, UpstreamID: "job"}))
	lease, err = ClaimAsyncTask(context.Background(), time.Now())
	require.NoError(t, err)
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("async_test_fail_user", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(errors.New("injected usage failure"))
		}
	}))
	result := AsyncTaskUpdate{State: AsyncTaskCompleted, ResultJSON: `{"videos":[{"url":"https://example.com/v.mp4"}]}`}
	require.Error(t, ApplyAsyncTaskUpdate(context.Background(), lease, result))
	require.NoError(t, DB.Callback().Update().Remove("async_test_fail_user"))
	persisted, err := GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	require.Equal(t, AsyncTaskQueued, persisted.State)
	require.Equal(t, AsyncBillingHeld, persisted.BillingState)
	require.NoError(t, ApplyAsyncTaskUpdate(context.Background(), lease, result))
}

// TestAsyncTaskSplitLogOutboxIsIdempotent simulates a crash AFTER the LOG_DB
// commit but BEFORE primary acknowledgement. Reopening/flushing cannot duplicate
// the usage log, and log failure cannot replay the charge.
func TestAsyncTaskSplitLogOutboxIsIdempotent(t *testing.T) {
	_, template := durableJobTestStore(t)
	task := reserveDurableJob(t, template)
	completeDurableJob(t, task)
	require.False(t, DB.Migrator().HasTable(&Log{}), "logs really live in a different database")
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("async_test_fail_log_ack", func(tx *gorm.DB) {
		if tx.Statement.Table == "async_tasks" {
			tx.AddError(errors.New("injected ack failure"))
		}
	}))
	require.Error(t, FlushAsyncTaskLogs(context.Background()))
	require.NoError(t, DB.Callback().Update().Remove("async_test_fail_log_ack"))
	for range 3 {
		require.NoError(t, FlushAsyncTaskLogs(context.Background()))
	}
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("request_id = ?", task.RequestID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, LOG_DB.Model(&AsyncTaskLogReceipt{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	var user User
	require.NoError(t, DB.Where("id = ?", task.UserID).Take(&user).Error)
	require.EqualValues(t, 800000, user.Quota)
	require.NoError(t, DB.Model(&UserRequestCost{}).Where("request_id = ?", task.RequestID).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

// TestAsyncTaskRetentionNeverDropsOutstandingFunds preserves uncertain holds
// indefinitely but prunes completed, logged receipts only after their window.
func TestAsyncTaskRetentionNeverDropsOutstandingFunds(t *testing.T) {
	_, template := durableJobTestStore(t)
	task := reserveDurableJob(t, template)
	completeDurableJob(t, task)
	require.NoError(t, FlushAsyncTaskLogs(context.Background()))
	now := time.Now().UTC().Add(32 * 24 * time.Hour)
	unknown := *template
	unknown.ID = ""
	unknown.DedupKey = AsyncTaskDedupKey(unknown.UserID, unknown.UserUUID, "unknown")
	unknown.RequestBody = `{"prompt":"still private"}`
	unknownTask := reserveDurableJob(t, &unknown)
	require.NoError(t, DB.Model(&AsyncTask{}).Where("id = ?", unknownTask.ID).Updates(map[string]any{"state": AsyncTaskUnknown, "completed_at": time.Now().UnixMilli()}).Error)
	require.NoError(t, CleanSettledAsyncTasks(context.Background(), now))
	require.NoError(t, CleanAsyncTaskLogReceipts(context.Background(), now))
	_, err := GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	retained, err := GetOwnedAsyncTask(context.Background(), unknownTask.ID, unknownTask.UserID, unknownTask.UserUUID)
	require.NoError(t, err)
	require.Equal(t, AsyncBillingHeld, retained.BillingState)
	var count int64
	require.NoError(t, LOG_DB.Model(&AsyncTaskLogReceipt{}).Count(&count).Error)
	require.Zero(t, count)
}

// TestAsyncTaskUUIDReuseCannotMoveFunds proves a replaced numeric user ID cannot
// receive the original owner's reserved funds or retrieve their private result.
func TestAsyncTaskUUIDReuseCannotMoveFunds(t *testing.T) {
	_, template := durableJobTestStore(t)
	task := reserveDurableJob(t, template)
	lease, err := ClaimAsyncTask(context.Background(), time.Now())
	require.NoError(t, err)
	newUUID := uuid.NewString()
	require.NoError(t, DB.Model(&User{}).Where("id = ?", task.UserID).Update("uuid", newUUID).Error)
	require.Error(t, ApplyAsyncTaskUpdate(context.Background(), lease, AsyncTaskUpdate{State: AsyncTaskFailed, Refund: true}))
	_, err = GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, newUUID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	var user User
	require.NoError(t, DB.Where("id = ?", task.UserID).Take(&user).Error)
	require.EqualValues(t, 800000, user.Quota)
}

// TestAsyncTaskTokenDeletionDoesNotStrandPrepaidJob reproduces a token being
// deleted while its already-paid task is running. Completion must still settle,
// and refund may credit the original owner but never a replacement token ID.
func TestAsyncTaskTokenDeletionDoesNotStrandPrepaidJob(t *testing.T) {
	for _, refund := range []bool{false, true} {
		t.Run(map[bool]string{false: "completion", true: "refund with reused token id"}[refund], func(t *testing.T) {
			_, template := durableJobTestStore(t)
			task := reserveDurableJob(t, template)
			lease, err := ClaimAsyncTask(context.Background(), time.Now())
			require.NoError(t, err)
			require.NoError(t, ApplyAsyncTaskUpdate(context.Background(), lease, AsyncTaskUpdate{State: AsyncTaskQueued, UpstreamID: "paid-job"}))
			require.NoError(t, DB.Unscoped().Where("id = ?", task.TokenID).Delete(&Token{}).Error)
			replacement := &Token{Id: task.TokenID, UUID: uuid.NewString(), UserId: task.UserID, Key: uuid.NewString(), Status: TokenStatusEnabled, RemainQuota: 7, ExpiredTime: -1}
			if refund {
				require.NoError(t, DB.Create(replacement).Error)
			}
			lease, err = ClaimAsyncTask(context.Background(), time.Now())
			require.NoError(t, err)
			update := AsyncTaskUpdate{State: AsyncTaskCompleted, ResultJSON: `{"videos":[{"url":"https://example.com/v.mp4"}]}`}
			if refund {
				update = AsyncTaskUpdate{State: AsyncTaskFailed, Refund: true}
			}
			require.NoError(t, ApplyAsyncTaskUpdate(context.Background(), lease, update))
			var owner User
			require.NoError(t, DB.Where("id = ?", task.UserID).Take(&owner).Error)
			if refund {
				require.EqualValues(t, 1000000, owner.Quota)
				var token Token
				require.NoError(t, DB.Where("id = ?", replacement.Id).Take(&token).Error)
				require.EqualValues(t, 7, token.RemainQuota)
			} else {
				require.EqualValues(t, 800000, owner.Quota)
				require.EqualValues(t, 200000, owner.UsedQuota)
			}
		})
	}
}
