package controller

import (
	"context"
	"net/http"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
)

// invokeMCPBillingBoundary exercises the real legacy or modern dispatch path.
func invokeMCPBillingBoundary(ctx context.Context, c *gin.Context, modern bool) (*mcp.CallToolResult, error) {
	if modern {
		return callMCPToolForUserLatest(ctx, c, modernMCPCallParams{Name: "fake-mcp.echo"})
	}
	return callMCPToolForUser(ctx, c, mcpCallParams{Name: "fake-mcp.echo"})
}

// setupMCPBillingBoundary adds a real limited API token to the HTTP/SQLite fixture.
func setupMCPBillingBoundary(t *testing.T) (*mcpFixture, *model.Token) {
	t.Helper()
	cleanup, fx := setupMCPProxyTest(t)
	t.Cleanup(cleanup)
	originalBatch := config.BatchUpdateEnabled
	config.BatchUpdateEnabled = false
	t.Cleanup(func() { config.BatchUpdateEnabled = originalBatch })
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}))
	token := &model.Token{Id: 731, UserId: fx.user.Id, Key: "mcp-billing-regression", Name: "billing regression", Status: model.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000}
	require.NoError(t, model.DB.Create(token).Error)
	fx.setToolPricing(t, "echo", model.ToolPricingLocal{QuotaPerCall: 75})
	return fx, token
}

// TestMCPBillingBoundaryAdmission verifies both protocols reject unfunded or
// mismatched callers before any side-effecting HTTP tool execution.
func TestMCPBillingBoundaryAdmission(t *testing.T) {
	for _, modern := range []bool{false, true} {
		protocol := "legacy"
		if modern {
			protocol = "modern"
		}
		for _, state := range []string{"user_low", "token_low", "missing_token", "wrong_owner"} {
			t.Run(protocol+"/"+state, func(t *testing.T) {
				fx, token := setupMCPBillingBoundary(t)
				c, _ := newMCPCallContext(t, fx.user.Id, "admission-"+state)
				c.Set(ctxkey.TokenId, token.Id)
				userQuota, tokenQuota := int64(1000), int64(1000)
				switch state {
				case "user_low":
					userQuota = 74
					require.NoError(t, model.DB.Model(fx.user).Update("quota", userQuota).Error)
				case "token_low":
					tokenQuota = 74
					require.NoError(t, model.DB.Model(token).Update("remain_quota", tokenQuota).Error)
				case "missing_token":
					c.Set(ctxkey.TokenId, 0)
				case "wrong_owner":
					// Both fields have unique indexes. Empty duplicates fail fixture
					// creation before the ownership boundary can actually be tested.
					require.NoError(t, model.DB.Create(&model.User{
						Id: 943, Username: "other-mcp-owner", Quota: 1000, Status: model.UserStatusEnabled,
						AccessToken: "other-mcp-management-token", AffCode: "other-mcp-affiliate",
					}).Error)
					require.NoError(t, model.DB.Model(token).Update("user_id", 943).Error)
				}
				result, err := invokeMCPBillingBoundary(context.Background(), c, modern)
				require.Error(t, err)
				require.Nil(t, result)
				require.Zero(t, fx.upstreamHits, "failed admission must not run the upstream tool")
				user, err := model.GetUserById(fx.user.Id, true)
				require.NoError(t, err)
				stored, err := model.GetTokenById(token.Id)
				require.NoError(t, err)
				require.Equal(t, userQuota, user.Quota)
				require.Equal(t, tokenQuota, stored.RemainQuota)
				require.Zero(t, stored.UsedQuota)
				if state == "wrong_owner" {
					owner, err := model.GetUserById(943, true)
					require.NoError(t, err)
					require.Equal(t, int64(1000), owner.Quota, "mismatched owner must not be charged either")
				}
			})
		}
	}
}

// TestMCPBillingBoundaryLifecycle observes persistent balances at the real HTTP
// side-effect boundary. Client cancellation cannot refund successful paid work;
// HTTP 500 after dispatch is uncertain execution, not proof that work was free.
func TestMCPBillingBoundaryLifecycle(t *testing.T) {
	for _, modern := range []bool{false, true} {
		protocol := "legacy"
		if modern {
			protocol = "modern"
		}
		for _, outcome := range []string{"success", "tool_error", "transport_error", "canceled"} {
			t.Run(protocol+"/"+outcome, func(t *testing.T) {
				fx, token := setupMCPBillingBoundary(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				c, _ := newMCPCallContext(t, fx.user.Id, "lifecycle-"+outcome)
				c.Set(ctxkey.TokenId, token.Id)
				c.Request = c.Request.WithContext(ctx)
				type snapshot struct {
					userQuota  int64
					tokenQuota int64
					err        error
				}
				observed := make(chan snapshot, 4)
				fx.respondPayload = func() ([]byte, int) {
					user, err := model.GetUserById(fx.user.Id, true)
					if err != nil {
						observed <- snapshot{err: err}
					} else if stored, err := model.GetTokenById(token.Id); err != nil {
						observed <- snapshot{err: err}
					} else {
						observed <- snapshot{userQuota: user.Quota, tokenQuota: stored.RemainQuota}
					}
					switch outcome {
					case "tool_error":
						return []byte(`{"jsonrpc":"2.0","id":1,"result":{"content":"error","is_error":true}}`), http.StatusOK
					case "transport_error":
						return []byte(`{"error":"upstream unavailable"}`), http.StatusInternalServerError
					case "canceled":
						cancel()
					}
					return []byte(`{"jsonrpc":"2.0","id":1,"result":{"content":"ok","is_error":false}}`), http.StatusOK
				}
				_, callErr := invokeMCPBillingBoundary(ctx, c, modern)
				if outcome == "transport_error" {
					require.Error(t, callErr)
					var uncertain *mcp.ToolExecutionUncertainError
					require.True(t, errors.As(callErr, &uncertain), "a dispatched HTTP 500 must preserve no-replay classification")
				} else {
					require.NoError(t, callErr)
				}
				require.Equal(t, 1, fx.upstreamHits, "one logical call must not replay an uncertain side effect")
				select {
				case atDispatch := <-observed:
					require.NoError(t, atDispatch.err)
					require.Equal(t, int64(925), atDispatch.userQuota, "user quota must be reserved before paid execution")
					require.Equal(t, int64(925), atDispatch.tokenQuota, "token quota must be reserved before paid execution")
				default:
					t.Fatal("upstream execution was not observed")
				}
				user, err := model.GetUserById(fx.user.Id, true)
				require.NoError(t, err)
				stored, err := model.GetTokenById(token.Id)
				require.NoError(t, err)
				remaining, used := int64(925), int64(75)
				if outcome == "tool_error" {
					remaining, used = 1000, 0
				}
				require.Equal(t, remaining, user.Quota, "must neither double-debit nor refund uncertain execution")
				require.Equal(t, remaining, stored.RemainQuota)
				require.Equal(t, used, stored.UsedQuota)
				var logs []model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ?", "lifecycle-"+outcome).Find(&logs).Error)
				if outcome == "tool_error" {
					require.Empty(t, logs)
				} else {
					require.Len(t, logs, 1)
					require.Equal(t, 75, logs[0].Quota)
					if outcome == "transport_error" {
						require.Contains(t, logs[0].Content, "reconciliation required")
					}
				}
			})
		}
	}
}
