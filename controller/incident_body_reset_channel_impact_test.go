package controller

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
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

	"github.com/Laisky/one-api/common/config"
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

// TestIncidentBodyResetChannelImpact proves whether a classified read reset changes routing even when automatic disabling and metric monitoring are off.
func TestIncidentBodyResetChannelImpact(t *testing.T) {
	for _, scenario := range []struct {
		name              string
		duration          time.Duration
		automaticDisable  bool
		kind              string
		counted, mutation bool
	}{
		{"reset_metrics_off_auto_off", 30 * time.Second, false, "reset", true, true},
		{"reset_metrics_off_auto_on", 30 * time.Second, true, "reset", true, true},
		{"reset_zero_duration", 0, false, "reset", true, true},
		{"caller_cancel", 30 * time.Second, false, "cancel", false, false},
		{"caller_deadline", 30 * time.Second, false, "deadline", false, false},
		{"local_conversion", 30 * time.Second, false, "conversion", false, false},
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
			}
			if scenario.mutation {
				require.Len(t, logs.FilterMessage("ability suspended due to server error (5xx)").All(), 1)
			}
			t.Logf("metric_enabled=false automatic_disable=%t duration=%s counted=%t suspension_written=%t available=%d channel_enabled=true", scenario.automaticDisable, scenario.duration, scenario.counted, scenario.mutation, available)
		})
	}
}

// TestIncidentBodyResetChannelImpactCacheRefresh exposes existing cache timing without sleeping, background consumers or changing suspension policy.
func TestIncidentBodyResetChannelImpactCacheRefresh(t *testing.T) {
	ctx, _ := incidentChannelImpactFixture(t, 30*time.Second, false)
	config.MemoryCacheEnabled = true // The fixture restores the original setting after this test.
	failure := incidentChannelImpactHandlerError(t, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET})
	require.True(t, countsAgainstChannelHealth(failure))
	incidentChannelImpactProcess(ctx, failure)
	available, err := dbmodel.CountAvailableChannels(ctx, "default", incidentBodyResetModel)
	require.NoError(t, err)
	require.Zero(t, available)
	_, err = dbmodel.CacheGetRandomSatisfiedChannelWithContext(ctx, "default", incidentBodyResetModel, false)
	require.NoError(t, err, "the current cache keeps serving an ability until its next refresh")
	dbmodel.InitChannelCache()
	_, err = dbmodel.CacheGetRandomSatisfiedChannelWithContext(ctx, "default", incidentBodyResetModel, false)
	require.Error(t, err, "a refresh during the suspension removes the only model ability")
	// Expire the synthetic suspension deterministically instead of waiting 30 seconds.
	require.NoError(t, dbmodel.DB.Exec("UPDATE abilities SET suspend_until = ? WHERE channel_id = ? AND model = ?", time.Now().UTC().Add(-time.Second), 94003, incidentBodyResetModel).Error)
	available, err = dbmodel.CountAvailableChannels(ctx, "default", incidentBodyResetModel)
	require.NoError(t, err)
	require.Equal(t, 1, available)
	_, err = dbmodel.CacheGetRandomSatisfiedChannelWithContext(ctx, "default", incidentBodyResetModel, false)
	require.Error(t, err, "the cache remains empty after expiration until another refresh")
	dbmodel.InitChannelCache()
	_, err = dbmodel.CacheGetRandomSatisfiedChannelWithContext(ctx, "default", incidentBodyResetModel, false)
	require.NoError(t, err)
	t.Log("database: pause immediate/recovery on expiry; memory: pause and recovery require refresh")
}
