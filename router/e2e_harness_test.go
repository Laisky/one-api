package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/pricing"
)

const (
	// e2eUserID and e2eTokenID identify the single wallet of every E2E fixture.
	e2eUserID  = 939391
	e2eTokenID = 939392
	// e2eBalance is the starting user and token quota. It is far above every
	// reservation in the suite, so no case is refused for insufficient quota.
	e2eBalance = int64(50_000_000)
	// e2eNativeOpenAIBase is the base URL of every native OpenAI channel. It is
	// what production uses, so supportsNativeResponseAPI selects the native
	// Responses route; the fake upstream's dialer serves every host locally.
	e2eNativeOpenAIBase = "https://api.openai.com"
	// e2eAzureBase is the base URL of every Azure channel.
	e2eAzureBase = "https://e2e-fixture.openai.azure.com"
	// e2eAzureConfig is the channel config every Azure fixture stores.
	e2eAzureConfig = `{"api_version":"2025-04-01-preview"}`
	// e2eAIProxyBase is the base URL of every AIProxy library channel.
	e2eAIProxyBase = "https://api.aiproxy.io"
	// e2eAIProxyConfig is the channel config every AIProxy library fixture stores.
	e2eAIProxyConfig = `{"library_id":"e2e-library"}`
)

// e2eChannel describes one channel row of an E2E fixture. Key doubles as the
// credential the fake upstream uses to attribute each call to its channel.
type e2eChannel struct {
	ID           int
	Type         int
	Key          string
	Priority     int64
	BaseURL      string
	Models       string
	Config       string
	ModelConfigs string
}

// e2eEnv is one isolated gateway: an in-memory SQLite ledger, the shipped relay
// router with real token auth and distributor, and a fake upstream.
type e2eEnv struct {
	engine   *gin.Engine
	key      string
	upstream *e2eUpstream
}

// e2eResult is what the client observed for one request.
type e2eResult struct {
	Code      int
	Body      string
	Header    http.Header
	RequestID string
}

// e2eLedger is a snapshot of the wallet balances.
type e2eLedger struct {
	UserQuota    int64
	UserUsed     int64
	RequestCount int
	TokenRemain  int64
	TokenUsed    int64
}

// newE2EEnv builds an isolated gateway. Parameters: t owns cleanup, respond
// scripts the fake upstream, and channels are created with their abilities.
// It sets RETRY_TIMES=1 and restores every global it changes. Returns: the env.
func newE2EEnv(t *testing.T, respond e2eResponder, channels ...e2eChannel) *e2eEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	// One connection keeps the in-memory database shared and serializes writers.
	conn.SetMaxOpenConns(1)

	oldDB, oldLOG, oldClient := model.DB, model.LOG_DB, client.HTTPClient
	oldRedis, oldSQLite, oldMemory, oldRate := common.IsRedisEnabled(), common.UsingSQLite.Load(), config.MemoryCacheEnabled, config.RateLimitDisabled
	oldLogging, oldRetry, oldBatch := config.IsLogConsumeEnabled(), config.RetryTimes, config.BatchUpdateEnabled
	model.DB, model.LOG_DB = db, db
	common.SetRedisEnabled(false)
	common.UsingSQLite.Store(true)
	config.MemoryCacheEnabled = false
	config.RateLimitDisabled = true
	config.SetLogConsumeEnabled(true)
	config.BatchUpdateEnabled = false
	// The shipped default is 0; every failover case in this suite needs one retry.
	config.RetryTimes = 1
	e2eUseProductionGlobalPricing(t)
	t.Cleanup(func() {
		drainCriticalTasksForRouter(t)
		model.DB, model.LOG_DB, client.HTTPClient = oldDB, oldLOG, oldClient
		common.SetRedisEnabled(oldRedis)
		common.UsingSQLite.Store(oldSQLite)
		config.MemoryCacheEnabled = oldMemory
		config.RateLimitDisabled = oldRate
		config.SetLogConsumeEnabled(oldLogging)
		config.RetryTimes = oldRetry
		config.BatchUpdateEnabled = oldBatch
		require.NoError(t, conn.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.ChannelGroup{}, &model.Ability{},
		&model.UserRequestCost{}, &model.QuotaRefund{}, &model.Log{}, &model.Trace{}, &model.MCPServer{}, &model.MCPTool{}))

	user := &model.User{Id: e2eUserID, UUID: uuid.NewString(), Username: "e2e-owner", Status: model.UserStatusEnabled, Quota: e2eBalance, Group: "default"}
	key := strings.ReplaceAll(uuid.NewString(), "-", "")
	token := &model.Token{Id: e2eTokenID, UUID: uuid.NewString(), UserId: user.Id, Name: "e2e-token", Key: key, Status: model.TokenStatusEnabled, RemainQuota: e2eBalance, ExpiredTime: -1}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(token).Error)
	for _, spec := range channels {
		base, priority := spec.BaseURL, spec.Priority
		row := &model.Channel{Id: spec.ID, UUID: uuid.NewString(), Type: spec.Type, Status: model.ChannelStatusEnabled, Name: spec.Key,
			Models: spec.Models, Group: "default", BaseURL: &base, Key: spec.Key, Priority: &priority, Config: spec.Config}
		if spec.ModelConfigs != "" {
			configs := spec.ModelConfigs
			row.ModelConfigs = &configs
		}
		require.NoError(t, db.Create(row).Error)
		require.NoError(t, row.AddAbilities())
	}

	env := &e2eEnv{key: key, upstream: newE2EUpstream(t, respond)}
	env.engine = gin.New()
	env.engine.Use(func(c *gin.Context) {
		gmw.SetLogger(c, logger.Logger)
		c.Next()
	})
	env.engine.Use(middleware.RequestId())
	SetRelayRouter(env.engine)
	return env
}

