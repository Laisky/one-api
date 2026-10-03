package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestClaude431ReviewRefundOwnership verifies cancellation, failure, and duplicate-call safety.
// A failed durable refund blocks replay, leaves a recoverable intent, and never fabricates zero cost.
func TestClaude431ReviewRefundOwnership(t *testing.T) {
	ensureResponseFallbackFixtures(t)
	oldRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(oldRedis) })
	oldLogging := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(oldLogging) })
	for _, failDB := range []bool{false, true} {
		name := "canceled_client"
		if failDB {
			name = "failed_credit"
		}
		t.Run(name, func(t *testing.T) {
			start := seedRetryDoubleChargeUser(t)
			c := setupClaudeRetryContext(t, httptest.NewRecorder(), "")
			c.Set(ctxkey.RequestId, "claude-refund-"+name)
			m := meta.GetByContext(c)
			var request ClaudeMessagesRequest
			require.NoError(t, json.Unmarshal([]byte(`{"model":"claude-sonnet-5-5","max_tokens":1024,"messages":[{"role":"user","content":"hello"}]}`), &request))
			cfg := anthropic.ModelRatios[request.Model]
			amount, admissionErr := preConsumeClaudeMessagesQuota(c, &request, 4784, cfg.Ratio, cfg.CompletionRatio, m)
			require.Nil(t, admissionErr)
			require.Positive(t, amount)
			markPreConsumed(c, amount)
			c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
			require.NoError(t, model.UpdateUserRequestCostQuotaByRequestID(fallbackUserID, c.GetString(ctxkey.RequestId), amount))
			response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"invalid_request_error","message":"rejected"}}`))}
			failure, usage := anthropic.ClaudeNativeHandler(c, response, 4784, request.Model)
			require.Nil(t, usage)
			require.True(t, anthropic.IsAdmissionRejection(failure))
			forged := &relaymodel.ErrorWithStatusCode{StatusCode: 400, Error: relaymodel.Error{Type: "invalid_request_error", RawError: errors.New(failure.RawError.Error())}}
			require.False(t, refundClaudeAdmission(c, forged, amount, fallbackTokenID))
			require.Equal(t, amount, start-reloadUserQuota(t))
			ctx, cancel := context.WithCancel(c.Request.Context())
			c.Request = c.Request.WithContext(ctx)
			cancel()
			var blocked atomic.Bool
			blocked.Store(failDB)
			const hook = "test:claude431_refund_credit"
			require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
				if blocked.Load() && tx.Statement.Table == "quota_refunds" {
					tx.AddError(errors.New("synthetic refund database outage"))
				}
			}))
			defer func() { require.NoError(t, model.DB.Callback().Update().Remove(hook)) }()
			require.True(t, refundClaudeAdmission(c, failure, amount, fallbackTokenID))
			require.True(t, refundClaudeAdmission(c, failure, amount, fallbackTokenID), "repeat must reuse ownership, not credit twice")
			billingAuditSafetyNet(c)
			ResetPerAttemptBillingForRetry(ctx, c)
			drainCriticalTasks(t)
			var intents []model.QuotaRefund
			require.NoError(t, model.DB.Where("request_id = ?", "claude-refund-"+name).Find(&intents).Error)
			require.Len(t, intents, 1)
			if failDB {
				require.False(t, BillingAllowsRetry(c))
				require.Equal(t, model.QuotaRefundPending, intents[0].Status)
				require.Equal(t, amount, start-reloadUserQuota(t))
				require.Equal(t, amount, requestCostQuota(t, "claude-refund-"+name))
				blocked.Store(false)
				require.NoError(t, model.ApplyQuotaRefund(context.Background(), intents[0].ID))
				require.NoError(t, model.ApplyQuotaRefund(context.Background(), intents[0].ID))
			} else {
				require.Equal(t, model.QuotaRefundCompleted, intents[0].Status)
				require.Equal(t, int64(0), requestCostQuota(t, "claude-refund-"+name))
			}
			require.Equal(t, start, reloadUserQuota(t), "durable replay must credit exactly once")
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, start, token.RemainQuota)
		})
	}
}
