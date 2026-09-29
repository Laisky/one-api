package model

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAsyncTaskTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&AsyncTaskBinding{}, &AsyncTaskBindingRetry{})
	require.NoError(t, err)
	return db
}

// TestAsyncTaskBindingRetrySurvivesRestart verifies a failed binding save remains recoverable after reopening the primary database.
func TestAsyncTaskBindingRetrySurvivesRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "async-tasks.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)
	originalDB := DB
	DB = db
	t.Cleanup(func() { DB = originalDB })
	require.NoError(t, db.AutoMigrate(&AsyncTaskBinding{}, &AsyncTaskBindingRetry{}))
	rejected := false
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail_first_async_binding", func(tx *gorm.DB) {
		if tx.Statement.Table == "async_task_bindings" && !rejected {
			rejected = true
			tx.AddError(errors.New("simulated transient binding write failure"))
		}
	}))
	binding := &AsyncTaskBinding{
		TaskID: "restart-job", TaskType: "video", UserID: 41, ChannelID: 9, ChannelType: 61,
		OriginModel: "veo3-fast", ActualModel: "veo3-fast", RequestMethod: "POST", RequestPath: "/v1/videos",
	}
	require.Error(t, SaveAsyncTaskBinding(context.Background(), binding))
	require.NoError(t, SaveAsyncTaskBindingRetry(context.Background(), binding))

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	restartedDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)
	DB = restartedDB
	restartedSQL, err := restartedDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = restartedSQL.Close() })

	recovered, err := RecoverAsyncTaskBindingRetry(context.Background(), "restart-job", 99)
	require.NoError(t, err)
	require.False(t, recovered, "a different user cannot replay another owner's binding")

	count, err := RecoverAsyncTaskBindingRetries(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	fetched, err := GetAsyncTaskBindingByTaskID(context.Background(), "restart-job")
	require.NoError(t, err)
	require.Equal(t, 41, fetched.UserID)
	require.Equal(t, 9, fetched.ChannelID)
	var retries int64
	require.NoError(t, restartedDB.Model(&AsyncTaskBindingRetry{}).Count(&retries).Error)
	require.Zero(t, retries, "the durable retry is acknowledged only after the binding is saved")
}

func TestSaveAndGetAsyncTaskBinding(t *testing.T) {
	testDB := setupAsyncTaskTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	binding := &AsyncTaskBinding{
		TaskID:        "video_123",
		TaskType:      "video",
		UserID:        42,
		TokenID:       7,
		ChannelID:     3,
		ChannelType:   9,
		OriginModel:   "sora-2",
		ActualModel:   "sora-2",
		RequestMethod: "POST",
		RequestPath:   "/v1/videos",
		RequestParams: "{\"model\":\"sora-2\"}",
	}

	err := SaveAsyncTaskBinding(context.Background(), binding)
	require.NoError(t, err)

	fetched, err := GetAsyncTaskBindingByTaskID(context.Background(), "video_123")
	require.NoError(t, err)
	require.Equal(t, "video", fetched.TaskType)
	require.Equal(t, 3, fetched.ChannelID)
	require.Equal(t, "sora-2", fetched.ActualModel)
	require.NotZero(t, fetched.CreatedAt)
	require.NotZero(t, fetched.LastAccessedAt)
}

func TestTouchAsyncTaskBinding(t *testing.T) {
	testDB := setupAsyncTaskTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	binding := &AsyncTaskBinding{
		TaskID:      "video_touch",
		TaskType:    "video",
		UserID:      1,
		ChannelID:   2,
		ChannelType: 11,
	}
	err := SaveAsyncTaskBinding(context.Background(), binding)
	require.NoError(t, err)

	fetched, err := GetAsyncTaskBindingByTaskID(context.Background(), "video_touch")
	require.NoError(t, err)
	originalAccess := fetched.LastAccessedAt
	require.NotZero(t, originalAccess)

	// Ensure timestamp changes
	time.Sleep(5 * time.Millisecond)
	err = TouchAsyncTaskBinding(context.Background(), "video_touch")
	require.NoError(t, err)

	updated, err := GetAsyncTaskBindingByTaskID(context.Background(), "video_touch")
	require.NoError(t, err)
	require.Greater(t, updated.LastAccessedAt, originalAccess)
}

func TestCleanExpiredAsyncTaskBindings(t *testing.T) {
	testDB := setupAsyncTaskTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	now := time.Now().UTC().UnixMilli()
	old := now - int64(8*24*time.Hour/time.Millisecond)

	stale := AsyncTaskBinding{
		TaskID:         "video_old",
		TaskType:       "video",
		UserID:         1,
		ChannelID:      2,
		ChannelType:    3,
		CreatedAt:      old,
		UpdatedAt:      old,
		LastAccessedAt: old,
	}
	fresh := AsyncTaskBinding{
		TaskID:         "video_new",
		TaskType:       "video",
		UserID:         1,
		ChannelID:      2,
		ChannelType:    3,
		CreatedAt:      now,
		UpdatedAt:      now,
		LastAccessedAt: now,
	}

	require.NoError(t, testDB.Create(&stale).Error)
	require.NoError(t, testDB.Create(&fresh).Error)

	deleted, err := CleanExpiredAsyncTaskBindings(7)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	_, err = GetAsyncTaskBindingByTaskID(context.Background(), "video_old")
	require.Error(t, err)

	still, err := GetAsyncTaskBindingByTaskID(context.Background(), "video_new")
	require.NoError(t, err)
	require.Equal(t, "video", still.TaskType)
}
