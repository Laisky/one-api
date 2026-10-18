package telemetry

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"

	"github.com/Laisky/one-api/common/logger/otelbridge"
	"github.com/Laisky/one-api/common/metrics"
)

// privacyLoggerProvider maps public SDK producers to a fixed scope before the
// SDK record is created. User-supplied scope names/options never enter a queue.
type privacyLoggerProvider struct{ otellog.LoggerProvider }

// Logger returns a canonical logger, deliberately ignoring untrusted options.
func (p privacyLoggerProvider) Logger(_ string, _ ...otellog.LoggerOption) otellog.Logger {
	return p.LoggerProvider.Logger(otelbridge.DefaultScopeName)
}

// privacyLogProcessor filters every SDK record before bounded admission,
// including direct global-provider Emit calls that bypass the Zap bridge.
type privacyLogProcessor struct {
	next         sdklog.Processor
	resource     *sdkresource.Resource
	resourceOnce sync.Once
	resourceSafe bool
}

// Enabled delegates filtering with the same bounded sanitized event as emission.
func (p *privacyLogProcessor) Enabled(ctx context.Context, param sdklog.EnabledParameters) bool {
	param.EventName = otelbridge.SanitizeLogText(param.EventName)
	return p.next.Enabled(ctx, param)
}

// OnEmit replaces untrusted text and filters values before queue accounting.
// Immutable unsafe SDK scope/resource metadata causes a counted privacy drop;
// the canonical public provider avoids that drop for ordinary direct producers.
func (p *privacyLogProcessor) OnEmit(ctx context.Context, record *sdklog.Record) error {
	if record == nil {
		return nil
	}
	// The SDK merges OTEL_RESOURCE_ATTRIBUTES even when WithResource is supplied.
	// Validate the actual immutable provider resource once, before first admission.
	p.resourceOnce.Do(func() {
		p.resource = record.Resource()
		p.resourceSafe = p.resource.Equal(privateResource(p.resource))
	})
	scope := record.InstrumentationScope()
	if scope.Name != otelbridge.DefaultScopeName || scope.Version != "" || scope.SchemaURL != "" || scope.Attributes.Len() != 0 ||
		!p.resourceSafe || record.Resource() != p.resource {
		metrics.RecordAppLogExport(metrics.AppLogExportOutcomeDroppedPrivacy, 1)
		return nil
	}
	if record.Body().Type() == attribute.STRING {
		record.SetBody(attribute.StringValue(otelbridge.SanitizeLogText(record.Body().AsString())))
	} else {
		record.SetBody(attribute.StringValue("[unsupported log body]"))
	}
	record.SetEventName(otelbridge.SanitizeLogText(record.EventName()))
	severity := record.Severity()
	if severity < otellog.SeverityTrace || severity > otellog.SeverityFatal4 {
		severity = otellog.SeverityInfo
		record.SetSeverity(severity)
	}
	record.SetSeverityText(severity.String())
	var kept []attribute.KeyValue
	omitted := false
	record.WalkAttributes(func(kv attribute.KeyValue) bool {
		if string(kv.Key) == "code.file.path" || string(kv.Key) == "code.function.name" || string(kv.Key) == "code.line.number" || string(kv.Key) == "log.fields_omitted" {
			if otelbridge.IsPrivateContext(ctx) && privateCallerAttribute(kv) {
				kept = append(kept, kv)
			} else {
				omitted = true
			}
			return true
		}
		if safe, ok := otelbridge.SanitizeLogAttribute(kv); ok {
			kept = append(kept, safe)
		} else {
			omitted = true
		}
		return true
	})
	if omitted {
		kept = append(kept, attribute.Bool("log.fields_omitted", true))
	}
	record.SetAttributes(kept...)
	return p.next.OnEmit(ctx, record)
}

// privateCallerAttribute accepts only core-derived caller location metadata.
func privateCallerAttribute(kv attribute.KeyValue) bool {
	switch string(kv.Key) {
	case "code.file.path", "code.function.name":
		return kv.Value.Type() == attribute.STRING
	case "code.line.number":
		return kv.Value.Type() == attribute.INT64 && kv.Value.AsInt64() >= 0
	case "log.fields_omitted":
		return kv.Value.Type() == attribute.BOOL && kv.Value.AsBool()
	}
	return false
}

// ForceFlush delegates the drain to the bounded processor.
func (p *privacyLogProcessor) ForceFlush(ctx context.Context) error { return p.next.ForceFlush(ctx) }

// Shutdown delegates resource cleanup to the bounded processor.
func (p *privacyLogProcessor) Shutdown(ctx context.Context) error { return p.next.Shutdown(ctx) }
