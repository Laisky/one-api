package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	rcontroller "github.com/Laisky/one-api/relay/controller"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// cohereDispatchObservation captures a complete provider request and both
// physical reservations before the local server responds or closes its socket.
type cohereDispatchObservation struct {
	path, method string
	body         []byte
	owner, token int64
	err          error
}

// drainCohereTransportBilling waits for all managed request side effects before
// t examines the isolated ledger or restores shared test configuration.
func drainCohereTransportBilling(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(ctx))
}

// TestCohereAcceptedTransportReset exercises the real Relay controller and
// registered Cohere adapter with a complete accepted POST followed by no headers.
// It checks bounded routing, durable estimated settlement and rejection controls
// against isolated SQLite, for both channel-selection implementations.
func TestCohereAcceptedTransportReset(t *testing.T) {
	for _, route := range routingPaths {
		for _, tc := range []struct {
			name   string
			status int
			retry  int
		}{
			{"accepted_reset", 0, 2},
			{"accepted_reset_terminal", 0, 0},
			{"uncertain_503", 503, 2},
			{"rejected_403", 403, 0},
			{"rejected_429", 429, 0},
			{"usable_second_channel", 200, 2},
		} {
			t.Run(route.name+"/"+tc.name, func(t *testing.T) {
				const balance = int64(100000)
				const ownerID, tokenID = 901, 902
				const requestModel = "embed-v4.0"
				previousGinMode := gin.Mode()
				gin.SetMode(gin.TestMode)
				t.Cleanup(func() { gin.SetMode(previousGinMode) })
				var firstCalls, secondCalls atomic.Int32
				observations := make(chan cohereDispatchObservation, 4)
				observe := func(r *http.Request) cohereDispatchObservation {
					body, err := io.ReadAll(r.Body)
					result := cohereDispatchObservation{path: r.URL.Path, method: r.Method, body: body, err: err}
					var owner dbmodel.User
					var token dbmodel.Token
					if result.err == nil {
						result.err = dbmodel.DB.First(&owner, ownerID).Error
					}
					if result.err == nil {
						result.err = dbmodel.DB.First(&token, tokenID).Error
					}
					result.owner, result.token = balance-owner.Quota, balance-token.RemainQuota
					return result
				}
				first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					firstCalls.Add(1)
					result := observe(r)
					if tc.status == 0 {
						conn, _, err := w.(http.Hijacker).Hijack()
						if err == nil {
							err = conn.Close()
						}
						if result.err == nil {
							result.err = err
						}
						observations <- result
						return
					}
					observations <- result
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, `{"message":"fixture rejection"}`)
				}))
				t.Cleanup(first.Close)
				second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					secondCalls.Add(1)
					observations <- observe(r)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"embeddings":{"float":[[0.1,0.2]]},"meta":{"billed_units":{"input_tokens":1000}}}`)
				}))
				t.Cleanup(second.Close)
				firstChannel := retryOrderChannel(801, "cohere-accepted", 10)
				secondChannel := retryOrderChannel(802, "cohere-fallback", 5)
				for _, ch := range []*dbmodel.Channel{firstChannel, secondChannel} {
					ch.Type, ch.Models, ch.Key = channeltype.Cohere, requestModel, "synthetic-cohere-fixture"
					ch.ModelConfigs = stringPtr(`{"embed-v4.0":{"ratio":1,"completion_ratio":1}}`)
				}
				firstChannel.BaseURL, secondChannel.BaseURL = stringPtr(first.URL), stringPtr(second.URL)
				// Run after the shared helper restores the original database so its
				// temporary routing pool cannot leak into later controller tests.
				t.Cleanup(func() {
					if dbmodel.DB != nil {
						dbmodel.InitChannelCache()
					}
				})
				setupRetryOrderDB(t, []*dbmodel.Channel{firstChannel, secondChannel})
				t.Cleanup(func() {
					// Empty this temporary pool before the helper rebuilds its cache;
					// when the original database is nil, no fixture channels remain.
					require.NoError(t, dbmodel.DB.Where("id IN ?", []int{firstChannel.Id, secondChannel.Id}).Delete(&dbmodel.Channel{}).Error)
				})
				require.NoError(t, dbmodel.DB.AutoMigrate(&dbmodel.User{}, &dbmodel.Token{}, &dbmodel.UserRequestCost{}, &dbmodel.Log{}, &dbmodel.MCPServer{}, &dbmodel.MCPTool{}))
				previousLogDB := dbmodel.LOG_DB
				dbmodel.LOG_DB = dbmodel.DB
				t.Cleanup(func() { dbmodel.LOG_DB = previousLogDB })
				owner := &dbmodel.User{Id: ownerID, Username: "cohere-owner", Quota: balance, Group: "default"}
				token := &dbmodel.Token{Id: tokenID, UserId: ownerID, Name: "finite-cohere-token", RemainQuota: balance, UnlimitedQuota: false}
				require.NoError(t, dbmodel.DB.Create(owner).Error)
				require.NoError(t, dbmodel.DB.Create(token).Error)
				previousRetry, previousMemory, previousBatch := config.RetryTimes, config.MemoryCacheEnabled, config.BatchUpdateEnabled
				previousRedis, previousConsume := common.IsRedisEnabled(), config.IsLogConsumeEnabled()
				previousPre, previousClient := config.PreConsumedQuota, client.HTTPClient
				config.RetryTimes, config.MemoryCacheEnabled, config.BatchUpdateEnabled = tc.retry, route.memoryCache, false
				config.PreConsumedQuota = 100
				common.SetRedisEnabled(false)
				config.SetLogConsumeEnabled(true)
				client.HTTPClient = first.Client()
				t.Cleanup(func() {
					drainCohereTransportBilling(t)
					config.RetryTimes, config.MemoryCacheEnabled, config.BatchUpdateEnabled = previousRetry, previousMemory, previousBatch
					config.PreConsumedQuota, client.HTTPClient = previousPre, previousClient
					common.SetRedisEnabled(previousRedis)
					config.SetLogConsumeEnabled(previousConsume)
				})
				requestID := fmt.Sprintf("cohere-reset-%d", time.Now().UnixNano())
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				gmw.SetLogger(c, logger.Logger)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"embed-v4.0","input":["hello"],"input_type":"search_query"}`))
				c.Request.Header.Set("Content-Type", "application/json")
				for key, value := range map[string]any{
					ctxkey.Id: ownerID, ctxkey.TokenId: tokenID, ctxkey.TokenName: token.Name,
					ctxkey.Group: "default", ctxkey.RequestModel: requestModel, ctxkey.RequestId: requestID,
					ctxkey.TokenQuota: balance, ctxkey.TokenQuotaUnlimited: false, ctxkey.UserObj: owner,
					ctxkey.Username: owner.Username,
				} {
					c.Set(key, value)
				}
				selected := firstChannel
				if tc.status == 200 {
					selected = secondChannel
				}
				middleware.SetupContextForSelectedChannel(c, selected, requestModel)
				Relay(c)
				drainCohereTransportBilling(t)
				t.Logf("dispatch first=%d second=%d HTTP=%d held=%d body=%s", firstCalls.Load(), secondCalls.Load(), recorder.Code, c.GetInt64(ctxkey.PreConsumedQuotaAmount), recorder.Body.String())
				if tc.status == 200 {
					require.Zero(t, firstCalls.Load())
					require.EqualValues(t, 1, secondCalls.Load(), "the fallback channel must serve a real registered-adapter request")
					require.Equal(t, http.StatusOK, recorder.Code)
				} else {
					require.EqualValues(t, 1, firstCalls.Load())
					require.Zero(t, secondCalls.Load(), "an uncertain accepted attempt must not be replayed")
					require.GreaterOrEqual(t, recorder.Code, 400)
				}
				var observedHold int64
				select {
				case observed := <-observations:
					require.NoError(t, observed.err)
					require.Equal(t, "/v2/embed", observed.path)
					require.Equal(t, http.MethodPost, observed.method)
					var wire map[string]any
					require.NoError(t, json.Unmarshal(observed.body, &wire))
					require.Equal(t, requestModel, wire["model"])
					require.Equal(t, []any{"hello"}, wire["texts"])
					require.Equal(t, "search_query", wire["input_type"])
					require.Positive(t, observed.owner)
					require.Equal(t, observed.owner, observed.token, "both holds exist before the server closes")
					observedHold = observed.owner
				case <-time.After(5 * time.Second):
					t.Fatal("the provider did not read the complete POST")
				}
				held := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
				require.Positive(t, held)
				require.Equal(t, observedHold, held, "settlement must use the physical hold seen before socket closure")
				expected := int64(0)
				if tc.status == 0 || tc.status == 503 {
					expected = held
					require.False(t, rcontroller.BillingAllowsRetry(c))
				} else if tc.status == 200 {
					expected = 1000
				}
				require.NoError(t, dbmodel.DB.First(owner, ownerID).Error)
				require.NoError(t, dbmodel.DB.First(token, tokenID).Error)
				require.Equal(t, balance-expected, owner.Quota)
				require.Equal(t, balance-expected, token.RemainQuota)
				require.Equal(t, expected, token.UsedQuota)
				var costs []dbmodel.UserRequestCost
				require.NoError(t, dbmodel.DB.Where("request_id = ?", requestID).Find(&costs).Error)
				require.Len(t, costs, 1)
				require.Equal(t, ownerID, costs[0].UserID)
				require.Equal(t, expected, costs[0].Quota)
				var rows []dbmodel.Log
				require.NoError(t, dbmodel.LOG_DB.Where("request_id = ? AND type IN ?", requestID, []int{dbmodel.LogTypeConsume, dbmodel.LogTypeProvisional}).Find(&rows).Error)
				require.Len(t, rows, 1)
				require.Equal(t, ownerID, rows[0].UserId)
				require.Equal(t, selected.Id, rows[0].ChannelId)
				require.Equal(t, token.Name, rows[0].TokenName)
				require.Equal(t, requestModel, rows[0].ModelName)
				require.EqualValues(t, expected, rows[0].Quota)
				if expected > 0 {
					require.Equal(t, dbmodel.LogTypeConsume, rows[0].Type)
				}
				if tc.status == 0 || tc.status == 503 {
					require.Equal(t, true, rows[0].Metadata["billing_estimated"])
					require.Equal(t, true, rows[0].Metadata[dbmodel.LogMetadataKeyEstimatedCharge])
					require.Contains(t, rows[0].Metadata["billing_estimate_reason"], "uncertain_upstream_admission_")
				}
			})
		}
	}
}
