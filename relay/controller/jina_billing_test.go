package controller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/jina"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// jinaAuditObservation captures physical balances and the wire request at the
// instant an upstream call arrives; the test asserts it on the test goroutine.
type jinaAuditObservation struct {
	UserQuota, TokenQuota int64
	Body                  map[string]any
	Err                   error
}

// jinaAuditContext constructs a real channel/request fixture and enables consume
// logs. The returned request ID is bounded to the production varchar(32) limit.
func jinaAuditContext(t *testing.T, path, body, base, actual string, balance int64, unlimited bool, group, inputRatio float64) (*gin.Context, string) {
	t.Helper()
	billingAccountingSetup(t, balance)
	oldBatch := config.BatchUpdateEnabled
	config.BatchUpdateEnabled = false
	t.Cleanup(func() { config.BatchUpdateEnabled = oldBatch })
	config.SetLogConsumeEnabled(true)
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).Updates(map[string]any{"remain_quota": balance, "used_quota": 0, "unlimited_quota": unlimited}).Error)
	channel := &model.Channel{Id: fallbackChannelID, Type: channeltype.Jina, Name: "jina-audit", Key: "jina-fixture-key"}
	if inputRatio >= 0 {
		require.NoError(t, channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{actual: {Ratio: inputRatio, CompletionRatio: 4}}))
	}
	hash := sha256.Sum256([]byte(t.Name()))
	requestID := fmt.Sprintf("ja-%x", hash[:10])
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Delete(&model.Log{}).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer jina-fixture-key")
	gmw.SetLogger(c, logger.Logger)
	for key, value := range map[string]any{
		ctxkey.Channel: channeltype.Jina, ctxkey.ChannelId: fallbackChannelID, ctxkey.ChannelModel: channel,
		ctxkey.TokenId: fallbackTokenID, ctxkey.TokenName: "fallback-token", ctxkey.Id: fallbackUserID,
		ctxkey.Group: "default", ctxkey.ModelMapping: map[string]string{"alias": actual}, ctxkey.ChannelRatio: group,
		ctxkey.RequestModel: "alias", ctxkey.BaseURL: base, ctxkey.ContentType: "application/json", ctxkey.RequestId: requestID,
		ctxkey.Username: "response-fallback", ctxkey.UserObj: &model.User{Id: fallbackUserID, Quota: balance},
		ctxkey.Config: model.ChannelConfig{}, ctxkey.TokenQuotaUnlimited: unlimited, ctxkey.TokenQuota: balance,
	} {
		c.Set(key, value)
	}
	return c, requestID
}

// jinaAuditAssertLedger compares physical debits, token usage, one consume log,
// and the request-cost record after all detached settlement work has drained.
func jinaAuditAssertLedger(t *testing.T, requestID string, start, charge int64, unlimited, estimated bool) {
	t.Helper()
	drainCriticalTasks(t)
	require.Equal(t, start-charge, reloadUserQuota(t))
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	if unlimited {
		require.Equal(t, start, token.RemainQuota)
		require.Zero(t, token.UsedQuota)
	} else {
		require.Equal(t, start-charge, token.RemainQuota)
		require.Equal(t, charge, token.UsedQuota)
	}
	require.Equal(t, charge, requestCostQuota(t, requestID))
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1, "one settled consume row, not an orphan provisional or duplicate")
	require.EqualValues(t, charge, logs[0].Quota)
	if estimated {
		require.Equal(t, true, logs[0].Metadata["billing_estimated"])
		require.NotEmpty(t, logs[0].Metadata["billing_estimate_reason"])
	} else {
		require.NotEqual(t, true, logs[0].Metadata["billing_estimated"])
	}
}

