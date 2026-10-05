package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
)

// checkSuccessfulRelayReconcilesOneReservation exercises both chat-style entry
// points, MCP and direct calls, and receipts on either side of the reservation.
// t provides assertions and cleanup. Both account balances and the durable log
// must describe exactly the same final charge, not merely a non-increasing quota.
func checkSuccessfulRelayReconcilesOneReservation(t *testing.T, responseAPI bool) {
	t.Helper()
	for _, sdk := range []bool{false, true} {
		for _, withMCP := range []bool{false, true} {
			for _, receipt := range []struct {
				name          string
				input, output int
				trusted       bool
			}{
				{name: "below reservation", input: 5, output: 8},
				{name: "above reservation", input: 400, output: 400},
				{name: "zero usage retains hold"},
				{name: "high balance still reserved", input: 5, output: 8, trusted: true},
			} {
				t.Run(fmt.Sprintf("sdk=%v/responses=%v/mcp=%v/%s", sdk, responseAPI, withMCP, receipt.name), func(t *testing.T) {
					gin.SetMode(gin.TestMode)
					ensureResponseFallbackFixtures(t)
					redis := common.IsRedisEnabled()
					common.SetRedisEnabled(false)
					t.Cleanup(func() { common.SetRedisEnabled(redis) })
					logging := config.IsLogConsumeEnabled()
					config.SetLogConsumeEnabled(true)
					t.Cleanup(func() { config.SetLogConsumeEnabled(logging) })
					pre := config.PreConsumedQuota
					config.PreConsumedQuota = 100
					t.Cleanup(func() { config.PreConsumedQuota = pre })
					balance := int64(1000)
					if receipt.trusted {
						balance = 1_000_000
					}
					require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", fallbackUserID).Update("quota", balance).Error)
					require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).Updates(map[string]any{
						"unlimited_quota": false, "remain_quota": balance, "used_quota": int64(0),
					}).Error)
					if withMCP {
						server := &model.MCPServer{Name: "reservation-mcp", Status: model.MCPServerStatusEnabled, BaseURL: "http://unused.invalid", ToolWhitelist: model.JSONStringSlice{"web_search"}}
						require.NoError(t, model.DB.Create(server).Error)
						tool := &model.MCPTool{ServerId: server.Id, Name: "web_search", InputSchema: `{"type":"object"}`, Status: 1}
						require.NoError(t, model.DB.Create(tool).Error)
						t.Cleanup(func() {
							require.NoError(t, model.DB.Delete(tool).Error)
							require.NoError(t, model.DB.Delete(server).Error)
						})
					}
					heldAtDispatch := make(chan int64, 4)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var current model.User
						if err := model.DB.First(&current, fallbackUserID).Error; err != nil {
							http.Error(w, "fixture quota read failed", http.StatusInternalServerError)
							return
						}
						heldAtDispatch <- balance - current.Quota
						w.Header().Set("Content-Type", "application/json")
						_, _ = fmt.Fprintf(w, `{"id":"msg-reservation","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"Done"}],"stop_reason":"end_turn","usage":{"input_tokens":%d,"output_tokens":%d}}`, receipt.input, receipt.output)
					}))
					t.Cleanup(upstream.Close)
					kind := channeltype.Anthropic
					if sdk {
						kind = channeltype.AwsClaude
						t.Setenv("AWS_ENDPOINT_URL", upstream.URL)
						t.Setenv("AWS_MAX_ATTEMPTS", "1")
					}
					prevClient := client.HTTPClient
					client.HTTPClient = upstream.Client()
					t.Cleanup(func() { client.HTTPClient = prevClient })
					tools := ""
					if withMCP {
						tools = `,"tools":[{"type":"web_search"}]`
					}
					path := "/v1/chat/completions"
					payload := `{"model":"claude-sonnet-4-5","max_tokens":64,"messages":[{"role":"user","content":"Hello"}]` + tools + `}`
					if responseAPI {
						path = "/v1/responses"
						payload = `{"model":"claude-sonnet-4-5","max_output_tokens":64,"input":"Hello"` + tools + `}`
					}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
					c.Request.Header.Set("Content-Type", "application/json")
					c.Request.Header.Set("Authorization", "Bearer test-upstream-key")
					gmw.SetLogger(c, logger.Logger)
					requestID := fmt.Sprintf("reservation-%d", time.Now().UnixNano())
					c.Set(ctxkey.Channel, kind)
					c.Set(ctxkey.ChannelId, fallbackAnthropicChannelID)
					c.Set(ctxkey.TokenId, fallbackTokenID)
					c.Set(ctxkey.TokenName, "fallback-token")
					c.Set(ctxkey.Id, fallbackUserID)
					c.Set(ctxkey.Group, "default")
					c.Set(ctxkey.ModelMapping, map[string]string{})
					c.Set(ctxkey.ChannelRatio, 1.0)
					c.Set(ctxkey.RequestModel, "claude-sonnet-4-5")
					c.Set(ctxkey.BaseURL, upstream.URL)
					c.Set(ctxkey.ContentType, "application/json")
					c.Set(ctxkey.RequestId, requestID)
					c.Set(ctxkey.TokenQuotaUnlimited, false)
					c.Set(ctxkey.TokenQuota, balance)
					c.Set(ctxkey.Username, "response-fallback")
					c.Set(ctxkey.UserObj, &model.User{Id: fallbackUserID, Quota: balance})
					pricing := `{"claude-sonnet-4-5":{"ratio":1,"completion_ratio":1}}`
					c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackAnthropicChannelID, Type: kind, ModelConfigs: &pricing})
					c.Set(ctxkey.Config, model.ChannelConfig{Region: "us-east-1", AK: "test-access-key", SK: "test-secret-key"})
					if responseAPI {
						require.Nil(t, RelayResponseAPIHelper(c))
					} else {
						require.Nil(t, RelayTextHelper(c))
					}
					drainResponseFallbackBilling(t)
					require.Equal(t, http.StatusOK, rec.Code)
					reserved := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
					require.Positive(t, reserved, "every paid admission reserves even at a high balance")
					expected := int64(receipt.input + receipt.output)
					if expected == 0 {
						select {
						case expected = <-heldAtDispatch:
						default:
							t.Fatal("upstream never observed the reservation")
						}
						require.GreaterOrEqual(t, expected, reserved, "missing usage must retain all MCP round reservations too")
					}
					var user model.User
					var token model.Token
					require.NoError(t, model.DB.First(&user, fallbackUserID).Error)
					require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
					require.Equal(t, balance-expected, user.Quota, "the user must pay exactly the reconciled charge")
					require.Equal(t, balance-expected, token.RemainQuota, "token and user balances must agree")
					require.Equal(t, expected, token.UsedQuota)
					var rows []model.Log
					require.NoError(t, model.LOG_DB.Where("request_id = ? AND type IN ?", requestID, []int{model.LogTypeConsume, model.LogTypeProvisional}).Find(&rows).Error)
					require.Len(t, rows, 1, "one logical request must have one reconciled consume log")
					require.Equal(t, model.LogTypeConsume, rows[0].Type)
					require.EqualValues(t, expected, rows[0].Quota)
				})
			}
		}
	}
}
