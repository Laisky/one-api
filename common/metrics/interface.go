package metrics

import (
	"sync/atomic"
	"time"
)

// MetricsRecorder defines the interface for recording metrics
type MetricsRecorder interface {
	// HTTP metrics
	RecordHTTPRequest(startTime time.Time, path, method, statusCode string)
	RecordHTTPActiveRequest(path, method string, delta float64)

	// Relay metrics
	RecordRelayRequest(startTime time.Time, channelId int, channelType, model, userId, group, tokenId, apiFormat, apiType string, success bool, promptTokens, completionTokens int, quotaUsed float64)

	// Channel metrics
	UpdateChannelMetrics(channelId int, channelName, channelType string, status int, balance float64, responseTimeMs int, successRate float64)
	UpdateChannelRequestsInFlight(channelId int, channelName, channelType string, delta float64)

	// User metrics
	RecordUserMetrics(userId, username, group string, quotaUsed float64, promptTokens, completionTokens int, balance float64)

	// Database metrics
	RecordDBQuery(startTime time.Time, operation, table string, success bool)
	UpdateDBConnectionMetrics(inUse, idle int)

	// Redis metrics
	RecordRedisCommand(startTime time.Time, command string, success bool)
	UpdateRedisConnectionMetrics(active int)

	// Rate limit metrics
	RecordRateLimitHit(limitType, identifier string)
	UpdateRateLimitRemaining(limitType, identifier string, remaining int)

	// Authentication metrics
	RecordTokenAuth(success bool)
	UpdateActiveTokens(userId, tokenName string, count int)

	// Error metrics
	RecordError(errorType, component string)

	// Model metrics
	RecordModelUsage(modelName, channelType string, latency time.Duration)

	// Billing metrics
	RecordBillingOperation(startTime time.Time, operation string, success bool, userId int, channelId int, modelName string, quotaAmount float64)
	RecordBillingTimeout(userId int, channelId int, modelName string, estimatedQuota float64, elapsedTime time.Duration)
	RecordBillingError(errorType, operation string, userId int, channelId int, modelName string)
	UpdateBillingStats(totalBillingOperations, successfulBillingOperations, failedBillingOperations int64)

	// External UUID backfill metrics
	//
	// IMPORTANT: every label argument below (role, phase, target, mode, result)
	// MUST come from a compile-time registry constant. Never pass an ID, UUID,
	// DSN, error message, table row value, or any other unbounded value: these
	// arguments become metric labels, and an unbounded label explodes time
	// series cardinality.
	RecordUUIDBackfillRows(role, phase, target, result string, count int)
	UpdateUUIDBackfillBacklog(role, target string, backlog float64)
	RecordUUIDBackfillCycle(role, mode, result string, duration time.Duration)
	RecordUUIDBackfillFinalizer(role, result string)

	// Compact UUID storage metrics
	//
	// IMPORTANT: every label argument below (role, state, target, kind, action,
	// result, reason, operation) MUST come from a compile-time registry
	// constant. Never pass an ID, UUID, DSN, credential, row content,
	// fingerprint, or error message: these arguments become metric labels, and
	// an unbounded label explodes time series cardinality.
	UpdateCompactUUIDState(role, state string, active bool)
	UpdateCompactUUIDBacklog(role, target, kind string, rows float64)
	RecordCompactUUIDAction(role, action, result string)
	RecordCompactUUIDLookupFallback(role, reason string)
	UpdateCompactUUIDLastProgress(role string, unixTime float64)
	RecordCompactUUIDDuration(role, operation string, duration time.Duration)

	// Gateway response-state metrics
	//
	// IMPORTANT: both label arguments (category, outcome) MUST come from a
	// compile-time registry constant (see response_state.go). Never pass a
	// gateway response/conversation id, prompt, model, error message, or any
	// other unbounded value: these become metric labels and an unbounded label
	// explodes time series cardinality.
	RecordResponseStateEvent(category, outcome string)

	// System metrics
	InitSystemMetrics(version, buildTime, goVersion string, startTime time.Time)
	UpdateSiteWideStats(totalQuota, usedQuota int64, totalUsers, activeUsers int)
}

