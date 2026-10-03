package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

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

// concurrentRetryUserQuota is a LARGE finite quota: large enough to absorb N
// concurrent base pre-consumes (~2054 each) without ever exhausting the balance,
// yet finite (and the token finite + not unlimited) so the trust-skip path in
// preConsumeClaudeMessagesQuota does NOT fire and pre-consume really deducts.
const concurrentRetryUserQuota = 5_000_000

// concurrentRetryChannelID is the retry (attempt-2) channel id. Switching the
// ChannelId between attempts (alongside BaseURL) mirrors what the production
// middleware.SetupContextForSelectedChannel does on a cross-channel retry: it is
// what makes meta.GetByContext refresh the cached meta's BaseURL to the retry
// channel. fallbackOpenAIChannelID is reused only as a distinct id sentinel; the
// channel stays Anthropic via ctxkey.Channel/ChannelModel below.
const concurrentRetryChannelID = fallbackOpenAIChannelID

// seedConcurrentRetryUser overrides the shared fixtures so the user/token carry a
// big finite quota for this concurrency test, then returns the starting quota.
// It mirrors seedRetryDoubleChargeUser but with a quota sized for N goroutines.
func seedConcurrentRetryUser(t *testing.T) int64 {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.User{}).
		Where("id = ?", fallbackUserID).
		Update("quota", int64(concurrentRetryUserQuota)).Error,
		"failed to seed large user quota")
	require.NoError(t, model.DB.Model(&model.Token{}).
		Where("id = ?", fallbackTokenID).
		Updates(map[string]any{
			"unlimited_quota": false,
			"remain_quota":    int64(concurrentRetryUserQuota),
		}).Error,
		"failed to seed finite token quota")
	return reloadUserQuota(t)
}