// TestJinaBillingHTTPBehavior exercises actual HTTP conversion and SQLite
// settlement, including malformed envelopes, missing/zero usage, model aliases,
// explicit free groups, overrides and trusted/unlimited token admission.
func TestJinaBillingHTTPBehavior(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, actual, response string
		unlimited                          bool
		group, override                    float64
		charge                             int64
		wantErr, estimated                 bool
	}{
		{"embedding", "/v1/embeddings", `{"model":"alias","input":["hello"]}`, "jina-embeddings-v3", `{"data":[{"embedding":[0.1]}],"usage":{"total_tokens":123}}`, false, 1, -1, 4, false, false},
		{"unlimited", "/v1/embeddings", `{"model":"alias","input":["hello"]}`, "jina-embeddings-v3", `{"data":[],"usage":{"total_tokens":123}}`, true, 1, -1, 4, false, false},
		{"missing", "/v1/embeddings", `{"model":"alias","input":["hello"]}`, "jina-embeddings-v3", `{"data":[]}`, false, 1, -1, -1, true, true},
		{"zero", "/v1/embeddings", `{"model":"alias","input":["hello"]}`, "jina-embeddings-v3", `{"data":[],"usage":{"total_tokens":0}}`, false, 1, -1, -1, true, true},
		{"negative", "/v1/embeddings", `{"model":"alias","input":["hello"]}`, "jina-embeddings-v3", `{"data":[],"usage":{"total_tokens":-1}}`, false, 1, -1, -1, true, true},
		{"bad_results_measured", "/v1/embeddings", `{"model":"alias","input":["hello"]}`, "jina-embeddings-v3", `{"data":{},"usage":{"total_tokens":123}}`, false, 1, -1, 4, true, false},
		{"rerank", "/v1/rerank", `{"model":"alias","query":"q","documents":["one","two"],"top_n":1}`, "jina-reranker-v3.5", `{"results":[],"usage":{"total_tokens":4321}}`, false, 1, -1, 109, false, false},
		{"rerank_missing", "/v1/rerank", `{"model":"alias","query":"q","documents":["one","two"],"top_n":1}`, "jina-reranker-v3.5", `{"results":[]}`, false, 1, -1, -1, true, true},
		{"group_rate", "/v1/embeddings", `{"model":"alias","input":["hello"]}`, "jina-embeddings-v3", `{"data":[],"usage":{"total_tokens":123}}`, false, 2, -1, 7, false, false},
		{"override", "/v1/embeddings", `{"model":"alias","input":["hello"]}`, "jina-embeddings-v3", `{"data":[],"usage":{"total_tokens":123}}`, false, 1, 1, 123, false, false},
		{"free_group", "/v1/embeddings", `{"model":"alias","input":["hello"]}`, "jina-embeddings-v3", `{"data":[],"usage":{"total_tokens":123}}`, false, 0, -1, 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(100000000)
			observations := make(chan jinaAuditObservation, 1)
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var user model.User
				var token model.Token
				var body map[string]any
				err := model.DB.First(&user, fallbackUserID).Error
				if err == nil {
					err = model.DB.First(&token, fallbackTokenID).Error
				}
				if err == nil {
					err = json.NewDecoder(r.Body).Decode(&body)
				}
				observations <- jinaAuditObservation{user.Quota, token.RemainQuota, body, err}
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, tc.response); err != nil {
					return
				}
			}))
			defer upstream.Close()
			previous := client.HTTPClient
			client.HTTPClient = upstream.Client()
			defer func() { client.HTTPClient = previous }()
			c, requestID := jinaAuditContext(t, tc.path, tc.body, upstream.URL, tc.actual, balance, tc.unlimited, tc.group, tc.override)
			var apiErr *relaymodel.ErrorWithStatusCode
			if tc.path == "/v1/rerank" {
				apiErr = RelayRerankHelper(c)
			} else {
				apiErr = RelayTextHelper(c)
			}
			if tc.wantErr {
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
			} else {
				require.Nil(t, apiErr)
			}
			observed := <-observations
			require.NoError(t, observed.Err)
			require.Equal(t, tc.actual, observed.Body["model"])
			reserved := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
			require.Equal(t, balance-reserved, observed.UserQuota, "real reservation must precede upstream, even for trusted tokens")
			if !tc.unlimited {
				require.Equal(t, balance-reserved, observed.TokenQuota)
			}
			charge := tc.charge
			if charge < 0 {
				charge = reserved
				require.Positive(t, charge)
			}
			jinaAuditAssertLedger(t, requestID, balance, charge, tc.unlimited, tc.estimated)
			if tc.wantErr {
				require.True(t, JinaAttemptMayHaveCost(c), "automatic replay could create a second paid attempt")
			}
		})
	}
}

