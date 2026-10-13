package controller_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
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
	entrycontroller "github.com/Laisky/one-api/controller"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

const billingTransportAuditBalance = int64(10000)
const billingTransportAuditUser = 95001
const billingTransportAuditToken = 95002
const billingTransportAuditFirst = 95003
const billingTransportAuditSecond = 95004
const billingTransportAuditModel = "deepseek-flash"
const billingTransportAuditRequest = "billing-transport-fixture"
const billingTransportAuditSuccess = `{"id":"fixture","object":"chat.completion","model":"deepseek-flash","choices":[{"index":0,"message":{"role":"assistant","content":"fixture answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`

// billingTransportAuditOpaque models an opaque transport whose error shape alone cannot prove non-forwarding.
type billingTransportAuditOpaque struct{ calls atomic.Int32 }

// RoundTrip returns a synthetic dial-shaped error without asserting the transport's forwarding semantics.
func (r *billingTransportAuditOpaque) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls.Add(1)
	return nil, billingTransportAuditDialError()
}

// billingTransportAuditDialError constructs a connection-refused cause for a fixture-only failed dial.
func billingTransportAuditDialError() error {
	return &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
}

// billingTransportAuditDrain joins tracked billing/error work before reading or restoring fixture state.
func billingTransportAuditDrain(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(ctx))
}

// billingTransportAuditChannel creates a synthetic DeepSeek route with a deterministic one-unit tariff.
func billingTransportAuditChannel(id int, base string, priority int64) *model.Channel {
	pricing := `{"deepseek-flash":{"ratio":1,"completion_ratio":1}}`
	return &model.Channel{Id: id, Name: "billing-transport-fixture", Type: channeltype.DeepSeek,
		Status: model.ChannelStatusEnabled, Models: billingTransportAuditModel, Group: "default",
		Priority: &priority, BaseURL: &base, ModelConfigs: &pricing, Key: "synthetic-fixture-key"}
}

// billingTransportAuditFixture adapts the existing incident SQLite fixture to the real outer Relay without internal test seams.
func billingTransportAuditFixture(t *testing.T, first *model.Channel, second *model.Channel) (*gin.Context, *httptest.ResponseRecorder, *observer.ObservedLogs) {
	t.Helper()
	billingTransportAuditDrain(t)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.Log{}, &model.UserRequestCost{}, &model.QuotaRefund{}, &model.MCPServer{}, &model.MCPTool{}, &model.Trace{}))
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldSQLite, oldMySQL, oldPostgres := common.UsingSQLite.Load(), common.UsingMySQL.Load(), common.UsingPostgreSQL.Load()
	oldRedis, oldConsume := common.IsRedisEnabled(), config.IsLogConsumeEnabled()
	oldBatch, oldMemory, oldApproximate, oldMetric := config.BatchUpdateEnabled, config.MemoryCacheEnabled, config.ApproximateTokenEnabled, config.EnableMetric
	oldRetry, oldPre, oldClient, oldGin := config.RetryTimes, config.PreConsumedQuota, client.HTTPClient, gin.Mode()
	model.DB, model.LOG_DB = db, db
	common.UsingSQLite.Store(true)
	common.UsingMySQL.Store(false)
	common.UsingPostgreSQL.Store(false)
	common.SetRedisEnabled(false)
	config.SetLogConsumeEnabled(true)
	config.BatchUpdateEnabled, config.MemoryCacheEnabled, config.ApproximateTokenEnabled, config.EnableMetric = false, false, true, false
	config.RetryTimes, config.PreConsumedQuota = 2, 100
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		billingTransportAuditDrain(t)
		require.NoError(t, db.Exec("DELETE FROM abilities").Error)
		require.NoError(t, db.Exec("DELETE FROM channels").Error)
		model.InitChannelCache()
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.UsingSQLite.Store(oldSQLite)
		common.UsingMySQL.Store(oldMySQL)
		common.UsingPostgreSQL.Store(oldPostgres)
		common.SetRedisEnabled(oldRedis)
		config.SetLogConsumeEnabled(oldConsume)
		config.BatchUpdateEnabled, config.MemoryCacheEnabled, config.ApproximateTokenEnabled, config.EnableMetric = oldBatch, oldMemory, oldApproximate, oldMetric
		config.RetryTimes, config.PreConsumedQuota, client.HTTPClient = oldRetry, oldPre, oldClient
		gin.SetMode(oldGin)
		require.NoError(t, sqlDB.Close())
	})
	for _, channel := range []*model.Channel{first, second} {
		if channel == nil {
			continue
		}
		require.NoError(t, db.Create(channel).Error)
		require.NoError(t, channel.AddAbilities())
	}
	model.InitChannelCache()
	require.NoError(t, db.Create(&model.User{Id: billingTransportAuditUser, Username: "billing-transport-owner", Quota: billingTransportAuditBalance, Status: model.UserStatusEnabled, Group: "default"}).Error)
	require.NoError(t, db.Create(&model.Token{Id: billingTransportAuditToken, UserId: billingTransportAuditUser, Name: "billing-transport-token", Key: "synthetic-transport-token", RemainQuota: billingTransportAuditBalance, Status: model.TokenStatusEnabled}).Error)
	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.New(glog.WithName("billing-transport-audit"), glog.WithLevel(glog.LevelDebug), glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	gmw.SetLogger(c, lg)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-flash","stream":false,"max_tokens":1000,"messages":[{"role":"user","content":"synthetic billing transport fixture"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	for key, value := range map[string]any{ctxkey.Id: billingTransportAuditUser, ctxkey.TokenId: billingTransportAuditToken, ctxkey.TokenName: "billing-transport-token", ctxkey.Group: "default", ctxkey.RequestModel: billingTransportAuditModel, ctxkey.RequestId: billingTransportAuditRequest, ctxkey.TokenQuota: billingTransportAuditBalance, ctxkey.TokenQuotaUnlimited: false, ctxkey.UserObj: &model.User{Id: billingTransportAuditUser, Quota: billingTransportAuditBalance}, ctxkey.Username: "billing-transport-owner"} {
		c.Set(key, value)
	}
	middleware.SetupContextForSelectedChannel(c, first, billingTransportAuditModel)
	return c, recorder, logs
}