// globalRecorder holds the active metrics recorder implementation.
//
// It is an atomic.Value rather than a plain variable because it is read from
// every request goroutine and from long-lived background workers (the UUID
// migration coordinator among them) while it can still be written: monitor.Init
// installs the real recorder after those workers may already be running, and
// tests swap it per-case. As a bare var that is a data race, and -race caught it
// as one between model.uuidMetricsRecorder and a test installing a recorder.
//
// A recorderBox wrapper keeps the stored dynamic type constant; storing differing
// concrete types directly in an atomic.Value panics.
var globalRecorder atomic.Value

// recorderBox pins one concrete type for atomic.Value storage.
type recorderBox struct{ recorder MetricsRecorder }

// Recorder returns the active metrics recorder, never nil.
//
// Return values:
//   - MetricsRecorder: the installed recorder, or a no-op when none is installed.
func Recorder() MetricsRecorder {
	if box, ok := globalRecorder.Load().(recorderBox); ok && box.recorder != nil {
		return box.recorder
	}
	return &NoOpRecorder{}
}

// SetRecorder installs the process-wide metrics recorder.
//
// Parameters:
//   - recorder: the recorder to install; nil restores the no-op behavior.
func SetRecorder(recorder MetricsRecorder) {
	globalRecorder.Store(recorderBox{recorder: recorder})
}

// NoOpRecorder is a no-operation implementation for when metrics are disabled
type NoOpRecorder struct{}

// RecordHTTPRequest implements MetricsRecorder.RecordHTTPRequest without collecting any data.
func (n *NoOpRecorder) RecordHTTPRequest(startTime time.Time, path, method, statusCode string) {}

// RecordHTTPActiveRequest implements MetricsRecorder.RecordHTTPActiveRequest without collecting any data.
func (n *NoOpRecorder) RecordHTTPActiveRequest(path, method string, delta float64) {}

// RecordRelayRequest implements MetricsRecorder.RecordRelayRequest without collecting any data.
func (n *NoOpRecorder) RecordRelayRequest(startTime time.Time, channelId int, channelType, model, userId, group, tokenId, apiFormat, apiType string, success bool, promptTokens, completionTokens int, quotaUsed float64) {
}

// UpdateChannelMetrics implements MetricsRecorder.UpdateChannelMetrics without collecting any data.
func (n *NoOpRecorder) UpdateChannelMetrics(channelId int, channelName, channelType string, status int, balance float64, responseTimeMs int, successRate float64) {
}

// UpdateChannelRequestsInFlight implements MetricsRecorder.UpdateChannelRequestsInFlight without collecting any data.
func (n *NoOpRecorder) UpdateChannelRequestsInFlight(channelId int, channelName, channelType string, delta float64) {
}

// RecordUserMetrics implements MetricsRecorder.RecordUserMetrics without collecting any data.
func (n *NoOpRecorder) RecordUserMetrics(userId, username, group string, quotaUsed float64, promptTokens, completionTokens int, balance float64) {
}

// RecordDBQuery implements MetricsRecorder.RecordDBQuery without collecting any data.
func (n *NoOpRecorder) RecordDBQuery(startTime time.Time, operation, table string, success bool) {}

// UpdateDBConnectionMetrics implements MetricsRecorder.UpdateDBConnectionMetrics without collecting any data.
func (n *NoOpRecorder) UpdateDBConnectionMetrics(inUse, idle int) {}

// RecordRedisCommand implements MetricsRecorder.RecordRedisCommand without collecting any data.
func (n *NoOpRecorder) RecordRedisCommand(startTime time.Time, command string, success bool) {}

// UpdateRedisConnectionMetrics implements MetricsRecorder.UpdateRedisConnectionMetrics without collecting any data.
func (n *NoOpRecorder) UpdateRedisConnectionMetrics(active int) {}

