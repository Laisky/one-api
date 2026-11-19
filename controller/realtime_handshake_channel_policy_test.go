package controller

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// Identity of the single user, token and channel every realtime handshake
// scenario routes through.
const (
	realtimeHandshakeUserID    = 9701
	realtimeHandshakeTokenID   = 9702
	realtimeHandshakeChannelID = 9703
	realtimeHandshakeGroup     = "default"
	// realtimeHandshakeRevision is the channel's persisted upstream revision.
	realtimeHandshakeRevision int64 = 3
	// realtimeHandshakeQuota is a balance far above any session reservation.
	realtimeHandshakeQuota int64 = 1_000_000_000_000
)

// realtimeHandshakeEnv is one end-to-end realtime fixture: the production
// RelayRealtime handler behind a real gin engine, a loopback provider and an
// isolated in-memory SQLite ledger with the routed ability. Every attempt the
// relay hands to channel error processing is captured and then processed by
// the real policy.
type realtimeHandshakeEnv struct {
	t        *testing.T
	gateway  string
	model    string
	handlers sync.WaitGroup
	mu       sync.Mutex
	captured []processChannelRelayErrorParams
}

// realtimeHandshakeScenario selects the provider backend and routing snapshot.
// ChannelType picks the adaptor, Model the routed name, ModelMapping the
// channel's optional JSON model mapping, Provider answers the upstream
// WebSocket handshake (nil dials a closed port) and RoutedRevision is the
// upstream revision of the channel snapshot the request was routed with.
type realtimeHandshakeScenario struct {
	ChannelType    int
	Model          string
	ModelMapping   string
	Provider       http.HandlerFunc
	RoutedRevision int64
}

