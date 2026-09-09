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
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
)

const (
	// maxRecorderURLBytes bounds the raw URL a recorder retains for the whole
	// request lifetime.
	//
	// model.SanitizeTraceURL already bounds what is STORED, but that runs only
	// when the request ends: until then a multi-megabyte request line was held
	// in full, multiplied by every in-flight request. The bound is set above
	// the storage limit so redaction -- which may lengthen a value -- still
	// sees the same prefix it would have seen, and the persisted URL is
	// therefore unchanged for every URL a client can realistically send.
	maxRecorderURLBytes = 8192

	// maxRecorderStringBytes bounds each individual string retained on an
	// external-call entry. Tool names and server labels come from upstream
	// payloads, so their length is not this process's to trust.
	maxRecorderStringBytes = 256

	// recorderBaseBytes is the fixed accounting cost of a recorder: the struct,
	// its timestamp document, and the map entry binding it to the request.
	recorderBaseBytes = 512

	// recorderExternalCallBytes is the fixed accounting cost of one external
	// call entry, on top of the strings it carries.
	recorderExternalCallBytes = 128

	// maxRecorderTraceIDBytes and maxRecorderMethodBytes bound the remaining
	// caller-provided strings retained for a request lifetime.
	maxRecorderTraceIDBytes = 64
	maxRecorderMethodBytes  = 16
)

// Recorder accumulates one request's trace document in memory.
//
// All methods are safe for concurrent use: a streaming relay marks
// FirstClientResponse from the writer goroutine while the adaptor marks
// UpstreamCompleted from another.
//
// Its retained size is bounded (W1, "Recorder memory"): exceeding
// TRACE_MAX_RECORD_BYTES or TRACE_MAX_EXTERNAL_CALLS truncates the accumulated
// detail and is counted as metrics.TraceOutcomeTruncated. Truncation never
// drops the trace and never fails the request, and the accounting records only
// counts -- never a URL, credential, header, or payload.
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

	// admitted records that this recorder holds an active-recorder slot, so
	// Finish releases it exactly once even when called twice.
	admitted bool

	// retainedBytes is the running estimate of what this recorder holds, and
	// truncated records that some detail was cut to stay inside the bounds.
	retainedBytes int64
	truncated     bool

	timestamps model.TraceTimestamps
}

