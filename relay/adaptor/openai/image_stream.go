package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/render"
	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/model"
)

// imageStreamEventLimit bounds the bytes of one Images API stream event the
// gateway keeps in memory to read its type and usage. A final image is one
// base64 event, so the bound is generous; a larger event is still forwarded in
// full, but its receipt is treated as unreadable.
var imageStreamEventLimit = 64 << 20

// imageStreamEvent is the part of an Images API stream event billing reads:
// the event type, the usage of a final-image event, and any error object.
type imageStreamEvent struct {
	Type  string          `json:"type"`
	Usage *ImageUsage     `json:"usage"`
	Error json.RawMessage `json:"error"`
}

// isImageEventStream reports whether an image response is a server-sent event
// stream, which GPT Image models send when the request sets stream=true.
// Parameters: resp is the upstream response. Returns: true for a
// text/event-stream Content-Type.
func isImageEventStream(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return err == nil && strings.EqualFold(mediaType, "text/event-stream")
}

// imageStreamRelay forwards one Images API event stream line by line and
// accumulates its billing receipt. data holds the current event's data lines,
// at most imageStreamEventLimit bytes, and overflow records that they did not
// fit.
type imageStreamRelay struct {
	writer   io.Writer
	flusher  http.Flusher
	data     bytes.Buffer
	overflow bool
	pending  bool
	usage    *model.Usage
	receipt  adaptor.ImageStreamReceipt
}

// imageStreamHandler relays a streamed GPT Image generation or edit to the
// client as it arrives and returns the summed usage of its final-image events.
// Every line is forwarded unchanged (line endings normalized to LF) with
// heartbeat comments while the provider is silent; the events are read only
// for billing. The observed ImageStreamReceipt is stored under
// adaptor.ImageStreamReceiptKey so settlement can tell a complete stream from
// an interrupted or failed one. Parameters: c is the client request and resp
// the 2xx event-stream response. Returns: a delivery or transport error, and
// the summed usage (nil when no final-image event carried usage), which is
// returned with an error too so forwarded work is still billed.
func imageStreamHandler(c *gin.Context, resp *http.Response) (apiErr *model.ErrorWithStatusCode, usage *model.Usage) {
	lg := gmw.GetLogger(c)
	copyImageErrorHeaders(c.Writer.Header(), resp.Header)
	lifecycle := beginResponseStream(c, resp)
	relay := &imageStreamRelay{writer: lifecycle.writer}
	if flusher, ok := any(lifecycle.writer.ResponseWriter).(http.Flusher); ok {
		relay.flusher = flusher
	}
	defer func() {
		c.Set(adaptor.ImageStreamReceiptKey, relay.receipt)
		lifecycle.finish(c, &apiErr)
		usage = relay.usage
		lg.Debug("relayed image event stream",
			zap.Int("completed_events", relay.receipt.Completed),
			zap.Int("usage_receipts", relay.receipt.Receipts),
			zap.Int("partial_images", relay.receipt.Partials),
			zap.Bool("failed", relay.receipt.Failed),
			zap.Bool("interrupted", relay.receipt.Interrupted))
	}()

	common.SetEventStreamHeaders(c)
	lines := render.NewHeartbeatLineReader(c, commonsse.NewLineReader(resp.Body, commonsse.DefaultLineBufferSize), render.DefaultHeartbeatInterval)
	defer lines.Close()
	for {
		line, err := lines.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			relay.receipt.Interrupted = true
			if lifecycle.writer.err == nil {
				apiErr = ErrorWrapper(err, "read_stream_failed", http.StatusInternalServerError)
			}
			break
		}
		if err := relay.forward(lg, line); err != nil {
			relay.receipt.Interrupted = true
			apiErr = ErrorWrapper(err, "read_stream_failed", http.StatusInternalServerError)
			break
		}
	}
	// A final event without its trailing blank line still counts.
	relay.dispatch(lg)
	return apiErr, relay.usage
}

// forward writes one stream line to the client and records data lines for
// the event being read. Delivery errors are kept by the lifecycle writer, and
// reading continues so the receipt is still observed. Parameters: lg logs
// unreadable events and line is the next stream line. Returns: an error only
// when the provider body cannot be read.
func (r *imageStreamRelay) forward(lg glog.Logger, line commonsse.Line) error {
	switch {
	case line.Kind == commonsse.LineKindBlank:
		r.write([]byte("\n"))
		r.flush()
		r.dispatch(lg)
		return nil
	case line.Kind == commonsse.LineKindData && line.Oversized:
		r.write([]byte("data: "))
		r.startData()
		if _, err := io.Copy(imageStreamSink{relay: r}, line.Large); err != nil {
			return errors.Wrap(err, "read oversized image stream event")
		}
		r.write([]byte("\n"))
		return nil
	case line.Kind == commonsse.LineKindData:
		r.write(line.Small)
		r.write([]byte("\n"))
		r.startData()
		r.appendData(bytes.TrimLeft(line.Small[len("data:"):], " "))
		return nil
	default:
		r.write(line.Small)
		r.write([]byte("\n"))
		return nil
	}
}

// write sends p to the client. A failed delivery is recorded once by the
// lifecycle writer; later lines are dropped while the receipt is still read.
// Parameters: p is the bytes to write. Returns: none.
func (r *imageStreamRelay) write(p []byte) {
	if w, ok := r.writer.(*responseStreamWriter); ok && w.err != nil {
		return
	}
	_, _ = r.writer.Write(p) // the lifecycle writer records the first failure
}

