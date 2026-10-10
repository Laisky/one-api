package controller

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/billing"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

const incidentBodyResetBillingBalance = int64(1000)
const incidentBodyResetBillingHold = int64(38)

// incidentBodyResetBillingDrain joins tracked refund or settlement work before database assertions and restoration.
func incidentBodyResetBillingDrain(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(ctx))
}

// incidentBodyResetBillingFixture seeds a real physical reservation in private in-memory SQLite, without dispatch or live database initialization.
func incidentBodyResetBillingFixture(t *testing.T, forwarded bool) (*gin.Context, *meta.Meta, int) {
	t.Helper()
	incidentBodyResetBillingDrain(t)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}, &model.Channel{}))
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldSQLite, oldMySQL, oldPostgres := common.UsingSQLite.Load(), common.UsingMySQL.Load(), common.UsingPostgreSQL.Load()
	oldRedis, oldConsume, oldBatch := common.IsRedisEnabled(), config.IsLogConsumeEnabled(), config.BatchUpdateEnabled
	oldGin := gin.Mode()
	gin.SetMode(gin.TestMode)
	model.DB, model.LOG_DB = db, db
	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(false)
	common.SetRedisEnabled(false)
	config.SetLogConsumeEnabled(true)
	config.BatchUpdateEnabled = false
	t.Cleanup(func() {
		incidentBodyResetBillingDrain(t)
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.UsingSQLite.Store(oldSQLite)
		common.UsingMySQL.Store(oldMySQL)
		common.UsingPostgreSQL.Store(oldPostgres)
		common.SetRedisEnabled(oldRedis)
		config.SetLogConsumeEnabled(oldConsume)
		config.BatchUpdateEnabled = oldBatch
		gin.SetMode(oldGin)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.Create(&model.User{Id: 93001, Username: "incident-billing-owner", Quota: incidentBodyResetBillingBalance}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 93002, UserId: 93001, Name: "incident-billing-token", Key: "synthetic-billing-token", RemainQuota: incidentBodyResetBillingBalance}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 93003, Type: channeltype.DeepSeek, Name: "incident-billing-channel"}).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	gmw.SetLogger(c, logger.Logger)
	info := &meta.Meta{UserId: 93001, TokenId: 93002, ChannelId: 93003, ChannelType: channeltype.DeepSeek, ActualModelName: "deepseek-flash", OriginModelName: "deepseek-flash", TokenName: "incident-billing-token"}
	for key, value := range map[string]any{ctxkey.Id: info.UserId, ctxkey.TokenId: info.TokenId, ctxkey.Channel: info.ChannelType, ctxkey.ChannelId: info.ChannelId, ctxkey.Meta: info, ctxkey.RequestId: "incident-billing", ctxkey.UpstreamRequestPossiblyForwarded: forwarded} {
		c.Set(key, value)
	}
	require.NoError(t, model.PostConsumeTokenQuota(context.Background(), info.TokenId, incidentBodyResetBillingHold))
	markPreConsumed(c, incidentBodyResetBillingHold)
	provisionalID := recordProvisionalLog(c, info, info.ActualModelName, incidentBodyResetBillingHold)
	require.Positive(t, provisionalID)
	c.Set(ctxkey.ProvisionalLogId, provisionalID)
	return c, info, provisionalID
}

// incidentBodyResetBillingAssert checks both physical balances and the one synthetic accounting row.
func incidentBodyResetBillingAssert(t *testing.T, expectedDebit int64, provisionalID, expectedType int) model.Log {
	t.Helper()
	incidentBodyResetBillingDrain(t)
	var owner model.User
	var token model.Token
	require.NoError(t, model.DB.First(&owner, 93001).Error)
	require.NoError(t, model.DB.First(&token, 93002).Error)
	require.Equal(t, incidentBodyResetBillingBalance-expectedDebit, owner.Quota)
	require.Equal(t, owner.Quota, token.RemainQuota)
	require.Equal(t, expectedDebit, token.UsedQuota)
	var rows []model.Log
	require.NoError(t, model.LOG_DB.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, provisionalID, rows[0].Id)
	require.Equal(t, expectedType, rows[0].Type)
	require.EqualValues(t, expectedDebit, rows[0].Quota)
	return rows[0]
}

// TestIncidentBodyResetBillingRetainedReservation proves terminal cleanup retains exactly one uncertain hold while marking the lifecycle reconciled.
func TestIncidentBodyResetBillingRetainedReservation(t *testing.T) {
	c, info, provisionalID := incidentBodyResetBillingFixture(t, true)
	require.False(t, returnPreConsumedQuotaConservative(context.Background(), c, incidentBodyResetBillingHold, info.TokenId, "do_response_failed_without_usage"))
	require.True(t, c.GetBool(ctxkey.BillingReconciled), "the lifecycle flag is not proof of final ledger reconciliation")
	billingAuditSafetyNet(c)
	billingAuditSafetyNet(c)
	row := incidentBodyResetBillingAssert(t, incidentBodyResetBillingHold, provisionalID, model.LogTypeProvisional)
	require.Equal(t, true, row.Metadata[model.LogMetadataKeyProvisional])
	require.Contains(t, row.Content, "awaiting reconciliation")
}

// TestIncidentBodyResetBillingUnforwardedRefund verifies a proven undispatched request restores both balances once and completes its accounting row.
func TestIncidentBodyResetBillingUnforwardedRefund(t *testing.T) {
	c, info, provisionalID := incidentBodyResetBillingFixture(t, false)
	require.True(t, returnPreConsumedQuotaConservative(context.Background(), c, incidentBodyResetBillingHold, info.TokenId, "fixture_not_dispatched"))
	billingAuditSafetyNet(c)
	billingAuditSafetyNet(c)
	incidentBodyResetBillingAssert(t, 0, provisionalID, model.LogTypeConsume)
}

// TestIncidentBodyResetBillingSelectedRetryRefund verifies the actual reset helper returns a superseded reservation once and clears attempt markers.
func TestIncidentBodyResetBillingSelectedRetryRefund(t *testing.T) {
	c, info, provisionalID := incidentBodyResetBillingFixture(t, true)
	require.False(t, returnPreConsumedQuotaConservative(context.Background(), c, incidentBodyResetBillingHold, info.TokenId, "do_response_failed_without_usage"))
	require.True(t, BillingAllowsRetry(c))
	ResetPerAttemptBillingForRetry(context.Background(), c)
	ResetPerAttemptBillingForRetry(context.Background(), c) // Cleared state must not launch a duplicate refund.
	incidentBodyResetBillingAssert(t, 0, provisionalID, model.LogTypeConsume)
	require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
	require.False(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
	require.False(t, c.GetBool(ctxkey.BillingReconciled))
}

// TestIncidentBodyResetBillingSuccessfulReceipt verifies normal final settlement replaces the reservation with exactly one measured debit.
func TestIncidentBodyResetBillingSuccessfulReceipt(t *testing.T) {
	c, info, provisionalID := incidentBodyResetBillingFixture(t, true)
	const measured = int64(15)
	markBillingReconciled(c)
	billing.PostConsumeQuotaWithLog(context.Background(), info.TokenId, measured-incidentBodyResetBillingHold, measured,
		&model.Log{UserId: info.UserId, ChannelId: info.ChannelId, ModelName: info.ActualModelName, RequestId: "incident-billing", TokenName: info.TokenName, PromptTokens: 10, CompletionTokens: 5, Content: "synthetic measured receipt"}, provisionalID)
	billingAuditSafetyNet(c)
	incidentBodyResetBillingAssert(t, measured, provisionalID, model.LogTypeConsume)
}
