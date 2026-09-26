package observation

import (
	"context"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestNumericSelectionNeverLogsHeaders checks hard identity bounds and excludes arbitrary header values.
func TestNumericSelectionNeverLogsHeaders(t *testing.T) {
	for _, id := range []string{"", "secret", "0\npassword", "-32", "8192", "1", "+32", "032", " 32", strings.Repeat("9", 100)} {
		emitted := false
		r := recorder{active: func() bool { return true }, clock: func() (int64, error) { return 123, nil }, emit: func(context.Context, string, string) { emitted = true }}
		span := r.begin(context.Background(), id, 0)
		span.BeforeFlush()
		span.AfterFlush()
		require.False(t, emitted, id)
		require.False(t, span.Tracked(), id)
	}
	for _, index := range []int{0, 32, 64, 8160} {
		require.True(t, Selected(index))
	}
	for _, index := range []int{-32, 1, 8192} {
		require.False(t, Selected(index))
	}
}

// TestFrameIdentitySurvivesTraceStart preserves the ordinal of frames emitted before the capture begins.
func TestFrameIdentitySurvivesTraceStart(t *testing.T) {
	enabled := false
	at := int64(100)
	var markers []string
	r := recorder{active: func() bool { return enabled }, clock: func() (int64, error) { at++; return at, nil }, emit: func(_ context.Context, category, message string) { markers = append(markers, category+":"+message) }}
	frame := 0
	for range 2 {
		s := r.begin(context.Background(), "32", frame)
		if s.Tracked() {
			frame++
		}
		s.BeforeFlush()
		s.AfterFlush()
	}
	require.Empty(t, markers)
	require.Equal(t, 2, frame)
	enabled = true
	s := r.begin(context.Background(), "32", frame)
	s.BeforeFlush()
	s.AfterFlush()
	require.Equal(t, []string{"oneapi.sse:32/2/begin/101", "oneapi.sse:32/2/flush_start/102", "oneapi.sse:32/2/flush_end/103"}, markers)
	enabled = false
	s.AfterFlush()
	require.Len(t, markers, 3)
}

// TestProbeErrorsAreExplicit emits bounded diagnostic errors without recording the original error text.
func TestProbeErrorsAreExplicit(t *testing.T) {
	var messages []string
	r := recorder{active: func() bool { return true }, clock: func() (int64, error) { return 0, errors.New("sensitive error") }, emit: func(_ context.Context, c, m string) { messages = append(messages, c+":"+m) }}
	r.begin(context.Background(), "0", 0)
	r.begin(context.Background(), "0", MaxFrames)
	require.Equal(t, []string{"oneapi.sse.error:clock_failure", "oneapi.sse.error:frame_limit"}, messages)
	require.False(t, r.begin(nil, "0", 0).Tracked())
}

// TestClientClockAndBounds prevents silent truncation and regressions in the received-event timeline.
func TestClientClockAndBounds(t *testing.T) {
	original := []int64{10}
	times, err := appendClient(original, func() (int64, error) { return 11, nil })
	require.NoError(t, err)
	require.Equal(t, []int64{10, 11}, times)
	for _, at := range []int64{0, -1, 9} {
		_, err := appendClient(original, func() (int64, error) { return at, nil })
		require.Error(t, err)
	}
	_, err = appendClient(original, func() (int64, error) { return 0, errors.New("clock") })
	require.Error(t, err)
	called := false
	_, err = appendClient(make([]int64, MaxFrames), func() (int64, error) { called = true; return 12, nil })
	require.Error(t, err)
	require.False(t, called)
}

// TestInertSpanHasNoSideEffects requires nonselected requests and nil contexts to remain unobserved.
func TestInertSpanHasNoSideEffects(t *testing.T) {
	var s Span
	require.False(t, s.Tracked())
	s.BeforeFlush()
	s.AfterFlush()
	require.False(t, Begin(nil, "0", 0).Tracked())
}
