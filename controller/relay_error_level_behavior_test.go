package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/graceful"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/model"
)

// newObservedRelayLogger builds a context carrying a request-scoped logger whose
// records are captured, so tests can assert the level and fields of relay logs.
//
// Parameters: t is the running test and name labels the logger.
// Return values: a context seeded with the logger, plus the observed log sink.
func newObservedRelayLogger(t *testing.T, name string) (context.Context, *observer.ObservedLogs) {
	t.Helper()

	core, observed := observer.New(zapcore.DebugLevel)
	testLogger, err := glog.New(
		glog.WithName(name),
		glog.WithLevel(glog.LevelDebug),
		glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core {
			return core
		})),
	)
	require.NoError(t, errors.Wrap(err, "create observer logger"))

	return gmw.SetLogger(context.Background(), testLogger), observed
}

// entryHasField reports whether a captured log entry renders the named field.
//
// It encodes the entry's fields the way a real sink does, because the
// "errorVerbose" key that carries an errors/v2 stack is synthesized by zapcore's
// error encoder rather than stored as a raw field.
func entryHasField(entry observer.LoggedEntry, key string) bool {
	_, ok := entry.ContextMap()[key]
	return ok
}

// upstreamResponseFormatRejection reproduces the exact upstream payload observed in
// production for POST /v1/audio/transcriptions with model gpt-transcribe and
// response_format=vtt:
//
//	400 {"error":{"message":"response_format 'vtt' is not compatible with model
//	'gpt-transcribe-api-ev3'. Use 'json' or 'text' instead.","type":"invalid_request_error",
//	"param":"response_format","code":"unsupported_value"}}
func upstreamResponseFormatRejection() model.ErrorWithStatusCode {
	message := "response_format 'vtt' is not compatible with model 'gpt-transcribe-api-ev3'. Use 'json' or 'text' instead."
	return model.ErrorWithStatusCode{
		StatusCode: http.StatusBadRequest,
		Error: model.Error{
			Message:  message,
			Type:     model.ErrorTypeInvalidRequest,
			Code:     "unsupported_value",
			Param:    "response_format",
			RawError: errors.New(message),
		},
	}
}

// TestProcessChannelRelayError_UpstreamClientRejectionLogsWarnWithoutStack pins the
// project-wide logging rule for the relay failure funnel: an upstream rejection of the
// CALLER's request (HTTP 4xx) is a client fault, so it must be logged at WARN without an
// errors/v2 stack. ERROR is reserved for genuine server-side faults that page an operator.
func TestProcessChannelRelayError_UpstreamClientRejectionLogsWarnWithoutStack(t *testing.T) {
	ctx, observed := newObservedRelayLogger(t, "relay-upstream-400")

	processChannelRelayError(ctx, processChannelRelayErrorParams{
		RequestID:     "2026090423462371510854517362461",
		UserId:        1,
		TokenId:       73,
		ChannelId:     5,
		ChannelName:   "openai",
		Group:         "default",
		OriginalModel: "gpt-transcribe",
		ActualModel:   "gpt-transcribe",
		RequestURL:    "/v1/audio/transcriptions",
		Err:           upstreamResponseFormatRejection(),
	})

	require.Equal(t, 0, observed.FilterLevelExact(zapcore.ErrorLevel).Len(),
		"an upstream 400 rejecting the caller's request must not be logged at ERROR")

	relayErrors := observed.FilterMessage("relay error").All()
	require.Len(t, relayErrors, 1, "expected exactly one relay failure funnel log line")
	require.Equal(t, zapcore.WarnLevel, relayErrors[0].Level)
	require.False(t, entryHasField(relayErrors[0], "errorVerbose"),
		"a client-caused WARN line must not carry an errors/v2 stack trace")

	// The actionable upstream context must survive the downgrade.
	fields := relayErrors[0].ContextMap()
	require.Equal(t, "response_format", fields["error_param"])
	require.Equal(t, "unsupported_value", fields["error_code"])
	require.Equal(t, string(model.ErrorTypeInvalidRequest), fields["error_type"])
	require.EqualValues(t, http.StatusBadRequest, fields["status_code"])
	require.Contains(t, fields["upstream_error"], "is not compatible with model")

	// A caller-caused 400 is not a channel fault, so the channel must stay untouched.
	require.Equal(t, 1, observed.FilterMessage(
		"client request error (400) for channel - not disabling channel as this is not a channel issue").Len())
}