// flush pushes buffered event bytes to the client when the writer supports it.
// Parameters: none. Returns: none.
func (r *imageStreamRelay) flush() {
	if r.flusher == nil {
		return
	}
	if w, ok := r.writer.(*responseStreamWriter); ok && w.err != nil {
		return
	}
	r.flusher.Flush()
}

// startData begins a data line of the current event: SSE joins the data
// lines of one event with a newline. Parameters: none. Returns: none.
func (r *imageStreamRelay) startData() {
	if r.pending {
		r.appendData([]byte("\n"))
	}
	r.pending = true
}

// appendData keeps p for the current event within imageStreamEventLimit.
// Parameters: p is data-line payload. Returns: none.
func (r *imageStreamRelay) appendData(p []byte) {
	if r.overflow {
		return
	}
	if r.data.Len()+len(p) > imageStreamEventLimit {
		r.overflow = true
		r.data.Reset()
		return
	}
	r.data.Write(p)
}

// dispatch reads the billing facts of the event that just ended and resets
// the event buffer. Final-image events add their usage; an unreadable event,
// or a final-image event without usage, marks the receipt interrupted, since
// its cost is unknown. Parameters: lg logs unreadable events. Returns: none.
func (r *imageStreamRelay) dispatch(lg glog.Logger) {
	if !r.pending {
		return
	}
	defer func() {
		r.data.Reset()
		r.overflow, r.pending = false, false
	}()
	payload := bytes.TrimSpace(r.data.Bytes())
	if r.overflow {
		r.receipt.Interrupted = true
		lg.Warn("image stream event exceeded the readable size; its receipt is unknown",
			zap.Int("limit_bytes", imageStreamEventLimit))
		return
	}
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return
	}
	var event imageStreamEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		r.receipt.Interrupted = true
		lg.Warn("image stream event is not readable JSON; its receipt is unknown",
			zap.Int("event_bytes", len(payload)), zap.Error(err))
		return
	}
	switch {
	case event.Type == "error" || len(event.Error) > 0 && !bytes.Equal(event.Error, []byte("null")):
		r.receipt.Failed = true
	case strings.HasSuffix(event.Type, ".partial_image"):
		r.receipt.Partials++
	case strings.HasSuffix(event.Type, ".completed"):
		r.receipt.Completed++
		if event.Usage == nil {
			r.receipt.Interrupted = true
			return
		}
		if !imageStreamUsageUsable(event.Usage) {
			// The image's cost is unknown, so the stream is not a complete
			// receipt. Its counters still raise the ceiling settlement charges.
			r.receipt.Interrupted = true
		} else {
			r.receipt.Receipts++
		}
		r.usage = addImageStreamUsage(r.usage, event.Usage.Convert2GeneralUsage())
	}
}

// imageStreamUsageUsable reports whether a final-image usage states both an
// output count and an input count (input_tokens, or a total above the output),
// the minimum that prices one image. Parameters: usage is the event usage.
// Returns: true when the usage can count as that image's receipt.
func imageStreamUsageUsable(usage *ImageUsage) bool {
	return usage.OutputTokens > 0 && (usage.InputTokens > 0 || usage.TotalTokens > usage.OutputTokens)
}

// imageStreamSink forwards an oversized data payload to the client while
// keeping it, within the event limit, for the receipt.
type imageStreamSink struct{ relay *imageStreamRelay }

// Write forwards p and records it. Parameters: p is payload bytes. Returns:
// len(p) and nil, because delivery failures are recorded, not returned, so the
// provider body keeps being read for its receipt.
func (s imageStreamSink) Write(p []byte) (int, error) {
	s.relay.write(p)
	s.relay.appendData(p)
	return len(p), nil
}

// addImageStreamUsage sums two image usages field by field, including the
// input modality and cache buckets. A cached-modality split survives only when
// both usages state one; otherwise the sum has none, so billing treats its
// cache reads as unsplit. Parameters: total is the running sum (nil for none)
// and next the usage to add. Returns: the new sum.
func addImageStreamUsage(total, next *model.Usage) *model.Usage {
	if total == nil {
		return next
	}
	total.PromptTokens += next.PromptTokens
	total.CompletionTokens += next.CompletionTokens
	total.TotalTokens += next.TotalTokens
	sum, add := total.PromptTokensDetails, next.PromptTokensDetails
	if sum == nil || add == nil {
		if add != nil {
			total.PromptTokensDetails = add
		}
		if sum != nil && add != nil || sum == nil && add == nil {
			return total
		}
		// One side has no details; its tokens are in the totals only.
		total.PromptTokensDetails.CachedTokensDetails = nil
		return total
	}
	sum.CachedTokens += add.CachedTokens
	sum.TextTokens += add.TextTokens
	sum.ImageTokens += add.ImageTokens
	if sum.CachedTokensDetails == nil || add.CachedTokensDetails == nil {
		sum.CachedTokensDetails = nil
	} else {
		sum.CachedTokensDetails.AudioTokens += add.CachedTokensDetails.AudioTokens
		sum.CachedTokensDetails.TextTokens += add.CachedTokensDetails.TextTokens
		sum.CachedTokensDetails.ImageTokens += add.CachedTokensDetails.ImageTokens
	}
	return total
}
