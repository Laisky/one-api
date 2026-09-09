package otelbridge

// Provider installation for the OTLP application-log bridge (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.2).
//
// The bridge exists to solve an ordering problem that has no clean solution at
// the call site. main.go configures the logger (logger.SetupLogger and
// logger.SetupEnhancedLogger) long BEFORE it initializes OpenTelemetry, because
// telemetry initialization itself logs. A log core built at that moment cannot
// be handed a real log.LoggerProvider, and building one against the global
// no-op provider would silently export nothing forever -- exactly the failure
// mode common/telemetry's ProviderInitialized flag was introduced to prevent
// for the OTLP trace sink.
//
// A ProviderHolder is therefore a one-way switch. Cores are built against it at
// logger-setup time, they emit nothing while it is empty (counted, not
// silent), they emit for real once telemetry installs the provider, and they go
// quiet again once shutdown closes it. Nothing in the request path ever needs
// to know which state it is in.

import (
	"context"
	"sync"

	"github.com/Laisky/errors/v2"
	"go.opentelemetry.io/otel/log"

	"github.com/Laisky/one-api/common/metrics"
)

// providerState names what a ProviderHolder can do for a log record right now.
type providerState int

const (
	// providerPending means no real provider has been installed yet. Records are
	// counted as dropped_not_ready and are NOT exported; they still reach the
	// file and stdout sinks, which are configured first for exactly this reason.
	providerPending providerState = iota
	// providerReady means a real provider is installed and records are emitted.
	providerReady
	// providerClosed means the provider was shut down. Records are counted as
	// dropped_shutdown. This is a terminal state: a closed holder is never
	// reopened, because a provider that has been shut down cannot export and
	// re-installing one during shutdown would resurrect a pipeline the shutdown
	// sequence has already accounted for.
	providerClosed
)

// Flusher is the optional capability a log provider may offer to drain buffered
// records on demand.
//
// The OpenTelemetry log.LoggerProvider interface has no flush method; only the
// SDK implementation does. Declaring the capability here lets Core.Sync do the
// right thing without the bridge depending on the SDK, and keeps a test double
// free to omit it.
type Flusher interface {
	// ForceFlush blocks until buffered records are handed to the exporter or
	// ctx expires.
	ForceFlush(ctx context.Context) error
}

// ProviderHolder owns the log.LoggerProvider a set of Cores emit through, and
// the scope-name-keyed loggers derived from it.
//
// It is safe for concurrent use: cores read it on every log line while startup
// or shutdown may be installing or closing the provider.
type ProviderHolder struct {
	mu       sync.RWMutex
	state    providerState
	provider log.LoggerProvider
	loggers  map[string]log.Logger
}

// NewProviderHolder returns an empty holder whose cores drop records until a
// provider is installed.
//
// Parameters: none.
//
// Return values:
//   - *ProviderHolder: a holder in the pending state.
func NewProviderHolder() *ProviderHolder {
	return &ProviderHolder{loggers: make(map[string]log.Logger, 4)}
}

// Shared is the process-wide holder used by the application logger.
//
// It is a package variable rather than a parameter threaded through
// logger.SetupEnhancedLogger because the core is created before the provider
// exists and the two are wired in different phases of main.go. Tests build
// their own holder with NewProviderHolder instead of touching this one.
var Shared = NewProviderHolder()

// Install publishes a real logger provider, moving the holder from pending to
// ready so its cores begin exporting.
//
// Installing on a closed holder is refused: shutdown has already reported what
// it drained, and a late installation would start a pipeline nothing will close.
//
// Parameters:
//   - provider: the provider to publish; nil is rejected.
//
// Return values:
//   - error: when provider is nil or the holder is already closed.
func (h *ProviderHolder) Install(provider log.LoggerProvider) error {
	if provider == nil {
		return errors.New("otelbridge: refusing to install a nil logger provider")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.state == providerClosed {
		return errors.New("otelbridge: logger provider was already shut down")
	}

	h.provider = provider
	h.state = providerReady
	clear(h.loggers)
	return nil
}

// Close moves the holder to its terminal state so no further record is emitted
// through a provider the shutdown sequence is tearing down.
//
// It does NOT shut the provider down. Ownership of the provider lifecycle stays
// with whoever built it (common/telemetry), so that the shutdown sequence keeps
// a single ordered place where exporter drain deadlines are enforced and
// reported.
//
// Parameters: none.
//
// Return values: none.
func (h *ProviderHolder) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.state = providerClosed
	h.provider = nil
	clear(h.loggers)
}

// ResetForTest returns the holder to its pending state.
//
// It exists only so a test can reuse a holder across cases; production code has
// no reason to reopen a closed holder and must not call it.
//
// Parameters: none.
//
// Return values: none.
func (h *ProviderHolder) ResetForTest() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.state = providerPending
	h.provider = nil
	clear(h.loggers)
}

// Ready reports whether records handed to this holder are currently exported.
//
// Parameters: none.
//
// Return values:
//   - bool: true only between a successful Install and Close.
func (h *ProviderHolder) Ready() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.state == providerReady
}

// logger resolves the log.Logger for an instrumentation scope, caching it so a
// named zap logger does not pay a provider lookup on every line.
//
// Parameters:
//   - scope: the instrumentation scope name; normally the package import path,
//     or the zap logger's name when the entry carries one.
//
// Return values:
//   - log.Logger: the logger to emit through; nil unless state is providerReady.
//   - providerState: the holder's state at the moment of the lookup.
func (h *ProviderHolder) logger(scope string) (log.Logger, providerState) {
	h.mu.RLock()
	if h.state != providerReady {
		state := h.state
		h.mu.RUnlock()
		return nil, state
	}
	if lg, ok := h.loggers[scope]; ok {
		h.mu.RUnlock()
		return lg, providerReady
	}
	provider := h.provider
	h.mu.RUnlock()

	// Build outside the read lock, then publish under the write lock. Two
	// concurrent first-uses of the same scope can both build a logger; the SDK
	// returns an equivalent logger either way, so the loser is simply discarded
	// rather than serializing every miss behind the write lock.
	built := provider.Logger(scope)

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != providerReady {
		return nil, h.state
	}
	if lg, ok := h.loggers[scope]; ok {
		return lg, providerReady
	}
	h.loggers[scope] = built
	return built, providerReady
}

// flush drains the installed provider when it supports flushing.
//
// Parameters:
//   - ctx: bounds how long the drain may take.
//
// Return values:
//   - error: the provider's flush failure, or nil when the holder is not ready
//     or the provider cannot flush.
func (h *ProviderHolder) flush(ctx context.Context) error {
	h.mu.RLock()
	provider := h.provider
	ready := h.state == providerReady
	h.mu.RUnlock()

	if !ready || provider == nil {
		return nil
	}
	flusher, ok := provider.(Flusher)
	if !ok {
		return nil
	}
	return errors.Wrap(flusher.ForceFlush(ctx), "flush otel log provider")
}

// recordUnavailable counts a record that could not be exported because the
// holder was not ready.
//
// Parameters:
//   - state: the holder state observed when the record was written.
//
// Return values: none.
func recordUnavailable(state providerState) {
	switch state {
	case providerClosed:
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedShutdown, 1)
	default:
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedNotReady, 1)
	}
}