// newRealtimeHandshakeEnv provisions the ledger, channel, ability, provider and
// gateway for one scenario and restores every global on cleanup.
//
// Parameters:
//   - t: the running test, which owns cleanup.
//   - scenario: the backend, model, provider behavior and routed revision.
//
// Returns:
//   - *realtimeHandshakeEnv: a fixture whose gateway runs RelayRealtime.
func newRealtimeHandshakeEnv(t *testing.T, scenario realtimeHandshakeScenario) *realtimeHandshakeEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	for _, name := range []string{"CHANNEL_SUSPEND_SECONDS_FOR_429", "CHANNEL_SUSPEND_SECONDS_FOR_5XX"} {
		t.Setenv(name, "") // registers restoration of the original value
		require.NoError(t, os.Unsetenv(name))
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// A single connection keeps every query on the same in-memory database.
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&dbmodel.User{}, &dbmodel.Token{}, &dbmodel.Channel{},
		&dbmodel.ChannelGroup{}, &dbmodel.Ability{}, &dbmodel.Log{}))
	previousDB, previousLogDB := dbmodel.DB, dbmodel.LOG_DB
	previousSQLite, previousRedis := common.UsingSQLite.Load(), common.IsRedisEnabled()
	previousBatch, previousMemory := config.BatchUpdateEnabled, config.MemoryCacheEnabled
	previousAutoDisable := config.AutomaticDisableChannelEnabled
	dbmodel.DB, dbmodel.LOG_DB = db, db
	common.UsingSQLite.Store(true)
	common.SetRedisEnabled(false)
	config.BatchUpdateEnabled, config.MemoryCacheEnabled = false, false
	// Auto-disable notifies the root user; the auth suspension is asserted instead.
	config.AutomaticDisableChannelEnabled = false

	env := &realtimeHandshakeEnv{t: t, model: scenario.Model}
	processChannelRelayErrorForTest = func(ctx context.Context, params processChannelRelayErrorParams) {
		env.mu.Lock()
		env.captured = append(env.captured, params)
		env.mu.Unlock()
		processChannelRelayError(ctx, params)
	}
	t.Cleanup(func() {
		env.handlers.Wait()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		require.NoError(t, graceful.Drain(ctx))
		processChannelRelayErrorForTest = nil
		dbmodel.DB, dbmodel.LOG_DB = previousDB, previousLogDB
		common.UsingSQLite.Store(previousSQLite)
		common.SetRedisEnabled(previousRedis)
		config.BatchUpdateEnabled, config.MemoryCacheEnabled = previousBatch, previousMemory
		config.AutomaticDisableChannelEnabled = previousAutoDisable
		require.NoError(t, sqlDB.Close())
	})

	baseURL := closedLoopbackURL(t)
	if scenario.Provider != nil {
		provider := httptest.NewServer(scenario.Provider)
		t.Cleanup(provider.Close)
		baseURL = provider.URL
	}
	userUUID := "018f0000-0000-7000-8000-000000009701"
	tokenUUID := "018f0000-0000-7000-8000-000000009702"
	require.NoError(t, db.Create(&dbmodel.User{Id: realtimeHandshakeUserID, UUID: userUUID, Username: "realtime-handshake-user",
		Password: "hash", Status: dbmodel.UserStatusEnabled, Group: realtimeHandshakeGroup, Quota: realtimeHandshakeQuota}).Error)
	require.NoError(t, db.Create(&dbmodel.Token{Id: realtimeHandshakeTokenID, UUID: tokenUUID, UserId: realtimeHandshakeUserID,
		UserUUID: &userUUID, Key: "realtime-handshake-token-key", Name: "realtime-handshake-token", Status: dbmodel.TokenStatusEnabled,
		ExpiredTime: -1, RemainQuota: realtimeHandshakeQuota}).Error)
	channel := &dbmodel.Channel{Id: realtimeHandshakeChannelID, UUID: "018f0000-0000-7000-8000-000000009703",
		Type: scenario.ChannelType, Name: "realtime-handshake-channel", Key: "realtime-handshake-provider-key",
		Status: dbmodel.ChannelStatusEnabled, Group: realtimeHandshakeGroup, Models: scenario.Model, BaseURL: &baseURL}
	if scenario.ModelMapping != "" {
		channel.ModelMapping = &scenario.ModelMapping
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Model(&dbmodel.Channel{}).Where("id = ?", channel.Id).
		Update("upstream_revision", realtimeHandshakeRevision).Error)
	require.NoError(t, db.First(channel, channel.Id).Error)
	require.Equal(t, realtimeHandshakeRevision, channel.UpstreamRevision)
	require.NoError(t, channel.AddAbilities())
	// The request was routed with this snapshot; a different revision models a
	// replica whose cache still holds a replaced key or endpoint.
	routed := *channel
	routed.UpstreamRevision = scenario.RoutedRevision

	engine := gin.New()
	engine.GET("/v1/realtime", func(c *gin.Context) {
		env.handlers.Add(1)
		defer env.handlers.Done()
		gmw.SetLogger(c, logger.Logger)
		// These are the values TokenAuth and Distribute publish in production.
		c.Set(ctxkey.RequestId, "realtime-handshake-request")
		c.Set(ctxkey.Id, realtimeHandshakeUserID)
		c.Set(ctxkey.UserUUID, userUUID)
		c.Set(ctxkey.Group, realtimeHandshakeGroup)
		c.Set(ctxkey.TokenId, realtimeHandshakeTokenID)
		c.Set(ctxkey.TokenUUID, tokenUUID)
		c.Set(ctxkey.TokenName, "realtime-handshake-token")
		c.Set(ctxkey.TokenQuota, realtimeHandshakeQuota)
		c.Set(ctxkey.RequestModel, scenario.Model)
		middleware.SetupContextForSelectedChannel(c, &routed, scenario.Model)
		RelayRealtime(c)
	})
	gateway := httptest.NewServer(engine)
	t.Cleanup(gateway.Close)
	env.gateway = "ws" + strings.TrimPrefix(gateway.URL, "http") + "/v1/realtime?model=" + scenario.Model
	return env
}

// closedLoopbackURL returns an http URL on a loopback port nothing listens on.
//
// Parameters:
//   - t: the running test.
//
// Returns:
//   - string: a URL whose dial fails before any provider answers.
func closedLoopbackURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closedURL := "http://" + listener.Addr().String()
	require.NoError(t, listener.Close())
	return closedURL
}

// run opens one downstream session, waits until the gateway ends it, and waits
// for every handler and detached processor to finish.
//
// Parameters: none.
//
// Returns:
//   - []processChannelRelayErrorParams: every attempt handed to channel error
//     processing, in order.
func (e *realtimeHandshakeEnv) run() []processChannelRelayErrorParams {
	e.t.Helper()
	conn, response, err := websocket.DefaultDialer.Dial(e.gateway, nil)
	if response != nil && response.Body != nil {
		require.NoError(e.t, response.Body.Close())
	}
	if err == nil {
		// The OpenAI-protocol adaptors upgrade the client before dialing the
		// provider; wait for the close frame or the provider's own close.
		require.NoError(e.t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
		for {
			if _, _, readErr := conn.ReadMessage(); readErr != nil {
				break
			}
		}
		require.NoError(e.t, conn.Close())
	}
	done := make(chan struct{})
	go func() { e.handlers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		require.FailNow(e.t, "RelayRealtime did not return")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(e.t, graceful.Drain(ctx))
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]processChannelRelayErrorParams(nil), e.captured...)
}

