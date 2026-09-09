package tracing

// Active-recorder admission (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1,
// "Active requests").
//
// The completed-record queue bounds only FINISHED traces. Nothing bounded the
// ACTIVE side: at 10000 requests/second with a 60-second mean streaming
// lifetime roughly 600000 requests hold a recorder at the same time, so the
// memory model was unprovable no matter how small a single record was.
//
// Admission is one atomic counter rather than a registry: the hot path runs on
// every request, a map would need a lock and would retain the recorders it
// tracks, and nothing here needs to enumerate the live set -- only to count it.

import (
	"sync/atomic"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
)

// activeRecorders counts requests that currently hold an admitted Recorder.
//
// It is incremented by admitRecorder and decremented by releaseRecorder, which
// Recorder.Finish calls exactly once per admitted recorder.
var activeRecorders atomic.Int64

// admitRecorder reserves one active-recorder slot.
//
// A limit of zero means unlimited, which is the standalone default: trace
// coverage there is expected to be complete, so admission must not silently
// start dropping traces on an upgrade.
//
// Parameters: none.
//
// Return values:
//   - bool: true when the caller owns a slot and must release it exactly once;
//     false when TRACE_MAX_ACTIVE_RECORDERS is already reached, in which case
//     the drop has been counted and the request must run untraced.
func admitRecorder() bool {
	limit := int64(config.TraceMaxActiveRecorders)
	if limit <= 0 {
		active := activeRecorders.Add(1)
		metrics.UpdateTraceActive(int(active), 0)
		return true
	}

	for {
		active := activeRecorders.Load()
		if active >= limit {
			metrics.RecordTraceOutcome(metrics.TraceOutcomeDroppedActiveLimit, 1)
			metrics.UpdateTraceActive(int(active), int(limit))
			return false
		}
		if activeRecorders.CompareAndSwap(active, active+1) {
			metrics.UpdateTraceActive(int(active+1), int(limit))
			return true
		}
	}
}

// releaseRecorder frees one active-recorder slot.
//
// Parameters: none.
//
// Return values: none.
func releaseRecorder() {
	active := activeRecorders.Add(-1)
	if active < 0 {
		// Defensive: a double release would otherwise drive the gauge negative
		// and permanently understate occupancy. Recorder.Finish guarantees a
		// single release, so this only guards a future caller.
		activeRecorders.CompareAndSwap(active, 0)
		active = 0
	}
	metrics.UpdateTraceActive(int(active), config.TraceMaxActiveRecorders)
}

// activeRecorderCount reports how many requests currently hold a recorder.
//
// Parameters: none.
//
// Return values:
//   - int64: the current occupancy.
func activeRecorderCount() int64 {
	return activeRecorders.Load()
}
