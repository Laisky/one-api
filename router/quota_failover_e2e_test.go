package router

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

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// quotaRouterResult records actual upstream HTTP calls, client response and
// settled account state from the public router with an isolated SQLite ledger.
type quotaRouterResult struct {
	status           int
	body             string
	failedCalls      int64
	healthyCalls     int64
	quotaDelta       int64
	billed           []string
	logQuota         int64
	positiveConsumes int
	tokenDebit       int64
	failedUsed       int64
	healthyUsed      int64
}

// quotaFlatBody is the provider's flat AccountQuotaExceeded envelope from issue #625.
const quotaFlatBody = `{"message":"You have exceeded the monthly usage quota","type":"TooManyRequests","param":"quota","code":"AccountQuotaExceeded"}`

// quotaRouterSpec scripts the failing provider and the routing configuration.
type quotaRouterSpec struct {
	// failStatus and failBody are the failing provider's response; zero values
	// select the issue #625 HTTP 429 flat AccountQuotaExceeded envelope.
	failStatus int
	failBody   string
	retries    int
	memory     bool
	alternate  string
	requests   int
}

// quotaFailure returns the issue #625 HTTP 429 failure in its flat or nested envelope.
func quotaFailure(nested bool) quotaRouterSpec {
	body := quotaFlatBody
	if nested {
		body = `{"error":` + body + `}`
	}
	return quotaRouterSpec{failStatus: http.StatusTooManyRequests, failBody: body}
}

// runQuotaRouter sends one authenticated HTTP request through the shipped router,
// real channel distribution, OpenAI-compatible adaptor and billing machinery.
// Parameters select envelope shape, budget, cache path and alternate eligibility.
// Returns the client response, provider counters and final account debit.
func runQuotaRouter(t *testing.T, nested bool, retries int, memory bool, alternate string) quotaRouterResult {
	spec := quotaFailure(nested)
	spec.retries, spec.memory, spec.alternate, spec.requests = retries, memory, alternate, 1
	return runQuotaRouterSpec(t, spec)
}

// runQuotaRouterRequests sends sequential public HTTP requests without rebuilding
// the channel cache, so persisted cooldown must influence the next request.
func runQuotaRouterRequests(t *testing.T, nested bool, retries int, memory bool, alternate string, requests int) quotaRouterResult {
	spec := quotaFailure(nested)
	spec.retries, spec.memory, spec.alternate, spec.requests = retries, memory, alternate, requests
	return runQuotaRouterSpec(t, spec)
}

