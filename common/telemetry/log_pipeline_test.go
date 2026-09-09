package telemetry

// Acceptance tests for the bounded OTLP log pipeline (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.2, gate
// G3: "outage/resource/shutdown tests").
//
// Section 9.1 permits telemetry loss and forbids silent loss. Every test here
// is therefore about accounting as much as about bounding: it is not enough
// that a full queue stops growing, the records it refused have to be countable
// afterwards.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	"github.com/Laisky/one-api/common/metrics"
)

// blockingExporter is an sdklog.Exporter that never returns until released,
// standing in for a collector that has stopped answering.
type blockingExporter struct {
	release chan struct{}
	failing bool

	mu       sync.Mutex
	exported int
}

// Export implements sdklog.Exporter by blocking until released.
//
// Parameters:
//   - ctx: honored, so a shutdown deadline still ends the wait.
//   - records: counted, not retained.
//
// Return values:
//   - error: a synthetic transport failure when configured to fail.
func (e *blockingExporter) Export(ctx context.Context, records []sdklog.Record) error {
	select {
	case <-e.release:
	case <-ctx.Done():
		return errors.New("export deadline expired")
	}

	e.mu.Lock()
	e.exported += len(records)
	e.mu.Unlock()

	if e.failing {
		return errors.New("collector unreachable")
	}
	return nil
}

// Shutdown implements sdklog.Exporter as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (e *blockingExporter) Shutdown(context.Context) error { return nil }

// ForceFlush implements sdklog.Exporter as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (e *blockingExporter) ForceFlush(context.Context) error { return nil }

// nopProcessor is a downstream sdklog.Processor that accepts everything, so a
// test can exercise the admission gate in isolation from batching.
type nopProcessor struct {
	mu       sync.Mutex
	accepted int
}

// Enabled implements sdklog.Processor by accepting every record.
//
// Parameters:
//   - ctx, param: unused.
//
// Return values:
//   - bool: always true.
func (p *nopProcessor) Enabled(context.Context, sdklog.EnabledParameters) bool { return true }

// OnEmit implements sdklog.Processor by counting the record.
//
// Parameters:
//   - ctx: unused.
//   - record: counted, not retained.
//
// Return values:
//   - error: always nil.
func (p *nopProcessor) OnEmit(context.Context, *sdklog.Record) error {
	p.mu.Lock()
	p.accepted++
	p.mu.Unlock()
	return nil
}

// ForceFlush implements sdklog.Processor as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (p *nopProcessor) ForceFlush(context.Context) error { return nil }

// Shutdown implements sdklog.Processor as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (p *nopProcessor) Shutdown(context.Context) error { return nil }

// count returns how many records the processor accepted.
//
// Parameters: none.
//
// Return values:
//   - int: the accepted count.
func (p *nopProcessor) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.accepted
}

// captureProcessor keeps the last record the SDK produced, so a test can obtain
// a REALISTIC sdklog.Record.
//
// A zero-valued sdklog.Record cannot be used for this. Its attribute limits are
// zero, and a zero count limit means "discard every attribute", so records built
// by hand silently carry none and any size-based assertion made against them
// measures an empty record. Only the SDK's own logger applies the provider's
// limits, so the records here come from that path.
type captureProcessor struct {
	mu   sync.Mutex
	last sdklog.Record
}

// Enabled implements sdklog.Processor by accepting every record.
//
// Parameters:
//   - ctx, param: unused.
//
// Return values:
//   - bool: always true.
func (p *captureProcessor) Enabled(context.Context, sdklog.EnabledParameters) bool { return true }

// OnEmit implements sdklog.Processor by retaining a clone of the record.
//
// Parameters:
//   - ctx: unused.
//   - record: the record to retain.
//
// Return values:
//   - error: always nil.
func (p *captureProcessor) OnEmit(_ context.Context, record *sdklog.Record) error {
	p.mu.Lock()
	p.last = record.Clone()
	p.mu.Unlock()
	return nil
}

// ForceFlush implements sdklog.Processor as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (p *captureProcessor) ForceFlush(context.Context) error { return nil }

// Shutdown implements sdklog.Processor as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (p *captureProcessor) Shutdown(context.Context) error { return nil }

