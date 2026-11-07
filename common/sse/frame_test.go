package sse

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFrameSize pins the exact frame size and every overflow boundary of the
// shared SSE frame size computation.
func TestFrameSize(t *testing.T) {
	t.Parallel()
	const (
		dataOnlyOverhead   = len(dataLinePrefix) + len(frameTerminator)
		namedEventOverhead = len(eventLinePrefix) + 1
	)

	tests := []struct {
		name         string
		payloadLen   int
		eventTypeLen int
		want         int
		wantOK       bool
	}{
		{name: "data-only frame", payloadLen: 128, want: 128 + dataOnlyOverhead, wantOK: true},
		{
			name:         "named event frame",
			payloadLen:   128,
			eventTypeLen: len("response.output_text.delta"),
			want:         128 + dataOnlyOverhead + namedEventOverhead + len("response.output_text.delta"),
			wantOK:       true,
		},
		{name: "empty data-only frame", want: dataOnlyOverhead, wantOK: true},
		{name: "negative payload length", payloadLen: -1},
		{name: "negative event type length", eventTypeLen: -1},
		{name: "data-only overflow", payloadLen: math.MaxInt - dataOnlyOverhead + 1},
		{name: "largest data-only frame", payloadLen: math.MaxInt - dataOnlyOverhead, want: math.MaxInt, wantOK: true},
		{name: "named event overhead overflow", payloadLen: math.MaxInt - dataOnlyOverhead, eventTypeLen: 1},
		{name: "event type length overflow", eventTypeLen: math.MaxInt},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := FrameSize(tt.eventTypeLen, tt.payloadLen)
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestEncodeFrame proves EncodeFrame renders the SSE wire format byte for
// byte, in exactly FrameSize bytes, and that its output round-trips through
// the package's own line reader.
func TestEncodeFrame(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		eventType string
		payload   string
		want      string
	}{
		"named":     {eventType: "message_start", payload: `{"type":"message_start"}`, want: "event: message_start\ndata: {\"type\":\"message_start\"}\n\n"},
		"data-only": {payload: `{"a":1}`, want: "data: {\"a\":1}\n\n"},
		"empty":     {eventType: "ping", want: "event: ping\ndata: \n\n"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			frame, err := EncodeFrame(tc.eventType, []byte(tc.payload))
			require.NoError(t, err)
			require.Equal(t, tc.want, string(frame))
			size, ok := FrameSize(len(tc.eventType), len(tc.payload))
			require.True(t, ok)
			require.Len(t, frame, size)
			require.Equal(t, size, cap(frame), "the frame must be allocated once at its exact size")

			reader := NewLineReader(strings.NewReader(string(frame)), DefaultLineBufferSize)
			var gotEvent, gotData string
			for {
				line, err := reader.Next()
				if err != nil {
					break
				}
				switch line.Kind {
				case LineKindEvent:
					gotEvent = strings.TrimPrefix(line.Text(), eventLinePrefix)
				case LineKindData:
					gotData = strings.TrimPrefix(line.Text(), dataLinePrefix)
				}
			}
			require.Equal(t, tc.eventType, gotEvent)
			require.Equal(t, tc.payload, gotData)
		})
	}
}
