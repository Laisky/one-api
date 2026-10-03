package telemetry

// Bounded, observable OTLP log pipeline (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.2:
// "test queue bounds, transport outage, sampling and final flush", and W3.3:
// "queue bytes, drops ... from their actual operational sources").
//
// The SDK's BatchProcessor is already a bounded ring, and this file does NOT
// replace it. What it adds is the two properties the SDK cannot give a gateway:
//
//  1. Observable loss. The SDK's queue drops the OLDEST record when it
//     overflows and reports that only through its own internal logger and an
//     experimental, env-gated self-observability signal. Section 9.1 permits
//     telemetry loss but requires it to be counted. An admission gate in front
//     of the batch queue makes every drop this process's own, countable event.
//
//  2. A byte bound. The SDK bounds records, not memory. One log line carrying a
//     large field can be orders of magnitude larger than the median line, so a
//     record count alone does not bound what the queue costs.
//
// The gate's ceiling is set to the same value as the batch queue's, so the SDK
// ring never overflows first and "dropped_queue_full" always means the gate.
// Residency is released when the exporter is handed a batch, which is the last
// moment the pipeline still owns the records.

import (
	"context"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	"github.com/Laisky/one-api/common/metrics"
)

// recordOverheadBytes approximates the fixed per-record cost that is not
// captured by walking attributes: the timestamps, severity, trace context,
// scope pointer and slice headers the SDK keeps alive for a queued record.
//
// It is an estimate, and it is stated as one. The byte ceiling exists to stop a
// pathological record from consuming the heap, not to account memory exactly,
// and an estimate that is roughly right is what makes the bound engage before
// the process is in trouble rather than after.
const recordOverheadBytes = 256

// boundedProcessor is a sdklog.Processor that admits records into a downstream
// processor only while the pipeline is within its record and byte ceilings.
//
// It never blocks: OnEmit runs on the goroutine that called the logger, which in
// this gateway is a request goroutine, and blocking it on a telemetry queue
// would turn a collector outage into a relay outage.
type boundedProcessor struct {
	next sdklog.Processor

	maxRecords int64
	maxBytes   int64

	residentRecords atomic.Int64
	residentBytes   atomic.Int64
}

// Compile-time proof that the gate is a processor the SDK will accept.
var _ sdklog.Processor = (*boundedProcessor)(nil)

// newBoundedProcessor wraps next with record and byte admission ceilings.
//
// Parameters:
//   - next: the downstream processor, normally the SDK batch processor.
//   - maxRecords: the resident record ceiling; values below one disable the
//     record bound rather than admitting nothing.
//   - maxBytes: the resident byte ceiling; values below one disable the byte
//     bound.
//
// Return values:
//   - *boundedProcessor: the configured gate.
func newBoundedProcessor(next sdklog.Processor, maxRecords int64, maxBytes int64) *boundedProcessor {
	return &boundedProcessor{next: next, maxRecords: maxRecords, maxBytes: maxBytes}
}

// Enabled implements sdklog.Processor by deferring to the downstream processor.
//
// The severity floor is applied on the zap side, in the bridge core, so a
// filtered record is never converted into a log.Record at all. Repeating it
// here would only duplicate a decision that has already been made.
//
// Parameters:
//   - ctx: the emitting context.
//   - param: the subset of record information available before construction.
//
// Return values:
//   - bool: whatever the downstream processor reports.
func (p *boundedProcessor) Enabled(ctx context.Context, param sdklog.EnabledParameters) bool {
	return p.next.Enabled(ctx, param)
}

// OnEmit implements sdklog.Processor by admitting the record when the pipeline
// is within both ceilings, and counting it as a drop otherwise.
//
// Parameters:
//   - ctx: the emitting context.
//   - record: the record to admit; nil is ignored.
//
// Return values:
//   - error: whatever the downstream processor reports; a rejected record is
//     NOT an error, because losing best-effort telemetry must not surface as a
//     failure at the log call site.
func (p *boundedProcessor) OnEmit(ctx context.Context, record *sdklog.Record) error {
	if record == nil {
		return nil
	}

	size := estimateRecordBytes(record)
	records := p.residentRecords.Add(1)
	bytes := p.residentBytes.Add(size)

	overRecords := p.maxRecords > 0 && records > p.maxRecords
	overBytes := p.maxBytes > 0 && bytes > p.maxBytes
	if overRecords || overBytes {
		p.release(1, size)
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedQueueFull, 1)
		return nil
	}

	p.publishQueue()
	return errors.Wrap(p.next.OnEmit(ctx, record), "hand log record to batch processor")
}

// ForceFlush implements sdklog.Processor by draining the downstream processor.
//
// Parameters:
//   - ctx: bounds how long the drain may take.
//
// Return values:
//   - error: whatever the downstream processor reports.
func (p *boundedProcessor) ForceFlush(ctx context.Context) error {
	return errors.Wrap(p.next.ForceFlush(ctx), "flush log batch processor")
}

// Shutdown implements sdklog.Processor by shutting the downstream processor
// down and reporting any records the drain did not place.
//
// Parameters:
//   - ctx: bounds how long the shutdown may take.
//
// Return values:
//   - error: whatever the downstream processor reports.
func (p *boundedProcessor) Shutdown(ctx context.Context) error {
	err := p.next.Shutdown(ctx)

	// Whatever is still resident after the downstream shutdown returned was
	// never handed to the exporter. Reporting it keeps a truncated drain
	// visible instead of letting a clean-looking shutdown imply delivery.
	if remaining := p.residentRecords.Swap(0); remaining > 0 {
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedShutdown, int(remaining))
	}
	p.residentBytes.Store(0)
	p.publishQueue()

	return errors.Wrap(err, "shutdown log batch processor")
}

