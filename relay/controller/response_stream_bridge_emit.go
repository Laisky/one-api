package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"math"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
)

const (
	responseStreamEventPrefix = "event: "
	responseStreamDataPrefix  = "data: "
	responseStreamFrameSuffix = "\n\n"
)

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

func (h *chatToResponseStreamBridge) emitEvent(c *gin.Context, eventType string, event openai.ResponseAPIStreamEvent) bool {
	if h.writeErr != nil {
		return false
	}
	event.Type = eventType
	if event.Id == "" {
		event.Id = h.responseID
	}

	payload, err := json.Marshal(event)
	if err != nil {
		gmw.GetLogger(c).Warn("failed to marshal response stream event", zap.String("event_type", eventType), zap.Error(err))
		h.writeErr = err
		return false
	}

	frameSize, ok := responseStreamFrameCapacity(len(payload), len(eventType))
	if !ok {
		gmw.GetLogger(c).Warn("response stream event is too large", zap.String("event_type", eventType))
		h.writeErr = fmt.Errorf("response stream event is too large")
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

func (h *chatToResponseStreamBridge) nextOutputIndex() int {
	idx := h.outputIndexCounter
	h.outputIndexCounter++
	return idx
}
