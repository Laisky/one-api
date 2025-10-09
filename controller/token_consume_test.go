package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
)

// setupConsumeTokenTest prepares an isolated in-memory database and test user/token for ConsumeToken tests.
func setupConsumeTokenTest(t *testing.T) (cleanup func(), user *model.User, token *model.Token) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:external_billing_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.TokenTransaction{}, &model.Log{}))

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	originalDB := model.DB
	originalLOG := model.LOG_DB
	model.DB = db
	model.LOG_DB = db

	originalUsingSQLite := common.UsingSQLite
	common.UsingSQLite = true

	originalRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)

	originalDefaultTimeout := config.ExternalBillingDefaultTimeoutSec
	originalMaxTimeout := config.ExternalBillingMaxTimeoutSec
	config.ExternalBillingDefaultTimeoutSec = 5
	config.ExternalBillingMaxTimeoutSec = 5

	user = &model.User{
		Id:       1,
		Username: "external-billing-user",
		Password: "password",
		Role:     model.RoleCommonUser,
		Status:   model.UserStatusEnabled,
		Quota:    1000,
	}
	require.NoError(t, model.DB.Create(user).Error)

	token = &model.Token{
		Id:           1,
		UserId:       user.Id,
		Key:          strings.Repeat("a", 48),
		Status:       model.TokenStatusEnabled,
		Name:         "test-token",
		RemainQuota:  1000,
		CreatedTime:  helper.GetTimestamp(),
		AccessedTime: helper.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(token).Error)

	cleanup = func() {
		model.DB = originalDB
		model.LOG_DB = originalLOG
		common.UsingSQLite = originalUsingSQLite
		common.SetRedisEnabled(originalRedis)
		config.ExternalBillingDefaultTimeoutSec = originalDefaultTimeout
		config.ExternalBillingMaxTimeoutSec = originalMaxTimeout
	}

	return cleanup, user, token
}

func newConsumeTokenContext(t *testing.T, method string, body string, userID, tokenID int, requestID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	req := httptest.NewRequest(method, "/api/token/consume", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	c.Set(ctxkey.Id, userID)
	c.Set(ctxkey.TokenId, tokenID)
	c.Set(helper.RequestIdKey, requestID)
	gmw.SetLogger(c, logger.Logger)

	return c, recorder
}

func TestConsumeTokenPreAndPostFlow(t *testing.T) {
	cleanup, user, token := setupConsumeTokenTest(t)
	defer cleanup()

	// Pre-consume request
	preBody := `{"phase":"pre","add_used_quota":100,"add_reason":"serviceA","timeout_seconds":30}`
	c, recorder := newConsumeTokenContext(t, http.MethodPost, preBody, user.Id, token.Id, "req-pre")

	ConsumeToken(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var preResp map[string]interface{}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &preResp))
	require.True(t, preResp["success"].(bool))

	data := preResp["data"].(map[string]interface{})
	require.Equal(t, float64(token.RemainQuota-100), data["remain_quota"])

	txnResp := preResp["transaction"].(map[string]interface{})
	require.Equal(t, "pending", txnResp["status"])
	transactionID := txnResp["transaction_id"].(string)
	require.NotEmpty(t, transactionID)

	txn, err := model.GetTokenTransactionByTokenAndID(context.Background(), token.Id, transactionID)
	require.NoError(t, err)
	require.Equal(t, model.TokenTransactionStatusPending, txn.Status)
	require.Nil(t, txn.FinalQuota)

	refreshedUser, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	require.Equal(t, int64(900), refreshedUser.Quota)

	// Post-consume request with adjusted amount
	postBody := fmt.Sprintf(`{"phase":"post","add_reason":"serviceA","transaction_id":"%s","final_used_quota":80}`, transactionID)
	c, recorder = newConsumeTokenContext(t, http.MethodPost, postBody, user.Id, token.Id, "req-post")

	ConsumeToken(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var postResp map[string]interface{}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &postResp))
	require.True(t, postResp["success"].(bool))

	postData := postResp["data"].(map[string]interface{})
	require.Equal(t, float64(token.RemainQuota-80), postData["remain_quota"])

	postTxnResp := postResp["transaction"].(map[string]interface{})
	require.Equal(t, "confirmed", postTxnResp["status"])
	require.EqualValues(t, 80, postTxnResp["final_quota"])

	txn, err = model.GetTokenTransactionByTokenAndID(context.Background(), token.Id, transactionID)
	require.NoError(t, err)
	require.Equal(t, model.TokenTransactionStatusConfirmed, txn.Status)
	require.NotNil(t, txn.FinalQuota)
	require.Equal(t, int64(80), *txn.FinalQuota)

	if txn.LogId != nil {
		var logEntry model.Log
		require.NoError(t, model.LOG_DB.First(&logEntry, *txn.LogId).Error)
		require.Equal(t, 80, logEntry.Quota)
		require.Contains(t, logEntry.Content, "finalized")
	}

	refreshedUser, err = model.GetUserById(user.Id, true)
	require.NoError(t, err)
	require.Equal(t, int64(920), refreshedUser.Quota)
}

