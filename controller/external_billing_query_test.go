package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
)

func TestGetTokenBalance(t *testing.T) {
	cleanup, user, token := setupConsumeTokenTest(t)
	defer cleanup()

	c, recorder := newConsumeTokenContext(t, http.MethodGet, "", user.Id, token.Id, "req-balance")
	GetTokenBalance(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp["success"].(bool))

	data := resp["data"].(map[string]any)
	require.Equal(t, float64(token.RemainQuota), data["remain_quota"])
	require.Equal(t, float64(token.UsedQuota), data["used_quota"])
	require.Equal(t, token.UnlimitedQuota, data["unlimited_quota"])
}

func TestGetTokenTransactions(t *testing.T) {
	cleanup, user, token := setupConsumeTokenTest(t)
	defer cleanup()

	// 1. Create some transactions first
	for i := 0; i < 5; i++ {
		preBody := fmt.Sprintf(`{"phase":"single","add_used_quota":%d,"add_reason":"test-%d"}`, (i+1)*10, i)
		c, recorder := newConsumeTokenContext(t, http.MethodPost, preBody, user.Id, token.Id, fmt.Sprintf("req-%d", i))
		ConsumeToken(c)
		require.Equal(t, http.StatusOK, recorder.Code)
	}

	// 2. Query transactions
	c, recorder := newConsumeTokenContext(t, http.MethodGet, "", user.Id, token.Id, "req-txns")
	c.Request.URL.RawQuery = "p=0&size=10"
	GetTokenTransactions(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp["success"].(bool))

	data := resp["data"].([]any)
	require.Len(t, data, 5)
	require.Equal(t, float64(5), resp["total"])

	// Check order (should be desc by ID)
	first := data[0].(map[string]any)
	require.Equal(t, "test-4", first["reason"])
}

func TestGetTokenLogs(t *testing.T) {
	cleanup, user, token := setupConsumeTokenTest(t)
	defer cleanup()

	// 1. Record some logs
	for i := 0; i < 3; i++ {
		log := &model.Log{
			UserId:    user.Id,
			TokenName: token.Name,
			ModelName: "gpt-4",
			Quota:     100,
			Content:   fmt.Sprintf("test-log-%d", i),
		}
		model.RecordConsumeLog(context.Background(), log)
	}

	// 2. Query logs
	c, recorder := newConsumeTokenContext(t, http.MethodGet, "", user.Id, token.Id, "req-logs")
	c.Set(ctxkey.TokenName, token.Name)
	GetTokenLogs(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp["success"].(bool))

	data := resp["data"].([]any)
	require.Len(t, data, 3)
	require.Equal(t, float64(3), resp["total"])

	first := data[0].(map[string]any)
	require.Equal(t, "gpt-4", first["model_name"])
}