// runQuotaRouterSpec runs spec.requests sequential public HTTP requests against a
// failing provider (the higher-priority channel) and an optional healthy alternate.
func runQuotaRouterSpec(t *testing.T, spec quotaRouterSpec) quotaRouterResult {
	t.Helper()
	retries, memory, alternate, requests := spec.retries, spec.memory, spec.alternate, max(spec.requests, 1)
	if spec.failStatus == 0 {
		failure := quotaFailure(false)
		spec.failStatus, spec.failBody = failure.failStatus, failure.failBody
	}
	var failedCalls, healthyCalls atomic.Int64
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failedCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(spec.failStatus)
		if _, err := io.WriteString(w, spec.failBody); err != nil {
			t.Errorf("write failing provider response: %v", err)
		}
	}))
	t.Cleanup(failed.Close)
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		healthyCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"id":"chatcmpl-quota","object":"chat.completion","created":1767225600,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"healthy"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`); err != nil {
			t.Errorf("write healthy response: %v", err)
		}
	}))
	t.Cleanup(healthy.Close)
	initialBase := failed.URL
	if alternate == "direct" {
		initialBase = healthy.URL
	}
	engine, key := ambiguousKeysRouter(t, initialBase, channeltype.OpenAICompatible, passthroughKeysAllowedModel, true)
	client.HTTPClient = failed.Client()
	client.HTTPClient.Timeout = 2 * time.Second
	oldRetries, oldBatch := config.RetryTimes, config.BatchUpdateEnabled
	config.RetryTimes, config.BatchUpdateEnabled = retries, false
	t.Cleanup(func() { config.RetryTimes, config.BatchUpdateEnabled = oldRetries, oldBatch })
	if alternate != "absent" && alternate != "direct" {
		priority := int64(-10)
		base := healthy.URL
		ch := &model.Channel{Id: 929294, UUID: uuid.NewString(), Type: channeltype.OpenAICompatible, Status: model.ChannelStatusEnabled, Name: "quota-healthy", Models: passthroughKeysAllowedModel, Group: "default", BaseURL: &base, Key: "fixture-only", Priority: &priority}
		switch alternate {
		case "wrong-group":
			ch.Group = "other"
		case "wrong-model":
			ch.Models = "other-model"
		case "disabled":
			ch.Status = model.ChannelStatusManuallyDisabled
		}
		require.NoError(t, model.DB.Create(ch).Error)
		require.NoError(t, ch.AddAbilities())
		if alternate == "suspended" {
			require.NoError(t, model.SuspendAbility(context.Background(), "default", passthroughKeysAllowedModel, ch.Id, time.Hour))
		}
	}
	config.MemoryCacheEnabled = memory
	if memory {
		require.NoError(t, model.InitChannelCacheContext(context.Background()))
	}
	before, _ := passthroughKeysLedger(t)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	var status int
	var body []byte
	for range requests {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer sk-"+key)
		req.Header.Set("Content-Type", "application/json")
		httpClient := server.Client()
		httpClient.Timeout = 5 * time.Second
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		body, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		drainCriticalTasksForRouter(t)
		status = resp.StatusCode
	}
	after, billed := passthroughKeysLedger(t)
	result := quotaRouterResult{status: status, body: string(body), failedCalls: failedCalls.Load(), healthyCalls: healthyCalls.Load(), quotaDelta: before - after, billed: billed}
	var token model.Token
	require.NoError(t, model.DB.Where("id = ?", passthroughKeysTokenID).Take(&token).Error)
	result.tokenDebit = passthroughKeysQuota - token.RemainQuota
	var channels []model.Channel
	require.NoError(t, model.DB.Find(&channels).Error)
	for _, ch := range channels {
		if ch.Id == 929294 || alternate == "direct" {
			result.healthyUsed += ch.UsedQuota
		} else {
			result.failedUsed += ch.UsedQuota
		}
	}
	var logs []model.Log
	require.NoError(t, model.DB.Where("user_id = ? AND type = ?", passthroughKeysUserID, model.LogTypeConsume).Find(&logs).Error)
	for _, log := range logs {
		result.logQuota += int64(log.Quota)
		if log.Quota > 0 {
			result.positiveConsumes++
		}
	}
	return result
}

// TestQuotaCooldownPublicRouter requires the next authenticated request to avoid
// the just-suspended channel without a cache rebuild, even with no retry budget.
func TestQuotaCooldownPublicRouter(t *testing.T) {
	for _, memory := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t", memory), func(t *testing.T) {
			got := runQuotaRouterRequests(t, true, 0, memory, "eligible", 2)
			require.Equal(t, http.StatusOK, got.status, got.body)
			require.EqualValues(t, 1, got.failedCalls)
			require.EqualValues(t, 1, got.healthyCalls)
			require.NotEmpty(t, got.billed)
			require.Positive(t, got.quotaDelta)
		})
	}
}

// TestQuotaFailoverPublicRouter checks the reported 429 through real HTTP auth,
// distribution, adaptor and billing with both envelopes, budgets and selectors.
func TestQuotaFailoverPublicRouter(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, memory := range []bool{false, true} {
			for _, retries := range []int{0, 20} {
				for _, alternate := range []string{"eligible", "absent", "wrong-group", "wrong-model", "disabled", "suspended"} {
					t.Run(fmt.Sprintf("nested=%t/cache=%t/retries=%d/%s", nested, memory, retries, alternate), func(t *testing.T) {
						got := runQuotaRouter(t, nested, retries, memory, alternate)
						t.Logf("status=%d failed_calls=%d healthy_calls=%d debit=%d consume_logs=%d", got.status, got.failedCalls, got.healthyCalls, got.quotaDelta, len(got.billed))
						require.EqualValues(t, 1, got.failedCalls)
						if retries > 0 && alternate == "eligible" {
							require.Equal(t, http.StatusOK, got.status, got.body)
							require.EqualValues(t, 1, got.healthyCalls)
							direct := runQuotaRouter(t, nested, 0, memory, "direct")
							require.Equal(t, http.StatusOK, direct.status, direct.body)
							require.Equal(t, direct.quotaDelta, got.quotaDelta, "abandoned hold must be refunded before successful retry")
							require.NotEmpty(t, got.billed)
							require.Positive(t, got.quotaDelta)
							require.Equal(t, got.quotaDelta, got.tokenDebit)
							require.Equal(t, got.quotaDelta, got.logQuota)
							require.Equal(t, 1, got.positiveConsumes)
							require.Zero(t, got.failedUsed)
							require.Equal(t, got.quotaDelta, got.healthyUsed)
						} else {
							require.Equal(t, http.StatusTooManyRequests, got.status, got.body)
							require.Zero(t, got.healthyCalls)
							require.Empty(t, got.billed, "an unsettled terminal hold is not a completed consume log")
							require.Positive(t, got.quotaDelta, "terminal forwarded errors retain their conservative hold")
						}
						t.Logf("status=%d failed_calls=%d healthy_calls=%d debit=%d consume_logs=%d", got.status, got.failedCalls, got.healthyCalls, got.quotaDelta, len(got.billed))
					})
				}
			}
		}
	}
}

// TestQuotaErrorMetadataPublicRouter requires equivalent provider attribution for
// flat and nested error envelopes after the complete public request pipeline.
func TestQuotaErrorMetadataPublicRouter(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(fmt.Sprintf("nested=%t", nested), func(t *testing.T) {
			got := runQuotaRouter(t, nested, 0, false, "absent")
			require.Equal(t, http.StatusTooManyRequests, got.status, got.body)
			var envelope struct {
				Error struct {
					Code  any    `json:"code"`
					Type  string `json:"type"`
					Param string `json:"param"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal([]byte(got.body), &envelope))
			require.Equal(t, "AccountQuotaExceeded", envelope.Error.Code)
			require.Equal(t, "TooManyRequests", envelope.Error.Type)
			require.Equal(t, "quota", envelope.Error.Param)
		})
	}
}

