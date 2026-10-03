package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAsyncTaskTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&AsyncTaskBinding{})
	require.NoError(t, err)
	return db
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
