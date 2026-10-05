package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// OCR allowance constants mirrored from the documented admission policy so the
// security tests state their expectations independently of production code.
const (
	ocrTestPageInputTokens  = 16_384
	ocrTestPageOutputTokens = 4_096
	ocrTestMaxPages         = 100
	ocrTestModel            = "glm-ocr"
	ocrTestLargeQuota       = int64(20_000_000)
	ocrTestSinglePageBody   = `{"model":"glm-ocr","file":"https://documents.example.test/scan.pdf","start_page_id":1,"end_page_id":1}`
)

// ocrTestReceipt returns a successful layout_parsing body carrying the given
// usage JSON (omitted when empty) and one processed page.
func ocrTestReceipt(usage string) string {
	if usage == "" {
		return `{"id":"ocr-fixture","model":"glm-ocr","md_results":"# page","data_info":{"num_pages":1}}`
	}
	return `{"id":"ocr-fixture","model":"glm-ocr","md_results":"# page","usage":` + usage + `,"data_info":{"num_pages":1}}`
}

// ocrSettlementCase describes one end-to-end native OCR request driven through
// RelayOCRHelper against an httptest upstream and the real SQLite ledger.
type ocrSettlementCase struct {
	requestID    string
	channelType  int
	configs      map[string]model.ModelConfigLocal
	groupRatio   float64
	payload      string
	upstreamBody string
	userQuota    int64
	tokenQuota   int64
	failDelivery bool
	// upstreamContentLength, when positive, overstates the upstream
	// Content-Length so the gateway observes a transport error after the body.
	upstreamContentLength int
}

// ocrSettlementResult captures every externally observable effect of one case.
type ocrSettlementResult struct {
	apiErr         *relaymodel.ErrorWithStatusCode
	dispatches     int32
	forwardedBody  string
	heldAtDispatch int64
	userDelta      int64
	tokenDelta     int64
	cost           int64
	costFound      bool
	logs           []model.Log
}

// ocrBrokenClientWriter simulates a client that disconnected after the provider
// accepted the work: headers are recorded but every body write fails.
type ocrBrokenClientWriter struct {
	*httptest.ResponseRecorder
}

// Write rejects the response body as a reset client connection would.
func (w ocrBrokenClientWriter) Write([]byte) (int, error) {
	return 0, errors.New("client connection reset by peer")
}