// e2eUseProductionGlobalPricing loads the global pricing layer exactly as
// main.go does, so a model borrowing a tariff from another provider would be
// visible. Parameters: t owns cleanup, which restores the previous adapters.
func e2eUseProductionGlobalPricing(t *testing.T) {
	t.Helper()
	wasInitialized := pricing.IsGlobalPricingInitialized()
	previousAdapters := pricing.GetContributingAdapters()
	relay.InitializeGlobalPricing()
	pricing.ReloadDefaultConfiguration()
	pricing.ReloadGlobalPricing()
	t.Cleanup(func() {
		// GetContributingAdapters preserves the unset (nil) state, so this
		// restores exactly what the suite found.
		pricing.SetContributingAdapters(previousAdapters)
		pricing.ReloadGlobalPricing()
		if !wasInitialized {
			pricing.InitializeGlobalPricingManager(nil)
		}
	})
}

// do sends one authenticated request through the shipped router and waits for
// detached billing. Parameters: t owns assertions, method/path/body form the
// request and header adds extra request headers. Returns: the client result.
func (e *e2eEnv) do(t *testing.T, method, path, body string, header http.Header) e2eResult {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer sk-"+e.key)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, values := range header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	w := httptest.NewRecorder()
	e.engine.ServeHTTP(w, req)
	drainCriticalTasksForRouter(t)
	return e2eResult{Code: w.Code, Body: w.Body.String(), Header: w.Header(), RequestID: w.Header().Get(helper.RequestIdKey)}
}

// post sends one authenticated JSON POST. Parameters: t owns assertions and
// path/body form the request. Returns: the client result.
func (e *e2eEnv) post(t *testing.T, path, body string) e2eResult {
	t.Helper()
	return e.do(t, http.MethodPost, path, body, nil)
}

// ledger returns the current wallet balances. Parameters: t owns assertions.
func (e *e2eEnv) ledger(t *testing.T) e2eLedger {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Where("id = ?", e2eUserID).Take(&user).Error)
	var token model.Token
	require.NoError(t, model.DB.Where("id = ?", e2eTokenID).Take(&token).Error)
	return e2eLedger{UserQuota: user.Quota, UserUsed: user.UsedQuota, RequestCount: user.RequestCount, TokenRemain: token.RemainQuota, TokenUsed: token.UsedQuota}
}

// logs returns every log row of requestID. Parameters: t owns assertions.
func (e *e2eEnv) logs(t *testing.T, requestID string) []model.Log {
	t.Helper()
	require.NotEmpty(t, requestID, "the gateway must return a request id")
	var rows []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Order("id").Find(&rows).Error)
	return rows
}

