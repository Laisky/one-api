package sse

import (
	"math"

	"github.com/Laisky/errors/v2"
)

// frameTerminator ends every encoded SSE frame with the blank line that
// dispatches the event.
const frameTerminator = "\n\n"

// ErrFrameTooLarge reports an SSE frame whose byte size cannot be represented
// as an int, so it cannot be allocated.
var ErrFrameTooLarge = errors.New("sse frame is too large")

// FrameSize returns the exact byte length of the frame EncodeFrame renders for
// an event name of eventTypeLen bytes and a payload of payloadLen bytes. A zero
// eventTypeLen means a data-only frame without an event line. It returns false
// when either length is negative or the size overflows int, so callers can
// size an allocation from untrusted lengths without an unchecked addition.
func FrameSize(eventTypeLen, payloadLen int) (int, bool) {
	const dataOnlyOverhead = len(dataLinePrefix) + len(frameTerminator)
	if payloadLen < 0 || eventTypeLen < 0 || payloadLen > math.MaxInt-dataOnlyOverhead {
		return 0, false
	}

	frameSize := payloadLen + dataOnlyOverhead
	if eventTypeLen == 0 {
		return frameSize, true
	}

	const namedEventOverhead = len(eventLinePrefix) + 1
	if frameSize > math.MaxInt-namedEventOverhead {
		return 0, false
	}
	frameSize += namedEventOverhead
	if eventTypeLen > math.MaxInt-frameSize {
		return 0, false
	}
	return frameSize + eventTypeLen, true
}

// EncodeFrame renders one SSE frame in a single allocation of exactly
// FrameSize bytes: "event: <eventType>\n" when eventType is non-empty, then
// "data: <payload>\n\n". Parameters: eventType is a single-line event name and
// payload a single-line data value such as compact JSON; neither is escaped,
// so callers must not pass embedded newlines. It returns the frame, or
// ErrFrameTooLarge when its size overflows int.
func EncodeFrame(eventType string, payload []byte) ([]byte, error) {
	size, ok := FrameSize(len(eventType), len(payload))
	if !ok {
		return nil, errors.WithStack(ErrFrameTooLarge)
	}

	frame := make([]byte, 0, size)
	if eventType != "" {
		frame = append(frame, eventLinePrefix...)
		frame = append(frame, eventType...)
		frame = append(frame, '\n')
	}
	frame = append(frame, dataLinePrefix...)
	frame = append(frame, payload...)
	frame = append(frame, frameTerminator...)
	return frame, nil
}
