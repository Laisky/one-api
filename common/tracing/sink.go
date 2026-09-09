package tracing

// Trace sinks (proposal docs/proposals/20260905_observability-data-tiering.md,
// Phase 1 / W1.2).
//
// A sink is where a finished, sampled-in trace goes. Separating it from the
// request path is what lets the same code write to SQL, emit an OTLP span, or
// drop the record, chosen entirely by configuration.

import (
	"context"
	"sync"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/common/telemetry"
	"github.com/Laisky/one-api/model"
)

// TraceSink accepts completed request traces for durable or external recording.
//
// Implementations must be safe for concurrent use and must never block the
// calling request goroutine.
type TraceSink interface {
	// Submit hands a completed trace to the sink. It returns an error only for
	// programming faults; transport and capacity failures are counted as
	// dropped records and reported through metrics, never propagated to the
	// request path.
	Submit(ctx context.Context, row *model.Trace) error
	// Flush blocks until buffered records are durably handed off or ctx
	// expires.
	//
	// A nil return means a durable sink persisted the records it accepted. An
	// OTLP sink can only report local SDK recording because collector delivery is
	// asynchronous and owned by the provider shutdown path. Durable trace loss
	// is permitted, but an implementation must report it with a count rather
	// than returning nil because no work is pending.
	Flush(ctx context.Context) error
	// Close flushes and releases resources. It is idempotent, and a deadline
	// that expires with work outstanding reports how much was unfinished.
	Close(ctx context.Context) error
}

var (
	activeSinkMu sync.RWMutex
	activeSink   TraceSink
)

// InitSinks builds the sinks named by TRACE_SINK and installs them as the
// process trace sink. Calling it twice closes the previous sink first, so a
// test or a reload can safely re-init.
//
// It must run AFTER telemetry.InitOpenTelemetry: an otlp sink built while the
// process still carries OpenTelemetry's global no-op provider exports nothing,
// and the check below turns that ordering mistake into a startup failure rather
// than a per-request span-record failure forever.
//
// Parameters:
//   - ctx: lifetime scope for the sinks' background workers.
//
// Return values:
//   - error: wrapped failure when the configuration is invalid, when the OTLP
//     sink is requested without an installed provider, or when a configured
//     sink cannot be constructed.
func InitSinks(ctx context.Context) error {
	// The combination rule is re-checked here, not only in the environment
	// matrix, because config.TraceSinks is a package variable: a direct
	// assignment (a test, an embedder, a future reload path) can reintroduce
	// "db,none", which traceDisabled()'s len(sinks)==1 check reads as "db plus
	// a sink that drops every record" instead of "tracing off".
	if err := config.ValidateTraceSinkCombination(config.TraceSinks); err != nil {
		return errors.Wrap(err, "validate trace sink combination")
	}
	if err := config.ValidateOpenTelemetryConfig(config.OpenTelemetryEnabled, config.OpenTelemetryEndpoint); err != nil {
		return errors.Wrap(err, "validate OpenTelemetry configuration")
	}
	if err := config.ValidateTraceSinkOpenTelemetryConfig(config.TraceSinks, config.OpenTelemetryEnabled); err != nil {
		return errors.Wrap(err, "validate trace sink configuration")
	}
	if err := config.ValidateSyncTraceConfiguration(config.TraceWriteMode, config.TraceSinks, config.TraceSampleRate); err != nil {
		return errors.Wrap(err, "validate synchronous trace configuration")
	}
	if err := validateSinkRuntimeRequirements(config.TraceSinks); err != nil {
		return err
	}

	sinks := make([]TraceSink, 0, len(config.TraceSinks))
	for _, name := range config.TraceSinks {
		switch name {
		case config.TraceSinkDB:
			sinks = append(sinks, newSQLSink(ctx))
		case config.TraceSinkOTLP:
			// OTEL_ENABLED alone only says an operator ASKED for a provider.
			// Whether one was actually installed is runtime state owned by
			// common/telemetry, and it is what the sink's
			// otel.GetTracerProvider() will resolve to. Without this check a
			// failed or not-yet-run InitOpenTelemetry leaves the global no-op
			// provider in place, every Submit finds !span.IsRecording(), and a
			// startup misconfiguration is rediscovered once per request --
			// counted as TraceOutcomeSpanRecordFailed -- for the life of the
			// process while traces go nowhere.
			sinks = append(sinks, newOTLPSink())
		case config.TraceSinkNone:
			sinks = append(sinks, nullSink{})
		default:
			return errors.Errorf("unknown trace sink %q", name)
		}
	}

	var next TraceSink
	switch len(sinks) {
	case 0:
		next = nullSink{}
	case 1:
		next = sinks[0]
	default:
		next = &multiSink{sinks: sinks}
	}

	activeSinkMu.Lock()
	prev := activeSink
	activeSink = next
	activeSinkMu.Unlock()

	if prev != nil {
		if err := prev.Close(ctx); err != nil {
			logger.Logger.Warn("failed to close previous trace sink", zap.Error(err))
		}
	}

	logger.Logger.Info("trace sinks initialized",
		zap.Strings("sinks", config.TraceSinks),
		zap.String("write_mode", config.TraceWriteMode),
		zap.Float64("sample_rate", config.TraceSampleRate),
		zap.Int("queue_size", config.TraceQueueSize),
		zap.Int("batch_size", config.TraceBatchSize))

	return nil
}