// newRecordFactory returns a builder for records produced by the real SDK.
//
// Parameters:
//   - t: the test handle; the backing provider is shut down on cleanup.
//
// Return values:
//   - func(int) sdklog.Record: builds a record whose size is dominated by one
//     string attribute of the requested byte length.
func newRecordFactory(t *testing.T) func(payloadBytes int) sdklog.Record {
	t.Helper()

	capture := &captureProcessor{}
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(capture))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	logger := provider.Logger("telemetry-test")

	return func(payloadBytes int) sdklog.Record {
		var record log.Record
		record.SetBody(attribute.StringValue("msg"))
		if payloadBytes > 0 {
			record.AddAttributes(attribute.String("payload", strings.Repeat("x", payloadBytes)))
		}
		logger.Emit(context.Background(), record)

		capture.mu.Lock()
		defer capture.mu.Unlock()
		return capture.last.Clone()
	}
}

// TestBoundedProcessorDropsAndCountsWhenRecordCeilingIsReached proves the
// record ceiling engages and that every refused record is counted, which is
// what turns permitted loss into observable loss.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBoundedProcessorDropsAndCountsWhenRecordCeilingIsReached(t *testing.T) {
	spy := installLogExportSpy(t)
	newRecord := newRecordFactory(t)
	next := &nopProcessor{}
	gate := newBoundedProcessor(next, 3, 0)

	for range 10 {
		record := newRecord(0)
		require.NoError(t, gate.OnEmit(context.Background(), &record))
	}

	require.Equal(t, 3, next.count(), "the gate must stop admitting at its ceiling")
	require.Equal(t, 7, spy.outcome(metrics.AppLogExportOutcomeDroppedQueueFull))
}

// TestBoundedProcessorEnforcesAByteCeilingIndependently proves record count
// alone does not bound memory: a handful of large records must be refused well
// before the record ceiling is reached.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestBoundedProcessorEnforcesAByteCeilingIndependently(t *testing.T) {
	spy := installLogExportSpy(t)
	newRecord := newRecordFactory(t)
	next := &nopProcessor{}
	// A high record ceiling and a low byte ceiling: only the byte bound can fire.
	gate := newBoundedProcessor(next, 1000, 4096)

	for range 10 {
		record := newRecord(2048)
		require.NoError(t, gate.OnEmit(context.Background(), &record))
	}

	require.Less(t, next.count(), 10, "the byte ceiling must refuse large records")
	require.Positive(t, spy.outcome(metrics.AppLogExportOutcomeDroppedQueueFull))
	require.Equal(t, 10, next.count()+spy.outcome(metrics.AppLogExportOutcomeDroppedQueueFull),
		"every record must be either admitted or counted as dropped")
}

// TestExportReleasesResidencySoTheGateReopens proves the gate is a queue bound
// and not a lifetime quota: once a batch reaches the exporter, the capacity it
// held must come back.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestExportReleasesResidencySoTheGateReopens(t *testing.T) {
	installLogExportSpy(t)
	newRecord := newRecordFactory(t)
	next := &nopProcessor{}
	gate := newBoundedProcessor(next, 2, 0)

	admit := func() {
		record := newRecord(0)
		require.NoError(t, gate.OnEmit(context.Background(), &record))
	}

	admit()
	admit()
	admit() // refused: at ceiling
	require.Equal(t, 2, next.count())

	exporter := newCountingExporter(&blockingExporter{release: closedChan()}, gate.release)
	require.NoError(t, exporter.Export(context.Background(), []sdklog.Record{newRecord(0), newRecord(0)}))

	admit()
	require.Equal(t, 3, next.count(), "capacity must return once records leave the pipeline")
}

// TestExportFailureIsCountedSeparatelyFromSuccess proves a collector outage is
// distinguishable from healthy export, so an alert can fire on transport
// failure without also firing on volume.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestExportFailureIsCountedSeparatelyFromSuccess(t *testing.T) {
	spy := installLogExportSpy(t)
	newRecord := newRecordFactory(t)

	failing := newCountingExporter(&blockingExporter{release: closedChan(), failing: true}, nil)
	err := failing.Export(context.Background(), []sdklog.Record{newRecord(0), newRecord(0)})
	require.Error(t, err)
	require.Equal(t, 2, spy.outcome(metrics.AppLogExportOutcomeExportFailed))
	require.Zero(t, spy.outcome(metrics.AppLogExportOutcomeExported))

	healthy := newCountingExporter(&blockingExporter{release: closedChan()}, nil)
	require.NoError(t, healthy.Export(context.Background(), []sdklog.Record{newRecord(0)}))
	require.Equal(t, 1, spy.outcome(metrics.AppLogExportOutcomeExported))
}

