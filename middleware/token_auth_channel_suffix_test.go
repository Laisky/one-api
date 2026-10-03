package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
)

// setupTokenAuthChannelSuffixTestDB creates a minimal database for TokenAuth channel suffix tests.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//
// Return values:
//   - *gorm.DB: in-memory SQLite database populated with one admin token and channel.
func setupTokenAuthChannelSuffixTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&dbmodel.User{}, &dbmodel.Token{}, &dbmodel.Channel{}))

	userUUID := "018f0000-0000-7000-8000-000000000201"
	require.NoError(t, db.Create(&dbmodel.User{
		Id:       1,
		UUID:     userUUID,
		Username: "admin",
		Password: "password-hash",
		Role:     dbmodel.RoleAdminUser,
		Status:   dbmodel.UserStatusEnabled,
		Group:    "default",
	}).Error)
	require.NoError(t, db.Create(&dbmodel.Token{
		Id:             1,
		UUID:           "018f0000-0000-7000-8000-000000000202",
		UserId:         1,
		UserUUID:       &userUUID,
		Key:            "admintoken",
		Status:         dbmodel.TokenStatusEnabled,
		Name:           "admin-token",
		ExpiredTime:    -1,
		RemainQuota:    1,
		UnlimitedQuota: true,
	}).Error)
	require.NoError(t, db.Create(&dbmodel.Channel{
		Id:     7,
		UUID:   "018f0000-0000-7000-8000-000000000207",
		Type:   1,
		Name:   "uuid-channel",
		Status: dbmodel.ChannelStatusEnabled,
		Models: "gpt-4o",
		Config: "{}",
	}).Error)
	return db
}

// TestTokenAuthSpecificChannelSuffixSupportsIntAndUUID verifies admin key suffix channel refs.
func TestTokenAuthSpecificChannelSuffixSupportsIntAndUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTokenAuthChannelSuffixTestDB(t)
	originalDB := dbmodel.DB
	originalSQLite := common.UsingSQLite.Load()
	originalPrefix := config.TokenKeyPrefix
	dbmodel.DB = db
	common.UsingSQLite.Store(true)
	config.TokenKeyPrefix = "sk-"
	t.Cleanup(func() {
		dbmodel.DB = originalDB
		common.UsingSQLite.Store(originalSQLite)
		config.TokenKeyPrefix = originalPrefix
	})

	for _, channelRef := range []string{"7", "018f0000-0000-7000-8000-000000000207"} {
		channelRef := channelRef
		t.Run(channelRef, func(t *testing.T) {
			engine := gin.New()
			engine.Use(TokenAuth())
			engine.GET("/v1/models", func(c *gin.Context) {
				require.Equal(t, 7, c.GetInt(ctxkey.SpecificChannelId))
				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil).WithContext(context.Background())
			req.Header.Set("Authorization", "Bearer sk-admintoken-"+channelRef)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusNoContent, recorder.Code)
		})
	}
}