// validateSinkRuntimeRequirements checks every requested child before any
// stateful sink starts background workers.
//
// Parameters:
//   - names: configured trace sink names.
//
// Return values:
//   - error: wrapped failure when a requested runtime dependency is absent.
func validateSinkRuntimeRequirements(names []string) error {
	for _, name := range names {
		switch name {
		case config.TraceSinkDB, config.TraceSinkNone:
			continue
		case config.TraceSinkOTLP:
			if telemetry.ProviderInitialized() {
				continue
			}
			return errors.Errorf(
				"trace sink %q requires an initialized OpenTelemetry provider "+
					"(OTEL_ENABLED=%t): none is installed, so every span would be handed "+
					"to the global no-op provider; initialize telemetry before trace sinks",
				config.TraceSinkOTLP, config.OpenTelemetryEnabled)
		default:
			return errors.Errorf("unknown trace sink %q", name)
		}
	}
	return nil
}

// Sink returns the installed trace sink, never nil.
//
// Parameters: none.
//
// Return values:
//   - TraceSink: the process sink; a drop-everything sink before InitSinks runs.
func Sink() TraceSink {
	activeSinkMu.RLock()
	s := activeSink
	activeSinkMu.RUnlock()
	if s == nil {
		return nullSink{}
	}
	return s
}

// SetSinkForTest installs a sink directly and returns a restore function.
//
// Parameters:
//   - s: the sink to install; nil installs the drop-everything sink.
//
// Return values:
//   - func(): restores the sink installed before this call.
func SetSinkForTest(s TraceSink) func() {
	activeSinkMu.Lock()
	prev := activeSink
	activeSink = s
	activeSinkMu.Unlock()
	return func() {
		activeSinkMu.Lock()
		activeSink = prev
		activeSinkMu.Unlock()
	}
}

// Flush blocks until the installed sink has handed off its buffered records.
//
// Parameters:
//   - ctx: deadline for the flush.
//
// Return values:
//   - error: wrapped failure reported by the sink, carrying the count of
//     records that did not persist; nil means everything accepted was stored.
func Flush(ctx context.Context) error {
	return Sink().Flush(ctx)
}

// Shutdown flushes and closes the installed sink. Call it during graceful
// shutdown, after the HTTP server stopped accepting requests.
//
// Parameters:
//   - ctx: deadline for the final flush.
//
// Return values:
//   - error: wrapped failure reported by the sink; a deadline that expires with
//     work outstanding reports how many accepted records were not persisted.
func Shutdown(ctx context.Context) error {
	return Sink().Close(ctx)
}

// nullSink discards every trace after metrics have been recorded.
type nullSink struct{}

// Submit implements TraceSink.Submit by dropping the record.
//
// Parameters:
//   - ctx: unused.
//   - row: the finished trace row.
//
// Return values:
//   - error: always nil.
func (nullSink) Submit(context.Context, *model.Trace) error { return nil }

// Flush implements TraceSink.Flush as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (nullSink) Flush(context.Context) error { return nil }

// Close implements TraceSink.Close as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (nullSink) Close(context.Context) error { return nil }

// multiSink fans a finished trace out to several sinks in order.
type multiSink struct {
	sinks []TraceSink
}

// Submit implements TraceSink.Submit by forwarding to every child sink.
//
// Parameters:
//   - ctx: forwarded to each child.
//   - row: the finished trace row; children must not mutate it.
//
// Return values:
//   - error: the first child failure, after all children were attempted.
func (m *multiSink) Submit(ctx context.Context, row *model.Trace) error {
	var firstErr error
	for _, s := range m.sinks {
		if err := s.Submit(ctx, row); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Flush implements TraceSink.Flush across every child sink.
//
// Parameters:
//   - ctx: forwarded to each child.
//
// Return values:
//   - error: the first child failure, after all children were attempted.
func (m *multiSink) Flush(ctx context.Context) error {
	var firstErr error
	for _, s := range m.sinks {
		if err := s.Flush(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Close implements TraceSink.Close across every child sink.
//
// Parameters:
//   - ctx: forwarded to each child.
//
// Return values:
//   - error: the first child failure, after all children were attempted.
func (m *multiSink) Close(ctx context.Context) error {
	var firstErr error
	for _, s := range m.sinks {
		if err := s.Close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// recordSubmitOutcome is a small helper so every sink reports the same
// bounded-label outcomes.
//
// Parameters:
//   - outcome: a compile-time constant from common/metrics/trace_pipeline.go.
//
// Return values: none.
func recordSubmitOutcome(outcome string) {
	metrics.RecordTraceOutcome(outcome, 1)
}
