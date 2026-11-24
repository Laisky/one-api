package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// strictImageRelayCase is one strict GPT Image request driven through
// RelayImageHelper against a fake provider and the SQLite ledger.
type strictImageRelayCase struct {
	// requestID uniquely identifies the request in the ledger.
	requestID string
	// channelType selects the upstream contract; zero means native OpenAI.
	channelType int
	// model is the mapped provider model.
	model string
	// path is the client endpoint, /v1/images/generations or /v1/images/edits.
	path string
	// contentType and body are the exact client request.
	contentType string
	body        []byte
	// configs is the channel's model_configs JSON; empty means none.
	configs string
	// usage is the receipt's usage JSON; "null" sends no usage.
	usage string
	// balance is the user's and the token's starting quota.
	balance int64
}

// strictImageRelayResult is what the provider, the client, and the ledger
// observed for one strictImageRelayCase.
type strictImageRelayResult struct {
	// err is the relay error returned to the client, nil on success.
	err *relaymodel.ErrorWithStatusCode
	// calls counts provider requests.
	calls int32
	// sentBody, sentContentType, and sentURL are the exact provider request
	// body, Content-Type header, and request URI.
	sentBody        []byte
	sentContentType string
	sentURL         string
	// userAtUpstream and tokenAtUpstream are the balances while the provider
	// handled the request, so start balance minus them is the hold.
	userAtUpstream, tokenAtUpstream int64
	// userQuota and tokenQuota are the settled balances.
	userQuota, tokenQuota int64
	// cost is the recorded request cost, nil when none was recorded.
	cost *model.UserRequestCost
	// consume is the settled consume log, nil when none was recorded.
	consume *model.Log
}

// runStrictImageRelay drives tc through RelayImageHelper with the real channel
// parser, the real admission and settlement path, and a provider that answers
// every request with one image and tc.usage. Parameters: t owns the fixture and
// tc describes the request. Returns: the provider, client, and ledger
// observations.
func runStrictImageRelay(t *testing.T, tc strictImageRelayCase) strictImageRelayResult {
	t.Helper()
	cleanup := setupCacheBillingLogTest(t)
	t.Cleanup(cleanup)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelGroup{}, &model.UserRequestCost{}))
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 1).Update("quota", tc.balance).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Update("remain_quota", tc.balance).Error)
	clearNativeSettledLedger(t, tc.requestID)
	oldLog := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(true)
	t.Cleanup(func() { config.SetLogConsumeEnabled(oldLog) })
	oldBatch := config.BatchUpdateEnabled
	config.BatchUpdateEnabled = false
	t.Cleanup(func() { config.BatchUpdateEnabled = oldBatch })

	channelType := tc.channelType
	if channelType == 0 {
		channelType = channeltype.OpenAI
	}
	var calls atomic.Int32
	var userAt, tokenAt atomic.Int64
	var fixtureErr atomic.Bool
	var mu sync.Mutex
	var sent []byte
	var sentContentType, sentURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			fixtureErr.Store(true)
		}
		mu.Lock()
		sent, sentContentType, sentURL = raw, r.Header.Get("Content-Type"), r.URL.RequestURI()
		mu.Unlock()
		var user model.User
		var token model.Token
		if model.DB.First(&user, 1).Error != nil || model.DB.First(&token, 1).Error != nil {
			fixtureErr.Store(true)
		}
		userAt.Store(user.Quota)
		tokenAt.Store(token.RemainQuota)
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintf(w, `{"created":1,"data":[{"b64_json":"AQ=="}],"usage":%s}`, tc.usage); err != nil {
			fixtureErr.Store(true)
		}
	}))
	t.Cleanup(upstream.Close)

	channel := &model.Channel{Id: 1, Type: channelType, Name: "strict-image-relay", Status: model.ChannelStatusEnabled}
	if tc.configs != "" {
		configs := tc.configs
		channel.ModelConfigs = &configs
	}
	require.NoError(t, model.DB.Create(channel).Error)
	if tc.configs != "" {
		require.Contains(t, channel.GetModelPriceConfigsWithContext(context.Background()), tc.model,
			"the real channel parser must recognize the test override")
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(tc.body))
	c.Request.Header.Set("Content-Type", tc.contentType)
	gmw.SetLogger(c, logger.Logger)
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 1)
	c.Set(ctxkey.TokenName, "relay-cache-log-token")
	c.Set(ctxkey.ChannelId, 1)
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.ChannelModel, channel)
	c.Set(ctxkey.ContentType, tc.contentType)
	c.Set(ctxkey.RequestId, tc.requestID)
	mode := relaymode.GetByPath(tc.path)
	metalib.Set2Context(c, &metalib.Meta{
		Mode: mode, ChannelType: channelType, APIType: channeltype.ToAPIType(channelType),
		ChannelId: 1, UserId: 1, TokenId: 1, TokenName: "relay-cache-log-token",
		BaseURL: upstream.URL, APIKey: "test-only", OriginModelName: tc.model,
		ActualModelName: tc.model, RequestURLPath: tc.path, StartTime: time.Now(),
	})

	result := strictImageRelayResult{err: RelayImageHelper(c, mode)}
	require.False(t, fixtureErr.Load(), "the fake provider must observe the ledger and answer")
	result.calls = calls.Load()
	result.userAtUpstream, result.tokenAtUpstream = userAt.Load(), tokenAt.Load()
	mu.Lock()
	result.sentBody, result.sentContentType, result.sentURL = sent, sentContentType, sentURL
	mu.Unlock()
	var user model.User
	var token model.Token
	require.NoError(t, model.DB.First(&user, 1).Error)
	require.NoError(t, model.DB.First(&token, 1).Error)
	result.userQuota, result.tokenQuota = user.Quota, token.RemainQuota
	var costs []model.UserRequestCost
	require.NoError(t, model.DB.Where("request_id = ?", tc.requestID).Find(&costs).Error)
	require.LessOrEqual(t, len(costs), 1, "a request records at most one cost row")
	if len(costs) == 1 {
		result.cost = &costs[0]
	}
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", tc.requestID, model.LogTypeConsume).Find(&logs).Error)
	require.LessOrEqual(t, len(logs), 1, "a request records at most one consume log")
	if len(logs) == 1 {
		result.consume = &logs[0]
	}
	return result
}

