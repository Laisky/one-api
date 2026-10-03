package tracing

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// newFailureTestContext builds a gin context carrying a relay request.
//
// Parameters:
//   - t: the test, used for helper bookkeeping.
//
// Return values:
//   - *gin.Context: the context.
func newFailureTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	return c
}

// TestRecordTraceFailureFirstReportWins verifies the earliest failure survives,
// so a client disconnect that follows an upstream error does not mask its cause.
func TestRecordTraceFailureFirstReportWins(t *testing.T) {
	c := newFailureTestContext(t)
	require.Equal(t, FailureNone, TraceFailure(c))

	RecordTraceFailure(c, FailureUpstream)
	RecordTraceFailure(c, FailureClientCanceled)

	require.Equal(t, FailureUpstream, TraceFailure(c))
}

// TestRecordTraceFailureIgnoresNoOps verifies the no-op inputs stay no-ops.
func TestRecordTraceFailureIgnoresNoOps(t *testing.T) {
	require.Equal(t, FailureNone, TraceFailure(nil))
	RecordTraceFailure(nil, FailureUpstream)

	c := newFailureTestContext(t)
	RecordTraceFailure(c, FailureNone)
	require.Equal(t, FailureNone, TraceFailure(c))

	gin.SetMode(gin.TestMode)
	requestless, _ := gin.CreateTestContext(httptest.NewRecorder())
	RecordTraceFailure(requestless, FailureUpstream)
	require.Equal(t, FailureNone, TraceFailure(requestless))
}

// TestTimeToFirstTokenMs verifies time-to-first-token is derived from the
// recorded marks and separated from the total lifetime.
func TestTimeToFirstTokenMs(t *testing.T) {
	received := int64(1_000_000)
	first := received + 120
	completed := received + 600_000

	t.Run("streaming request", func(t *testing.T) {
		in := model.TraceRowInput{
			CreatedAt: received,
			Timestamps: &model.TraceTimestamps{
				RequestReceived:     &received,
				FirstClientResponse: &first,
				RequestCompleted:    &completed,
			},
		}
		ttft, known := timeToFirstTokenMs(in)
		require.True(t, known)
		require.Equal(t, int64(120), ttft,
			"a ten-minute stream that answered in 120ms has a 120ms TTFT")
	})

	t.Run("no first client response", func(t *testing.T) {
		in := model.TraceRowInput{
			CreatedAt:  received,
			Timestamps: &model.TraceTimestamps{RequestReceived: &received},
		}
		ttft, known := timeToFirstTokenMs(in)
		require.False(t, known)
		require.Zero(t, ttft,
			"zero makes the slow rule fall back to the total lifetime")
	})

	t.Run("nil timestamps", func(t *testing.T) {
		ttft, known := timeToFirstTokenMs(model.TraceRowInput{CreatedAt: received})
		require.False(t, known)
		require.Zero(t, ttft)
	})

	t.Run("clock skew never yields a negative TTFT", func(t *testing.T) {
		earlier := received - 50
		in := model.TraceRowInput{
			CreatedAt: received,
			Timestamps: &model.TraceTimestamps{
				RequestReceived:     &received,
				FirstClientResponse: &earlier,
			},
		}
		ttft, known := timeToFirstTokenMs(in)
		require.False(t, known)
		require.Zero(t, ttft)
	})

	t.Run("immediate first response is known", func(t *testing.T) {
		in := model.TraceRowInput{
			CreatedAt: received,
			Timestamps: &model.TraceTimestamps{
				RequestReceived:     &received,
				FirstClientResponse: &received,
			},
		}
		ttft, known := timeToFirstTokenMs(in)
		require.True(t, known)
		require.Zero(t, ttft)
	})
}
