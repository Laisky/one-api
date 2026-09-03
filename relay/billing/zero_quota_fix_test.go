package billing

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
)

// This file previously contained two tests with zero assertions: one wrapped the
// call in `defer recover()` and treated BOTH panic and no-panic as success, the
// other was three t.Log("✓ ...") lines describing what the code should do. The
// bug they were named for — "requests with 0 quota were not being logged" — was
// therefore not pinned at all: restoring the `if totalQuota != 0 { skip logging }`
// early return passed both. These replace them with assertions against a real DB.

// setupZeroQuotaBillingDB gives the billing package a real SQLite database with one
// user and one token, so a consume log can actually be observed.
//
// Parameters:
//   - t: the running test.
//
// Return values:
//   - func(): restores the previous globals.
func setupZeroQuotaBillingDB(t *testing.T) func() {
	t.Helper()

	dsn := fmt.Sprintf("file:billing_zero_quota_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}))

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	originalDB, originalLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db

	originalSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	originalRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)

	require.NoError(t, db.Create(&model.User{
		Id: 1, Username: "zero-quota-user", Password: "x",
		Role: model.RoleCommonUser, Status: model.UserStatusEnabled, Quota: 1_000_000,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		Id: 123, UserId: 1, Key: strings.Repeat("z", 48),
		Status: model.TokenStatusEnabled, Name: "zero-quota-token",
		RemainQuota: 1_000_000, CreatedTime: helper.GetTimestamp(), AccessedTime: helper.GetTimestamp(),
	}).Error)

	return func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		common.UsingSQLite.Store(originalSQLite)
		common.SetRedisEnabled(originalRedis)
	}
}

// TestPostConsumeQuotaWithLogRecordsZeroQuotaRequests pins the fix this file is
// named for: a request that costs nothing still has to leave an audit trail.
//
// A free model, a cached-only turn, or an upstream that reports no usage all
// produce totalQuota == 0. Skipping the log for those loses the record that the
// request happened at all.
func TestPostConsumeQuotaWithLogRecordsZeroQuotaRequests(t *testing.T) {
	defer setupZeroQuotaBillingDB(t)()

	PostConsumeQuotaWithLog(context.Background(), 123, 0, 0, &model.Log{
		UserId:    1,
		ChannelId: 5,
		ModelName: "free-model",
		TokenName: "zero-quota-token",
		Content:   "zero quota request",
		RequestId: "req_zero_quota",
	})
	require.NoError(t, graceful.Drain(context.Background()))

	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "req_zero_quota").Find(&logs).Error)
	require.Len(t, logs, 1, "a zero-quota request must still be logged")
	require.Equal(t, "free-model", logs[0].ModelName)
	require.Zero(t, logs[0].Quota)
}

// TestPostConsumeQuotaWithLogRejectsInvalidIdentifiers pins the input validation
// the old TestInputValidation only claimed to cover: its helpers could distinguish
// nothing but "panicked" from "did not panic", and its shouldFail field was
// inverted relative to its name.
func TestPostConsumeQuotaWithLogRejectsInvalidIdentifiers(t *testing.T) {
	defer setupZeroQuotaBillingDB(t)()

	for _, tc := range []struct {
		name      string
		tokenID   int
		userID    int
		channelID int
	}{
		{name: "token id is zero", tokenID: 0, userID: 1, channelID: 5},
		{name: "token id is negative", tokenID: -1, userID: 1, channelID: 5},
		{name: "user id is zero", tokenID: 123, userID: 0, channelID: 5},
		{name: "channel id is zero", tokenID: 123, userID: 1, channelID: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requestID := "req_invalid_" + tc.name

			require.NotPanics(t, func() {
				PostConsumeQuotaWithLog(context.Background(), tc.tokenID, 10, 10, &model.Log{
					UserId:    tc.userID,
					ChannelId: tc.channelID,
					ModelName: "test-model",
					TokenName: "zero-quota-token",
					RequestId: requestID,
				})
			})
			require.NoError(t, graceful.Drain(context.Background()))

			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Find(&logs).Error)
			require.Empty(t, logs, "an entry with an invalid identifier must not be billed or logged")
		})
	}
}

// TestPostConsumeQuotaWithLogIgnoresNilInputs keeps the nil guards honest.
func TestPostConsumeQuotaWithLogIgnoresNilInputs(t *testing.T) {
	defer setupZeroQuotaBillingDB(t)()

	require.NotPanics(t, func() {
		PostConsumeQuotaWithLog(context.Background(), 123, 10, 10, nil)
	})
	//nolint:staticcheck // deliberately passing a nil context to pin the guard
	require.NotPanics(t, func() {
		PostConsumeQuotaWithLog(nil, 123, 10, 10, &model.Log{UserId: 1, ChannelId: 5})
	})
}