// requireStrictImageRefusedBeforeUpstream asserts that a request was refused
// before any reservation or provider call. Parameters: t owns the assertions,
// res is the observation, balance the starting quota, and status and code the
// expected client error. Returns: none.
func requireStrictImageRefusedBeforeUpstream(t *testing.T, res strictImageRelayResult, balance int64, status int, code string) {
	t.Helper()
	require.NotNil(t, res.err, "the request must be refused")
	require.Equal(t, status, res.err.StatusCode)
	require.Equal(t, code, res.err.Code)
	require.Zero(t, res.calls, "a refused request must never reach the paid provider")
	require.Equal(t, balance, res.userQuota, "a refused request must not move the user balance")
	require.Equal(t, balance, res.tokenQuota, "a refused request must not move the token balance")
	require.Nil(t, res.cost, "a refused request records no request cost")
	require.Nil(t, res.consume, "a refused request records no consume log")
}

// requireStrictImageSettled asserts the hold taken before the provider call and
// the settled charge on balances, the request cost, and the consume log.
// Parameters: t owns the assertions, res is the observation, balance the
// starting quota, hold the expected reservation, charge the expected final
// charge, and source the expected billing_source label. Returns: none.
func requireStrictImageSettled(t *testing.T, res strictImageRelayResult, balance, hold, charge int64, source string) {
	t.Helper()
	require.Nil(t, res.err)
	require.Equal(t, int32(1), res.calls)
	require.Equal(t, balance-hold, res.userAtUpstream, "the user hold is taken before the provider call")
	require.Equal(t, balance-hold, res.tokenAtUpstream, "the token hold is taken before the provider call")
	require.Equal(t, balance-charge, res.userQuota, "the user balance settles at the charge")
	require.Equal(t, balance-charge, res.tokenQuota, "the token balance settles at the charge")
	require.NotNil(t, res.cost)
	require.Equal(t, charge, res.cost.Quota, "the recorded request cost is the charge")
	require.NotNil(t, res.consume)
	require.Equal(t, int(charge), res.consume.Quota)
	require.Contains(t, res.consume.Content, "billing_source="+source)
}
