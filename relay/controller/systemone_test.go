package controller

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

const systemOneRequest = `{"model":"alias","state":{"id":9007199254740993},"questions":{"q":{"type":"noul","instructions":"Is this an object?"}}}`
const systemOneSuccess = `{"model":"jev-1.13.0","answers":{"q":{"type":"noul","noul":0.95}},"usage":{"input_tokens":312,"output_tokens":999999}}`

// systemOneContext prepares real SQLite balances and the distributor's request state.
func systemOneContext(t *testing.T, base string, balance int64, unlimited bool, group, price float64) (*gin.Context, *httptest.ResponseRecorder, string) {
	t.Helper()
	billingAccountingSetup(t, balance)
	oldBatch := config.BatchUpdateEnabled
	config.BatchUpdateEnabled = false
	t.Cleanup(func() { config.BatchUpdateEnabled = oldBatch })
	oldLogEnabled := config.LogConsumeEnabled.Load()
	config.SetLogConsumeEnabled(true)
	t.Cleanup(func() { config.SetLogConsumeEnabled(oldLogEnabled) })
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).Updates(map[string]any{"remain_quota": balance, "used_quota": 0, "unlimited_quota": unlimited}).Error)
	channel := &model.Channel{Id: fallbackChannelID, Type: channeltype.TypeSafe, Name: "typesafe-fixture", Key: "provider-fixture-key"}
	if price >= 0 {
		require.NoError(t, channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{"jev-latest": {Ratio: price, CompletionRatio: 99}}))
	}
	hash := sha256.Sum256([]byte(t.Name()))
	requestID := fmt.Sprintf("ts-%x", hash[:10])
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Delete(&model.Log{}).Error)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", strings.NewReader(systemOneRequest))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer provider-fixture-key")
	gmw.SetLogger(c, logger.Logger)
	for key, value := range map[string]any{
		ctxkey.Channel: channeltype.TypeSafe, ctxkey.ChannelId: fallbackChannelID, ctxkey.ChannelModel: channel,
		ctxkey.TokenId: fallbackTokenID, ctxkey.TokenName: "fallback-token", ctxkey.Id: fallbackUserID,
		ctxkey.Group: "default", ctxkey.ModelMapping: map[string]string{"alias": "jev-latest"}, ctxkey.ChannelRatio: group,
		ctxkey.RequestModel: "alias", ctxkey.BaseURL: base, ctxkey.ContentType: "application/json", ctxkey.RequestId: requestID,
		ctxkey.Username: "response-fallback", ctxkey.UserObj: &model.User{Id: fallbackUserID, Quota: balance},
		ctxkey.Config: model.ChannelConfig{}, ctxkey.TokenQuotaUnlimited: unlimited, ctxkey.TokenQuota: balance,
	} {
		c.Set(key, value)
	}
	return c, writer, requestID
}