// NewRecorder creates a Recorder for a request that has just been received.
//
// Admission is bounded by TRACE_MAX_ACTIVE_RECORDERS. When the bound is already
// reached this returns nil, counts metrics.TraceOutcomeDroppedActiveLimit, and
// the request runs untraced but otherwise unaffected; every Recorder method is
// nil-safe so callers need no extra branch.
//
// Parameters:
//   - traceID: the per-request trace identifier; must not be empty.
//   - url: the raw request URL, sanitized later when the row is built.
//   - method: the HTTP method.
//   - bodySize: the request body size in bytes.
//
// Return values:
//   - *Recorder: a recorder whose RequestReceived mark is already set, or nil
//     when active-recorder admission was denied.
func NewRecorder(traceID, url, method string, bodySize int64) *Recorder {
	if !admitRecorder() {
		return nil
	}

	limit := recorderByteLimit()
	remaining := int(limit - recorderBaseBytes)
	boundedTraceID, traceIDTruncated := clipString(traceID, min(maxRecorderTraceIDBytes, remaining))
	remaining -= len(boundedTraceID)
	boundedMethod, methodTruncated := clipString(method, min(maxRecorderMethodBytes, remaining))
	remaining -= len(boundedMethod)
	boundedURL, urlTruncated := clipString(url, min(maxRecorderURLBytes, remaining))

	now := time.Now().UnixMilli()
	r := &Recorder{
		traceID:   boundedTraceID,
		url:       boundedURL,
		method:    boundedMethod,
		bodySize:  bodySize,
		createdAt: now,
		admitted:  true,
		truncated: traceIDTruncated || methodTruncated || urlTruncated,
	}
	r.retainedBytes = recorderBaseBytes + int64(len(boundedTraceID)+len(boundedURL)+len(boundedMethod))
	r.timestamps.RequestReceived = &now

	if r.truncated {
		metrics.RecordTraceOutcome(metrics.TraceOutcomeTruncated, 1)
	}
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
// The entry is dropped, not the trace, when it would push the record past
// TRACE_MAX_EXTERNAL_CALLS or TRACE_MAX_RECORD_BYTES. A relay retrying across
// many channels, or a tool loop, would otherwise append for the whole request
// lifetime with nothing bounding the result.
//
// Parameters:
//   - call: the call entry; missing timing fields are defaulted the same way
//     the synchronous path defaults them, and over-long strings are clipped.
//
// Return values: none.
func (r *Recorder) AppendExternalCall(call model.TraceExternalCall) {
	if r == nil {
		return
	}
	call, clipped := normalizeExternalCall(call)
	cost := externalCallBytes(call)

	r.mu.Lock()
	overflow := len(r.timestamps.ExternalCalls) >= config.TraceMaxExternalCalls ||
		r.retainedBytes+cost > recorderByteLimit()
	if !overflow {
		r.timestamps.ExternalCalls = append(r.timestamps.ExternalCalls, call)
		r.retainedBytes += cost
	}
	firstTruncation := (overflow || clipped) && r.noteTruncationLocked()
	r.mu.Unlock()

	// Counted once per record, outside the lock: the metric says a record lost
	// detail, and carries no trace id, URL, or payload of any kind.
	if firstTruncation {
		metrics.RecordTraceOutcome(metrics.TraceOutcomeTruncated, 1)
	}
}

// recorderByteLimit returns an enforceable recorder memory ceiling.
//
// Parameters: none.
//
// Return values:
//   - int64: a ceiling no smaller than config.MinTraceRecordBytes, which is
//     large enough for the recorder's mandatory state.
func recorderByteLimit() int64 {
	return max(int64(config.TraceMaxRecordBytes), int64(config.MinTraceRecordBytes))
}

// noteTruncationLocked marks the record as truncated; the caller must hold r.mu.
//
// Parameters: none.
//
// Return values:
//   - bool: true when this call was the first truncation of this record, so the
//     outcome is counted once per record rather than once per lost entry.
func (r *Recorder) noteTruncationLocked() bool {
	if r.truncated {
		return false
	}
	r.truncated = true
	return true
}

// Truncated reports whether any retained detail was cut to stay within bounds.
//
// Parameters: none.
//
// Return values:
//   - bool: true when the record was truncated.
func (r *Recorder) Truncated() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.truncated
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

	if r.finished {
		r.mu.Unlock()
		return model.TraceRowInput{}, 0, false
	}
	r.finished = true

	// The admission slot is released exactly once. The finished guard above
	// makes a second Finish a no-op, so a panicking handler or a doubly
	// registered middleware can neither emit a second row nor free a slot twice.
	release := r.admitted
	r.admitted = false

	// Copy the document so the sink never shares memory with a late mark
	// arriving from a straggling goroutine.
	snapshot := r.timestamps
	if len(r.timestamps.ExternalCalls) > 0 {
		snapshot.ExternalCalls = make([]model.TraceExternalCall, len(r.timestamps.ExternalCalls))
		copy(snapshot.ExternalCalls, r.timestamps.ExternalCalls)
	}

	in := model.TraceRowInput{
		TraceId:    r.traceID,
		URL:        r.url,
		Method:     r.method,
		BodySize:   r.bodySize,
		Status:     r.status,
		CreatedAt:  r.createdAt,
		Timestamps: &snapshot,
	}
	durationMs := r.durationMsLocked()
	r.mu.Unlock()

	// Outside the lock: the installed metrics recorder is third-party code and
	// must never be able to block a streaming goroutine's late Mark.
	if release {
		releaseRecorder()
	}

	return in, durationMs, true
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

// normalizeExternalCall fills in the defaults the synchronous path applied and
// bounds every string the entry retains.
//
// Parameters:
//   - call: the raw entry.
//
// Return values:
//   - model.TraceExternalCall: the entry with source, timing, and key defaulted
//     and its strings clipped to maxRecorderStringBytes.
//   - bool: whether any string had to be clipped, so the caller can count the
//     record as truncated.
func normalizeExternalCall(call model.TraceExternalCall) (model.TraceExternalCall, bool) {
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

	var clipped, one bool
	call.Key, one = clipString(call.Key, maxRecorderStringBytes)
	clipped = clipped || one
	call.Source, one = clipString(call.Source, maxRecorderStringBytes)
	clipped = clipped || one
	call.Tool, one = clipString(call.Tool, maxRecorderStringBytes)
	clipped = clipped || one
	call.ServerLabel, one = clipString(call.ServerLabel, maxRecorderStringBytes)
	clipped = clipped || one

	return call, clipped
}

// externalCallBytes estimates what one external-call entry retains.
//
// Parameters:
//   - call: the normalized entry.
//
// Return values:
//   - int64: the estimated retained size in bytes.
func externalCallBytes(call model.TraceExternalCall) int64 {
	return int64(recorderExternalCallBytes +
		len(call.Key) + len(call.Source) + len(call.Tool) + len(call.ServerLabel))
}

// clipString bounds a retained string, cutting on a rune boundary so the value
// stays valid UTF-8 for JSON serialization and for text columns.
//
// The result is COPIED, not sliced. `s[:cut]` would share the caller's backing
// array, so a recorder holding the "clipped" value would keep the entire
// original alive for the whole request: a 1 MiB URL clipped to 8 KiB still
// pinned 1,056,944 bytes per active recorder, measured in
// docs/benchmarks/20260908_w0-w1-acceptance.md. That defeats the point of the
// bound exactly where it matters -- the input is attacker-influenced and the
// active set can hold hundreds of thousands of recorders -- so the copy is
// load-bearing, not defensive. Only oversized values pay for it; the common
// path returns s untouched and allocates nothing.
//
// Parameters:
//   - s: the value to bound.
//   - maxBytes: the ceiling in bytes; values below 1 clip to the empty string.
//
// Return values:
//   - string: the bounded value, owning its own storage when it was cut.
//   - bool: whether the value had to be cut.
func clipString(s string, maxBytes int) (string, bool) {
	if len(s) <= maxBytes {
		return s, false
	}
	if maxBytes <= 0 {
		return "", true
	}

	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.Clone(s[:cut]), true
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