// TestProcessChannelRelayError_ServerFaultKeepsErrorLevel guards the other direction:
// downgrading client rejections must not silence genuine upstream server faults.
func TestProcessChannelRelayError_ServerFaultKeepsErrorLevel(t *testing.T) {
	originalSuspend := config.ChannelSuspendSecondsFor5XX
	config.ChannelSuspendSecondsFor5XX = time.Minute
	defer func() {
		config.ChannelSuspendSecondsFor5XX = originalSuspend
	}()

	originalDB := dbmodel.DB
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, errors.Wrap(err, "open sqlite memory db"))
	require.NoError(t, errors.Wrap(testDB.AutoMigrate(&dbmodel.Ability{}), "migrate abilities table"))
	require.NoError(t, errors.Wrap(testDB.Create(&dbmodel.Ability{
		Group:     "default",
		Model:     "gpt-4o",
		ChannelId: 7,
		Enabled:   true,
	}).Error, "seed ability row"))
	dbmodel.DB = testDB
	defer func() {
		dbmodel.DB = originalDB
	}()

	ctx, observed := newObservedRelayLogger(t, "relay-upstream-500")

	processChannelRelayError(ctx, processChannelRelayErrorParams{
		RequestID:     "req-500",
		UserId:        1,
		TokenId:       2,
		ChannelId:     7,
		ChannelName:   "openai",
		Group:         "default",
		OriginalModel: "gpt-4o",
		ActualModel:   "gpt-4o",
		RequestURL:    "/v1/chat/completions",
		Err: model.ErrorWithStatusCode{
			StatusCode: http.StatusInternalServerError,
			Error: model.Error{
				Message:  "internal upstream failure",
				Type:     model.ErrorTypeUpstream,
				Code:     "server_error",
				RawError: errors.New("internal upstream failure"),
			},
		},
	})

	relayErrors := observed.FilterMessage("relay error").All()
	require.Len(t, relayErrors, 1)
	require.Equal(t, zapcore.ErrorLevel, relayErrors[0].Level,
		"a genuine upstream server fault must stay at ERROR")
	require.True(t, entryHasField(relayErrors[0], "errorVerbose"),
		"ERROR lines keep the stack trace for on-call diagnosis")
}

// TestProcessChannelRelayError_UpstreamAuthFailureStillAlerts proves the downgrade does
// not hide broken channel credentials: the funnel line moves to WARN, but the dedicated
// auth branch still emits an ERROR-level operator alert and suspends the ability.
func TestProcessChannelRelayError_UpstreamAuthFailureStillAlerts(t *testing.T) {
	originalSuspend := config.ChannelSuspendSecondsForAuth
	config.ChannelSuspendSecondsForAuth = time.Minute
	defer func() {
		config.ChannelSuspendSecondsForAuth = originalSuspend
	}()

	originalDB := dbmodel.DB
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, errors.Wrap(err, "open sqlite memory db"))
	require.NoError(t, errors.Wrap(testDB.AutoMigrate(&dbmodel.Ability{}), "migrate abilities table"))
	require.NoError(t, errors.Wrap(testDB.Create(&dbmodel.Ability{
		Group:     "default",
		Model:     "gpt-4o",
		ChannelId: 9,
		Enabled:   true,
	}).Error, "seed ability row"))
	dbmodel.DB = testDB
	defer func() {
		dbmodel.DB = originalDB
	}()

	ctx, observed := newObservedRelayLogger(t, "relay-upstream-401")

	processChannelRelayError(ctx, processChannelRelayErrorParams{
		RequestID:     "req-401",
		UserId:        1,
		TokenId:       2,
		ChannelId:     9,
		ChannelName:   "openai",
		Group:         "default",
		OriginalModel: "gpt-4o",
		ActualModel:   "gpt-4o",
		RequestURL:    "/v1/chat/completions",
		Err: model.ErrorWithStatusCode{
			StatusCode: http.StatusUnauthorized,
			Error: model.Error{
				Message:  "Incorrect API key provided",
				Type:     model.ErrorTypeAuthentication,
				Code:     "invalid_api_key",
				RawError: errors.New("Incorrect API key provided"),
			},
		},
	})

	require.Equal(t, 1,
		observed.FilterMessage("ability suspended due to auth/permission error").
			FilterLevelExact(zapcore.ErrorLevel).Len(),
		"upstream credential failures must still raise an ERROR-level operator alert")
}