// TestQuotaErrorMetadataPublicRouterToleratesObjectParam is the public-boundary
// regression for the review's P1: a flat envelope whose optional param is an
// object must keep the provider message and attribution, and the client must
// never receive the raw provider body with its internal identifiers. HTTP 402
// shows the provider message verbatim; the relay replaces 429 messages with its
// saturation text, so only attribution is observable there.
func TestQuotaErrorMetadataPublicRouterToleratesObjectParam(t *testing.T) {
	const secret = "internal-account-123"
	for _, status := range []int{http.StatusPaymentRequired, http.StatusTooManyRequests} {
		t.Run(fmt.Sprintf("status=%d", status), func(t *testing.T) {
			got := runQuotaRouterSpec(t, quotaRouterSpec{
				failStatus: status,
				failBody:   `{"message":"You have exceeded the monthly usage quota","type":"TooManyRequests","code":"AccountQuotaExceeded","param":{"account":"` + secret + `"}}`,
				alternate:  "absent",
			})
			require.Equal(t, status, got.status, got.body)
			require.NotContains(t, got.body, secret, "the client must not receive the raw provider body")
			require.NotContains(t, got.body, "failed to unmarshal response body")
			var envelope struct {
				Error struct {
					Message string `json:"message"`
					Code    any    `json:"code"`
					Type    string `json:"type"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal([]byte(got.body), &envelope))
			if status == http.StatusPaymentRequired {
				require.Contains(t, envelope.Error.Message, "You have exceeded the monthly usage quota")
			}
			require.Equal(t, "AccountQuotaExceeded", envelope.Error.Code)
			require.Equal(t, "TooManyRequests", envelope.Error.Type)
		})
	}
}

// TestUpstreamOneAPIErrorTypeCannotVetoFailoverPublicRouter covers the review's
// error-origin trust boundary with the realistic chained-gateway case: the
// channel points at another one-api whose key ran out of quota, so that gateway
// answers 403 with its OWN local error type and a user-quota code. That type
// describes the upstream gateway, not this one; it must not be mistaken for this
// gateway's caller error, which would veto failover and leave the exhausted
// channel in rotation.
func TestUpstreamOneAPIErrorTypeCannotVetoFailoverPublicRouter(t *testing.T) {
	const chainedQuota = `{"error":{"message":"user quota is not enough","type":"one_api_error","code":"insufficient_user_quota"}}`
	for _, memory := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t/403_failover", memory), func(t *testing.T) {
			got := runQuotaRouterSpec(t, quotaRouterSpec{failStatus: http.StatusForbidden, failBody: chainedQuota, retries: 1, memory: memory, alternate: "eligible"})
			require.Equal(t, http.StatusOK, got.status, got.body)
			require.EqualValues(t, 1, got.failedCalls)
			require.EqualValues(t, 1, got.healthyCalls)
			require.Equal(t, 1, got.positiveConsumes)
			require.Zero(t, got.failedUsed)
		})
		t.Run(fmt.Sprintf("cache=%t/5xx_cooldown", memory), func(t *testing.T) {
			// A provider 5xx typed as one_api_error must still cool the ability, so
			// the next request avoids the failing channel even with no retry budget.
			got := runQuotaRouterSpec(t, quotaRouterSpec{
				failStatus: http.StatusInternalServerError,
				failBody:   `{"error":{"message":"upstream gateway failed","type":"one_api_error","code":"do_request_failed"}}`,
				memory:     memory, alternate: "eligible", requests: 2,
			})
			require.Equal(t, http.StatusOK, got.status, got.body)
			require.EqualValues(t, 1, got.failedCalls)
			require.EqualValues(t, 1, got.healthyCalls)
		})
	}
}
