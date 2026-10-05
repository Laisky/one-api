package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// securityAdmissionContext supplies one isolated request with deliberately stale high cached balances.
func securityAdmissionContext(token int, unlimited bool) (*gin.Context, *metalib.Meta) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	gmw.SetLogger(c, logger.Logger)
	c.Set(ctxkey.TokenId, token)
	c.Set(ctxkey.TokenQuota, int64(10_000_000))
	c.Set(ctxkey.TokenQuotaUnlimited, unlimited)
	return c, &metalib.Meta{UserId: fallbackUserID, TokenId: token, ChannelId: fallbackChannelID, ChannelType: channeltype.OpenAI, Mode: relaymode.ChatCompletions}
}

// securityReserveViaEntry uses existing production entry helpers, not a substitute budget implementation.
func securityReserveViaEntry(c *gin.Context, m *metalib.Meta, entry int) (int64, *relaymodel.ErrorWithStatusCode) {
	switch entry {
	case 0:
		return preConsumeQuota(c, &relaymodel.GeneralOpenAIRequest{Model: "fixture"}, &relaymodel.Usage{PromptTokens: 10, TotalTokens: 10}, 1, 1, nil, 1, nil, nil, m)
	case 1:
		return preConsumeClaudeMessagesQuota(c, &ClaudeMessagesRequest{Model: "fixture"}, 10, 1, 1, m)
	case 2:
		return preConsumeRerankQuota(c, 10, m)
	case 3:
		return preConsumeOCRQuota(c, 10, m)
	default:
		return preConsumeVoiceCloneQuota(c, 10, m)
	}
}

// securityAdmissionSetup resets durable fixtures and disables the optional metadata cache.
func securityAdmissionSetup(t *testing.T, balance int64) {
	t.Helper()
	ensureResponseFallbackFixtures(t)
	oldRedis, oldPre := common.IsRedisEnabled(), config.PreConsumedQuota
	common.SetRedisEnabled(false)
	config.PreConsumedQuota = 0
	t.Cleanup(func() { common.SetRedisEnabled(oldRedis); config.PreConsumedQuota = oldPre })
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", fallbackUserID).Update("quota", balance).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).Updates(map[string]any{"remain_quota": balance, "used_quota": 0, "unlimited_quota": false}).Error)
}

// TestSecurityPaidAdmissionReservesOutstanding reproduces every high-balance reservation shortcut.
func TestSecurityPaidAdmissionReservesOutstanding(t *testing.T) {
	for entry, name := range []string{"text-and-response-fallback", "claude", "rerank", "ocr", "voice-clone"} {
		t.Run(name, func(t *testing.T) {
			const balance = int64(10010)
			securityAdmissionSetup(t, balance)
			c, m := securityAdmissionContext(fallbackTokenID, false)
			for i := int64(1); i <= 3; i++ {
				held, apiErr := securityReserveViaEntry(c, m, entry)
				require.Nil(t, apiErr)
				require.EqualValues(t, 10, held)
				require.Equal(t, balance-i*10, reloadUserQuota(t))
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				require.Equal(t, balance-i*10, token.RemainQuota)
			}
		})
	}
}

// TestSecurityConcurrentMixedAdmission cannot reuse one owner's money across outstanding requests and tokens.
func TestSecurityConcurrentMixedAdmission(t *testing.T) {
	for _, unlimited := range []bool{false, true} {
		t.Run(fmt.Sprintf("second-token-unlimited=%v", unlimited), func(t *testing.T) {
			const balance = int64(1001)
			securityAdmissionSetup(t, balance)
			token := &model.Token{UserId: fallbackUserID, Name: "security-concurrent", Key: fmt.Sprintf("security-concurrent-%v", unlimited), Status: model.TokenStatusEnabled, RemainQuota: balance, UnlimitedQuota: unlimited}
			require.NoError(t, model.DB.Create(token).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Delete(token).Error) })
			sqlDB, err := model.DB.DB()
			require.NoError(t, err)
			previous := sqlDB.Stats().MaxOpenConnections
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { sqlDB.SetMaxOpenConns(previous) })
			type result struct {
				held  int64
				token int
				err   *relaymodel.ErrorWithStatusCode
			}
			results := make(chan result, 202)
			start := make(chan struct{})
			var workers sync.WaitGroup
			for i := range 202 {
				id := fallbackTokenID
				if i%2 == 1 {
					id = token.Id
				}
				c, m := securityAdmissionContext(id, unlimited && id == token.Id)
				workers.Add(1)
				go func(entry, id int) {
					defer workers.Done()
					<-start
					held, apiErr := securityReserveViaEntry(c, m, entry%5)
					results <- result{held, id, apiErr}
				}(i, id)
			}
			close(start)
			workers.Wait()
			close(results)
			accepted := 0
			var holds []result
			for result := range results {
				if result.err != nil {
					require.Zero(t, result.held, "rejected work owns no reservation")
					continue
				}
				accepted++
				require.EqualValues(t, 10, result.held)
				holds = append(holds, result)
			}
			require.Equal(t, 100, accepted, "atomic admission must not overbook or strand the funded budget")
			require.EqualValues(t, 1, reloadUserQuota(t))
			// Release each test-owned hold exactly once, using the real ledger.
			for _, hold := range holds {
				require.NoError(t, model.PostConsumeTokenQuota(context.Background(), hold.token, -hold.held))
			}
			require.Equal(t, balance, reloadUserQuota(t))
			var restored model.Token
			require.NoError(t, model.DB.First(&restored, token.Id).Error)
			require.Equal(t, balance, restored.RemainQuota)
		})
	}
}