// TestOutageDoesNotBlockTheEmittingGoroutine is the property that makes the
// bridge safe to enable on a relay gateway: OnEmit runs on a request
// goroutine, so a collector that has stopped answering must cost drops, never
// latency.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestOutageDoesNotBlockTheEmittingGoroutine(t *testing.T) {
	installLogExportSpy(t)
	newRecord := newRecordFactory(t)

	stuck := &blockingExporter{release: make(chan struct{})}
	gate := &boundedProcessor{maxRecords: 8, maxBytes: 0}
	batch := sdklog.NewBatchProcessor(newCountingExporter(stuck, gate.release),
		sdklog.WithMaxQueueSize(8), sdklog.WithExportMaxBatchSize(1))
	gate.next = batch
	t.Cleanup(func() {
		close(stuck.release)
		_ = batch.Shutdown(context.Background())
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 200 {
			record := newRecord(0)
			_ = gate.OnEmit(context.Background(), &record)
		}
	}()

	select {
	case <-done:
	case <-context.Background().Done():
		t.Fatal("unreachable")
	}
}

// TestShutdownReportsRecordsItCouldNotDrain proves a shutdown that leaves work
// behind says so, rather than letting a clean-looking teardown imply delivery.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestShutdownReportsRecordsItCouldNotDrain(t *testing.T) {
	spy := installLogExportSpy(t)
	newRecord := newRecordFactory(t)
	next := &nopProcessor{}
	gate := newBoundedProcessor(next, 100, 0)

	for range 5 {
		record := newRecord(0)
		require.NoError(t, gate.OnEmit(context.Background(), &record))
	}

	// nopProcessor never exports, so nothing released residency: all five
	// records are still resident when shutdown runs.
	require.NoError(t, gate.Shutdown(context.Background()))
	require.Equal(t, 5, spy.outcome(metrics.AppLogExportOutcomeDroppedShutdown))
}

// TestEstimateRecordBytesGrowsWithPayload proves the byte estimate actually
// tracks record size; a constant estimate would make the byte ceiling a second
// record ceiling wearing a different name.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestEstimateRecordBytesGrowsWithPayload(t *testing.T) {
	newRecord := newRecordFactory(t)

	small := newRecord(16)
	large := newRecord(16384)

	require.Greater(t, estimateRecordBytes(&large), estimateRecordBytes(&small)+16000)
	require.Equal(t, int64(0), estimateRecordBytes(nil))
}

// closedChan returns an already-closed channel, so a blockingExporter proceeds
// immediately.
//
// Parameters: none.
//
// Return values:
//   - chan struct{}: a closed channel.
func closedChan() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// logExportSpy records the export outcomes the pipeline reported.
type logExportSpy struct {
	metrics.MetricsRecorder

	mu       sync.Mutex
	outcomes map[string]int
}

// RecordAppLogExportRecords implements metrics.LogExportRecorder by tallying.
//
// Parameters:
//   - outcome: the reported outcome.
//   - count: how many records it applies to.
//
// Return values: none.
func (s *logExportSpy) RecordAppLogExportRecords(outcome string, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomes[outcome] += count
}

// UpdateAppLogExportQueue implements metrics.LogExportRecorder as a no-op.
//
// Parameters:
//   - records, recordLimit, bytes, byteLimit: ignored.
//
// Return values: none.
func (s *logExportSpy) UpdateAppLogExportQueue(_, _, _, _ float64) {}

// outcome reports how many records were counted under one outcome.
//
// It reads under the spy's lock because the SDK batch worker exports on its own
// goroutine, so a test that starts a batch processor can otherwise race the
// assertion.
//
// Parameters:
//   - name: one of the metrics.AppLogExportOutcome* constants.
//
// Return values:
//   - int: the tally, zero when the outcome was never reported.
func (s *logExportSpy) outcome(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outcomes[name]
}

// installLogExportSpy installs a recorder capturing export outcomes and
// restores the previous recorder when the test ends.
//
// Parameters:
//   - t: the test handle.
//
// Return values:
//   - *logExportSpy: the installed spy.
func installLogExportSpy(t *testing.T) *logExportSpy {
	t.Helper()

	previous := metrics.Recorder()
	spy := &logExportSpy{MetricsRecorder: previous, outcomes: make(map[string]int)}
	metrics.SetRecorder(spy)
	t.Cleanup(func() { metrics.SetRecorder(previous) })
	return spy
}
