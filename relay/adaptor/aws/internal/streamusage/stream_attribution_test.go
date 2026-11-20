package streamusage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"unsafe"

	"github.com/Laisky/errors/v2"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/streaming"
)

// attributionFailingWriter is a gin.ResponseWriter whose body writes fail, as a
// disconnected client's would.
type attributionFailingWriter struct {
	gin.ResponseWriter
	fault error
}

// Write fails every body write with the configured fault.
//
// Parameters:
//   - p: the bytes the renderer tried to deliver; ignored.
//
// Returns:
//   - int: always 0.
//   - error: the configured fault.
func (w *attributionFailingWriter) Write(p []byte) (int, error) {
	return 0, w.fault
}

// WriteString fails every string write with the configured fault; the SSE
// encoder writes through io.WriteString, which prefers this method.
//
// Parameters:
//   - s: the text the renderer tried to deliver; ignored.
//
// Returns:
//   - int: always 0.
//   - error: the configured fault.
func (w *attributionFailingWriter) WriteString(s string) (int, error) {
	return 0, w.fault
}

// newAttributionContext builds a gin test context for one observed AWS stream.
//
// Returns:
//   - *gin.Context: the request context with a chat-completions request.
func newAttributionContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

// attributionChunk returns a one-choice chat chunk whose text costs tokens, so
// Render must account for it before delivery.
//
// Returns:
//   - *openai.ChatCompletionsStreamResponse: the chunk.
func attributionChunk() *openai.ChatCompletionsStreamResponse {
	return &openai.ChatCompletionsStreamResponse{
		Object: "chat.completion.chunk",
		Choices: []openai.ChatCompletionsStreamResponseChoice{{
			Delta: relaymodel.Message{Role: "assistant", Content: "hello world from the provider"},
		}},
	}
}

// storeExhaustedTracker attaches a streaming quota tracker whose next funding
// attempt fails with err, exactly as an exhausted user balance does.
//
// Parameters:
//   - c: the request context that receives the tracker.
//   - err: the failure every later RecordCompletionTokens call returns.
func storeExhaustedTracker(c *gin.Context, err error) {
	tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{ModelName: "deepseek.r1-v1:0", ModelRatio: 1, GroupRatio: 1})
	field := reflect.ValueOf(tracker).Elem().FieldByName("abortErr")
	*(*error)(unsafe.Pointer(field.UnsafeAddr())) = err
	streaming.StoreTracker(c, tracker)
}

// completeReceipt records a valid usage receipt and the stop event, so that only
// the error under test decides the outcome.
//
// Parameters:
//   - t: the test.
//   - o: the observer.
func completeReceipt(t *testing.T, o *Observer) {
	t.Helper()
	in, out, total := int32(3), int32(4), int32(7)
	require.True(t, o.RecordMetadata(&types.TokenUsage{InputTokens: &in, OutputTokens: &out, TotalTokens: &total}))
	o.RecordStop()
}

// TestFinishAttributesProviderStreamFailures requires every failure the
// provider's stream caused (an SDK stream error such as ThrottlingException,
// invalid usage counters, or a stream that ended without its stop and usage
// receipt) to be provider-marked, so the relay applies ability cooldown and
// channel health instead of treating it as a one-api internal 5xx. The
// client-visible envelope (502 aws_stream_incomplete, one_api_error) is kept.
func TestFinishAttributesProviderStreamFailures(t *testing.T) {
	sdkErr := errors.New("bedrock event stream: ThrottlingException: Too many requests")
	cases := []struct {
		name   string
		finish func(*Observer) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage)
		cause  error
	}{
		{
			name: "sdk_stream_error_via_fail_and_finish",
			finish: func(o *Observer) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
				o.Fail(sdkErr)
				return o.Finish(false, sdkErr)
			},
			cause: sdkErr,
		},
		{
			name: "sdk_stream_error_only_at_finish",
			finish: func(o *Observer) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
				return o.Finish(false, sdkErr)
			},
			cause: sdkErr,
		},
		{
			name: "stream_closed_without_receipt",
			finish: func(o *Observer) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
				o.RecordStop()
				return o.Finish(false, nil)
			},
		},
		{
			name: "invalid_usage_counters",
			finish: func(o *Observer) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
				require.False(t, o.RecordMetadata(&types.TokenUsage{}))
				o.RecordStop()
				return o.Finish(false, nil)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var usage relaymodel.Usage
			o := New(newAttributionContext(), &usage)
			failure, got := tc.finish(o)
			require.NotNil(t, failure)
			require.NotNil(t, got)
			require.Equal(t, http.StatusBadGateway, failure.StatusCode)
			require.Equal(t, "aws_stream_incomplete", failure.Code)
			require.Equal(t, relaymodel.ErrorTypeOneAPI, failure.Type, "client-visible envelope is unchanged")
			require.True(t, failure.FromUpstream, "a provider stream failure must be provider-marked")
			require.False(t, failure.IsOneAPIError(), "a provider 5xx must not read as one-api's own error")
			if tc.cause != nil {
				require.ErrorIs(t, failure.RawError, tc.cause)
			}
		})
	}
}