func TestConsumeTokenCancelFlow(t *testing.T) {
	cleanup, user, token := setupConsumeTokenTest(t)
	defer cleanup()

	preBody := `{"phase":"pre","add_used_quota":60,"add_reason":"serviceB","timeout_seconds":30}`
	c, recorder := newConsumeTokenContext(t, http.MethodPost, preBody, user.Id, token.Id, "req-pre-cancel")
	ConsumeToken(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var preResp map[string]interface{}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &preResp))
	require.True(t, preResp["success"].(bool))
	txnResp := preResp["transaction"].(map[string]interface{})
	transactionID := txnResp["transaction_id"].(string)
	require.NotEmpty(t, transactionID)

	cancelBody := fmt.Sprintf(`{"phase":"cancel","add_reason":"serviceB","transaction_id":"%s"}`, transactionID)
	c, recorder = newConsumeTokenContext(t, http.MethodPost, cancelBody, user.Id, token.Id, "req-cancel")
	ConsumeToken(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var cancelResp map[string]interface{}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &cancelResp))
	require.True(t, cancelResp["success"].(bool))

	cancelTxn := cancelResp["transaction"].(map[string]interface{})
	require.Equal(t, "canceled", cancelTxn["status"])

	txn, err := model.GetTokenTransactionByTokenAndID(context.Background(), token.Id, transactionID)
	require.NoError(t, err)
	require.Equal(t, model.TokenTransactionStatusCanceled, txn.Status)

	refreshedUser, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	require.Equal(t, int64(1000), refreshedUser.Quota)

	if txn.LogId != nil {
		var logEntry model.Log
		require.NoError(t, model.LOG_DB.First(&logEntry, *txn.LogId).Error)
		require.Equal(t, 0, logEntry.Quota)
		require.Contains(t, logEntry.Content, "canceled")
	}
}

func TestConsumeTokenAutoConfirmTimeout(t *testing.T) {
	cleanup, user, token := setupConsumeTokenTest(t)
	defer cleanup()

	preBody := `{"phase":"pre","add_used_quota":40,"add_reason":"serviceC","timeout_seconds":1}`
	c, recorder := newConsumeTokenContext(t, http.MethodPost, preBody, user.Id, token.Id, "req-pre-auto")
	ConsumeToken(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var preResp map[string]interface{}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &preResp))
	require.True(t, preResp["success"].(bool))
	txnResp := preResp["transaction"].(map[string]interface{})
	transactionID := txnResp["transaction_id"].(string)
	require.NotEmpty(t, transactionID)

	time.Sleep(1100 * time.Millisecond)

	cAuto, _ := gin.CreateTestContext(httptest.NewRecorder())
	gmw.SetLogger(cAuto, logger.Logger)
	require.NoError(t, autoConfirmExpiredTokenTransactions(context.Background(), cAuto, token.Id))

	txn, err := model.GetTokenTransactionByTokenAndID(context.Background(), token.Id, transactionID)
	require.NoError(t, err)
	require.Equal(t, model.TokenTransactionStatusAutoConfirmed, txn.Status)
	require.NotNil(t, txn.FinalQuota)
	require.Equal(t, int64(40), *txn.FinalQuota)

	if txn.LogId != nil {
		var logEntry model.Log
		require.NoError(t, model.LOG_DB.First(&logEntry, *txn.LogId).Error)
		require.Contains(t, logEntry.Content, "auto-confirmed")
	}
}

func TestConsumeTokenDefaultsToSinglePhase(t *testing.T) {
	cleanup, user, token := setupConsumeTokenTest(t)
	defer cleanup()

	body := `{"add_used_quota":120,"add_reason":"serviceD"}`
	c, recorder := newConsumeTokenContext(t, http.MethodPost, body, user.Id, token.Id, "req-single")

	ConsumeToken(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp["success"].(bool))

	data := resp["data"].(map[string]interface{})
	require.Equal(t, float64(token.RemainQuota-120), data["remain_quota"])

	transaction := resp["transaction"].(map[string]interface{})
	require.Equal(t, "confirmed", transaction["status"])
	require.EqualValues(t, 120, transaction["final_quota"])

	refreshedToken, err := model.GetTokenByIds(token.Id, user.Id)
	require.NoError(t, err)
	require.Equal(t, int64(880), refreshedToken.RemainQuota)

	refreshedUser, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	require.Equal(t, int64(880), refreshedUser.Quota)
}
