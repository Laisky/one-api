package controller

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

const incidentBodyResetBalance = int64(10000)
const incidentBodyResetModel = "deepseek-flash"

// incidentBodyResetReader returns the typed read failure after successful HTTP headers, without provider bytes.
type incidentBodyResetReader struct{}

// Read returns an injected TCP reset without accessing any network.
func (incidentBodyResetReader) Read([]byte) (int, error) {
	return 0, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
}

// Close releases the synthetic body, which has no external resources.
func (incidentBodyResetReader) Close() error { return nil }

// incidentBodyResetRoundTripper confines the registered adapter to synthetic HTTP responses.
type incidentBodyResetRoundTripper func(*http.Request) (*http.Response, error)

// RoundTrip executes the local fixture function without dialing.
func (f incidentBodyResetRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// incidentBodyResetDrain joins all tracked side effects before inspecting or restoring the isolated database.
func incidentBodyResetDrain(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(ctx))
}

// incidentBodyResetChannel constructs one synthetic DeepSeek channel with explicit deterministic pricing.
func incidentBodyResetChannel(id int, host string, priority int64) *dbmodel.Channel {
	base := "https://" + host
	pricing := `{"deepseek-flash":{"ratio":1,"completion_ratio":1}}`
	return &dbmodel.Channel{Id: id, Name: host, Type: channeltype.DeepSeek,
		Status: dbmodel.ChannelStatusEnabled, Models: incidentBodyResetModel, Group: "default",
		Priority: &priority, BaseURL: &base, ModelConfigs: &pricing, Key: "synthetic-incident-key"}
}

// incidentBodyResetRoutingFixture replaces database handles and global knobs exclusively inside this test process.
func incidentBodyResetRoutingFixture(t *testing.T, channels []*dbmodel.Channel, memory bool) {
	t.Helper()
	incidentBodyResetDrain(t)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&dbmodel.Channel{}, &dbmodel.Ability{}, &dbmodel.User{}, &dbmodel.Token{}, &dbmodel.Log{}, &dbmodel.UserRequestCost{}, &dbmodel.MCPServer{}, &dbmodel.MCPTool{}, &dbmodel.Trace{}))
	oldDB, oldLogDB := dbmodel.DB, dbmodel.LOG_DB
	oldSQLite, oldMySQL, oldPostgres := common.UsingSQLite.Load(), common.UsingMySQL.Load(), common.UsingPostgreSQL.Load()
	oldRetry, oldMemory, oldBatch, oldPre := config.RetryTimes, config.MemoryCacheEnabled, config.BatchUpdateEnabled, config.PreConsumedQuota
	oldRedis, oldConsume, oldApproximate := common.IsRedisEnabled(), config.IsLogConsumeEnabled(), config.ApproximateTokenEnabled
	oldClient, oldHelper, oldProcessor := client.HTTPClient, relayHelperForTest, processChannelRelayErrorForTest
	dbmodel.DB, dbmodel.LOG_DB = db, db
	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(false)
	config.RetryTimes, config.MemoryCacheEnabled, config.BatchUpdateEnabled, config.PreConsumedQuota = 2, memory, false, 100
	config.ApproximateTokenEnabled = true
	common.SetRedisEnabled(false)
	config.SetLogConsumeEnabled(true)
	relayHelperForTest = nil                                                                   // Exercise the registered adapter and real billing, not scripted relay outcomes.
	processChannelRelayErrorForTest = func(context.Context, processChannelRelayErrorParams) {} // Health policy is tested separately.
	t.Cleanup(func() {
		incidentBodyResetDrain(t)
		require.NoError(t, db.Exec("DELETE FROM abilities").Error)
		require.NoError(t, db.Exec("DELETE FROM channels").Error)
		dbmodel.InitChannelCache() // Clear fixture-only routing without querying a restored handle.
		dbmodel.DB, dbmodel.LOG_DB = oldDB, oldLogDB
		common.UsingSQLite.Store(oldSQLite)
		common.UsingMySQL.Store(oldMySQL)
		common.UsingPostgreSQL.Store(oldPostgres)
		config.RetryTimes, config.MemoryCacheEnabled, config.BatchUpdateEnabled, config.PreConsumedQuota = oldRetry, oldMemory, oldBatch, oldPre
		config.ApproximateTokenEnabled = oldApproximate
		common.SetRedisEnabled(oldRedis)
		config.SetLogConsumeEnabled(oldConsume)
		client.HTTPClient, relayHelperForTest, processChannelRelayErrorForTest = oldClient, oldHelper, oldProcessor
		require.NoError(t, sqlDB.Close())
	})
	for _, ch := range channels {
		require.NoError(t, db.Create(ch).Error)
		require.NoError(t, ch.AddAbilities())
	}
	dbmodel.InitChannelCache()
	require.NoError(t, db.Create(&dbmodel.User{Id: 92001, Username: "incident-fixture-owner", Quota: incidentBodyResetBalance, Status: dbmodel.UserStatusEnabled, Group: "default"}).Error)
	require.NoError(t, db.Create(&dbmodel.Token{Id: 92002, UserId: 92001, Name: "incident-fixture-token", Key: "synthetic-token-only", RemainQuota: incidentBodyResetBalance, Status: dbmodel.TokenStatusEnabled}).Error)
}