// billingTransportAuditAssert checks physical conservation and request-row reconciliation after real Relay work drains.
func billingTransportAuditAssert(t *testing.T, logs *observer.ObservedLogs, debit int64, rowCount int, retained, auditExpected bool) {
	t.Helper()
	billingTransportAuditDrain(t)
	var user model.User
	var token model.Token
	require.NoError(t, model.DB.First(&user, billingTransportAuditUser).Error)
	require.NoError(t, model.DB.First(&token, billingTransportAuditToken).Error)
	require.Equal(t, billingTransportAuditBalance-debit, user.Quota, "the user must have exactly one net accounting effect")
	require.Equal(t, user.Quota, token.RemainQuota)
	require.Equal(t, debit, token.UsedQuota, "a refund must not over-credit or leave a duplicate debit")
	var rows []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", billingTransportAuditRequest).Order("id").Find(&rows).Error)
	require.Len(t, rows, rowCount)
	var total int64
	for _, row := range rows {
		total += int64(row.Quota)
		if retained {
			require.Equal(t, model.LogTypeProvisional, row.Type)
			require.Equal(t, true, row.Metadata[model.LogMetadataKeyProvisional])
		} else {
			require.Equal(t, model.LogTypeConsume, row.Type, "proven unforwarded attempts must not leave a stale provisional row")
			require.NotEqual(t, true, row.Metadata[model.LogMetadataKeyProvisional])
		}
	}
	require.Equal(t, debit, total)
	audit := logs.FilterMessage("CRITICAL BILLING AUDIT: pre-consumed quota was not reconciled (no post-billing or refund)").All()
	if auditExpected {
		require.NotEmpty(t, audit, "ambiguous attempts must retain their existing audit signal")
	} else {
		require.Empty(t, audit)
	}
}

// billingTransportAuditHold reads the recorded admission estimate without substituting a context forwarding flag for evidence.
func billingTransportAuditHold(t *testing.T, logs *observer.ObservedLogs) int64 {
	t.Helper()
	entries := logs.FilterMessage("recorded provisional consume log").All()
	require.NotEmpty(t, entries, "the real controller must reserve and record quota before transport")
	hold, ok := entries[0].ContextMap()["estimated_quota"].(int64)
	require.True(t, ok)
	require.Positive(t, hold)
	return hold
}