// RecordRateLimitHit implements MetricsRecorder.RecordRateLimitHit without collecting any data.
func (n *NoOpRecorder) RecordRateLimitHit(limitType, identifier string) {}

// UpdateRateLimitRemaining implements MetricsRecorder.UpdateRateLimitRemaining without collecting any data.
func (n *NoOpRecorder) UpdateRateLimitRemaining(limitType, identifier string, remaining int) {}

// RecordTokenAuth implements MetricsRecorder.RecordTokenAuth without collecting any data.
func (n *NoOpRecorder) RecordTokenAuth(success bool) {}

// UpdateActiveTokens implements MetricsRecorder.UpdateActiveTokens without collecting any data.
func (n *NoOpRecorder) UpdateActiveTokens(userId, tokenName string, count int) {}

// RecordError implements MetricsRecorder.RecordError without collecting any data.
func (n *NoOpRecorder) RecordError(errorType, component string) {}

// RecordModelUsage implements MetricsRecorder.RecordModelUsage without collecting any data.
func (n *NoOpRecorder) RecordModelUsage(modelName, channelType string, latency time.Duration) {}

// RecordBillingOperation implements MetricsRecorder.RecordBillingOperation without collecting any data.
func (n *NoOpRecorder) RecordBillingOperation(startTime time.Time, operation string, success bool, userId int, channelId int, modelName string, quotaAmount float64) {
}

// RecordBillingTimeout implements MetricsRecorder.RecordBillingTimeout without collecting any data.
func (n *NoOpRecorder) RecordBillingTimeout(userId int, channelId int, modelName string, estimatedQuota float64, elapsedTime time.Duration) {
}

// RecordBillingError implements MetricsRecorder.RecordBillingError without collecting any data.
func (n *NoOpRecorder) RecordBillingError(errorType, operation string, userId int, channelId int, modelName string) {
}

// UpdateBillingStats implements MetricsRecorder.UpdateBillingStats without collecting any data.
func (n *NoOpRecorder) UpdateBillingStats(totalBillingOperations, successfulBillingOperations, failedBillingOperations int64) {
}

// RecordUUIDBackfillRows implements MetricsRecorder.RecordUUIDBackfillRows without collecting any data.
func (n *NoOpRecorder) RecordUUIDBackfillRows(role, phase, target, result string, count int) {}

// UpdateUUIDBackfillBacklog implements MetricsRecorder.UpdateUUIDBackfillBacklog without collecting any data.
func (n *NoOpRecorder) UpdateUUIDBackfillBacklog(role, target string, backlog float64) {}

// RecordUUIDBackfillCycle implements MetricsRecorder.RecordUUIDBackfillCycle without collecting any data.
func (n *NoOpRecorder) RecordUUIDBackfillCycle(role, mode, result string, duration time.Duration) {}

// RecordUUIDBackfillFinalizer implements MetricsRecorder.RecordUUIDBackfillFinalizer without collecting any data.
func (n *NoOpRecorder) RecordUUIDBackfillFinalizer(role, result string) {}

// UpdateCompactUUIDState implements MetricsRecorder.UpdateCompactUUIDState without collecting any data.
func (n *NoOpRecorder) UpdateCompactUUIDState(role, state string, active bool) {}

// UpdateCompactUUIDBacklog implements MetricsRecorder.UpdateCompactUUIDBacklog without collecting any data.
func (n *NoOpRecorder) UpdateCompactUUIDBacklog(role, target, kind string, rows float64) {}

// RecordCompactUUIDAction implements MetricsRecorder.RecordCompactUUIDAction without collecting any data.
func (n *NoOpRecorder) RecordCompactUUIDAction(role, action, result string) {}

// RecordCompactUUIDLookupFallback implements MetricsRecorder.RecordCompactUUIDLookupFallback without collecting any data.
func (n *NoOpRecorder) RecordCompactUUIDLookupFallback(role, reason string) {}

// UpdateCompactUUIDLastProgress implements MetricsRecorder.UpdateCompactUUIDLastProgress without collecting any data.
func (n *NoOpRecorder) UpdateCompactUUIDLastProgress(role string, unixTime float64) {}

