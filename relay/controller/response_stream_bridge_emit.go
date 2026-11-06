package controller

import (
	"encoding/json"
	"errors"
	"io"
	"math"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/relay/adaptor/openai"
)

const (
	responseStreamEventPrefix = "event: "
	responseStreamDataPrefix  = "data: "
	responseStreamFrameSuffix = "\n\n"
)

// errResponseStreamEventTooLarge reports a frame whose size cannot be represented.
// It is a plain stdlib error, matching the former fmt.Errorf value without a stack.
var errResponseStreamEventTooLarge = errors.New("response stream event is too large")

// responseStreamFrameCapacity returns the byte size of one SSE frame for a
// payload and event name of the given lengths, or false when it overflows int.
func responseStreamFrameCapacity(payloadLen, eventTypeLen int) (int, bool) {
	const dataOnlyOverhead = len(responseStreamDataPrefix) + len(responseStreamFrameSuffix)
	if payloadLen < 0 || eventTypeLen < 0 || payloadLen > math.MaxInt-dataOnlyOverhead {
		return 0, false
	}

	frameSize := payloadLen + dataOnlyOverhead
	if eventTypeLen == 0 {
		return frameSize, true
	}

	const namedEventOverhead = len(responseStreamEventPrefix) + 1
	if frameSize > math.MaxInt-namedEventOverhead {
		return 0, false
	}
	frameSize += namedEventOverhead
	if eventTypeLen > math.MaxInt-frameSize {
		return 0, false
	}
	return frameSize + eventTypeLen, true
}

// emitEvent writes one Responses SSE frame: it stamps the event type, the
// response id and the next sequence_number, marshals the event through the
// spec-conformant DTO encoder and flushes it. Parameters: c is the request
// context, eventType the SSE event name and event the payload. It returns
// false, and keeps the error for StreamWriteError, once a marshal or write
// fails; the sequence number advances only for written events.
func (h *chatToResponseStreamBridge) emitEvent(c *gin.Context, eventType string, event openai.ResponseAPIStreamEvent) bool {
	if h.writeErr != nil {
		return false
	}
	event.Type = eventType
	if event.Id == "" {
		event.Id = h.responseID
	}
	// Every Responses stream event carries its position; clients order and
	// de-duplicate by it, so it starts at 0 and advances once per emitted event.
	event.SequenceNumber = h.sequenceNumber

	payload, err := json.Marshal(event)
	if err != nil {
		gmw.GetLogger(c).Warn("failed to marshal response stream event", zap.String("event_type", eventType), zap.Error(err))
		h.writeErr = err
		return false
	}

	frameSize, ok := responseStreamFrameCapacity(len(payload), len(eventType))
	if !ok {
		gmw.GetLogger(c).Warn("response stream event is too large", zap.String("event_type", eventType))
		h.writeErr = errResponseStreamEventTooLarge
		return false
	}

	frame := make([]byte, 0, frameSize)
	if eventType != "" {
		frame = append(frame, responseStreamEventPrefix...)
		frame = append(frame, eventType...)
		frame = append(frame, '\n')
	}
	frame = append(frame, responseStreamDataPrefix...)
	frame = append(frame, payload...)
	frame = append(frame, responseStreamFrameSuffix...)

	n, err := c.Writer.Write(frame)
	if err == nil && n != len(frame) {
		err = io.ErrShortWrite
	}
	if err != nil {
		gmw.GetLogger(c).Warn("failed to write response stream event", zap.String("event_type", eventType), zap.Error(err))
		h.writeErr = err
		return false
	}
	h.sequenceNumber++
	c.Writer.Flush()
	return true
}

// StreamWriteError reports the first error encountered while writing an event.
func (h *chatToResponseStreamBridge) StreamWriteError() error { return h.writeErr }

// DeliveredImagesInLastChunk reports image events fully accepted by the writer.
func (h *chatToResponseStreamBridge) DeliveredImagesInLastChunk() int { return h.deliveredImages }

// emitDone preserves the existing terminal frame while recording write failures.
func (h *chatToResponseStreamBridge) emitDone(c *gin.Context) {
	if h.writeErr != nil {
		return
	}
	if err := (common.CustomEvent{Data: "data: [DONE]"}).Render(c.Writer); err != nil {
		h.writeErr = err
		return
	}
	c.Writer.Flush()
}

// nextOutputIndex reserves and returns the next dense output_index.
func (h *chatToResponseStreamBridge) nextOutputIndex() int {
	idx := h.outputIndexCounter
	h.outputIndexCounter++
	return idx
}
