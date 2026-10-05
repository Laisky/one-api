package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/realtime"
)

// liveBudgetModel is the documented Developer API Live model whose bundled
// prices are $0.75/M text input, $3/M audio input, $4.50/M text output and
// $12/M audio output, i.e. 0.375, 1.5, 2.25 and 6 quota per token.
const liveBudgetModel = "gemini-3.8-live"

// liveBudgetTextQuotaPerToken is the published text input price in quota.
const liveBudgetTextQuotaPerToken = 0.375

// liveBudgetEnv is one end-to-end Live billing fixture: the production
// RelayRealtime handler, a loopback Google upstream and an in-memory SQLite
// quota ledger. Fields are read by the owning test goroutine only after the
// sessions finished; counters shared with server goroutines are atomic.
type liveBudgetEnv struct {
	t         *testing.T
	userID    int
	tokenID   int
	channelID int
	gateway   string
	model     string
	logs      *observer.ObservedLogs
	handlers  sync.WaitGroup
	requests  atomic.Int64
	requestMu sync.Mutex
	requestID []string
}

// unmeteredLiveGate funds all work for transport tests that bypass admission.
type unmeteredLiveGate struct{}

// Commit ignores receipts. Parameters: record is unused. Returns: none.
func (unmeteredLiveGate) Commit(realtime.Record) {}

// Ensure funds everything. Parameters: pending is unused. Returns: nil.
func (unmeteredLiveGate) Ensure(realtime.Estimate) error { return nil }

// Finish ignores evidence. Parameters: evidence is unused. Returns: none.
func (unmeteredLiveGate) Finish(realtime.Estimate) {}

// liveBudgetServe scripts one provider session after setupComplete. Parameters:
// conn is the provider-side socket. Returns: an error for fixture violations.
type liveBudgetServe func(conn *websocket.Conn) error

// liveBudgetOptions seeds one ledger. UserQuota and TokenQuota are balances,
// Unlimited marks the token unlimited, Group is the channel group ratio and
// Vertex selects the Vertex AI OAuth backend with explicit channel prices.
type liveBudgetOptions struct {
	UserQuota, TokenQuota int64
	Unlimited, Vertex     bool
	Group                 float64
	Model                 string
}

