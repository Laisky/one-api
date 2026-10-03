package config

// Optional OTLP application-log settings (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.2, and
// section 3.3 "APP_LOG_SINK=otlp | Optional new value after W3 adapter
// integration passes").
//
// Two rules shape every default here.
//
// First, a setting that changes what an operator observes defaults to the
// legacy behavior. APP_LOG_SINK keeps its meaning byte for byte: without the
// new token, this file changes nothing at all and no log exporter is built.
//
// Second, a setting whose ABSENCE is a defect defaults to enabled and
// generously sized. Every bound below is on rather than unlimited, sized so it
// does not engage in healthy operation, because the deployment running pure
// defaults is the least able to discover an unbounded queue.

import (
	"strings"

	"github.com/Laisky/one-api/common/env"
)

// AppLogSinkOTLP is the additive APP_LOG_SINK token that turns on the OTLP
// application-log bridge.
//
// It is ADDITIVE ONLY: "both,otlp" and "stdout,otlp" are valid, a bare "otlp"
// is rejected at startup. The bridge is best-effort by construction -- it drops
// on a full queue, before the provider is installed, and after it is shut down
// -- so making it the only destination would leave a gateway whose sole log
// record can be discarded without a trace. Section 2.1 requires the existing
// file and stdout selection to survive; this keeps a local sink of record.
const AppLogSinkOTLP = "otlp"

// Application-log severity floor names accepted by LOG_OTLP_MIN_LEVEL.
const (
	// AppLogOTLPLevelDebug exports everything the process logger admits.
	AppLogOTLPLevelDebug = "debug"
	// AppLogOTLPLevelInfo is the default floor.
	AppLogOTLPLevelInfo = "info"
	// AppLogOTLPLevelWarn exports warnings and errors only.
	AppLogOTLPLevelWarn = "warn"
	// AppLogOTLPLevelError exports errors only.
	AppLogOTLPLevelError = "error"
)

var (
	// AppLogOTLPEnabled reports whether APP_LOG_SINK asked for the OTLP
	// application-log bridge.
	//
	// Environment variable: APP_LOG_SINK (the "otlp" token)
	// Default: false for every profile, including external. The external
	// profile moves TRACES off the database; it does not silently start
	// shipping application logs to a collector.
	AppLogOTLPEnabled = appLogSinkHasOTLP(env.String(EnvAppLogSink, AppLogSinkBoth))

	// AppLogOTLPMinLevel is the severity floor for exported records, applied on
	// the zap side so a filtered record is never converted or queued.
	//
	// It can only RAISE the threshold, never lower it: a gateway running at
	// debug for local diagnosis should not start shipping debug volume to a
	// shared collector, and conversely a floor below the process log level would
	// make the whole process construct entries it is configured not to log.
	// common/logger.effectiveBridgeLevel enforces the clamp.
	//
	// Environment variable: LOG_OTLP_MIN_LEVEL
	// Default: "info"
	AppLogOTLPMinLevel = normalizeAppLogOTLPLevel(env.String(EnvAppLogOTLPMinLevel, AppLogOTLPLevelInfo))

	// AppLogOTLPQueueSize is the maximum number of log records resident in the
	// export pipeline before new records are dropped and counted.
	//
	// The drop is deliberate and must stay: OnEmit runs on the goroutine that
	// called the logger, which in a gateway is a request goroutine. Blocking it
	// on collector I/O would convert a telemetry outage into a relay outage.
	//
	// Environment variable: LOG_OTLP_QUEUE_SIZE
	// Default: 10000 records
	AppLogOTLPQueueSize = positiveOr(env.Int(EnvAppLogOTLPQueueSize, 10000), 10000)

	// AppLogOTLPQueueMaxMB bounds the same queue in bytes, because record count
	// alone does not bound memory: one log line carrying a large field can be
	// orders of magnitude bigger than the median line.
	//
	// Environment variable: LOG_OTLP_QUEUE_MAX_MB
	// Default: 64 (MiB)
	// Unit: mebibytes
	AppLogOTLPQueueMaxMB = positiveOr(env.Int(EnvAppLogOTLPQueueMaxMB, 64), 64)

	// AppLogOTLPBatchSize is the maximum number of records per export request.
	//
	// Environment variable: LOG_OTLP_BATCH_SIZE
	// Default: 512, the OpenTelemetry SDK default
	AppLogOTLPBatchSize = positiveOr(env.Int(EnvAppLogOTLPBatchSize, 512), 512)

	// AppLogOTLPExportIntervalMs is how often a partial batch is exported.
	//
	// Environment variable: LOG_OTLP_EXPORT_INTERVAL_MS
	// Default: 1000, the OpenTelemetry SDK default
	AppLogOTLPExportIntervalMs = positiveOr(env.Int(EnvAppLogOTLPExportIntervalMs, 1000), 1000)

	// AppLogOTLPExportTimeoutMs bounds a single export request.
	//
	// The shutdown deadline must exceed this value or the final drain is cut
	// short; the shutdown step documents that relationship.
	//
	// Environment variable: LOG_OTLP_EXPORT_TIMEOUT_MS
	// Default: 30000, the OpenTelemetry SDK default
	AppLogOTLPExportTimeoutMs = positiveOr(env.Int(EnvAppLogOTLPExportTimeoutMs, 30000), 30000)

	// AppLogOTLPMaxAttributes bounds the attributes kept on one record.
	//
	// Environment variable: LOG_OTLP_MAX_ATTRIBUTES
	// Default: 128, the OpenTelemetry SDK default
	AppLogOTLPMaxAttributes = positiveOr(env.Int(EnvAppLogOTLPMaxAttributes, 128), 128)

	// AppLogOTLPMaxAttributeValueBytes bounds one attribute value.
	//
	// The SDK default is unlimited. Unlimited is not a conservative default for
	// a resource bound; a single relay adaptor logging an upstream payload
	// would otherwise put an unbounded string on the wire. 4 KiB keeps ordinary
	// fields (URLs, model names, error text) intact.
	//
	// Environment variable: LOG_OTLP_MAX_ATTRIBUTE_VALUE_BYTES
	// Default: 4096
	// Unit: bytes
	AppLogOTLPMaxAttributeValueBytes = positiveOr(env.Int(EnvAppLogOTLPMaxAttributeValueBytes, 4096), 4096)
)

