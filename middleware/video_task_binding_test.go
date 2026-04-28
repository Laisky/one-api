package middleware

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
)

func setupVideoBindingTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&dbmodel.AsyncTaskBinding{})
	require.NoError(t, err)
	return db
}

func TestBindAsyncTaskChannelSetsContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	testDB := setupVideoBindingTestDB(t)
	originalDB := dbmodel.DB
	dbmodel.DB = testDB
	defer func() { dbmodel.DB = originalDB }()

	binding := dbmodel.AsyncTaskBinding{
		TaskID:         "video_ctx",
		TaskType:       "video",
		UserID:         10,
		TokenID:        20,
		ChannelID:      5,
		ChannelType:    7,
		OriginModel:    "sora-2",
		ActualModel:    "sora-2",
		CreatedAt:      time.Now().UTC().UnixMilli(),
		UpdatedAt:      time.Now().UTC().UnixMilli(),
		LastAccessedAt: time.Now().UTC().UnixMilli(),
	}
	require.NoError(t, testDB.Create(&binding).Error)

	handler := BindAsyncTaskChannel()
	engine.Use(handler)
	engine.GET("/v1/videos/:video_id", func(c *gin.Context) {
		require.Equal(t, 5, c.GetInt(ctxkey.SpecificChannelId))
		require.Equal(t, "sora-2", c.GetString(ctxkey.RequestModel))
		c.Status(204)
	})

	req := httptest.NewRequest("GET", "/v1/videos/video_ctx", nil).WithContext(context.Background())
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, 204, w.Code)

	fetched, err := dbmodel.GetAsyncTaskBindingByTaskID(context.Background(), "video_ctx")
	require.NoError(t, err)
	require.GreaterOrEqual(t, fetched.LastAccessedAt, binding.LastAccessedAt)
}

func TestBindAsyncTaskChannelNoRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	testDB := setupVideoBindingTestDB(t)
	originalDB := dbmodel.DB
	dbmodel.DB = testDB
	defer func() { dbmodel.DB = originalDB }()

	engine.Use(BindAsyncTaskChannel())
	engine.GET("/v1/videos/:video_id", func(c *gin.Context) {
		_, exists := c.Get(ctxkey.RequestModel)
		require.False(t, exists)
		c.Status(200)
	})

	req := httptest.NewRequest("GET", "/v1/videos/unknown", nil).WithContext(context.Background())
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
}
