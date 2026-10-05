package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// securityImageAccount prepares real isolated quota ledgers; tests remain serial
// because the application stores its database and HTTP clients globally.
func securityImageAccount(t *testing.T, owner, token int64, unlimited bool) {
	t.Helper()
	t.Cleanup(setupCacheBillingLogTest(t))
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.UserRequestCost{}))
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 1).Update("quota", owner).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"remain_quota": token, "used_quota": 0, "unlimited_quota": unlimited}).Error)
	oldLog, oldBatch := config.IsLogConsumeEnabled(), config.BatchUpdateEnabled
	config.SetLogConsumeEnabled(true)
	config.BatchUpdateEnabled = false
	t.Cleanup(func() { config.SetLogConsumeEnabled(oldLog); config.BatchUpdateEnabled = oldBatch })
}

// securityImageChannel creates a parsed operator tariff rather than mocking the
// pricing resolver. Unknown custom image models remain valid compatibility cases.
func securityImageChannel(t *testing.T, kind int, name string, tariff string) *model.Channel {
	t.Helper()
	pricing := fmt.Sprintf(`{%q:%s}`, name, tariff)
	ch := &model.Channel{Id: 1, Type: kind, Name: "security-image-fixture", Status: model.ChannelStatusEnabled, ModelConfigs: &pricing}
	require.NoError(t, model.DB.Create(ch).Error)
	return ch
}

// securityImageContext selects the actual registered adaptor and real image
// controller; a fresh context is used for every request, including concurrent ones.
func securityImageContext(ch *model.Channel, name, target, id string, n int, group float64) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	raw := fmt.Sprintf(`{"model":%q,"prompt":"synthetic lighthouse","n":%d}`, name, n)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	gmw.SetLogger(c, logger.Logger)
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 1)
	c.Set(ctxkey.ChannelId, 1)
	c.Set(ctxkey.TokenName, "relay-cache-log-token")
	c.Set(ctxkey.RequestId, id)
	c.Set(ctxkey.ChannelModel, ch)
	c.Set(ctxkey.ChannelRatio, group)
	c.Set(ctxkey.ContentType, "application/json")
	metalib.Set2Context(c, &metalib.Meta{Mode: relaymode.ImagesGenerations, ChannelType: ch.Type, APIType: channeltype.ToAPIType(ch.Type), ChannelId: 1, UserId: 1, TokenId: 1, TokenName: "relay-cache-log-token", BaseURL: target, APIKey: "synthetic.local", OriginModelName: name, ActualModelName: name, RequestURLPath: "/v1/images/generations", StartTime: time.Now()})
	return c
}

// securityImageClient prevents the fixture from using any inherited HTTP client.
func securityImageClient(t *testing.T, server *httptest.Server) {
	t.Helper()
	old := client.HTTPClient
	local := server.Client()
	local.Timeout = 10 * time.Second
	client.HTTPClient = local
	t.Cleanup(func() { client.HTTPClient = old })
	t.Cleanup(server.Close)
}

// securityImageLedger checks both balances and exactly one reconciled cost/log,
// rather than treating client success or a logged amount as proof of a debit.
func securityImageLedger(t *testing.T, id string, owner, token, charge int64) {
	t.Helper()
	var user model.User
	var key model.Token
	require.NoError(t, model.DB.First(&user, 1).Error)
	require.NoError(t, model.DB.First(&key, 1).Error)
	require.Equal(t, owner, user.Quota)
	require.Equal(t, token, key.RemainQuota)
	var cost model.UserRequestCost
	require.NoError(t, model.DB.Where("request_id = ?", id).First(&cost).Error)
	require.Equal(t, charge, cost.Quota)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type IN ?", id, []int{model.LogTypeConsume, model.LogTypeProvisional}).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, model.LogTypeConsume, logs[0].Type)
	require.EqualValues(t, charge, logs[0].Quota)
}