// TestJinaBillingAdmissionRejectsUnboundedWork verifies no upstream request and
// no balance mutation for opaque media, invalid rates or insufficient funds.
func TestJinaBillingAdmissionRejectsUnboundedWork(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		balance    int64
		group      float64
	}{
		{"pdf", `{"model":"alias","input":[{"pdf":"https://example.com/a.pdf"}]}`, 1000000, 1},
		{"audio", `{"model":"alias","input":[{"audio":"https://example.com/a.mp3"}]}`, 1000000, 1},
		{"funds", `{"model":"alias","input":["hello"]}`, 1, 1},
		{"invalid_rate", `{"model":"alias","input":["hello"]}`, 1000000, math.NaN()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
			defer upstream.Close()
			previous := client.HTTPClient
			client.HTTPClient = upstream.Client()
			defer func() { client.HTTPClient = previous }()
			c, _ := jinaAuditContext(t, "/v1/embeddings", tc.body, upstream.URL, "jina-embeddings-v5-omni-small", tc.balance, false, tc.group, -1)
			require.NotNil(t, RelayTextHelper(c))
			drainCriticalTasks(t)
			require.Zero(t, calls.Load())
			require.Equal(t, tc.balance, reloadUserQuota(t))
		})
	}
}

// jinaFailedTransport cancels the request after dispatch was attempted.
type jinaFailedTransport struct{ cancel context.CancelFunc }

// RoundTrip returns a cancellation error to model an uncertain, possibly paid
// upstream failure without relying on external networks or sleep timing.
func (r jinaFailedTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	r.cancel()
	return nil, context.Canceled
}

