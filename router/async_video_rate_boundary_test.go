package router

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestAsyncVideoGlobalRateLimitCoversPrepaidReads verifies the limiter executes
// before replay and before distribution rewrites Authorization to an upstream
// key. A second client token is isolated, while same-token retrieval is counted.
func TestAsyncVideoGlobalRateLimitCoversPrepaidReads(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"cost":0.4,"currency":"USD"}`)
	}))
	defer provider.Close()
	_, key, userID, _, _ := disconnectBillingRouter(t, provider.URL)
	client.HTTPClient = provider.Client()
	oldNum, oldDuration := config.GlobalRelayRateLimitNum, config.GlobalRelayRateLimitDuration
	config.RateLimitDisabled = false
	config.GlobalRelayRateLimitNum = 1
	config.GlobalRelayRateLimitDuration = 60
	t.Cleanup(func() { config.GlobalRelayRateLimitNum = oldNum; config.GlobalRelayRateLimitDuration = oldDuration })
	second := strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, model.DB.Create(&model.Token{Id: 919194, UUID: uuid.NewString(), UserId: userID, Key: second,
		Status: model.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 200000}).Error)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { gmw.SetLogger(c, logger.Logger); c.Next() })
	SetRelayRouter(engine)
	request := func(key, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer sk-"+key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "one-task")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}
	created := request(key, http.MethodPost, "/v1/async/videos", `{"model":"veo3-fast","duration":5}`)
	require.Equal(t, http.StatusAccepted, created.Code, created.Body.String())
	var task struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &task))
	denied := request(key, http.MethodGet, "/v1/async/videos/"+task.ID, "")
	require.Equal(t, http.StatusTooManyRequests, denied.Code, denied.Body.String())
	require.NotEmpty(t, denied.Header().Get("Retry-After"))
	replay := request(key, http.MethodPost, "/v1/async/videos", `{"model":"veo3-fast","duration":5}`)
	require.Equal(t, http.StatusTooManyRequests, replay.Code, replay.Body.String())
	independent := request(second, http.MethodGet, "/v1/async/videos/"+task.ID, "")
	require.Equal(t, http.StatusOK, independent.Code, independent.Body.String())
	var count int64
	require.NoError(t, model.DB.Model(&model.AsyncTask{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	require.Zero(t, user.Quota, "only one reservation consumes the wallet")
}
