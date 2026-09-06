package tracing

// In-memory trace accumulation (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1.1).
//
// The pre-proposal path turned every lifecycle mark into a SELECT plus a
// read-modify-write UPDATE of a JSON TEXT column, on the request goroutine.
// A Recorder holds the same document in memory for the life of the request and
// is handed to a sink exactly once, when the request ends.

import (
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// Recorder accumulates one request's trace document in memory.
//
// All methods are safe for concurrent use: a streaming relay marks
// FirstClientResponse from the writer goroutine while the adaptor marks
// UpstreamCompleted from another.
type Recorder struct {
	mu sync.Mutex

	traceID   string
	url       string
	method    string
	bodySize  int64
	createdAt int64
	status    int
	forced    bool
	finished  bool

	timestamps model.TraceTimestamps
}

// NewRecorder creates a Recorder for a request that has just been received.
//
// Parameters:
//   - traceID: the per-request trace identifier; must not be empty.
//   - url: the raw request URL, sanitized later when the row is built.
//   - method: the HTTP method.
//   - bodySize: the request body size in bytes.
//
// Return values:
//   - *Recorder: a recorder whose RequestReceived mark is already set.
func NewRecorder(traceID, url, method string, bodySize int64) *Recorder {
	now := time.Now().UnixMilli()
	r := &Recorder{
		traceID:   traceID,
		url:       url,
		method:    method,
		bodySize:  bodySize,
		createdAt: now,
	}
	r.timestamps.RequestReceived = &now
	return r
}

// TraceID returns the identifier the recorder was created with.
//
// Parameters: none.
//
// Return values:
//   - string: the trace identifier.
func (r *Recorder) TraceID() string {
	if r == nil {
		return ""
	}
	return r.traceID
}

// Mark records a lifecycle timestamp.
//
// Marks are idempotent per key in the sense that the last write wins, matching
// the pre-proposal behavior where each key had a single slot in the document.
//
// Parameters:
//   - key: one of the model.Timestamp* constants; unknown keys are ignored.
//
// Return values:
//   - bool: whether the key was recognized, so the caller can warn once.
func (r *Recorder) Mark(key string) bool {
	if r == nil {
		return false
	}
	now := time.Now().UnixMilli()

	r.mu.Lock()
	defer r.mu.Unlock()

	switch key {
	case model.TimestampRequestReceived:
		r.timestamps.RequestReceived = &now
	case model.TimestampRequestForwarded:
		r.timestamps.RequestForwarded = &now
	case model.TimestampFirstUpstreamResponse:
		r.timestamps.FirstUpstreamResponse = &now
	case model.TimestampFirstClientResponse:
		r.timestamps.FirstClientResponse = &now
	case model.TimestampUpstreamCompleted:
		r.timestamps.UpstreamCompleted = &now
	case model.TimestampRequestCompleted:
		r.timestamps.RequestCompleted = &now
	default:
		return false
	}
	return true
}

// AppendExternalCall records one external call performed during the request.
//
// Parameters:
//   - call: the call entry; missing timing fields are defaulted the same way
//     the synchronous path defaults them.
//
// Return values: none.
func (r *Recorder) AppendExternalCall(call model.TraceExternalCall) {
	if r == nil {
		return
	}
	call = normalizeExternalCall(call)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.timestamps.ExternalCalls = append(r.timestamps.ExternalCalls, call)
}

// SetStatus records the final HTTP status code.
//
// Parameters:
//   - status: the HTTP status code; non-positive values are ignored.
//
// Return values: none.
func (r *Recorder) SetStatus(status int) {
	if r == nil || status <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
}

// ForceSample pins this trace to be persisted regardless of the sample rate.
//
// Parameters: none.
//
// Return values: none.
func (r *Recorder) ForceSample() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.forced = true
}

// Status returns the recorded HTTP status code.
//
// Parameters: none.
//
// Return values:
//   - int: the status code, or 0 when none was recorded.
func (r *Recorder) Status() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