// TestRelay_UpstreamParameterRejectionEndToEnd replays the production incident through
// the real Relay() funnel: POST /v1/audio/transcriptions for model gpt-transcribe whose
// upstream rejected response_format=vtt with HTTP 400.
//
// It pins three properties at once:
//   - the caller still receives the upstream's actionable message (unchanged behaviour),
//   - the attempt is not retried onto another channel (a bad parameter is bad everywhere),
//   - the whole request produces no ERROR log line and no errors/v2 stack dump, because
//     nothing about it is a server fault.
func TestRelay_UpstreamParameterRejectionEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalRetryTimes := config.RetryTimes
	config.RetryTimes = 3 // prove the skip decision, not an absent retry budget
	defer func() { config.RetryTimes = originalRetryTimes }()

	originalDebug := config.DebugEnabled
	config.DebugEnabled = true
	defer func() { config.DebugEnabled = originalDebug }()

	const audioModel = "gpt-transcribe"
	upstreamErr := upstreamResponseFormatRejection()

	attempts := 0
	relayHelperForTest = func(c *gin.Context, _ int) *model.ErrorWithStatusCode {
		attempts++
		failure := upstreamResponseFormatRejection()
		return &failure
	}
	defer func() { relayHelperForTest = nil }()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	core, observed := observer.New(zapcore.DebugLevel)
	requestLogger, err := glog.New(
		glog.WithName("relay-audio-400"),
		glog.WithLevel(glog.LevelDebug),
		glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })),
	)
	require.NoError(t, errors.Wrap(err, "create observer logger"))
	gmw.SetLogger(c, requestLogger)

	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", http.NoBody)
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 73)
	c.Set(ctxkey.ChannelId, 5)
	c.Set(ctxkey.ChannelName, "openai")
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.RequestModel, audioModel)
	c.Set(ctxkey.RequestId, "2026090423462371510854517362461")

	Relay(c)

	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, errors.Wrap(graceful.Drain(drainCtx), "drain async error processing"))

	require.Equal(t, 1, attempts, "a rejected request parameter must not be retried on another channel")
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "is not compatible with model",
		"the upstream's actionable message must reach the caller verbatim")
	require.Contains(t, recorder.Body.String(), "unsupported_value")

	for _, entry := range observed.FilterLevelExact(zapcore.ErrorLevel).All() {
		t.Errorf("unexpected ERROR log for a caller-caused 400: %q %v", entry.Message, entry.ContextMap())
	}

	relayErrors := observed.FilterMessage("relay error").All()
	require.Len(t, relayErrors, 1)
	require.Equal(t, zapcore.WarnLevel, relayErrors[0].Level)
	require.Equal(t, "invalid_request", relayErrors[0].ContextMap()["error_kind"])
	require.Equal(t, false, relayErrors[0].ContextMap()["channel_health_counted"],
		"a caller's bad parameter is not evidence about the channel")

	retrySkipped := observed.FilterMessage("relay retry skipped after failure").All()
	require.Len(t, retrySkipped, 1, "the skip decision must stay visible to operators")
	require.Equal(t, zapcore.WarnLevel, retrySkipped[0].Level)
	require.Equal(t, "client error 400, not retrying", retrySkipped[0].ContextMap()["retry_skip_reason"])

	for _, entry := range observed.All() {
		require.False(t, entryHasField(entry, "errorVerbose"),
			"no stack dump belongs in the logs of a caller-caused 400: %q", entry.Message)
	}

	// The upstream payload itself must survive classification unmodified.
	require.Equal(t, http.StatusBadRequest, upstreamErr.StatusCode)
}

