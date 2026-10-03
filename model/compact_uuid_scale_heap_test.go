package model

// Heap-sampling machinery for the AUTO-T25 scale suite. Section 11 requires the qualification
// workload to sample heap at least every 100 milliseconds; this sampler is that requirement,
// kept in its own file only for the 600-line ceiling (proposal section 9.3).

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// compactScaleRaiseMax raises an atomic high-water mark to value when value is higher.
//
// Parameters:
//   - ceiling: atomic high-water mark to raise.
//   - value: candidate observation.
//
// Return values: none.
func compactScaleRaiseMax(ceiling *atomic.Int64, value int64) {
	for {
		current := ceiling.Load()
		if value <= current || ceiling.CompareAndSwap(current, value) {
			return
		}
	}
}

// compactScaleHeapSampler samples HeapAlloc on a fixed interval and keeps the peak.
type compactScaleHeapSampler struct {
	// peak is the highest observed HeapAlloc in bytes.
	peak atomic.Int64
	// samples counts observations taken, so a test can prove sampling happened.
	samples atomic.Int64
	// stop closes to end the sampling goroutine; done closes once it has exited.
	stop, done chan struct{}
	// once guards a double stop.
	once sync.Once
}

// compactScaleStartHeapSampler starts sampling the heap every compactScaleHeapSampleInterval.
//
// Parameters: none.
//
// Return values:
//   - *compactScaleHeapSampler: a running sampler the caller must stop.
func compactScaleStartHeapSampler() *compactScaleHeapSampler {
	sampler := &compactScaleHeapSampler{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(sampler.done)
		ticker := time.NewTicker(compactScaleHeapSampleInterval)
		defer ticker.Stop()
		for {
			stats := runtime.MemStats{}
			runtime.ReadMemStats(&stats)
			sampler.samples.Add(1)
			compactScaleRaiseMax(&sampler.peak, int64(stats.HeapAlloc))
			select {
			case <-sampler.stop:
				return
			case <-ticker.C:
			}
		}
	}()
	return sampler
}

// stopSampling ends sampling and waits for the goroutine to exit.
//
// Parameters: none.
//
// Return values: none.
func (sampler *compactScaleHeapSampler) stopSampling() {
	sampler.once.Do(func() {
		close(sampler.stop)
		<-sampler.done
	})
}

// compactScaleBoundsRecorder observes every statement the coordinator issues on a live handle.
//
// This turns "the bounds hold" from a code-reading claim into an observation: gorm's raw and row
// callbacks fire with Statement.Vars still un-substituted, so the recorder sees the exact bind
// count and LIMIT value the engine was handed. A static assertion on compactBatchRows alone would
// not catch a query that bypassed the batch formula.
type compactScaleBoundsRecorder struct {
	// enabled gates recording so the fixture build is not measured as coordinator work.
	enabled atomic.Bool
	// maxBinds is the highest bind count observed on any statement.
	maxBinds atomic.Int64
	// maxLimit is the highest LIMIT bind value observed on any statement.
	maxLimit atomic.Int64
	// statements counts recorded statements, proving the recorder actually saw traffic.
	statements atomic.Int64
}