// requestCost returns the user_request_costs quota of requestID and whether a
// row exists. Parameters: t owns assertions.
func (e *e2eEnv) requestCost(t *testing.T, requestID string) (int64, bool) {
	t.Helper()
	var rows []model.UserRequestCost
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Find(&rows).Error)
	require.LessOrEqual(t, len(rows), 1, "request_id is unique in user_request_costs")
	if len(rows) == 0 {
		return 0, false
	}
	return rows[0].Quota, true
}

// e2eEstimateReason returns the billing_estimate_reason metadata of a log row.
func e2eEstimateReason(row model.Log) string {
	reason, _ := row.Metadata["billing_estimate_reason"].(string)
	return reason
}

// requireSettled asserts that one request was charged exactly charge on every
// ledger surface: user quota and used quota, token remain and used quota, a
// single consume log, no lingering provisional row, and the request cost.
// Parameters: t owns assertions, res is the client result, before the ledger
// snapshot taken before the request, and estimated whether the consume log
// must carry a billing_estimate_reason. Returns: the consume log row.
func (e *e2eEnv) requireSettled(t *testing.T, res e2eResult, before e2eLedger, charge int64, estimated bool) model.Log {
	t.Helper()
	after := e.ledger(t)
	require.Equal(t, charge, before.UserQuota-after.UserQuota, "user quota delta; body=%s", res.Body)
	require.Equal(t, charge, after.UserUsed-before.UserUsed, "user used_quota delta")
	require.Equal(t, charge, before.TokenRemain-after.TokenRemain, "token remain_quota delta")
	require.Equal(t, charge, after.TokenUsed-before.TokenUsed, "token used_quota delta")
	rows := e.logs(t, res.RequestID)
	var consume []model.Log
	for _, row := range rows {
		switch {
		case row.Type == model.LogTypeConsume && row.Quota != 0:
			consume = append(consume, row)
		case row.Type == model.LogTypeConsume:
			// A refunded attempt (failed over, or refused locally after its
			// reservation) has its provisional row voided in place.
			require.True(t, strings.HasPrefix(row.Content, "refunded:"), "a zero consume row must be a voided, refunded attempt: %+v", row)
		case row.Type == model.LogTypeProvisional:
			require.Zero(t, row.Quota, "a provisional row must not retain an unreconciled charge")
		}
	}
	require.Len(t, consume, 1, "exactly one charged consume log per request; rows=%+v", rows)
	require.EqualValues(t, charge, consume[0].Quota, "consume log quota")
	reason := e2eEstimateReason(consume[0])
	if estimated {
		require.NotEmpty(t, reason, "an estimated charge must record billing_estimate_reason")
	} else {
		require.Empty(t, reason, "an exact receipt must not be recorded as estimated")
	}
	cost, ok := e.requestCost(t, res.RequestID)
	require.True(t, ok, "a charged request must have a user_request_costs row")
	require.Equal(t, charge, cost, "user_request_costs quota")
	return consume[0]
}

// requireNothingCharged asserts that a request left every ledger surface
// unchanged: balances, no charged log row and a zero (or absent) request cost.
// Parameters: t owns assertions, res is the client result and before the
// ledger snapshot taken before the request.
func (e *e2eEnv) requireNothingCharged(t *testing.T, res e2eResult, before e2eLedger) {
	t.Helper()
	after := e.ledger(t)
	require.Equal(t, before.UserQuota, after.UserQuota, "user quota must be unchanged; body=%s", res.Body)
	require.Equal(t, before.UserUsed, after.UserUsed, "user used_quota must be unchanged")
	require.Equal(t, before.TokenRemain, after.TokenRemain, "token remain_quota must be unchanged")
	require.Equal(t, before.TokenUsed, after.TokenUsed, "token used_quota must be unchanged")
	if res.RequestID != "" {
		for _, row := range e.logs(t, res.RequestID) {
			require.Zero(t, row.Quota, "no log row may retain a charge: %+v", row)
		}
		cost, _ := e.requestCost(t, res.RequestID)
		require.Zero(t, cost, "the request cost must be zero or absent")
	}
}