// TestJinaBillingCancelledDispatchRetainsAuditableHold verifies a cancelled
// request cannot erase the reservation or leave its request-cost record at zero.
func TestJinaBillingCancelledDispatchRetainsAuditableHold(t *testing.T) {
	const balance = int64(1000000)
	c, requestID := jinaAuditContext(t, "/v1/embeddings", `{"model":"alias","input":["hello"]}`, "https://api.jina.ai", "jina-embeddings-v3", balance, false, 1, -1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	previous := client.HTTPClient
	client.HTTPClient = &http.Client{Transport: jinaFailedTransport{cancel}}
	defer func() { client.HTTPClient = previous }()
	require.NotNil(t, RelayTextHelper(c))
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	reserved := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
	require.Positive(t, reserved)
	jinaAuditAssertLedger(t, requestID, balance, reserved, false, true)
	ResetPerAttemptBillingForRetry(context.Background(), c)
	drainCriticalTasks(t)
	require.Equal(t, balance-reserved, reloadUserQuota(t), "uncertain paid attempts cannot be refunded for replay")
	require.True(t, JinaAttemptMayHaveCost(c))
	_, prepared := jina.GetBillingBudget(c)
	require.True(t, prepared)
}

// TestJinaBillingOCRFormats verifies receipt-based billing and missing-receipt
// retention through Chat Completions, Responses fallback and Claude Messages.
func TestJinaBillingOCRFormats(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages"} {
		for _, stream := range []bool{false, true} {
			for _, receipt := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%v/receipt=%v", path, stream, receipt), func(t *testing.T) {
					const balance = int64(1000000)
					observed := make(chan map[string]any, 1)
					upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var body map[string]any
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							w.WriteHeader(400)
							observed <- nil
							return
						}
						observed <- body
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							if _, err := io.WriteString(w, "data: {\"id\":\"ocr-stream\",\"object\":\"chat.completion.chunk\",\"model\":\"jina-ocr-v1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"read text\"},\"finish_reason\":null}]}\n\n"); err != nil {
								return
							}
							if _, err := io.WriteString(w, "data: {\"id\":\"ocr-stream\",\"object\":\"chat.completion.chunk\",\"model\":\"jina-ocr-v1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"); err != nil {
								return
							}
							if receipt {
								if _, err := io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"total_tokens\":120}}\n\n"); err != nil {
									return
								}
							}
							if _, err := io.WriteString(w, "data: [DONE]\n\n"); err != nil {
								return
							}
						} else {
							w.Header().Set("Content-Type", "application/json")
							usage := ""
							if receipt {
								usage = `,"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120}`
							}
							if _, err := io.WriteString(w, `{"id":"ocr-json","object":"chat.completion","model":"jina-ocr-v1","choices":[{"index":0,"message":{"role":"assistant","content":"read text"},"finish_reason":"stop"}]`+usage+`}`); err != nil {
								return
							}
						}
					}))
					defer upstream.Close()
					old := client.HTTPClient
					client.HTTPClient = upstream.Client()
					defer func() { client.HTTPClient = old }()
					payload := map[string]any{"model": "alias", "stream": stream}
					switch path {
					case "/v1/responses":
						payload["input"] = "read this image"
						payload["max_output_tokens"] = 32
					default:
						payload["messages"] = []map[string]any{{"role": "user", "content": "read this image"}}
						payload["max_tokens"] = 32
					}
					raw, err := json.Marshal(payload)
					require.NoError(t, err)
					c, requestID := jinaAuditContext(t, path, string(raw), upstream.URL, "jina-ocr-v1", balance, false, 1, -1)
					var apiErr *relaymodel.ErrorWithStatusCode
					switch path {
					case "/v1/responses":
						apiErr = RelayResponseAPIHelper(c)
					case "/v1/messages":
						apiErr = RelayClaudeMessagesHelper(c)
					default:
						apiErr = RelayTextHelper(c)
					}
					if receipt {
						require.Nil(t, apiErr)
					} else {
						require.NotNil(t, apiErr)
						require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
					}
					wire := <-observed
					require.Equal(t, "jina-ocr-v1", wire["model"])
					require.Equal(t, float64(32), wire["max_completion_tokens"])
					if stream {
						require.Equal(t, true, wire["stream_options"].(map[string]any)["include_usage"])
					}
					charge := int64(45)
					if !receipt {
						charge = c.GetInt64(ctxkey.PreConsumedQuotaAmount)
						require.Greater(t, charge, int64(45))
					}
					jinaAuditAssertLedger(t, requestID, balance, charge, false, !receipt)
				})
			}
		}
	}
}

// TestJinaBillingRejectionRefundsOnce verifies a documented admission rejection
// clears both physical balances and the request-cost estimate, without a second
// refund when preparing a safe cross-channel retry.
func TestJinaBillingRejectionRefundsOnce(t *testing.T) {
	const balance = int64(1000000)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		if _, err := io.WriteString(w, `{"error":{"message":"invalid API key","type":"authentication_error"}}`); err != nil {
			return
		}
	}))
	defer upstream.Close()
	old := client.HTTPClient
	client.HTTPClient = upstream.Client()
	defer func() { client.HTTPClient = old }()
	c, requestID := jinaAuditContext(t, "/v1/embeddings", `{"model":"alias","input":["hello"]}`, upstream.URL, "jina-embeddings-v3", balance, false, 1, -1)
	require.NotNil(t, RelayTextHelper(c))
	drainCriticalTasks(t)
	require.Equal(t, balance, reloadUserQuota(t))
	require.Zero(t, requestCostQuota(t, requestID))
	require.True(t, BillingAllowsRetry(c))
	ResetPerAttemptBillingForRetry(context.Background(), c)
	drainCriticalTasks(t)
	require.Equal(t, balance, reloadUserQuota(t))
}