// billingTransportAuditServer supplies measured receipts or proves upstream request receipt before an abrupt close or redirect.
func billingTransportAuditServer(t *testing.T, mode string, calls, bytes *atomic.Int64) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		n, err := io.Copy(io.Discard, r.Body)
		bytes.Add(n)
		if err != nil {
			t.Errorf("read synthetic upstream body: %v", err)
			return
		}
		if mode == "close_after_read" {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack fixture connection: %v", err)
				return
			}
			if err := conn.Close(); err != nil {
				t.Errorf("close fixture connection: %v", err)
			}
			return
		}
		if mode == "redirect_after_read" {
			w.Header().Set("Location", "http://billing-redirect-refused.invalid/v1/chat/completions")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, billingTransportAuditSuccess); err != nil {
			t.Errorf("write synthetic receipt: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// billingTransportAuditTrace attaches observation to the incoming request; the production helper must propagate that context.
func billingTransportAuditTrace(c *gin.Context, getConn, gotConn, wrote *atomic.Int64) {
	trace := &httptrace.ClientTrace{GetConn: func(string) { getConn.Add(1) }, GotConn: func(httptrace.GotConnInfo) { gotConn.Add(1) }, WroteRequest: func(info httptrace.WroteRequestInfo) {
		if info.Err == nil {
			wrote.Add(1)
		}
	}}
	c.Request = c.Request.WithContext(httptrace.WithClientTrace(c.Request.Context(), trace))
}

// billingTransportAuditHTTPClient uses an actual transport with a custom dialer and confines successful dials to loopback fixtures.
// A custom dialer remains opaque to production, even when this fixture proves that it returned no connection.
func billingTransportAuditHTTPClient(t *testing.T, allowed []string, refused string, dialCalls *atomic.Int64) {
	t.Helper()
	transport := &http.Transport{Proxy: nil, TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if address == refused {
			dialCalls.Add(1)
			return nil, billingTransportAuditDialError() // A nil connection makes an HTTP write physically impossible.
		}
		for _, addressAllowed := range allowed {
			if address == addressAllowed {
				return (&net.Dialer{}).DialContext(ctx, network, address)
			}
		}
		t.Errorf("unexpected fixture dial destination %q", address)
		return nil, billingTransportAuditDialError()
	}}
	client.HTTPClient = &http.Client{Transport: transport, Timeout: 5 * time.Second}
	t.Cleanup(transport.CloseIdleConnections)
}

// TestBillingTransportAuditIntegration exercises actual DeepSeek dispatch, strict-priority retries and physical quota accounting.
func TestBillingTransportAuditIntegration(t *testing.T) {
	for _, kind := range []string{"invalid_header", "invalid_url", "custom_dial_refused", "after_write_close", "success"} {
		for _, retry := range []bool{false, true} {
			if kind == "success" && retry {
				continue
			}
			name := kind + "/terminal"
			if retry {
				name = kind + "/selected_retry"
			}
			t.Run(name, func(t *testing.T) {
				var received, bodyBytes, secondReceived, secondBytes, refused, getConn, gotConn, wrote atomic.Int64
				mode := "success"
				if kind == "after_write_close" {
					mode = "close_after_read"
				}
				firstServer := billingTransportAuditServer(t, mode, &received, &bodyBytes)
				base := firstServer.URL
				if kind == "invalid_url" {
					base = "http://%zz"
				}
				if kind == "custom_dial_refused" {
					base = "http://billing-prewrite-refused.invalid"
				}
				first := billingTransportAuditChannel(billingTransportAuditFirst, base, 10)
				if kind == "invalid_header" {
					encoded, err := json.Marshal(model.ChannelConfig{CustomHeaders: map[string]string{"bad\nname": "synthetic"}})
					require.NoError(t, err)
					first.Config = string(encoded)
				}
				var second *model.Channel
				allowed := []string{firstServer.Listener.Addr().String()}
				if retry {
					server := billingTransportAuditServer(t, "success", &secondReceived, &secondBytes)
					second = billingTransportAuditChannel(billingTransportAuditSecond, server.URL, 5)
					allowed = append(allowed, server.Listener.Addr().String())
				}
				c, response, logs := billingTransportAuditFixture(t, first, second)
				billingTransportAuditTrace(c, &getConn, &gotConn, &wrote)
				billingTransportAuditHTTPClient(t, allowed, "billing-prewrite-refused.invalid:80", &refused)
				entrycontroller.Relay(c)
				billingTransportAuditDrain(t)
				hold := billingTransportAuditHold(t, logs)
				if kind == "invalid_header" || kind == "invalid_url" {
					require.Zero(t, received.Load())
					require.Zero(t, refused.Load())
				}
				if kind == "custom_dial_refused" {
					require.EqualValues(t, 1, refused.Load())
					require.Zero(t, received.Load())
					require.Positive(t, getConn.Load())
					if !retry {
						require.Zero(t, gotConn.Load())
						require.Zero(t, wrote.Load())
					}
				}
				if kind == "after_write_close" || kind == "success" {
					require.EqualValues(t, 1, received.Load())
					require.Positive(t, bodyBytes.Load())
					require.Positive(t, wrote.Load())
				}
				if retry {
					require.EqualValues(t, 1, secondReceived.Load())
					require.Positive(t, secondBytes.Load())
					require.Equal(t, http.StatusOK, response.Code)
					billingTransportAuditAssert(t, logs, 15, 2, false, kind == "custom_dial_refused" || kind == "after_write_close")
				} else if kind == "success" {
					require.Equal(t, http.StatusOK, response.Code)
					billingTransportAuditAssert(t, logs, 15, 1, false, false)
				} else if kind == "after_write_close" || kind == "custom_dial_refused" {
					require.Equal(t, http.StatusInternalServerError, response.Code)
					billingTransportAuditAssert(t, logs, hold, 1, true, true)
				} else {
					require.Equal(t, http.StatusInternalServerError, response.Code)
					billingTransportAuditAssert(t, logs, 0, 1, false, false)
				}
			})
		}
	}
}

// TestBillingTransportAuditOpaqueDialErrorRetainsHold prevents error-shape inference for a custom transport with unknown write semantics.
func TestBillingTransportAuditOpaqueDialErrorRetainsHold(t *testing.T) {
	first := billingTransportAuditChannel(billingTransportAuditFirst, "http://billing-opaque.invalid", 10)
	c, response, logs := billingTransportAuditFixture(t, first, nil)
	transport := &billingTransportAuditOpaque{}
	client.HTTPClient = &http.Client{Transport: transport}
	entrycontroller.Relay(c)
	billingTransportAuditDrain(t)
	require.EqualValues(t, 1, transport.calls.Load())
	require.Equal(t, http.StatusInternalServerError, response.Code)
	billingTransportAuditAssert(t, logs, billingTransportAuditHold(t, logs), 1, true, true)
}

// TestBillingTransportAuditRedirectThenDialFailureRetainsHold proves an earlier written request remains ambiguous despite a later connection refusal.
func TestBillingTransportAuditRedirectThenDialFailureRetainsHold(t *testing.T) {
	var received, bodyBytes, refused, getConn, gotConn, wrote atomic.Int64
	server := billingTransportAuditServer(t, "redirect_after_read", &received, &bodyBytes)
	c, response, logs := billingTransportAuditFixture(t, billingTransportAuditChannel(billingTransportAuditFirst, server.URL, 10), nil)
	billingTransportAuditTrace(c, &getConn, &gotConn, &wrote)
	billingTransportAuditHTTPClient(t, []string{server.Listener.Addr().String()}, "billing-redirect-refused.invalid:80", &refused)
	entrycontroller.Relay(c)
	billingTransportAuditDrain(t)
	require.EqualValues(t, 1, received.Load())
	require.Positive(t, bodyBytes.Load())
	require.EqualValues(t, 1, refused.Load())
	require.Positive(t, gotConn.Load())
	require.Positive(t, wrote.Load())
	require.Equal(t, http.StatusInternalServerError, response.Code)
	billingTransportAuditAssert(t, logs, billingTransportAuditHold(t, logs), 1, true, true)
}

// TestBillingTransportAuditCancelledBeforeDoRefunds reserves real quota before cancellation and prevents both standard and context-ignoring opaque transport dispatch.
func TestBillingTransportAuditCancelledBeforeDoRefunds(t *testing.T) {
	for _, opaque := range []bool{false, true} {
		name := "standard_transport"
		if opaque {
			name = "opaque_transport_ignores_context"
		}
		t.Run(name, func(t *testing.T) {
			first := billingTransportAuditChannel(billingTransportAuditFirst, "http://billing-prewrite-refused.invalid", 10)
			c, response, logs := billingTransportAuditFixture(t, first, nil)
			ctx, cancel := context.WithCancel(c.Request.Context())
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			var cancelled, refused, getConn, gotConn, wrote atomic.Int64
			require.NoError(t, model.LOG_DB.Callback().Create().After("gorm:commit_or_rollback_transaction").Register("billing_transport_cancel_after_provisional_commit", func(tx *gorm.DB) {
				if row, ok := tx.Statement.Dest.(*model.Log); ok && row.Type == model.LogTypeProvisional {
					cancelled.Add(1)
					cancel()
				}
			}))
			billingTransportAuditTrace(c, &getConn, &gotConn, &wrote)
			var opaqueTransport *billingTransportAuditOpaque
			if opaque {
				// The fixture RoundTrip intentionally never reads Request.Context:
				// the request helper must prevent invocation before handing it work.
				opaqueTransport = &billingTransportAuditOpaque{}
				client.HTTPClient = &http.Client{Transport: opaqueTransport}
			} else {
				billingTransportAuditHTTPClient(t, nil, "billing-prewrite-refused.invalid:80", &refused)
			}
			entrycontroller.Relay(c)
			billingTransportAuditDrain(t)
			require.EqualValues(t, 1, cancelled.Load())
			require.ErrorIs(t, c.Request.Context().Err(), context.Canceled)
			if opaqueTransport != nil {
				require.Zero(t, opaqueTransport.calls.Load(), "pre-cancelled work must never reach an opaque RoundTripper that ignores cancellation")
			}
			require.Zero(t, refused.Load())
			require.Zero(t, gotConn.Load())
			require.Zero(t, wrote.Load())
			require.NotEqual(t, http.StatusOK, response.Code)
			require.Positive(t, billingTransportAuditHold(t, logs))
			billingTransportAuditAssert(t, logs, 0, 1, false, false)
		})
	}
}