// ability reads the routed ability row.
//
// Parameters: none.
//
// Returns:
//   - dbmodel.Ability: the persisted ability of the fixture channel.
func (e *realtimeHandshakeEnv) ability() dbmodel.Ability {
	e.t.Helper()
	var row dbmodel.Ability
	require.NoError(e.t, dbmodel.DB.Where("channel_id = ? AND model = ?", realtimeHandshakeChannelID, e.model).Take(&row).Error)
	return row
}

// balances reads the durable user quota and token remaining quota.
//
// Parameters: none.
//
// Returns:
//   - int64: the user quota.
//   - int64: the token remaining quota.
func (e *realtimeHandshakeEnv) balances() (int64, int64) {
	e.t.Helper()
	var user dbmodel.User
	var token dbmodel.Token
	require.NoError(e.t, dbmodel.DB.First(&user, realtimeHandshakeUserID).Error)
	require.NoError(e.t, dbmodel.DB.First(&token, realtimeHandshakeTokenID).Error)
	return user.Quota, token.RemainQuota
}

// refuseHandshake returns a provider that answers every WebSocket upgrade with
// status and a private body.
//
// Parameters:
//   - status: the HTTP status the provider answers with.
//
// Returns:
//   - http.HandlerFunc: the provider handler.
func refuseHandshake(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "private-provider-handshake-detail", status)
	}
}

// requireRealtimeHandshakeParams asserts that the captured attempt carries the
// complete routing identity of a pre-session realtime failure.
//
// Parameters:
//   - t: the running test.
//   - params: the attempt handed to channel error processing.
//   - routed: the routed model name whose ability exists.
//   - actual: the channel-mapped upstream model name.
//   - revision: the routed snapshot's upstream revision.
//   - status: the expected failure status.
//
// Returns: none.
func requireRealtimeHandshakeParams(t *testing.T, params processChannelRelayErrorParams, routed, actual string, revision int64, status int) {
	t.Helper()
	require.Equal(t, status, params.Err.StatusCode)
	require.Equal(t, realtimeHandshakeChannelID, params.ChannelId)
	require.Equal(t, "realtime-handshake-channel", params.ChannelName)
	require.Equal(t, realtimeHandshakeGroup, params.Group)
	require.Equal(t, routed, params.OriginalModel, "channel policy must address the routed ability")
	require.Equal(t, actual, params.ActualModel)
	require.Equal(t, revision, params.Revision)
	require.False(t, params.ReadOnly, "a WebSocket upgrade is a GET but opens a generation session")
	require.Equal(t, 1, params.Attempt)
	require.Equal(t, "realtime-handshake-request", params.RequestID)
	require.Equal(t, realtimeHandshakeUserID, params.UserId)
	require.Equal(t, realtimeHandshakeTokenID, params.TokenId)
	require.True(t, strings.HasPrefix(params.RequestURL, "/v1/realtime?model="), params.RequestURL)
	require.False(t, params.StartedAt.IsZero())
	require.False(t, params.CompletedAt.IsZero())
	require.Equal(t, time.UTC, params.StartedAt.Location())
	require.False(t, params.CompletedAt.Before(params.StartedAt))
	require.WithinDuration(t, time.Now().UTC(), params.CompletedAt, time.Minute)
}

