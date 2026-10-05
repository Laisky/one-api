package controller

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	gmw "github.com/Laisky/gin-middlewares/v7"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

var traceDBSetup sync.Once

func init() {
	gin.SetMode(gin.TestMode)

	traceDBSetup.Do(func() {
		if model.DB != nil {
			return
		}

		db, err := gorm.Open(sqlite.Open("file:response_api_tests?mode=memory&cache=shared"), &gorm.Config{})
		if err != nil {
			panic(err)
		}

		// Pin the pool to one connection. This handle can win the race to become the
		// package-wide model.DB, and shared-cache SQLite raises SQLITE_LOCKED immediately
		// when two pooled connections touch one table, which busy_timeout does not cover.
		// The background billing/logging tasks other tests spawn would otherwise flake
		// fixture resets with "database table is locked".
		sqlDB, err := db.DB()
		if err != nil {
			panic(err)
		}
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		sqlDB.SetConnMaxLifetime(0)

		if err := db.AutoMigrate(&model.Trace{}); err != nil {
			panic(err)
		}

		model.DB = db
	})

	if client.HTTPClient == nil {
		client.HTTPClient = &http.Client{}
	}
}

func setupResponseAPIContext(t *testing.T, method, target, baseURL string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Authorization", "Bearer upstream-key")
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	gmw.SetLogger(c, logger.Logger)

	c.Set(ctxkey.Channel, channeltype.OpenAI)
	c.Set(ctxkey.ChannelId, 42)
	c.Set(ctxkey.TokenId, 7)
	c.Set(ctxkey.TokenName, "test-token")
	c.Set(ctxkey.Id, 99)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.BaseURL, baseURL)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestModel, "gpt-4o-2024-08-06")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestId, "req-test")

	return c, recorder
}
