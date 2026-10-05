package openai

import (
	"bytes"
	"encoding/json"
	"io"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
)

// maxWebSearchCounterLine bounds the bytes buffered for one SSE line. Longer
// lines are skipped for counting only; output_item.done events that carry a
// web_search_call are small, so the paid call is still observed.
const maxWebSearchCounterLine = 8 << 20

// webSearchStreamCounter observes a Responses SSE body while another reader
// converts it, counting chargeable web search calls without altering bytes.
type webSearchStreamCounter struct {
	body     io.ReadCloser
	pending  []byte
	skipping bool
	seen     map[string]struct{}
	count    int
}

// newWebSearchStreamCounter wraps body. Parameters: body is the upstream stream.
// Returns: a pass-through reader that counts web search calls.
func newWebSearchStreamCounter(body io.ReadCloser) *webSearchStreamCounter {
	return &webSearchStreamCounter{body: body, seen: make(map[string]struct{})}
}

// Read forwards upstream bytes unchanged and scans each completed line.
func (w *webSearchStreamCounter) Read(p []byte) (int, error) {
	n, err := w.body.Read(p)
	if n > 0 {
		w.scan(p[:n])
	}
	return n, err
}

// Close closes the wrapped upstream body.
func (w *webSearchStreamCounter) Close() error {
	return w.body.Close()
}

// scan splits chunk into lines, carrying a bounded partial line forward.
func (w *webSearchStreamCounter) scan(chunk []byte) {
	for len(chunk) > 0 {
		newline := bytes.IndexByte(chunk, '\n')
		if newline < 0 {
			if !w.skipping {
				w.pending = append(w.pending, chunk...)
				if len(w.pending) > maxWebSearchCounterLine {
					w.pending, w.skipping = nil, true
				}
			}
			return
		}
		if !w.skipping {
			w.observeLine(append(w.pending, chunk[:newline]...))
		}
		w.pending, w.skipping = w.pending[:0], false
		chunk = chunk[newline+1:]
	}
}

// observeLine counts web_search_call items in one Responses SSE data line,
// deduplicating by item ID across output_item events and the terminal response.
func (w *webSearchStreamCounter) observeLine(line []byte) {
	payload, ok := bytes.CutPrefix(bytes.TrimSpace(line), []byte("data:"))
	if !ok || !bytes.Contains(payload, []byte("web_search_call")) {
		return
	}
	var event struct {
		Item     *OutputItem `json:"item"`
		Response *struct {
			Output []OutputItem `json:"output"`
		} `json:"response"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(payload), &event); err != nil {
		return
	}
	var items []OutputItem
	if event.Item != nil {
		items = append(items, *event.Item)
	}
	if event.Response != nil {
		items = append(items, event.Response.Output...)
	}
	w.count += countNewWebSearchSearchActions(items, w.seen)
}

// record stores the observed count for the shared tooling billing when the
// converted stream carried no earlier counter. Parameters: c is the request context.
func (w *webSearchStreamCounter) record(c *gin.Context) {
	if c == nil || w.count <= 0 {
		return
	}
	c.Set(ctxkey.WebSearchCallCount, w.count)
	lg := gmw.GetLogger(c)
	lg.Debug("recorded converted Claude stream web search calls", zap.Int("web_search_calls", w.count))
}