// TestSecurityImageEveryPaidContract reserves every positive quote before the
// actual HTTP request, including the legacy ratio contract and finite tokens.
func TestSecurityImageEveryPaidContract(t *testing.T) {
	for _, tc := range []struct {
		name, tariff          string
		owner, token, reserve int64
		count                 int
		group                 float64
		unlimited, reject     bool
	}{
		{"ratio_funded", `{"ratio":10}`, 100, 100, 20, 2, 1, false, false},
		{"ratio_finite_token_reject", `{"ratio":10}`, 100, 19, 0, 2, 1, false, true},
		{"ratio_owner_reject", `{"ratio":10}`, 19, 100, 0, 2, 1, false, true},
		{"ratio_unlimited", `{"ratio":10}`, 100, 0, 20, 2, 1, true, false},
		{"ratio_group_multiplier", `{"ratio":10}`, 100, 100, 40, 2, 2, false, false},
		{"catalog_free", `{"ratio":0}`, 0, 0, 0, 2, 1, false, false},
		{"free_group", `{"ratio":10}`, 0, 0, 0, 2, 0, false, false},
		{"per_image_funded", `{"image":{"price_per_image_usd":0.00002}}`, 100, 100, 20, 2, 1, false, false},
		{"per_image_token_reject", `{"image":{"price_per_image_usd":0.00002}}`, 100, 19, 0, 2, 1, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name, kind, path := "tenant-image-fixture", channeltype.OpenAI, "/v1/images/generations"
			if tc.name == "catalog_free" {
				name, kind, path = "cogview-3-flash", channeltype.Zhipu, "/api/paas/v4/images/generations"
			}
			securityImageAccount(t, tc.owner, tc.token, tc.unlimited)
			ch := securityImageChannel(t, kind, name, tc.tariff)
			var calls atomic.Int32
			var seenUser, seenToken atomic.Int64
			var bad atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var u model.User
				var k model.Token
				if model.DB.First(&u, 1).Error != nil || model.DB.First(&k, 1).Error != nil {
					bad.Store(true)
				}
				seenUser.Store(u.Quota)
				seenToken.Store(k.RemainQuota)
				var payload struct {
					Model string `json:"model"`
					N     int    `json:"n"`
				}
				if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&payload) != nil || payload.Model != name || (kind == channeltype.OpenAI && payload.N != tc.count) || r.URL.Path != path {
					bad.Store(true)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"created":1,"data":[{"b64_json":"AQ=="}],"usage":null}`)
			}))
			securityImageClient(t, server)
			c := securityImageContext(ch, name, server.URL, "img-"+tc.name, tc.count, tc.group)
			apiErr := RelayImageHelper(c, relaymode.ImagesGenerations)
			if tc.reject {
				require.Zero(t, calls.Load(), "unfunded paid work must never reach the upstream")
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
				var u model.User
				var k model.Token
				require.NoError(t, model.DB.First(&u, 1).Error)
				require.NoError(t, model.DB.First(&k, 1).Error)
				require.Equal(t, tc.owner, u.Quota)
				require.Equal(t, tc.token, k.RemainQuota)
				var rows int64
				require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Count(&rows).Error)
				require.Zero(t, rows)
				return
			}
			require.Nil(t, apiErr)
			require.EqualValues(t, 1, calls.Load())
			require.False(t, bad.Load())
			require.Equal(t, tc.owner-tc.reserve, seenUser.Load(), "reserve before dispatch, not after delivery")
			wantToken := tc.token - tc.reserve
			if tc.unlimited {
				wantToken = tc.token
			}
			require.Equal(t, wantToken, seenToken.Load())
			securityImageLedger(t, "img-"+tc.name, tc.owner-tc.reserve, wantToken, tc.reserve)
		})
	}
}

// TestSecurityImageKnownVideoEndpointReject treats the published catalog's video
// identity as an endpoint constraint. Local replies do not claim real Vidu image
// endpoint acceptance, and custom unknown image providers stay covered above.
func TestSecurityImageKnownVideoEndpointReject(t *testing.T) {
	for _, name := range []string{"viduq1-image", "viduq1-start-end", "viduq1-text", "vidu2-image", "vidu2-start-end", "vidu2-reference", "cogvideox-flash"} {
		t.Run(name, func(t *testing.T) {
			securityImageAccount(t, 1_000_000, 1_000_000, false)
			ch := securityImageChannel(t, channeltype.Zhipu, name, `{"ratio":10}`)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"created":1,"data":[{"url":"https://example.invalid/synthetic.png"}]}`)
			}))
			securityImageClient(t, server)
			c := securityImageContext(ch, name, server.URL, "wrong-endpoint-"+name, 1, 1)
			apiErr := RelayImageHelper(c, relaymode.ImagesGenerations)
			require.Zero(t, calls.Load(), "catalog video output cannot authorize image dispatch")
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.Equal(t, "model_endpoint_mismatch", apiErr.Code)
			var user model.User
			var token model.Token
			var costs int64
			require.NoError(t, model.DB.First(&user, 1).Error)
			require.NoError(t, model.DB.First(&token, 1).Error)
			require.EqualValues(t, 1_000_000, user.Quota)
			require.EqualValues(t, 1_000_000, token.RemainQuota)
			require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Count(&costs).Error)
			require.Zero(t, costs)
		})
	}
}

// TestSecurityImageObservedReceiptDebt settles an authoritative receipt even
// when it exceeds both the prepaid quote and remaining balance. The complete
// receipt and expected tariff are the existing GPT Image fixture contract.
func TestSecurityImageObservedReceiptDebt(t *testing.T) {
	const name = "gpt-image-2.5-flare"
	for _, unlimited := range []bool{false, true} {
		t.Run(fmt.Sprint(unlimited), func(t *testing.T) {
			securityImageAccount(t, 50_000, 50_000, unlimited)
			ch := securityImageChannel(t, channeltype.OpenAI, name, `{"image":{"price_per_image_usd":0.1}}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"created":1,"data":[{"b64_json":"AQ=="}],"usage":{"input_tokens":1000,"output_tokens":10000}}`)
			}))
			securityImageClient(t, server)
			c := securityImageContext(ch, name, server.URL, "image-receipt-debt", 1, 1)
			require.Nil(t, RelayImageHelper(c, relaymode.ImagesGenerations))
			token := int64(-102_500)
			if unlimited {
				token = 50_000
			}
			securityImageLedger(t, "image-receipt-debt", -102_500, token, 152_500)
			require.True(t, c.GetBool(ctxkey.BillingReconciled))
		})
	}
}