// RecordCompactUUIDDuration implements MetricsRecorder.RecordCompactUUIDDuration without collecting any data.
func (n *NoOpRecorder) RecordCompactUUIDDuration(role, operation string, duration time.Duration) {}

// InitSystemMetrics implements MetricsRecorder.InitSystemMetrics without collecting any data.
func (n *NoOpRecorder) InitSystemMetrics(version, buildTime, goVersion string, startTime time.Time) {}

// UpdateSiteWideStats implements MetricsRecorder.UpdateSiteWideStats without collecting any data.
func (n *NoOpRecorder) UpdateSiteWideStats(totalQuota, usedQuota int64, totalUsers, activeUsers int) {
}

// RecordResponseStateEvent implements MetricsRecorder.RecordResponseStateEvent without collecting any data.
func (n *NoOpRecorder) RecordResponseStateEvent(category, outcome string) {}

// Initialize with no-op recorder by default
func init() {
	SetRecorder(&NoOpRecorder{})
}

// MultiRecorder wraps multiple MetricsRecorder implementations
type MultiRecorder struct {
	Recorders []MetricsRecorder
}

// RecordHTTPRequest implements MetricsRecorder.RecordHTTPRequest
func (m *MultiRecorder) RecordHTTPRequest(startTime time.Time, path, method, statusCode string) {
	for _, r := range m.Recorders {
		r.RecordHTTPRequest(startTime, path, method, statusCode)
	}
}

// RecordHTTPActiveRequest implements MetricsRecorder.RecordHTTPActiveRequest
func (m *MultiRecorder) RecordHTTPActiveRequest(path, method string, delta float64) {
	for _, r := range m.Recorders {
		r.RecordHTTPActiveRequest(path, method, delta)
	}
}

// RecordRelayRequest implements MetricsRecorder.RecordRelayRequest
func (m *MultiRecorder) RecordRelayRequest(startTime time.Time, channelId int, channelType, model, userId, group, tokenId, apiFormat, apiType string, success bool, promptTokens, completionTokens int, quotaUsed float64) {
	for _, r := range m.Recorders {
		r.RecordRelayRequest(startTime, channelId, channelType, model, userId, group, tokenId, apiFormat, apiType, success, promptTokens, completionTokens, quotaUsed)
	}
}

// UpdateChannelMetrics implements MetricsRecorder.UpdateChannelMetrics
func (m *MultiRecorder) UpdateChannelMetrics(channelId int, channelName, channelType string, status int, balance float64, responseTimeMs int, successRate float64) {
	for _, r := range m.Recorders {
		r.UpdateChannelMetrics(channelId, channelName, channelType, status, balance, responseTimeMs, successRate)
	}
}

// UpdateChannelRequestsInFlight implements MetricsRecorder.UpdateChannelRequestsInFlight
func (m *MultiRecorder) UpdateChannelRequestsInFlight(channelId int, channelName, channelType string, delta float64) {
	for _, r := range m.Recorders {
		r.UpdateChannelRequestsInFlight(channelId, channelName, channelType, delta)
	}
}

// RecordUserMetrics implements MetricsRecorder.RecordUserMetrics
func (m *MultiRecorder) RecordUserMetrics(userId, username, group string, quotaUsed float64, promptTokens, completionTokens int, balance float64) {
	for _, r := range m.Recorders {
		r.RecordUserMetrics(userId, username, group, quotaUsed, promptTokens, completionTokens, balance)
	}
}

// RecordDBQuery implements MetricsRecorder.RecordDBQuery
func (m *MultiRecorder) RecordDBQuery(startTime time.Time, operation, table string, success bool) {
	for _, r := range m.Recorders {
		r.RecordDBQuery(startTime, operation, table, success)
	}
}

// UpdateDBConnectionMetrics implements MetricsRecorder.UpdateDBConnectionMetrics
func (m *MultiRecorder) UpdateDBConnectionMetrics(inUse, idle int) {
	for _, r := range m.Recorders {
		r.UpdateDBConnectionMetrics(inUse, idle)
	}
}

