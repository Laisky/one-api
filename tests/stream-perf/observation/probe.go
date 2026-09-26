// Package observation supplies bounded numeric-only hooks for explicit diagnostic build overlays.
// Normal gateway and load-driver builds do not import this package.
package observation

import (
	"context"
	"runtime/trace"
	"strconv"

	"github.com/Laisky/errors/v2"
)

const (
	// Header carries a synthetic numeric request index, never a credential or payload.
	Header = "X-Oneapi-Stream-Observation"
	// MaxRequests bounds diagnostic driver work and the numeric request identity.
	MaxRequests = 8192
	// MaxFrames bounds retained client timestamps and gateway frame identities per request.
	MaxFrames = 2048
	// Stride samples one request in every thirty-two without a content-dependent decision.
	Stride = 32
	// StateKey names the request-local counter in the overlaid renderer.
	StateKey = "oneapi.stream.observation.frame"
)

// Selected reports whether index is a valid, deterministically sampled diagnostic request.
func Selected(index int) bool { return index >= 0 && index < MaxRequests && index%Stride == 0 }

// recorder owns injectable measurement dependencies without mutating process-global test state.
type recorder struct {
	clock  func() (int64, error)
	active func() bool
	emit   func(context.Context, string, string)
}

var live = recorder{clock: MonotonicNS, active: trace.IsEnabled, emit: trace.Log}

// Span names one data frame and retains only the request context, never its content.
type Span struct {
	request  int
	frame    int
	ctx      context.Context
	recorder *recorder
}

// Begin observes render entry for a sampled request using the caller-owned frame counter.
func Begin(ctx context.Context, id string, frame int) Span { return live.begin(ctx, id, frame) }

// begin validates numeric identity and frame bounds without accepting or retaining payload data.
func (r *recorder) begin(ctx context.Context, id string, frame int) Span {
	index, err := strconv.Atoi(id)
	if ctx == nil || err != nil || !Selected(index) || strconv.Itoa(index) != id {
		return Span{}
	}
	if frame < 0 || frame >= MaxFrames {
		if r.active() {
			r.emit(ctx, "oneapi.sse.error", "frame_limit")
		}
		return Span{}
	}
	s := Span{request: index, frame: frame, ctx: ctx, recorder: r}
	s.mark("begin")
	return s
}

// Tracked reports whether the diagnostic caller must advance this request's frame counter.
// The caller owns the counter even when the trace is not active, preserving capture-boundary identity.
func (s Span) Tracked() bool { return s.recorder != nil }

// BeforeFlush records entry to the original flush call; it does not flush or retain bytes itself.
func (s Span) BeforeFlush() { s.mark("flush_start") }

// AfterFlush records return from the original flush call, not physical packet delivery.
func (s Span) AfterFlush() { s.mark("flush_end") }

// mark emits a fixed-schema numeric marker only while the same trace is active.
func (s Span) mark(stage string) {
	if s.recorder == nil || !s.recorder.active() {
		return
	}
	now, err := s.recorder.clock()
	if err != nil || now <= 0 {
		s.recorder.emit(s.ctx, "oneapi.sse.error", "clock_failure")
		return
	}
	message := strconv.Itoa(s.request) + "/" + strconv.Itoa(s.frame) + "/" + stage + "/" + strconv.FormatInt(now, 10)
	s.recorder.emit(s.ctx, "oneapi.sse", message)
}

// AppendClient records observation of one complete SSE data event before JSON validation.
// It is called only by the overlaid driver for sampled requests, never by the normal driver.
func AppendClient(times []int64) ([]int64, error) { return appendClient(times, MonotonicNS) }

// appendClient validates the measurement clock and rejects overflow before allocating more event slots.
func appendClient(times []int64, clock func() (int64, error)) ([]int64, error) {
	if len(times) >= MaxFrames {
		return times, errors.New("correlation frame limit exceeded")
	}
	at, err := clock()
	if err != nil {
		return times, errors.Wrap(err, "read correlation clock")
	}
	if at <= 0 || len(times) > 0 && at < times[len(times)-1] {
		return times, errors.New("invalid correlation clock")
	}
	return append(times, at), nil
}
