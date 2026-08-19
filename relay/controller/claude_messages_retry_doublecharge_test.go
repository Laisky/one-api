package controller

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// retryDoubleChargeModel is a Claude model with a deterministic ratio
// (0.8 * MilliTokensUsd = 0.4) and completion ratio 5.0, so the pre-consumed
// base quota is comfortably large relative to the seeded (small) user quota.
const retryDoubleChargeModel = "claude-3-5-haiku-20241022"

// retryDoubleChargeMaxTokens drives the pre-consume completion estimate
// (max_tokens * ratio * completionRatio = 1024 * 0.4 * 5.0 = 2048), keeping
// baseQuota well above zero so PreConsumeTokenQuota actually deducts.
const retryDoubleChargeMaxTokens = 1024

// claudeNonStreamUpstreamBody is a valid Claude non-stream messages response
// with a usage block, used for the successful retry attempt.
const claudeNonStreamUpstreamBody = `{
  "id": "msg_retry_doublecharge",
  "type": "message",
  "role": "assistant",
  "model": "claude-3-5-haiku-20241022",
  "content": [{"type": "text", "text": "Hello from the retry channel."}],
  "stop_reason": "end_turn",
  "usage": {"input_tokens": 11, "output_tokens": 7}
}`

// setupClaudeRetryContext wires a gin.Context exactly the way the production
// retry loop drives it for a Claude Messages request against the Anthropic
// channel fixture, mirroring the ctxkey block from the response-fallback tests.
func setupClaudeRetryContext(t *testing.T, recorder *httptest.ResponseRecorder, baseURL string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(recorder)

	requestPayload := `{"model":"` + retryDoubleChargeModel +
		`","stream":false,"max_tokens":` + strconv.Itoa(retryDoubleChargeMaxTokens) +
		`,"messages":[{"role":"user","content":"Hello, retry double charge regression."}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(requestPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer anthropic-key")
	c.Request = req

	gmw.SetLogger(c, logger.Logger)

	c.Set(ctxkey.Channel, channeltype.Anthropic)
	c.Set(ctxkey.ChannelId, fallbackAnthropicChannelID)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, retryDoubleChargeModel)
	c.Set(ctxkey.BaseURL, baseURL)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, "req_retry_doublecharge")
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserObj, &model.User{Id: fallbackUserID, Quota: retryDoubleChargeUserQuota})
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackAnthropicChannelID, Type: channeltype.Anthropic})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	// CRITICAL: a finite (not unlimited) token quota together with the small
	// seeded user quota keeps preConsumeClaudeMessagesQuota out of the trust
	// path (userQuota is NOT > 100*baseQuota), so PreConsumeTokenQuota really
	// deducts and the leak is observable.
	c.Set(ctxkey.TokenQuotaUnlimited, false)
	c.Set(ctxkey.TokenQuota, int64(retryDoubleChargeUserQuota))

	return c
}

// retryDoubleChargeUserQuota is a small finite quota (~5x the estimated
// baseQuota of ~2054) so pre-consume succeeds but the trust-skip never fires.
const retryDoubleChargeUserQuota = 10_000

// seedRetryDoubleChargeUser overrides the shared fixtures so the user/token
// have a SMALL finite quota for this test, then returns the starting quota.
func seedRetryDoubleChargeUser(t *testing.T) int64 {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.User{}).
		Where("id = ?", fallbackUserID).
		Update("quota", retryDoubleChargeUserQuota).Error,
		"failed to seed small user quota")
	require.NoError(t, model.DB.Model(&model.Token{}).
		Where("id = ?", fallbackTokenID).
		Updates(map[string]any{
			"unlimited_quota": false,
			"remain_quota":    retryDoubleChargeUserQuota,
		}).Error,
		"failed to seed finite token quota")
	return reloadUserQuota(t)
}

func reloadUserQuota(t *testing.T) int64 {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Where("id = ?", fallbackUserID).First(&user).Error,
		"failed to reload user quota")
	return user.Quota
}

func requestCostQuota(t *testing.T, requestID string) int64 {
	t.Helper()
	var cost model.UserRequestCost
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&cost).Error,
		"failed to load user request cost for request %q", requestID)
	return cost.Quota
}

func drainCriticalTasks(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(ctx), "failed to drain critical billing tasks")
}

// TestRelayClaudeMessages_CrossChannelRetryDoesNotDoubleCharge reproduces the
// cross-channel retry double-charge: attempt 1 forwards upstream then fails
// (conservative refund-skip leaves the pre-consumed quota outstanding), and the
// retry attempt pre-consumes + post-consumes again. The user must be charged
// for exactly ONE settled request.
func TestRelayClaudeMessages_CrossChannelRetryDoesNotDoubleCharge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })

	// Attempt 1 -> HTTP 500 (forwards upstream then fails => conservative skip).
	// Attempt 2 -> HTTP 200 with a valid Claude non-stream messages body.
	var attempt int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"upstream boom"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(claudeNonStreamUpstreamBody))
	}))
	t.Cleanup(upstream.Close)

	prevClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = prevClient })

	startQuota := seedRetryDoubleChargeUser(t)
	require.EqualValues(t, retryDoubleChargeUserQuota, startQuota, "unexpected seeded start quota")

	recorder := httptest.NewRecorder()
	c := setupClaudeRetryContext(t, recorder, upstream.URL)
	ctx := gmw.Ctx(c)

	// --- Attempt 1: forwarded, then upstream 500 (expect failure) ---
	err1 := RelayClaudeMessagesHelper(c)
	require.NotNil(t, err1, "attempt 1 must fail (upstream 500)")

	// The async conservative-skip refund / billing-audit defer runs via
	// graceful.GoCritical; drain so the post-attempt-1 state is settled.
	drainCriticalTasks(t)

	afterAttempt1 := reloadUserQuota(t)
	require.Less(t, afterAttempt1, startQuota,
		"attempt 1 must have actually pre-consumed quota (trust-skip must NOT fire); "+
			"start=%d after=%d", startQuota, afterAttempt1)
	require.True(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded),
		"attempt 1 should have marked the request as forwarded upstream")

	// --- BETWEEN ATTEMPTS: the exact production step the retry loop runs ---
	// This invokes the same helper controller.Relay calls immediately before
	// middleware.SetupContextForSelectedChannel; no test-only inline logic.
	ResetPerAttemptBillingForRetry(ctx, c)
	drainCriticalTasks(t)

	// Restore the request body exactly as the production loop does between
	// attempts (controller/relay.go reuses the same context + body).
	requestBody, gerr := common.GetRequestBody(c)
	require.NoError(t, gerr, "failed to fetch request body for retry")
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

	// --- Attempt 2: succeeds on the retry channel (HTTP 200) ---
	err2 := RelayClaudeMessagesHelper(c)
	require.Nil(t, err2, "attempt 2 must succeed (upstream 200); err=%v", err2)
	require.Equal(t, http.StatusOK, recorder.Code, "expected 200 from successful retry")

	// Drain post-billing (GoCritical) before reading final quota / request cost.
	drainCriticalTasks(t)

	endQuota := reloadUserQuota(t)
	decremented := startQuota - endQuota

	// The single settled charge: UserRequestCost for the shared request_id holds
	// attempt 2's final reconciled quota (attempt 1 was reconciled to 0).
	singleCharge := requestCostQuota(t, "req_retry_doublecharge")
	require.Positive(t, singleCharge, "expected a positive settled charge for the request")

	require.EqualValues(t, singleCharge, decremented,
		"cross-channel retry must charge for exactly ONE settled request; "+
			"single_charge=%d actual_decrement=%d (a ~2x decrement indicates the "+
			"leaked attempt-1 pre-consume was never refunded)",
		singleCharge, decremented)
}

// TestResetPerAttemptBillingForRetry_NoOutstandingPreConsume is a guard: when an
// attempt left no outstanding pre-consumed quota, the reset helper must be a pure
// no-op for user quota (no spurious negative refund / credit) and still clear the
// per-attempt markers.
func TestResetPerAttemptBillingForRetry_NoOutstandingPreConsume(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	startQuota := seedRetryDoubleChargeUser(t)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}"))
	gmw.SetLogger(c, logger.Logger)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.Id, fallbackUserID)
	// No outstanding pre-consume for this attempt.
	c.Set(ctxkey.PreConsumedQuotaAmount, int64(0))
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true) // even if forwarded, amount==0 => nothing to refund
	c.Set(ctxkey.ProvisionalLogId, 0)

	ResetPerAttemptBillingForRetry(gmw.Ctx(c), c)
	drainCriticalTasks(t)

	require.EqualValues(t, startQuota, reloadUserQuota(t),
		"reset helper must not change user quota when there is no outstanding pre-consume")
	require.False(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded), "forwarded marker must be cleared")
	require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount), "pre-consumed amount must be cleared")
	require.Zero(t, c.GetInt(ctxkey.ProvisionalLogId), "provisional log id must be cleared")
	require.False(t, c.GetBool(ctxkey.BillingReconciled), "billing-reconciled flag must be cleared")
}

// TestResetPerAttemptBillingForRetry_AlreadyRefundedNotDoubleRefunded is a guard
// for the idempotency gate: an attempt that failed BEFORE forwarding has its
// pre-consume returned by the normal refund path (forwarded marker stays false),
// yet PreConsumedQuotaAmount is left non-zero. The reset helper must NOT refund
// again (no over-credit), proving each attempt's pre-consume nets to exactly one
// outcome. This models the "fails before forwarding then retries" case without
// the impractical full pre-forward upstream simulation.
func TestResetPerAttemptBillingForRetry_AlreadyRefundedNotDoubleRefunded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	startQuota := seedRetryDoubleChargeUser(t)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}"))
	gmw.SetLogger(c, logger.Logger)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.Id, fallbackUserID)

	// Simulate the post-state of a NOT-forwarded failed attempt whose pre-consume
	// was already returned via the normal conservative-refund path: the amount is
	// recorded, BillingReconciled was set, but the forwarded marker is false.
	const alreadyRefunded = int64(2056)
	markPreConsumed(c, alreadyRefunded)
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, false)
	markBillingReconciled(c)

	ResetPerAttemptBillingForRetry(gmw.Ctx(c), c)
	drainCriticalTasks(t)

	require.EqualValues(t, startQuota, reloadUserQuota(t),
		"reset helper must NOT re-refund an attempt that was not forwarded (already refunded); "+
			"a credit here would over-refund the user")
	require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount), "pre-consumed amount must be cleared")
	require.False(t, c.GetBool(ctxkey.BillingReconciled), "billing-reconciled flag must be cleared for the next attempt")
}
