package openrouter

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

const (
	// maxReportedCostLine bounds one buffered SSE line; a longer line is not
	// inspected for a cost (usage chunks are small).
	maxReportedCostLine = 1 << 20
	// maxReportedCostBody bounds a buffered non-stream body.
	maxReportedCostBody = 16 << 20
)

// BillsAtLeastReportedCost implements adaptor.ReportedCostBillingPolicy.
// OpenRouter lists each model at its top provider's price but load-balances
// requests across providers whose prices can be 10-200x higher, and it reports
// the real charge in usage.cost on every response
// (https://openrouter.ai/docs/cookbook/administration/usage-accounting.md).
// Settlement therefore never bills less than that cost. It takes no
// parameters and returns true.
func (a *Adaptor) BillsAtLeastReportedCost() bool { return true }

// reportedCostUsage is the cost part of an OpenRouter usage object. Cost is
// USD credits charged to the account; for BYOK requests it is only the BYOK
// fee and the provider bills upstream_inference_cost directly.
type reportedCostUsage struct {
	Cost        *float64 `json:"cost"`
	IsBYOK      bool     `json:"is_byok"`
	CostDetails *struct {
		UpstreamInferenceCost *float64 `json:"upstream_inference_cost"`
	} `json:"cost_details"`
}

// reportedCostEnvelope finds usage at a Chat Completions root or inside a
// Responses envelope (response.completed / response.incomplete).
type reportedCostEnvelope struct {
	Usage    *reportedCostUsage `json:"usage"`
	Response *struct {
		Usage *reportedCostUsage `json:"usage"`
	} `json:"response"`
}

// ReportedCostReader observes an OpenRouter provider body for OpenRouter's reported
// cost without altering the bytes the handler reads. A heartbeat reader may
// abandon a blocked Read on client disconnect, so mu serializes observation
// with finish and bytes after finish are ignored.
type ReportedCostReader struct {
	io.ReadCloser
	mu       sync.Mutex
	stream   bool
	line     []byte
	skipping bool
	body     []byte
	overflow bool
	costUSD  float64
	found    bool
	finished bool
}

// ObserveReportedCost wraps resp so the reported cost can be read after the
// handler consumed the body. Parameter resp is the provider response. It
// returns the observer, or nil when there is no body.
func ObserveReportedCost(resp *http.Response) *ReportedCostReader {
	if resp == nil || resp.Body == nil {
		return nil
	}
	observer := &ReportedCostReader{ReadCloser: resp.Body,
		stream: strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")}
	resp.Body = observer
	return observer
}

// Unwrap exposes the provider body for transport-cancellation provenance.
// It takes no parameters and returns the wrapped body.
func (r *ReportedCostReader) Unwrap() io.ReadCloser { return r.ReadCloser }

// Read forwards provider bytes unchanged and observes them. Parameters and
// returns follow io.Reader.
func (r *ReportedCostReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		r.mu.Lock()
		if !r.finished {
			r.observe(p[:n])
		}
		r.mu.Unlock()
	}
	return n, err
}

// observe buffers a chunk: SSE data lines are parsed as they complete; a JSON
// body is accumulated (bounded) and parsed at finish. Must hold mu.
func (r *ReportedCostReader) observe(chunk []byte) {
	if !r.stream {
		if r.overflow {
			return
		}
		if len(r.body)+len(chunk) > maxReportedCostBody {
			r.body, r.overflow = nil, true
			return
		}
		r.body = append(r.body, chunk...)
		return
	}
	for len(chunk) > 0 {
		newline := bytes.IndexByte(chunk, '\n')
		if newline < 0 {
			if !r.skipping {
				r.line = append(r.line, chunk...)
				if len(r.line) > maxReportedCostLine {
					r.line, r.skipping = nil, true
				}
			}
			return
		}
		if !r.skipping {
			r.parseLine(append(r.line, chunk[:newline]...))
		}
		r.line, r.skipping = r.line[:0], false
		chunk = chunk[newline+1:]
	}
}