// TestFinishKeepsLocalStreamFailuresLocal requires failures one-api produced
// itself (quota exhaustion while funding rendered output, a non-quota funding
// failure, a downstream write failure, client cancellation, deadlines and
// disconnects) to stay one-api attributed, so a user's quota or a client's
// disconnect never cools down or penalizes the channel. Quota exhaustion keeps
// its 403 insufficient_user_quota envelope.
func TestFinishKeepsLocalStreamFailuresLocal(t *testing.T) {
	fundingFault := errors.New("post consume token quota during streaming flush: database is locked")
	writeFault := errors.New("downstream connection reset")
	cases := []struct {
		name       string
		run        func(*gin.Context) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage)
		wantStatus int
		wantCode   string
	}{
		{
			name: "quota_exhausted_during_render",
			run: func(c *gin.Context) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
				storeExhaustedTracker(c, errors.WithStack(streaming.ErrQuotaExceeded))
				var usage relaymodel.Usage
				o := New(c, &usage)
				require.ErrorIs(t, o.Render(c, attributionChunk()), streaming.ErrQuotaExceeded)
				return o.Finish(false, nil)
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "insufficient_user_quota",
		},
		{
			name: "quota_error_reported_through_fail",
			run: func(c *gin.Context) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
				var usage relaymodel.Usage
				o := New(c, &usage)
				o.Fail(errors.Wrap(streaming.ErrQuotaExceeded, "flush"))
				return o.Finish(false, nil)
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "insufficient_user_quota",
		},
		{
			name: "funding_failure_during_render",
			run: func(c *gin.Context) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
				storeExhaustedTracker(c, fundingFault)
				var usage relaymodel.Usage
				o := New(c, &usage)
				require.ErrorIs(t, o.Render(c, attributionChunk()), fundingFault)
				return o.Finish(false, errors.New("late provider error must not re-attribute"))
			},
			wantStatus: http.StatusBadGateway,
			wantCode:   "aws_stream_incomplete",
		},
		{
			name: "downstream_write_failure",
			run: func(c *gin.Context) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
				c.Writer = &attributionFailingWriter{ResponseWriter: c.Writer, fault: writeFault}
				var usage relaymodel.Usage
				o := New(c, &usage)
				require.Error(t, o.Render(c, attributionChunk()))
				return o.Finish(false, nil)
			},
			wantStatus: http.StatusBadGateway,
			wantCode:   "aws_stream_incomplete",
		},
		{
			name: "request_context_cancelled",
			run: func(c *gin.Context) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
				var usage relaymodel.Usage
				o := New(c, &usage)
				o.Fail(context.Canceled)
				return o.Finish(false, nil)
			},
			wantStatus: http.StatusBadGateway,
			wantCode:   "aws_stream_incomplete",
		},
		{
			name: "sdk_error_wrapping_deadline",
			run: func(c *gin.Context) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
				var usage relaymodel.Usage
				o := New(c, &usage)
				deadline := errors.Wrap(context.DeadlineExceeded, "read event stream")
				o.Fail(deadline)
				return o.Finish(false, deadline)
			},
			wantStatus: http.StatusBadGateway,
			wantCode:   "aws_stream_incomplete",
		},
		{
			name: "client_disconnected",
			run: func(c *gin.Context) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
				var usage relaymodel.Usage
				o := New(c, &usage)
				return o.Finish(true, errors.New("event stream closed after client left"))
			},
			wantStatus: http.StatusBadGateway,
			wantCode:   "aws_stream_incomplete",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failure, usage := tc.run(newAttributionContext())
			require.NotNil(t, failure)
			require.NotNil(t, usage, "an estimate is always returned for settlement")
			require.Equal(t, tc.wantStatus, failure.StatusCode)
			require.Equal(t, tc.wantCode, failure.Code)
			require.False(t, failure.FromUpstream, "a local failure must not be provider-marked")
			require.True(t, failure.IsOneAPIError())
		})
	}
}

// TestFinishCompleteStreamHasNoFailure requires a stream with a valid receipt,
// a stop event and no error to finish cleanly with the measured usage.
func TestFinishCompleteStreamHasNoFailure(t *testing.T) {
	var usage relaymodel.Usage
	o := New(newAttributionContext(), &usage)
	completeReceipt(t, o)
	failure, got := o.Finish(false, nil)
	require.Nil(t, failure)
	require.Equal(t, 7, got.TotalTokens)
}