// release removes residency for records that have left the pipeline.
//
// Parameters:
//   - records: how many records left.
//   - bytes: how many estimated bytes they occupied.
//
// Return values: none.
func (p *boundedProcessor) release(records int64, bytes int64) {
	if records > 0 {
		if remaining := p.residentRecords.Add(-records); remaining < 0 {
			// A negative residency would make the gate admit forever. It can
			// only happen if the SDK hands the exporter more records than were
			// admitted, which nothing should do; clamping keeps the bound
			// enforceable rather than trusting the invariant.
			p.residentRecords.CompareAndSwap(remaining, 0)
		}
	}
	if bytes > 0 {
		if remaining := p.residentBytes.Add(-bytes); remaining < 0 {
			p.residentBytes.CompareAndSwap(remaining, 0)
		}
	}
}

// publishQueue reports current occupancy against the configured ceilings.
//
// Parameters: none.
//
// Return values: none.
func (p *boundedProcessor) publishQueue() {
	metrics.UpdateAppLogExportQueue(
		p.residentRecords.Load(), p.maxRecords,
		p.residentBytes.Load(), p.maxBytes)
}

// countingExporter wraps an OTLP log exporter to release queue residency and
// count export outcomes.
//
// It reports LOCAL handoff only. A successful Export means the exporter
// accepted and transmitted the batch without returning an error; it is not
// proof the collector durably stored anything, exactly as the OTLP trace sink
// refuses to claim collector persistence.
type countingExporter struct {
	next    sdklog.Exporter
	release func(records int64, bytes int64)
}

// Compile-time proof that the wrapper is an exporter the SDK will accept.
var _ sdklog.Exporter = (*countingExporter)(nil)

// newCountingExporter wraps next so batches released to it are accounted.
//
// Parameters:
//   - next: the real exporter.
//   - release: called with the record count and estimated bytes of every batch
//     handed to next, before the export is attempted.
//
// Return values:
//   - *countingExporter: the wrapper.
func newCountingExporter(next sdklog.Exporter, release func(records int64, bytes int64)) *countingExporter {
	return &countingExporter{next: next, release: release}
}

// Export implements sdklog.Exporter by releasing residency for the batch and
// counting the outcome.
//
// Residency is released BEFORE the export is attempted, not after. An export
// can take up to the configured export timeout, and holding the gate closed for
// that whole window would make one slow collector round trip drop records that
// the queue had room for.
//
// Parameters:
//   - ctx: bounds the export.
//   - records: the batch to export.
//
// Return values:
//   - error: whatever the wrapped exporter reports.
func (e *countingExporter) Export(ctx context.Context, records []sdklog.Record) error {
	if len(records) == 0 {
		return nil
	}

	var bytes int64
	for i := range records {
		bytes += estimateRecordBytes(&records[i])
	}
	if e.release != nil {
		e.release(int64(len(records)), bytes)
	}

	if err := e.next.Export(ctx, records); err != nil {
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeExportFailed, len(records))
		return errors.Wrap(err, "export log records")
	}
	metrics.RecordAppLogExport(metrics.AppLogExportOutcomeExported, len(records))
	return nil
}

// Shutdown implements sdklog.Exporter by shutting the wrapped exporter down.
//
// Parameters:
//   - ctx: bounds the shutdown.
//
// Return values:
//   - error: whatever the wrapped exporter reports.
func (e *countingExporter) Shutdown(ctx context.Context) error {
	return errors.Wrap(e.next.Shutdown(ctx), "shutdown log exporter")
}

// ForceFlush implements sdklog.Exporter by flushing the wrapped exporter.
//
// Parameters:
//   - ctx: bounds the flush.
//
// Return values:
//   - error: whatever the wrapped exporter reports.
func (e *countingExporter) ForceFlush(ctx context.Context) error {
	return errors.Wrap(e.next.ForceFlush(ctx), "flush log exporter")
}

// estimateRecordBytes approximates the memory one queued log record occupies.
//
// Parameters:
//   - record: the record to measure; nil measures zero.
//
// Return values:
//   - int64: the estimated size in bytes, never negative.
func estimateRecordBytes(record *sdklog.Record) int64 {
	if record == nil {
		return 0
	}

	size := int64(recordOverheadBytes)
	size += int64(len(record.SeverityText()))
	size += attributeValueBytes(record.Body())
	record.WalkAttributes(func(kv attribute.KeyValue) bool {
		size += int64(len(kv.Key)) + attributeValueBytes(kv.Value)
		return true
	})
	return size
}

// attributeValueBytes approximates the memory an attribute value occupies.
//
// Nested maps and slices are measured by their rendered form rather than by
// walking them: the walk would need its own depth bound, and the estimate only
// has to be close enough for a ceiling to engage.
//
// Parameters:
//   - value: the value to measure.
//
// Return values:
//   - int64: the estimated size in bytes.
func attributeValueBytes(value attribute.Value) int64 {
	switch value.Type() {
	case attribute.STRING:
		return int64(len(value.AsString()))
	case attribute.BOOL:
		return 1
	case attribute.INT64, attribute.FLOAT64:
		return 8
	case attribute.INVALID:
		return 0
	default:
		return int64(len(value.Emit()))
	}
}
