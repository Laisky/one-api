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

// Enabled delegates the severity decision without retaining event text.
func (p *privacyLogProcessor) Enabled(ctx context.Context, param sdklog.EnabledParameters) bool {
	param.EventName = ""
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
	record.SetBody(attribute.StringValue("application log"))
	record.SetEventName("")
	severity := record.Severity()
	if severity < otellog.SeverityTrace || severity > otellog.SeverityFatal4 {
		severity = otellog.SeverityInfo
		record.SetSeverity(severity)
	}
	record.SetSeverityText(severity.String())
	var kept []attribute.KeyValue
	record.WalkAttributes(func(kv attribute.KeyValue) bool {
		if otelbridge.PrivateAttribute(kv) || (otelbridge.IsPrivateContext(ctx) && privateCallerAttribute(kv)) {
			kept = append(kept, kv)
		}
		return true
	})
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
	}
	return false
}

// ForceFlush delegates the drain to the bounded processor.
func (p *privacyLogProcessor) ForceFlush(ctx context.Context) error { return p.next.ForceFlush(ctx) }

// Shutdown delegates resource cleanup to the bounded processor.
func (p *privacyLogProcessor) Shutdown(ctx context.Context) error { return p.next.Shutdown(ctx) }