// parseLine records the cost carried by one SSE data line. Must hold mu.
func (r *ReportedCostReader) parseLine(line []byte) {
	payload, ok := bytes.CutPrefix(bytes.TrimSpace(line), []byte("data:"))
	if !ok {
		return
	}
	r.parseDocument(bytes.TrimSpace(payload))
}

// parseDocument records the cost of one JSON document, keeping the last one
// observed (the terminal usage chunk). Must hold mu.
func (r *ReportedCostReader) parseDocument(doc []byte) {
	if !bytes.Contains(doc, []byte(`"cost"`)) {
		return
	}
	var envelope reportedCostEnvelope
	if err := json.Unmarshal(doc, &envelope); err != nil {
		return
	}
	usage := envelope.Usage
	if envelope.Response != nil && envelope.Response.Usage != nil {
		usage = envelope.Response.Usage
	}
	if usage == nil || usage.Cost == nil {
		return
	}
	cost := *usage.Cost
	if usage.IsBYOK && usage.CostDetails != nil && usage.CostDetails.UpstreamInferenceCost != nil {
		cost += *usage.CostDetails.UpstreamInferenceCost
	}
	if math.IsNaN(cost) || math.IsInf(cost, 0) || cost < 0 {
		return
	}
	r.costUSD, r.found = cost, true
}

// Finish freezes the observation exactly once. It takes no parameters and
// returns the reported cost and whether one was observed.
func (r *ReportedCostReader) Finish() (float64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.finished {
		if !r.stream && !r.overflow {
			r.parseDocument(bytes.TrimSpace(r.body))
		}
		if r.stream && !r.skipping && len(r.line) > 0 {
			r.parseLine(r.line)
		}
		r.finished, r.body, r.line = true, nil, nil
	}
	return r.costUSD, r.found
}

// AttachReportedCost puts the observed cost on the usage settlement will
// price: the handler's usage, or the converted Claude usage stored in the
// context for a non-stream Claude Messages conversion. Parameters: c carries
// the request, usage is the handler result and costUSD the reported cost.
// It returns nothing.
func AttachReportedCost(c *gin.Context, usage *model.Usage, costUSD float64) {
	target := usage
	if target == nil && c != nil {
		if stored, exists := c.Get(ctxkey.ConvertedResponseUsage); exists {
			target, _ = stored.(*model.Usage)
		}
	}
	if target == nil {
		gmw.GetLogger(c).Debug("openrouter reported a cost but the response produced no usage to settle",
			zap.Float64("reported_cost_usd", costUSD))
		return
	}
	target.UpstreamCostUSD = costUSD
	gmw.GetLogger(c).Debug("openrouter reported request cost", zap.Float64("reported_cost_usd", costUSD))
}

// reportedCostUsageKeys are OpenRouter usage fields that describe the
// operator's own provider charge and BYOK status, not the client's bill.
var reportedCostUsageKeys = []string{"cost", "is_byok", "cost_details"}

// ScrubReportedCostLine removes OpenRouter's operator-side cost fields from
// the usage object of one forwarded SSE data line, so a streamed Chat
// Completions client never sees the gateway's provider cost (non-stream and
// converted responses are re-rendered without them). Parameter data is a
// normalized "data: {...}" line. It returns the line, unchanged unless it
// carried such fields.
func ScrubReportedCostLine(data string) string {
	if !strings.Contains(data, `"cost`) && !strings.Contains(data, `"is_byok"`) {
		return data
	}
	payload, ok := strings.CutPrefix(data, "data:")
	if !ok {
		return data
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(payload)), &root); err != nil || root == nil {
		return data
	}
	var usage map[string]json.RawMessage
	if err := json.Unmarshal(root["usage"], &usage); err != nil || usage == nil {
		return data
	}
	changed := false
	for _, key := range reportedCostUsageKeys {
		if _, present := usage[key]; present {
			delete(usage, key)
			changed = true
		}
	}
	if !changed {
		return data
	}
	encodedUsage, err := json.Marshal(usage)
	if err != nil {
		return data
	}
	root["usage"] = encodedUsage
	encoded, err := json.Marshal(root)
	if err != nil {
		return data
	}
	return "data: " + string(encoded)
}