// DurationMs returns how long the request has been running.
//
// Parameters: none.
//
// Return values:
//   - int64: elapsed milliseconds since the request was received.
func (r *Recorder) DurationMs() int64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.durationMsLocked()
}

// durationMsLocked computes the elapsed time; the caller must hold r.mu.
//
// Parameters: none.
//
// Return values:
//   - int64: elapsed milliseconds, preferring the recorded completion mark.
func (r *Recorder) durationMsLocked() int64 {
	end := time.Now().UnixMilli()
	if r.timestamps.RequestCompleted != nil {
		end = *r.timestamps.RequestCompleted
	}
	if end < r.createdAt {
		return 0
	}
	return end - r.createdAt
}

// Finish closes the recorder and returns the finalized row input exactly once.
//
// The single-shot contract matters: TracingMiddleware always calls it, and a
// panicking handler must not be able to produce two rows for one request.
//
// Parameters: none.
//
// Return values:
//   - model.TraceRowInput: the finalized trace state.
//   - int64: total request duration in milliseconds.
//   - bool: whether this call was the one that closed the recorder; false means
//     the trace was already finished and must not be submitted again.
func (r *Recorder) Finish() (model.TraceRowInput, int64, bool) {
	if r == nil {
		return model.TraceRowInput{}, 0, false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.finished {
		return model.TraceRowInput{}, 0, false
	}
	r.finished = true

	// Copy the document so the sink never shares memory with a late mark
	// arriving from a straggling goroutine.
	snapshot := r.timestamps
	if len(r.timestamps.ExternalCalls) > 0 {
		snapshot.ExternalCalls = make([]model.TraceExternalCall, len(r.timestamps.ExternalCalls))
		copy(snapshot.ExternalCalls, r.timestamps.ExternalCalls)
	}

	return model.TraceRowInput{
		TraceId:    r.traceID,
		URL:        r.url,
		Method:     r.method,
		BodySize:   r.bodySize,
		Status:     r.status,
		CreatedAt:  r.createdAt,
		Timestamps: &snapshot,
	}, r.durationMsLocked(), true
}

// Forced reports whether this trace was pinned for retention.
//
// Parameters: none.
//
// Return values:
//   - bool: true when ForceSample was called.
func (r *Recorder) Forced() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.forced
}

// normalizeExternalCall fills in the defaults the synchronous path applied.
//
// Parameters:
//   - call: the raw entry.
//
// Return values:
//   - model.TraceExternalCall: the entry with source, timing, and key defaulted.
func normalizeExternalCall(call model.TraceExternalCall) model.TraceExternalCall {
	if call.Source == "" {
		call.Source = "external"
	}
	if call.StartedAt == 0 {
		call.StartedAt = time.Now().UnixMilli()
	}
	if call.EndedAt == 0 {
		call.EndedAt = call.StartedAt
	}
	if call.DurationMs == 0 && call.EndedAt >= call.StartedAt {
		call.DurationMs = call.EndedAt - call.StartedAt
	}
	if call.Key == "" {
		call.Key = call.Source + ":" + strconv.FormatInt(call.StartedAt, 10)
	}
	return call
}

// recorderFromGin returns the Recorder bound to a request, if any.
//
// Parameters:
//   - c: the gin context; nil yields nil.
//
// Return values:
//   - *Recorder: the bound recorder, or nil when the request is not traced.
func recorderFromGin(c *gin.Context) *Recorder {
	if c == nil {
		return nil
	}
	v, ok := c.Get(ctxkey.TraceRecorder)
	if !ok {
		return nil
	}
	r, _ := v.(*Recorder)
	return r
}

// bindRecorder attaches a Recorder to the request context.
//
// Parameters:
//   - c: the gin context; nil is a no-op.
//   - r: the recorder to bind.
//
// Return values: none.
func bindRecorder(c *gin.Context, r *Recorder) {
	if c == nil || r == nil {
		return
	}
	c.Set(ctxkey.TraceRecorder, r)
}