// TestRealtimeHandshakeProviderFailureFeedsChannelPolicy drives the production
// RelayRealtime handler against OpenAI-protocol and Gemini Live providers that
// refuse the WebSocket handshake. A provider 429 or 5xx must reach the same
// channel policy as the HTTP relay and cool the routed ability on the adaptive
// ladder; a provider 401 must suspend it for authentication. No session
// started, so the reservation is refunded and nothing is charged.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestRealtimeHandshakeProviderFailureFeedsChannelPolicy(t *testing.T) {
	for _, tc := range []struct {
		name        string
		channelType int
		model       string
		mapping     string
		actual      string
		status      int
		adaptive    bool
	}{
		{name: "openai_429", channelType: channeltype.OpenAI, model: "gpt-realtime", status: http.StatusTooManyRequests, adaptive: true},
		{name: "openai_503", channelType: channeltype.OpenAI, model: "gpt-realtime", status: http.StatusServiceUnavailable, adaptive: true},
		{name: "openai_401", channelType: channeltype.OpenAI, model: "gpt-realtime", status: http.StatusUnauthorized},
		{name: "openai_mapped_alias_429", channelType: channeltype.OpenAI, model: "realtime-handshake-alias",
			mapping: `{"realtime-handshake-alias":"gpt-realtime"}`, actual: "gpt-realtime", status: http.StatusTooManyRequests, adaptive: true},
		{name: "gemini_live_429", channelType: channeltype.Gemini, model: liveBudgetModel, status: http.StatusTooManyRequests, adaptive: true},
		{name: "gemini_live_500", channelType: channeltype.Gemini, model: liveBudgetModel, status: http.StatusInternalServerError, adaptive: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newRealtimeHandshakeEnv(t, realtimeHandshakeScenario{ChannelType: tc.channelType, Model: tc.model,
				ModelMapping: tc.mapping, Provider: refuseHandshake(tc.status), RoutedRevision: realtimeHandshakeRevision})
			actual := tc.actual
			if actual == "" {
				actual = tc.model
			}

			captured := env.run()
			require.Len(t, captured, 1, "the failed handshake must reach channel error processing exactly once")
			requireRealtimeHandshakeParams(t, captured[0], tc.model, actual, realtimeHandshakeRevision, tc.status)
			require.True(t, captured[0].Err.FromUpstream, "a provider handshake status is provider-caused")

			ability := env.ability()
			if tc.adaptive {
				require.Equal(t, 1, ability.CooldownFailures, "the provider failure must climb the adaptive ladder")
				require.NotNil(t, ability.AdaptiveSuspendUntil)
				require.True(t, ability.AdaptiveSuspendUntil.After(time.Now().UTC()))
			} else {
				require.NotNil(t, ability.SuspendUntil, "a provider 401 must suspend the ability for authentication")
				require.True(t, ability.SuspendUntil.After(time.Now().UTC()))
			}
			userQuota, tokenQuota := env.balances()
			require.Equal(t, realtimeHandshakeQuota, userQuota, "a session that never started is not charged")
			require.Equal(t, realtimeHandshakeQuota, tokenQuota, "the reservation of a session that never started is refunded")
		})
	}
}

// TestRealtimeHandshakeChannelPolicyControls pins the cases that must not cool
// the channel: a session that started, a dial that no provider answered, and a
// provider failure measured against a replaced channel snapshot.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestRealtimeHandshakeChannelPolicyControls(t *testing.T) {
	t.Run("started_session_is_not_a_failure", func(t *testing.T) {
		env := newRealtimeHandshakeEnv(t, realtimeHandshakeScenario{ChannelType: channeltype.OpenAI, Model: "gpt-realtime",
			RoutedRevision: realtimeHandshakeRevision, Provider: func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				_ = conn.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second))
				_ = conn.Close()
			}})

		require.Empty(t, env.run(), "a session that started is settled by billing, not channel error processing")
		ability := env.ability()
		require.Zero(t, ability.CooldownFailures)
		require.Nil(t, ability.AdaptiveSuspendUntil)
		require.Nil(t, ability.SuspendUntil)
	})
	t.Run("dial_without_provider_status_stays_local", func(t *testing.T) {
		env := newRealtimeHandshakeEnv(t, realtimeHandshakeScenario{ChannelType: channeltype.OpenAI, Model: "gpt-realtime",
			RoutedRevision: realtimeHandshakeRevision})

		captured := env.run()
		require.Len(t, captured, 1)
		requireRealtimeHandshakeParams(t, captured[0], "gpt-realtime", "gpt-realtime", realtimeHandshakeRevision, http.StatusBadGateway)
		require.False(t, captured[0].Err.FromUpstream)
		ability := env.ability()
		require.Zero(t, ability.CooldownFailures, "one-api's own dial failure is not provider evidence")
		require.Nil(t, ability.AdaptiveSuspendUntil)
		require.Nil(t, ability.SuspendUntil)
	})
	t.Run("stale_routed_revision_is_fenced", func(t *testing.T) {
		env := newRealtimeHandshakeEnv(t, realtimeHandshakeScenario{ChannelType: channeltype.OpenAI, Model: "gpt-realtime",
			Provider: refuseHandshake(http.StatusTooManyRequests), RoutedRevision: realtimeHandshakeRevision - 1})

		captured := env.run()
		require.Len(t, captured, 1)
		requireRealtimeHandshakeParams(t, captured[0], "gpt-realtime", "gpt-realtime", realtimeHandshakeRevision-1, http.StatusTooManyRequests)
		ability := env.ability()
		require.Zero(t, ability.CooldownFailures, "an attempt routed with a replaced snapshot must not cool the channel")
		require.Nil(t, ability.AdaptiveSuspendUntil)
	})
}