// RecordRedisCommand implements MetricsRecorder.RecordRedisCommand
func (m *MultiRecorder) RecordRedisCommand(startTime time.Time, command string, success bool) {
	for _, r := range m.Recorders {
		r.RecordRedisCommand(startTime, command, success)
	}
}

// UpdateRedisConnectionMetrics implements MetricsRecorder.UpdateRedisConnectionMetrics
func (m *MultiRecorder) UpdateRedisConnectionMetrics(active int) {
	for _, r := range m.Recorders {
		r.UpdateRedisConnectionMetrics(active)
	}
}

// RecordRateLimitHit implements MetricsRecorder.RecordRateLimitHit
func (m *MultiRecorder) RecordRateLimitHit(limitType, identifier string) {
	for _, r := range m.Recorders {
		r.RecordRateLimitHit(limitType, identifier)
	}
}

// UpdateRateLimitRemaining implements MetricsRecorder.UpdateRateLimitRemaining
func (m *MultiRecorder) UpdateRateLimitRemaining(limitType, identifier string, remaining int) {
	for _, r := range m.Recorders {
		r.UpdateRateLimitRemaining(limitType, identifier, remaining)
	}
}

// RecordTokenAuth implements MetricsRecorder.RecordTokenAuth
func (m *MultiRecorder) RecordTokenAuth(success bool) {
	for _, r := range m.Recorders {
		r.RecordTokenAuth(success)
	}
}

// UpdateActiveTokens implements MetricsRecorder.UpdateActiveTokens
func (m *MultiRecorder) UpdateActiveTokens(userId, tokenName string, count int) {
	for _, r := range m.Recorders {
		r.UpdateActiveTokens(userId, tokenName, count)
	}
}

// RecordError implements MetricsRecorder.RecordError
func (m *MultiRecorder) RecordError(errorType, component string) {
	for _, r := range m.Recorders {
		r.RecordError(errorType, component)
	}
}

// RecordModelUsage implements MetricsRecorder.RecordModelUsage
func (m *MultiRecorder) RecordModelUsage(modelName, channelType string, latency time.Duration) {
	for _, r := range m.Recorders {
		r.RecordModelUsage(modelName, channelType, latency)
	}
}

// RecordBillingOperation implements MetricsRecorder.RecordBillingOperation
func (m *MultiRecorder) RecordBillingOperation(startTime time.Time, operation string, success bool, userId int, channelId int, modelName string, quotaAmount float64) {
	for _, r := range m.Recorders {
		r.RecordBillingOperation(startTime, operation, success, userId, channelId, modelName, quotaAmount)
	}
}

// RecordBillingTimeout implements MetricsRecorder.RecordBillingTimeout
func (m *MultiRecorder) RecordBillingTimeout(userId int, channelId int, modelName string, estimatedQuota float64, elapsedTime time.Duration) {
	for _, r := range m.Recorders {
		r.RecordBillingTimeout(userId, channelId, modelName, estimatedQuota, elapsedTime)
	}
}

// RecordBillingError implements MetricsRecorder.RecordBillingError
func (m *MultiRecorder) RecordBillingError(errorType, operation string, userId int, channelId int, modelName string) {
	for _, r := range m.Recorders {
		r.RecordBillingError(errorType, operation, userId, channelId, modelName)
	}
}

// UpdateBillingStats implements MetricsRecorder.UpdateBillingStats
func (m *MultiRecorder) UpdateBillingStats(totalBillingOperations, successfulBillingOperations, failedBillingOperations int64) {
	for _, r := range m.Recorders {
		r.UpdateBillingStats(totalBillingOperations, successfulBillingOperations, failedBillingOperations)
	}
}

// RecordUUIDBackfillRows implements MetricsRecorder.RecordUUIDBackfillRows
func (m *MultiRecorder) RecordUUIDBackfillRows(role, phase, target, result string, count int) {
	for _, r := range m.Recorders {
		r.RecordUUIDBackfillRows(role, phase, target, result, count)
	}
}