// AppLogOTLPQueueMaxBytes returns the configured queue byte ceiling.
//
// Parameters: none.
//
// Return values:
//   - int64: the ceiling in bytes, converted from mebibytes.
func AppLogOTLPQueueMaxBytes() int64 {
	return int64(AppLogOTLPQueueMaxMB) << 20
}

// appLogSinkHasOTLP reports whether a raw APP_LOG_SINK value contains the otlp
// token.
//
// Parameters:
//   - raw: the raw APP_LOG_SINK value, possibly a comma-separated list.
//
// Return values:
//   - bool: true when one of the tokens is "otlp".
func appLogSinkHasOTLP(raw string) bool {
	for token := range strings.SplitSeq(raw, ",") {
		if strings.EqualFold(strings.TrimSpace(token), AppLogSinkOTLP) {
			return true
		}
	}
	return false
}

// appLogSinkBaseTokens returns the non-otlp tokens of a raw APP_LOG_SINK value.
//
// Parameters:
//   - raw: the raw APP_LOG_SINK value, possibly a comma-separated list.
//
// Return values:
//   - []string: the lowercase, trimmed tokens that are not "otlp"; empty
//     entries are dropped so "both," parses as "both".
func appLogSinkBaseTokens(raw string) []string {
	var tokens []string
	for token := range strings.SplitSeq(raw, ",") {
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" || token == AppLogSinkOTLP {
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens
}

// normalizeAppLogOTLPLevel lowercases and validates a severity floor name.
//
// Parameters:
//   - raw: the raw LOG_OTLP_MIN_LEVEL value.
//
// Return values:
//   - string: one of the AppLogOTLPLevel* constants; unrecognized input falls
//     back to "info". Raw validation rejects a typo before this is reached.
func normalizeAppLogOTLPLevel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case AppLogOTLPLevelDebug:
		return AppLogOTLPLevelDebug
	case AppLogOTLPLevelWarn:
		return AppLogOTLPLevelWarn
	case AppLogOTLPLevelError:
		return AppLogOTLPLevelError
	default:
		return AppLogOTLPLevelInfo
	}
}