// setupConcurrentRetryContext wires a per-goroutine gin.Context exactly the way
// the production retry loop drives a Claude Messages request, but with a UNIQUE
// request id and its OWN cancellable request context.
//
// The request id MUST be unique per goroutine: UserRequestCost is keyed by
// request_id, so a shared id across concurrent requests would collide and the
// per-request exactly-once accounting could not be checked. (This is why we do
// not reuse setupClaudeRetryContext, which hardcodes ctxkey.RequestId.)
func setupConcurrentRetryContext(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	baseURL string,
	requestID string,
) (*gin.Context, context.CancelFunc) {
	t.Helper()
	c, _ := gin.CreateTestContext(recorder)

	requestPayload := `{"model":"` + retryDoubleChargeModel +
		`","stream":false,"max_tokens":` + strconv.Itoa(retryDoubleChargeMaxTokens) +
		`,"messages":[{"role":"user","content":"Hello, concurrent retry double charge regression."}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(requestPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer anthropic-key")

	// Give each request its own cancellable context so the test also exercises the
	// detached-context refund path (graceful.GoCritical refunds must survive request
	// cancellation), matching how a real handler's request context is scoped.
	reqCtx, cancel := context.WithCancel(context.Background())
	c.Request = req.WithContext(reqCtx)

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
	c.Set(ctxkey.RequestId, requestID)
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserObj, &model.User{Id: fallbackUserID, Quota: concurrentRetryUserQuota})
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackAnthropicChannelID, Type: channeltype.Anthropic})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	// Finite (not unlimited) token quota that is NOT > 100*baseQuota keeps
	// preConsumeClaudeMessagesQuota out of the trust path, so PreConsumeTokenQuota
	// really deducts and any leaked attempt-1 pre-consume is observable.
	c.Set(ctxkey.TokenQuotaUnlimited, false)
	c.Set(ctxkey.TokenQuota, int64(concurrentRetryUserQuota))

	return c, cancel
}

// TestRelayClaudeMessages_ConcurrentCrossChannelRetry_ExactlyOnceCharge is the
// CONCURRENT regression guard for the documented cross-channel retry
// double-charge. Each request does the exact production retry dance:
//
//	attempt 1 -> failServer (HTTP 500, forwarded upstream): the conservative
//	             refund-skip leaves the base pre-consume (~2054) OUTSTANDING;
//	ResetPerAttemptBillingForRetry -> refunds that outstanding pre-consume and
//	             clears the per-attempt markers (spawning an async GoCritical
//	             refund on a detached context);
//	attempt 2 -> okServer (HTTP 200, valid usage): pre-consumes + post-consumes,
//	             settling exactly one charge into UserRequestCost.
//
// N such requests run concurrently (own gin.Context + recorder + unique request
// id each) so gin pool recycling and many simultaneous async refunds race the
// retry step. After draining all async refunds:
//
//   - the user quota must drop by EXACTLY the sum of the per-request settled
//     charges (one charge per request);
//   - a ~2x total decrement would mean attempt-1's pre-consume leaked (was never
//     refunded by ResetPerAttemptBillingForRetry) => the double-charge bug;
//   - a too-small decrement would mean over-refund (a charge or the wrong amount
//     was credited back).
//
// Two httptest servers (one always-500, one always-200) keep the test
// shared-counter-free, so the only contention is the real billing/refund path.
func TestRelayClaudeMessages_ConcurrentCrossChannelRetry_ExactlyOnceCharge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })

	// In-memory sqlite serializes writes and returns "database is locked" under many
	// concurrent writers. Serialize the pool to ONE connection (with a generous busy
	// timeout) so concurrent quota UPDATEs queue instead of erroring. This does NOT
	// serialize the goroutines themselves: all N relay attempts + async refunds still
	// run concurrently, racing the retry step and gin's context recycling — only their
	// individual DB writes are funneled through one connection (production uses
	// MySQL/Postgres which handle the concurrency natively).
	sqlDB, err := model.DB.DB()
	require.NoError(t, err, "failed to access underlying sql.DB")
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.SetMaxOpenConns(config.SQLMaxOpenConns) })
	require.NoError(t, model.DB.Exec("PRAGMA busy_timeout = 30000").Error,
		"failed to set sqlite busy_timeout")

	// failServer always 500s with a Claude error body AFTER receiving the request
	// (forwarded upstream => conservative refund-skip keeps the base pre-consume
	// outstanding). okServer always 200s with a valid Claude usage block.
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"upstream boom"}}`))
	}))
	t.Cleanup(failServer.Close)

	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(claudeNonStreamUpstreamBody))
	}))
	t.Cleanup(okServer.Close)

	// Both servers are plain http; failServer.Client() reaches either.
	prevClient := client.HTTPClient
	client.HTTPClient = failServer.Client()
	t.Cleanup(func() { client.HTTPClient = prevClient })

	startQuota := seedConcurrentRetryUser(t)
	require.EqualValues(t, concurrentRetryUserQuota, startQuota, "unexpected seeded start quota")

	const (
		// N concurrent requests, each performing a fail->retry->success dance. With
		// ~2054 outstanding per leaked attempt, a double-charge would roughly double
		// the total decrement; N=60 makes the gap unmistakable while staying within
		// the seeded quota even in the (buggy) double-charge case (60*~4108 << 5M).
		n           = 60
		maxInFlight = 32 // bound goroutines actually running at once (>=16 required)
	)

	requestIDs := make([]string, n)
	for i := 0; i < n; i++ {
		requestIDs[i] = fmt.Sprintf("req_concurrent_retry_%d", i)
	}

	sem := make(chan struct{}, maxInFlight)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			requestID := requestIDs[i]
			recorder := httptest.NewRecorder()
			// Attempt 1 targets failServer (the attempt-1 channel).
			c, cancel := setupConcurrentRetryContext(t, recorder, failServer.URL, requestID)
			defer cancel()

			// --- Attempt 1: forwarded upstream, then 500 (must fail). ---
			err1 := RelayClaudeMessagesHelper(c)
			require.NotNil(t, err1, "[%s] attempt 1 must fail (upstream 500)", requestID)

			// --- BETWEEN ATTEMPTS: the exact production step controller.Relay runs
			// before re-selecting the channel. This refunds the abandoned attempt-1
			// pre-consume (forwarded => conservative skip left it outstanding) and
			// clears the per-attempt billing markers. Concurrently across N goroutines
			// this is the prime double-charge / over-refund race. ---
			ResetPerAttemptBillingForRetry(gmw.Ctx(c), c)

			// Restore the request body exactly as controller/relay.go does between
			// attempts (same context + body reused for the retry).
			requestBody, gerr := common.GetRequestBody(c)
			require.NoError(t, gerr, "[%s] failed to fetch request body for retry", requestID)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

			// Switch to the retry channel (okServer). Mirror what production's
			// middleware.SetupContextForSelectedChannel does on a cross-channel retry:
			// it selects a NEW channel, which changes ctxkey.ChannelId and is precisely
			// the condition meta.GetByContext keys on to refresh the cached meta's
			// BaseURL. Setting BaseURL alone would be ignored (the meta is cached after
			// attempt 1), so the channel id must change too. The channel stays Anthropic.
			c.Set(ctxkey.BaseURL, okServer.URL)
			c.Set(ctxkey.ChannelId, concurrentRetryChannelID)
			c.Set(ctxkey.ChannelModel, &model.Channel{Id: concurrentRetryChannelID, Type: channeltype.Anthropic})

			// --- Attempt 2: succeeds on the retry channel (HTTP 200). ---
			err2 := RelayClaudeMessagesHelper(c)
			require.Nil(t, err2, "[%s] attempt 2 must succeed (upstream 200); err=%v", requestID, err2)
			require.Equal(t, http.StatusOK, recorder.Code, "[%s] expected 200 from successful retry", requestID)
		}(i)
	}
	wg.Wait()

	// Drain all async refunds (ResetPerAttemptBillingForRetry's GoCritical) and the
	// post-billing GoCritical of every successful retry before reading final state.
	drainCriticalTasks(t)

	endQuota := reloadUserQuota(t)
	totalDecrement := startQuota - endQuota
	require.Positive(t, totalDecrement, "expected the concurrent requests to net a positive charge")

	// Sum the per-request settled charges; assert each request settled EXACTLY ONE
	// positive charge and that all N request-cost rows are present.
	var sumSingleCharges int64
	for i := 0; i < n; i++ {
		single := requestCostQuota(t, requestIDs[i])
		require.Positive(t, single,
			"[%s] expected a positive settled charge (the retry attempt's reconciled quota)",
			requestIDs[i])
		sumSingleCharges += single
	}

	// EXACTLY-ONCE: the user's total decrement must equal the sum of per-request
	// charges. If attempt-1's pre-consume leaked for any request, totalDecrement
	// would be roughly 2x the per-request charge for that request (double charge);
	// if a charge was over-refunded, totalDecrement would fall short.
	require.EqualValues(t, sumSingleCharges, totalDecrement,
		"concurrent cross-channel retry must charge for EXACTLY ONE settled request each; "+
			"sum_of_per_request_charges=%d actual_total_decrement=%d (a ~2x total decrement "+
			"indicates a leaked attempt-1 pre-consume = double charge; a shortfall indicates over-refund)",
		sumSingleCharges, totalDecrement)

	// Same model/tokens for every request => every settled charge should be equal.
	// This catches a partial leak that happens to keep the sum self-consistent but
	// inflates individual rows.
	expectedSingle := requestCostQuota(t, requestIDs[0])
	for i := 1; i < n; i++ {
		require.EqualValues(t, expectedSingle, requestCostQuota(t, requestIDs[i]),
			"[%s] settled charge must match the other identical requests", requestIDs[i])
	}
}
