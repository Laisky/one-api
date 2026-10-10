package controller

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// incidentChannelImpactBody supplies one synthetic body-read failure without a network connection.
type incidentChannelImpactBody struct{ err error }

// Read returns the configured error without provider content.
func (b incidentChannelImpactBody) Read([]byte) (int, error) { return 0, b.err }

// Close releases a fixture body that owns no external resources.
func (incidentChannelImpactBody) Close() error { return nil }

// incidentChannelImpactHandlerError constructs a failure through the actual buffered response handler.
func incidentChannelImpactHandlerError(t *testing.T, raw error) *relaymodel.ErrorWithStatusCode {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	failure, usage := openai_compatible.Handler(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: incidentChannelImpactBody{err: raw}}, 0, incidentBodyResetModel)
	require.NotNil(t, failure)
	require.Nil(t, usage)
	require.False(t, c.Writer.Written())
	require.Equal(t, errkind.Upstream, errkind.Of(failure.RawError))
	require.Equal(t, errkind.Upstream, relayFailureKind(failure))
	return failure
}

// incidentChannelImpactFixture creates a single-channel SQLite routing pool and records the actual processor's logs with metric consumers disabled.
func incidentChannelImpactFixture(t *testing.T, duration time.Duration, automaticDisable bool) (context.Context, *observer.ObservedLogs) {
	t.Helper()
	ch := incidentBodyResetChannel(94003, "channel-impact.invalid", 10)
	incidentBodyResetRoutingFixture(t, []*dbmodel.Channel{ch}, false)
	for _, ability := range []dbmodel.Ability{
		{Group: "default", Model: "unrelated-fixture-model", ChannelId: ch.Id, Enabled: true},
		{Group: "unrelated-fixture-group", Model: incidentBodyResetModel, ChannelId: ch.Id, Enabled: true},
	} {
		require.NoError(t, dbmodel.DB.Create(&ability).Error)
	}
	oldMetric, oldAuto, oldDuration := config.EnableMetric, config.AutomaticDisableChannelEnabled, config.ChannelSuspendSecondsFor5XX
	// Real metric consumers can notify administrators and have no shutdown hook.
	// Keep them disabled: log classification and the real ability UPDATE are observed here.
	config.EnableMetric, config.AutomaticDisableChannelEnabled, config.ChannelSuspendSecondsFor5XX = false, automaticDisable, duration
	t.Cleanup(func() {
		incidentBodyResetDrain(t)
		config.EnableMetric, config.AutomaticDisableChannelEnabled, config.ChannelSuspendSecondsFor5XX = oldMetric, oldAuto, oldDuration
	})
	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.New(glog.WithName("incident-channel-impact"), glog.WithLevel(glog.LevelDebug), glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
	require.NoError(t, err)
	return gmw.SetLogger(context.Background(), lg), logs
}

// incidentChannelImpactProcess exercises the production error processor without its asynchronous test replacement.
func incidentChannelImpactProcess(ctx context.Context, failure *relaymodel.ErrorWithStatusCode) {
	processChannelRelayError(ctx, processChannelRelayErrorParams{RequestID: "incident-channel-impact", UserId: 92001, TokenId: 92002, ChannelId: 94003, ChannelName: "channel-impact.invalid", Group: "default", OriginalModel: incidentBodyResetModel, ActualModel: incidentBodyResetModel, RequestURL: "/v1/chat/completions", Err: *failure})
}

// TestIncidentBodyResetChannelImpact preserves legacy read-failure availability while exercising existing provider-fault suspension controls.
func TestIncidentBodyResetChannelImpact(t *testing.T) {
	for _, scenario := range []struct {
		name              string
		duration          time.Duration
		automaticDisable  bool
		kind              string
		counted, mutation bool
	}{
		{"reset_metrics_off_auto_off", 30 * time.Second, false, "reset", false, false},
		{"reset_metrics_off_auto_on", 30 * time.Second, true, "reset", false, false},
		{"reset_zero_duration", 0, false, "reset", false, false},
		{"caller_cancel", 30 * time.Second, false, "cancel", false, false},
		{"caller_deadline", 30 * time.Second, false, "deadline", false, false},
		{"local_conversion", 30 * time.Second, false, "conversion", false, false},
		{"internal_infrastructure", 30 * time.Second, false, "infra", false, false},
		{"provider_500", 30 * time.Second, false, "provider", true, true},
		{"provider_retry_hint", 30 * time.Second, false, "hint", true, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, logs := incidentChannelImpactFixture(t, scenario.duration, scenario.automaticDisable)
			var failure *relaymodel.ErrorWithStatusCode
			switch scenario.kind {
			case "reset":
				failure = incidentChannelImpactHandlerError(t, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET})
			case "cancel":
				failure = incidentChannelImpactHandlerError(t, context.Canceled)
			case "deadline":
				failure = incidentChannelImpactHandlerError(t, context.DeadlineExceeded)
			case "conversion":
				failure = openai_compatible.ErrorWrapper(&strconv.NumError{Func: "Atoi", Num: "synthetic", Err: strconv.ErrSyntax}, "convert_request_failed", http.StatusInternalServerError)
			case "infra":
				failure = openai_compatible.ErrorWrapper(helper.ErrFFProbeUnavailable, "count_audio_tokens_failed", http.StatusInternalServerError)
			case "provider", "hint":
				message := "synthetic provider failure"
				if scenario.kind == "hint" {
					message = "synthetic provider temporarily unavailable; please retry"
				}
				failure = &relaymodel.ErrorWithStatusCode{StatusCode: http.StatusInternalServerError, Error: relaymodel.Error{Message: message, Type: relaymodel.ErrorTypeUpstream, RawError: errors.New(message)}}
			}
			require.Equal(t, scenario.counted, countsAgainstChannelHealth(failure))
			before := time.Now().UTC()
			incidentChannelImpactProcess(ctx, failure)
			after := time.Now().UTC()
			var abilities []dbmodel.Ability
			require.NoError(t, dbmodel.DB.Order("model, channel_id").Find(&abilities).Error)
			require.Len(t, abilities, 3)
			for _, ability := range abilities {
				if ability.Group != "default" || ability.Model != incidentBodyResetModel {
					require.Nil(t, ability.SuspendUntil, "other model/group abilities must remain untouched")
					continue
				}
				if scenario.mutation {
					require.NotNil(t, ability.SuspendUntil)
					require.False(t, ability.SuspendUntil.Before(before.Add(scenario.duration)))
					require.False(t, ability.SuspendUntil.After(after.Add(scenario.duration)))
				} else {
					require.Nil(t, ability.SuspendUntil)
				}
			}
			available, err := dbmodel.CountAvailableChannels(ctx, "default", incidentBodyResetModel)
			require.NoError(t, err)
			if scenario.mutation && scenario.duration > 0 {
				require.Zero(t, available)
			} else {
				require.Equal(t, 1, available)
			}
			var channel dbmodel.Channel
			require.NoError(t, dbmodel.DB.First(&channel, 94003).Error)
			require.Equal(t, dbmodel.ChannelStatusEnabled, channel.Status, "ability pause must not disable the entire channel")
			if scenario.kind != "cancel" && scenario.kind != "deadline" {
				entries := logs.FilterMessage("relay error").All()
				require.Len(t, entries, 1)
				require.Equal(t, scenario.counted, entries[0].ContextMap()["channel_health_counted"])
				if scenario.kind == "reset" {
					require.Equal(t, errkind.Upstream.String(), entries[0].ContextMap()["error_kind"], "upstream diagnostics must remain visible despite health exclusion")
				}
			}
			if scenario.mutation {
				require.Len(t, logs.FilterMessage("ability suspended due to server error (5xx)").All(), 1)
			} else {
				require.Empty(t, logs.FilterMessage("ability suspended due to server error (5xx)").All())
			}
			t.Logf("metric_enabled=false automatic_disable=%t duration=%s counted=%t suspension_written=%t available=%d channel_enabled=true", scenario.automaticDisable, scenario.duration, scenario.counted, scenario.mutation, available)
		})
	}
}