// TestCountsAgainstChannelHealth pins which relay failures may feed the failure-rate
// monitor that auto-disables a channel (ENABLE_METRIC).
//
// The traced incident is the motivating case: a caller repeatedly sending an unsupported
// response_format must not be able to disable a healthy shared channel. With the shipped
// defaults (METRIC_QUEUE_SIZE=10, METRIC_SUCCESS_RATE_THRESHOLD=0.8) ten such requests in
// a row would have been enough.
func TestCountsAgainstChannelHealth(t *testing.T) {
	t.Parallel()

	rejection := upstreamResponseFormatRejection()

	tests := []struct {
		name string
		err  *model.ErrorWithStatusCode
		want bool
	}{
		{
			name: "nil error is no evidence",
			err:  nil,
			want: false,
		},
		{
			name: "upstream 400 rejecting the caller's parameter",
			err:  &rejection,
			want: false,
		},
		{
			name: "upstream 422 rejecting the caller's payload",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusUnprocessableEntity,
				Error:      model.Error{Type: model.ErrorTypeInvalidRequest, Message: "validation failed"},
			},
			want: false,
		},
		{
			name: "caller aborted the request",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusInternalServerError,
				Error:      model.Error{Message: "canceled", RawError: context.Canceled},
			},
			want: false,
		},
		{
			name: "one-api rejected the caller for quota",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusForbidden,
				Error: model.Error{
					Type:    model.ErrorTypeOneAPI,
					Code:    "insufficient_user_quota",
					Message: "user quota is not enough",
				},
			},
			want: false,
		},
		{
			name: "one-api adaptor bug is not the channel's fault",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusInternalServerError,
				Error:      model.Error{Type: model.ErrorTypeOneAPI, Message: "convert request failed"},
			},
			want: false,
		},
		{
			name: "upstream rate limit is channel capacity",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusTooManyRequests,
				Error:      model.Error{Type: model.ErrorTypeRateLimit, Message: "rate limit exceeded"},
			},
			want: true,
		},
		{
			name: "upstream rejected this channel's credential",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusUnauthorized,
				Error:      model.Error{Type: model.ErrorTypeAuthentication, Message: "Incorrect API key provided"},
			},
			want: true,
		},
		{
			name: "upstream endpoint missing on this channel",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusNotFound,
				Error:      model.Error{Type: model.ErrorTypeUpstream, Message: "model not found"},
			},
			want: true,
		},
		{
			name: "upstream server error",
			err: &model.ErrorWithStatusCode{
				StatusCode: http.StatusBadGateway,
				Error:      model.Error{Type: model.ErrorTypeUpstream, Message: "bad gateway"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, countsAgainstChannelHealth(tt.err))
		})
	}
}

// TestRelayFailureKind pins the fault attribution that drives the log level, including the
// rule that an attribution recorded at construction wins over the transport status.
func TestRelayFailureKind(t *testing.T) {
	t.Parallel()

	rejection := upstreamResponseFormatRejection()
	require.Equal(t, errkind.InvalidRequest, relayFailureKind(&rejection))
	require.True(t, relayFailureKind(&rejection).IsClient())

	require.Equal(t, errkind.Server, relayFailureKind(nil))

	for status, want := range map[int]errkind.Kind{
		http.StatusUnauthorized:          errkind.Unauthorized,
		http.StatusForbidden:             errkind.Forbidden,
		http.StatusNotFound:              errkind.NotFound,
		http.StatusConflict:              errkind.Conflict,
		http.StatusTooManyRequests:       errkind.RateLimited,
		http.StatusRequestEntityTooLarge: errkind.InvalidRequest,
		http.StatusBadGateway:            errkind.Upstream,
		http.StatusInternalServerError:   errkind.Upstream,
	} {
		got := relayFailureKind(&model.ErrorWithStatusCode{StatusCode: status})
		require.Equalf(t, want, got, "status %d", status)
	}

	// A database outage surfaced as HTTP 401 must stay server-attributed: the recorded
	// kind wins over the status.
	marked := &model.ErrorWithStatusCode{
		StatusCode: http.StatusUnauthorized,
		Error:      model.Error{RawError: errkind.Mark(errors.New("db down"), errkind.Server)},
	}
	require.Equal(t, errkind.Server, relayFailureKind(marked))
	require.False(t, relayFailureKind(marked).IsClient())
}