// TestSystemOneBillingIntegration verifies real reservations, mapping, receipts,
// refunds, estimates and free output through the native HTTP handler.
func TestSystemOneBillingIntegration(t *testing.T) {
	for _, tc := range []struct {
		name, response             string
		upstreamStatus, wantStatus int
		unlimited                  bool
		group, price               float64
		charge                     int64
		estimated                  bool
	}{
		{"measured", systemOneSuccess, 200, 200, false, 1, -1, 7, false},
		{"unlimited", systemOneSuccess, 200, 200, true, 1, -1, 7, false},
		{"group", systemOneSuccess, 200, 200, false, 2, -1, 14, false},
		{"free_group", systemOneSuccess, 200, 200, false, 0, -1, 0, false},
		{"override_output_still_free", systemOneSuccess, 200, 200, false, 1, 1, 312, false},
		{"free_input", systemOneSuccess, 200, 200, false, 1, 0, 0, false},
		{"missing_receipt", `{"model":"jev-latest","answers":{}}`, 200, 502, false, 1, -1, 1377, true},
		{"invalid_answer_measured", `{"model":"jev-latest","answers":{},"usage":{"input_tokens":312,"output_tokens":1}}`, 200, 502, false, 1, -1, 7, false},
		{"auth_rejection", `{"detail":"unauthorized"}`, 401, 401, false, 1, -1, 0, false},
		{"validation_rejection", `{"detail":"invalid"}`, 422, 422, false, 1, -1, 0, false},
		{"rate_limit", `{"detail":"rate limit"}`, 429, 429, false, 1, -1, 0, false},
		{"capacity", `{"detail":"overloaded"}`, 529, 529, false, 1, -1, 0, false},
		{"ambiguous_server_error", `{"detail":"server error"}`, 500, 500, false, 1, -1, 1377, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(100000000)
			var calls atomic.Int32
			type observation struct {
				body       string
				userQuota  int64
				err        error
				path, auth string
			}
			seen := make(chan observation, 1)
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var user model.User
				err := model.DB.First(&user, fallbackUserID).Error
				body, readErr := io.ReadAll(r.Body)
				if err == nil {
					err = readErr
				}
				seen <- observation{string(body), user.Quota, err, r.URL.Path, r.Header.Get("Authorization")}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "10")
				w.WriteHeader(tc.upstreamStatus)
				_, _ = io.WriteString(w, tc.response)
			}))
			defer upstream.Close()
			previous := client.HTTPClient
			client.HTTPClient = upstream.Client()
			defer func() { client.HTTPClient = previous }()
			c, writer, requestID := systemOneContext(t, upstream.URL, balance, tc.unlimited, tc.group, tc.price)
			RelaySystemOne(c)
			drainCriticalTasks(t)
			require.Equal(t, tc.wantStatus, writer.Code, writer.Body.String())
			require.EqualValues(t, 1, calls.Load(), "no automatic replay")
			observed := <-seen
			require.NoError(t, observed.err)
			require.Equal(t, "/v1/systemone", observed.path)
			require.Equal(t, "Bearer provider-fixture-key", observed.auth)
			require.Contains(t, observed.body, "9007199254740993")
			var request map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(observed.body), &request))
			require.Equal(t, `"jev-latest"`, string(request["model"]))
			require.Equal(t, balance-c.GetInt64(ctxkey.PreConsumedQuotaAmount), observed.userQuota)
			require.Equal(t, balance-tc.charge, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			if tc.unlimited {
				require.Equal(t, balance, token.RemainQuota)
				require.Zero(t, token.UsedQuota)
			} else {
				require.Equal(t, balance-tc.charge, token.RemainQuota)
				require.Equal(t, tc.charge, token.UsedQuota)
			}
			require.Equal(t, tc.charge, requestCostQuota(t, requestID))
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.EqualValues(t, tc.charge, logs[0].Quota)
			require.Equal(t, tc.estimated, logs[0].Metadata["billing_estimated"] == true)
			require.Equal(t, tc.estimated, writer.Header().Get("X-OneAPI-Billing-Estimated") == "true")
			if tc.wantStatus == 200 {
				require.Equal(t, tc.response, writer.Body.String())
			}
			if tc.upstreamStatus >= 400 {
				require.Equal(t, "10", writer.Header().Get("Retry-After"))
			}
		})
	}
}

// TestSystemOneAdmissionRejectsBeforeNetwork checks balance and malformed payload gates.
func TestSystemOneAdmissionRejectsBeforeNetwork(t *testing.T) {
	for _, invalidBody := range []bool{false, true} {
		t.Run(fmt.Sprint(invalidBody), func(t *testing.T) {
			c, writer, _ := systemOneContext(t, "https://api.typesafe.ai", 1, false, 1, -1)
			want := http.StatusForbidden
			if invalidBody {
				c.Request.Body = io.NopCloser(strings.NewReader(`{"model":"alias","messages":[]}`))
				want = http.StatusBadRequest
			}
			RelaySystemOne(c)
			drainCriticalTasks(t)
			require.Equal(t, want, writer.Code)
			require.False(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
			require.EqualValues(t, 1, reloadUserQuota(t))
		})
	}
}