// TestIncidentBodyResetRetryRouting drives the real DeepSeek adapter and strict-priority retry loop using only local fake HTTP responses.
func TestIncidentBodyResetRetryRouting(t *testing.T) {
	for _, memory := range []bool{false, true} {
		for _, scenario := range []string{"no_alternative", "alternative_success", "initial_success"} {
			name := scenario + "/database"
			if memory {
				name = scenario + "/memory"
			}
			t.Run(name, func(t *testing.T) {
				previousGinMode := gin.Mode()
				gin.SetMode(gin.TestMode)
				t.Cleanup(func() { gin.SetMode(previousGinMode) })
				first := incidentBodyResetChannel(92003, "first.incident.invalid", 10)
				channels := []*dbmodel.Channel{first}
				if scenario == "alternative_success" {
					channels = append(channels, incidentBodyResetChannel(92004, "second.incident.invalid", 5))
				}
				incidentBodyResetRoutingFixture(t, channels, memory)
				core, logs := observer.New(zapcore.DebugLevel)
				lg, err := glog.New(glog.WithName("incident-body-reset"), glog.WithLevel(glog.LevelDebug), glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
				require.NoError(t, err)
				var firstCalls, secondCalls atomic.Int32
				var firstHold int64
				client.HTTPClient = &http.Client{Transport: incidentBodyResetRoundTripper(func(r *http.Request) (*http.Response, error) {
					require.Equal(t, http.MethodPost, r.Method)
					require.Equal(t, "/v1/chat/completions", r.URL.Path)
					_, err := io.Copy(io.Discard, r.Body)
					require.NoError(t, err)
					body := io.ReadCloser(incidentBodyResetReader{})
					switch r.URL.Host {
					case "first.incident.invalid":
						firstCalls.Add(1)
						var u dbmodel.User
						require.NoError(t, dbmodel.DB.First(&u, 92001).Error)
						firstHold = incidentBodyResetBalance - u.Quota
					case "second.incident.invalid":
						secondCalls.Add(1)
					default:
						t.Fatalf("unexpected fixture destination %q", r.URL.Host)
					}
					if scenario == "initial_success" || r.URL.Host == "second.incident.invalid" {
						body = io.NopCloser(strings.NewReader(`{"id":"fixture","object":"chat.completion","model":"deepseek-flash","choices":[{"index":0,"message":{"role":"assistant","content":"fixture answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
					}
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: body, Request: r}, nil
				})}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				gmw.SetLogger(c, lg)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-flash","stream":false,"max_tokens":1000,"messages":[{"role":"user","content":"synthetic incident fixture"}]}`))
				c.Request.Header.Set("Content-Type", "application/json")
				requestID := "incident-reset"
				for key, value := range map[string]any{ctxkey.Id: 92001, ctxkey.TokenId: 92002, ctxkey.TokenName: "incident-fixture-token", ctxkey.Group: "default", ctxkey.RequestModel: incidentBodyResetModel, ctxkey.RequestId: requestID, ctxkey.TokenQuota: incidentBodyResetBalance, ctxkey.TokenQuotaUnlimited: false, ctxkey.UserObj: &dbmodel.User{Id: 92001, Quota: incidentBodyResetBalance}, ctxkey.Username: "incident-fixture-owner"} {
					c.Set(key, value)
				}
				middleware.SetupContextForSelectedChannel(c, first, incidentBodyResetModel)
				Relay(c)
				incidentBodyResetDrain(t)
				require.EqualValues(t, 1, firstCalls.Load())
				require.Positive(t, firstHold, "a real reservation must precede dispatch")
				var u dbmodel.User
				var token dbmodel.Token
				require.NoError(t, dbmodel.DB.First(&u, 92001).Error)
				require.NoError(t, dbmodel.DB.First(&token, 92002).Error)
				require.Equal(t, u.Quota, token.RemainQuota)
				var rows []dbmodel.Log
				require.NoError(t, dbmodel.LOG_DB.Where("request_id = ?", requestID).Find(&rows).Error)
				if scenario == "no_alternative" {
					require.Equal(t, http.StatusInternalServerError, recorder.Code)
					require.Contains(t, recorder.Body.String(), "read_response_body_failed")
					require.Zero(t, secondCalls.Load())
					require.Equal(t, incidentBodyResetBalance-firstHold, u.Quota, "one uncertain hold, without duplicate dispatch or debit")
					require.Len(t, rows, 1)
					require.Equal(t, dbmodel.LogTypeProvisional, rows[0].Type)
					require.EqualValues(t, firstHold, rows[0].Quota)
					require.Equal(t, true, rows[0].Metadata[dbmodel.LogMetadataKeyProvisional])
					selection := logs.FilterMessage("relay retry exhausted: no alternative channel available").All()
					require.Len(t, selection, 1)
					require.Equal(t, "strict_priority", selection[0].ContextMap()["retry_selection_policy"])
					require.Empty(t, logs.FilterMessage("refunding abandoned attempt pre-consumed quota before cross-channel retry").All())
				} else {
					require.Equal(t, http.StatusOK, recorder.Code)
					require.Contains(t, recorder.Body.String(), "fixture answer")
					var cost dbmodel.UserRequestCost
					require.NoError(t, dbmodel.DB.Where("request_id = ?", requestID).First(&cost).Error)
					require.EqualValues(t, 15, cost.Quota, "the deterministic tariff bills only the successful receipt")
					require.Equal(t, incidentBodyResetBalance-cost.Quota, u.Quota, "failed-attempt hold must not double-charge a successful retry")
					if scenario == "alternative_success" {
						require.EqualValues(t, 1, secondCalls.Load())
						require.Len(t, rows, 2)
						require.Len(t, logs.FilterMessage("refunding abandoned attempt pre-consumed quota before cross-channel retry").All(), 1)
					} else {
						require.Zero(t, secondCalls.Load())
						require.Len(t, rows, 1)
					}
					for _, row := range rows {
						require.Equal(t, dbmodel.LogTypeConsume, row.Type)
					}
				}
			})
		}
	}
}