// TestIncidentBodyResetChannelImpactCacheRefresh proves a legacy read failure does not remove singleton availability before or after cache rebuilding.
func TestIncidentBodyResetChannelImpactCacheRefresh(t *testing.T) {
	ctx, _ := incidentChannelImpactFixture(t, 30*time.Second, false)
	config.MemoryCacheEnabled = true // The fixture restores the original setting after this test.
	failure := incidentChannelImpactHandlerError(t, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET})
	require.False(t, countsAgainstChannelHealth(failure))
	incidentChannelImpactProcess(ctx, failure)
	available, err := dbmodel.CountAvailableChannels(ctx, "default", incidentBodyResetModel)
	require.NoError(t, err)
	require.Equal(t, 1, available)
	_, err = dbmodel.CacheGetRandomSatisfiedChannelWithContext(ctx, "default", incidentBodyResetModel, false)
	require.NoError(t, err, "the singleton remains available before refresh")
	dbmodel.InitChannelCache()
	_, err = dbmodel.CacheGetRandomSatisfiedChannelWithContext(ctx, "default", incidentBodyResetModel, false)
	require.NoError(t, err, "refresh must preserve availability without a suspension")
	t.Log("database and memory routes remain available; no new cooldown or cache recovery gap")
}

// TestIncidentBodyResetChannelImpactRouting exercises the real Relay, async error processor and accounting in singleton and alternative synthetic pools.
func TestIncidentBodyResetChannelImpactRouting(t *testing.T) {
	for _, memory := range []bool{false, true} {
		for _, alternative := range []bool{false, true} {
			name := "singleton/database"
			if alternative {
				name = "alternative/database"
			}
			if memory {
				name = strings.ReplaceAll(name, "database", "memory")
			}
			t.Run(name, func(t *testing.T) {
				ctx, logs := incidentChannelImpactFixture(t, 30*time.Second, false)
				config.MemoryCacheEnabled = memory
				processChannelRelayErrorForTest = nil // Exercise the real tracked processor; the base fixture restores its original hook.
				var first dbmodel.Channel
				require.NoError(t, dbmodel.DB.First(&first, 94003).Error)
				if alternative {
					second := incidentBodyResetChannel(94004, "alternative-impact.invalid", 5)
					require.NoError(t, dbmodel.DB.Create(second).Error)
					require.NoError(t, second.AddAbilities())
				}
				dbmodel.InitChannelCache()
				var firstCalls, secondCalls atomic.Int32
				var hold int64
				client.HTTPClient = &http.Client{Transport: incidentBodyResetRoundTripper(func(r *http.Request) (*http.Response, error) {
					_, err := io.Copy(io.Discard, r.Body)
					require.NoError(t, err)
					var body io.ReadCloser
					switch r.URL.Host {
					case "channel-impact.invalid":
						firstCalls.Add(1)
						var user dbmodel.User
						require.NoError(t, dbmodel.DB.First(&user, 92001).Error)
						hold = incidentBodyResetBalance - user.Quota
						body = incidentBodyResetReader{}
					case "alternative-impact.invalid":
						secondCalls.Add(1)
						body = io.NopCloser(strings.NewReader(`{"id":"fixture","object":"chat.completion","model":"deepseek-flash","choices":[{"index":0,"message":{"role":"assistant","content":"fixture answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
					default:
						t.Fatalf("unexpected synthetic destination %q", r.URL.Host)
					}
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: body, Request: r}, nil
				})}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				gmw.SetLogger(c, gmw.GetLogger(ctx))
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-flash","stream":false,"max_tokens":1000,"messages":[{"role":"user","content":"synthetic impact fixture"}]}`))
				c.Request.Header.Set("Content-Type", "application/json")
				for key, value := range map[string]any{ctxkey.Id: 92001, ctxkey.TokenId: 92002, ctxkey.TokenName: "incident-fixture-token", ctxkey.Group: "default", ctxkey.RequestModel: incidentBodyResetModel, ctxkey.RequestId: "incident-impact-routing", ctxkey.TokenQuota: incidentBodyResetBalance, ctxkey.TokenQuotaUnlimited: false, ctxkey.UserObj: &dbmodel.User{Id: 92001, Quota: incidentBodyResetBalance}, ctxkey.Username: "incident-fixture-owner"} {
					c.Set(key, value)
				}
				middleware.SetupContextForSelectedChannel(c, &first, incidentBodyResetModel)
				Relay(c)
				incidentBodyResetDrain(t)
				require.EqualValues(t, 1, firstCalls.Load())
				require.Positive(t, hold)
				entries := logs.FilterMessage("relay error").All()
				require.Len(t, entries, 1)
				require.Equal(t, "upstream", entries[0].ContextMap()["error_kind"])
				require.Equal(t, false, entries[0].ContextMap()["channel_health_counted"])
				require.Empty(t, logs.FilterMessage("ability suspended due to server error (5xx)").All())
				var abilities []dbmodel.Ability
				require.NoError(t, dbmodel.DB.Find(&abilities).Error)
				for _, ability := range abilities {
					require.Nil(t, ability.SuspendUntil)
				}
				available, err := dbmodel.CountAvailableChannels(ctx, "default", incidentBodyResetModel)
				require.NoError(t, err)
				expectedAvailable := 1
				if alternative {
					expectedAvailable = 2
				}
				require.Equal(t, expectedAvailable, available)
				dbmodel.InitChannelCache()
				route, err := dbmodel.CacheGetRandomSatisfiedChannelWithContext(ctx, "default", incidentBodyResetModel, false)
				require.NoError(t, err)
				require.Equal(t, 94003, route.Id, "the original highest-priority channel remains eligible after the failed request")
				var owner dbmodel.User
				var token dbmodel.Token
				require.NoError(t, dbmodel.DB.First(&owner, 92001).Error)
				require.NoError(t, dbmodel.DB.First(&token, 92002).Error)
				require.Equal(t, owner.Quota, token.RemainQuota)
				var rows []dbmodel.Log
				require.NoError(t, dbmodel.LOG_DB.Where("request_id = ?", "incident-impact-routing").Find(&rows).Error)
				if alternative {
					require.EqualValues(t, 1, secondCalls.Load())
					require.Equal(t, http.StatusOK, recorder.Code)
					require.Equal(t, incidentBodyResetBalance-15, owner.Quota)
					require.Len(t, rows, 2)
					for _, row := range rows {
						require.Equal(t, dbmodel.LogTypeConsume, row.Type)
					}
				} else {
					require.Zero(t, secondCalls.Load())
					require.Equal(t, http.StatusInternalServerError, recorder.Code)
					require.Contains(t, recorder.Body.String(), "read_response_body_failed")
					require.Equal(t, incidentBodyResetBalance-hold, owner.Quota)
					require.Len(t, rows, 1)
					require.Equal(t, dbmodel.LogTypeProvisional, rows[0].Type)
					require.EqualValues(t, hold, rows[0].Quota)
				}
			})
		}
	}
}