// UpdateUUIDBackfillBacklog implements MetricsRecorder.UpdateUUIDBackfillBacklog
func (m *MultiRecorder) UpdateUUIDBackfillBacklog(role, target string, backlog float64) {
	for _, r := range m.Recorders {
		r.UpdateUUIDBackfillBacklog(role, target, backlog)
	}
}

// RecordUUIDBackfillCycle implements MetricsRecorder.RecordUUIDBackfillCycle
func (m *MultiRecorder) RecordUUIDBackfillCycle(role, mode, result string, duration time.Duration) {
	for _, r := range m.Recorders {
		r.RecordUUIDBackfillCycle(role, mode, result, duration)
	}
}

// RecordUUIDBackfillFinalizer implements MetricsRecorder.RecordUUIDBackfillFinalizer
func (m *MultiRecorder) RecordUUIDBackfillFinalizer(role, result string) {
	for _, r := range m.Recorders {
		r.RecordUUIDBackfillFinalizer(role, result)
	}
}

// UpdateCompactUUIDState implements MetricsRecorder.UpdateCompactUUIDState
func (m *MultiRecorder) UpdateCompactUUIDState(role, state string, active bool) {
	for _, r := range m.Recorders {
		r.UpdateCompactUUIDState(role, state, active)
	}
}

// UpdateCompactUUIDBacklog implements MetricsRecorder.UpdateCompactUUIDBacklog
func (m *MultiRecorder) UpdateCompactUUIDBacklog(role, target, kind string, rows float64) {
	for _, r := range m.Recorders {
		r.UpdateCompactUUIDBacklog(role, target, kind, rows)
	}
}

// RecordCompactUUIDAction implements MetricsRecorder.RecordCompactUUIDAction
func (m *MultiRecorder) RecordCompactUUIDAction(role, action, result string) {
	for _, r := range m.Recorders {
		r.RecordCompactUUIDAction(role, action, result)
	}
}

// RecordCompactUUIDLookupFallback implements MetricsRecorder.RecordCompactUUIDLookupFallback
func (m *MultiRecorder) RecordCompactUUIDLookupFallback(role, reason string) {
	for _, r := range m.Recorders {
		r.RecordCompactUUIDLookupFallback(role, reason)
	}
}

// UpdateCompactUUIDLastProgress implements MetricsRecorder.UpdateCompactUUIDLastProgress
func (m *MultiRecorder) UpdateCompactUUIDLastProgress(role string, unixTime float64) {
	for _, r := range m.Recorders {
		r.UpdateCompactUUIDLastProgress(role, unixTime)
	}
}

// RecordCompactUUIDDuration implements MetricsRecorder.RecordCompactUUIDDuration
func (m *MultiRecorder) RecordCompactUUIDDuration(role, operation string, duration time.Duration) {
	for _, r := range m.Recorders {
		r.RecordCompactUUIDDuration(role, operation, duration)
	}
}

// InitSystemMetrics implements MetricsRecorder.InitSystemMetrics
func (m *MultiRecorder) InitSystemMetrics(version, buildTime, goVersion string, startTime time.Time) {
	for _, r := range m.Recorders {
		r.InitSystemMetrics(version, buildTime, goVersion, startTime)
	}
}

// UpdateSiteWideStats implements MetricsRecorder.UpdateSiteWideStats
func (m *MultiRecorder) UpdateSiteWideStats(totalQuota, usedQuota int64, totalUsers, activeUsers int) {
	for _, r := range m.Recorders {
		r.UpdateSiteWideStats(totalQuota, usedQuota, totalUsers, activeUsers)
	}
}

// RecordResponseStateEvent implements MetricsRecorder.RecordResponseStateEvent
func (m *MultiRecorder) RecordResponseStateEvent(category, outcome string) {
	for _, r := range m.Recorders {
		r.RecordResponseStateEvent(category, outcome)
	}
}