// newLiveBudgetEnv provisions the user/token/channel ledger and both sockets.
// Parameters: t owns cleanup; opts seeds balances and backend; serve scripts
// every provider session. Returns: a fixture whose gateway runs the unmodified
// production RelayRealtime handler.
func newLiveBudgetEnv(t *testing.T, opts liveBudgetOptions, serve liveBudgetServe) *liveBudgetEnv {
	t.Helper()
	userQuota, tokenQuota, unlimited := opts.UserQuota, opts.TokenQuota, opts.Unlimited
	modelName := opts.Model
	if modelName == "" {
		modelName = liveBudgetModel
	}
	gin.SetMode(gin.TestMode)
	setupTokenAuthListModelsEnv(t)
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	// In-memory SQLite gives each connection a separate database.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	previousLog, previousBatch := model.LOG_DB, config.BatchUpdateEnabled
	previousLogEnabled, previousTraceMode := config.IsLogConsumeEnabled(), config.TraceWriteMode
	model.LOG_DB, config.BatchUpdateEnabled, config.TraceWriteMode = model.DB, false, "batch"
	config.SetLogConsumeEnabled(true)
	require.NoError(t, model.DB.AutoMigrate(&model.Log{}))
	env := &liveBudgetEnv{t: t, userID: 9601, tokenID: 9602, channelID: 9603}
	t.Cleanup(func() {
		env.handlers.Wait()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		require.NoError(t, graceful.Drain(ctx))
		model.LOG_DB, config.BatchUpdateEnabled, config.TraceWriteMode = previousLog, previousBatch, previousTraceMode
		config.SetLogConsumeEnabled(previousLogEnabled)
	})
	userUUID := "018f0000-0000-7000-8000-000000009601"
	tokenUUID := "018f0000-0000-7000-8000-000000009602"
	channelUUID := "018f0000-0000-7000-8000-000000009603"
	require.NoError(t, model.DB.Create(&model.User{Id: env.userID, UUID: userUUID, Username: "live-budget-user",
		Password: "hash", Status: model.UserStatusEnabled, Group: "default", Quota: userQuota}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: env.tokenID, UUID: tokenUUID, UserId: env.userID,
		UserUUID: &userUUID, Key: "live-budget-token-key", Name: "live-budget-token", Status: model.TokenStatusEnabled,
		ExpiredTime: -1, RemainQuota: tokenQuota, UnlimitedQuota: unlimited}).Error)
	channelType := channeltype.Gemini
	if opts.Vertex {
		channelType = channeltype.VertextAI
	}
	channel := &model.Channel{Id: env.channelID, UUID: channelUUID, Type: channelType, Name: "live-budget-channel",
		Status: model.ChannelStatusEnabled, Group: "default", Models: modelName}
	channelConfig := model.ChannelConfig{}
	if opts.Vertex || modelName != liveBudgetModel {
		// Vertex and operator-configured models require explicit channel prices.
		require.NoError(t, channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{modelName: vertexLivePrices()}))
	}
	if opts.Vertex {
		channelConfig = model.ChannelConfig{VertexAIProjectID: "operator-project", Region: "europe-west4",
			VertexAIADC: `{"type":"service_account","client_email":"fixture@example.invalid"}`}
		cacheKey := fmt.Sprintf("vertexai-token-%d", env.channelID)
		vertexai.Cache.Set(cacheKey, "vertex-fixture-token", time.Hour)
		t.Cleanup(func() { vertexai.Cache.Delete(cacheKey) })
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.First(channel, env.channelID).Error)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		credential := r.Header.Get("X-Goog-Api-Key") == "live-budget-provider-key"
		if opts.Vertex {
			credential = r.Header.Get("Authorization") == "Bearer vertex-fixture-token" &&
				r.URL.Path == "/ws/google.cloud.aiplatform.v1.LlmBidiService/BidiGenerateContent"
		}
		if !credential {
			t.Error("provider credential was not forwarded")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		if err := conn.SetReadDeadline(time.Now().Add(time.Minute)); err != nil {
			t.Error(err)
			return
		}
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Error(errors.Wrap(err, "read provider setup"))
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"setupComplete":{}}`)); err != nil {
			t.Error(errors.Wrap(err, "acknowledge provider setup"))
			return
		}
		if err := serve(conn); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(upstream.Close)

	core, observed := observer.New(zapcore.DebugLevel)
	requestLogger, err := glog.New(glog.WithName("live-budget"), glog.WithLevel(glog.LevelDebug),
		glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
	require.NoError(t, err)
	env.logs = observed
	engine := gin.New()
	engine.GET("/v1/realtime", func(c *gin.Context) {
		env.handlers.Add(1)
		defer env.handlers.Done()
		requestID := fmt.Sprintf("live-budget-%d", env.requests.Add(1))
		env.requestMu.Lock()
		env.requestID = append(env.requestID, requestID)
		env.requestMu.Unlock()
		gmw.SetLogger(c, requestLogger)
		// These are the values TokenAuth and Distribute publish in production.
		c.Request.Header.Set("Authorization", "Bearer live-budget-provider-key")
		c.Set(ctxkey.RequestId, requestID)
		c.Set(ctxkey.Id, env.userID)
		c.Set(ctxkey.UserUUID, userUUID)
		c.Set(ctxkey.Group, "default")
		c.Set(ctxkey.TokenId, env.tokenID)
		c.Set(ctxkey.TokenUUID, tokenUUID)
		c.Set(ctxkey.TokenName, "live-budget-token")
		c.Set(ctxkey.TokenQuota, tokenQuota)
		c.Set(ctxkey.TokenQuotaUnlimited, unlimited)
		c.Set(ctxkey.Channel, channelType)
		c.Set(ctxkey.Config, channelConfig)
		c.Set(ctxkey.ChannelId, env.channelID)
		c.Set(ctxkey.ChannelUUID, channelUUID)
		c.Set(ctxkey.ChannelName, channel.Name)
		c.Set(ctxkey.ChannelModel, channel)
		c.Set(ctxkey.ChannelRatio, opts.Group)
		c.Set(ctxkey.BaseURL, upstream.URL)
		c.Set(ctxkey.RequestModel, modelName)
		RelayRealtime(c)
	})
	gateway := httptest.NewServer(engine)
	t.Cleanup(gateway.Close)
	env.gateway = "ws" + strings.TrimPrefix(gateway.URL, "http") + "/v1/realtime?model=" + modelName
	env.model = modelName
	return env
}

// connect opens one downstream session and completes native setup. Parameters:
// none. Returns: the client socket, or a test failure when admission is refused.
func (e *liveBudgetEnv) connect() *websocket.Conn {
	e.t.Helper()
	conn, response, err := websocket.DefaultDialer.Dial(e.gateway, http.Header{"Authorization": []string{"Bearer caller-fixture"}})
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	require.NoError(e.t, err)
	e.t.Cleanup(func() { _ = conn.Close() })
	require.NoError(e.t, conn.SetReadDeadline(time.Now().Add(time.Minute)))
	require.NoError(e.t, conn.WriteMessage(websocket.TextMessage, []byte(`{"setup":{"model":"`+e.model+`"}}`)))
	_, ack, err := conn.ReadMessage()
	require.NoError(e.t, err)
	require.JSONEq(e.t, `{"setupComplete":{}}`, string(ack))
	return conn
}

// connectSetup opens one downstream session with a caller-chosen native setup.
// Parameters: setup is the first client frame. Returns: the client socket.
func (e *liveBudgetEnv) connectSetup(setup string) *websocket.Conn {
	e.t.Helper()
	conn, response, err := websocket.DefaultDialer.Dial(e.gateway, http.Header{"Authorization": []string{"Bearer caller-fixture"}})
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	require.NoError(e.t, err)
	e.t.Cleanup(func() { _ = conn.Close() })
	require.NoError(e.t, conn.SetReadDeadline(time.Now().Add(time.Minute)))
	require.NoError(e.t, conn.WriteMessage(websocket.TextMessage, []byte(setup)))
	_, ack, err := conn.ReadMessage()
	require.NoError(e.t, err)
	require.JSONEq(e.t, `{"setupComplete":{}}`, string(ack))
	return conn
}

// awaitClose reads until the gateway closes the session or wait elapses.
// Parameters: conn is a client socket and wait bounds a quiet session. Returns:
// the gateway close reason, or "" when the client had to close the socket itself.
func awaitClose(conn *websocket.Conn, wait time.Duration) string {
	_ = conn.SetReadDeadline(time.Now().Add(wait))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				return closeErr.Text
			}
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client_done"), time.Now().Add(time.Second))
			return ""
		}
	}
}

// finish waits for every handler and detached settlement. Parameters: none.
// Returns: none; afterwards balances and logs are durable and stable.
func (e *liveBudgetEnv) finish() {
	e.t.Helper()
	done := make(chan struct{})
	go func() { e.handlers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(45 * time.Second):
		e.t.Fatal("Live handlers did not finish")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(e.t, graceful.Drain(ctx))
}

// balances returns the durable user quota and token remaining quota.
// Parameters: none. Returns: both balances after settlement.
func (e *liveBudgetEnv) balances() (int64, int64) {
	e.t.Helper()
	var user model.User
	var token model.Token
	require.NoError(e.t, model.DB.First(&user, e.userID).Error)
	require.NoError(e.t, model.DB.First(&token, e.tokenID).Error)
	return user.Quota, token.RemainQuota
}

// consumeLogs returns every settled consume row per gateway request.
// Parameters: none. Returns: rows keyed by request ID.
func (e *liveBudgetEnv) consumeLogs() map[string][]model.Log {
	e.t.Helper()
	e.requestMu.Lock()
	ids := append([]string(nil), e.requestID...)
	e.requestMu.Unlock()
	result := map[string][]model.Log{}
	for _, id := range ids {
		var rows []model.Log
		require.NoError(e.t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&rows).Error)
		result[id] = rows
	}
	return result
}

// liveTextFrame returns one valid native text input of exactly size bytes.
// Parameters: size is the encoded frame length. Returns: the JSON frame.
func liveTextFrame(size int) string {
	const prefix, suffix = `{"realtimeInput":{"text":"`, `"}}`
	return prefix + strings.Repeat("a", size-len(prefix)-len(suffix)) + suffix
}

// liveAudioOutputFrame returns one provider audio chunk of seconds of 24 kHz PCM.
// Parameters: seconds is the chunk duration. Returns: a native modelTurn frame.
func liveAudioOutputFrame(seconds int) []byte {
	data := base64.StdEncoding.EncodeToString(make([]byte, 48000*seconds))
	return []byte(`{"serverContent":{"modelTurn":{"parts":[{"inlineData":{"mimeType":"audio/pcm;rate=24000","data":"` + data + `"}}]}}}`)
}

// liveReceiptFrame returns a terminal Live receipt in the real provider shape:
// thinking is reported outside totalTokenCount. Parameters: textIn, audioIn,
// remainder, textOut, audioOut and thoughts are token counts. Returns: JSON.
func liveReceiptFrame(textIn, audioIn, remainder, textOut, audioOut, thoughts int64) []byte {
	prompt, response := textIn+audioIn+remainder, textOut+audioOut
	return fmt.Appendf(nil, `{"serverContent":{"turnComplete":true},"usageMetadata":{"promptTokenCount":%d,"responseTokenCount":%d,"totalTokenCount":%d,"thoughtsTokenCount":%d,"promptTokensDetails":[{"modality":"TEXT","tokenCount":%d},{"modality":"AUDIO","tokenCount":%d}],"responseTokensDetails":[{"modality":"TEXT","tokenCount":%d},{"modality":"AUDIO","tokenCount":%d}]}}`,
		prompt, response, prompt+response, thoughts, textIn, audioIn, textOut, audioOut)
}

// liveReceiptQuota prices one fixture receipt with the published Live rates.
// Parameters: as liveReceiptFrame. Returns: the exact unrounded quota of that
// receipt. The unattributed prompt remainder uses the cheapest (text) rate and
// thinking reported outside the totals is billed only beyond the text output.
func liveReceiptQuota(textIn, audioIn, remainder, textOut, audioOut, thoughts int64) float64 {
	return float64(textIn+remainder)*0.375 + float64(audioIn)*1.5 + float64(max(textOut, thoughts))*2.25 + float64(audioOut)*6
}