// runOCRSettlementCase executes tc through the production OCR controller and
// returns the upstream dispatch count, the hold observed at dispatch time, the
// final user/token ledger deltas, the recorded request cost and consume logs.
func runOCRSettlementCase(t *testing.T, tc ocrSettlementCase) ocrSettlementResult {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	oldRedis, oldLogs, oldClient := common.IsRedisEnabled(), config.IsLogConsumeEnabled(), client.HTTPClient
	common.SetRedisEnabled(false)
	config.SetLogConsumeEnabled(true)
	t.Cleanup(func() {
		drainCriticalTasks(t)
		common.SetRedisEnabled(oldRedis)
		config.SetLogConsumeEnabled(oldLogs)
		client.HTTPClient = oldClient
	})
	if tc.channelType == 0 {
		tc.channelType = channeltype.Zhipu
	}
	if tc.payload == "" {
		tc.payload = ocrTestSinglePageBody
	}
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", fallbackUserID).Update("quota", tc.userQuota).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).
		Updates(map[string]any{"unlimited_quota": false, "remain_quota": tc.tokenQuota}).Error)

	var dispatches atomic.Int32
	var held atomic.Int64
	var forwarded atomic.Value
	held.Store(-1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatches.Add(1)
		var body strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			body.Write(buf[:n])
			if err != nil {
				break
			}
		}
		forwarded.Store(body.String())
		var user model.User
		if err := model.DB.Select("quota").Where("id = ?", fallbackUserID).First(&user).Error; err == nil {
			held.Store(tc.userQuota - user.Quota)
		}
		w.Header().Set("Content-Type", "application/json")
		if tc.upstreamContentLength > 0 {
			w.Header().Set("Content-Length", strconv.Itoa(tc.upstreamContentLength))
		}
		_, _ = w.Write([]byte(tc.upstreamBody))
	}))
	t.Cleanup(upstream.Close)
	client.HTTPClient = upstream.Client()

	channel := &model.Channel{Id: fallbackChannelID, Type: tc.channelType}
	if tc.configs != nil {
		require.NoError(t, channel.SetModelPriceConfigs(tc.configs))
	}
	recorder := httptest.NewRecorder()
	var writer http.ResponseWriter = recorder
	if tc.failDelivery {
		writer = ocrBrokenClientWriter{ResponseRecorder: recorder}
	}
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/layout_parsing", strings.NewReader(tc.payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer fixture-upstream-key")
	gmw.SetLogger(c, logger.Logger)
	for key, value := range map[string]any{
		ctxkey.Channel: tc.channelType, ctxkey.ChannelId: fallbackChannelID, ctxkey.ChannelModel: channel,
		ctxkey.TokenId: fallbackTokenID, ctxkey.TokenName: "fallback-token", ctxkey.Id: fallbackUserID,
		ctxkey.Group: "default", ctxkey.ModelMapping: map[string]string{}, ctxkey.ChannelRatio: tc.groupRatio,
		ctxkey.RequestModel: ocrTestModel, ctxkey.BaseURL: upstream.URL, ctxkey.ContentType: "application/json",
		ctxkey.RequestId: tc.requestID, ctxkey.Username: "response-fallback",
		ctxkey.UserObj: &model.User{Id: fallbackUserID, Quota: tc.userQuota}, ctxkey.Config: model.ChannelConfig{},
		ctxkey.TokenQuotaUnlimited: false, ctxkey.TokenQuota: tc.tokenQuota,
	} {
		c.Set(key, value)
	}

	result := ocrSettlementResult{apiErr: RelayOCRHelper(c)}
	drainCriticalTasks(t)
	result.dispatches = dispatches.Load()
	result.heldAtDispatch = held.Load()
	if value, ok := forwarded.Load().(string); ok {
		result.forwardedBody = value
	}
	result.userDelta = tc.userQuota - reloadUserQuota(t)
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	result.tokenDelta = tc.tokenQuota - token.RemainQuota
	var cost model.UserRequestCost
	if err := model.DB.Where("request_id = ?", tc.requestID).First(&cost).Error; err == nil {
		result.cost, result.costFound = cost.Quota, true
	}
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", tc.requestID, model.LogTypeConsume).Find(&result.logs).Error)
	return result
}

// requireOCRSettledOnce asserts exactly one final ledger delta of want on both
// the user and token balances, a matching request cost, and a single log row.
func requireOCRSettledOnce(t *testing.T, res ocrSettlementResult, want int64) {
	t.Helper()
	require.Equal(t, int32(1), res.dispatches, "exactly one upstream dispatch")
	require.EqualValues(t, want, res.userDelta, "user ledger delta")
	require.EqualValues(t, want, res.tokenDelta, "token ledger delta")
	require.True(t, res.costFound, "request cost must be recorded")
	require.EqualValues(t, want, res.cost, "request cost")
	require.Len(t, res.logs, 1, "exactly one consume log row")
	require.EqualValues(t, want, res.logs[0].Quota, "consume log quota")
}

// requireOCREstimate asserts the single log row is explicitly labelled as a
// conservative estimate for reason rather than presented as measured usage.
func requireOCREstimate(t *testing.T, res ocrSettlementResult, reason string) {
	t.Helper()
	require.Len(t, res.logs, 1)
	metadata := res.logs[0].Metadata
	require.Equal(t, true, metadata["billing_estimated"], "estimate must be labelled")
	require.Equal(t, reason, metadata["billing_estimate_reason"])
}

// requireOCRMeasured asserts the single log row is not labelled as an estimate.
func requireOCRMeasured(t *testing.T, res ocrSettlementResult) {
	t.Helper()
	require.Len(t, res.logs, 1)
	_, estimated := res.logs[0].Metadata["billing_estimated"]
	require.False(t, estimated, "measured settlement must not be labelled as an estimate")
}

// requireOCRRejectedBeforeDispatch asserts admission failed with status, without
// contacting the provider and without any ledger movement or consume log.
func requireOCRRejectedBeforeDispatch(t *testing.T, res ocrSettlementResult, status int) {
	t.Helper()
	require.NotNil(t, res.apiErr, "request must be rejected")
	require.Equal(t, status, res.apiErr.StatusCode)
	require.Zero(t, res.dispatches, "provider must not be contacted")
	require.Zero(t, res.userDelta, "user balance must not move")
	require.Zero(t, res.tokenDelta, "token balance must not move")
	require.Empty(t, res.logs, "no consume log for rejected admission")
}