// TestSecurityImageConcurrentAdmission freezes accepted HTTP calls so no final
// settlement can conceal a missing outstanding reservation. Twelve requests
// compete for exactly five funded two-image quotes on real atomic SQLite APIs.
func TestSecurityImageConcurrentAdmission(t *testing.T) {
	securityImageAccount(t, 101, 101, false)
	ch := securityImageChannel(t, channeltype.OpenAI, "tenant-image-fixture", `{"ratio":10}`)
	decisions := make(chan struct{}, 12)
	release := make(chan struct{})
	var calls, accepted, rejected atomic.Int32
	var bad atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		decisions <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"created":1,"data":[{"b64_json":"AQ=="}],"usage":null}`)
	}))
	securityImageClient(t, server)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			c := securityImageContext(ch, "tenant-image-fixture", server.URL, fmt.Sprintf("image-concurrent-%d", i), 2, 1)
			ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			err := RelayImageHelper(c, relaymode.ImagesGenerations)
			if err != nil {
				rejected.Add(1)
				if err.StatusCode != http.StatusForbidden {
					bad.Store(true)
				}
				decisions <- struct{}{}
				return
			}
			accepted.Add(1)
		}(i)
	}
	allDecided := true
	for i := 0; i < 12; i++ {
		select {
		case <-decisions:
		case <-time.After(5 * time.Second):
			allDecided = false
		}
	}
	var heldOwner model.User
	var heldToken model.Token
	ownerErr := model.DB.First(&heldOwner, 1).Error
	tokenErr := model.DB.First(&heldToken, 1).Error
	unblock()
	workers.Wait()
	require.True(t, allDecided, "every request must reach a bounded admission decision")
	require.NoError(t, ownerErr)
	require.NoError(t, tokenErr)
	require.False(t, bad.Load())
	require.EqualValues(t, 5, calls.Load())
	require.EqualValues(t, 5, accepted.Load())
	require.EqualValues(t, 7, rejected.Load())
	require.EqualValues(t, 1, heldOwner.Quota)
	require.EqualValues(t, 1, heldToken.RemainQuota)
	var user model.User
	var token model.Token
	var logs int64
	var sum int64
	require.NoError(t, model.DB.First(&user, 1).Error)
	require.NoError(t, model.DB.First(&token, 1).Error)
	require.EqualValues(t, 1, user.Quota)
	require.EqualValues(t, 1, token.RemainQuota)
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&logs).Error)
	require.EqualValues(t, 5, logs)
	require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Select("COALESCE(SUM(quota),0)").Scan(&sum).Error)
	require.EqualValues(t, 100, sum)
}

// TestSecurityImageSettlementFailureStaysPending injects a real SQLite failure
// after paid dispatch. Failed persistence must not be reported as a final charge;
// the original hold and provisional audit stay available for manual recovery.
func TestSecurityImageSettlementFailureStaysPending(t *testing.T) {
	securityImageAccount(t, 50000, 50000, false)
	const name = "gpt-image-2.5-flare"
	ch := securityImageChannel(t, channeltype.OpenAI, name, `{"image":{"price_per_image_usd":0.1}}`)
	var triggerFailed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if model.DB.Exec("CREATE TRIGGER security_reject_image_debit BEFORE UPDATE OF quota ON users BEGIN SELECT RAISE(ABORT, 'synthetic image settlement failure'); END").Error != nil {
			triggerFailed.Store(true)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"created":1,"data":[{"b64_json":"AQ=="}],"usage":{"input_tokens":1000,"output_tokens":10000}}`)
	}))
	securityImageClient(t, server)
	c := securityImageContext(ch, name, server.URL, "image-persistence-failure", 1, 1)
	require.Nil(t, RelayImageHelper(c, relaymode.ImagesGenerations), "delivery preceded final persistence")
	require.False(t, triggerFailed.Load())
	require.False(t, c.GetBool(ctxkey.BillingReconciled), "failed debit must not declare successful settlement")
	var user model.User
	var token model.Token
	var cost model.UserRequestCost
	var logs []model.Log
	require.NoError(t, model.DB.First(&user, 1).Error)
	require.NoError(t, model.DB.First(&token, 1).Error)
	require.EqualValues(t, 0, user.Quota)
	require.EqualValues(t, 0, token.RemainQuota)
	require.NoError(t, model.DB.Where("request_id = ?", "image-persistence-failure").First(&cost).Error)
	require.EqualValues(t, 50000, cost.Quota)
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "image-persistence-failure").Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, model.LogTypeProvisional, logs[0].Type)
	require.EqualValues(t, 50000, logs[0].Quota)
}
